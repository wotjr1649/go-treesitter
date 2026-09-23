# Architecture Decision Records

## When to write one

Write an ADR when a choice will still constrain work three sessions from now **and** a reasonable
engineer could have chosen otherwise. Do not write one for naming, local refactors, or anything a
contract document already states.

If a decision only affects one session, it belongs in `docs/plans/`, not here.

## Naming

```
ADR-NNNN-kebab-case-title.md
```

Numbers are allocated in order and never reused.

## Lifecycle

- An ADR is **immutable once accepted**, except for its `Status` line and a `Superseded by` link.
- To change a decision, write a **new** ADR that says `Supersedes: ADR-NNNN`, and set the old one's
  status to `Superseded by ADR-MMMM`. Never edit the old reasoning — the record of why we once
  thought otherwise is the point.
- `Status` is one of `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Withdrawn`.

## Relationship to contracts

An ADR records **why**. A contract in `docs/specs/` records **what you must do**.
When they overlap, the contract is what an agent obeys; the ADR is what a reviewer reads to
understand whether the contract is still right.

## Index

| ADR | Title | Status |
|---|---|---|
| 0001 | Strategy A — pinned upstream dependency, fork as escalation | Superseded in part by ADR-0013 |
| 0002 | Inherit the existing oracle epoch | Accepted |
| 0003 | Windows Tier-1, CGO-free production | Accepted |
| 0004 | Parser boundary and result model | Accepted |
| 0005 | Git branching and commit discipline | Accepted |
| 0006 | Instruction architecture — AGENTS kernel, docs router, no skill | Accepted |
| 0007 | Dynamic generator selection, immutable evidence inputs | Accepted |
| 0008 | Windows native C oracle execution | Accepted |
| 0009 | Public consumer entry | Accepted |
| 0010 | Request resource limits | Accepted |
| 0011 | Snapshot lookup | Accepted |
| 0012 | Complete maintained TypeScript oracle patch | Accepted |
| 0013 | Bundle the approved runtime | Accepted |
| 0014 | Separate GPL grammar distribution | Accepted |
| 0015 | Derived Go grammar and recovery | Accepted |
| 0016 | Curated publication and release evidence | Accepted |
