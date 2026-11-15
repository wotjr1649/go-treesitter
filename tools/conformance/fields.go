package conformance

import (
	"bufio"
	"io"
	"strconv"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// WriteFields writes the canonical "#tsg-fields/1" text of s: the Go-side twin of `native-ref <exe> fields`. For every
// node in preorder and every field name of the language (id order, ids 1..FieldCount-1) it calls the public
// Node.ChildByFieldName and prints "<node index>\t<field>\t<result index>" for each non-null result, after the header
// "#tsg-fields/1 nodes=<n> fields=<f>". The C side resolves results by node id; here Node.Index is the preorder index.
func WriteFields(w io.Writer, s *treesitter.Snapshot) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	l := s.Language()
	nf := l.FieldCount() - 1 // C counts ids 1..n; Go's FieldCount includes the empty id 0
	bw.WriteString("#tsg-fields/1 nodes=" + strconv.Itoa(s.Len()) + " fields=" + strconv.Itoa(nf) + "\n")
	for n := range s.All() {
		for id := 1; id <= nf; id++ {
			name := l.FieldName(treesitter.FieldID(id))
			if r, ok := n.ChildByFieldName(name); ok {
				bw.WriteString(strconv.Itoa(n.Index()) + "\t" + name + "\t" + strconv.Itoa(r.Index()) + "\n")
			}
		}
	}
	return bw.Flush()
}

// Fields returns the canonical field-lookup dump as bytes.
func Fields(s *treesitter.Snapshot) []byte {
	var b sliceWriter
	WriteFields(&b, s)
	return b.b
}
