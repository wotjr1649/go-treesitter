package treesitter

import (
	"encoding/binary"
	"fmt"
	"iter"
	"sync"
)

// Point is a zero-based row and byte column.
type Point struct{ Row, Column uint32 }

func (p Point) key() uint64 { return uint64(p.Row)<<32 | uint64(p.Column) }

// Range is a span of the source.
type Range struct {
	StartByte, EndByte   uint32
	StartPoint, EndPoint Point
}

// Node flag bits as exported by the engine (engine-abi.md section 2).
const (
	flagNamed    = 1
	flagExtra    = 2
	flagMissing  = 4
	flagError    = 8
	flagHasError = 16
)

// Snapshot is an immutable, goroutine-safe copy of one parse tree in struct-of-arrays form (21 bytes per node plus sparse field exceptions and a lazy line index).
// It owns its source text (Parse: one copy; ParseOwned: the caller's slice). Node i is the i-th node in preorder, which
// equals C's descendant index.
type Snapshot struct {
	lang *Language
	src  string
	n    int

	symbol, field      []uint16
	flags              []uint8
	startByte, endByte []uint32
	parent             []int32 // -1 for the root
	descEnd            []int32 // exclusive end of the subtree in preorder (i+1 for a leaf; a node has children iff descEnd > i+1)

	fieldLookups []fieldLookup // sorted by node, then field; target -1 is an explicit null

	invalidUTF8 bool

	// Lazily computed once, safe for concurrent readers (primitives of api-v2 section 7).
	linesOnce sync.Once
	lines     *LineIndex
	hashOnce  sync.Once
	hash      []uint64
	shapeOnce sync.Once
	shape     []uint64
	atomOnce  sync.Once
	atoms     atomTable
	indexOnce sync.Once
	index     *Index
}

// Root is the root node (its start may be after leading padding; its end is the input length).
func (s *Snapshot) Root() Node { return Node{s, 0} }

// Len is the number of nodes.
func (s *Snapshot) Len() int { return s.n }

// Node returns the i-th node in preorder; it panics when i is out of range.
func (s *Snapshot) Node(i int) Node {
	if i < 0 || i >= s.n {
		panic(fmt.Sprintf("treesitter: node index %d out of range [0,%d)", i, s.n))
	}
	return Node{s, int32(i)}
}

func (s *Snapshot) Language() *Language { return s.lang }

// Source is the owned source text. Node offsets index into it.
func (s *Snapshot) Source() string { return s.src }

// HasInvalidUTF8 reports whether the parsed source had invalid UTF-8 (parsed like C: decode-error bytes).
func (s *Snapshot) HasInvalidUTF8() bool { return s.invalidUTF8 }

// WalkAction steers Walk.
type WalkAction uint8

const (
	Continue     WalkAction = iota // descend into the children
	SkipChildren                   // do not visit this node's descendants
	Stop                           // end the walk
)

// Walk visits nodes in preorder with their depth (root = 0).
func (s *Snapshot) Walk(fn func(n Node, depth int) WalkAction) {
	ends := make([]int32, 0, 32) // subtree ends of the open ancestors
	for i := int32(0); int(i) < s.n; {
		for len(ends) > 0 && i >= ends[len(ends)-1] {
			ends = ends[:len(ends)-1]
		}
		switch fn(Node{s, i}, len(ends)) {
		case Stop:
			return
		case SkipChildren:
			i = s.descEnd[i]
			continue
		}
		if s.descEnd[i] > i+1 {
			ends = append(ends, s.descEnd[i])
		}
		i++
	}
}

// All iterates every node in preorder.
func (s *Snapshot) All() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for i := 0; i < s.n; i++ {
			if !yield(Node{s, int32(i)}) {
				return
			}
		}
	}
}

// DescendantForByteRange returns the smallest visible node of the snapshot spanning [start, end]; named=true considers
// only named nodes. Like C it returns the null node, here the zero Node (check Node.IsZero before use), when
// start > end. It is not C's ts_node_(named_)descendant_for_byte_range: C walks the full tree, including hidden
// nodes such as zero-width scanner marker tokens, and a snapshot holds visible nodes only. For a zero-width range at a
// node boundary C may therefore stop at an ancestor of this result, or step over a zero-width MISSING node that this
// function returns. No equality with C is promised there (see docs/specs/parser-result.md,
// "Navigation differs from C at zero-width nodes").
// Cost is O(depth x fan-out); use an Index for bulk lookups.
func (s *Snapshot) DescendantForByteRange(start, end uint32, named bool) Node {
	return s.descendantFor(uint64(start), uint64(end), named, false)
}

