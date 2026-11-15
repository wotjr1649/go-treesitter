# ADR-0026 — Never block on upstream: mirrors, grammar patches, health watch

**Status:** Accepted by the owner · 2026-11-15 · extends ADR-0023 (patch scope) and ADR-0024 (tier re-check).

## Context

Library users do not depend on upstream repositories: generated Go code is committed and released versions are kept by
the Go module proxy. Our development pipeline does: regeneration, the native reference, nightly and bench lanes fetch
pinned archives from upstream, and most grammar defects need a `grammar.js` change that only upstream can release.
Both risks have occurred: the T-SQL repository moved (`meloncholera` → `alfpricot`) and its pinned commit is no longer
reachable from any branch; the kit's Kotlin repository stopped merging changes.

## Decision

1. **Mirror (S1).** Every pinned archive (grammars, runtime and the downloads regeneration needs) is attached to each
   immutable `vX.Y.Z` release. `pins.json` names the mirror; tools fetch upstream first and fall back to the mirror;
   sha256 verification is unchanged and mandatory.
2. **Grammar patches (S2).** ADR-0023 patches may also target `grammar.js` and related grammar sources; the patched
   grammar is regenerated to `parser.c` with the pinned tree-sitter CLI, deterministically, and both engine and native
   reference use that C. Disclosure, the unpatched comparison and retirement rules of ADR-0023 apply. Each grammar's
   patch count is reported; crossing the alert threshold triggers an owner review of the source tier.
3. **Health watch (S3).** The weekly upstream watch reports rename/transfer, archival, deletion, pin reachability from
   the default branch, inactivity over 12 months and new releases; an unreachable pin or a missing repository fails it.
4. **Principle (S4).** Work never waits for upstream: a needed fix is carried as a recorded patch, reported upstream in
   parallel, and removed when upstream releases it. The tree-sitter runtime itself remains unmodified.

## Consequences

- Release creation attaches more assets (the archives of that release's pins).
- A growing patch list is visible and bounded by review, which guards against drifting back to a carried fork.
