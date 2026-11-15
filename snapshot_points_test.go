package treesitter

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestSnapshotPointsFromBytes(t *testing.T) {
	for _, abi := range []uint32{1, 2, 3} {
		for _, src := range []string{"", "\r\n", "a\nb\n", "a\rb", "한글😀\r\nx", "a\xff\xc3\n"} {
			t.Run(fmt.Sprintf("abi%d/%q", abi, src), func(t *testing.T) {
				fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.abi = int32(abi) })
				fl.lang.g.ABI = abi
				p := newP(t, fl)
				s := snapOf(t, p, src)
				if s.lines != nil {
					t.Fatal("export eagerly built the line index")
				}
				p.Close()
				var wg sync.WaitGroup
				for range 8 {
					wg.Go(func() {
						for n := range s.All() {
							if got, want := n.StartPoint(), pointAt([]byte(src), n.StartByte()); got != want {
								t.Errorf("start node %d: %v want %v", n.Index(), got, want)
							}
							if got, want := n.EndPoint(), pointAt([]byte(src), n.EndByte()); got != want {
								t.Errorf("end node %d: %v want %v", n.Index(), got, want)
							}
						}
					})
				}
				wg.Wait()
				if len(s.lines.starts) != strings.Count(src, "\n")+1 || cap(s.lines.starts) != len(s.lines.starts) {
					t.Fatal("line starts must retain exactly four bytes per line")
				}
				if s.lines.nonASCII != nil {
					t.Fatal("byte points built the UTF-16/rune acceleration cache")
				}
				for range 8 {
					wg.Go(func() {
						for off := 0; off <= len(src); off++ {
							if got, want := s.Lines().Pos(uint32(off)), naivePos(src, off); got != want {
								t.Errorf("Pos(%d): %v want %v", off, got, want)
							}
						}
					})
				}
				wg.Wait()
			})
		}
	}
}

func TestChangedRangePointsFromBytes(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	old := mustParse(t, p, "a\r\n한글\n")
	result := mustParse(t, p, "a\r\n한글\n")
	fl.cur().ranges = []Range{{3, 10, Point{9, 9}, Point{99, 99}}}
	for _, snapshot := range []bool{false, true} {
		if snapshot {
			if _, err := result.Snapshot(bg); err != nil {
				t.Fatal(err)
			}
		}
		ranges, err := result.ChangedRanges(old)
		if err != nil || len(ranges) != 1 || ranges[0].StartPoint != (Point{1, 0}) || ranges[0].EndPoint != (Point{2, 0}) {
			t.Fatalf("snapshot=%v: ranges=%v error=%v", snapshot, ranges, err)
		}
	}
}

func TestEditAllowsMismatchedPoints(t *testing.T) {
	fl := newFakeLang(nil)
	tree := mustParse(t, newP(t, fl), "a\nb")
	e := InputEdit{StartByte: 0, OldEndByte: 0, NewEndByte: 0, StartPoint: Point{8, 1}, OldEndPoint: Point{8, 1}, NewEndPoint: Point{9, 2}}
	if err := tree.Edit(e); err != nil {
		t.Fatal(err)
	}
	if got := fl.cur().edits[0]; got != [10]int32{1, 0, 0, 0, 8, 1, 8, 1, 9, 2} {
		t.Fatalf("caller points changed: %v", got)
	}
}

func TestSnapshotMissingPointAtEOF(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
		f.corrupt = func(nodes []fnode) {
			n := &nodes[1]
			n.sb, n.eb, n.flags = 2, 2, flagNamed|flagMissing
			// Stale engine points must not escape the snapshot.
			n.sr, n.sc, n.er, n.ec = 99, 99, 99, 99
		}
	})
	s := snapOf(t, newP(t, fl), "a\n")
	n := s.Node(1)
	if !n.IsMissing() || n.StartPoint() != (Point{1, 0}) || n.EndPoint() != (Point{1, 0}) {
		t.Fatalf("missing EOF node: %v %v", n.StartPoint(), n.EndPoint())
	}
	if got := s.DescendantForPointRange(Point{1, 0}, Point{1, 0}, true); got != n {
		t.Fatalf("point lookup found %v instead of missing EOF node", got.Index())
	}
}
