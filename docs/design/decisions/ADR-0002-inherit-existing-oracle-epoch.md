# ADR-0002 — Inherit the existing oracle epoch

**Status:** Accepted · 2026-09-22

## Context

The syntax oracle is the C `tree-sitter` runtime. Two candidate epochs existed: the one upstream
`gotreesitter` already locks (`v0.25.1` @ `f5afe475…`), and current `tree-sitter/master`.

Measured: `master` was **958 commits** ahead of `v0.25.1`, with **56** touching the semantic core —
error-recovery child nesting, error-cost accumulation through hidden nodes, state on missing nodes,
deterministic ordering of recovery reductions, and a large set of query anchor/quantifier changes.

Those are exactly the areas every existing parity board measures. Moving the epoch would not improve
correctness; it would make ~79 recovery cases, ~103 query cases, and the 206-language highlight
board **incomparable** rather than wrong.

## Decision

Inherit upstream's epoch unchanged. Define the oracle as a tuple (runtime, transport, grammar
commits, generator version, build flags, artifact hash), not as a repository URL.
Full values: `docs/specs/oracle.md`.

Enforce the first comparison scope narrowly — node type, named-child order, field name, byte range,
completeness — because the only current consumer walks the AST directly and uses no queries.
Query, supertype, tags, and highlight dimensions are **adopted but not enforced** until a consumer
depends on them.

Migration requires all five conditions in `docs/specs/oracle.md` § Changing the epoch, including
"every digest difference is attributed to a named upstream commit". One unattributed difference
blocks the migration.

## Consequences

- Our parity claims are directly comparable with upstream's own boards.
- We do not lead upstream on oracle version; we follow it.
- Oracle evidence is Linux/container-only (the upstream harness uses `dlopen`). Windows compares
  against checked-in digests instead. If the container lane is unavailable, oracle checks are
  `NOT_RUN` — never replaced by a Windows-only substitute.
- "Newer is better" is explicitly rejected as a reason to migrate.

## Supersedes

None.
