# Getting started

How to parse source code with go-treesitter, from install to concurrent use. The root module has no grammar: you add
one grammar module per language. Everything below runs as a test (`tools/conformance/getting_started_test.go`),
and each Go snippet is that test's text. A test compares them, so a drift between the guide and the code fails it. The
conformance workflow runs these tests once its step selects `^Example` (the change in PR #127); until that is on main,
run them yourself: `cd tools/conformance && go test -run Example .`.

This is a Preview (v0.x): the compatibility rules of [api-stability.md](../design/api-stability.md) start at root 1.0.
For the exact contracts read [parser-result.md](../specs/parser-result.md) and the package documentation
(`go doc -all github.com/wotjr1649/go-treesitter`).

## Install

Add the root module and a grammar module, each at the tag the [README](../../README.md#use) lists:

```sh
go mod init example.com/app   # in a new, empty directory
go get github.com/wotjr1649/go-treesitter@v0.7.0
go get github.com/wotjr1649/go-treesitter/grammars/golang@v0.6.0
go mod tidy                   # after the imports below are in your code
```

Go 1.26 or later; no C compiler is needed (`CGO_ENABLED=0` builds). Import both packages. The grammar package
exposes `Language()`, which is what `NewParser` takes:

```go
import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)
```

The snippets below also use `context`, `errors`, `fmt`, `log`, `sync`, `strings` and `time` from the standard library and
leave the `func main` around them out. Paste them one after another into one `func main`: they use distinct names, and
only `ctx` and `parser` (from the first snippet) and `snap` (the walk snippet) carry over. Each snippet can also stand
alone in its own function if you add those lines to it.

## Parse a file and walk the snapshot

`ParseSnapshot` parses and returns an immutable `*Snapshot`. A snapshot owns a copy of the source and its node
data, needs no `Close`, and can be shared between goroutines. `Parser.Close` is the only cleanup.

<!-- snippet:parse -->
```go
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
```

Nodes are small value handles. `Walk` visits them in preorder and lets you skip a subtree; `ChildByFieldName`,
`NamedChild`, `Parent`, the sibling methods and `Snapshot.All` navigate; `Snapshot.Lines` converts byte offsets to
lines and columns:

<!-- snippet:walk -->
```go
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
```

Offsets are bytes of the UTF-8 source (`Node.StartByte`), and `Node.Text` is a substring of the snapshot's source.
`Index` (`snap.Index()`) answers repeated "nodes of kind K" and "node at offset" lookups.

## Syntax errors and Outcome

A **nil error means a complete tree**, which can still contain syntax errors. They are not Go errors: check
`Node.HasError`, or `Tree.Outcome` (`AcceptedClean` or `AcceptedWithErrors`). `ERROR` nodes and `MISSING` nodes mark
where the grammar recovered:

<!-- snippet:syntax-errors -->
```go
// A syntax error is not a Go error: the tree is complete and Outcome says it has errors.
broken, err := parser.Parse(ctx, []byte("package p\n\nfunc (\n"), nil)
if err != nil {
	log.Fatal(err)
}
defer broken.Close()
fmt.Println(broken.Outcome())
brokenSnap, err := broken.Snapshot(ctx)
if err != nil {
	log.Fatal(err)
}
fmt.Println(brokenSnap.Root().HasError())
for n := range brokenSnap.All() {
	if n.IsError() || n.IsMissing() {
		fmt.Println(n.Kind(), n.IsMissing(), n.StartPoint())
	}
}
// Output:
// AcceptedWithErrors
// true
// ERROR false {2 0}
```

When there is no tree you get a `*ParseError` whose `Outcome` is `Timeout`, `Cancelled`, `ResourceLimit`,
`InvalidInput`, `EngineFault` or `IncompatibleLanguage`. Classify it with `errors.Is` against the `Err…` sentinels
(`ErrTimeout` also matches `context.DeadlineExceeded`, `ErrCancelled` matches `context.Canceled`) and read
`ParseError.Outcome` and `InstanceReset` with `errors.As`. Do not match on message text; it may change in any release.

<!-- snippet:parse-error -->
```go
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
```

## Queries

`NewQuery` validates a tree-sitter query once; a `*Query` is immutable and can be reused by every parser of its
language. `Tree.Matches` streams matches whose captures are snapshot nodes. Queries run on a `*Tree` (from `Parse`),
so close it when you are done:

<!-- snippet:query -->
```go
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
```

`QueryOptions` restricts the byte range or the match-state limit. Two costs to know: the first `Matches` call on a
tree indexes the whole tree, and the parser stays busy until you finish (or break out of) the iteration. Text
predicates use a regex subset; `#any-*` predicates are validated but do not filter. See [query-abi.md](../design/query-abi.md)
and [limitations](../reports/limitations.md) row 5. Queries need a grammar module of engine ABI 2 or later.

## Incremental editing

For an editor-style loop, parse once with `Parse`, apply the edit to that tree, and parse again with it as the old
tree. `EditFor` builds a consistent `InputEdit` from the old and new text; `ChangedRanges` reports where the
structure differs. An incremental result is not guaranteed to equal a fresh parse of the same text.

<!-- snippet:edit -->
```go
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
```

After `Edit` the tree is only useful as the old tree of the next `Parse`: its `Snapshot` fails with
`ErrTreeEdited`, and `Matches` is for an unedited tree. Take snapshots and run queries first, and close both trees.

## Resource limits and cancellation

Options bound the cost of untrusted input. A value of 0 means no explicit limit (for memory: the engine default of
1 GiB); a negative or out-of-range value makes `NewParser` fail with `InvalidInput`.

| Option | Bounds |
|---|---|
| `WithMaxInputBytes` | source length, checked before any parsing (`ResourceLimit`) |
| `WithMaxSnapshotNodes` | node count of an exported snapshot, checked before the export (`ResourceLimit`) |
| `WithMaxMemoryBytes` | the linear memory of **this parser's** engine instance, including query indexes and match buffers; from 32 MiB to 4 GiB, anything else fails `NewParser`. It does not cover the Go heap of sources, snapshots and line indexes. A refused growth is `ResourceLimit` |
| `WithRecycleAboveBytes` | not a limit: recreates the instance after a large parse so its memory returns to the OS. It waits while the parser holds a live `Tree` (`ParseSnapshot` recycles at once), so close your trees |
| `WithStrictUTF8` | rejects invalid UTF-8 (`InvalidInput`) instead of parsing it as C tree-sitter does |

Every parse takes a `context.Context`; a deadline or cancellation ends the parse between parser steps with
`Timeout` or `Cancelled`, and the parser stays usable afterwards.

<!-- snippet:limits -->
```go
limited, err := treesitter.NewParser(golang.Language(),
	treesitter.WithMaxInputBytes(1<<20),     // reject larger sources before any parsing
	treesitter.WithMaxSnapshotNodes(100000), // reject a larger export (ParseSnapshot, Tree.Snapshot)
	treesitter.WithMaxMemoryBytes(256<<20),  // linear memory of this parser's engine instance (32 MiB to 4 GiB)
)
if err != nil {
	log.Fatal(err)
}
defer limited.Close()

input := []byte("package p\n\nfunc F() {}\n")

// A context that is already cancelled or expired stops the parse before it starts.
done, cancel := context.WithCancel(context.Background())
cancel()
_, err = limited.ParseSnapshot(done, input)
fmt.Println(errors.Is(err, treesitter.ErrCancelled), errors.Is(err, context.Canceled))

late, cancelLate := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
defer cancelLate()
_, err = limited.ParseSnapshot(late, input)
fmt.Println(errors.Is(err, treesitter.ErrTimeout), errors.Is(err, context.DeadlineExceeded))

// The parser stays usable after a failed parse.
again, err := limited.ParseSnapshot(context.Background(), input)
fmt.Println(err, again.Root().Kind())
// Output:
// true true
// true true
// <nil> source_file
```

Cancellation is checked by the parser, not inside a grammar's external scanner: a scanner that never returns cannot
be interrupted ([limitations](../reports/limitations.md) row 12). Run untrusted input in a process you can stop if that
matters to you. `WithMaxMemoryBytes` is per parser: a pool of N parsers can use N times that.

## Lifetimes and concurrency

- A `Snapshot` and its `Node`s are immutable and safe for any number of goroutines. They stay valid after the parser
  is closed.
- A `Parser` and the `Tree`s it returned are **one confinement unit**: use them from one goroutine at a time. Misuse
  is detected on a best-effort basis and reported as `InvalidInput`.
- Close every `Tree` and every `Parser`. `Parser.Close` and `Parser.Reset` invalidate the parser's live trees
  (`ErrTreeInvalidated`); snapshots are unaffected.
- A leaked `Tree` is freed later by its parser's owner, but do not rely on it for memory you care about.
- For concurrent parsing use `NewParserPool`: `Get`, then `Put` (never `Close` a pooled parser). `Put` frees every tree you
  left open, so return snapshots, not trees, across goroutines. The pool's own `Close` closes the idle parsers.

<!-- snippet:pool -->
```go
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
```

## Korean and other non-UTF-8 input

The parser takes UTF-8. A UTF-8 BOM is fine as it is (the first line's columns then count its bytes: see
[korean-input.md](korean-input.md)); `NewSource` converts UTF-16 that starts with a BOM,
`NewSourceUTF16` converts UTF-16 of a known byte order, and `NewSourceCP949` converts CP949 (windows-949). Nothing
guesses an encoding. `Source.OriginalOffset` maps a UTF-8 offset back to the file's bytes, and `LineIndex` gives byte,
UTF-16 and rune columns (for LSP). Check `Source.Replacements` before you write converted text back.

<!-- snippet:korean -->
```go
// "// 한글" followed by a newline, in CP949. The library never guesses an encoding: name it.
raw := []byte("// \xc7\xd1\xb1\xdb\npackage p\n")
src, err := treesitter.NewSourceCP949(raw)
if err != nil {
	log.Fatal(err)
}
if src.Replacements() != 0 {
	log.Fatal("input holds bytes that are not CP949; do not write the converted text back")
}

koParser, err := treesitter.NewParser(golang.Language())
if err != nil {
	log.Fatal(err)
}
defer koParser.Close()
koSnap, err := koParser.ParseSnapshot(context.Background(), src.Bytes()) // UTF-8 from here on
if err != nil {
	log.Fatal(err)
}

comment := koSnap.Root().NamedChild
first, _ := comment(0)
end := first.EndByte()
at := koSnap.Lines().Pos(end)
fmt.Println(first.Text(), end, src.OriginalOffset(end))
fmt.Println(at.Line, at.ByteCol, at.UTF16Col, at.RuneCol)
// Output:
// // 한글 9 7
// 0 9 5 5
```

[Korean and legacy-encoded input](korean-input.md) covers the details: choosing the entry point, columns for LSP,
which grammars accept Korean identifiers, writing edited text back to a CP949 file, CRLF, and incremental edits on
converted text. No other legacy code page (Shift_JIS, for example) is built in; convert it to UTF-8 first.

## Choosing grammar versions

Each grammar module is versioned and tagged independently of the root (`grammars/<dir>/vX.Y.Z`), and a grammar tag
declares the minimum root it needs. Go selects the highest root that any module you use requires. Consequences:

- Take the tag per grammar from the [README table](../../README.md#use); the root can move without moving your
  grammars, and a grammar changes only when you ask for its new tag.
- A grammar tag fixes the tree shapes and captures; a newer tag can change them when its pinned grammar source moves.
  Re-run your tests when you move a grammar tag.
- The [compatibility table](../reports/compatibility.md) records the tested root and grammar combinations; the
  [support table](../reports/grammar-support.md) lists each tag's known grammar limits.
- A module built for an engine ABI the root does not know fails `NewParser` with `IncompatibleLanguage`.

## Where limits are documented

| Question | Document |
|---|---|
| What a parse result guarantees, and what not to assume | [parser-result.md](../specs/parser-result.md) |
| What the Preview does not do or has not shown (queries, memory, cancellation, encodings) | [limitations.md](../reports/limitations.md) |
| Known defects per grammar tag, with the input that shows each | [grammar-support.md](../reports/grammar-support.md) and [known-gaps](../known-gaps/) |
| Which API is stable, experimental or generator-only | [api-stability.md](../design/api-stability.md) |
| Query predicates, regex subset and query costs | [query-abi.md](../design/query-abi.md) |
| Which root and grammar tags were tested together | [compatibility.md](../reports/compatibility.md) |
| Moving from the removed `syntax` API | [MIGRATING.md](../../MIGRATING.md) |
