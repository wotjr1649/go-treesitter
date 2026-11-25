package consumer_test

import (
	"slices"
	"testing"

	ts "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)

func TestGo(t *testing.T) {
	const input = "package p\nfunc f() { g(1) }\n"
	const pattern = "(comment) @comment\n(call_expression function: (identifier) @callee arguments: (argument_list) @args)"
	lang := golang.Language()
	p := parser(t, lang)
	tree, s := parse(t, p, input, nil)

	call := onlyNode(t, s, "call_expression")
	args := onlyNode(t, s, "argument_list")
	checkField(t, call, "arguments", args)
	checkField(t, call, "consumer_compat_unknown_field", ts.Node{})
	// The tagged metadata maps the first named raw argument_list (173) to public ID 175.
	checkSymbol(t, lang, args, "argument_list", 175)
	hits := slices.Collect(s.Index().OfSymbol(175))
	if len(hits) != 1 || hits[0] != args {
		t.Fatalf("Index.OfSymbol(175) = %v, want [%v]", hits, args)
	}
	checkQuery(t, p, tree, lang, input, pattern, []string{"comment", "callee", "args"},
		[]capture{{"callee", "identifier", 21, 22}, {"args", "argument_list", 22, 25}})
}
