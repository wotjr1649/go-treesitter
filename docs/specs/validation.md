# Validation contract

Normative. Owns lanes, gates, evidence levels, claim vocabulary, failure policy, and the
Definition of Done.

## Lanes

| Lane | Build | Decides product gates? | Purpose |
|---|---|---|---|
| **product** | `CGO_ENABLED=0`, Windows native | **yes** | The only lane that can pass or fail a product gate. |
| **race** | CGO + `-race` | no | Data-race and concurrency diagnostics. |
| **engine CI** | `CGO_ENABLED=0` on hosted Linux/macOS/Windows, amd64 and arm64 (`.github/workflows/engine.yml`) | yes, for the platform it ran on | Execution evidence per platform; a platform is claimed only after this lane ran there. |
| **cross** | `CGO_ENABLED=0`, other `GOOS/GOARCH` | scope-limited | Link-only proof that no C toolchain is required. |
| **native reference** | C compiler, same runtime release and grammar sources (ADR-0020) | yes, for C-agreement claims | Produces the reference dumps. See `docs/specs/oracle.md`. |
| **conformance** | `CGO_ENABLED=0`, `tools/conformance` with the local reference data | yes, for C-agreement claims | Byte-for-byte comparison of the public API's dumps with the native reference. |
| **bench** | `CGO_ENABLED=0`, `tools/bench` paired with native executables | yes, for resource gates | Speed and memory gates on the product build. |

Rules:

- Never plan `CGO_ENABLED=0` together with `-race`; they are mutually exclusive by construction.
- A race-lane timeout or slowdown is a diagnostic finding, never a product-gate failure.
  Race instrumentation has been measured at roughly **14×** the product lane on the same input.
- Windows amd64 is Tier-1. Linux, macOS and arm64 are claimed only for platforms where the engine CI lane
  executed; otherwise they stay `NOT_RUN`.

## Evidence levels

Every claim carries a level. Do not state a claim above the level you actually produced.

| Level | Name | What it is |
|---|---|---|
| `E0` | assertion | Someone said so. **Never sufficient for any claim.** |
| `E1` | static inspection | Read the source, spec, or a recorded artifact. |
| `E2` | focused probe | A one-off executable check, not retained in the repository. |
| `E3` | retained test | A test in this repository that re-runs in the product lane. |
| `E4` | controlled comparison | A/B with exactly one variable changed and all identities bound. |
| `E5` | C reference differential | Compared byte-for-byte with native C reference dumps of the same runtime release and grammar sources. |
| `E6` | release gate | Full lane set, provenance, budgets, packaging, and limitations. |

### Claim vocabulary

| To write… | You need |
|---|---|
| "builds on Windows without CGO" | `E3` |
| "parses this fixture cleanly" | `E3` |
| "incremental equals fresh" | `E3` (and say it is self-consistency, not parity) |
| "faster / slower than X" | `E4` with identity binding and stated conditions |
| "matches the C oracle" / "parity" | `E5` |
| "proven" / "verified" | `E5` or `E6`, and name the level inline |
| "release-ready" | `E6` |

`E2` may support a decision. It may not support a durable claim in a contract document.

## Gates

| Gate | Passes when | Known upstream blockers allowed? |
|---|---|---|
| **Development** | Product lane builds; focused tests for changed behavior pass; every known-regression signature still matches its record. | **yes**, if ratcheted |
| **Integration** | Development gate, plus the result contract is exercised end to end and a consumer can depend on the API shape. | yes, if documented in the consumer-facing limitations |
| **Release-Critical** | For each language *claimed in scope*: zero unresolved correctness blockers; conformance (fresh and edit cases) agrees with the native reference; every `incremental != fresh` case is an upstream-attributed ratchet. | **no** |
| **Release** | Release-Critical for the claimed scope, plus platform validation, reproducible generation, resource budgets (large-input parse geomean <= 2.5x native; small files <= 1 ms or <= 2.5x native; peak <= 100 x input + 64 MB; release to start + 16 MB), packaging, and an explicit limitations list. | **no** |

A language with an open known regression keeps the **Development gate green** and the
**Release-Critical gate red for that language**. Both statements are true at once; say both.

