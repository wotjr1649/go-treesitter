package syntax

import (
	"errors"
	"iter"
	"slices"
)

// Index supports repeated position/type lookups in one preorder snapshot.
// It stores offsets and node indexes, not the source or backend tree. After
// construction it is immutable and may be shared by readers. An edit needs a
// new index. Building costs O(n log n) time and O(n) additional memory.
type Index struct {
	byType map[string][]int
	spans  []indexedSpan
}

type indexedSpan struct {
	start, end, maxEnd uint32
	node, depth        int
}

// NewIndex validates ranges and parent links, then indexes a snapshot.
// Empty snapshots are allowed. Subsequent changes to nodes do not change the
// index; returned indexes always identify entries in the original snapshot.
func NewIndex(nodes []Node) (*Index, error) {
	x := &Index{byType: make(map[string][]int), spans: make([]indexedSpan, len(nodes))}
	for i, n := range nodes {
		if n.Type == "" || n.StartByte > n.EndByte ||
			(i == 0 && n.Parent != -1) || (i > 0 && (n.Parent < 0 || n.Parent >= i)) {
			return nil, errors.New("invalid snapshot node")
		}
		depth := 0
		if i > 0 {
			p := nodes[n.Parent]
			if n.StartByte < p.StartByte || n.EndByte > p.EndByte {
				return nil, errors.New("node range escapes parent")
			}
			depth = x.spans[n.Parent].depth + 1
		}
		x.spans[i] = indexedSpan{start: n.StartByte, end: n.EndByte, node: i, depth: depth}
		x.byType[n.Type] = append(x.byType[n.Type], i)
	}
	slices.SortFunc(x.spans, func(a, b indexedSpan) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return a.node - b.node
	})
	var augment func(int, int) uint32
	augment = func(lo, hi int) uint32 {
		if lo == hi {
			return 0
		}
		mid := lo + (hi-lo)/2
		s := &x.spans[mid]
		s.maxEnd = max(s.end, augment(lo, mid), augment(mid+1, hi))
		return s.maxEnd
	}
	augment(0, len(x.spans))
	return x, nil
}

// OfType yields matching node indexes in original preorder, without allocating
// a result slice. An unknown type yields nothing. Each call takes O(matches).
func (x *Index) OfType(nodeType string) iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, i := range x.byType[nodeType] {
			if !yield(i) {
				return
			}
		}
	}
}

// NodeAt returns the deepest node whose half-open byte range contains offset.
// Equal-depth overlaps choose the earlier preorder entry. Zero-width missing
// nodes and the EOF position are excluded; query missing nodes with OfType.
// Disjoint ranges are pruned. Worst-case cost is O(n) for overlapping ranges,
// with O(log n) traversal stack space.
func (x *Index) NodeAt(offset uint32) (int, bool) {
	best, depth := -1, -1
	var visit func(int, int)
	visit = func(lo, hi int) {
		if lo == hi {
			return
		}
		mid := lo + (hi-lo)/2
		s := x.spans[mid]
		if s.maxEnd <= offset {
			return
		}
		visit(lo, mid)
		if s.start > offset {
			return
		}
		if offset < s.end && (s.depth > depth || (s.depth == depth && s.node < best)) {
			best, depth = s.node, s.depth
		}
		visit(mid+1, hi)
	}
	visit(0, len(x.spans))
	return best, best >= 0
}
