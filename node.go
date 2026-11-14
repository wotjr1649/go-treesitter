package treesitter

import (
	"iter"
	"strconv"
	"strings"
)

// Node is a small value handle (snapshot pointer + preorder index). The zero Node is the "null node": every method
// on it returns its zero value (false, 0, "", the zero Node, an empty iterator, or (Node{}, false) for the (Node, bool)
// methods) instead of panicking.
// Nodes are immutable and safe for concurrent use.
type Node struct {
	s *Snapshot
	i int32
}

// IsZero reports whether n is the zero Node, the "null node" returned for example by DescendantForByteRange when
// start > end. Use it to tell a null node from a real one; the other methods return zero values on it.
func (n Node) IsZero() bool { return n.s == nil }

// Snapshot is the snapshot the node belongs to (nil for the zero Node).
func (n Node) Snapshot() *Snapshot { return n.s }

// Index is the node's preorder index in its snapshot (0 for the zero Node).
func (n Node) Index() int { return int(n.i) }

// flag reports whether the node has the flag bit; false on the zero Node.
func (n Node) flag(f uint8) bool { return n.s != nil && n.s.flags[n.i]&f != 0 }

// Kind is the node type name ("ERROR" for error nodes).
func (n Node) Kind() string {
	if n.s == nil {
		return ""
	}
	return n.s.lang.SymbolName(Symbol(n.s.symbol[n.i]))
}

func (n Node) Symbol() Symbol {
	if n.s == nil {
		return 0
	}
	return Symbol(n.s.symbol[n.i])
}

// Field is the name of the field the node fills in its parent ("" for none).
func (n Node) Field() string {
	if n.s == nil {
		return ""
	}
	return n.s.lang.FieldName(FieldID(n.s.field[n.i]))
}

func (n Node) FieldID() FieldID {
	if n.s == nil {
		return 0
	}
	return FieldID(n.s.field[n.i])
}

func (n Node) IsNamed() bool   { return n.flag(flagNamed) }
func (n Node) IsExtra() bool   { return n.flag(flagExtra) }
func (n Node) IsMissing() bool { return n.flag(flagMissing) }
func (n Node) IsError() bool   { return n.flag(flagError) }

// HasError reports an ERROR or MISSING node in the subtree (including the node itself).
func (n Node) HasError() bool { return n.flag(flagHasError) }

func (n Node) StartByte() uint32 {
	if n.s == nil {
		return 0
	}
	return n.s.startByte[n.i]
}

func (n Node) EndByte() uint32 {
	if n.s == nil {
		return 0
	}
	return n.s.endByte[n.i]
}

func (n Node) StartPoint() Point {
	if n.s == nil {
		return Point{}
	}
	return Point{n.s.startRow[n.i], n.s.startCol[n.i]}
}

func (n Node) EndPoint() Point {
	if n.s == nil {
		return Point{}
	}
	return Point{n.s.endRow[n.i], n.s.endCol[n.i]}
}

// Text is the node's source text: a substring of the snapshot's owned source, no allocation.
func (n Node) Text() string {
	if n.s == nil {
		return ""
	}
	return n.s.src[n.s.startByte[n.i]:n.s.endByte[n.i]]
}

// AppendText appends the node's source text to dst.
func (n Node) AppendText(dst []byte) []byte { return append(dst, n.Text()...) }

func (n Node) at(i int32) Node { return Node{n.s, i} }

func (n Node) Parent() (Node, bool) {
	if n.s == nil {
		return Node{}, false
	}
	p := n.s.parent[n.i]
	if p < 0 {
		return Node{}, false
	}
	return n.at(p), true
}

func (n Node) ChildCount() int {
	if n.s == nil {
		return 0
	}
	return int(n.s.childCount[n.i])
}

// Child returns the i-th child; it walks the siblings, so it costs O(i).
func (n Node) Child(i int) (Node, bool) {
	if i < 0 || i >= n.ChildCount() {
		return Node{}, false
	}
	c, _ := n.s.firstChild(n.i)
	for ; i > 0; i-- {
		c, _ = n.s.nextSibling(c)
	}
	return n.at(c), true
}

