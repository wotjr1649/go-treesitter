package consumer_test

import (
	"crypto/sha256"
	"errors"
	"os"
	"slices"
	"testing"

	ts "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/json"
)

func TestJSON(t *testing.T) {
	const input = "{\"a\":1}"
	const pattern = "(comment) @comment\n(pair key: (string) @key value: (number) @value)"
	lang := json.Language()
	p := parser(t, lang)
	tree, s := parse(t, p, input, nil)

	checkSymbol(t, lang, s.Root(), "document", 15)
	pair := onlyNode(t, s, "pair")
	value := onlyNode(t, s, "number")
	checkField(t, pair, "value", value)

	switch abi := os.Getenv("TSG_JSON_ABI"); abi {
	case "1":
		t.Logf("ABI 1 query %q sha256=%x want nil query and ErrQueryUnsupported", pattern, sha256.Sum256([]byte(pattern)))
		if q, err := ts.NewQuery(lang, pattern); q != nil || !errors.Is(err, ts.ErrQueryUnsupported) {
			t.Fatalf("ABI 1 query = %v, %v, want nil and ErrQueryUnsupported", q, err)
		}
	case "2":
		checkQuery(t, p, tree, lang, input, pattern, []string{"comment", "key", "value"},
			[]capture{{"key", "string", 1, 4}, {"value", "number", 5, 6}})
	default:
		t.Fatalf("unexpected JSON ABI %q", abi)
	}

	const updated = "{\"a\":true}"
	if err := tree.Edit(ts.EditFor([]byte(input), []byte(updated), 5, 6, 9)); err != nil {
		t.Fatal(err)
	}
	next, edited := parse(t, p, updated, tree)
	checkField(t, onlyNode(t, edited, "pair"), "value", onlyNode(t, edited, "true"))
	want := []ts.Range{{StartByte: 5, EndByte: 9, StartPoint: ts.Point{Column: 5}, EndPoint: ts.Point{Column: 9}}}
	t.Logf("want changed ranges %+v", want)
	if got, err := next.ChangedRanges(tree); err != nil || !slices.Equal(got, want) {
		t.Fatalf("ChangedRanges = %+v, %v, want %+v", got, err, want)
	}
}
