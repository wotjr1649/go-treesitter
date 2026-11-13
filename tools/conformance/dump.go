// Package conformance holds the real-grammar conformance lane of go-treesitter: it compares the public treesitter API,
// running the generated grammar modules, with recorded native C tree-sitter results. The data comes from the local
// directory named by TSG_DATA_DIR (tools/native-ref/ciconf output) or TSG_S15_DIR (local session-15 tree); without one every test skips.
package conformance

import (
	"bufio"
	"io"
	"strconv"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// WriteDump writes the canonical "#tsg-dump/1" text of s: byte-identical to the C dumper .scratch/s15/shim/tsg_dump.c
// (a header line, then one line per node in preorder: depth, field or "-", type, named, extra, missing, is_error,
// has_error, start_byte, end_byte, start_row:start_col, end_row:end_col, tab separated). It uses only the public API.
func WriteDump(w io.Writer, s *treesitter.Snapshot) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	buf := make([]byte, 0, 256)
	root := "0"
	if s.Root().HasError() {
		root = "1"
	}
	buf = append(buf, "#tsg-dump/1 root_has_error="...)
	buf = append(buf, root...)
	buf = append(buf, " nodes="...)
	buf = strconv.AppendUint(buf, uint64(s.Len()), 10)
	buf = append(buf, '\n')
	bw.Write(buf)
	flag := func(b []byte, v bool) []byte {
		if v {
			return append(b, '1', '\t')
		}
		return append(b, '0', '\t')
	}
	s.Walk(func(n treesitter.Node, depth int) treesitter.WalkAction {
		b := strconv.AppendUint(buf[:0], uint64(depth), 10)
		b = append(b, '\t')
		if f := n.Field(); f != "" {
			b = append(b, f...)
		} else {
			b = append(b, '-')
		}
		b = append(b, '\t')
		b = append(b, n.Kind()...)
		b = append(b, '\t')
		b = flag(b, n.IsNamed())
		b = flag(b, n.IsExtra())
		b = flag(b, n.IsMissing())
		b = flag(b, n.IsError())
		b = flag(b, n.HasError())
		b = strconv.AppendUint(b, uint64(n.StartByte()), 10)
		b = append(b, '\t')
		b = strconv.AppendUint(b, uint64(n.EndByte()), 10)
		b = append(b, '\t')
		sp, ep := n.StartPoint(), n.EndPoint()
		b = strconv.AppendUint(b, uint64(sp.Row), 10)
		b = append(b, ':')
		b = strconv.AppendUint(b, uint64(sp.Column), 10)
		b = append(b, '\t')
		b = strconv.AppendUint(b, uint64(ep.Row), 10)
		b = append(b, ':')
		b = strconv.AppendUint(b, uint64(ep.Column), 10)
		b = append(b, '\n')
		bw.Write(b)
		return treesitter.Continue
	})
	return bw.Flush()
}

// Dump returns the canonical dump as bytes.
func Dump(s *treesitter.Snapshot) []byte {
	var b sliceWriter
	b.b = make([]byte, 0, 64*s.Len()+64)
	WriteDump(&b, s)
	return b.b
}

type sliceWriter struct{ b []byte }

func (w *sliceWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
