package treesitter

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// Coverage-gap tests on the fake engine: error, limit and recovery paths the contract names but no test reached.

func TestInputLimitBoundary(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl, WithMaxInputBytes(4))
	if _, err := p.Parse(bg, []byte("abcd"), nil); err != nil { // n == limit is accepted
		t.Fatalf("n == limit: %v", err)
	}
	for name, call := range map[string]func() error{
		"Parse":         func() error { _, e := p.Parse(bg, []byte("abcde"), nil); return e },
		"ParseOwned":    func() error { _, e := p.ParseOwned(bg, []byte("abcde"), nil); return e },
		"ParseSnapshot": func() error { _, e := p.ParseSnapshot(bg, []byte("abcde")); return e },
	} {
		err := call()
		wantPE(t, err, ResourceLimit, false)
		if !errors.Is(err, ErrResourceLimit) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(fl.mods) != 1 {
		t.Fatal("a refused input must keep the instance")
	}
	zero := newP(t, fl, WithMaxInputBytes(0)) // zero = no explicit limit
	mustParse(t, zero, "abcdefghij")
}

func TestExpiredDeadlineIsTimeoutBeforeTheEngine(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	ctx, cancel := context.WithDeadline(bg, time.Now().Add(-time.Second))
	defer cancel()
	fresh := mustParse(t, p, "x y") // a tree whose snapshot is not cached yet
	f := fl.cur()
	beforeCalls, beforeMods := len(f.calls), len(fl.mods)
	check := func(name string, err error) {
		t.Helper()
		wantPE(t, err, Timeout, false)
		if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrCancelled) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := p.Parse(ctx, []byte("a"), nil)
	check("Parse", err)
	_, err = p.ParseSnapshot(ctx, []byte("a"))
	check("ParseSnapshot", err)
	_, err = fresh.Snapshot(ctx)
	check("Tree.Snapshot", err)
	_, err = collectQuery(tree, ctx, q, nil)
	check("Tree.Matches", err)
	if len(f.calls) != beforeCalls || len(fl.mods) != beforeMods {
		t.Fatalf("an expired context reached the engine: calls %v, instances %d -> %d", f.calls[beforeCalls:], beforeMods, len(fl.mods))
	}
	if !tree.valid() || !fresh.valid() {
		t.Fatal("an expired context invalidated a tree")
	}
}

