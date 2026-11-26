package conformance

import (
	"bufio"
	"io"
	"strconv"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// WriteNav writes the canonical "#tsg-nav/1" text of s: the Go-side twin of `native-ref <exe> nav`. For every node in
// preorder it prints the preorder index of Parent, NextSibling, PrevSibling, NextNamedSibling, PrevNamedSibling ("-" =
// none) and of DescendantForByteRange (named false/true) for the node's span, the zero-width range at its start and at
// its end. The line format is documented at nav_tree in tools/native-ref/c/native_ref.c.
func WriteNav(w io.Writer, s *treesitter.Snapshot) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	idx := func(n treesitter.Node, ok bool) string {
		if !ok || n.IsZero() {
			return "-"
		}
		return strconv.Itoa(n.Index())
	}
	bw.WriteString("#tsg-nav/1 nodes=" + strconv.Itoa(s.Len()) + "\n")
	for n := range s.All() {
		i := strconv.Itoa(n.Index())
		p, pok := n.Parent()
		nx, nxok := n.NextSibling()
		pv, pvok := n.PrevSibling()
		nn, nnok := n.NextNamedSibling()
		pn, pnok := n.PrevNamedSibling()
		bw.WriteString("n\t" + i + "\t" + idx(p, pok) + "\t" + idx(nx, nxok) + "\t" + idx(pv, pvok) + "\t" + idx(nn, nnok) + "\t" + idx(pn, pnok) + "\n")
		bw.WriteString("r\t" + i)
		st, en := n.StartByte(), n.EndByte()
		for _, r := range [3][2]uint32{{st, en}, {st, st}, {en, en}} {
			for _, named := range [2]bool{false, true} {
				bw.WriteString("\t" + idx(s.DescendantForByteRange(r[0], r[1], named), true))
			}
		}
		bw.WriteString("\n")
	}
	return bw.Flush()
}

// Nav returns the canonical navigation dump as bytes.
func Nav(s *treesitter.Snapshot) []byte {
	var b sliceWriter
	WriteNav(&b, s)
	return b.b
}
