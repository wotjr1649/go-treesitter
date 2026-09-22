# Release-Critical status board

Updated 2026-09-23, Session 07. **Release is BLOCKED_EXTERNAL.** Development,
Integration and the retained seven-route Release-Critical correctness gates
pass. This is finite-corpus E5 evidence, not proof for all source programs or
all 206 grammars. [Final review](../../artifacts/session-07/full-pass/11-release/review.md).

The tested product source commit is `23d6885ebe8709616614c89ffb6511ea815fb23f`.
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
reasons are enumerated in `11-release/fallbacks.json`.

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
| Windows ARM64 | CGO=0 cross-link PASS; native execution NOT_RUN. |
| Linux AMD64, Darwin ARM64, wasip1/wasm | CGO=0 cross-link PASS; execution NOT_RUN and outside first-release native scope. |
| Hosted CI | NOT_RUN. Native AMD64/ARM64 jobs are prepared; origin is not configured and push authority/target is pending. |
| License release gate | BLOCKED_EXTERNAL: Brightscript and Cooklang conflict between ISC and MIT declarations. |

Own code remains MIT. The optional GPL module contains caddy, disassembly and
jq with source archives and notices. All 206 inventory rows have identified
license evidence, but the two conflicting declarations need rights-holder
clarification before release. A local candidate ZIP is not publication approval.

The initial version remains `v0.0.1`. No tag, release, push or remote write was
performed. MemoryBudgetBytes covers parser-managed accounting, not total process
RSS; grammar caches, source copies, snapshots and concurrent workers require
separate application budgets. The independent private-diff review limitation is
documented in `09-audit/review.md` rather than reported as a completed review.
