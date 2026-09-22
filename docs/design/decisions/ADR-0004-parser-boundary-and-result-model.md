# ADR-0004 — Parser boundary and result model

**Status:** Accepted · 2026-09-22

## Context

A downstream consumer using this class of runtime hit three failures, all of the same shape: the
parse reported success and the result was not usable.

- returned `error == nil` while the stop reason was `timeout`;
- `root.EndByte() == len(source)` while the last consumed token was at byte 3233 of 5986;
- `stopReason == accepted`, not stopped early, full root span, `HasError() == true`.

It also lost a session to a comparison defect: a complete result was compared against an incomplete
one without checking completeness first, producing a false "drift" report with no retained evidence
of either side.

Separately: the consumer uses **zero** tree-sitter queries. Its entire dependency surface is node
type, named-child order, field name, and byte span. There is exactly one consumer, one backend, and
no second implementation on any roadmap.

## Options considered

| Option | Verdict |
|---|---|
| **D. Explicit result/state model** | **Adopt.** Every failure above is a result-state failure. This is the thing the evidence demands. |
| **A. Ports and adapters** | **Adopt in reduced form** — a one-method seam, enough for test fakes and for a future second implementation. Not a layered port architecture. |
| **C. Anti-corruption layer** | **Adopt as a rule, not a layer** — "no upstream type crosses the boundary", enforced by an import test. A dedicated translation layer would be ceremony. |
| **B. Strategy + adapter for swappable backends** | **Reject now.** There is no second backend. A fork would be a `go.mod replace`, i.e. a build-time substitution, not a runtime strategy. |
| **E. Thin direct wrapper** | **Reject.** It leaks upstream types and, crucially, would not force the result model — the exact thing that failed downstream. |

## Decision

```
syntax        Result, Outcome, Diagnostics, Request
              Parser interface { Parse(ctx, Request) (Result, error) }    // one method

internal/gtsadapter    the only package importing github.com/odvcencio/gotreesitter
```

- The `Outcome` set is closed and defined in `docs/specs/parser-result.md`. Adding a member is a
  contract change.
- Evaluation order is fixed by that contract; short-circuiting it is the defect class above.
- Raw upstream reason strings are stored verbatim — they are the join key to upstream code and to
  `docs/validation/known-regressions.md`.
- Timing values never enter any deterministic identity (digests, golden files, semantic IDs).
- The import boundary is enforced by a **test over the package graph**, not by prose.
- Semantics (names, scopes, overloads, types, code graphs, persistence) are out of scope here, and
  downstream publish/stale policy is out of scope too.

## Repository identity (resolved)

**Internal product first, public option preserved.** `syntax` is a normal top-level package, not
under `internal/`, so promoting it to a public API later is a documentation change rather than a
move. Until that promotion is explicitly decided:

- **no version tag is published**, so there is no API-stability obligation yet;
- `Result`, `Outcome`, and `Diagnostics` may change shape freely, recorded in an ADR when they do;
- module path: `github.com/wotjr1649/go-treesitter`.

Reversing a published public API is expensive; promoting an unpublished package is cheap. This
ordering keeps the cheap direction available.

## Consequences

- One extra hop between consumer and upstream, and a type mapping to maintain. That cost buys the
  only thing that has actually broken in practice.
- Swapping backends later needs a second adapter, not a redesign — the seam already exists.
- Anyone tempted to "just use the upstream tree directly" is stopped by a failing test rather than
  by a code review that may not happen.

## Supersedes

None.
