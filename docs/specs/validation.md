# Validation contract

Normative. Owns lanes, gates, evidence levels, claim vocabulary, failure policy, and the
Definition of Done.

## Lanes

| Lane | Build | Decides product gates? | Purpose |
|---|---|---|---|
| **product** | `CGO_ENABLED=0`, Windows native | **yes** | The only lane that can pass or fail a product gate. |
| **race** | CGO + `-race` | no | Data-race and concurrency diagnostics. |
| **engine CI** | `CGO_ENABLED=0` on hosted Linux/macOS/Windows, amd64 and arm64 (`.github/workflows/engine.yml`) | yes, for the platform it ran on | Execution evidence per platform; a platform is claimed only after this lane ran there. A pull request runs Linux amd64 and Windows amd64 (and the Go 1.26 floor on the root and shard 1); a push to `main`, a dispatch and a change to `engine.yml` run all five platforms and the floor on every shard, so the evidence for Linux arm64, macOS arm64 and Windows arm64 is the `main` run of the commit, not the pull request run. |
| **API guard** | `CGO_ENABLED=0` on hosted Linux amd64 (`.github/workflows/api-guard.yml`, `tools/apiguard`) | no | Fails on incompatible root Go API changes between the latest released root tag (`API_BASELINE_TAG` in the workflow, moved at each root release by the post-release PR) and the checked-out commit, using pinned `apidiff`. `apidiff` does not report a field added to or reordered in a struct whose fields are all exported (other packages can write it as an unkeyed literal); `tools/apiguard/structliterals` checks that field list against the baseline. The root tests in the product and engine CI lanes check the documented identifier inventory (the set of exported identifiers, fields and methods, not their signatures, values or classes) for the root and `experimental/match`. |
| **cross** | `CGO_ENABLED=0`, other `GOOS/GOARCH` | scope-limited | Link-only proof that no C toolchain is required. |
| **native reference** | C compiler, same runtime release and grammar sources (ADR-0020), with the engine's pinned wasi-libc character semantics; `tools/native-ref` | yes, for C-agreement claims | Produces the reference dumps. See `docs/specs/oracle.md`. |
| **conformance** | `CGO_ENABLED=0`, `tools/conformance` with reference data: local (`TSG_S15_DIR`) or built in CI from the pinned grammars' own `test/corpus` plus seeded edits (`.github/workflows/conformance.yml`, `TSG_DATA_DIR`) | yes, for C-agreement claims on the inputs used | Byte-for-byte comparison of the public API's dumps, edits and changed ranges with the native reference. The strict field-lookup shard also compares `ChildByFieldName` on fresh corpus trees and on C's incremental tree for every seeded edit; `TSG_REQUIRE_FIELD_LOOKUP=1` fails missing prerequisites or zero inputs. Per post-generation scanner patch, it also compares the native build before and after the patch over the pinned corpus, fixed fuzz and shared regression inputs against `tools/gen/patches/<grammar>/scanner-expectations.json` (ADR-0023 decision 4; `E4` for those inputs on hosted Linux amd64/arm64). The CI job compiles the native reference with the runner's gcc from the pinned sources, or restores that build from a cache that only a push to main writes: the key holds the runner image version, compiler and Go versions, the group's grammars and the content of `pins.json`, `tools/native-ref`, the patches and the packages native-ref imports, and the workflow file, and `tools/native-ref/cachesums.sh` rejects a restore whose files differ from the recorded sha256 (the build then runs). A restored build is the same build, bound to the run and commit the job summary names; it is not a different oracle. |
| **bench** | `CGO_ENABLED=0`, `tools/bench` paired with native executables | yes, for resource gates | Speed and memory gates on the product build. |
| **nightly diff** | `CGO_ENABLED=0` on hosted Linux amd64/arm64; native reference from the pins (`.github/workflows/nightly-diff.yml`, `TestNightlyDiff`) | no, finds regressions | Daily time-budgeted random mutations and edits of the CI corpora, compared byte-for-byte with the native reference (dumps, incremental dumps, changed ranges), plus a short run of the Go fuzz targets. A mismatch uploads a shrunk reproducer with seed and commands. It is `E5` evidence only for the cases it ran, bound to the date seed and commit; absence of a finding is not parity. |
| **bench CI** | `CGO_ENABLED=0` on hosted Linux amd64/arm64, macOS arm64 and Windows amd64/arm64 (`.github/workflows/bench.yml`, `tools/bench/ci`) | yes, for the platform it ran on | Weekly paired-window speed and memory measurement against the native executables on sha256-pinned large inputs and the CI corpora, with the criteria of the Release gate (private bytes on macOS: the ADR-0022 macOS amendment gate, reported next to the raw delta; a missing measurement field there is a FAIL). On macOS, bench CI measures engine-region counts with `vmmap`. On Windows, the same criteria apply without adjustment: private bytes from `GetProcessMemoryInfo` (`PrivateUsage`) and engine regions from `VirtualQuery` (private reservations of at least 512 MiB that were not present at process start), as in the local Windows measurement; the native reference is the runner's MinGW `gcc -static` build on Windows amd64 and MSYS2 CLANGARM64 `clang -static` on Windows arm64 (native-ref `-cc`; the arm64 image's gcc is x86_64), and the job fails when that compiler does not target the runner's architecture. The arm64 clang is installed from the rolling MSYS2 repository at run time and is not version-pinned, so each run's `summary.md` records the compiler's `--version` line and target; the amd64 (gcc) and arm64 (clang) ratios have different native denominators and are judged only against their own platform's criteria, never compared with each other. The `-static` link and the 60-minute limit on windows-11-arm are unmeasured until the first `workflow_dispatch` rehearsal, which is not an RC bench run (record it as a rehearsal and keep its failures). A criterion the platform cannot measure is `NOT_RUN`. Shared runners: ratios only, never absolute times. |
| **upstream watch** | none (`.github/workflows/upstream-watch.yml`, `tools/native-ref/upstreamwatch`) | no | Weekly comparison of `tools/gen/pins.json` with newest stable runtime/tool releases (a newer one fails), plus repository health for all source/tool pins: rename/transfer, archive, default-head age >12 months and latest release are reported; missing repositories or pins unreachable from every branch and published tag fail the job; a containing default branch, release tag or other branch is named in the report. Rename, archive and age are warnings. It never changes a pin. |
| **nightly CLI rehearsal** | none, hosted Linux amd64 (`.github/workflows/nightly-cli-rehearsal.yml`, `gen nightly-rehearsal`) | no, report only | Weekly regeneration of every grammar with the unpinned tree-sitter `nightly` CLI beside the pinned CLI, from the same inputs and patches: which generated files differ, the node-types schema diff summary, the nightly CLI identity. It is `E2`-level evidence bound to the recorded nightly identity and run; it never gates, never moves a pin and does not run the conformance corpus. See ADR-0027, "Nightly rehearsal". |