func TestPoolGetFailureReleasesItsSlot(t *testing.T) {
	fl := newFakeLang(nil)
	var fail atomic.Bool
	create := fl.lang.g.New
	fl.lang.g.New = func(h *engine.Host) engine.Module {
		if fail.Load() {
			panic("scripted instantiate trap")
		}
		return create(h)
	}
	pl, err := NewParserPool(fl.lang, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	fail.Store(true)
	for i := 0; i < 3; i++ { // each failure must give the only slot back
		ctx, cancel := context.WithTimeout(bg, 2*time.Second)
		_, err := pl.Get(ctx)
		cancel()
		wantPE(t, err, EngineFault, false)
		if errors.Is(err, ErrTimeout) {
			t.Fatal("the slot of a failed Get leaked: the next Get waited for it")
		}
	}
	fail.Store(false)
	p, err := pl.Get(bg)
	if err != nil {
		t.Fatalf("Get after the failures: %v", err)
	}
	pl.Put(p)
}

func TestPoolPutOfBusyParserIsNotReused(t *testing.T) {
	fl := newFakeLang(nil)
	q, err := NewQuery(fl.lang, strings.Repeat(" ", 100))
	if err != nil {
		t.Fatal(err)
	}
	pl, _ := NewParserPool(fl.lang, 1)
	defer pl.Close()
	p, _ := pl.Get(bg)
	tr := mustParse(t, p, "one two")
	for _, err := range tr.Matches(bg, q, nil) { // the parser is busy for the whole loop
		if err != nil {
			t.Fatal(err)
		}
		pl.Put(p)
		break
	}
	ctx, cancel := context.WithTimeout(bg, 2*time.Second)
	defer cancel()
	p2, err := pl.Get(ctx) // the slot was released although the parser was not returned to the idle list
	if err != nil {
		t.Fatalf("Get after Put of a busy parser: %v", err)
	}
	if p2 == p {
		t.Fatal("a parser that was busy during Put must not be lent again")
	}
	pl.Put(p2)
}

func TestFailedRecreateIsRetriedByTheNextOperation(t *testing.T) {
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
	var fail atomic.Bool
	create := fl.lang.g.New
	fl.lang.g.New = func(h *engine.Host) engine.Module {
		if fail.Load() {
			panic("scripted instantiate trap")
		}
		return create(h)
	}
	p := newP(t, fl)
	old := mustParse(t, p, "a b")
	fail.Store(true) // the engine fault below cannot recreate the instance
	_, err := p.Parse(bg, []byte("boom"), nil)
	wantPE(t, err, EngineFault, true)
	if p.c.inst != nil {
		t.Fatal("a failed recreate left an instance installed")
	}
	_, err = p.Parse(bg, []byte("a"), nil) // still failing: an error, and the parser must not stay busy
	wantPE(t, err, EngineFault, false)
	if p.c.busy.Load() {
		t.Fatal("a failed begin left the parser busy")
	}
	if _, err := old.Snapshot(bg); !errors.Is(err, ErrTreeInvalidated) {
		t.Fatalf("old tree: %v", err)
	}
	old.Close() // no engine to talk to: must not panic
	fail.Store(false)
	if tr := mustParse(t, p, "a b"); tr.Outcome() != AcceptedClean {
		t.Fatal("the next operation must recreate the instance")
	}
	fail.Store(true)
	_, _ = p.Parse(bg, []byte("boom"), nil) // inst == nil again
	if err := p.Reset(); err == nil {
		t.Fatal("Reset must report a recreate failure")
	}
	fail.Store(false)
	mustParse(t, p, "ok")
}

func TestSnapshotCancelledBeforeTheFirstChunk(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	ctx, cancel := context.WithCancel(bg)
	fl.cur().onCall = func(name string) {
		if name == "export_begin" {
			cancel()
		}
	}
	_, err := p.ParseSnapshot(ctx, []byte("a b c"))
	wantPE(t, err, Cancelled, false)
	f := fl.cur()
	if f.count("export_next") != 0 || f.count("export_end") != 1 || len(f.trees) != 0 || f.count("reset") < 1 || len(fl.mods) != 1 {
		t.Fatalf("export must be ended, the tree freed and the parser reset: %v trees %d", f.calls, len(f.trees))
	}
	f.onCall = nil
	if _, err := p.ParseSnapshot(bg, []byte("a b")); err != nil {
		t.Fatal(err)
	}
}

func TestChangedRangesCorruptCountIsEngineFault(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) { f.badRangeCount = len(fl.mods) == 1 })
	p := newP(t, fl)
	old := mustParse(t, p, "a")
	nt, _ := p.Parse(bg, []byte("a b"), old)
	_, err := nt.ChangedRanges(old)
	wantPE(t, err, EngineFault, true)
	if len(fl.mods) != 2 || nt.valid() || old.valid() {
		t.Fatal("instance must be recreated and both trees invalidated")
	}
}

func TestIndexIteratorsStopWhenTheConsumerBreaks(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s := snapOf(t, p, "a + b + c d")
	sym, ok := s.Language().SymbolForName("word", true)
	if !ok {
		t.Fatal("word")
	}
	for name, seq := range map[string]func(func(Node) bool){
		"OfSymbol":              s.Index().OfSymbol(sym),
		"OfType single symbol":  s.Index().OfType("+"),
		"OfType merged symbols": s.Index().OfType("word"), // named and anonymous symbols share the name
	} {
		n := 0
		for range seq {
			n++
		}
		if n < 2 {
			t.Fatalf("%s needs at least two results to test early termination, got %d", name, n)
		}
		n = 0
		for range seq {
			n++
			break
		}
		if n != 1 {
			t.Errorf("%s yielded %d before the break", name, n)
		}
	}
}

