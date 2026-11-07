# ADR-0019 — Explicit authority for every main merge

**Status:** Accepted · 2026-11-07 · supersedes ADR-0005 in part

## Context

ADR-0005 let a session merge a SMALL branch (≤ 8 commits, additive, no
`docs/specs/` change) into `main` by itself. It also assumed that no remote
existed and that branches were named `session/NN-<slug>`.

Those assumptions no longer hold. The repository is public. `main` is protected
by a ruleset, and new branches are restricted by another. Since ADR-0016, work
has reached `main` through pull requests from `validation/release-v0.0.1`. The
publication contract (`docs/specs/publication.md`) and `AGENTS.md` require
explicit authority for each main merge, a local merge included. ADR-0005's
SMALL rule therefore told agents to do something the contracts forbid.

## Decision

- No session merges into `main`, locally or remotely, without the owner's
  explicit authority for that merge. Branch size does not change this.
- ADR-0005's SMALL/LARGE classification still decides how much review material
  a branch needs. A LARGE branch carries the PR description and reviewer guide.
  It no longer decides who may merge.
- A short-lived local branch may use any `<type>/<NN>-<slug>` name; the
  `session/NN-<slug>` form is no longer required.
- The remote exists. Each push, PR, merge, branch deletion, tag or release
  needs its own authority, as `AGENTS.md` and `docs/specs/publication.md`
  state. Even with authority, an agent never uses the ruleset administrator
  bypass.

## Consequences

- Every verified branch stops before `main` and waits for an explicit merge
  decision, even a one-commit documentation fix.
- An agent reads one merge rule. ADR-0005 and the contracts no longer disagree.

## Supersedes

ADR-0005's SMALL self-merge rule, its bootstrap exception (already used), its
`session/NN-<slug>` naming requirement and its statement that no remote exists.
ADR-0005's commit discipline, PR-readiness, `--no-ff` merges, no-squash rule and
history preservation remain in force.
