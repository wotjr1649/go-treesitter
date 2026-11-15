package treesitter

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// installFieldLookups models the shim's per-chunk tables, including targets in later chunks.
func installFieldLookups(f *fakeModule, entries []fieldLookup) {
	f.nextHook = func(f *fakeModule) {
		le := binary.LittleEndian
		le.PutUint32(f.mem()[f.buf+12:], 0)
		var chunk []fieldLookup
		for _, e := range entries {
			if int(e.node) >= f.pos && int(e.node) < min(f.pos+f.kc, len(f.exp)) {
				chunk = append(chunk, e)
			}
		}
		if len(chunk) == 0 {
			return
		}
		ptr := f.Xmalloc(int32(4 + 12*len(chunk)))
		le.PutUint32(f.mem()[f.buf+12:], uint32(ptr))
		le.PutUint32(f.mem()[ptr:], uint32(len(chunk)))
		for i, e := range chunk {
			base := int(ptr) + 4 + 12*i
			le.PutUint32(f.mem()[base:], uint32(e.node))
			le.PutUint32(f.mem()[base+4:], e.field)
			le.PutUint32(f.mem()[base+8:], uint32(e.target))
		}
	}
}

func fieldIndices(n Node, name string) []int {
	var got []int
	for c := range n.ChildrenByFieldName(name) {
		got = append(got, c.Index())
	}
	return got
}

func TestFieldLookupExceptions(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
		f.corrupt = func(nodes []fnode) {
			nodes[2].field, nodes[3].field = 1, 1 // two direct children filling the same field
			nodes[6].field = 2
		}
		installFieldLookups(f, []fieldLookup{{0, 1, 4}, {0, 2, -1}, {1, 1, 2}})
	})
	p := newP(t, fl)
	s, err := p.ParseSnapshot(bg, []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	// Node 4 belongs to the nested group in the next chunk; it wins over a direct match.
	if got, ok := s.Root().ChildByFieldName("left"); !ok || got.Index() != 4 {
		t.Fatalf("inherited target: %v %v", got.Index(), ok)
	}
	if got := fieldIndices(s.Root(), "left"); !slices.Equal(got, []int{4}) {
		t.Fatalf("inherited children: %v", got)
	}
	if got := fieldIndices(s.Node(1), "left"); !slices.Equal(got, []int{2, 3}) {
		t.Fatalf("C first must not duplicate a direct child: %v", got)
	}
	if got, ok := s.Root().ChildByFieldName("right"); ok || !got.IsZero() || len(fieldIndices(s.Root(), "right")) != 0 {
		t.Fatal("explicit null must suppress direct matches")
	}
	if _, ok := s.Root().ChildByFieldName("unknown"); ok || len(fieldIndices(s.Root(), "unknown")) != 0 {
		t.Fatal("unknown field")
	}
	if _, ok := (Node{}).ChildByFieldName("left"); ok || len(fieldIndices(Node{}, "left")) != 0 {
		t.Fatal("zero node")
	}
	p.Close()
	clear(fl.cur().mem()) // neither table nor node storage may alias engine memory
	if got, ok := s.Root().ChildByFieldName("left"); !ok || got.Index() != 4 || got.Text() != "b" {
		t.Fatal("snapshot field result did not survive parser close")
	}
}

func TestFieldLookupDifferentDirectTarget(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
		installFieldLookups(f, []fieldLookup{{0, 1, 2}})
	})
	p := newP(t, fl)
	s, err := p.ParseSnapshot(bg, []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldIndices(s.Root(), "left"); !slices.Equal(got, []int{2, 6}) {
		t.Fatalf("C target then direct target: %v", got)
	}
	for range s.Root().ChildrenByFieldName("left") {
		break
	}
}