func TestLanguageFieldTables(t *testing.T) {
	l := newFakeLang(nil).lang
	if l.FieldCount() != 3 { // ids 0..2, id 0 is the empty field
		t.Fatalf("FieldCount %d", l.FieldCount())
	}
	if l.FieldName(0) != "" || l.FieldName(99) != "" || l.FieldName(1) != "left" {
		t.Fatal("FieldName")
	}
	if _, ok := l.FieldForName(""); ok {
		t.Fatal("the empty name is not a field")
	}
	if id, ok := l.FieldForName("right"); !ok || id != 2 {
		t.Fatal("FieldForName")
	}
}

func TestEditForClampsOffsetsAndCountsBytes(t *testing.T) {
	old := []byte("é\nxy") // é is two bytes
	e := EditFor(old, old, 3, 99, 99)
	if e.StartPoint != (Point{1, 0}) || e.OldEndPoint != (Point{1, 2}) || e.NewEndPoint != (Point{1, 2}) || e.OldEndByte != 99 {
		t.Fatalf("%+v", e)
	}
	if got := EditFor(nil, nil, 0, 0, 0); got != (InputEdit{}) {
		t.Fatalf("empty: %+v", got)
	}
}

func TestSuccessiveEditsTrackTheInputLength(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr := mustParse(t, p, "a b\nc") // 5 bytes
	if err := tr.Edit(InputEdit{StartByte: 1, OldEndByte: 3, NewEndByte: 1, StartPoint: Point{0, 1}, OldEndPoint: Point{0, 3}, NewEndPoint: Point{0, 1}}); err != nil {
		t.Fatal(err)
	}
	if tr.InputBytes() != 3 {
		t.Fatalf("after a deletion: %d", tr.InputBytes())
	}
	if err := tr.Edit(InputEdit{StartByte: 3, OldEndByte: 4, NewEndByte: 4}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("an edit beyond the shrunken input: %v", err)
	}
	if err := tr.Edit(InputEdit{StartByte: 3, OldEndByte: 3, NewEndByte: 5}); err != nil || tr.InputBytes() != 5 {
		t.Fatalf("insertion: %v %d", err, tr.InputBytes())
	}
	if len(fl.cur().edits) != 2 {
		t.Fatalf("rejected edits must not reach the engine: %v", fl.cur().edits)
	}
	if _, err := p.Parse(bg, []byte("a b c"), tr); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Parse(bg, []byte("a b c"), tr); err != nil { // an edited old tree stays usable as old
		t.Fatalf("second incremental parse from the same old tree: %v", err)
	}
}

func TestUTF16ByteOrderMarksAndSurrogates(t *testing.T) {
	// A BOM of the other order is not dropped: U+FFFE is a valid scalar, not a replacement.
	other, _ := NewSourceUTF16([]byte{0xFE, 0xFF, 0x61, 0x00}, false)
	if string(other.Bytes()) != "\uFFFEa" || other.Replacements() != 0 || other.OriginalOffset(3) != 2 {
		t.Fatalf("%q %d %d", other.Bytes(), other.Replacements(), other.OriginalOffset(3))
	}
	rev, _ := NewSourceUTF16([]byte{0x00, 0xDC, 0x00, 0xD8}, false) // low then high
	if string(rev.Bytes()) != "\uFFFD\uFFFD" || rev.Replacements() != 2 || rev.OriginalOffset(3) != 2 {
		t.Fatalf("reversed pair: %q %d", rev.Bytes(), rev.Replacements())
	}
	hhl, _ := NewSourceUTF16([]byte{0x00, 0xD8, 0x00, 0xD8, 0x00, 0xDC}, false) // high, high, low
	if string(hhl.Bytes()) != "\uFFFD\U00010000" || hhl.Replacements() != 1 || hhl.OriginalOffset(3) != 2 {
		t.Fatalf("high high low: %q %d", hhl.Bytes(), hhl.Replacements())
	}
	trail, _ := NewSourceUTF16([]byte{0x61, 0x00, 0x00, 0xD8}, false) // lone high surrogate at the end
	if string(trail.Bytes()) != "a\uFFFD" || trail.Replacements() != 1 || trail.UTF8Offset(4) != 4 {
		t.Fatalf("trailing: %q %d", trail.Bytes(), trail.Replacements())
	}
	for e, want := range map[Encoding]string{UTF8: "UTF-8", UTF16LE: "UTF-16LE", UTF16BE: "UTF-16BE", CP949: "CP949", 9: "Encoding(9)"} {
		if e.String() != want {
			t.Errorf("%d: %q", e, e.String())
		}
	}
}

