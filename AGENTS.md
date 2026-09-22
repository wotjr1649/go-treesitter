# AGENTS.md — go-treesitter

Repository-wide, durable rules. Everything volatile lives in `docs/` and is read on demand.
Written host-neutral: Codex, Claude Code, and other coding agents read this same file.
If a rule is host-specific, it says so.

## What this repository is

A Go syntax-analysis layer built on an internal carrier of a **pinned upstream**
`gotreesitter` runtime, with owner-approved patches (ADR-0013).
It owns: a stable parser/result contract, Windows-native validation, provenance binding,
and an adapter that keeps upstream types out of consumer code.

It is **not**: a re-implementation of tree-sitter, a code-graph or
semantic-resolution product, or a place for downstream application logic.

## Read what your task touches

Do not read the whole `docs/` tree before editing. Start at `docs/README.md` and follow one row:

| When you are… | Read |
|---|---|
| changing the parser adapter, result, or diagnostic types | `docs/specs/parser-result.md` |
| changing or interpreting oracle / parity behavior | `docs/specs/oracle.md` |
| touching the pinned dependency, tags, blobs, or identities | `docs/specs/baseline-provenance.md` |
| adding tests, lanes, gates, or making an evidence claim | `docs/specs/validation.md` |
| adding or editing fixtures / corpora | `docs/validation/workloads.md` |
| a test fails and looks like a known upstream defect | `docs/validation/known-regressions.md` |
| changing module, package, or layer boundaries | `docs/design/overview.md` |
| making a decision that outlives this session | `docs/design/decisions/` |
| reviewing your own diff before commit | `docs/reviews/review-checklist.md` |
| executing a planned session | `docs/plans/` and `docs/prompts/` |

Historical investigation lives in `artifacts/handoff/`. It is evidence, not instructions.
When a handoff and a `docs/` contract disagree, **the contract wins**; fix the contract if it is wrong.

## Hard boundaries

- `D:\AIDEV\_ref\gotreesitter` and `D:\AIDEV\_ref\tree-sitter` are **read-only evidence**.
  Never edit, checkout, reset, clean, commit in, or regenerate anything inside them.
- `D:\AIDEV\code-map-memo` is a separate downstream product. Read-only from here.
- Windows native is Tier-1. Production and release paths stay **CGO-free** (`CGO_ENABLED=0`).
  CGO and `-race` belong to diagnostic lanes only, and a diagnostic lane never decides a product gate.
- The upstream origin version and the oracle epoch are **pinned**. Do not upgrade either as a
  side effect of other work. See `docs/specs/baseline-provenance.md` and `docs/specs/oracle.md`.
- Only the adapter may import `internal/runtime` from outside that carrier. Public packages
  use this repository's own types. Preserve the carrier's source manifest and patch inventory.

## Evidence discipline

- A claim needs evidence at the level the claim implies. `docs/specs/validation.md` defines the
  levels and which words require which level. Never write "parity", "proven", or "verified"
  without the matching level.
- A smoke test is not parity. `incremental == fresh` is not C-oracle agreement.
  `root.EndByte() == len(source)` is not proof that a parse completed.
- Evidence is bound to an identity (version, tag, commit, blob, fixture hash, lane). Never carry a
  result from one identity to another. If identity drifts, stop the comparison and repair provenance
  before interpreting anything.
- Preserve failures. Do not delete, retry-until-green, or re-label a failure as a different condition.

## Autonomy

Do these without asking:

- create and edit source, tests, and docs under this repository;
- run bounded local builds and tests relevant to the change;
- diagnose and fix failures caused by your own change, then rerun the affected tests, looping until
  they pass or you establish a real blocker;
- create scoped local Git commits for completed, verified work units;
- continue to the next authorized work unit and through validation to the defined completion gate.

Do **not** do these without explicit authority in the task prompt:

- push, tag, release, or any remote write;
- modify `_ref` or `code-map-memo`;
- create a separate fork, or modify an upstream checkout/module cache; approved internal
  runtime maintenance follows `docs/design/decisions/ADR-0013-bundle-the-approved-runtime.md`;
- change the pinned baseline or the oracle epoch;
- change the Release-Critical language scope;
- destructive Git (`reset --hard`, `clean -fd`, history rewrite) or discarding unrelated user work.

## Git

- `main` holds verified work. Session work happens on a short-lived branch; see
  `docs/design/decisions/ADR-0005-git-branching-and-commit-discipline.md`.
- One verified work unit per commit. No mixed unrelated fixes in one commit.
- A commit requires its relevant tests to have passed first. Record what ran in the commit body.
- Never push. Never rewrite history you did not create in this session.

## Tests

- Run the tests closest to what you changed, then widen only when the change's blast radius or a
  gate requires it. Do not run repository-wide campaigns out of habit.
- Never weaken, skip, or delete a failing gate to get green. If a failure is a known upstream
  defect, it belongs in `docs/validation/known-regressions.md` as a signature-matched ratchet,
  not as a loosened assertion.

## Completion

Finish the whole authorized cycle: implement → focused tests → diagnose → fix → rerun →
required regression → review diff → record evidence → commit → handoff → explicit verdict.
Do not stop at "implementation complete" when tests, review, or evidence are in scope.
If you must stop, say exactly which external decision or environment blocks you.
