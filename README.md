# go-treesitter

A Go syntax-analysis layer over a pinned `gotreesitter` runtime, with explicit
parse outcomes, an adapter boundary, and Windows-native CGO-free validation.

This repository owns syntax results and their evidence. Language semantics,
code graphs, and downstream application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md).
The [status board](docs/reports/release-critical-status.md) separates development
use from release approval. No release is approved yet.

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

Product tests include 50 source-bound C oracle records without invoking a C
compiler. [Oracle tooling](tools/native-oracle/README.md) documents producing
new records, using an updated generator, and comparing artifacts before adoption.
Generator selection has no release constant; runtime/grammar identities remain
pinned. The [isolated scanner evaluation](artifacts/session-06/phase3/patch-decision.md)
records the candidate patch recommendation and its limits. The product runtime
still contains the registered JSX/TSX and C# blockers.
