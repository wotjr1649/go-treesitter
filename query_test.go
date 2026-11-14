package treesitter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

func queryFixture(t *testing.T, setup func(*fakeModule)) (*fakeLang, *Parser, *Tree, *Query) {
	t.Helper()
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
		if setup != nil {
			setup(f)
		}
	})
	q, err := NewQuery(fl.lang, strings.Repeat(" ", 100))
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(fl.lang)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	tree, err := p.Parse(context.Background(), []byte("one two one"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Close)
	return fl, p, tree, q
}
func collectQuery(t *Tree, ctx context.Context, q *Query, opts *QueryOptions) ([]QueryMatch, error) {
	var out []QueryMatch
	for m, err := range t.Matches(ctx, q, opts) {
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}
func TestQueryStreamingAndCache(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	got, err := collectQuery(tree, context.Background(), q, nil)
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Captures[0].Node.Text() != "one" || got[1].Captures[0].Node.Index() != 2 {
		t.Fatal(got)
	}
	names := q.CaptureNames()
	names[0] = "changed"
	if q.CaptureNames()[0] != "word" || q.PatternCount() != 1 {
		t.Fatal("metadata alias")
	}
	for _, err := range tree.Matches(context.Background(), q, nil) {
		if err != nil {
			t.Fatal(err)
		}
		if _, e := p.Parse(context.Background(), nil, nil); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("Parse busy: %v", e)
		}
		if e := p.Reset(); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("Reset busy: %v", e)
		}
		if e := p.Close(); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("Close busy: %v", e)
		}
		break
	}
	if fl.cur().count("query_new") != 1 || fl.cur().count("qcursor_delete") != 2 {
		t.Fatal("cache/early cleanup")
	}
	if _, err := collectQuery(tree, context.Background(), q, &QueryOptions{StartByte: 4, EndByte: 7, MatchLimit: 12}); err != nil {
		t.Fatal(err)
	}
	if fl.cur().qs().options != [3]int32{4, 7, 12} {
		t.Fatal("options")
	}
	if err := p.Reset(); err != nil {
		t.Fatal(err)
	}
	next, err := p.Parse(context.Background(), []byte("new"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err := collectQuery(next, context.Background(), q, nil); err != nil {
		t.Fatal(err)
	}
	if fl.cur().count("query_new") != 1 {
		t.Fatal("cache survived reset")
	}
	if _, err := collectQuery(tree, context.Background(), q, nil); !errors.Is(err, ErrTreeInvalidated) {
		t.Fatal(err)
	}
	if got[0].Captures[0].Node.Text() != "one" {
		t.Fatal("snapshot lost after reset")
	}
}

func TestQueryPredicates(t *testing.T) {
	for _, tc := range []struct {
		op, value string
		want      []string
	}{
		{"eq?", "one", []string{"one", "one"}}, {"not-eq?", "one", []string{"two"}},
		{"match?", "^o", []string{"one", "one"}}, {"not-match?", "^o", []string{"two"}},
		{"any-of?", "one", []string{"one", "one"}}, {"not-any-of?", "one", []string{"two"}},
	} {
		t.Run(tc.op, func(t *testing.T) {
			_, _, tree, q := queryFixture(t, func(f *fakeModule) {
				f.qs().meta = fakeQueryMeta([]string{tc.op, tc.value}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {0, 0}})
			})
			matches, err := collectQuery(tree, context.Background(), q, nil)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range matches {
				got = append(got, m.Captures[0].Node.Text())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%v want %v", got, tc.want)
			}
		})
	}
	t.Run("capture equality", func(t *testing.T) {
		_, _, tree, q := queryFixture(t, func(f *fakeModule) {
			f.qs().meta = fakeQueryMeta([]string{"eq?"}, [][2]uint32{{2, 0}, {1, 0}, {1, 1}, {0, 0}})
			f.qs().nodes = [][]uint32{{1, 3}, {1, 2}}
		})
		got, err := collectQuery(tree, context.Background(), q, nil)
		if err != nil || len(got) != 1 || len(got[0].Captures) != 2 {
			t.Fatalf("%v %v", got, err)
		}
	})
	t.Run("properties", func(t *testing.T) {
		_, _, _, q := queryFixture(t, func(f *fakeModule) {
			f.qs().meta = fakeQueryMeta([]string{"set!", "injection.language", "sql", "is?", "local"}, [][2]uint32{{2, 0}, {2, 1}, {2, 2}, {0, 0}, {2, 3}, {2, 4}, {0, 0}})
		})
		props := q.Properties(0)
		if props["injection.language"] != "sql" || props["is?"] != "local" {
			t.Fatal(props)
		}
		props["injection.language"] = "bad"
		if q.Properties(0)["injection.language"] != "sql" || q.Properties(-1) != nil {
			t.Fatal("property alias")
		}
	})
	for _, tc := range []struct {
		name     string
		literals []string
		steps    [][2]uint32
	}{
		{"regex", []string{"match?", "["}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {0, 0}}},
		{"arity", []string{"eq?"}, [][2]uint32{{2, 0}, {1, 0}, {0, 0}}},
		{"any-of capture", []string{"any-of?"}, [][2]uint32{{2, 0}, {1, 0}, {1, 1}, {0, 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.qs().meta = fakeQueryMeta(tc.literals, tc.steps) })
			_, err := NewQuery(fl.lang, "query")
			var qe *QueryError
			if !errors.As(err, &qe) || qe.Type != QueryErrorPredicate || qe.Offset != 0 {
				t.Fatal(err)
			}
		})
	}
}

func TestQueryOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*fakeModule)
		want  error
		reset bool
	}{
		{"match limit", func(f *fakeModule) { f.qs().exceeded = 1 }, ErrQueryMatchLimit, false},
		{"unexplained halt", func(f *fakeModule) { f.qs().halted = 1 }, ErrEngineFault, true},
		{"symbol", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[32:], 99) } }, ErrEngineFault, true},
		{"range", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[36:], 99) } }, ErrEngineFault, true},
		{"node index", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[28:], 999) } }, ErrEngineFault, true},
		{"capture index", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[24:], 999) } }, ErrEngineFault, true},
		{"header", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { copy(b, "bad!") } }, ErrEngineFault, true},
		{"pattern", func(f *fakeModule) { f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 999) } }, ErrEngineFault, true},
		{"size", func(f *fakeModule) {
			f.qs().corrupt = func(b []byte) { binary.LittleEndian.PutUint32(b[12:], 0xffffffff) }
		}, ErrEngineFault, true},
		{"trap", func(f *fakeModule) {
			f.onCall = func(name string) {
				if name == "qcursor_next" {
					panic("trap")
				}
			}
		}, ErrEngineFault, true},
		{"memory refused", func(f *fakeModule) {
			f.onCall = func(name string) {
				if name == "qcursor_next" {
					f.refused = true
				}
			}
		}, ErrResourceLimit, true},
		{"memory trap", func(f *fakeModule) {
			f.onCall = func(name string) {
				if name == "qcursor_next" {
					f.refused = true
					panic("oom")
				}
			}
		}, ErrResourceLimit, true},
		{"cleanup trap", func(f *fakeModule) {
			f.onCall = func(name string) {
				if name == "qcursor_delete" {
					panic("delete trap")
				}
			}
		}, ErrEngineFault, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fl, _, tree, q := queryFixture(t, nil)
			tc.setup(fl.cur())
			_, err := collectQuery(tree, context.Background(), q, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("%v want %v", err, tc.want)
			}
			var pe *ParseError
			if tc.reset && (!errors.As(err, &pe) || !pe.InstanceReset || tree.valid()) {
				t.Fatalf("missing reset: %v", err)
			}
			if !tc.reset && fl.cur().count("qcursor_delete") != 1 {
				t.Fatal("cursor leaked")
			}
		})
	}
}

func TestQueryCancellationThenParse(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[deadline], func(t *testing.T) {
			fl, p, tree, q := queryFixture(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			want := error(ErrCancelled)
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
				want = ErrTimeout
			}
			defer cancel()
			f := fl.cur()
			f.onCall = func(name string) {
				if name == "qcursor_next" {
					if !deadline {
						cancel()
					}
					for f.h.Stop.Load() == 0 {
						runtime.Gosched()
					}
					f.qs().halted = 1
				}
			}
			_, err := collectQuery(tree, ctx, q, nil)
			if !errors.Is(err, want) || !tree.valid() || f.count("qcursor_delete") != 1 {
				t.Fatalf("%v valid=%v", err, tree.valid())
			}
			f.onCall = nil
			next, err := p.Parse(context.Background(), []byte("different"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			snap, err := next.Snapshot(context.Background())
			if err != nil || snap.Source() != "different" {
				t.Fatal(err)
			}
		})
	}
}