**Change detection.** Each PR-required workflow (engine, regen, conformance, windows, API guard, pin move) starts with a
`<name>-changes` job that classifies the pull request's changed files (`tools/lane/ci_changes.py`) and skips the jobs the
change cannot affect; a job skipped that way passes its required check. Unknown paths, an empty diff, a change to the workflow
itself or to the classifier (the classifier of the base commit is the one that runs, so a pull request cannot switch its own
classification off), and every event except `pull_request` run the whole workflow. Documentation is not skipped
blindly: `TestKnownGap*` reads every tracked file and the root `TestAPIInventory` reads `docs/design/api-stability.md`, so a
pull request always keeps the engine job on Linux amd64 and conformance group g1 on Linux amd64. The required checks are the
six gates (`engine-gate`, `regen-gate`, `conformance-gate`, `windows-gate`, `api-guard-gate`, `pin-move-gate`): a gate fails
when its `changes` job or any job it needs failed or was cancelled, or when a job the classifier expected to run was skipped (a
misspelled condition); every `changes` job also runs the classifier's unit tests. A green gate on a pull request says only that the lanes
the change could affect passed; the full matrix result of a commit is its `main` run, when that run was not superseded (Main push runs).

Rules:

- Never plan `CGO_ENABLED=0` together with `-race`; they are mutually exclusive by construction.
- A race-lane timeout or slowdown is a diagnostic finding, never a product-gate failure.
  Race instrumentation has been measured at roughly **14×** the product lane on the same input.
