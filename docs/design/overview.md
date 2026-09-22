# Architecture overview

Normative for module layout and layer boundaries. Decisions and their reasoning live in
`docs/design/decisions/`.

## Layers

```
        consumer  (a product; not in this repository)
            │  depends only on this repository's own types
            ▼
      syntax      result / outcome / diagnostics / request types
            │     a one-method Parser seam, for fakes and for a future second implementation
            ▼
  internal/gtsadapter   the ONLY package that imports github.com/odvcencio/gotreesitter
            │
            ▼
   gotreesitter v0.53.0  (pinned; see docs/specs/baseline-provenance.md)
            ▲
            │ compared against
   oracle digests  ← produced in the Linux oracle lane (docs/specs/oracle.md)
```

## Planned layout

```
AGENTS.md              always-loaded kernel
README.md              human entry point
.gitattributes         MANDATORY before the first fixture lands (LF pinning)
.gitignore
go.mod

syntax/                Result, Outcome, Diagnostics, Request, Parser seam
                       no dependency on gotreesitter, no I/O, no language semantics

internal/
  gtsadapter/          maps gotreesitter state -> syntax.Result. The only importer of upstream.
  provenance/          identities.json (source of truth) + typed accessors + binding test

testdata/              external fixtures, LF-pinned, hashed in docs/validation/workloads.md
docs/                  contract plane (see docs/README.md)
artifacts/             evidence plane, append-only
.github/workflows/     product + cross lanes
```

Whether `syntax/` is exported or lives under `internal/` is an **open decision**; the layering is
the same either way. See `docs/design/decisions/ADR-0004-parser-boundary-and-result-model.md`.

## Boundary rules

1. **Single import point.** Only `internal/gtsadapter` may import
   `github.com/odvcencio/gotreesitter`. Enforce this with a test, not prose — a test that inspects
   the package graph and fails on any other importer. Prose boundaries erode; a failing test does not.
2. **No upstream types cross the boundary.** `syntax` exposes this repository's own types.
   An upstream `*Tree` may be carried as an opaque handle; its methods are not re-exported.
3. **No semantics in this repository.** Names, scopes, overloads, types, imports, code graphs, and
   memory/persistence are downstream concerns. See `docs/specs/parser-result.md` § Scope boundary.
4. **No downstream policy in this repository.** We make "incomplete" unambiguous; we do not decide
   whether a consumer publishes, keeps the previous state, or marks it stale.
5. **CGO only in diagnostic and oracle lanes.** The product path must build and run with
   `CGO_ENABLED=0`. The upstream module graph already guarantees this — upstream confines CGO to a
   separate module — and we must not reintroduce it.

## Chosen pattern, in one line

**An explicit result/state model behind a one-method internal seam.**
No backend strategy registry, no plugin system, no layered ports for a single implementation.

Why this and not the alternatives is recorded in ADR-0004. The short version: the evidence that
forced this design is entirely about *result state* (a parse that reports success while returning a
broken tree), not about *swappable backends*. Build the thing the evidence demands.

## What changes if a fork ever happens

A fork is a **build-time** substitution (`replace` in `go.mod`), not a runtime strategy.
The adapter does not change shape; only which module it compiles against changes.
That is precisely why no plugin or registry machinery is needed now.
Fork topology and triggers: `docs/design/decisions/ADR-0001-strategy-a-pinned-upstream-dependency.md`.