func TestQueryInputErrors(t *testing.T) {
	_, _, tree, q := queryFixture(t, nil)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		q    *Query
		opts *QueryOptions
		want error
	}{
		{"nil context", nil, q, nil, ErrInvalidInput}, {"done context", cancelled, q, nil, ErrCancelled},
		{"nil query", context.Background(), nil, nil, ErrInvalidInput},
		{"foreign language", context.Background(), &Query{lang: newFakeLang(nil).lang}, nil, ErrInvalidInput},
		{"reverse range", context.Background(), q, &QueryOptions{StartByte: 8, EndByte: 2}, ErrInvalidInput},
		{"range end", context.Background(), q, &QueryOptions{EndByte: 99}, ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collectQuery(tree, tc.ctx, tc.q, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
	if err := tree.Edit(InputEdit{}); err != nil {
		t.Fatal(err)
	}
	if _, err := collectQuery(tree, context.Background(), q, nil); !errors.Is(err, ErrTreeEdited) {
		t.Fatal(err)
	}
	if _, err := NewQuery(nil, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.qs().errorType = 2; f.qs().errorOffset = 3 })
	_, err := NewQuery(fl.lang, "bad query")
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Type != QueryErrorNodeType || qe.Offset != 3 {
		t.Fatal(err)
	}
}

// Embedding only the v1 interface proves that ABI 1 does not require query exports.
type legacyQueryModule struct{ engine.Module }

func TestQueryABI1Compatibility(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.abi = 1 })
	fl.lang.g.ABI = 1
	create := fl.lang.g.New
	fl.lang.g.New = func(h *engine.Host) engine.Module { return legacyQueryModule{create(h)} }
	p, err := NewParser(fl.lang)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	tree, err := p.Parse(context.Background(), []byte("old grammar"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	if _, err := tree.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewQuery(fl.lang, "query"); !errors.Is(err, ErrQueryUnsupported) {
		t.Fatal(err)
	}
	if _, err := collectQuery(tree, context.Background(), &Query{lang: fl.lang}, nil); !errors.Is(err, ErrQueryUnsupported) {
		t.Fatal(err)
	}
	fl.lang.g.ABI = 2
	if _, err := NewParser(fl.lang); !errors.Is(err, ErrIncompatibleLanguage) {
		t.Fatal(err)
	}
}

func FuzzNewQuery(f *testing.F) {
	for _, s := range []string{"", "(word) @word", "((word) @a (#eq? @a \"x\"))", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, source string) {
		fl := newFakeLang(func(_ *fakeLang, m *fakeModule) {
			m.qs().errorType = 1
			if len(source) > 0 {
				m.qs().errorOffset = int32(len(source) - 1)
			}
		})
		q, err := NewQuery(fl.lang, source)
		var qe *QueryError
		if q != nil || !errors.As(err, &qe) || uint64(qe.Offset) > uint64(len(source)) {
			t.Fatalf("%v %v", q, err)
		}
	})
}
func TestQueryABITraps(t *testing.T) {
	for _, name := range []string{"query_new", "query_meta", "query_error_offset", "query_error_type", "query_delete"} {
		t.Run("compile/"+name, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
				if strings.HasPrefix(name, "query_error") {
					f.qs().errorType = 1
				}
				f.onCall = func(call string) {
					if call == name {
						panic("scripted compile trap")
					}
				}
			})
			_, err := NewQuery(fl.lang, "query")
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Outcome != EngineFault || !pe.InstanceReset {
				t.Fatalf("%v", err)
			}
		})
	}
	for _, name := range []string{"query_new", "qcursor_new", "qcursor_exec", "qcursor_next", "qcursor_halted", "qcursor_exceeded", "qcursor_buf", "qcursor_delete"} {
		t.Run("iterator/"+name, func(t *testing.T) {
			fl, _, tree, q := queryFixture(t, nil)
			fl.cur().onCall = func(call string) {
				if call == name {
					panic("scripted iterator trap")
				}
			}
			_, err := collectQuery(tree, context.Background(), q, nil)
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Outcome != EngineFault || !pe.InstanceReset {
				t.Fatal(err)
			}
		})
	}
	t.Run("refused compiling", func(t *testing.T) {
		fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
			f.onCall = func(call string) {
				if call == "query_new" {
					f.refused = true
				}
			}
		})
		_, err := NewQuery(fl.lang, "query")
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Outcome != ResourceLimit || !pe.InstanceReset {
			t.Fatal(err)
		}
	})
}

