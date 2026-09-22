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
`artifacts/session-05/`. See `experiments/v052-v053/` for the isolated comparison
and ADR-0007 for generator selection without a fixed release number.
