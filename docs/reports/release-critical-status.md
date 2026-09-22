# Release-Critical status board

Current state, not a promise. Updated when a gate moves; never used as evidence by itself.
Gate definitions: `docs/specs/validation.md`. Baseline: `docs/specs/baseline-provenance.md`.

**Last updated:** 2026-09-22 (Session 06) · **Baseline:** unchanged, see identities.json.

## Per-language state

| Language | State | Development gate | Release-Critical gate | Notes |
|---|---|---|---|---|
| Go | `READY_FOR_NEXT_GATE` | green | not fully assessed | Phase 2 registered smoke tree equals C for represented fields, E5. |
| Python | `READY_FOR_NEXT_GATE` | green | not fully assessed | Phase 2 registered smoke tree equals C, E5; larger corpus pending. |
| JavaScript `.js` | `READY_FOR_NEXT_GATE` | green | not fully assessed | Phase 2 `.js` route equals C, E5; this smoke contains JSX. |
| TypeScript `.ts` | `READY_FOR_NEXT_GATE` | green | not fully assessed | Phase 2 smoke and generic-arrow controls equal C, E5. |
| C# | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | Phase 2: two corpus files equal C; excerpt recovery differs, KR-0002, E5. |
| **JSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | Phase 2 KR-0001b differential E5; product patch not adopted. |
| **TSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | Phase 2 KR-0001b differential E5; product patch not adopted. |

`READY_FOR_NEXT_GATE` means "nothing known blocks moving to the next gate", **not** "passed".
No language has passed the full Release-Critical gate. The 50-input oracle lane
is a foundation for broader fresh-corpus, incremental and recovery coverage.

## Blocked ≠ removed

All seven routes remain **in** the intended Release-Critical scope. The red languages are blocked by an upstream
defect with a signature-matched ratchet. Removing them from scope would be a user-owned decision and
a contract change — it is not something an implementation session may do to get a gate green.

## Platform state

| Platform | State |
|---|---|
| `windows/amd64` | Tier-1. Phase 5 CGO-free build/vet/tests passed, E3. |
| `windows/arm64` | CGO-free build passed; execution NOT_RUN. |
| `linux`, `darwin`, `wasip1` | Out of first-release scope; no Session 06 execution claim. Linux/container is NOT_RUN. |

## Open items carried forward

| ID | Item | Owner |
|---|---|---|
| `KR-0001a` | Bare `&` remains erroneous in both grammars' C and Go outputs, Phase 2 E5; recovered trees differ. | Do not make it clean. |
| `KR-0001b` | Product ratchet active. Separate Phase 3 candidate corrects four inputs with 46 other Go snapshots unchanged. | Product adoption remains separate. |
| `KR-0002` | C# excerpt: Go HasMissing=true/HasError=false; C has ERROR nodes and a different namespace end. Raw runtime reproduces Go shape. | Deeper recovery cause remains open. |
| `KC-0001` | Existing corpus-specific compact declines remain characteristics, not scanner-patch targets. | upstream |
| `OI-0001` | Earlier route-dependent HasError allegation remains unreproduced. Current C recovery difference has its own KR-0002 record. | upstream recovery investigation |
| — | WP2: 50 checked-in records, exact inventory/build/input identities and Windows product comparisons implemented. | broader release corpus remains WP4/WP5 |
| — | Phase 4 actually regenerates six grammars using a task-local CLI and compares 50 fresh C results per artifact set. | No generator release constant or epoch migration. |

Fork recommendation: retain the minimal patch candidate and its retirement plan,
prefer upstream contribution, and decide product adoption separately. An open issue
without replies does not establish that upstream is not tracking it. No remote
fork, push or product substitution occurred. Remote CI execution is NOT_RUN.

Evidence is phase-local under `artifacts/session-06/`. Phase 2 compares the
baseline, Phase 3 evaluates the isolated patch, Phase 4 compares generator
artifacts, and Phase 5 records the closing product regression. None implies
query/supertype metadata coverage or release approval.

## Update rule

Update this file when a gate changes state, a ratchet is added or retired, or the baseline moves.
Do not update it to record a single test run — that belongs in an `artifacts/handoff/` document.
