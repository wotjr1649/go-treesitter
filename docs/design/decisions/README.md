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
| 0002 | Inherit the existing oracle epoch | Superseded by ADR-0020 |
| 0003 | Windows Tier-1, CGO-free production | Accepted |
| 0004 | Parser boundary and result model | Superseded in part by ADR-0021 |
| 0005 | Git branching and commit discipline | Superseded in part by ADR-0019 |
| 0006 | Instruction architecture — AGENTS kernel, docs router, no skill | Accepted |
| 0007 | Dynamic generator selection, immutable evidence inputs | Accepted |
| 0008 | Windows native C oracle execution | Accepted |
| 0009 | Public consumer entry | Superseded by ADR-0021 |
| 0010 | Request resource limits | Accepted |
| 0011 | Snapshot lookup | Accepted |
| 0012 | Complete maintained TypeScript oracle patch | Accepted |
| 0013 | Bundle the approved runtime | Superseded by ADR-0020 |
| 0014 | Separate GPL grammar distribution | Superseded by ADR-0022 (removed) |
| 0015 | Derived Go grammar and recovery | Superseded by ADR-0022 (removed) |
| 0016 | Curated publication and release evidence | Accepted |
| 0017 | Exclude unclear grammars; assurance by release scope | Superseded in part by ADR-0020 |
| 0018 | Evaluate the approved runtime baseline transition | Superseded by ADR-0020 |
| 0019 | Explicit authority for every main merge | Accepted |
| 0020 | A public CGO-free tree-sitter library built from unmodified upstream C | Accepted; §4 amended by ADR-0022 |
| 0021 | Public API v2: owned immutable snapshots on a confined parser | Accepted |
| 0022 | Wave-1 preview, multi-module versioning, release-memory criterion, carrier removal | Accepted |
