# ADR-0030 — Opt-in Go query budgets

**Status:** Accepted · 2026-11-25

## Context

`NewQuery` validates in a scratch instance. The first `Matches` on each parser instance compiles
the C query again. The C cursor polls context and `QueryOptions.MatchLimit` bounds its
in-progress match states. Neither bounds the Go match stream or text predicate work. The root
1.x contract preserves existing query signatures, struct literals and the behavior of ABI 1–4
grammar consumers. [ADR-0028](ADR-0028-root-stability-and-independent-grammar-versioning.md)

## Decision

Ship additive Go budgets before RC. `NewQueryWithOptions` and `Tree.MatchesWithOptions` take
functional options with `int64` values. Zero leaves each limit unset. The options preserve
existing representation and engine bounds. The caller can continue to use `NewQuery`,
`Tree.Matches`, and existing `QueryOptions` literals.

- Compile limits cover source bytes before scratch allocation, scratch linear memory through
  the parser's existing limit path, TSGQ bytes before Go decoding, independently capped derived
  property copies, and cumulative regex units. Reserve translation and class growth before
  allocating. Reserve conservative bounds for standard-library parsing and expansion before
  parsing, simplifying and compiling. Check context between these calls and inside Go loops.
  Count opaque regex predicates during validation. The new compile path builds one
  `syntax.Prog` per regex. Legacy queries retain their compiled regexp for the legacy fast path.
- Match limits count raw candidate captures before Go capture allocation, accepted matches
  (including zero-capture matches), and cumulative Go predicate work. Count rejected
  candidates toward capture and work limits. Instrument the existing VM in the new match path
  so cancellation and work limits apply within regex evaluation. Resolve options when creating
  a sequence and start with fresh counters on each iteration. At exactly N accepted matches,
  continue until the cursor ends or finds another accepted match unless the caller breaks.
- Preserve the parser's busy guard, cursor and cache lifetimes, and existing match-limit and
  fault handling. A stopped iterator preserves results it already yielded. Clean cancellation
  releases unpublished C queries on cold compile and leaves existing trees reusable.
- Classify configuration errors and new-constructor syntax errors as `InvalidInput`. The
  constructor preserves `*QueryError` as a cause. Go budget exhaustion is `ResourceLimit`
  without an instance reset. A refused engine grow uses the existing resource-limit and reset
  rules. An observed context error takes precedence at new Go budget checkpoints.

The [query contract](../query-abi.md#opt-in-go-budgets) specifies units, checkpoints, and limits
on their guarantees. A service that must enforce a total memory bound or a hard deadline uses
an independently killable worker with an OS memory limit and a supervisor timeout.

## Deferred until after 1.0

C compilation on ABI 2–4 remains cooperative at the Go call boundary. A future runtime
constructor must poll during pattern parsing and structural analysis and release partial
state on cancellation. Its allocation limit counts cumulative requested allocation bytes,
including the full requested size of reallocations, on both scratch and lazy compilation.
A future `WithMaxQueryEngineCompileBytes` and `ErrQueryBudgetUnsupported` require those
capabilities. ABI 1 continues to report `ErrQueryUnsupported`.

Bounded metadata and cursor exports would let the shim check TSGQ size before malloc and
candidate capture count before indexing and buffer growth. The Go checks apply after these
engine allocations. Preserve the current formats and interfaces under any ABI extension.
The generator owns shim and generated code. Runtime changes require a separate approved pin
move or an owner decision revising the unmodified-runtime policy.
[ADR-0023](ADR-0023-temporary-upstream-grammar-patches.md) · [Engine ABI](../engine-abi.md)

## Alternatives

A field addition to `QueryOptions` would break unkeyed literals. A single `MatchLimit` cannot
count Go results or predicate work. Default caps could reject queries accepted today. A
goroutine or context deadline alone cannot force a running C compilation to end or enforce
total process memory.
