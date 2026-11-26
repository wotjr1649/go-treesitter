package treesitter

import (
	"context"
	"errors"
	"iter"
	"math"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

func TestQueryCompileBudgets(t *testing.T) {
	ctx := context.Background()
	const source = "(word) @word"
	meta := fakeQueryMeta(nil, nil)
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.qs().meta = meta })
	before := len(fl.mods)
	check := func(want error, opts ...QueryCompileOption) {
		t.Helper()
		q, err := NewQueryWithOptions(ctx, fl.lang, source, opts...)
		if !errors.Is(err, want) || (q == nil) != (want != nil) {
			t.Fatalf("query=%v error=%v, want %v", q, err, want)
		}
	}
	check(ErrResourceLimit, WithMaxQuerySourceBytes(int64(len(source)-1)))
	for _, opt := range []QueryCompileOption{
		nil, WithMaxQuerySourceBytes(-1), WithMaxQuerySourceBytes(hardMaxInput + 1),
		WithMaxQueryMemoryBytes(-1), WithMaxQueryMemoryBytes(engine.MinLimit - 1),
		WithMaxQueryMemoryBytes(hardMaxMemory + 1),
		WithMaxQueryMetadataBytes(-1), WithMaxQueryMetadataBytes(math.MaxUint32 + 1),
		WithMaxQueryRegexUnits(-1),
	} {
		check(ErrInvalidInput, opt)
	}
	if len(fl.mods) != before {
		t.Fatal("invalid options or excess source caused engine creation")
	}
	check(nil, WithMaxQuerySourceBytes(int64(len(source))))
	check(nil, WithMaxQuerySourceBytes(1), WithMaxQuerySourceBytes(0))
	check(nil, WithMaxQueryMetadataBytes(int64(len(meta))))
	check(ErrResourceLimit, WithMaxQueryMetadataBytes(int64(len(meta)-1)))
	if f := fl.cur(); f.count("query_delete") != 1 || f.count("free") != 2 || !f.released.Load() {
		t.Fatal("scratch query and metadata must be freed on budget exhaustion")
	}

	const needMemory = engine.MinLimit + 65536
	fl = newFakeLang(func(_ *fakeLang, f *fakeModule) {
		f.onCall = func(name string) {
			if name == "query_new" && f.limit != 0 && f.limit < needMemory {
				f.refused = true
			}
		}
	})
	check(nil, WithMaxQueryMemoryBytes(needMemory))
	if fl.cur().limit != needMemory {
		t.Fatal("scratch limit was not forwarded to the engine")
	}
	_, err := NewQueryWithOptions(ctx, fl.lang, source, WithMaxQueryMemoryBytes(needMemory-1))
	var pe *ParseError
	if !errors.Is(err, ErrResourceLimit) || !errors.As(err, &pe) || !pe.InstanceReset {
		t.Fatalf("refused memory.grow: %v", err)
	}
}

func TestQueryCompileBudgetPropertyCopies(t *testing.T) {
	const literal = "a long property value"
	const copies = 128
	steps := [][2]uint32{{2, 0}, {1, 0}}
	for range copies {
		steps = append(steps, [2]uint32{2, 1})
	}
	steps = append(steps, [2]uint32{})
	meta := fakeQueryMeta([]string{"opaque!", literal}, steps)
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.qs().meta = meta })
	want := "@word" + strings.Repeat(" "+literal, copies)
	if len(meta) >= len(want) {
		t.Fatal("fixture must fit the wire cap while testing derived copies")
	}
	ctx := context.Background()
	q, err := NewQueryWithOptions(ctx, fl.lang, "query", WithMaxQueryMetadataBytes(int64(len(want))))
	if err != nil || q.Properties(0)["opaque!"] != want {
		t.Fatalf("exact property limit: %v", err)
	}
	if _, err := NewQueryWithOptions(ctx, fl.lang, "query", WithMaxQueryMetadataBytes(int64(len(want)-1))); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("property limit exceeded: %v", err)
	}
}

func TestQueryCompileBudgetRegexUnits(t *testing.T) {
	const pattern = "(?i:[a-z]){20,40}"
	ctx := context.Background()
	meter := newQueryMeter(ctx, math.MaxInt64, "query regex")
	if _, err := compileQueryRegexWithMeter(pattern, &meter); err != nil {
		t.Fatal(err)
	}
	// Both the filtering and opaque predicates compile and count the same regex.
	meta := fakeQueryMeta([]string{"match?", "any-match?", pattern},
		[][2]uint32{{2, 0}, {1, 0}, {2, 2}, {}, {2, 1}, {1, 0}, {2, 2}, {}})
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { f.qs().meta = meta })
	need := 2 * (math.MaxInt64 - meter.remaining)
	q, err := NewQueryWithOptions(ctx, fl.lang, "query", WithMaxQueryRegexUnits(need))
	if err != nil || q.patterns[0].predicates[0].re.re != nil {
		t.Fatalf("exact regex budget with VM-only compilation: %v", err)
	}
	for _, limit := range []int64{1, need - 1} {
		if _, err := NewQueryWithOptions(ctx, fl.lang, "query", WithMaxQueryRegexUnits(limit)); !errors.Is(err, ErrResourceLimit) {
			t.Fatalf("regex limit %d exceeded: %v", limit, err)
		}
	}
	ast := &syntax.Regexp{Op: syntax.OpLiteral, Rune: []rune{'a'}}
	for range 8 {
		ast = &syntax.Regexp{Op: syntax.OpRepeat, Min: 0, Max: 1000, Sub: []*syntax.Regexp{ast}}
	}
	if got := expandedRegexUnits(ast, math.MaxInt64, &meter); got >= 0 {
		t.Fatalf("overflowing expansion fits: %d", got)
	}
}