func TestErrorFieldLookup(t *testing.T) {
	for _, abi := range []uint32{1, 2, engine.ABIVersion} {
		t.Run(string(rune('0'+abi)), func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
				f.abi = int32(abi)
				f.corrupt = func(nodes []fnode) {
					nodes[1].sym, nodes[1].flags = uint16(ErrorSymbol), flagNamed|flagError|flagHasError
					nodes[2].field = 1
				}
			})
			fl.lang.g.ABI = abi
			p := newP(t, fl)
			s, err := p.ParseSnapshot(bg, []byte("(a)"))
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := s.Node(1).ChildByFieldName("left"); ok || !got.IsZero() || len(fieldIndices(s.Node(1), "left")) != 0 {
				t.Fatal("ERROR nodes must have no field result")
			}
			if s.Node(2).Field() != "left" {
				t.Fatal("ERROR child cursor metadata must remain available")
			}
		})
	}
}

func TestFieldLookupMalformedBlocks(t *testing.T) {
	cases := []struct {
		name    string
		entries []fieldLookup
		mutate  func(*fakeModule)
	}{
		{"pointer", nil, func(f *fakeModule) { binary.LittleEndian.PutUint32(f.mem()[f.buf+12:], uint32(len(f.mem())-2)) }},
		{"count", []fieldLookup{{0, 1, 2}}, func(f *fakeModule) {
			ptr := binary.LittleEndian.Uint32(f.mem()[f.buf+12:])
			binary.LittleEndian.PutUint32(f.mem()[ptr:], ^uint32(0))
		}},
		{"node outside chunk", []fieldLookup{{4, 1, 5}}, nil},
		{"unknown field", []fieldLookup{{0, 3, 2}}, nil},
		{"zero field", []fieldLookup{{0, 0, 2}}, nil},
		{"target outside snapshot", []fieldLookup{{0, 1, 7}}, nil},
		{"negative target", []fieldLookup{{0, 1, -2}}, nil},
		{"self target", []fieldLookup{{0, 1, 0}}, nil},
		{"target outside subtree", []fieldLookup{{1, 1, 6}}, nil},
		{"duplicate", []fieldLookup{{0, 1, 2}, {0, 1, 3}}, nil},
		{"unordered", []fieldLookup{{0, 2, 2}, {0, 1, 3}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
				// Force the malformed records into the first chunk, without the valid producer's filtering.
				f.nextHook = func(f *fakeModule) {
					le := binary.LittleEndian
					le.PutUint32(f.mem()[f.buf+12:], 0)
					if f.pos != 0 {
						return
					}
					if len(tc.entries) > 0 {
						ptr := f.Xmalloc(int32(4 + 12*len(tc.entries)))
						le.PutUint32(f.mem()[f.buf+12:], uint32(ptr))
						le.PutUint32(f.mem()[ptr:], uint32(len(tc.entries)))
						for i, e := range tc.entries {
							base := int(ptr) + 4 + 12*i
							le.PutUint32(f.mem()[base:], uint32(e.node))
							le.PutUint32(f.mem()[base+4:], e.field)
							le.PutUint32(f.mem()[base+8:], uint32(e.target))
						}
					}
					if tc.mutate != nil {
						tc.mutate(f)
					}
				}
			})
			p := newP(t, fl)
			_, err := p.ParseSnapshot(bg, []byte(sample))
			wantPE(t, err, EngineFault, true)
		})
	}
}

func TestFieldLookupLegacyABI(t *testing.T) {
	for _, abi := range []uint32{1, 2} {
		fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.abi = int32(abi) })
		fl.lang.g.ABI = abi
		p := newP(t, fl)
		s, err := p.ParseSnapshot(bg, []byte(sample))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := s.Root().ChildByFieldName("left"); !ok || got.Index() != 6 {
			t.Fatalf("ABI %d must retain direct lookup: %v %v", abi, got.Index(), ok)
		}
		mem, ptr := fl.cur().mem(), fl.cur().buf
		binary.LittleEndian.PutUint16(mem[ptr+4:], engine.ExportFormat)
		if _, err := parseExportHeader(mem, ptr, abi); err == nil || err.Outcome != IncompatibleLanguage {
			t.Fatalf("ABI %d accepted format 2: %v", abi, err)
		}
	}
}

func TestFieldLookupABI3RejectsFormat1(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.exportFormat = 1 })
	p, err := NewParser(fl.lang)
	if p != nil {
		p.Close()
		t.Fatal("ABI 3 module with format 1 created a parser")
	}
	wantPE(t, err, IncompatibleLanguage, false)
}