// DescendantForPointRange is the point-based variant of DescendantForByteRange, with the same difference from C's
// ts_node_(named_)descendant_for_point_range; it also returns the zero Node when start > end.
func (s *Snapshot) DescendantForPointRange(start, end Point, named bool) Node {
	return s.descendantFor(start.key(), end.key(), named, true)
}

func (s *Snapshot) startKey(i int32, points bool) uint64 {
	if points {
		return s.Lines().point(s.startByte[i]).key()
	}
	return uint64(s.startByte[i])
}

func (s *Snapshot) endKey(i int32, points bool) uint64 {
	if points {
		return s.Lines().point(s.endByte[i]).key()
	}
	return uint64(s.endByte[i])
}

// The loop of C's ts_node__descendant_for_byte_range/_point_range (node.c, v0.27.2) run over the visible nodes only,
// so "relevant" is "named" or "any"; hidden nodes C also walks are absent (see DescendantForByteRange).
func (s *Snapshot) descendantFor(start, end uint64, named, points bool) Node {
	if start > end {
		return Node{}
	}
	node, last := int32(0), int32(0)
	for descended := true; descended; {
		descended = false
		for c, ok := s.firstChild(node); ok; c, ok = s.nextSibling(c) {
			cStart, cEnd := s.startKey(c, points), s.endKey(c, points)
			if cEnd < end {
				continue
			}
			// The child must exceed the start of the range, unless it is empty (a zero-width MISSING node): then
			// reaching the start is enough.
			if (cStart == cEnd && cEnd < start) || (cStart != cEnd && cEnd <= start) {
				continue
			}
			if start < cStart {
				break
			}
			node = c
			if !named || s.flags[c]&flagNamed != 0 {
				last = c
			}
			descended = true
			break
		}
	}
	return Node{s, last}
}

func (s *Snapshot) firstChild(i int32) (int32, bool) {
	if s.descEnd[i] == i+1 {
		return 0, false
	}
	return i + 1, true
}

func (s *Snapshot) nextSibling(i int32) (int32, bool) {
	p := s.parent[i]
	e := s.descEnd[i]
	if p < 0 || e >= s.descEnd[p] {
		return 0, false
	}
	return e, true
}

type fieldLookup struct {
	node   int32
	field  uint32
	target int32
}

// snapshotBuilder fills a Snapshot from the engine's chunks and validates everything later reads rely on.
type snapshotBuilder struct {
	s      *Snapshot
	filled int
}

func newSnapshotBuilder(n int, lang *Language, src string, invalidUTF8 bool) *snapshotBuilder {
	return &snapshotBuilder{s: &Snapshot{
		lang: lang, src: src, n: n, invalidUTF8: invalidUTF8,
		symbol: make([]uint16, n), field: make([]uint16, n), flags: make([]uint8, n),
		startByte: make([]uint32, n), endByte: make([]uint32, n),
		parent: make([]int32, n), descEnd: make([]int32, n),
	}}
}

