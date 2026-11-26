package treesitter

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

var bg = context.Background()

func newP(t *testing.T, fl *fakeLang, opts ...Option) *Parser {
	t.Helper()
	p, err := NewParser(fl.lang, opts...)
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func mustParse(t *testing.T, p *Parser, src string) *Tree {
	t.Helper()
	tr, err := p.Parse(bg, []byte(src), nil)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return tr
}

func wantPE(t *testing.T, err error, o Outcome, reset bool) *ParseError {
	t.Helper()
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T %v", err, err)
	}
	if pe.Outcome != o || pe.InstanceReset != reset {
		t.Fatalf("want %v reset=%v, got %v reset=%v (%v)", o, reset, pe.Outcome, pe.InstanceReset, pe)
	}
	return pe
}

func blockUntilStop(f *fakeModule, src []byte, _ int32) (int32, bool) {
	for f.h.Stop.Load() == 0 {
		runtime.Gosched()
	}
	f.halted = buildFake(src)
	return 0, true
}

func TestAcceptedOutcomes(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	clean, bad := mustParse(t, p, "a + b"), mustParse(t, p, "a ! b")
	if clean.Outcome() != AcceptedClean || bad.Outcome() != AcceptedWithErrors {
		t.Fatalf("outcomes %v %v", clean.Outcome(), bad.Outcome())
	}
	if clean.InputBytes() != 5 || clean.HasInvalidUTF8() {
		t.Fatalf("input bytes %d invalid %v", clean.InputBytes(), clean.HasInvalidUTF8())
	}
	snap, err := bad.Snapshot(bg)
	if err != nil || !snap.Root().HasError() {
		t.Fatalf("snapshot %v err %v", snap, err)
	}
}

func TestTimeoutThenReparse(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.parseHook = blockUntilStop })
	p := newP(t, fl)
	ctx, cancel := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel()
	tr, err := p.Parse(ctx, []byte("aaa bbb"), nil)
	pe := wantPE(t, err, Timeout, false)
	if tr != nil || !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrCancelled) {
		t.Fatalf("tree %v, errors.Is mismatch for %v", tr, pe)
	}
	f := fl.cur()
	if f.count("reset") != 1 || len(fl.mods) != 1 || f.h.Stop.Load() != 0 {
		t.Fatalf("resets %d instances %d stop %d", f.count("reset"), len(fl.mods), f.h.Stop.Load())
	}
	f.parseHook = nil // cancel-then-reparse: a different source must give its own tree, not the halted one
	tr2 := mustParse(t, p, "xyz")
	snap, err := tr2.Snapshot(bg)
	if err != nil || snap.Root().EndByte() != 3 || snap.Root().Text() != "xyz" {
		t.Fatalf("second parse did not return its own tree: %v %v", snap, err)
	}
}

func TestCancelled(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.parseHook = blockUntilStop })
	p := newP(t, fl)
	ctx, cancel := context.WithCancel(bg)
	time.AfterFunc(10*time.Millisecond, cancel)
	_, err := p.Parse(ctx, []byte("a"), nil)
	wantPE(t, err, Cancelled, false)
	if !errors.Is(err, ErrCancelled) || !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
		t.Fatalf("errors.Is mismatch: %v", err)
	}
	if fl.cur().count("reset") != 1 {
		t.Fatalf("resets %d", fl.cur().count("reset"))
	}
}

func TestContextAdmission(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	//lint:ignore SA1012 deliberately nil
	_, err := p.Parse(nil, []byte("a"), nil) //nolint
	wantPE(t, err, InvalidInput, false)
	done, cancel := context.WithCancel(bg)
	cancel()
	_, err = p.Parse(done, []byte("a"), nil)
	wantPE(t, err, Cancelled, false)
	if fl.cur().count("parse") != 0 || fl.cur().count("malloc") != 0 {
		t.Fatal("the engine must not be touched when the context is nil or done")
	}
	if _, err = p.ParseSnapshot(done, []byte("a")); !errors.Is(err, ErrCancelled) {
		t.Fatalf("ParseSnapshot: %v", err)
	}
}

