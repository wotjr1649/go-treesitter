package consumer_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"

	ts "github.com/wotjr1649/go-treesitter"
)

var ctx = context.Background()

func parser(t *testing.T, lang *ts.Language) *ts.Parser {
	t.Helper()
	p, err := ts.NewParser(lang)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func parse(t *testing.T, p *ts.Parser, input string, old *ts.Tree) (*ts.Tree, *ts.Snapshot) {
	t.Helper()
	t.Logf("input %q sha256=%x", input, sha256.Sum256([]byte(input)))
	tree, err := p.Parse(ctx, []byte(input), old)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tree.Close() })
	if tree.Outcome() != ts.AcceptedClean {
		t.Fatalf("parse outcome = %v, want %v", tree.Outcome(), ts.AcceptedClean)
	}
	s, err := tree.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if root := s.Root(); s.Source() != input || root.IsZero() || root.HasError() || root.EndByte() != uint32(len(input)) {
		t.Fatalf("incomplete or erroneous snapshot root: %v", root)
	}
	return tree, s
}

type capture struct {
	name, kind string
	start, end uint32
}

func checkSymbol(t *testing.T, lang *ts.Language, n ts.Node, kind string, want ts.Symbol) {
	t.Helper()
	symbol, ok := lang.SymbolForName(kind, true)
	if !ok || symbol != want || lang.SymbolName(want) != kind || n.Symbol() != want || n.Kind() != kind {
		t.Fatalf("want %s symbol %d, lookup = %d (%t), symbol name = %q, node = %s (%d)",
			kind, want, symbol, ok, lang.SymbolName(want), n.Kind(), n.Symbol())
	}
}

func checkField(t *testing.T, parent ts.Node, name string, want ts.Node) {
	t.Helper()
	got, ok := parent.ChildByFieldName(name)
	if got != want || ok != !want.IsZero() {
		t.Fatalf("%s.%s = %v (%t), want %v", parent.Kind(), name, got, ok, want)
	}
	nodes := slices.Collect(parent.ChildrenByFieldName(name))
	if want.IsZero() {
		if len(nodes) != 0 {
			t.Fatalf("%s.%s iterates %v, want empty", parent.Kind(), name, nodes)
		}
		return
	}
	if !slices.Equal(nodes, []ts.Node{want}) {
		t.Fatalf("%s.%s iterates %v, want [%v]", parent.Kind(), name, nodes, want)
	}
}

func checkQuery(t *testing.T, p *ts.Parser, tree *ts.Tree, lang *ts.Language, src, pattern string, wantNames []string, want []capture) {
	t.Helper()
	// All supported rows use a comment as the first, unmatched pattern.
	const wantPattern = 1
	t.Logf("query %q sha256=%x want pattern=%d capture names=%v captures=%+v",
		pattern, sha256.Sum256([]byte(pattern)), wantPattern, wantNames, want)
	q, err := ts.NewQuery(lang, pattern)
	if err != nil {
		t.Fatal(err)
	}
	captureNames := q.CaptureNames()
	if !slices.Equal(captureNames, wantNames) {
		t.Fatalf("capture names = %v, want %v", captureNames, wantNames)
	}
	matches := 0
	for m, err := range tree.Matches(ctx, q, nil) {
		if err != nil {
			t.Fatal(err)
		}
		var got []capture
		for _, c := range m.Captures {
			if int(c.Index) >= len(captureNames) {
				t.Fatalf("capture index %d is outside %v", c.Index, captureNames)
			}
			got = append(got, capture{captureNames[c.Index], c.Node.Kind(), c.Node.StartByte(), c.Node.EndByte()})
		}
		if m.Pattern != wantPattern || !slices.Equal(got, want) {
			t.Fatalf("match = pattern %d, %+v, want pattern %d, %+v", m.Pattern, got, wantPattern, want)
		}
		matches++
	}
	if matches != 1 {
		t.Fatalf("match count = %d, want 1", matches)
	}

	const invalid = "(consumer_compat_unknown_node)"
	t.Logf("invalid query %q sha256=%x", invalid, sha256.Sum256([]byte(invalid)))
	var queryErr *ts.QueryError
	if q, err := ts.NewQuery(lang, invalid); q != nil || !errors.As(err, &queryErr) || queryErr.Type != ts.QueryErrorNodeType {
		t.Fatalf("invalid query returned %v, %v, want nil and QueryErrorNodeType", q, err)
	}
	broke := false
	for _, err := range tree.Matches(ctx, q, nil) {
		if err != nil {
			t.Fatal(err)
		}
		broke = true
		break
	}
	if !broke {
		t.Fatal("query had no match to break from")
	}
	parse(t, p, src, nil)
}

func onlyNode(t *testing.T, s *ts.Snapshot, kind string) ts.Node {
	t.Helper()
	nodes := slices.Collect(s.Index().OfType(kind))
	if len(nodes) != 1 {
		t.Fatalf("want one %s, found %d", kind, len(nodes))
	}
	return nodes[0]
}
