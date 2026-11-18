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
| 0020 | A public CGO-free tree-sitter library built from unmodified upstream C | Accepted; §4 amended by ADR-0022; grammar sources by ADR-0023; decision 6 by ADR-0025; decision 1 (generated grammar C) by ADR-0027 |
| 0021 | Public API v2: owned immutable snapshots on a confined parser | Accepted; decision 6 amended 2026-11-17 (built-in CP949 instead of an optional `encoding` package) |
| 0022 | Wave-1 preview, multi-module versioning, release-memory criterion, carrier removal | Accepted |
| 0023 | Temporary, retirable patches to upstream grammar C | Accepted; amends ADR-0020 §1, §4 for grammars; decision 1 amended 2026-11-17 (PostgreSQL Korean support) and 2026-11-18 (scope, five-patch limit, 12-month review) |
| 0024 | Grammar source tiers: one upstream per language | Accepted; Consequences amended by ADR-0025; rule 6 (follow a maintained fork) added 2026-11-16; rule 7 (owner-maintained repositories) added 2026-11-18 |
| 0025 | Absorbing tree-sitter-grammar-kit | Accepted; amends ADR-0020 (decision 6), ADR-0024; decisions 4 and 5 amended 2026-11-16 |
| 0026 | Never block on upstream: mirrors, grammar patches, health watch | Accepted; extends ADR-0023, ADR-0024; S1 amended 2026-11-15: sources mirrored, toolchains deferred until complete bundled notices |
| 0027 | One pinned generator CLI for every grammar | Accepted 2026-11-16, implementation pending; amends ADR-0020 decision 1 |
