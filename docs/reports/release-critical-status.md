# Release-Critical status board

Current state, not a promise. Updated when a gate moves; never used as evidence by itself.
Gate definitions: `docs/specs/validation.md`. Baseline: `docs/specs/baseline-provenance.md`.

**Last updated:** 2026-09-22 (Session 05) · **Baseline:** `gotreesitter v0.53.0` @ `c871b1f5`

## Per-language state

| Language | State | Development gate | Release-Critical gate | Notes |
|---|---|---|---|---|
| Go | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Phase 1 smoke and incremental self-consistency, E3. |
| Python | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Phase 1 smoke/self-consistency, E3; full-reparse reason retained. |
| JavaScript (non-JSX) | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Phase 1 SM-JS fixture, E3; broad non-JSX corpus not run. |
| TypeScript `.ts` | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Phase 1 smoke/self-consistency, E3. |
| C# | `READY_FOR_NEXT_GATE` with open missing-node investigation | green | not yet evaluated | Phase 1 smoke/self-consistency, E3. Separate Phase 3 finding below. |
| **JSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | Phase 2 KR-0001b Go signature retained at E3; C-cause evidence E1. |
| **TSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | Phase 2 KR-0001b F4/F5 differential E5; patch not applied. |

`READY_FOR_NEXT_GATE` means "nothing known blocks moving to the next gate", **not** "passed".
No language has passed the Release-Critical gate; that gate has not been run from this repository.

## Blocked ≠ removed

JSX and TSX remain **in** the intended Release-Critical scope. They are blocked by an upstream
defect with a signature-matched ratchet. Removing them from scope would be a user-owned decision and
a contract change — it is not something an implementation session may do to get a gate green.

## Platform state

| Platform | State |
|---|---|
| `windows/amd64` | Tier-1. Phase 1 CGO-free build/vet/tests passed, E3; Phase 4 has its own regression receipt. |
| `windows/arm64` | CGO-free build passed; execution NOT_RUN. |
| `linux`, `darwin`, `wasip1` | Out of first-release scope; no Session 05 build or execution claim. |

## Open items carried forward

| ID | Item | Owner |
|---|---|---|
| `KR-0001a` | Bare `&` remains erroneous. Phase 2 TSX C and Go error states agree; recovered trees differ. Do not make it clean. | grammar usability limitation; no runtime correction authorized |
| `KR-0001b` | Bare `=`: TSX F4/F5 C clean, Go error (Phase 2 E5). JavaScript C comparison remains NOT_RUN. | upstream `#1242`; proposed diff is an unapplied artifact |
| `KC-0001` | Phase 3 corpus compact attempts decline; no claim about every C# input | upstream |
| `OI-0001` | Phase 3 2×2 did not reproduce HasError=true. Excerpt has missing `;` at byte 335 in all cells. | C# oracle diagnosis remains open |
| — | Native TSX diagnostic probe completed. Full corpus/digest oracle lane remains unbuilt. | next work package WP2 |
| — | Generator selection is dynamic; actual artifacts retain producer/ABI/hash receipts (ADR-0007). | no global installation or automatic epoch upgrade |

`KR-0001b` is the first and only item that has ever satisfied all three fork-trigger conditions.
TSX F4/F5 now have E5 evidence. Creating a fork or applying a patch remains a
user-owned decision. Development completion is not release approval.

Evidence is phase-local under `artifacts/session-05/phase1/` through `phase4/`.
No claim combines smoke, C differentials and timings into one conclusion.

## Update rule

Update this file when a gate changes state, a ratchet is added or retired, or the baseline moves.
Do not update it to record a single test run — that belongs in an `artifacts/handoff/` document.