func TestInputLimit(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl, WithMaxInputBytes(4))
	_, err := p.Parse(bg, []byte("abcdef"), nil)
	wantPE(t, err, ResourceLimit, false)
	if !errors.Is(err, ErrResourceLimit) || fl.cur().count("malloc") != 0 {
		t.Fatalf("limit must be checked before any engine work: %v", err)
	}
	mustParse(t, p, "ab")
	if len(fl.mods) != 1 {
		t.Fatal("instance must be kept")
	}
}

func TestSnapshotNodeCap(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl, WithMaxSnapshotNodes(3))
	_, err := p.ParseSnapshot(bg, []byte("a b c")) // root + 3 words = 4 nodes
	wantPE(t, err, ResourceLimit, false)
	f := fl.cur()
	if f.count("export_begin") != 0 || f.count("delete") != 1 || f.count("reset") < 1 || len(fl.mods) != 1 {
		t.Fatalf("cap must be checked before export, tree freed and parser reset: %v", f.calls)
	}
	tr := mustParse(t, p, "a b c")
	_, err = tr.Snapshot(bg)
	wantPE(t, err, ResourceLimit, false)
	_, exportErr := p.c.export(bg, tr.ptr, func() string {
		t.Fatal("snapshot source requested before checking the node limit")
		return ""
	}, false)
	wantPE(t, exportErr, ResourceLimit, false)
	tr.Close()
	if _, err := p.ParseSnapshot(bg, []byte("a b")); err != nil { // 3 nodes fit
		t.Fatal(err)
	}
}

func TestParseSnapshotCancelBeforeCopies(t *testing.T) {
	for _, tc := range []struct {
		cancelAt        string
		parses, exports int
	}{
		{cancelAt: "malloc"},
		{cancelAt: "parse", parses: 1},
		{cancelAt: "export_begin", parses: 1, exports: 1},
	} {
		t.Run(tc.cancelAt, func(t *testing.T) {
			fl := newFakeLang(nil)
			p := newP(t, fl)
			f := fl.cur()
			ctx, cancel := context.WithCancel(bg)
			defer cancel()
			f.onCall = func(name string) {
				if name == tc.cancelAt {
					cancel()
				}
			}
			snap, err := p.ParseSnapshot(ctx, []byte("a b"))
			wantPE(t, err, Cancelled, false)
			if snap != nil || f.count("free") != 1 || len(f.trees) != 0 || f.count("export_next") != 0 {
				t.Fatalf("unexpected work or retained tree after cancellation: %v", f.calls)
			}
			if f.count("parse") != tc.parses || f.count("export_begin") != tc.exports || f.count("export_end") != tc.exports {
				t.Fatalf("unexpected parse or export calls: %v", f.calls)
			}
			f.onCall = nil
			if snap, err := p.ParseSnapshot(bg, []byte("next")); err != nil || snap.Source() != "next" || fl.cur() != f {
				t.Fatalf("reuse after cancellation: snapshot %v, error %v", snap, err)
			}
		})
	}
}

func TestRefusedGrow(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
		if len(fl.mods) == 1 {
			f.parseHook = func(f *fakeModule, src []byte, _ int32) (int32, bool) {
				f.refused = true
				return f.register(buildFake(src)), true
			}
		}
	})
	p := newP(t, fl)
	tr, err := p.Parse(bg, []byte("a b"), nil)
	wantPE(t, err, ResourceLimit, true)
	if tr != nil || len(fl.mods) != 2 || !fl.mods[0].released.Load() {
		t.Fatalf("instances %d released %v", len(fl.mods), fl.mods[0].released.Load())
	}
	if mustParse(t, p, "a b").Outcome() != AcceptedClean { // the recreated instance works
		t.Fatal("recreated instance")
	}
}

