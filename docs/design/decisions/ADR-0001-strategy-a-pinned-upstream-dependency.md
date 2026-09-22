# ADR-0001 — Strategy A: pinned upstream dependency, fork as escalation

**Status:** Accepted · 2026-09-22

## Context

Four strategies were compared with measured evidence
(`artifacts/handoff/2026-09-22-feasibility-oracle-architecture-review.md`):

| Strategy | Initial surface | Upstream sync | Verdict |
|---|---|---|---|
| A — pinned dependency | 0 | none | chosen |
| B — managed minimal fork | low | **high** (~431 commits/week upstream) | escalation only |
| C — hard fork | high | none, but upstream mergeability lost | rejected |
| D — ground-up runtime | ~418k LOC + ~360k LOC of tests + 206 grammars | n/a | rejected |

Two facts decided it. First, upstream at the baseline already builds and runs on Windows with
`CGO_ENABLED=0` for all seven target languages, with zero external runtime dependencies. Second, the
problems we found are not fixed by owning the code: the largest one (C# never taking the compact
route) is already documented and tracked upstream. A fork moves the maintenance cost to us without
moving the defect.

## Decision

Depend on `github.com/odvcencio/gotreesitter` at a pinned version. Do not fork.
Treat a fork as an escalation mechanism with an explicit trigger.

### Fork trigger

A fork is considered only when **all three** hold:

1. the failure reproduces at the **exact pinned baseline**, not at some other tip;
2. it appears in the **CGO-free product lane** — a race-lane or diagnostic-only failure never
   qualifies;
3. upstream has **no fix in a tagged release**, and is not already tracking it.

An item upstream already documents and tracks does not count toward the trigger. The correct
response there is to contribute a reproducer, not to fork.

### Runtime patch boundary

A minimal patch to upstream source is **not** authorized by: a performance problem, a compact-route
decline, a plausible optimization idea, or a diagnostic-lane timeout. It enters consideration only
with baseline reproduction, product-lane relevance, a correctness or required-release impact, a
minimum reproducer, oracle/regression evidence, and a stated retirement condition.

The `canReach` negative-reachability cache idea remains **EXPERIMENT ONLY**: its correctness
argument holds, but it was never built or run, and it writes one entry per visited node into a
fixed 32,768-entry 2-way cache, so it could plausibly be slower. It is not a candidate for adoption.

### Contribution before patching

Where a finding is genuinely useful upstream — a real-corpus decline witness, a per-parse admission
API, a byte→UTF-16 projection over UTF-8 input, a Windows lane — the first action is an upstream
report or PR, not a local patch.

## Consequences

- Zero upstream-sync cost while the trigger stays unfired.
- We inherit upstream's open issues, including `KR-0001`. That is accepted and made visible through
  the ratchet, not hidden.
- Availability risk: upstream is a young repository with maintenance highly concentrated in one
  author. Mitigation is a read-only mirror of the exact baseline plus `go.sum` pinning — insurance,
  not a fork.
- If the trigger ever fires, the fork keeps upstream's module path and is consumed through a
  `replace` in the main module. Topology: `upstream-baseline` (never committed to) → `main`
  (baseline + patches) → `patch/<id>` per patch, each with a retirement condition.

## Supersedes

None.
