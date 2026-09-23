# go-treesitter

A Go syntax-analysis layer over an internally bundled, pinned `gotreesitter` runtime, with explicit
parse outcomes, an adapter boundary, and Windows-native CGO-free validation.

This repository owns syntax results and their evidence. Language semantics,
code graphs, and downstream application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md).
The [status board](docs/reports/release-critical-status.md) separates development
use from release approval. The first version will be `v0.0.1`; release gates are
still being completed. The module includes its approved runtime patches and
needs no consumer `replace` directive or external runtime dependency.

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
error outcomes, cancellation, tree release and the JSX/TSX scanner correction
from a separate Go module, installed through a local versioned module proxy.
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

Consumers that need only the seven assessed routes can use upstream's existing
embedded grammar selection. This keeps the selected blobs inside the executable
and needs no runtime downloads or external grammar directory:

```powershell
$subset = 'grammar_subset,grammar_subset_go,grammar_subset_python,grammar_subset_javascript,grammar_subset_typescript,grammar_subset_tsx,grammar_subset_c_sharp'
go build -tags $subset ./...
go test -tags $subset ./... -count=1
```

Executable size depends on the selected grammars and application. See
[resource measurements and limitations](docs/reports/release-critical-status.md).
`go run ./tools/measure -language go -iterations 20` reproduces one fixed
workload; available routes are `go`, `py`, `js`, `jsx`, `ts`, `tsx` and `cs`.
The tool reports Go heap metrics, not OS RSS, and separates first-use grammar
loading from subsequent parse API calls.

The base module contains 203 grammars. The separate
[`grammars/gpl`](grammars/gpl/README.md) module adds the pinned `caddy`,
`disassembly` and `jq` grammars by a side-effect import, preserving 206 total.
The base module has no dependency on that module and its ZIP excludes the GPL
payload. Including the optional module brings its GPL distribution terms.

[Basic catalog checks](tools/catalog/README.md) cover all 206 grammars with
three fresh runs, determinism, incremental edits, cancellation and input limits.
This coverage is separate from the seven Release-Critical C oracle gates.
Completion preserves grammar-owned root coordinates and requires the runtime's
actual and expected EOF receipts; the retained COBOL controls also compare all
ordered nodes with the pinned C oracle.

Public contracts are under `docs/specs/`. Selected, source-bound validation
evidence is under `artifacts/release-candidate/`; original regression inputs
and C expectations stay in `testdata/`. Session prompts, plans, handoffs,
raw investigation logs and old experiment worktrees are local-only material.
See [publication boundaries](docs/specs/publication.md).

Product tests include 688 source-bound C oracle records and 439 edit comparisons
across the seven core routes, without invoking a C compiler.
[Oracle tooling](tools/native-oracle/README.md) documents producing
new records, using an updated generator, and comparing artifacts before adoption.
Generator selection has no release constant; runtime/grammar identities remain
pinned. Original scanner failures remain in the retained oracle records. The approved
[internal runtime](tools/runtime-bundle/README.md) now carries that six-line
scanner correction; KR-0001b is retired after product and consumer validation.
The separate extended catalog adds 35 fresh comparisons and 21 edit comparisons
over all seven routes, including UTF-8/CRLF and wide sibling boundaries.
Eight further TypeScript/TSX controls now agree with the officially adopted C
grammar built with the complete maintained upstream patch. KR-0003 is retired;
the original unpatched C evidence is preserved.
The C# recovery changes preserve missing-token scheduling, ambiguity over a
shared error history and the order in which results were accepted. Source-based
reconstruction preserves error-bearing trees. The retained C# catalogs add 74
fresh records and 72 edits, including reduced inputs, identifier changes,
Unicode, CRLF and the original real-world excerpts. Original KR-0002/KR-0004
failure evidence remains available; those inputs now require exact C agreement.

The final recovery corpus also covers Go EOF/NUL terminals, reserved keywords,
version-specific lookahead, hidden missing nodes and incremental dependencies.
No active C-tree exception remains. The internal Go blob is derived from the
same pinned C grammar, with distinct origin and product identities (ADR-0015).
Performance and memory measurements, controlled fallback reasons and remaining
release blockers are recorded in the [status board](docs/reports/release-critical-status.md)
and [selected evidence](artifacts/release-candidate/README.md).
Brightscript and Cooklang still have conflicting upstream ISC/MIT declarations;
native Windows ARM64 and hosted CI execution remain required for release.

The library's own code is [MIT licensed](LICENSE). See
[third-party notices](THIRD_PARTY_NOTICES.md) for the pinned runtime, all catalog
grammars, oracle tooling and copied validation fixtures.