func TestQueryCompileBudgetOutcomes(t *testing.T) {
	ctx := context.Background()
	for _, engineError := range []bool{false, true} {
		fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
			if engineError {
				f.qs().errorType, f.qs().errorOffset = 2, 3
			} else {
				f.qs().meta = fakeQueryMeta([]string{"match?", "["}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {}})
			}
		})
		_, legacy := NewQuery(fl.lang, "query")
		_, err := NewQueryWithOptions(ctx, fl.lang, "query")
		want, ok := legacy.(*QueryError)
		var qe *QueryError
		if !ok || !errors.Is(err, ErrInvalidInput) || !errors.As(err, &qe) || *qe != *want {
			t.Fatalf("legacy=%v, opt-in=%v (%#v)", legacy, err, qe)
		}
	}
	for _, checkpoint := range []string{"initialize", "query_new", "query_meta"} {
		t.Run(checkpoint, func(t *testing.T) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
				f.onCall = func(name string) {
					if name == checkpoint {
						cancel()
					}
				}
			})
			_, err := NewQueryWithOptions(ctx, fl.lang, "query")
			f := fl.cur()
			if !errors.Is(err, ErrCancelled) || !errors.Is(err, context.Canceled) || !f.released.Load() {
				t.Fatalf("compilation cancellation: %v", err)
			}
			if f.count("query_delete") != f.count("query_new") || (checkpoint == "initialize" && f.count("malloc") != 0) {
				t.Fatal("compilation cancellation left transient engine allocations")
			}
		})
	}
}

func TestQueryMatchBudgets(t *testing.T) {
	_, _, tree, q := queryFixture(t, nil)
	ctx := context.Background()
	for name, opt := range map[string]func(int64) QueryMatchOption{
		"captures": WithMaxQueryCaptures, "results": WithMaxQueryResults, "candidate work": WithMaxQueryPredicateWork,
	} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				limit int64
				count int
				err   error
			}{{0, 3, nil}, {3, 3, nil}, {2, 2, ErrResourceLimit}, {-1, 0, ErrInvalidInput}} {
				seq := tree.MatchesWithOptions(ctx, q, nil, opt(tc.limit))
				for range 2 {
					requireQueryIteration(t, seq, tc.count, tc.err)
				}
			}
		})
	}
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, nil), 0, ErrInvalidInput)
	requireQueryIteration(t, tree.MatchesWithOptions(nil, q, nil), 0, ErrInvalidInput)
	options := QueryOptions{StartByte: 4, EndByte: 7}
	budgets := []QueryMatchOption{WithMaxQueryResults(1)}
	seq := tree.MatchesWithOptions(ctx, q, &options, budgets...)
	options.EndByte, budgets[0] = 0, WithMaxQueryResults(-1)
	requireQueryIteration(t, seq, 1, nil)
	for m, err := range tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryResults(1)) {
		if err != nil || m.Captures[0].Node.Text() != "one" {
			t.Fatalf("early break: %v %v", m, err)
		}
		break
	}
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryResults(1), WithMaxQueryResults(0)), 3, nil)
}

func requireQueryIteration(t *testing.T, seq iter.Seq2[QueryMatch, error], count int, want error) {
	t.Helper()
	var terminal error
	var matches []QueryMatch
	for m, err := range seq {
		if terminal != nil {
			t.Fatal("yield after terminal error")
		}
		if err != nil {
			if m.Pattern != 0 || m.Captures != nil {
				t.Fatal("terminal error carried a match")
			}
			terminal = err
			continue
		}
		matches = append(matches, m)
	}
	if len(matches) != count || !errors.Is(terminal, want) {
		t.Fatalf("got %d matches and %v, want %d and %v", len(matches), terminal, count, want)
	}
	var pe *ParseError
	if errors.As(terminal, &pe) && pe.InstanceReset {
		t.Fatalf("clean stop reset the instance: %v", terminal)
	}
	for _, m := range matches {
		for _, c := range m.Captures {
			if c.Node.Text() == "" {
				t.Fatal("earlier match lost its capture after iteration")
			}
		}
	}
}

