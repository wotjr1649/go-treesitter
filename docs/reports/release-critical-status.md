# Release-Critical status board

Current state, not a promise. Updated when a gate moves; never used as evidence by itself.
Gate definitions: `docs/specs/validation.md`. Baseline: `docs/specs/baseline-provenance.md`.

**Last updated:** 2026-09-22 (Session 04) · **Baseline:** `gotreesitter v0.53.0` @ `c871b1f5`

## Per-language state

| Language | State | Development gate | Release-Critical gate | Notes |
|---|---|---|---|---|
| Go | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Compact route admits cleanly upstream (2 PASS / 0 FALLBACK). |
| Python | `READY_FOR_NEXT_GATE` | green | not yet evaluated | Incremental falls back to full reparse; correctness preserved. |
| JavaScript (non-JSX) | `READY_FOR_NEXT_GATE` | green | not yet evaluated | JSX text is a separate concern — see below. |
| TypeScript `.ts` | `READY_FOR_NEXT_GATE` | green | not yet evaluated | No certified runtime profile upstream; conservative defaults. |
| C# | `READY_FOR_NEXT_GATE` **with performance / fallback risk** | green | not yet evaluated | `KC-0001`, `KC-0002`, and open item `OI-0001`. |
| **JSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | `KR-0001b` (bare `=`, suspected Go-only divergence). `KR-0001a` (bare `&`) is C-faithful and blocks nothing on correctness grounds. |
| **TSX** | `BLOCKED_UPSTREAM` | green (ratcheted) | **red** | `KR-0001b`, same cause. |

`READY_FOR_NEXT_GATE` means "nothing known blocks moving to the next gate", **not** "passed".
No language has passed the Release-Critical gate; that gate has not been run from this repository.

## Blocked ≠ removed

JSX and TSX remain **in** the intended Release-Critical scope. They are blocked by an upstream
defect with a signature-matched ratchet. Removing them from scope would be a user-owned decision and
a contract change — it is not something an implementation session may do to get a gate green.

## Platform state

| Platform | State |
|---|---|
| `windows/amd64` | Tier-1. Build and seven-language smoke verified at the baseline. |
| `windows/arm64` | Cross-build links at the baseline. Not validated. |
| `linux`, `darwin`, `wasip1` | Out of first-release scope. Linked at an earlier commit only; **not** re-verified at the baseline. |

## Open items carried forward

| ID | Item | Owner |
|---|---|---|
| `KR-0001a` | JSX/TSX bare `&` → error tree. **C-faithful**; grammar requires `&…;` | `tree-sitter-javascript` grammar, not the runtime |
| `KR-0001b` | JSX/TSX bare `=` → error tree. Go-only heuristic with no C counterpart | upstream `#1242`. Evidence `E1`; needs `E5` |
| `KC-0001` | C# always declines the compact route | upstream (tracked in its own matrix) |
| `OI-0001` | `accepted` + `HasError` on the upstream C# excerpt | needs baseline reproduction here |
| — | Oracle lane has never been executed from this repository | this repository |
| — | `.gitattributes` does not exist yet; `core.autocrlf=true` | this repository, first work unit |

`KR-0001b` is the first and only item that has ever satisfied all three fork-trigger conditions.
Acting on it requires `E5` evidence first, and creating a fork remains a user-owned decision.

## Update rule

Update this file when a gate changes state, a ratchet is added or retired, or the baseline moves.
Do not update it to record a single test run — that belongs in an `artifacts/handoff/` document.