// NamedChildCount costs O(children).
func (n Node) NamedChildCount() int {
	k := 0
	for c := range n.NamedChildren() {
		_ = c
		k++
	}
	return k
}

// NamedChild returns the i-th named child, costing O(children).
func (n Node) NamedChild(i int) (Node, bool) {
	if i < 0 {
		return Node{}, false
	}
	for c := range n.NamedChildren() {
		if i == 0 {
			return c, true
		}
		i--
	}
	return Node{}, false
}

func (n Node) NextSibling() (Node, bool) {
	if n.s == nil {
		return Node{}, false
	}
	c, ok := n.s.nextSibling(n.i)
	if !ok {
		return Node{}, false
	}
	return n.at(c), true
}

func (n Node) PrevSibling() (Node, bool) {
	if n.s == nil {
		return Node{}, false
	}
	p := n.s.parent[n.i]
	if p < 0 {
		return Node{}, false
	}
	prev, found := int32(0), false
	for c, ok := n.s.firstChild(p); ok && c != n.i; c, ok = n.s.nextSibling(c) {
		prev, found = c, true
	}
	if !found {
		return Node{}, false
	}
	return n.at(prev), true
}

func (n Node) NextNamedSibling() (Node, bool) {
	if n.s == nil {
		return Node{}, false
	}
	for c, ok := n.s.nextSibling(n.i); ok; c, ok = n.s.nextSibling(c) {
		if n.s.flags[c]&flagNamed != 0 {
			return n.at(c), true
		}
	}
	return Node{}, false
}

func (n Node) PrevNamedSibling() (Node, bool) {
	if n.s == nil {
		return Node{}, false
	}
	p := n.s.parent[n.i]
	if p < 0 {
		return Node{}, false
	}
	prev, found := int32(0), false
	for c, ok := n.s.firstChild(p); ok && c != n.i; c, ok = n.s.nextSibling(c) {
		if n.s.flags[c]&flagNamed != 0 {
			prev, found = c, true
		}
	}
	if !found {
		return Node{}, false
	}
	return n.at(prev), true
}

// Children iterates the children in order.
func (n Node) Children() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if n.s == nil {
			return
		}
		for c, ok := n.s.firstChild(n.i); ok; c, ok = n.s.nextSibling(c) {
			if !yield(n.at(c)) {
				return
			}
		}
	}
}

// NamedChildren iterates the named children in order.
func (n Node) NamedChildren() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for c := range n.Children() {
			if c.IsNamed() && !yield(c) {
				return
			}
		}
	}
}

// ChildByFieldName returns the first child filling the named field.
func (n Node) ChildByFieldName(name string) (Node, bool) {
	for c := range n.ChildrenByFieldName(name) {
		return c, true
	}
	return Node{}, false
}

// ChildrenByFieldName iterates the children filling the named field (none for an unknown name).
func (n Node) ChildrenByFieldName(name string) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if n.s == nil {
			return
		}
		id, ok := n.s.lang.FieldForName(name)
		if !ok {
			return
		}
		for c := range n.Children() {
			if c.FieldID() == id && !yield(c) {
				return
			}
		}
	}
}

// String is an S-expression of the named nodes (like C's ts_node_string, for tests and debugging). Anonymous nodes
// are omitted unless MISSING; ERROR nodes print as (ERROR). The zero Node prints as "".
func (n Node) String() string {
	if n.s == nil {
		return ""
	}
	var b strings.Builder
	n.writeSexp(&b, true)
	return b.String()
}

func (n Node) writeSexp(b *strings.Builder, root bool) {
	if !n.IsNamed() && !n.IsMissing() {
		for c := range n.Children() { // like C, children of an invisible-in-output node are printed in place
			c.writeSexp(b, false)
		}
		return
	}
	if !root {
		b.WriteByte(' ')
		if f := n.Field(); f != "" {
			b.WriteString(f)
			b.WriteString(": ")
		}
	}
	if n.IsMissing() {
		b.WriteString("(MISSING ")
		if n.IsNamed() {
			b.WriteString(n.Kind())
		} else {
			b.WriteString(strconv.Quote(n.Kind()))
		}
	} else {
		b.WriteString("(" + n.Kind())
	}
	for c := range n.Children() {
		c.writeSexp(b, false)
	}
	b.WriteByte(')')
}
