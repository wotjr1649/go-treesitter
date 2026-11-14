package treesitter

import "encoding/binary"

type fakeQueryState struct {
	meta                   []byte
	errorOffset, errorType int32
	tree                   int32
	nodes                  [][]uint32
	pos                    int
	buf                    int32
	halted, exceeded       int32
	corrupt                func([]byte)
	options                [3]int32
}

func (f *fakeModule) qs() *fakeQueryState {
	if f.query == nil {
		f.query = &fakeQueryState{}
	}
	return f.query
}
func (f *fakeModule) Xtsg_query_new(src, n int32) int32 {
	f.rec("query_new")
	if f.qs().errorType != 0 {
		return 0
	}
	return 100
}
func (f *fakeModule) Xtsg_query_error_offset() int32 {
	f.rec("query_error_offset")
	return f.qs().errorOffset
}
func (f *fakeModule) Xtsg_query_error_type() int32 {
	f.rec("query_error_type")
	return f.qs().errorType
}
func (f *fakeModule) Xtsg_query_delete(q int32) { f.rec("query_delete") }
func (f *fakeModule) Xtsg_query_meta(q int32) int32 {
	f.rec("query_meta")
	b := f.qs().meta
	if b == nil {
		b = fakeQueryMeta(nil, nil)
	}
	p := f.Xmalloc(int32(len(b)))
	copy(f.mem()[p:], b)
	return p
}
func (f *fakeModule) Xtsg_qcursor_new() int32     { f.rec("qcursor_new"); return 101 }
func (f *fakeModule) Xtsg_qcursor_delete(c int32) { f.rec("qcursor_delete") }
func (f *fakeModule) Xtsg_qcursor_exec(c, q, t, start, end, limit int32) int32 {
	f.rec("qcursor_exec")
	s := f.qs()
	s.tree = t
	s.pos = 0
	s.options = [3]int32{start, end, limit}
	return 0
}
func (f *fakeModule) Xtsg_qcursor_next(c int32) int32 {
	f.rec("qcursor_next")
	s := f.qs()
	nodes := s.nodes
	if nodes == nil {
		for i, n := range f.tree(s.tree).nodes {
			if n.sym == fWord && int32(n.eb) > s.options[0] && (s.options[1] == 0 || int32(n.sb) < s.options[1]) {
				nodes = append(nodes, []uint32{uint32(i)})
			}
		}
	}
	if s.pos >= len(nodes) {
		return 0
	}
	indices := nodes[s.pos]
	s.pos++
	b := make([]byte, 24+20*len(indices))
	copy(b, "TSGM")
	le := binary.LittleEndian
	le.PutUint32(b[4:], 1)
	le.PutUint32(b[8:], 1)
	le.PutUint32(b[12:], uint32(len(b)-16))
	le.PutUint32(b[20:], uint32(len(indices)))
	for j, i := range indices {
		n := f.tree(s.tree).nodes[i]
		for k, v := range []uint32{uint32(j), i, uint32(n.sym), n.sb, n.eb} {
			le.PutUint32(b[24+20*j+4*k:], v)
		}
	}
	if s.corrupt != nil {
		s.corrupt(b)
	}
	s.buf = f.Xmalloc(int32(len(b)))
	copy(f.mem()[s.buf:], b)
	return 1
}
func (f *fakeModule) Xtsg_qcursor_halted(c int32) int32 {
	f.rec("qcursor_halted")
	return f.qs().halted
}
func (f *fakeModule) Xtsg_qcursor_exceeded(c int32) int32 {
	f.rec("qcursor_exceeded")
	return f.qs().exceeded
}
func (f *fakeModule) Xtsg_qcursor_buf(c int32) int32 { f.rec("qcursor_buf"); return f.qs().buf }

// One pattern with two captures. Steps are {type,id}, as in TSQueryPredicateStep.
func fakeQueryMeta(literals []string, steps [][2]uint32) []byte {
	b := []byte("TSGQ")
	put := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
	put(1)
	put(0)
	put(1)
	put(2)
	put(uint32(len(literals)))
	for _, s := range append([]string{"word", "other"}, literals...) {
		put(uint32(len(s)))
		b = append(b, []byte(s)...)
	}
	put(0)
	put(uint32(len(steps)))
	for _, s := range steps {
		put(s[0])
		put(s[1])
	}
	binary.LittleEndian.PutUint32(b[8:], uint32(len(b)))
	return b
}
