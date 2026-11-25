//go:build !no_go

package conformance_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)

// These examples are quoted verbatim by docs/guides/getting-started.md. A snippet runs from its "// snippet:<name>"
// line to the next "// endsnippet" line, or to the closing brace of its function (so a trailing "// Output:" comment
// belongs to the snippet). Example_gettingStartedGuide fails when the guide and this file drift apart.
// They live in this module rather than in the root package because a grammar module requires the root module.

func Example_gettingStartedParse() {
	// snippet:parse
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	snap, err := parser.ParseSnapshot(ctx, []byte("package p\n\nfunc F() {}\n"))
	if err != nil {
		log.Fatal(err)
	}
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

func Example_gettingStartedWalk() {
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()
	snap, err := parser.ParseSnapshot(context.Background(), []byte("package p\n\nfunc F() {}\n"))
	if err != nil {
		log.Fatal(err)
	}

	// snippet:walk
	snap.Walk(func(n treesitter.Node, depth int) treesitter.WalkAction {
		if n.Kind() == "block" {
			return treesitter.SkipChildren
		}
		if n.IsNamed() {
			fmt.Printf("%*s%s\n", depth*2, "", n.Kind())
		}
		return treesitter.Continue
	})
	fn, _ := snap.Root().NamedChild(1)
	name, _ := fn.ChildByFieldName("name")
	pos := snap.Lines().Pos(name.StartByte())
	fmt.Println(name.Text(), pos.Line, pos.ByteCol)
	// Output:
	// source_file
	//   package_clause
	//     package_identifier
	//   function_declaration
	//     identifier
	//     parameter_list
	// F 2 5
}

func Example_gettingStartedErrors() {
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	// snippet:syntax-errors
	// A syntax error is not a Go error: the tree is complete and Outcome says it has errors.
	tree, err := parser.Parse(ctx, []byte("package p\n\nfunc (\n"), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tree.Close()
	fmt.Println(tree.Outcome())
	snap, err := tree.Snapshot(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snap.Root().HasError())
	for n := range snap.All() {
		if n.IsError() || n.IsMissing() {
			fmt.Println(n.Kind(), n.IsMissing(), n.StartPoint())
		}
	}
	// Output:
	// AcceptedWithErrors
	// true
	// ERROR false {2 0}
}

func Example_gettingStartedParseError() {
	ctx := context.Background()

	// snippet:parse-error
	// No tree at all is a *ParseError. Match it with errors.Is on a sentinel and read details with errors.As.
	small, err := treesitter.NewParser(golang.Language(), treesitter.WithMaxInputBytes(8))
	if err != nil {
		log.Fatal(err)
	}
	defer small.Close()

	_, err = small.ParseSnapshot(ctx, []byte("package p\n"))
	var perr *treesitter.ParseError
	fmt.Println(errors.Is(err, treesitter.ErrResourceLimit), errors.As(err, &perr))
	fmt.Println(perr.Outcome)
	// Output:
	// true true
	// ResourceLimit
}

func Example_gettingStartedQuery() {
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	// snippet:query
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

func Example_gettingStartedEdit() {
	ctx := context.Background()
	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	// snippet:edit
	oldSrc := []byte("package p\n\nfunc F() {}\n")
	newSrc := []byte("package p\n\nfunc F(a int) {}\n")
	oldTree, err := parser.Parse(ctx, oldSrc, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer oldTree.Close()

	// "a int" is inserted at byte 18, between the parentheses of "F()". An edited tree is only the old tree of
	// the next parse (its Snapshot returns ErrTreeEdited), so take any Snapshot before editing.
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

func Example_gettingStartedLimits() {
	// snippet:limits
	parser, err := treesitter.NewParser(golang.Language(),
		treesitter.WithMaxInputBytes(1<<20),     // reject larger sources before any parsing
		treesitter.WithMaxSnapshotNodes(100000), // reject a larger export (ParseSnapshot, Tree.Snapshot)
		treesitter.WithMaxMemoryBytes(256<<20),  // linear memory of this parser's engine instance
	)
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()

	src := []byte("package p\n\nfunc F() {}\n")

	// A context that is already cancelled or expired stops the parse before it starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = parser.ParseSnapshot(ctx, src)
	fmt.Println(errors.Is(err, treesitter.ErrCancelled), errors.Is(err, context.Canceled))

	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err = parser.ParseSnapshot(ctx, src)
	fmt.Println(errors.Is(err, treesitter.ErrTimeout), errors.Is(err, context.DeadlineExceeded))

	// The parser stays usable after a failed parse.
	snap, err := parser.ParseSnapshot(context.Background(), src)
	fmt.Println(err, snap.Root().Kind())
	// Output:
	// true true
	// true true
	// <nil> source_file
}

func Example_gettingStartedPool() {
	// snippet:pool
	pool, err := treesitter.NewParserPool(golang.Language(), 4) // at most 4 parsers alive at once
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	parse := func(ctx context.Context, src []byte) (*treesitter.Snapshot, error) {
		parser, err := pool.Get(ctx) // waits for a free slot until ctx is done
		if err != nil {
			return nil, err
		}
		defer pool.Put(parser) // do not Close a pooled parser; Put frees the trees you left open
		return parser.ParseSnapshot(ctx, src)
	}

	var wg sync.WaitGroup
	kinds := make([]string, 8)
	for i := range kinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap, err := parse(context.Background(), []byte("package p\n"))
			if err != nil {
				log.Print(err)
				return
			}
			kinds[i] = snap.Root().Kind() // a Snapshot stays valid after Put
		}()
	}
	wg.Wait()
	fmt.Println(strings.Join(kinds, " "))
	// Output:
	// source_file source_file source_file source_file source_file source_file source_file source_file
}

func Example_gettingStartedKorean() {
	// snippet:korean
	// "// 한글" followed by a newline, in CP949. The library never guesses an encoding: name it.
	raw := []byte("// \xc7\xd1\xb1\xdb\npackage p\n")
	src, err := treesitter.NewSourceCP949(raw)
	if err != nil {
		log.Fatal(err)
	}
	if src.Replacements() != 0 {
		log.Fatal("input holds bytes that are not CP949; do not write the converted text back")
	}

	parser, err := treesitter.NewParser(golang.Language())
	if err != nil {
		log.Fatal(err)
	}
	defer parser.Close()
	snap, err := parser.ParseSnapshot(context.Background(), src.Bytes()) // UTF-8 from here on
	if err != nil {
		log.Fatal(err)
	}

	comment := snap.Root().NamedChild
	first, _ := comment(0)
	end := first.EndByte()
	pos := snap.Lines().Pos(end)
	fmt.Println(first.Text(), end, src.OriginalOffset(end))
	fmt.Println(pos.Line, pos.ByteCol, pos.UTF16Col, pos.RuneCol)
	// Output:
	// // 한글 9 7
	// 0 9 5 5
}

func Example_gettingStartedGuide() {
	guide, err := os.ReadFile("../../docs/guides/getting-started.md")
	if err != nil {
		log.Fatal(err)
	}
	code, err := os.ReadFile("getting_started_test.go")
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(code), "\r\n", "\n"), "\n")
	region := func(name string) string {
		var out []string
		in := false
		for _, l := range lines {
			switch {
			case !in && strings.TrimSpace(l) == "// snippet:"+name:
				in = true
			case in && (strings.TrimSpace(l) == "// endsnippet" || l == "}"):
				return strings.TrimSpace(strings.Join(out, "\n"))
			case in:
				out = append(out, l)
			}
		}
		return ""
	}
	dedent := func(s string) string { return strings.ReplaceAll("\n"+s, "\n\t", "\n")[1:] }

	// A snippet is quoted as <!-- snippet:name --> followed by a go block.
	blocks := regexp.MustCompile("(?s)<!-- snippet:([a-z-]+) -->\\r?\\n```go\\r?\\n(.*?)\\r?\\n```").FindAllStringSubmatch(string(guide), -1)
	bad := 0
	for _, b := range blocks {
		if want := dedent(region(b[1])); want == "" || want != strings.TrimSpace(strings.ReplaceAll(b[2], "\r\n", "\n")) {
			fmt.Println("guide snippet differs from the test:", b[1])
			bad++
		}
	}
	fmt.Println(bad, len(blocks) > 0)
	// Output:
	// 0 true
}