func TestQueryMalformedMetadata(t *testing.T) {
	cases := map[string]func([]byte){
		"magic":           func(b []byte) { copy(b, "bad!") },
		"format":          func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 99) },
		"size":            func(b []byte) { binary.LittleEndian.PutUint32(b[8:], 0xffffffff) },
		"counts":          func(b []byte) { binary.LittleEndian.PutUint32(b[12:], 0xffffffff) },
		"string length":   func(b []byte) { binary.LittleEndian.PutUint32(b[24:], 0xffffffff) },
		"pattern offset":  func(b []byte) { binary.LittleEndian.PutUint32(b[len(b)-8:], 99) },
		"predicate count": func(b []byte) { binary.LittleEndian.PutUint32(b[len(b)-4:], 99) },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { b := fakeQueryMeta(nil, nil); corrupt(b); f.qs().meta = b })
			_, err := NewQuery(fl.lang, "query")
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Outcome != EngineFault || !pe.InstanceReset {
				t.Fatal(err)
			}
		})
	}
}

func TestQuerySnapshotLimit(t *testing.T) {
	fl := newFakeLang(nil)
	q, err := NewQuery(fl.lang, "query")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(fl.lang, WithMaxSnapshotNodes(1))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	tree, err := p.Parse(context.Background(), []byte("one two"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	_, err = collectQuery(tree, context.Background(), q, nil)
	if !errors.Is(err, ErrResourceLimit) || !tree.valid() || fl.cur().count("qcursor_new") != 0 {
		t.Fatal(err)
	}
}

func TestQueryTreeClosedDuringIteration(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	closed := false
	for _, err := range tree.Matches(context.Background(), q, nil) {
		if err != nil {
			if !errors.Is(err, ErrTreeInvalidated) {
				t.Fatal(err)
			}
			closed = true
			break
		}
		tree.Close()
	}
	if !closed || fl.cur().count("qcursor_delete") != 1 {
		t.Fatal("missing invalidation or cleanup")
	}
	next, err := p.Parse(context.Background(), []byte("safe"), nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
func TestQueryABI2RequiresExports(t *testing.T) {
	fl := newFakeLang(nil)
	create := fl.lang.g.New
	fl.lang.g.New = func(h *engine.Host) engine.Module { return legacyQueryModule{create(h)} }
	_, err := NewParser(fl.lang)
	if !errors.Is(err, ErrIncompatibleLanguage) || !fl.cur().released.Load() {
		t.Fatal(err)
	}
}

func TestQueryPredicateCaptureLists(t *testing.T) {
	s := &Snapshot{src: "one two one", n: 3, startByte: []uint32{0, 4, 8}, endByte: []uint32{3, 7, 11}}
	cap := func(id uint32, i int) QueryCapture { return QueryCapture{Index: id, Node: s.Node(i)} }
	for _, tc := range []struct {
		name, op string
		captures []QueryCapture
		want     bool
	}{
		{"all equal", "eq?", []QueryCapture{cap(0, 0), cap(0, 2), cap(1, 2), cap(1, 0)}, true},
		{"unequal count", "eq?", []QueryCapture{cap(0, 0), cap(0, 2), cap(1, 0)}, false},
		{"empty optional", "eq?", nil, true},
		{"all different", "not-eq?", []QueryCapture{cap(0, 0), cap(0, 2), cap(1, 1), cap(1, 1)}, true},
		{"negate each", "not-eq?", []QueryCapture{cap(0, 0), cap(0, 2), cap(1, 1), cap(1, 0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &Query{source: "query"}
			if err := q.readMeta(fakeQueryMeta([]string{tc.op}, [][2]uint32{{2, 0}, {1, 0}, {1, 1}, {0, 0}})); err != nil {
				t.Fatal(err)
			}
			if got := q.accepts(QueryMatch{Captures: tc.captures}); got != tc.want {
				t.Fatalf("%v want %v", got, tc.want)
			}
		})
	}
}

func TestQueryIteratorPanicCleanup(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("missing consumer panic")
			}
		}()
		for range tree.Matches(context.Background(), q, nil) {
			panic("consumer")
		}
	}()
	if fl.cur().count("qcursor_delete") != 1 {
		t.Fatal("cursor leaked after panic")
	}
	if err := p.Reset(); err != nil {
		t.Fatal(err)
	}
}
func TestQueryDefaultRangePreservesZero(t *testing.T) {
	fl, _, tree, q := queryFixture(t, nil)
	if _, err := collectQuery(tree, context.Background(), q, nil); err != nil {
		t.Fatal(err)
	}
	if fl.cur().qs().options[1] != 0 {
		t.Fatal("default end was changed before C execution", fl.cur().qs().options)
	}
}

func TestQueryCLIPredicateValidation(t *testing.T) {
	for _, tc := range []struct {
		op   string
		args []string
		ok   bool
	}{
		{"any-of?", []string{"@word"}, true}, {"not-any-of?", []string{"@word"}, true},
		{"any-of?", nil, false}, {"any-of?", []string{"@word", "@other"}, false},
		{"set!", []string{"@word", "k", "v"}, true}, {"set!", []string{"k", "@word", "v"}, true},
		{"set!", []string{"k", "v", "@word"}, true}, {"set!", []string{"k", "v", "w"}, false},
		{"set!", nil, false}, {"set!", []string{"@word"}, false}, {"set!", []string{"@word", "@other", "k"}, false},
		{"is?", nil, false}, {"is-not?", nil, false}, {"is?", []string{"@word", "k"}, true},
		{"any-eq?", []string{"@word", "k"}, true}, {"any-not-eq?", []string{"@word", "@other"}, true},
		{"any-eq?", []string{"k", "@word"}, false}, {"any-eq?", []string{"@word"}, false},
		{"any-match?", []string{"@word", "@word"}, false}, {"any-not-match?", []string{"@word", "("}, false},
		{"any-match?", []string{"@word", "^k"}, true}, {"any-not-any-of?", []string{"@word", "k"}, true},
	} {
		t.Run(tc.op+strings.Join(tc.args, "/"), func(t *testing.T) {
			literals := []string{tc.op}
			steps := [][2]uint32{{2, 0}}
			for _, a := range tc.args {
				switch a {
				case "@word":
					steps = append(steps, [2]uint32{1, 0})
				case "@other":
					steps = append(steps, [2]uint32{1, 1})
				default:
					literals = append(literals, a)
					steps = append(steps, [2]uint32{2, uint32(len(literals) - 1)})
				}
			}
			steps = append(steps, [2]uint32{0, 0})
			q := &Query{source: strings.Repeat(" ", 100)}
			err := q.readMeta(fakeQueryMeta(literals, steps))
			if (err == nil) != tc.ok {
				t.Fatalf("%v want accepted=%v", err, tc.ok)
			}
			if err == nil && tc.op == "set!" && q.Properties(0)["k"] != "v" {
				t.Fatal(q.Properties(0))
			}
			if err == nil && strings.HasPrefix(tc.op, "any-") && tc.op != "any-of?" && len(q.patterns[0].predicates) != 0 {
				t.Fatal("CLI any-* operators must not filter")
			}
		})
	}
}

func TestQueryCleanupQueue(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	defer runtime.KeepAlive(q)
	if _, err := collectQuery(tree, context.Background(), q, nil); err != nil {
		t.Fatal(err)
	}
	f := fl.cur()
	ephemeral := func() weak.Pointer[Query] {
		temp, err := NewQuery(fl.lang, "temporary")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := collectQuery(tree, context.Background(), temp, nil); err != nil {
			t.Fatal(err)
		}
		return weak.Make(temp)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		p.c.mu.Lock()
		pending := len(p.c.leakedQueries)
		p.c.mu.Unlock()
		if pending > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("query cleanup was not queued within 5s")
		}
		time.Sleep(time.Millisecond)
	}
	if ephemeral.Value() != nil {
		t.Fatal("cache kept the Query reachable")
	}
	if f.count("query_delete") != 0 {
		t.Fatal("GC cleanup called into the engine")
	}
	next, err := p.Parse(context.Background(), []byte("drain"), nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
	if f.count("query_delete") != 1 || len(p.c.inst.queries) != 1 {
		t.Fatal("owner did not delete only the retired query")
	}
	if _, err := collectQuery(tree, context.Background(), q, nil); err != nil {
		t.Fatal(err)
	}
	if f.count("query_new") != 2 {
		t.Fatal("live query cache entry was lost")
	}
}

func TestQueryCleanupGenerationAndTrap(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(fmt.Sprint("stale=", stale), func(t *testing.T) {
			fl, p, tree, q := queryFixture(t, nil)
			if _, err := collectQuery(tree, context.Background(), q, nil); err != nil {
				t.Fatal(err)
			}
			arg := queryCleanup{owner: weak.Make(p.c), queryRetirement: queryRetirement{key: weak.Make(q), instance: weak.Make(p.c.inst)}}
			if stale {
				if err := p.Reset(); err != nil {
					t.Fatal(err)
				}
			} else {
				fl.cur().onCall = func(name string) {
					if name == "query_delete" {
						panic("cleanup trap")
					}
				}
			}
			retireQuery(arg)
			next, err := p.Parse(context.Background(), []byte("next"), nil)
			if stale {
				if err != nil || fl.cur().count("query_delete") != 0 {
					t.Fatal(err)
				}
				next.Close()
			} else {
				var pe *ParseError
				if !errors.As(err, &pe) || pe.Outcome != EngineFault || !pe.InstanceReset || p.c.busy.Load() {
					t.Fatal(err)
				}
			}
			runtime.KeepAlive(q)
		})
	}
}
