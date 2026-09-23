# Release-Critical status board

Updated 2026-09-23, Session 07. **Release is BLOCKED_EXTERNAL.** Development,
Integration and the retained seven-route Release-Critical correctness gates
pass. This is finite-corpus E5 evidence, not proof for all source programs or
all 206 grammars. [Selected evidence](../../artifacts/release-candidate/README.md).

The historical measured product source commit is `23d6885ebe8709616614c89ffb6511ea815fb23f`.
The curated candidate preserves all 1,807 files in that production identity.
Runtime origin remains `gotreesitter v0.53.0`; the internal carrier manifest is
`5d1f526c35e223341914cbfada8abe1c4c49d52824a09950bde05d331ff41e46`.
The Go product blob now derives from the already pinned C tables (ADR-0015).
C runtime v0.25.1 and grammar base commits remain pinned. Candidate C's complete
TypeScript patch remains the approved C artifact (ADR-0012).

| Route | Retained Release-Critical gate | Limitation |
|---|---|---|
| Go | PASS, fresh/edit/C agreement | C-derived Go artifact has its own identity; generic compact admission is not a transferred certificate. |
| Python | PASS, fresh/edit/C agreement | Scanner-prefix checks may require an explicit fresh fallback. |
| JavaScript | PASS, fresh/edit/C agreement | Recovery can decline compact admission. |
| JSX | PASS, fresh/edit/C agreement | Bare `&` remains a C grammar error; the recovered tree now agrees exactly. |
| TypeScript | PASS, fresh/edit/C agreement | Maintained Candidate C patch is required for the pinned oracle artifacts. |
| TSX | PASS, fresh/edit/C agreement | Conservative reuse and recovery checks have a measured deletion cost. |
| C# | PASS, fresh/edit/C agreement | KR-0002/4 retired; the external scanner still uses an explicit full reparse on edits. |

KR-0001b, KR-0002, KR-0003 and KR-0004 are retired for their retained controls.
Original failures remain recorded. No active C-tree exception or unexplained
comparison mismatch remains in the executed final corpus. Controlled fallback
reasons are enumerated in `artifacts/release-candidate/historical/fallbacks.json`.

## Executed evidence

- 688 C records, 439 edited records; repeated fresh recovery observations and
  three native C executions per record. One additional 8,192-declaration Go
  edit agrees with C over all 57,348 ordered nodes.
- 206 basic grammar checks: 203 in the base module and 3 in the optional GPL
  module; three fresh runs, edit/fresh equality, cancellation, admission and
  close. This does not certify C parity for the entire catalog.
- Product and selected-grammar suites, vet, oracle-tool tests, four private
  runtime invariants, 66 race test/subtest results and two 60-second fuzz runs.
- 193 memory cycles / 3,281 closed trees. Post-GC heap stayed within
  83,966,168–83,967,832 bytes; peak RSS was 295,403,520 bytes on the measured
  Windows AMD64 host. This finite observation is not a hard RSS guarantee.
- Five randomized paired performance seeds plus longer lookup samples and
  profiles. Correctness has costs: TSX delete median +41.8%, Go replace +23.8%
  versus the earlier `2e18f90` baseline. Profiled avoidable raw-cost walks and
  conflict-driven stack copying were fixed; all samples remain recorded.
- Official Go module ZIP creation and checks, empty-cache consumers without
  replace, 100 external-consumer C comparisons, and separate GPL consumption.

## Platform and remaining release blockers

| Gate | State |
|---|---|
| Windows AMD64 native | PASS: CGO=0 build/tests/consumer execution. |
| Product dependency/PE audit | PASS: 137 dependency packages, zero CgoFiles, no runtime/cgo; ordinary PE imports kernel32.dll only. |
| Windows ARM64 | CGO=0 native build, vet, product/C-record tests and selected-grammar tests PASS in the first hosted run; the remaining native CI steps did not run after the oracle-tool test failure. |
| Linux AMD64, Darwin ARM64, wasip1/wasm | CGO=0 cross-link PASS; execution NOT_RUN and outside first-release native scope. |
| Hosted CI | First run FAILED in Python oracle-tool tests on both architectures because a fresh checkout had no `.scratch` directory. Race diagnostics passed. The initialization fix passes six Python tests. The curated candidate schedules full native packaging and catalog checks; inspect its exact-commit workflow result before claiming completion. |
| License release gate | BLOCKED_EXTERNAL: Brightscript and Cooklang conflict between ISC and MIT declarations. |

Own code remains MIT. The optional GPL module contains caddy, disassembly and
jq with source archives and notices. All 206 inventory rows have identified
license evidence, but the two conflicting declarations need rights-holder
clarification before release. A local candidate ZIP is not publication approval.

The initial version remains `v0.0.1`. The owner authorized publication of
`session/07-product-hardening` at `2494b35499a3b87281e6837de0f9820a28a6f7ec`.
[The first hosted run](https://github.com/wotjr1649/go-treesitter/actions/runs/35799931220)
records the native checks and the retained tooling failure. The independent
`validation/release-v0.0.1` candidate excludes the raw session/experiment history
and publishes technical contracts and selected evidence. Its local pre-commit
checks passed 911 product and 895 selected-grammar test/subtest results, six
oracle-tool tests, one packaging boundary test, four private runtime invariants
and vet. See [the receipt](../../artifacts/release-candidate/precommit.json).

The Windows workflow runs official module ZIP and empty-cache consumer checks
on both native architectures, including all 206 basic catalog cases against
the packaged modules. The independent license job intentionally remains failing
until the two upstream declarations are resolved. No gate is waived by another
job passing. Existing published session history is a separate disclosure surface;
the curated branch does not erase it. No main merge, tag or release was performed.

MemoryBudgetBytes covers parser-managed accounting, not total process
RSS; grammar caches, source copies, snapshots and concurrent workers require
separate application budgets. The independent private-diff review limitation is
retained in the local audit: the primary agent reviewed the owned diff;
independent public-source reviews did not review the private repository diff.