- Windows amd64 is Tier-1. Linux, macOS and arm64 are claimed only for platforms where the engine CI lane
  executed; otherwise they stay `NOT_RUN`.

## Nightly input watchdog

`TestNightlyDiff` gives each Go case (old/new fresh parses, incremental parse and changed ranges together) and each
native operation a **30 second wall-clock limit**, including process startup. The Go case runs in a disposable test
binary process: a scanner that never polls the context can therefore be killed. The native executable has the same
process watchdog. The supervisor waits for process exit before proceeding. `TSG_NIGHTLY_INPUT_TIMEOUT` sets the
limit; the workflow pins it to `30s` and records it in each summary/reproducer.

The bound was selected from the wave-2 batch 2 (2026-11-14/15) Windows amd64 product/native measurements at base `2f9e000` plus batch2-v1:
the maximum corpus per-input median was **0.7535 ms** (Dart, `flutter-002.in`, engine), the maximum large-input
individual sample was **925.9276 ms** (Ruby, `gb18030-tbl.rb`, engine), and the maximum native edit process duration
was **3006.6 ms** (Ruby, `expressions-068-e1`, including process startup). Thirty seconds gives about 10x the largest
observed operation; nightly mutations can still expose slower inputs, which are failures to investigate, not reasons
to silently raise the limit. These measurements select a diagnostic bound and do not claim hosted timing evidence.

A timeout is a FAIL for that grammar. It saves the exact original old/new bytes, edit offsets, side, limit, seed and
replay command under `$TSG_CONF_OUT/nightly/`; it does not shrink or retry a hung case. The next grammar still runs
(the workflow matrix also has `fail-fast: false`), and failure artifacts retain the input. This watchdog bounds the
nightly differential comparisons; `ciconf -inputs-only` extracts their seeds without unsupervised preflight parses.
Go fuzz targets retain their separate fuzz/test budgets.

The 2026-11-15 fwcd Kotlin run also stayed within this diagnostic bound: its largest large-input sample was
3.702327 s (`_Arrays.kt`, engine), leaving about an 8.1x margin under 30 s. The nightly mutation inputs
are bounded by its existing `maxCase` policy; this observation does not change any resource gate.

## Evidence levels

Every claim carries a level. Do not state a claim above the level you actually produced.

| Level | Name | What it is |
|---|---|---|
| `E0` | assertion | Someone said so. **Never sufficient for any claim.** |
| `E1` | static inspection | Read the source, spec, or a recorded artifact. |
| `E2` | focused probe | A one-off executable check, not retained in the repository. |
| `E3` | retained test | A test in this repository that re-runs in the product lane. |
| `E4` | controlled comparison | A/B with exactly one variable changed and all identities bound. |
| `E5` | C reference differential | Compared byte-for-byte with native C reference dumps of the same runtime release and grammar sources, built with the engine's pinned wasi-libc character semantics. |
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

