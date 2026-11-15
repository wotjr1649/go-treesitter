# go-treesitter

**Preview (v0.4.0).** A CGO-free Go library for [tree-sitter](https://tree-sitter.github.io/). Each grammar
module is generated from the unmodified upstream tree-sitter runtime and pinned grammar C (wasm translated to Go by
`tools/gen`). Grammar C may carry recorded temporary
[ADR-0023 patches](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md); each patched module discloses them.
Native C comparisons and their measured scope are recorded on the status board.

This repository owns the `treesitter` API, its result contract, the engine ABI and the generator. Language semantics,
code graphs and application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md). The
[status board](docs/reports/release-critical-status.md) separates development use from release approval.

## Use

The root module has no grammar. v0.4.0 provides 23 grammar modules using engine ABI 3. Add the modules you need
under `grammars/`: `bash`, `c`, `cpp`, `csharp`, `css`, `dart`, `golang`, `html`, `java`, `javascript` (including JSX),
`json`, `kotlin`, `php` (PHP parser only), `powershell`, `python`, `r`, `ruby`, `rust`, `tsql`, `tsx`, `typescript`,
`xml` (XML parser only), and `yaml`.

Use `typescript` for `.ts` and `tsx` for `.tsx`: the same upstream repository provides two grammars.
`<T>expr` type assertions are TypeScript-only; JSX is TSX-only.

Each language has exactly one upstream repository, chosen by the tiers of
[ADR-0024](docs/design/decisions/ADR-0024-grammar-source-tiers.md):

| Tier | Modules | Upstream |
|---|---|---|
| 1 | `bash`, `c`, `cpp`, `csharp`, `css`, `golang`, `html`, `java`, `javascript`, `json`, `php`, `python`, `ruby`, `rust`, `tsx`, `typescript` | `tree-sitter` organisation (`golang` uses `tree-sitter-go`; TSX and TypeScript share `tree-sitter-typescript`); `c` follows the maintained fork `tree-sitter-grammars/tree-sitter-c` that nvim-treesitter adopted (ADR-0024 rule 6) |
| 2 | `xml`, `yaml` | `tree-sitter-grammars/tree-sitter-xml`, `tree-sitter-grammars/tree-sitter-yaml` |
| 3 | `dart`, `kotlin`, `powershell`, `r` | `UserNobody14/tree-sitter-dart`, `fwcd/tree-sitter-kotlin`, `airbus-cert/tree-sitter-powershell`, `r-lib/tree-sitter-r` |
| Owner-designated | `tsql` | `alfpricot/tree-sitter-mssql` |

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
