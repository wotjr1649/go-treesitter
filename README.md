# go-treesitter

**[Preview v0.7.0](https://github.com/wotjr1649/go-treesitter/releases/tag/v0.7.0)** (GitHub pre-release).
A CGO-free Go library for [tree-sitter](https://tree-sitter.github.io/). Each grammar
module is generated from the unmodified upstream tree-sitter runtime and pinned grammar C (wasm translated to Go by
`tools/gen`). Grammar C may carry recorded temporary
[ADR-0023 patches](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md); each patched module discloses them.
Native C comparisons and their measured scope are recorded on the status board.

This repository owns the `treesitter` API, its result contract, the engine ABI and the generator. Language semantics,
code graphs and application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md). The
[status board](docs/reports/release-critical-status.md) separates development use from release approval.

## When to use go-treesitter

Use it when your Go program should not depend on a C toolchain. The library builds with `CGO_ENABLED=0`
([getting started](docs/guides/getting-started.md): no C compiler is needed), so:

- cross-compiling needs no C cross-compiler, and a binary can be built statically;
- Windows needs no C toolchain (Windows amd64 is the Tier-1 platform; the other platforms claimed are listed under
  [Support](#support));
- each `Parser` runs its grammar in its own translated-wasm instance, and its linear memory can be capped
  (`WithMaxMemoryBytes`), with input size, snapshot size and query budgets and `context` cancellation alongside, which
  bounds the cost of parsing untrusted input ([resource limits and cancellation](docs/guides/getting-started.md#resource-limits-and-cancellation)).

It runs the upstream tree-sitter runtime and grammar C pinned in [pins.json](tools/gen/pins.json), translated by
`tools/gen`. Failures come back as one result contract: a complete tree, or a `*ParseError` with an `Outcome`
([parser-result](docs/specs/parser-result.md)). What is not shown yet is on the
[limitations](docs/reports/limitations.md) page.

Trade-offs: parsing runs translated code, not native C, so it can be slower than native tree-sitter, and
`NewParser` and `NewQuery` have a start-up cost. This README gives no comparison numbers; the only measurements here are
the labeled local diagnostics under [Performance guidance](#performance-guidance), which also say how to reduce the cost
(reuse `Parser` and `Query`; use `Parse` rather than `ParseSnapshot` where a snapshot is not needed). If raw
parsing speed is your priority and your build can use C, consider the official
[Go bindings of the tree-sitter C library](https://github.com/tree-sitter/go-tree-sitter) instead, and measure on your own
grammar and inputs.

## Use

The root module has no grammar. v0.7.0 provides 25 grammar modules using engine ABI 4: five at new `v0.7.0` tags
(`css`, `powershell`, `ruby`, `swift`, `xml`) and 20 that keep their `v0.6.0` tags ([tag table](#use)).
Add the modules you need
under `grammars/`: `bash`, `c`, `cpp`, `csharp`, `css`, `dart`, `golang`, `html`, `java`, `javascript` (including JSX),
`json`, `kotlin`, `php` (PHP parser only), `postgres` (PostgreSQL SQL only; PL/pgSQL excluded), `powershell`,
`python`, `r`, `ruby`, `rust`, `swift`, `tsql`, `tsx`, `typescript`, `xml` (XML parser only), and `yaml`.

Use `typescript` for `.ts` and `tsx` for `.tsx`: the same owner repository provides both grammars.
`<T>expr` type assertions are TypeScript-only; JSX is TSX-only.

Each language has exactly one grammar source repository, chosen by the tiers of
[ADR-0024](docs/design/decisions/ADR-0024-grammar-source-tiers.md):

| Tier | Modules | Grammar source |
|---|---|---|
| 1 | `bash`, `cpp`, `csharp`, `css`, `golang`, `html`, `json`, `php`, `python`, `ruby`, `rust` | `tree-sitter` organisation (`golang` uses `tree-sitter-go`; `csharp` regenerates from upstream v0.24.0 and `php` uses [upstream v0.25.2](https://github.com/tree-sitter/tree-sitter-php/releases/tag/v0.25.2)) |
| 1 (maintained fork) | `c` | `tree-sitter-grammars/tree-sitter-c@f3202ac` (ADR-0024 rule 6) |
| 2 | `xml`, `yaml` | `tree-sitter-grammars/tree-sitter-xml`, `tree-sitter-grammars/tree-sitter-yaml` |
| 3 | `dart`, `kotlin`, `powershell`, `r`, `swift` | `UserNobody14/tree-sitter-dart`, `fwcd/tree-sitter-kotlin`, `airbus-cert/tree-sitter-powershell`, `r-lib/tree-sitter-r`, `alex-pinkus/tree-sitter-swift@fef1236` |
| Owner-designated | `postgres` | `gmr/tree-sitter-postgres` (upstream v19.0.0; ADR-0024 rule 4) |
| 7 (owner-maintained) | `tsql` | `wotjr1649/tree-sitter-mssql` (owner release v2.8.0; origin `alfpricot/tree-sitter-mssql` v1.0.0; ADR-0024 rule 7) |
| 7 (owner-maintained) | `java` | `wotjr1649/tree-sitter-java` (owner release v3.0.3; origin `tree-sitter/tree-sitter-java` v0.23.5; ADR-0024 rule 7) |
| 7 (owner-maintained) | `javascript` (including JSX) | `wotjr1649/tree-sitter-javascript` (owner release v4.4.0; origin `tree-sitter/tree-sitter-javascript` v0.25.0; ADR-0024 rule 7) |
| 7 (owner-maintained) | `typescript`, `tsx` | `wotjr1649/tree-sitter-typescript` (owner release v3.1.0; origin `tree-sitter/tree-sitter-typescript` v0.23.2; ADR-0024 rule 7) |

Exact revisions are in [pins.json](tools/gen/pins.json). Bash, CSS, Go, HTML, PowerShell, Python, Ruby, Rust, XML and YAML carry recorded temporary patches
([ADR-0023](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md); each module's README lists them).
See the [status board](docs/reports/release-critical-status.md) for gate results and publication evidence, and
[limitations](docs/reports/limitations.md) for the scope of this Preview.

A grammar module is versioned independently of the root ([ADR-0028](docs/design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md#decision)).
v0.7.0 gives new tags to the five modules whose pin or patches changed after v0.6.0; the other 20 modules have no
change to release and keep their published tags.

| Grammar tag | Modules | Minimum root |
|---|---|---|
| `grammars/<dir>/v0.7.0` (new) | `css`, `powershell`, `ruby`, `swift`, `xml` | `v0.6.0` |
| `grammars/<dir>/v0.6.0` (carried forward) | `bash`, `c`, `cpp`, `csharp`, `dart`, `golang`, `html`, `java`, `javascript`, `json`, `kotlin`, `php`, `postgres`, `python`, `r`, `rust`, `tsql`, `tsx`, `typescript`, `yaml` | `v0.6.0` |

Go selects the highest root that any selected module requires. Each new tag declares root `v0.6.0` as its minimum;
root `v0.7.0` is the recommended pairing. The [compatibility table](docs/reports/compatibility.md) records the tested combinations and the
[support table](docs/reports/grammar-support.md) lists known limits by tag.

Install the root and the grammar modules you use, each at the tag in the table above. For example,

```sh
go get github.com/wotjr1649/go-treesitter@v0.7.0
go get github.com/wotjr1649/go-treesitter/grammars/golang@v0.6.0
go get github.com/wotjr1649/go-treesitter/grammars/swift@v0.7.0
```

Upgrading only the root to v0.7.0 keeps your existing grammar tags; a grammar changes only when you request its new tag.

```go
import (
	"context"
	"fmt"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)

parser, err := treesitter.NewParser(golang.Language())
if err != nil {
	return err
}
defer parser.Close()

snap, err := parser.ParseSnapshot(context.Background(), []byte("package p\nfunc F() {}\n"))
if err != nil {
	return err
}
fmt.Println(snap.Root().Kind()) // source_file
```

Queries run on a `*Tree` and stream matches whose captures are snapshot nodes:

```go
q, err := treesitter.NewQuery(golang.Language(), `(function_declaration name: (identifier) @name)`)
if err != nil {
	return err // *QueryError carries the offset and type
}
ctx := context.Background()
tree, err := parser.Parse(ctx, []byte("package p\nfunc F() {}\n"), nil)
if err != nil {
	return err
}
defer tree.Close()
for m, err := range tree.Matches(ctx, q, nil) {
	if err != nil {
		return err
	}
	for _, c := range m.Captures {
		fmt.Println(q.CaptureNames()[c.Index], c.Node.Text())
	}
}
```

- `ParseSnapshot` returns an immutable `*Snapshot` that needs no `Close` and is safe to share between goroutines.
  `Parse` returns a `*Tree` for incremental work (`Edit`, then `Parse(ctx, src, old)`); close every `Tree`.
- A `Parser` and its `Tree`s are one confinement unit: use them from one goroutine at a time. `NewParserPool` lends parsers
  to concurrent goroutines.
- Input is UTF-8. `NewSource` converts UTF-16 that starts with a byte order mark; for UTF-16 without one, call
  `NewSourceUTF16` with the byte order; for CP949 (Korean), call `NewSourceCP949`. The library never guesses an
  encoding, and no other legacy code page (for example Shift_JIS) is built in: convert it to UTF-8 first. See the
  [Korean and CP949 input guide](docs/guides/korean-input.md) for decoding, offset and column conversion, and writing
  edited text back. A C# file that ends in a preprocessor directive without a final newline gets a
  root error without an ERROR node ([known gap](docs/known-gaps/csharp.md#csharp-directive-eof)); append a newline.
- A nil error means a complete tree, which can still contain syntax errors: check `snap.Root().HasError()` (`Tree.Outcome()`
  is `AcceptedClean` or `AcceptedWithErrors`). No result is a `*ParseError` carrying the `Outcome` (`Timeout`, `Cancelled`, …).

New to the library? Read the [getting-started guide](docs/guides/getting-started.md).

Contracts: [`docs/specs/parser-result.md`](docs/specs/parser-result.md), [`docs/design/api-v2.md`](docs/design/api-v2.md).

### Performance guidance

- **Create a `Parser` once and reuse it.** `NewParser` instantiates the grammar's linear memory, which is far more
  expensive than a parse of a typical file. Do not call it per file or per request: keep one `Parser` per goroutine, or
  use a `ParserPool` when goroutines come and go. Reuse keeps the warm instance.
- **`ParserPool` is for concurrency, not speed.** Its `Get`/`Put` add a few percent over reusing one `Parser`.
  A single goroutine should hold a `Parser` directly.
- **`Parse` + `Close` is cheaper than `ParseSnapshot`.** `ParseSnapshot` also exports the whole tree into an
  immutable `*Snapshot`. Use `Parse` when you query or walk the `*Tree` and then close it, or edit incrementally;
  use `ParseSnapshot` when the result must outlive the parser or be shared between goroutines.
- **Create a `Query` once and reuse it.** `NewQuery` compiles the pattern in the grammar's engine on a scratch
  `Parser` that it creates and closes on every call, so each call costs at least a `NewParser` plus the compile. Each
  `Parser` compiles a `Query` once more on its first `Tree.Matches`, at about the same cost. The compile cost follows the
  pattern's structure, not its length. A local Windows diagnostic (commit `7ffa3f9`, host CPU load 97–100%, 2026-11-26;
  E4 in the terms of docs/specs/validation.md) took one `NewQuery` call about 33–191 ms with tsql queries, 9–64 ms with typescript
  queries and 0.1 ms with json queries. A `Query` is immutable and may be shared by parsers of its language: cache it and do not call
  `NewQuery` per file or per request.
- The first parse on a new instance is slower than later ones (memory pages are committed on first use), another
  reason to reuse instances.

A local Windows measurement, not a release guarantee: Windows amd64 on a developer machine under 63–97% load, commit
`e5e4b29`, warm instances, 2026-11-26. It is an E4 local diagnostic in the terms of
[docs/specs/validation.md](docs/specs/validation.md) and says nothing about other platforms; results depend on
grammar, input and machine:

| Operation | Measured cost |
|---|---|
| `NewParser` | about 7–9 ms (tsql), 1.4 ms (typescript), 0.12 ms (json) |
| New `Parser` for every file instead of reuse | 13–380 times slower, depending on grammar and file size |
| `ParserPool` `Get`/`Put` instead of one reused `Parser` | +3–11% |
| `ParseSnapshot` instead of `Parse` + `Close` | +33–76% (large inputs), +18–48% (small files) |
| First parse on a new instance | +15–20% |

Preview [limitations](docs/reports/limitations.md); coming from the removed `syntax` API: [MIGRATING.md](MIGRATING.md).

## Support

The support policy takes effect at 1.0; until then this is a Preview. From 1.0, security and defect fixes cover the
latest 1.x minor and the preceding 1.x minor (if any) of the root module, and the latest tag of each grammar module.
Response time is best-effort. Details: [SECURITY.md](SECURITY.md).

- **Go:** the two Go releases the Go team supports; the floor is Go 1.26 (`go.mod`), tested on 1.26.x and 1.27.x. When the Go team's window moves, the floor and CI follow in the next root
  minor, so a release the Go team no longer supports can stay buildable until then
  ([ADR-0028](docs/design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md) decision 5).
- **Platforms:** Windows amd64 is Tier-1 and decides product gates
  ([ADR-0003](docs/design/decisions/ADR-0003-windows-tier1-cgo-free-production.md)). Linux amd64/arm64, macOS arm64 and
  Windows arm64 are claimed because the engine CI lane executes there; any other platform is not claimed
  ([validation](docs/specs/validation.md)). All use `CGO_ENABLED=0`.

## Develop

```sh
CGO_ENABLED=0 go build ./... && go vet ./... && go test -count=1 ./...
```

Grammar modules, `tools/gen`, `tools/bench` and `tools/conformance` are separate Go modules; run their tests from
their own directories. Pins live in [`tools/gen/pins.json`](tools/gen/pins.json). Third-party notices:
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