`development baseline usable` ≠ `release approved`. Never compress that into "it works".

## Known-regression ratchet

Not an `xfail`. A loose skip hides both "upstream fixed it" and "upstream made it worse".

Record format and current entries: `docs/validation/known-regressions.md`.

Behavior on a run:

```
signature matches the record      -> known blocker; Development gate stays green
failure disappears                -> STALE: investigate, then retire the record. Do not delete silently.
signature changed                 -> NEW REGRESSION: fail. This is not the known issue.
an additional fixture now fails   -> REGRESSION EXPANSION: fail.
a fixture in the record is missing -> provenance failure: fail before interpreting anything.
```

Never widen a record to absorb a new failure. Open a new record.

## Failure and iteration policy

| Situation | Required action |
|---|---|
| Focused test fails because of your change | Diagnose, fix, rerun. Loop without asking. |
| Test fails with a recorded known signature | Record the observation. Do **not** weaken the test. |
| Signature differs from the record | Treat as a new regression. Stop and report. |
| Identity mismatch (hash, version, blob, lane) | Stop the comparison. Repair provenance before interpreting. |
| Required environment unavailable | Mark `NOT_RUN` / `BLOCKED`. Never substitute a weaker check. |
| A measurement is noisy | Report the spread. Do **not** re-run until it looks good. |
| A local test times out on a loaded host | Keep the failure. Record the commit, Go version, command, measured CPU load and output with `tools/lane/record.py`, then compare on the same commit with `go test -p 1` for a declared number of runs. A hosted pass does not erase the local failure. |
| Three attempts, same goal, no new evidence | Stop. Change layer, or report the blocker. |

Declare the number of runs **before** running. A benchmark is never repeated until it passes.

## Scratch retention

`.scratch/` is disposable working space, not an evidence store. When a run is created, record its role
(failure, intermediate comparison, or adoption candidate) in its receipt. Before a session closes, copy
into the local `artifacts/handoff/`, with hashes, anything that a claim, handoff, or release depends
on. When a historical record names a `.scratch` path, it says where something ran, not where it is kept.

To delete `.scratch` content:

1. Confirm that no live session or process uses it.
2. Copy any unique evidence into `artifacts/handoff/` and verify its hash. Publish only reviewed
   projections, under `artifacts/release-candidate/` (`docs/specs/publication.md`).
3. Add a cleanup ledger to `artifacts/handoff/` that lists each old path, its kept location or `deleted`,
   its SHA-256, and the reason.

Never edit old records to drop a reference. Live tools read `.scratch/oracle`
(`tools/native-oracle/run.py`), so do not delete it without re-running `prepare`.
Other tools (`tools/release/check.py`, `tools/runtime-bundle/check.py`, CI) recreate their
`.scratch` content on demand.

## Definition of Done

### Work unit done

```
[ ] the scoped change is implemented, and nothing outside the scope changed
[ ] focused tests for the changed behavior pass in the product lane
[ ] required regression tests pass
[ ] every known-regression signature still matches its record
[ ] the diff was reviewed against docs/reviews/review-checklist.md
[ ] provenance unchanged, or intentionally changed with an ADR
[ ] contract docs updated if and only if a contract changed
[ ] evidence recorded: commands, inputs, environment, pass/fail, not-run, limitations
[ ] one scoped local commit created, with what-was-run in the body
```

### Session done

```
[ ] every authorized work unit is work-unit-done, or explicitly deferred with a reason
[ ] the cross-work-unit regression set passed
[ ] working tree state is known and intentional (clean, or every remaining file explained)
[ ] no unauthorized change to _ref, code-map-memo, baseline, oracle, or release scope
[ ] a handoff exists in artifacts/handoff/ with commit SHAs
[ ] exactly one completion verdict is stated
```

### Completion verdicts

```
PASS                      every authorized unit done, all gates green
PASS_WITH_KNOWN_BLOCKERS  all gates green except ratcheted known regressions
BLOCKED_EXTERNAL          an environment or decision outside this repository blocks progress
FAILED_REGRESSION         a gate that was green is now red
PARTIAL_NOT_ACCEPTED      work stopped mid-scope; state exactly where and why
```

Never invent a verdict, and never report `PASS` with an unexplained skipped check.
