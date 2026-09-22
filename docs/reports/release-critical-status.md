# Release-Critical status board

Current state, not a promise. Gate definitions: `docs/specs/validation.md`.
Baseline: unchanged, see `docs/specs/baseline-provenance.md` and identities.json.
Last updated: 2026-09-22, Session 07.

Development and Integration are green with exact known-difference ratchets.
The public constructor and external-module consumer now exercise the result
contract end to end. Release-Critical and Release have not passed.

| Language | Development | Release-Critical | Current limitation |
|---|---|---|---|
| Go | green | not fully assessed | Registered fresh/edit snapshots agree with C for represented fields; finite corpus only. |
| Python | green | not fully assessed | Registered fresh/edit snapshots agree with C; broader release corpus pending. |
| JavaScript `.js` | green | not fully assessed | Registered fresh/edit controls agree; JSX text limitation is listed separately. |
| TypeScript `.ts` | green | not fully assessed | KR-0003 retired after explicit full-patch Candidate C adoption; registered fresh/edit trees agree with the new C artifacts. |
| JSX | green, ratcheted | **red** | KR-0001b `=` defect remains in the product runtime. |
| TSX | green, ratcheted | **red** | KR-0001b remains; KR-0003 is retired. |
| C# | green, ratcheted | **red** | KR-0002 recovered-tree difference and KR-0004 false-clean source reconstruction. |

All seven routes remain in scope. No blocked language was removed to pass a
gate. Bare `&` remains erroneous in C and Go (KR-0001a); making it clean would
break the characterization. No language has passed the full release assessment.

## Independent phase evidence

| Session 07 phase | What it establishes |
|---|---|
| Phase 1 | Public API and actual external consumer, E3. |
| Phase 2 | Fresh 94-input baseline/candidate/C comparison; four `=` inputs corrected only in the isolated scanner candidate, 90 snapshots unchanged. New KR-0003/4 retain their own C records and signatures, E5. C# diagnostic patches remain unsuitable for adoption. |
| Phase 3 | Retained input/snapshot/work/memory admission limits, cancellation, ownership and worker tests; bounded fuzz and race diagnostics remain separately labeled. |
| Phase 4 | 35 additional fresh C records and 21 incremental/fresh/C comparisons, E5; independent ABI/epoch/input checks reject coherent stale metadata. |
| Phase 5 search | Optional immutable position/type index and five-sample lookup measurements on the declared Go workload, E4. |
| Phase 5 bundle | Existing selected-grammar tags reduce one measured executable's size; C# parse cost remains high. |
| Phase 5 runtime | Separate duplicate-cost-call candidate: measured allocation reduction; unchanged snapshots on its fixed 85-case set. It was not combined with the scanner patch or adopted. |
| Phase 6 | MIT/notices, product graph and ordinary PE checks, consumer execution and CGO-free cross-links. Other native targets and remote CI remain NOT_RUN. |

These are separate observations. A candidate or measurement does not inherit a
different phase's result. Full commands, input hashes, failed attempts and limits
are under `artifacts/session-07/` in the named phase.

## Platform and packaging

| Target | State |
|---|---|
| Windows AMD64 | CGO=0 build, consumer execution, default/subset regression and vet passed, E3. Product graph excludes runtime/cgo; ordinary PE imports kernel32.dll only. |
| Windows ARM64 | Ordinary CGO=0 executable linked; native execution NOT_RUN. |
| Linux AMD64, Darwin ARM64, wasip1/wasm | Ordinary CGO=0 executables linked; execution NOT_RUN; outside first-release platform scope. |
| Hosted CI / Linux C container | NOT_RUN. Windows native C is the executed oracle transport. |

Own code is MIT. Notices cover the runtime, six assessed grammars, C oracle and
licensed corpus. The broader upstream default aggregate grammar catalog still
needs a complete notice inventory for release; the selected assessed bundle is
documented. Memory admission is not a hard process-RSS cap. Grammar caches,
source copies, snapshots and aggregate worker memory need application budgets.

Prefer an upstream contribution for the small scanner fix. The C# scheduler and
reconstruction changes require further upstream correction. TypeScript grammar
patch reconciliation was explicitly approved and is recorded in ADR-0012. Neither a remote
fork nor a product runtime replacement was made. A local Go replace directive
would not propagate to consuming modules. No release is approved.

Update this board when a gate changes, a record is added/retired, or the baseline
moves. Individual test runs belong in phase evidence.