func TestTrap(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refused bool
		want    Outcome
	}{{"unreachable", false, EngineFault}, {"refused grow then unreachable", true, ResourceLimit}} {
		t.Run(tc.name, func(t *testing.T) {
			fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
				if len(fl.mods) == 1 {
					f.parseHook = func(f *fakeModule, _ []byte, _ int32) (int32, bool) {
						f.refused = tc.refused
						panic("unreachable")
					}
				}
			})
			p := newP(t, fl)
			tr, err := p.Parse(bg, []byte("a"), nil)
			pe := wantPE(t, err, tc.want, true)
			var trap *engine.Trap
			if tr != nil || !errors.As(err, &trap) || !strings.Contains(pe.Reason, "tsg_parse") {
				t.Fatalf("tree %v trap %v reason %q", tr, trap, pe.Reason)
			}
			if len(fl.mods) != 2 || !fl.mods[0].released.Load() {
				t.Fatalf("instance must be recreated: %d", len(fl.mods))
			}
			mustParse(t, p, "a")
		})
	}
}

func TestTrapInvalidatesTrees(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
		if len(fl.mods) == 1 {
			f.parseHook = func(f *fakeModule, src []byte, _ int32) (int32, bool) {
				if string(src) == "boom" {
					panic("unreachable")
				}
				return 0, false
			}
		}
	})
	p := newP(t, fl)
	tr := mustParse(t, p, "a b")
	snap, _ := tr.Snapshot(bg)
	_, err := p.Parse(bg, []byte("boom"), nil)
	wantPE(t, err, EngineFault, true)
	if _, err := tr.Snapshot(bg); !errors.Is(err, ErrTreeInvalidated) {
		t.Fatalf("tree after instance reset: %v", err)
	}
	if snap.Root().Text() != "a b" { // snapshots stay valid
		t.Fatal("snapshot must survive")
	}
	tr.Close() // must not touch the dead instance
	if fl.mods[1].count("delete") != 0 {
		t.Fatal("old tree handle must not be freed in the new instance")
	}
}

func TestNoTreePathsReset(t *testing.T) {
	t.Run("NULL without cancellation", func(t *testing.T) {
		fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
			if len(fl.mods) == 1 {
				f.parseHook = func(f *fakeModule, src []byte, _ int32) (int32, bool) { return 0, true }
			}
		})
		p := newP(t, fl)
		_, err := p.Parse(bg, []byte("a"), nil)
		wantPE(t, err, EngineFault, true)
		if fl.mods[0].count("reset") < 1 {
			t.Fatalf("parser must be reset before the instance is replaced: %v", fl.mods[0].calls)
		}
	})
	t.Run("root end mismatch", func(t *testing.T) {
		fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
			if len(fl.mods) == 1 {
				f.rootEndDelta = 1
			}
		})
		p := newP(t, fl)
		_, err := p.Parse(bg, []byte("a"), nil)
		wantPE(t, err, EngineFault, true)
		f := fl.mods[0]
		if f.count("delete") != 1 || f.count("reset") < 1 || len(fl.mods) != 2 {
			t.Fatalf("calls %v", f.calls)
		}
	})
}

func TestStrictUTF8(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr, err := p.Parse(bg, []byte("a\xffb"), nil)
	if err != nil || !tr.HasInvalidUTF8() {
		t.Fatalf("lenient: %v %v", tr, err)
	}
	s := newP(t, fl, WithStrictUTF8())
	_, err = s.Parse(bg, []byte("a\xffb"), nil)
	wantPE(t, err, InvalidInput, false)
	if fl.mods[1].count("parse") != 0 { // the strict parser's engine was never reached
		t.Fatalf("strict mode must reject before the engine: %d", fl.mods[1].count("parse"))
	}
	if tr, err := s.Parse(bg, []byte("\xef\xbb\xbfa"), nil); err != nil || tr == nil { // a BOM is valid UTF-8
		t.Fatalf("BOM input: %v", err)
	}
}