| Gate | Passes when | Known blockers in pinned sources allowed? |
|---|---|---|
| **Development** | Product lane builds; focused tests for changed behavior pass; every known-regression signature still matches its record. | **yes**, if ratcheted |
| **Integration** | Development gate, plus the result contract is exercised end to end and a consumer can depend on the API shape. | yes, if documented in the consumer-facing limitations |
| **Release-Critical** | For each language *claimed in scope*: zero unresolved correctness blockers; conformance (fresh and edit cases) agrees with the native reference; every `incremental != fresh` case is a ratchet attributed to the pinned source. | **no** |
| **Release** | Release-Critical for the claimed scope, plus platform validation, reproducible generation, resource budgets (large-input parse geomean <= 2.5x native; small files <= 1 ms or <= 2.5x native, where a file failing the first pass gets one paired confirmation pass and fails only if both fail, with both passes recorded; peak <= 100 x input + 64 MB; after `Close` every engine region returned and private bytes within start + 16 MB, ADR-0022; on macOS the footprint delta is net of the growth of the runtime's released heap bytes and a snapshot case must have been collected, ADR-0022 macOS amendment), packaging, and an explicit limitations list. | **no** |

A language with an open known regression keeps the **Development gate green** and the
**Release-Critical gate red for that language**. Both statements are true at once; say both.

`development baseline usable` ≠ `release approved`. Never compress that into "it works".

## Root RC and 1.0 scope

[ADR-0028](../design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md#amendment-2026-11-25-decisions-1-and-3-root-scope-rc-bench-rc-exit-and-windows-resources)
decides these rules for the root RC and 1.0.

- **Scope.** The Release-Critical and Release gates cover all 25 grammar modules. Hosted engine and bench CI
  cover Linux amd64/arm64, macOS arm64 and Windows amd64/arm64. Hosted conformance covers Linux amd64/arm64 (on `main`; a pull request that changes nothing the lane builds runs g1 on amd64).
  Grammar known gaps stay documented limits. A grammar's resource-budget failure fails the root gate. Only an
  owner-recorded per-case exception narrows the scope; the v0.7.0 Preview macOS kotlin exception covers
  that Preview only.
- **RC bench.** Run the bench CI exactly once on the RC candidate commit, and declare that run before it
  starts. A failure is kept and classified. Fix it and cut a new RC, or the owner decides a new per-case
  exception. The Release gate criteria stay as written above.
- **Windows resources.** Bench CI includes hosted Windows amd64 and arm64. For the RC, also run the Windows
  amd64 local bench of all 25 grammars on the same candidate commit.
- **RC exit.** A change to root code, contract or inputs during an RC cuts a new RC and repeats the affected
  gates. Otherwise 14 days without root defect reports lead to 1.0.

## Known-regression ratchet

Not an `xfail`. A loose skip hides both fixes and regressions in the pinned source.

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

A push to a PR may cancel superseded automatic `pull_request` runs of the same workflow for that PR (engine, regen,
conformance, windows, API guard and pin move alike). Scheduled, dispatched, re-run and release runs are exempt, and so is a
main push run that is executing: a newer push never cancels it, it only replaces the run waiting behind it (below). Keep
completed results under their original commit. Work canceled before execution is `NOT_RUN`.

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

### Main push runs

Pushes to `main` of the six workflows with a `main` trigger (engine, regen, conformance, windows, API guard, pin move) share
one concurrency group per workflow (`<workflow>-main`, first attempt only) with `cancel-in-progress: false`. A run that is
executing or waiting for runners always finishes; behind it only the newest push waits, and GitHub cancels an older waiting run
when a newer push arrives. Merging several pull requests in a row is therefore allowed, and the merge commits in between are
`NOT_RUN` by design. Re-runs and dispatches keep their own group and are never replaced.

- **Per workflow.** A superseded run makes its commit `NOT_RUN` for that workflow only; the other workflows may have run on it. The
  full matrix result of a commit means a completed first-attempt push run of all six workflows on that commit.
- **Never cited.** A `NOT_RUN` commit is not called validated, tested or in agreement with anything. A completed run on a later
  commit is evidence for that commit's tree, not for the commits it skipped.
- **Failures keep their range.** A failed run on commit T fails every commit after the previous green run of that workflow up
  to T (`git log --first-parent <green>..T`), none of which is recorded as passing. Find the commit by dispatching the workflow on
  a branch at a suspect merge commit (dispatched runs are exempt); never re-run until green.
- **Record.** `tools/lane/main-coverage.sh [COUNT [REF]]` prints each workflow's push-run result per first-parent commit (`NOT_RUN`
  for no run or a cancelled one). A claim that depends on a range copies that table into its receipt, and release evidence into
  `artifacts/release-candidate/`.
- **Release candidate.** Evidence for a candidate commit C is a completed first-attempt push run of each of the six workflows
  on C itself, with the job list the declaration names. Merge nothing after C until they finish; nothing then replaces them. If a
  run on C was superseded or `main` moved past C, push a branch at C (it triggers nothing) and dispatch each missing workflow
  with `--ref <branch>`: a dispatched run executes the whole workflow on C, and the declaration records it as a dispatch.

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

Never edit old records to drop a reference. The native reference executables are rebuilt on demand from the pins by
`tools/native-ref`; the conformance and bench lanes read their local data tree (corpus, edits, large inputs, reference
dumps) from `TSG_S15_DIR`, which is not tracked, so do not delete it without copying its evidence first. The
pre-ADR-0022 tools (`.scratch/oracle`, `tools/native-oracle`, `tools/release`, `tools/runtime-bundle`) were removed;
they remain in Git history at `9ad82eb`.

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
