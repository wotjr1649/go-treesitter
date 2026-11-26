//go:build !no_go

package conformance_test

import (
	"context"
	"fmt"
	"log"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)

// These examples live here rather than in the root package because a grammar module requires the root module:
// package treesitter cannot import one. They are the runnable form of the package overview in doc.go.

// Parse into an immutable snapshot, which needs no Close and can be shared between goroutines.
func Example() {
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	snap, err := parser.ParseSnapshot(context.Background(), []byte("package p\n\nfunc F() {}\n"))
	if err != nil {
		log.Fatal(err)
	}
	// A nil error means a complete tree. It can still contain syntax errors.
	root := snap.Root()
	fmt.Println(root.Kind(), root.HasError())
	for child := range root.NamedChildren() {
		fmt.Println(child.Kind(), child.StartByte(), child.EndByte())
	}
	// Output:
	// source_file false
	// package_clause 0 9
	// function_declaration 11 22
}

// Run a query on a tree. Captures are snapshot nodes.
func Example_query() {
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	q, err := treesitter.NewQuery(golang.Language(), `(function_declaration name: (identifier) @name)`)
	if err != nil {
		log.Fatal(err) // a *treesitter.QueryError carries the offset and type
	}
	tree, err := parser.Parse(ctx, []byte("package p\n\nfunc F() {}\nfunc G() {}\n"), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tree.Close()

	for m, err := range tree.Matches(ctx, q, nil) {
		if err != nil {
			log.Fatal(err)
		}
		for _, c := range m.Captures {
			fmt.Println(q.CaptureNames()[c.Index], c.Node.Text())
		}
	}
	// Output:
	// name F
	// name G
}

// Edit a tree in place, parse again with it as the old tree, and read the changed ranges.
func Example_incrementalEdit() {
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	oldSrc := []byte("package p\n\nfunc F() {}\n")
	newSrc := []byte("package p\n\nfunc F(a int) {}\n")
	oldTree, err := parser.Parse(ctx, oldSrc, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer oldTree.Close()

	// The edit inserts "a int" at byte 18, between the parentheses of "F()".
	// An edited tree is only useful as the old tree of the next parse (Snapshot returns ErrTreeEdited), so take any
	// Snapshot before editing.
	edit := treesitter.EditFor(oldSrc, newSrc, 18, 18, 23)
	if err := oldTree.Edit(edit); err != nil {
		log.Fatal(err)
	}
	newTree, err := parser.Parse(ctx, newSrc, oldTree)
	if err != nil {
		log.Fatal(err)
	}
	defer newTree.Close()

	ranges, err := newTree.ChangedRanges(oldTree)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range ranges {
		fmt.Println(r.StartByte, r.EndByte)
	}
	fmt.Println(newTree.Outcome())
	// Output:
	// 18 23
	// AcceptedClean
}

// Walk a snapshot and convert byte offsets to line and column positions.
func Example_snapshot() {
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	snap, err := parser.ParseSnapshot(context.Background(), []byte("package p\n\nfunc F() {}\n"))
	if err != nil {
		log.Fatal(err)
	}
	snap.Walk(func(n treesitter.Node, depth int) treesitter.WalkAction {
		if n.Kind() == "block" {
			return treesitter.SkipChildren
		}
		if n.IsNamed() {
			fmt.Printf("%*s%s\n", depth*2, "", n.Kind())
		}
		return treesitter.Continue
	})
	pos := snap.Lines().Pos(snap.Root().EndByte())
	fmt.Println(pos.Line, pos.ByteCol)
	// Output:
	// source_file
	//   package_clause
	//     package_identifier
	//   function_declaration
	//     identifier
	//     parameter_list
	// 3 0
}
