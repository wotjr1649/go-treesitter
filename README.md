# go-treesitter

**Preview (v0.5.0).** A CGO-free Go library for [tree-sitter](https://tree-sitter.github.io/). Each grammar
module is generated from the unmodified upstream tree-sitter runtime and pinned grammar C (wasm translated to Go by
`tools/gen`). Grammar C may carry recorded temporary
[ADR-0023 patches](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md); each patched module discloses them.
Native C comparisons and their measured scope are recorded on the status board.

This repository owns the `treesitter` API, its result contract, the engine ABI and the generator. Language semantics,
code graphs and application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md). The
[status board](docs/reports/release-critical-status.md) separates development use from release approval.

## Use

The root module has no grammar. v0.5.0 provides 24 grammar modules using engine ABI 3.
Add the modules you need
under `grammars/`: `bash`, `c`, `cpp`, `csharp`, `css`, `dart`, `golang`, `html`, `java`, `javascript` (including JSX),
`json`, `kotlin`, `postgres` (PostgreSQL SQL only; PL/pgSQL excluded), `php` (PHP parser only), `powershell`, `python`, `r`, `ruby`, `rust`, `tsql`, `tsx`, `typescript`,
`xml` (XML parser only), and `yaml`. The development tree after v0.5.0 also has `swift` (not part of any release).

Use `typescript` for `.ts` and `tsx` for `.tsx`: the same owner repository provides both grammars.
`<T>expr` type assertions are TypeScript-only; JSX is TSX-only.

Each language has exactly one grammar source repository, chosen by the tiers of
[ADR-0024](docs/design/decisions/ADR-0024-grammar-source-tiers.md):

| Tier | Modules | Grammar source |
|---|---|---|
| 1 | `bash`, `cpp`, `csharp`, `css`, `golang`, `html`, `json`, `php`, `python`, `ruby`, `rust` | `tree-sitter` organisation (`golang` uses `tree-sitter-go`) |
| 1 (maintained fork) | `c` | `tree-sitter-grammars/tree-sitter-c@f3202ac` (ADR-0024 rule 6) |
| 2 | `xml`, `yaml` | `tree-sitter-grammars/tree-sitter-xml`, `tree-sitter-grammars/tree-sitter-yaml` |
| 3 | `dart`, `kotlin`, `powershell`, `r`, `swift` | `UserNobody14/tree-sitter-dart`, `fwcd/tree-sitter-kotlin`, `airbus-cert/tree-sitter-powershell`, `r-lib/tree-sitter-r`, `alex-pinkus/tree-sitter-swift` (development tree after v0.5.0) |
| Owner-designated | `postgres` | `gmr/tree-sitter-postgres` (upstream v19.0.0 in the development tree after v0.5.0; ADR-0024 rule 4) |
| 7 (owner-maintained) | `tsql` | `wotjr1649/tree-sitter-mssql` (owner release v2.8.0 in the development tree after v0.5.0; origin `alfpricot/tree-sitter-mssql` v1.0.0; ADR-0024 rule 7) |
| 7 (owner-maintained) | `java` | `wotjr1649/tree-sitter-java` (owner release v3.0.3 in the development tree after v0.5.0; origin `tree-sitter/tree-sitter-java` v0.23.5; ADR-0024 rule 7) |
| 7 (owner-maintained) | `javascript` (including JSX) | `wotjr1649/tree-sitter-javascript` (owner release v4.4.0 in the development tree after v0.5.0; origin `tree-sitter/tree-sitter-javascript` v0.25.0; ADR-0024 rule 7) |
| 7 (owner-maintained) | `typescript`, `tsx` | `wotjr1649/tree-sitter-typescript` (owner release v3.1.0 in the development tree after v0.5.0; origin `tree-sitter/tree-sitter-typescript` v0.23.2; ADR-0024 rule 7) |

Exact revisions are in [pins.json](tools/gen/pins.json). Bash, Ruby, YAML and Python carry recorded temporary C-only patches
([ADR-0023](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md); each module's README lists them).
See the [status board](docs/reports/release-critical-status.md) for measured scopes and pending release gates, and
[limitations](docs/reports/limitations.md).

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

Contracts: [`docs/specs/parser-result.md`](docs/specs/parser-result.md), [`docs/design/api-v2.md`](docs/design/api-v2.md).

Preview [limitations](docs/reports/limitations.md); coming from the removed `syntax` API: [MIGRATING.md](MIGRATING.md).

## Develop

```sh
CGO_ENABLED=0 go build ./... && go vet ./... && go test -count=1 ./...
```

Grammar modules, `tools/gen`, `tools/bench` and `tools/conformance` are separate Go modules; run their tests from
their own directories. Pins live in [`tools/gen/pins.json`](tools/gen/pins.json). Third-party notices:
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
