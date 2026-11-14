package treesitter

import (
	"runtime"
	"testing"
	"time"
)

// gcAndWait lets cleanups registered with runtime.AddCleanup run (they execute on their own goroutine after a GC).
func gcAndWait() {
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
	}
}

// Finding 1: after "c := p.c" nothing used p, so p could become unreachable mid-call and its cleanup released the
// instance while the call was still using it. Every scenario below drops its only reference to the Parser (or Tree)
// before the engine call that forces a GC; the instance must not be released while the call runs.
func TestParserAndTreeKeptAliveDuringCalls(t *testing.T) {
	setup := func(trigger string, saw *bool, mod int) func(fl *fakeLang, f *fakeModule) {
		return func(fl *fakeLang, f *fakeModule) {
			if len(fl.mods) != mod {
				return
			}
			f.onCall = func(name string) {
				if name == trigger {
					gcAndWait()
					*saw = *saw || f.released.Load()
				}
			}
		}
	}
	src := []byte("a b c")
	for _, tc := range []struct {
		name, trigger string
		mod           int
		run           func(fl *fakeLang)
	}{
		{"Parse", "parse", 1, func(fl *fakeLang) { p, _ := NewParser(fl.lang); p.Parse(bg, src, nil) }},
		{"ParseOwned", "parse", 1, func(fl *fakeLang) { p, _ := NewParser(fl.lang); p.ParseOwned(bg, src, nil) }},
		{"ParseSnapshot export", "export_next", 1, func(fl *fakeLang) { p, _ := NewParser(fl.lang); p.ParseSnapshot(bg, src) }},
		{"Reset recreate", "init", 2, func(fl *fakeLang) { p, _ := NewParser(fl.lang); p.Reset() }},
		{"Tree.Snapshot", "export_next", 1, func(fl *fakeLang) {
			p, _ := NewParser(fl.lang)
			tr, _ := p.Parse(bg, src, nil)
			p = nil
			tr.Snapshot(bg)
		}},
		{"Tree.Edit", "edit", 1, func(fl *fakeLang) {
			p, _ := NewParser(fl.lang)
			tr, _ := p.Parse(bg, src, nil)
			p = nil
			tr.Edit(InputEdit{})
		}},
		{"Tree.ChangedRanges", "changed_ranges", 1, func(fl *fakeLang) {
			p, _ := NewParser(fl.lang)
			old, _ := p.Parse(bg, src, nil)
			nt, _ := p.Parse(bg, src, old)
			p = nil
			nt.ChangedRanges(old)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var saw bool
			fl := newFakeLang(setup(tc.trigger, &saw, tc.mod))
			tc.run(fl)
			if saw {
				t.Fatal("the instance was released by the Parser's cleanup while the call was running")
			}
		})
	}
}

// Finding 2: C treats an empty child (start == end, e.g. a MISSING node) as matching when it ends exactly at the start
// of the range; a non-empty child must end after it.
func TestDescendantForEmptyChildAtRangeStart(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
		f.corrupt = func(n []fnode) { // turn "b" ([2,3)) into a zero-width MISSING word at byte 1
			n[2].flags, n[2].sb, n[2].eb, n[2].sr, n[2].sc, n[2].er, n[2].ec = flagNamed|flagMissing, 1, 1, 0, 1, 0, 1
		}
	})
	s, err := newP(t, fl).ParseSnapshot(bg, []byte("a b"))
	if err != nil {
		t.Fatal(err)
	}
	for _, named := range []bool{false, true} {
		if n := s.DescendantForByteRange(1, 1, named); n.Index() != 2 || !n.IsMissing() {
			t.Errorf("named=%v: got node %d, want the empty MISSING child 2", named, n.Index())
		}
		if n := s.DescendantForPointRange(Point{0, 1}, Point{0, 1}, named); n.Index() != 2 {
			t.Errorf("point range, named=%v: got node %d, want 2", named, n.Index())
		}
	}
	if n := s.DescendantForByteRange(0, 0, true); n.Index() != 1 { // an ordinary child still wins at its own start
		t.Errorf("got %d, want 1", n.Index())
	}
	if n := s.DescendantForByteRange(1, 2, true); n.Index() != 0 { // the empty child cannot span [1,2]
		t.Errorf("got %d, want the root", n.Index())
	}
}

// Finding 3: start > end is C's null node: the zero Node, reported by IsZero.
func TestDescendantForReversedRangeIsZeroNode(t *testing.T) {
	s := sampleSnapshot(t)
	if n := s.DescendantForByteRange(5, 4, false); !n.IsZero() {
		t.Fatal("byte range start > end must return the zero Node")
	}
	if n := s.DescendantForPointRange(Point{1, 0}, Point{0, 5}, true); !n.IsZero() {
		t.Fatal("point range start > end must return the zero Node")
	}
	if n := s.DescendantForByteRange(4, 4, false); n.IsZero() || s.Root().IsZero() {
		t.Fatal("start == end is a valid range and real nodes are not zero")
	}
}

// Finding 4: the shim's "more nodes than counted" check only runs when export_next is called again; with n a multiple
// of the chunk capacity the loop used to stop without that call.
func TestExportOverrunWhenCountIsChunkMultiple(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	if _, err := p.ParseSnapshot(bg, []byte("a b c")); err != nil { // root + 3 words = 4 nodes = one full chunk (kc 4)
		t.Fatal(err)
	}
	if got := fl.cur().count("export_next"); got != 2 {
		t.Fatalf("export_next must be called until it returns 0: %d calls", got)
	}
	fl = newFakeLang(func(fl *fakeLang, f *fakeModule) { f.overrun = len(fl.mods) == 1 })
	tr := mustParse(t, newP(t, fl), "a b c")
	_, err := tr.Snapshot(bg)
	wantPE(t, err, EngineFault, true)
	if len(fl.mods) != 2 {
		t.Fatal("instance must be recreated")
	}
	fl = newFakeLang(func(fl *fakeLang, f *fakeModule) { f.exportErr = 1 })
	_, err = newP(t, fl).ParseSnapshot(bg, []byte("a b c"))
	wantPE(t, err, EngineFault, true)
}

// Finding 5: a failure that coincides with a refused memory.grow is a ResourceLimit, not an EngineFault.
func TestExportFailureWithRefusedGrowIsResourceLimit(t *testing.T) {
	for name, setup := range map[string]func(*fakeModule){
		"export_begin returns 0":  func(f *fakeModule) { f.refusedBegin = true },
		"export_err with refused": func(f *fakeModule) { f.exportErr, f.exportErrRefused = 1, true },
	} {
		t.Run(name, func(t *testing.T) {
			fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
				if len(fl.mods) == 1 {
					setup(f)
				}
			})
			p := newP(t, fl)
			tr := mustParse(t, p, "a b")
			_, err := tr.Snapshot(bg)
			wantPE(t, err, ResourceLimit, true)
			if len(fl.mods) != 2 || !fl.mods[0].released.Load() {
				t.Fatal("instance must be reset")
			}
			if _, err := p.ParseSnapshot(bg, []byte("a b")); err != nil {
				t.Fatalf("recreated instance: %v", err)
			}
		})
	}
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) { f.refusedBegin = len(fl.mods) == 1 })
	_, err := newP(t, fl).ParseSnapshot(bg, []byte("a"))
	wantPE(t, err, ResourceLimit, true)
}