func TestOldTreeChecks(t *testing.T) {
	a, b := newFakeLang(nil), newFakeLang(nil)
	pa, pb := newP(t, a), newP(t, b)
	ta := mustParse(t, pa, "a")
	_, err := pb.Parse(bg, []byte("a"), ta)
	wantPE(t, err, InvalidInput, false)
	if b.cur().count("parse") != 0 {
		t.Fatal("foreign old tree must be rejected before the engine")
	}
	ta.Close()
	_, err = pa.Parse(bg, []byte("a"), ta)
	wantPE(t, err, InvalidInput, false)
	if !errors.Is(err, ErrTreeInvalidated) {
		t.Fatalf("closed old tree: %v", err)
	}
}

func TestIncompatibleLanguage(t *testing.T) {
	reject := func(fl *fakeLang, instances int) {
		t.Helper()
		p, err := NewParser(fl.lang)
		if p != nil {
			p.Close()
			t.Fatal("incompatible language created a parser")
		}
		wantPE(t, err, IncompatibleLanguage, false)
		if !errors.Is(err, ErrIncompatibleLanguage) {
			t.Fatalf("NewParser: %v", err)
		}
		if len(fl.mods) != instances {
			t.Fatalf("created %d instances, want %d", len(fl.mods), instances)
		}
		if instances == 1 && !fl.mods[0].released.Load() {
			t.Fatal("rejected instance must be released")
		}
	}

	fl := newFakeLang(nil)
	fl.lang.g.ABI = engine.ABIVersion + 1
	reject(fl, 0)
	fl = newFakeLang(func(_ *fakeLang, f *fakeModule) { f.abi = engine.ABIVersion + 1 })
	reject(fl, 1)
	fl = newFakeLang(func(_ *fakeLang, f *fakeModule) { f.initRC = -1 })
	reject(fl, 1)
	fl = newFakeLang(func(_ *fakeLang, f *fakeModule) { f.badMagic = true }) // header wrong from tsg_init on
	reject(fl, 1)
	if fl.mods[0].count("export_begin") != 0 {
		t.Fatal("NewParser must reject the init-time header before export")
	}
	fl = newFakeLang(func(_ *fakeLang, f *fakeModule) { f.kc = 3 }) // capacity not a multiple of 4
	_, err := NewParser(fl.lang)
	wantPE(t, err, EngineFault, false)
	fl = newFakeLang(nil)
	p := newP(t, fl)
	tr := mustParse(t, p, "a b")
	fl.cur().badExportMagic = true // the per-export check stays
	_, err = tr.Snapshot(bg)
	wantPE(t, err, IncompatibleLanguage, false)
	if fl.cur().count("export_end") != 1 {
		t.Fatal("a rejected export must be abandoned")
	}
	if _, err = p.ParseSnapshot(bg, []byte("a")); !errors.Is(err, ErrIncompatibleLanguage) {
		t.Fatalf("ParseSnapshot: %v", err)
	}
}

func TestOptionValidation(t *testing.T) {
	fl := newFakeLang(nil)
	for name, o := range map[string]Option{
		"nil option":                  nil,
		"input<0":                     WithMaxInputBytes(-1),
		"input>2GiB":                  WithMaxInputBytes(1 << 31),
		"memory<0":                    WithMaxMemoryBytes(-1),
		"memory>4GiB":                 WithMaxMemoryBytes(1<<32 + 1),
		"nodes<0":                     WithMaxSnapshotNodes(-5),
		"recycle<0":                   WithRecycleAboveBytes(-1),
		"memory below engine minimum": WithMaxMemoryBytes(4096),
	} {
		p, err := NewParser(fl.lang, o)
		if p != nil {
			t.Errorf("%s: parser created", name)
		}
		wantPE(t, err, InvalidInput, false)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
		if pool, err := NewParserPool(fl.lang, 1, o); pool != nil || !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: NewParserPool = %p, %v", name, pool, err)
		}
	}
	if _, err := NewParser(nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil language: %v", err)
	}
	n := len(fl.mods)
	p := newP(t, fl, WithMaxMemoryBytes(8<<20), WithMaxInputBytes(1<<20))
	if fl.mods[n].limit != 8<<20 {
		t.Fatalf("limit not applied: %d", fl.mods[n].limit)
	}
	_ = p
}

