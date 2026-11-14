package treesitter

import (
	"iter"
	"slices"
)

// Index supports repeated kind/symbol and position lookups in one Snapshot (ADR-0011). It is immutable and safe for
// concurrent readers; after an edit take a new Snapshot and a new Index. Building costs O(n log n) time and about
// 24 bytes per node; type lookups cost O(matches), position lookups O(log n) plus the overlapping spans.
type Index struct {
	s     *Snapshot
	bySym map[Symbol][]int32 // preorder indexes per symbol
	spans []indexedSpan      // sorted by start, then node; maxEnd is an implicit balanced-tree augmentation
}

type indexedSpan struct {
	start, end, maxEnd uint32
	node, depth        int32
}

// NewIndex builds an Index over s.
func NewIndex(s *Snapshot) *Index {
	x := &Index{s: s, bySym: map[Symbol][]int32{}, spans: make([]indexedSpan, s.n)}
	for i := 0; i < s.n; i++ {
		d := int32(0)
		if p := s.parent[i]; p >= 0 {
			d = x.spans[p].depth + 1 // parents precede children in preorder, so x.spans[p] is still in node order
		}
		x.spans[i] = indexedSpan{start: s.startByte[i], end: s.endByte[i], node: int32(i), depth: d}
		sym := Symbol(s.symbol[i])
		x.bySym[sym] = append(x.bySym[sym], int32(i))
	}
	slices.SortFunc(x.spans, func(a, b indexedSpan) int {
		if a.start != b.start {
			return int(int64(a.start) - int64(b.start))
		}
		return int(a.node - b.node)
	})
	var augment func(lo, hi int) uint32
	augment = func(lo, hi int) uint32 {
		if lo == hi {
			return 0
		}
		mid := lo + (hi-lo)/2
		sp := &x.spans[mid]
		sp.maxEnd = max(sp.end, augment(lo, mid), augment(mid+1, hi))
		return sp.maxEnd
	}
	augment(0, len(x.spans))
	return x
}

// Index is the snapshot's Index, built once on first use.
func (s *Snapshot) Index() *Index {
	s.indexOnce.Do(func() { s.index = NewIndex(s) })
	return s.index
}

// OfSymbol yields the nodes with symbol sym in preorder, without allocating a result slice. An unknown symbol yields
// nothing.
func (x *Index) OfSymbol(sym Symbol) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for _, i := range x.bySym[sym] {
			if !yield(Node{x.s, i}) {
				return
			}
		}
	}
}

// OfType yields the nodes whose Kind is kind in preorder (named and anonymous symbols sharing the name are merged).
// An unknown kind yields nothing.
func (x *Index) OfType(kind string) iter.Seq[Node] {
	syms := x.s.lang.symbolsNamed(kind)
	return func(yield func(Node) bool) {
		if len(syms) == 1 {
			for _, i := range x.bySym[syms[0]] {
				if !yield(Node{x.s, i}) {
					return
				}
			}
			return
		}
		var all []int32
		for _, sym := range syms {
			all = append(all, x.bySym[sym]...)
		}
		slices.Sort(all)
		for _, i := range all {
			if !yield(Node{x.s, i}) {
				return
			}
		}
	}
}

// NodeAt returns the deepest node whose half-open byte range [start, end) contains offset; equal-depth overlaps pick
// the earlier preorder node. Zero-width nodes (MISSING) and the end-of-file position match nothing; find missing
// nodes with OfType.
func (x *Index) NodeAt(offset uint32) (Node, bool) {
	best, depth := int32(-1), int32(-1)
	var visit func(lo, hi int)
	visit = func(lo, hi int) {
		if lo == hi {
			return
		}
		mid := lo + (hi-lo)/2
		sp := x.spans[mid]
		if sp.maxEnd <= offset {
			return
		}
		visit(lo, mid)
		if sp.start > offset {
			return
		}
		if offset < sp.end && (sp.depth > depth || (sp.depth == depth && sp.node < best)) {
			best, depth = sp.node, sp.depth
		}
		visit(mid+1, hi)
	}
	visit(0, len(x.spans))
	return Node{x.s, best}, best >= 0
}
