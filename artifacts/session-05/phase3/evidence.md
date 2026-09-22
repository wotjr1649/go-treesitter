# Phase 3 controlled comparison

Go 1.27.1, Windows/amd64, CGO_ENABLED=0, one observation per fixture/cell.
Four cells, three fixtures: exactly 12 timed parses. No warmup or profiler.
Each cell starts a new process, loads the grammar once, constructs one parser
per fixture and uses the same fixed fixture order. Only the parse API is timed.
See `conditions.json`, `measurements.json` and individual cell logs.

| Input | A: v0.52 default | B: v0.52 disabled | C: v0.53 default | D: v0.53 disabled |
|---|---:|---:|---:|---:|
| JsonPosition.cs | 288.039 ms | 290.443 ms | 398.214 ms | 377.923 ms |
| JsonTextReader-excerpt.cs | 124.900 ms | 126.656 ms | 187.623 ms | 149.936 ms |
| MathUtils.cs | 24.550 ms | 24.151 ms | 46.015 ms | 30.495 ms |

These are single-run observations (E4), not statistical speed conclusions.
Main and nested module pins were not changed. All corpus and license hashes
matched; the actual C# blobs are identical for both versions. Raw compact
decline strings are retained verbatim. A/C decline to classic on all three
inputs; B/D use classic directly. All 12 parses finish accepted with full spans.

OI-0001's `HasError=true` was not reproduced: every cell reports false. The
excerpt does contain a missing node in every cell and therefore has outcome
`accepted_with_errors`. A separate, untimed adapter diagnosis (one parse)
locates the missing semicolon at byte 335, row 6 column 26, after `ReadType`.
That receipt is now covered by `TestMissingNodesCannotLookClean` (E3). C# oracle
diagnosis remains open; no correctness conclusion is inferred from false HasError.

The initial C invocation stopped before parsing: the test's runtime working
directory made the metadata subprocess find the nested module. A discriminating
`go list -m` check identified that cause. C/D then explicitly used the unchanged
main go.mod via GOFLAGS. Harness source is identical in every measured cell;
A/B were not repeated. The failed preflight log is preserved.

Copying the excerpt from the module cache preserved its read-only attribute.
An attempted normalization write was denied. No control was changed or bypassed;
the copied bytes already matched the exact LF hash. Its line 136 trailing space
is deliberately preserved because it is part of the registered fixture identity.

Review: no remaining implementation findings. Scoped module isolation, fixture
identity, sequential counter attribution, lifetime, bounds, and all 12 receipts
checked. Main dependency binding passed again after the comparison. No phase's
smoke or C-probe results are used to support this phase's conclusions.

NOT_RUN: repeated/statistical benchmarks, memory budgets, race, C# C differential,
remote CI and other platforms. No baseline, oracle-runtime or scope change.