func TestInvalidationAfterCloseAndReset(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr, tr2 := mustParse(t, p, "a b"), mustParse(t, p, "c d")
	snap, err := tr.Snapshot(bg)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(fl.mods) != 2 || !fl.mods[0].released.Load() {
		t.Fatal("Reset must recreate the instance and release the old memory")
	}
	check := func(tr *Tree) {
		t.Helper()
		if _, err := tr.Snapshot(bg); !errors.Is(err, ErrTreeInvalidated) {
			t.Fatalf("Snapshot: %v", err)
		}
		if err := tr.Edit(InputEdit{}); !errors.Is(err, ErrTreeInvalidated) {
			t.Fatalf("Edit: %v", err)
		}
		if _, err := tr.ChangedRanges(tr); !errors.Is(err, ErrTreeInvalidated) {
			t.Fatalf("ChangedRanges: %v", err)
		}
	}
	check(tr)
	check(tr2)
	tr.Close() // no engine call for a dead tree
	if fl.mods[1].count("delete") != 0 {
		t.Fatal("stale handle freed in the new instance")
	}
	tr3 := mustParse(t, p, "e")
	if err := p.Close(); err != nil || p.Close() != nil {
		t.Fatal("Close must be idempotent")
	}
	if !fl.mods[1].released.Load() {
		t.Fatal("Close must release the instance")
	}
	check(tr3)
	if _, err := p.Parse(bg, []byte("a"), nil); !errors.Is(err, ErrParserClosed) {
		t.Fatalf("Parse after Close: %v", err)
	}
	if snap.Root().Text() != "a b" || snap.Len() != 3 {
		t.Fatal("snapshots must stay valid after Close")
	}
}

func TestEditAndChangedRanges(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	old := []byte("a b\nc")
	tr, err := p.Parse(bg, old, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := tr.Snapshot(bg)
	edit := EditFor(old, []byte("a bx\nc"), 3, 3, 4)
	if edit.StartPoint != (Point{0, 3}) || edit.NewEndPoint != (Point{0, 4}) || edit.OldEndPoint != (Point{0, 3}) {
		t.Fatalf("EditFor %+v", edit)
	}
	for _, bad := range []InputEdit{{StartByte: 5, OldEndByte: 4}, {OldEndByte: 99, NewEndByte: 99}, {StartPoint: Point{2, 0}}} {
		if err := tr.Edit(bad); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("bad edit %+v accepted: %v", bad, err)
		}
	}
	if err := tr.Edit(edit); err != nil {
		t.Fatal(err)
	}
	if tr.InputBytes() != 6 {
		t.Fatalf("InputBytes %d", tr.InputBytes())
	}
	if _, err := tr.Snapshot(bg); !errors.Is(err, ErrTreeEdited) {
		t.Fatalf("edited tree snapshot: %v", err)
	}
	if got := fl.cur().edits[0]; got != [10]int32{1, 3, 3, 4, 0, 3, 0, 3, 0, 4} {
		t.Fatalf("edit args %v", got)
	}
	nt, err := p.Parse(bg, []byte("a bx\nc"), tr)
	if err != nil || fl.cur().lastOld != 1 {
		t.Fatalf("incremental parse: %v old=%d", err, fl.cur().lastOld)
	}
	fl.cur().ranges = []Range{{3, 4, Point{0, 3}, Point{0, 4}}, {5, 6, Point{1, 0}, Point{1, 1}}}
	rs, err := nt.ChangedRanges(tr)
	if err != nil || !slices.Equal(rs, fl.cur().ranges) {
		t.Fatalf("ranges %v %v", rs, err)
	}
	if _, err := tr.ChangedRanges(nt); !errors.Is(err, ErrTreeEdited) {
		t.Fatalf("an edited tree is not a result tree: %v", err)
	}
	if _, err := nt.ChangedRanges(nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil old: %v", err)
	}
	if fl.cur().count("free") < 2 {
		t.Fatal("source buffers and range blocks must be freed")
	}
	if snap.Root().Text() != "a b\nc" {
		t.Fatal("pre-edit snapshot must stay valid")
	}
}