func TestQueryMatchBudgetFilteredAndZeroCaptures(t *testing.T) {
	fl, _, tree, q := queryFixture(t, func(f *fakeModule) {
		f.qs().meta = fakeQueryMeta([]string{"eq?", "two"}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {}})
	})
	ctx := context.Background()
	budget := func(n int64) iter.Seq2[QueryMatch, error] {
		return tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryCaptures(n), WithMaxQueryResults(1))
	}
	requireQueryIteration(t, budget(3), 1, nil)
	requireQueryIteration(t, budget(2), 1, ErrResourceLimit)
	// Each candidate costs four dispatch/scan/occurrence units, a length
	// check, and three bytes of comparison, including rejected candidates.
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryPredicateWork(24)), 1, nil)
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryPredicateWork(23)), 1, ErrResourceLimit)
	fl.cur().qs().nodes = [][]uint32{{}, {}}
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryCaptures(1), WithMaxQueryResults(2)), 2, nil)
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil, WithMaxQueryCaptures(1), WithMaxQueryResults(1)), 1, ErrResourceLimit)
}

type queryCheckpointContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *queryCheckpointContext) Err() error {
	if c.checks > 0 {
		c.checks--
		if c.checks == 0 {
			c.cancel()
		}
	}
	return c.Context.Err()
}

func TestQueryMatchBudgetPredicateCancellation(t *testing.T) {
	for _, tc := range []struct{ op, arg string }{
		{"not-eq?", strings.Repeat("a", 2047) + "b"},
		{"not-match?", "a+z$"},
		{"not-any-of?", strings.Repeat("a", 2047) + "b"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			fl, p, _, q := queryFixture(t, func(f *fakeModule) {
				f.qs().meta = fakeQueryMeta([]string{tc.op, tc.arg}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {}})
			})
			ctx := context.Background()
			tree, err := p.Parse(ctx, []byte(strings.Repeat("a", 2048)), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Close()
			base, cancel := context.WithCancel(ctx)
			defer cancel()
			checkpoint := &queryCheckpointContext{Context: base, cancel: cancel}
			f := fl.cur()
			f.onCall = func(name string) {
				if name == "qcursor_next" {
					checkpoint.checks = 14
				}
			}
			requireQueryIteration(t, tree.MatchesWithOptions(checkpoint, q, nil), 0, context.Canceled)
			if checkpoint.Err() != context.Canceled || f.count("qcursor_next") != 1 {
				t.Fatal("context was not interrupted during first candidate evaluation")
			}
			f.onCall = nil
			requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil), 1, nil)
			if fl.cur() != f || f.count("query_new") != 1 {
				t.Fatal("predicate cancellation discarded the parser or query")
			}
		})
	}
}

func TestQueryMatchBudgetColdCompileCancellation(t *testing.T) {
	fl, p, tree, q := queryFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := fl.cur()
	f.onCall = func(name string) {
		if name == "query_new" {
			cancel()
		}
	}
	requireQueryIteration(t, tree.MatchesWithOptions(ctx, q, nil), 0, context.Canceled)
	if f.count("query_delete") != 1 || len(p.c.inst.queries) != 0 {
		t.Fatal("cancelled cold compilation must free the unpublished query")
	}
	f.onCall = nil
	requireQueryIteration(t, tree.MatchesWithOptions(context.Background(), q, nil), 3, nil)
	if f.count("query_new") != 2 || fl.cur() != f {
		t.Fatal("retry must compile on the same live instance")
	}
}

func TestQueryRegexExpansionEstimate(t *testing.T) {
	meter := newQueryMeter(context.Background(), math.MaxInt64, "query regex")
	// The parser can represent an empty class as OpCharClass rather than OpNoMatch.
	ops := []syntax.Op{syntax.OpNoMatch}
	if got := expandedRegexUnits(&syntax.Regexp{Op: syntax.OpNoMatch}, math.MaxInt64, &meter); got < 3 {
		t.Fatalf("no-match units = %d", got)
	}
	for _, pattern := range []string{"[^\\x{0}-\\x{10FFFF}]", "", "ab", "[a-z]", ".", "(?s:.)", "(?m:^$)", "\\A\\z\\b\\B", "(a)*b+c?", "a|bc", "(a[cd-z]){2,30}", "(?:a{2,3}b){8,}", "a{0}"} {
		ast, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		limit := expandedRegexUnits(ast, math.MaxInt64, &meter)
		var visit func(*syntax.Regexp)
		visit = func(r *syntax.Regexp) {
			ops = append(ops, r.Op)
			for _, sub := range r.Sub {
				visit(sub)
			}
		}
		visit(ast)
		var treeUnits func(*syntax.Regexp) int64
		treeUnits = func(r *syntax.Regexp) int64 {
			n := int64(1 + len(r.Rune))
			for _, sub := range r.Sub {
				n += treeUnits(sub)
			}
			return n
		}
		simple := ast.Simplify()
		prog, err := syntax.Compile(simple)
		if err != nil {
			t.Fatal(err)
		}
		actual := treeUnits(simple) + int64(len(prog.Inst))
		for _, inst := range prog.Inst {
			actual += int64(len(inst.Rune))
		}
		if actual > limit {
			t.Errorf("%s: actual %d > estimate %d", pattern, actual, limit)
		}
	}
	for op := syntax.OpNoMatch; op <= syntax.OpAlternate; op++ {
		if !slices.Contains(ops, op) {
			t.Errorf("estimator fixture did not exercise %v", op)
		}
	}
}
