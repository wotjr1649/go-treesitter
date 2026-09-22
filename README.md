# go-treesitter

A Go syntax-analysis layer over a pinned `gotreesitter` runtime, with explicit
parse outcomes, an adapter boundary, and Windows-native CGO-free validation.

This repository owns syntax results and their evidence. Language semantics,
code graphs, and downstream application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md).
The [status board](docs/reports/release-critical-status.md) separates development
use from release approval. No release is approved yet.

Create a parser through the public package; the implementation stays internal:

```go
parser := treesitter.New() // import treesitter "github.com/wotjr1649/go-treesitter"
result, err := parser.Parse(ctx, syntax.Request{
    Filename: "hello.go",
    Source:   []byte("package hello\nfunc Hello() {}\n"),
    Timeout:  time.Second,
})
if result.Tree != nil {
    defer result.Tree.Close()
}
if err != nil || !result.Complete() {
    // Parsing stopped or could not run. Inspect result.Diagnostics.
    return
}
if result.Outcome == syntax.AcceptedWithErrors {
    // A complete recovery tree is available; the source has syntax errors.
}
```

`syntax` is `github.com/wotjr1649/go-treesitter/syntax`. The runnable
[external consumer](testdata/consumer/main.go) exercises fresh parsing, an edit,
error outcomes, cancellation and tree release from a separate Go module.
Keep each tree on one worker; close every returned tree. The API remains
unreleased and may change before the first version tag.

For repeated AST lookup, build `index, err := syntax.NewIndex(result.Tree.Nodes())`
after a complete result. `index.NodeAt(byteOffset)` returns the deepest node's
snapshot index, and `index.OfType("identifier")` yields matching indexes in
preorder. Positions use half-open UTF-8 byte ranges; EOF and zero-width missing
nodes are excluded from position lookup. The index is optional, retains no
backend handle, and supports concurrent readers. Rebuild it after an edit.
Construction adds time and memory, so a single scan can be cheaper for a few
lookups. See the retained `BenchmarkSnapshotLookup` for the measured workload.

For bounded inputs, set `Request.Limits`, for example
`syntax.Limits{MaxInputBytes: 8 << 20, MaxSnapshotNodes: 100_000,
MemoryBudgetBytes: 64 << 20}`. Runtime memory and work limits are checked at
parser checkpoints; they are not a hard process-memory cap and may change the
runtime route. Grammar caches, source copies and snapshots are outside that
runtime memory threshold. Context cancellation covers snapshot construction.
An explicit memory budget also rejects results whose reported arena plus
scratch footprint exceeds it, even if the upstream growth check accepted them.
`Timeout` covers the parse API, excluding grammar loading. Negative limits or
timeouts are rejected as `not_run`; a local cap reports `resource_limit` and
an adapter-owned `LimitReason` without changing the raw runtime `StopReason`.

Run the Windows product checks with:

```powershell
$env:CGO_ENABLED = '0'
go build ./...
go vet ./...
go test ./... -count=1
```

`docs/prompts/`, `docs/specs/`, `docs/plans/`, and `artifacts/handoff/` are local
documents intentionally excluded from Git. Retained test evidence is under
`artifacts/session-05/` and `artifacts/session-06/`. See `experiments/v052-v053/`
for the older runtime comparison.

Product tests include 85 source-bound C oracle records without invoking a C
compiler. [Oracle tooling](tools/native-oracle/README.md) documents producing
new records, using an updated generator, and comparing artifacts before adoption.
Generator selection has no release constant; runtime/grammar identities remain
pinned. The [isolated scanner evaluation](artifacts/session-06/phase3/patch-decision.md)
records the candidate patch recommendation and its limits. The product runtime
still contains the registered JSX/TSX and C# blockers.
The separate extended catalog adds 35 fresh comparisons and 21 edit comparisons
over all seven routes, including UTF-8/CRLF and wide sibling boundaries.