func TestChangedRangesNullIsEngineFault(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) { f.nullRanges = len(fl.mods) == 1 })
	p := newP(t, fl)
	old := mustParse(t, p, "a")
	nt, _ := p.Parse(bg, []byte("a b"), old)
	_, err := nt.ChangedRanges(old)
	wantPE(t, err, EngineFault, true)
	if len(fl.mods) != 2 {
		t.Fatal("instance must be recreated")
	}
}

//go:noinline
func leakTree(p *Parser) { p.Parse(bg, []byte("a"), nil) }

func TestLeakedTreeFreedByOwner(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	leakTree(p)
	pending := func() int { p.c.mu.Lock(); defer p.c.mu.Unlock(); return len(p.c.leaked) }
	for i := 0; i < 400 && pending() == 0; i++ {
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
	if pending() == 0 {
		t.Fatal("leaked tree was never queued by its cleanup")
	}
	if fl.cur().count("delete") != 0 {
		t.Fatal("the GC goroutine must only enqueue, never call the engine")
	}
	mustParse(t, p, "b")
	if fl.cur().count("delete") != 1 || p.c.live != 1 {
		t.Fatalf("owner must drain on the next Parse: deletes %d live %d", fl.cur().count("delete"), p.c.live)
	}
}

func TestTreeCloseOnce(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr := mustParse(t, p, "a")
	tr.Close()
	tr.Close()
	if fl.cur().count("delete") != 1 || p.c.live != 0 {
		t.Fatalf("deletes %d live %d", fl.cur().count("delete"), p.c.live)
	}
}

func TestTreeBusyGuard(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr, old := mustParse(t, p, "a"), mustParse(t, p, "b")
	defer runtime.KeepAlive(old)
	if _, err := tr.Snapshot(bg); err != nil {
		t.Fatal(err)
	}
	started, resume := make(chan struct{}), make(chan struct{})
	f := fl.cur()
	f.onCall = func(name string) {
		if name == "edit" {
			close(started)
			<-resume
		}
	}
	edited := make(chan error, 1)
	go func() { edited <- tr.Edit(InputEdit{NewEndByte: 1}) }()
	select {
	case <-started:
	case err := <-edited:
		t.Fatalf("Edit did not reach the engine: %v", err)
	}
	snap, snapshotErr := tr.Snapshot(bg)
	editErr := tr.Edit(InputEdit{})
	_, rangesErr := tr.ChangedRanges(old)

	var closers sync.WaitGroup
	for range 2 {
		closers.Go(tr.Close)
	}
	closers.Wait()
	deletesWhileBusy := f.count("delete")

	inputBytes := make(chan int, 1)
	go func() {
		<-resume
		inputBytes <- tr.InputBytes()
	}()
	close(resume)
	if err := <-edited; err != nil {
		t.Fatal(err)
	}
	if n := <-inputBytes; (n != 1 && n != 2) || tr.InputBytes() != 2 {
		t.Fatalf("input length during/after Edit: %d/%d", n, tr.InputBytes())
	}
	if snap != nil || !errors.Is(snapshotErr, ErrInvalidInput) || !errors.Is(editErr, ErrInvalidInput) ||
		!errors.Is(rangesErr, ErrInvalidInput) {
		t.Fatalf("busy Snapshot cache hit = %t, Snapshot error = %v, Edit error = %v, ChangedRanges error = %v",
			snap != nil, snapshotErr, editErr, rangesErr)
	}
	if deletesWhileBusy != 0 {
		t.Fatal("Close deleted a tree while the parser was busy")
	}
	if _, err := tr.Snapshot(bg); !errors.Is(err, ErrTreeInvalidated) {
		t.Fatalf("Snapshot after Close: %v", err)
	}
	mustParse(t, p, "next")
	if f.count("delete") != 1 {
		t.Fatalf("deferred Close deleted %d trees, want 1", f.count("delete"))
	}
}

func TestParseOldTreeClosedDuringDrain(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	old, queued := mustParse(t, p, "a"), mustParse(t, p, "b")
	if err := old.Edit(InputEdit{}); err != nil {
		t.Fatal(err)
	}
	p.c.busy.Store(true)
	queued.Close()
	p.c.end()
	f := fl.cur()
	f.onCall = func(name string) {
		if name == "delete" {
			old.Close()
		}
	}
	tr, err := p.Parse(bg, []byte("c"), old)
	if tr != nil || !errors.Is(err, ErrTreeInvalidated) || f.count("parse") != 2 {
		t.Fatalf("Parse with old tree closed during cleanup: tree %p, error %v, calls %v", tr, err, f.calls)
	}
	f.onCall = nil
	mustParse(t, p, "next")
	if f.count("delete") != 2 || p.c.live != 1 {
		t.Fatalf("deferred closes: deletes %d, live trees %d", f.count("delete"), p.c.live)
	}
}

func TestRecycle(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl, WithRecycleAboveBytes(1<<10))
	if _, err := p.ParseSnapshot(bg, []byte("a b")); err != nil {
		t.Fatal(err)
	}
	if len(fl.mods) != 2 || !fl.mods[0].released.Load() {
		t.Fatalf("ParseSnapshot must recycle at once: %d instances", len(fl.mods))
	}
	tr := mustParse(t, p, "a") // a live tree must never be invalidated by a recycle
	if len(fl.mods) != 2 {
		t.Fatal("recycled while a tree was live")
	}
	if _, err := tr.Snapshot(bg); err != nil {
		t.Fatal(err)
	}
	tr.Close()
	if len(fl.mods) != 3 {
		t.Fatalf("recycle after the last tree closes: %d instances", len(fl.mods))
	}
}

