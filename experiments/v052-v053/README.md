# Controlled runtime/route comparison

This nested module pins the older runtime. The main module retains its own pin.
The same `internal/gtsadapter/probe_test.go` file is compiled from each module
root, so both cells for a version share identical harness source.

Check inputs before measuring:

```powershell
go test ./internal/gtsadapter -run TestCorpusIdentity -v -count=1 -args -corpus D:/AIDEV/go-treesitter/testdata/newtonsoft
```

Run once per cell, with `TestComparison`, explicit `-cell A`/`B`/`C`/`D`,
`-expected-version`, and `-compact-disabled` only for B and D. A/B run inside
this module. C/D run from the repository root using the test file path as the
Go test package argument and `GOFLAGS=-modfile=<absolute main go.mod>` so the
metadata subprocess also resolves the main pin from its nested test directory.
Exact session invocations live in Phase 3 evidence.

Every corpus hash and the shared C# grammar blob are checked before parsing.
Parses are sequential; process-global admission counter deltas are therefore
valid in this standalone test process. Each parser has a 60-second budget.
Timing covers the parse API only, with no profiling and no warmup; grammar load
is separate. Fixed fixture order means later fixtures can use upstream pools.
One observation per fixture/cell supports no statistical speed claim.

This module is excluded from root `go test ./...` by Go's module boundary.
