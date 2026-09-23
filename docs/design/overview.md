# Architecture overview

Normative for module layout and layer boundaries. Decisions and their reasoning live in
`docs/design/decisions/`.

## Layers

```
        consumer  (a product; not in this repository)
            │  depends only on this repository's own types
            ▼
      treesitter.New()   public constructor; no upstream types exposed
            │
      syntax      result / outcome / diagnostics / request types
            │     a one-method Parser seam, for fakes and for a future second implementation
            ▼
  internal/gtsadapter   the ONLY importer outside the internal runtime carrier
            │
            ▼
   internal/runtime  (pinned origin + approved patches; ADR-0013)
            ▲
            │ compared against
   oracle digests  ← produced in an identified C oracle lane (docs/specs/oracle.md)
```

## Planned layout

```
AGENTS.md              always-loaded kernel
README.md              human entry point
.gitattributes         MANDATORY before the first fixture lands (LF pinning)
.gitignore
go.mod
parser.go              treesitter.New returns syntax.Parser

syntax/                Result, Outcome, Diagnostics, Request, Parser seam
                       no dependency on gotreesitter, no I/O, no language semantics

internal/
  gtsadapter/          maps runtime state -> syntax.Result. The only external carrier importer.
  runtime/             identified upstream production sources + approved patches
  provenance/          identities.json (source of truth) + typed accessors + binding test

testdata/              external fixtures, LF-pinned, hashed in docs/validation/workloads.md
docs/                  contract plane (see docs/README.md)
artifacts/             evidence plane, append-only
.github/workflows/     product + cross lanes
```

`syntax/` and the root constructor are public import paths; the adapter remains
internal. They are not versioned as a stable release yet. See ADR-0009.
`syntax.Index` optionally indexes snapshot byte ranges and node types for
repeated lookup. It carries no parser handle or semantic-resolution behavior.
See ADR-0011 for ownership and cost.

## Boundary rules

1. **Single import point.** Only `internal/gtsadapter` may import
   `internal/runtime` from outside that carrier. Enforce this with a test, not prose — a test that inspects
   the package graph and fails on any other importer. Prose boundaries erode; a failing test does not.
2. **No upstream types cross the boundary.** `syntax` exposes this repository's own types.
   An upstream `*Tree` may be carried as an opaque handle; its methods are not re-exported.
3. **No semantics in this repository.** Names, scopes, overloads, types, imports, code graphs, and
   memory/persistence are downstream concerns. See `docs/specs/parser-result.md` § Scope boundary.
4. **No downstream policy in this repository.** We make "incomplete" unambiguous; we do not decide
   whether a consumer publishes, keeps the previous state, or marks it stale.
5. **CGO only in diagnostic and oracle lanes.** The product path must build and run with
   `CGO_ENABLED=0`. The carrier contains only Go product sources; graph and consumer checks must
   keep C-dependent packages outside the product.

## Chosen pattern, in one line

**An explicit result/state model behind a one-method internal seam.**
No backend strategy registry, no plugin system, no layered ports for a single implementation.

Why this and not the alternatives is recorded in ADR-0004. The short version: the evidence that
forced this design is entirely about *result state* (a parse that reports success while returning a
broken tree), not about *swappable backends*. Build the thing the evidence demands.

## Runtime distribution

The owner-approved internal carrier distributes the tested patches as part of
the ordinary Go module. It uses no consumer replace directive or runtime backend
selector. Its source/patch manifest, import boundary and lifecycle are described
in ADR-0013. The first project version will be `v0.0.1`; a version plan does not
establish release readiness.

ADR-0014 separates three GPL grammars into the optional `grammars/gpl` module.
The base's 203 grammars and that module's three entries retain 206 basic-test
paths. Its own internal adapter is the sole runtime importer in that module;
the base module's boundary remains unchanged. A side-effect import registers
the entries at initialization, with no public runtime types or runtime download.