// add copies cnt nodes out of a chunk laid out as in engine-abi.md section 2 (kc = chunk capacity) and checks each
// node: known symbol and field, and a source range that Text can slice.
func (b *snapshotBuilder) add(cnt, kc int, chunk []byte) error {
	s, o := b.s, b.filled
	if cnt <= 0 || o+cnt > s.n || len(chunk) < 33*kc {
		return fmt.Errorf("bad chunk (count %d, filled %d of %d)", cnt, o, s.n)
	}
	le := binary.LittleEndian
	u32 := func(col int, dst []uint32) {
		base := col * kc
		for i := 0; i < cnt; i++ {
			dst[o+i] = le.Uint32(chunk[base+4*i:])
		}
	}
	nSym, nField, srcLen := len(s.lang.g.Meta.Symbols), len(s.lang.g.Meta.Fields), uint32(len(s.src))
	for i := 0; i < cnt; i++ {
		sym, fld := le.Uint16(chunk[2*i:]), le.Uint16(chunk[2*kc+2*i:])
		if (int(sym) >= nSym && sym != uint16(ErrorSymbol)) || int(fld) >= nField {
			return fmt.Errorf("node %d: symbol %d or field %d not in the language", o+i, sym, fld)
		}
		s.symbol[o+i], s.field[o+i], s.flags[o+i] = sym, fld, chunk[4*kc+i]
	}
	u32(5, s.startByte)
	u32(9, s.endByte)
	// Formats 1/2 still export point columns at 13/17/21/25; derive them from the owned source instead.
	for i := 0; i < cnt; i++ {
		s.parent[o+i] = int32(le.Uint32(chunk[29*kc+4*i:]))
		if s.startByte[o+i] > s.endByte[o+i] || s.endByte[o+i] > srcLen {
			return fmt.Errorf("node %d: byte range [%d,%d) outside the %d-byte source", o+i, s.startByte[o+i], s.endByte[o+i], srcLen)
		}
	}
	b.filled += cnt
	return nil
}

// finish checks that the parent column is a valid preorder and derives descEnd.
func (b *snapshotBuilder) finish() (*Snapshot, error) {
	s := b.s
	if b.filled != s.n {
		return nil, fmt.Errorf("incomplete snapshot (%d of %d nodes)", b.filled, s.n)
	}
	if s.parent[0] != -1 {
		return nil, fmt.Errorf("node 0 is not the root")
	}
	stack := make([]int32, 1, 64) // open ancestors of the node being added
	for i := int32(1); int(i) < s.n; i++ {
		p := s.parent[i]
		if p < 0 || p >= i {
			return nil, fmt.Errorf("node %d: parent %d is not a preceding node", i, p)
		}
		for stack[len(stack)-1] != p {
			s.descEnd[stack[len(stack)-1]] = i
			if stack = stack[:len(stack)-1]; len(stack) == 0 {
				return nil, fmt.Errorf("node %d: parent %d is not on the root path", i, p)
			}
		}
		stack = append(stack, i)
	}
	for _, j := range stack {
		s.descEnd[j] = int32(s.n)
	}
	for _, e := range s.fieldLookups {
		if e.target != -1 && (e.target <= e.node || e.target >= s.descEnd[e.node]) {
			return nil, fmt.Errorf("field lookup target %d is not a descendant of %d", e.target, e.node)
		}
	}
	return s, nil
}

// addFieldLookups copies and validates the format-2 sparse table for the just-copied node chunk.
func (b *snapshotBuilder) addFieldLookups(mem []byte, ptr uint32, start int) error {
	if ptr == 0 {
		return nil
	}
	if uint64(ptr)+4 > uint64(len(mem)) {
		return fmt.Errorf("block pointer %d is outside memory", ptr)
	}
	le := binary.LittleEndian
	count := uint64(le.Uint32(mem[ptr:]))
	end := uint64(ptr) + 4 + 12*count
	s := b.s
	if end > uint64(len(mem)) || count > uint64(b.filled-start)*uint64(len(s.lang.g.Meta.Fields)-1) {
		return fmt.Errorf("invalid exception count %d", count)
	}
	data := mem[uint64(ptr)+4 : end]
	for len(data) > 0 {
		e := fieldLookup{int32(le.Uint32(data)), le.Uint32(data[4:]), int32(le.Uint32(data[8:]))}
		if int(e.node) < start || int(e.node) >= b.filled || e.field == 0 || uint64(e.field) >= uint64(len(s.lang.g.Meta.Fields)) || e.target < -1 || int(e.target) >= s.n {
			return fmt.Errorf("invalid exception %+v", e)
		}
		if n := len(s.fieldLookups); n > 0 {
			prev := s.fieldLookups[n-1]
			if e.node < prev.node || (e.node == prev.node && e.field <= prev.field) {
				return fmt.Errorf("exceptions are not strictly ordered")
			}
		}
		s.fieldLookups = append(s.fieldLookups, e)
		data = data[12:]
	}
	return nil
}