func TestOutcomeAndErrorStringsAreUsable(t *testing.T) {
	seen := map[string]bool{}
	for o := AcceptedClean; o <= IncompatibleLanguage; o++ {
		s := o.String()
		if s == "" || seen[s] {
			t.Errorf("outcome %d: %q", o, s)
		}
		seen[s] = true
	}
	if Outcome(0).String() == "" || Outcome(200).String() == "" {
		t.Error("an unknown outcome must still print")
	}
	for _, pe := range []*ParseError{{Outcome: Timeout}, {Outcome: EngineFault, Reason: "r", InstanceReset: true}} {
		if pe.Error() == "" {
			t.Errorf("empty diagnostic for outcome %d", pe.Outcome)
		}
	}
	if (&QueryError{Type: QueryErrorSyntax, Offset: 7}).Error() == "" || ColumnKind(9).String() == "" {
		t.Error("empty text")
	}
}

func TestQueryPredicatesOverCaptureLists(t *testing.T) {
	s := &Snapshot{src: "one two one", n: 3, startByte: []uint32{0, 4, 8}, endByte: []uint32{3, 7, 11}}
	cap0 := func(i int) QueryCapture { return QueryCapture{Index: 0, Node: s.Node(i)} }
	accepts := func(op string, extra []string, captures ...QueryCapture) bool {
		q := &Query{source: "query"}
		steps := [][2]uint32{{2, 0}, {1, 0}}
		lits := append([]string{op}, extra...)
		for i := range extra {
			steps = append(steps, [2]uint32{2, uint32(i + 1)})
		}
		if err := q.readMeta(fakeQueryMeta(lits, append(steps, [2]uint32{0, 0})), nil, nil); err != nil {
			t.Fatal(err)
		}
		return q.accepts(QueryMatch{Captures: captures}, nil)
	}
	one, two := cap0(0), cap0(1)
	for _, tc := range []struct {
		name  string
		op    string
		extra []string
		caps  []QueryCapture
		want  bool
	}{
		{"match? needs every capture to match", "match?", []string{"^o"}, []QueryCapture{one, two}, false},
		{"match? all match", "match?", []string{"^o"}, []QueryCapture{one, cap0(2)}, true},
		{"not-match? rejects when any capture matches", "not-match?", []string{"^o"}, []QueryCapture{one, two}, false},
		{"not-match? all differ", "not-match?", []string{"^o"}, []QueryCapture{two}, true},
		{"any-of? needs every capture in the list", "any-of?", []string{"one"}, []QueryCapture{one, two}, false},
		{"not-any-of? all outside", "not-any-of?", []string{"one"}, []QueryCapture{two}, true},
		{"eq? with a string and no capture is vacuous", "eq?", []string{"one"}, nil, true},
		{"not-eq? with a string and no capture is vacuous", "not-eq?", []string{"one"}, nil, true},
		{"match? with no capture is vacuous", "match?", []string{"^o"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := accepts(tc.op, tc.extra, tc.caps...); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// Predicates belong to their own pattern: a second pattern without predicates must not inherit them.
func TestQueryPredicatesArePerPattern(t *testing.T) {
	s := &Snapshot{src: "one two", n: 2, startByte: []uint32{0, 4}, endByte: []uint32{3, 7}}
	b := []byte("TSGQ")
	put := func(v uint32) { b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
	put(1)
	put(0) // size, patched below
	put(2) // patterns
	put(2) // captures
	put(2) // strings
	for _, str := range []string{"word", "other", "eq?", "one"} {
		put(uint32(len(str)))
		b = append(b, str...)
	}
	put(0) // pattern 0: start, 4 steps + terminator => (eq? @word "one")
	put(4)
	for _, st := range [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {0, 0}} {
		put(st[0])
		put(st[1])
	}
	put(0) // pattern 1: no predicates
	put(0)
	b[8], b[9], b[10], b[11] = byte(len(b)), byte(len(b)>>8), 0, 0
	q := &Query{source: "query"}
	if err := q.readMeta(b, nil, nil); err != nil {
		t.Fatal(err)
	}
	two := QueryCapture{Index: 0, Node: s.Node(1)}
	if q.accepts(QueryMatch{Pattern: 0, Captures: []QueryCapture{two}}, nil) {
		t.Fatal("pattern 0 must filter \"two\"")
	}
	if !q.accepts(QueryMatch{Pattern: 1, Captures: []QueryCapture{two}}, nil) {
		t.Fatal("pattern 1 must not inherit the predicate of pattern 0")
	}
}

// A trap at any call into the core ABI is an EngineFault with an instance reset, or a ResourceLimit when a grow was
// refused first; the parser recovers on the next operation (TestQueryABITraps does the same for the query ABI).
func TestCoreABITraps(t *testing.T) {
	type step func(p *Parser) error
	parse := func(p *Parser) error { _, e := p.Parse(bg, []byte("a b"), nil); return e }
	for _, tc := range []struct {
		name, trapAt string
		run          step
		prep         func(t *testing.T, p *Parser) step
	}{
		{name: "malloc", trapAt: "malloc", run: parse},
		{name: "free", trapAt: "free", run: parse},
		{name: "tsg_init during NewParser", trapAt: "init", run: nil}, // NewParser path, handled below
		{name: "tree_edit", trapAt: "edit", prep: func(t *testing.T, p *Parser) step {
			tr := mustParse(t, p, "a b")
			return func(*Parser) error { return tr.Edit(InputEdit{StartByte: 1, OldEndByte: 1, NewEndByte: 2}) }
		}},
		{name: "changed_ranges", trapAt: "changed_ranges", prep: func(t *testing.T, p *Parser) step {
			old := mustParse(t, p, "a")
			nt, _ := p.Parse(bg, []byte("a b"), old)
			return func(*Parser) error { _, e := nt.ChangedRanges(old); return e }
		}},
	} {
		for _, refused := range []bool{false, true} {
			name := tc.name
			if refused {
				name += "/refused grow first"
			}
			t.Run(name, func(t *testing.T) {
				want := EngineFault
				if refused {
					want = ResourceLimit
				}
				trapped := false
				armTrap := func(f *fakeModule) {
					f.onCall = func(name string) {
						if name == tc.trapAt {
							trapped = true
							f.refused = refused
							f.onCall = nil
							panic("scripted trap")
						}
					}
				}
				if tc.run == nil && tc.prep == nil { // NewParser traps
					fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { armTrap(f) })
					p, err := NewParser(fl.lang)
					if !trapped {
						t.Fatalf("NewParser did not trigger the %s trap", tc.trapAt)
					}
					if p != nil {
						t.Fatal("a parser must not exist after a trap during instantiation")
					}
					wantPE(t, err, want, false)
					if !fl.mods[0].released.Load() {
						t.Fatal("the failed instance must be released")
					}
					return
				}
				fl := newFakeLang(nil)
				p := newP(t, fl)
				run := tc.run
				if tc.prep != nil {
					run = tc.prep(t, p)
				}
				armTrap(fl.cur())
				err := run(p)
				if !trapped {
					t.Fatalf("operation did not trigger the %s trap", tc.trapAt)
				}
				wantPE(t, err, want, true)
				if len(fl.mods) != 2 || p.c.busy.Load() {
					t.Fatalf("instances %d busy %v", len(fl.mods), p.c.busy.Load())
				}
				mustParse(t, p, "a b") // the parser recovers
			})
		}
	}
}

func TestStringSexpMissingNodes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sym   uint16 // 2 = named word, 3 = anonymous "+"
		named uint8
		want  string
	}{
		{"named MISSING", fWord, flagNamed, "(source_file left: (MISSING word))"},
		{"anonymous MISSING", fPlus, 0, "(source_file left: (MISSING \"+\"))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
				f.corrupt = func(n []fnode) { n[1].sym, n[1].flags = tc.sym, tc.named|flagMissing }
			})
			p := newP(t, fl)
			s, err := p.ParseSnapshot(bg, []byte("x"))
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Root().String(); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
