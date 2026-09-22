# ADR-0005 — Git branching and commit discipline

**Status:** Accepted · 2026-09-22

## Context

Single maintainer. Work arrives as discrete AI sessions. The repository currently has **zero
commits**; `main` is unborn. Bisectability matters more than usual here: a parser regression is
often only identifiable by walking history.

Negative evidence is available. The downstream sibling project used a long-lived
`integrate/session…` branch; it is still unmerged and holding a blocked gate. Long-lived integration
branches accumulate unverified state and make "what is actually true right now" hard to answer.

## Options considered

| Option | Verdict |
|---|---|
| **Trunk-based, short-lived session branches** | **Chosen.** Small verified merges, clean bisect, each session bounded. |
| GitHub Flow with mandatory PR review | Same shape, extra ceremony with no second reviewer. PR is optional, not required. |
| Long-lived integration branch | Rejected — directly observed to accumulate risk in the sibling project. |
| GitFlow develop/release branches | Rejected — excessive for one maintainer with no parallel release trains. |

## Decision

### Branching

```
main                 verified work only; every merge passed its session gate
session/NN-<slug>    one short-lived branch per implementation session
```

- Session work happens on `session/NN-<slug>`, merged into `main` with `--no-ff`.
  The merge commit is the audit artifact, so the branch can be deleted afterwards.
- **Bootstrap exception:** the repository's root commit is made directly on `main`, because there is
  nothing to branch from. It contains only the foundation (`.gitattributes`, `.gitignore`, `go.mod`,
  `AGENTS.md`, `README.md`, `docs/`). Everything after it follows the branch rule.
- `.gitattributes` **must** be in that root commit. Adding it later triggers a renormalization pass
  over files already committed.
- Never push, tag, or touch a remote without explicit authorization in the task prompt.

### Commits

- **One verified work unit per commit.** No mixed unrelated fixes.
- A commit is created only **after** the tests relevant to it have passed.
- Conventional-Commits style subject, imperative, ≤ 72 characters:
  `feat|fix|test|docs|chore|refactor|build(scope): summary`
- The body records what was actually run and what it produced. An agent-authored commit must be able
  to answer "what evidence supports this?" from the commit alone:

  ```
  Evidence: go test ./syntax/... ./internal/gtsadapter/... -count=1  -> ok
  Lane: product (windows/amd64, CGO_ENABLED=0)
  Not run: oracle lane (no container)
  ```

- Documentation and implementation go in the **same** commit when the doc records a contract the
  code introduces. Editorial-only doc work is a separate `docs:` commit.
- A failing experiment is not committed to a session branch. Keep it out of history, or record it in
  the handoff as an unmerged observation with its own rationale.
- **Do not squash** within a session — the per-unit commits are the evidence trail. The `--no-ff`
  merge already groups them.
- Never `reset --hard`, `clean -fd`, amend, or rebase history you did not create in this session.
  Never stage or commit unrelated pre-existing user work.

### Merge mechanism: PR-ready by construction, chosen by size

A large branch must be reviewable as a PR. A small one should not pay that cost. Both are satisfied
by making **every** branch PR-ready and choosing the merge mechanism by size and risk.

Every session branch is PR-ready by construction:

- conventional commits, one verified work unit each, in a readable order;
- no force-push, no rebase, no history rewrite;
- self-contained — it does not depend on another unmerged branch;
- the session handoff contains a ready-to-paste PR description and a reviewer guide.

Merge mechanism:

```
SMALL   ≤ 8 commits, additive only, no change under docs/specs/
        -> the session performs the local --no-ff merge itself at the end.

LARGE   > 8 commits, OR changes any contract under docs/specs/,
        OR touches the baseline, the oracle epoch, or Release-Critical scope,
        OR contains or proposes a runtime patch
        -> the session STOPS at the branch. It does not merge.
           It produces the PR description and reviewer guide, and hands over.
           The user chooses: push + open a PR, or review locally and merge --no-ff.
```

The agent classifies its own branch honestly and states the classification in the handoff.
When in doubt, treat it as LARGE — an unmerged branch is recoverable; a wrongly merged one is not.

**Bootstrap exception (applies once).** The LARGE rule exists to protect a `main` that already holds
verified work. In the bootstrap session, `main` holds only the root commit that same session created;
there is nothing to protect, and stopping at the branch would leave `main` as a bare skeleton while
all real work sits unmerged.

So: **if `main` contains no commit authored by an earlier session, the session merges its own branch
with `--no-ff` after its session gate passes**, and still produces the full PR package so the work
can be reviewed retroactively. From the next session onward the LARGE rule applies unmodified.

This exception is keyed on the state of `main`, not on a session number — it cannot be reused by
claiming "my branch is also foundational".

Opening an actual PR needs a remote, which does not exist yet. Creating the remote and pushing are
**user-owned**; the agent never does either.

## Consequences

- History reads as: root commit, then one merge per session, each containing 5–10 verified units.
- `git bisect` lands on a single verified unit, not a session-sized blob.
- Session branches are deletable, so branch drift does not accumulate.
- An interrupted session leaves an unmerged branch, which is a visible and recoverable state rather
  than a half-applied `main`.

## Supersedes

None.