func TestSingleOwnerGuard(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.parseHook = blockUntilStop })
	p := newP(t, fl)
	ctx, cancel := context.WithCancel(bg)
	done := make(chan error, 1)
	go func() { _, err := p.Parse(ctx, []byte("a"), nil); done <- err }()
	for fl.cur().count("parse") == 0 {
		runtime.Gosched()
	}
	_, err := p.Parse(bg, []byte("b"), nil)
	wantPE(t, err, InvalidInput, false)
	if err := p.Reset(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Reset while busy: %v", err)
	}
	cancel()
	wantPE(t, <-done, Cancelled, false)
}

func TestSnapshotExportCancel(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr := mustParse(t, p, "a b c d e f g h") // root + 8 = 9 nodes, chunk capacity 4
	ctx, cancel := context.WithCancel(bg)
	fl.cur().nextHook = func(f *fakeModule) {
		if f.count("export_next") == 1 {
			cancel()
		}
	}
	_, err := tr.Snapshot(ctx)
	wantPE(t, err, Cancelled, false)
	f := fl.cur()
	if f.count("export_end") != 1 || f.count("export_next") != 1 {
		t.Fatalf("export must stop between chunks: %v", f.calls)
	}
	f.nextHook = nil
	snap, err := tr.Snapshot(bg) // the tree is intact
	if err != nil || snap.Len() != 9 {
		t.Fatalf("retry: %v", err)
	}
	if again, err := tr.Snapshot(ctx); err != nil || again != snap || f.count("export_begin") != 2 {
		t.Fatalf("cached snapshot with a canceled context: %v", err)
	}
}

