package consumer_test

import (
	"os"
	"testing"

	ts "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/cpp"
)

func TestCPP(t *testing.T) {
	// Pinned tree-sitter-cpp corpus, expressions.txt case 25, MIT. Keep both line feeds.
	const input = "\nwhile ((a = b)) {}\n"
	const pattern = "(comment) @comment\n(parenthesized_expression (assignment_expression left: (identifier) @left right: (identifier) @right))"
	lang := cpp.Language()
	p := parser(t, lang)
	tree, s := parse(t, p, input, nil)

	checkSymbol(t, lang, s.Root(), "translation_unit", 225)
	parent := onlyNode(t, s, "parenthesized_expression")
	assignment := onlyNode(t, s, "assignment_expression")
	left, ok := assignment.NamedChild(0)
	if !ok || left.Kind() != "identifier" || left.StartByte() != 9 || left.EndByte() != 10 {
		t.Fatalf("assignment left = %v, want identifier at [9,10)", left)
	}
	checkField(t, assignment, "left", left)

	switch abi := os.Getenv("TSG_CPP_ABI"); abi {
	case "2":
		checkField(t, parent, "left", ts.Node{})
	case "3":
		checkField(t, parent, "left", left)
	default:
		t.Fatalf("unexpected C++ ABI %q", abi)
	}
	checkQuery(t, p, tree, lang, input, pattern, []string{"comment", "left", "right"},
		[]capture{{"left", "identifier", 9, 10}, {"right", "identifier", 13, 14}})
}