func TestCorruptExportIsEngineFault(t *testing.T) {
	for name, corrupt := range map[string]func([]fnode){
		"symbol out of range":   func(n []fnode) { n[1].sym = 500 },
		"field out of range":    func(n []fnode) { n[1].field = 9 },
		"range beyond source":   func(n []fnode) { n[1].eb = 999 },
		"reversed range":        func(n []fnode) { n[1].sb, n[1].eb = 2, 1 },
		"parent is a successor": func(n []fnode) { n[1].parent = 2 },
		"parent off the path":   func(n []fnode) { n[3].parent = 1 }, // word 1 is a leaf, not on node 2's path
		"root has a parent":     func(n []fnode) { n[0].parent = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
				if len(fl.mods) == 1 {
					f.corrupt = corrupt
				}
			})
			p := newP(t, fl)
			tr := mustParse(t, p, "a b c")
			_, err := tr.Snapshot(bg)
			wantPE(t, err, EngineFault, true)
			if len(fl.mods) != 2 {
				t.Fatal("instance must be recreated")
			}
		})
	}
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.dcDelta = 1 })
	tr := mustParse(t, newP(t, fl), "a")
	_, err := tr.Snapshot(bg)
	wantPE(t, err, EngineFault, true)
}

func TestOwnership(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	src := []byte("abc def")
	tr, _ := p.Parse(bg, src, nil)
	snap, _ := tr.Snapshot(bg)
	if unsafe.StringData(snap.Source()) == &src[0] {
		t.Fatal("Parse must copy")
	}
	src[0] = 'X'
	if snap.Source() != "abc def" {
		t.Fatal("snapshot must own its source")
	}
	own := []byte("abc def")
	tr2, _ := p.ParseOwned(bg, own, nil)
	snap2, _ := tr2.Snapshot(bg)
	if unsafe.StringData(snap2.Source()) != &own[0] {
		t.Fatal("ParseOwned must not copy")
	}
	ps, err := p.ParseSnapshot(bg, own)
	if err != nil || unsafe.StringData(ps.Source()) == &own[0] {
		t.Fatalf("ParseSnapshot copies: %v", err)
	}
}

func TestParseCallsLeaveNoTreeBehind(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	if _, err := p.ParseSnapshot(bg, []byte("a b")); err != nil {
		t.Fatal(err)
	}
	if f := fl.cur(); len(f.trees) != 0 || f.count("free") != 1 {
		t.Fatalf("ParseSnapshot must free the tree and the source buffer: trees %d", len(f.trees))
	}
}

// WASI proc_exit from generated code (e.g. Swift's scanner OOM branch) is a trap: EngineFault, instance recreated,
// and the same parser parses again. A refused memory.grow before it explains it as ResourceLimit (TestTrap).
func TestProcExitIsEngineFault(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
		if len(fl.mods) == 1 {
			f.parseHook = func(f *fakeModule, _ []byte, _ int32) (int32, bool) {
				f.h.Xproc_exit(1)
				return 0, false
			}
		}
	})
	p := newP(t, fl)
	tr, err := p.Parse(bg, []byte("a"), nil)
	pe := wantPE(t, err, EngineFault, true)
	var trap *engine.Trap
	if tr != nil || !errors.As(err, &trap) || !strings.Contains(pe.Reason, "proc_exit(1)") {
		t.Fatalf("tree %v trap %v reason %q", tr, trap, pe.Reason)
	}
	if len(fl.mods) != 2 || !fl.mods[0].released.Load() {
		t.Fatalf("instance must be recreated: %d", len(fl.mods))
	}
	mustParse(t, p, "a")
}
