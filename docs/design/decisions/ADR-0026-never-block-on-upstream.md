# ADR-0026 — Never block on upstream: mirrors, grammar patches, health watch

**Status:** Accepted by the owner · 2026-11-15 · extends ADR-0023 (patch scope) and ADR-0024 (tier re-check).
S1 amended by the owner on 2026-11-15 as follows; the original reasoning below is retained.

## Amendment (2026-11-15)

Mirror source archives (grammars, tree-sitter runtime and wasi-libc) with the release. Toolchain binaries
(wasi-sdk, binaryen and tree-sitter CLI, on all pinned host platforms) are not mirrored until their complete
bundled notices are established, for example from an SBOM of the exact archives. Fetch those tools from their
official releases with the pinned SHA-256 checks; the release supplies no fallback assets for them. For v0.4.0,
release assets are the 24 source archives plus `SHA256SUMS`. This amendment narrows decision S1 below; the
[publication procedure](../../specs/publication.md#pinned-archive-release-assets) defines the source-row selection.

## Context

Library users do not depend on upstream repositories: generated Go code is committed and released versions are kept by
the Go module proxy. Our development pipeline does: regeneration, the native reference, nightly and bench lanes fetch
pinned archives from upstream, and most grammar defects need a `grammar.js` change that only upstream can release.
Both risks have occurred: the T-SQL repository moved (`meloncholera` → `alfpricot`) and its former pinned commit became
unreachable from any branch (the owner-approved 2026-11-15 pin move selects a tree-identical reachable commit);
the kit's Kotlin repository stopped merging changes. The development T-SQL source is now the owner's
`wotjr1649/tree-sitter-mssql` repository, continuing the alfpricot history under the
[2026-11-18 ADR-0024 amendment](ADR-0024-grammar-source-tiers.md#amendment-2026-11-18-owner-designated-t-sql-source).

## Decision

1. **Mirror (S1).** Every pinned archive (grammars, runtime and the downloads regeneration needs) is attached to each
   immutable `vX.Y.Z` release. `pins.json` names the mirror; tools fetch upstream first and fall back to the mirror;
   sha256 verification is unchanged and mandatory.
2. **Grammar patches (S2).** ADR-0023 patches may also target `grammar.js` and related grammar sources; the patched
   grammar is regenerated to `parser.c` with the pinned tree-sitter CLI, deterministically, and both engine and native
   reference use that C. Disclosure, the unpatched comparison and retirement rules of ADR-0023 apply. Each grammar's
   patch count is reported; crossing the alert threshold triggers an owner review of the source tier. For a grammar under
   [ADR-0024 rule 7](ADR-0024-grammar-source-tiers.md#amendment-2026-11-18-rule-7-owner-maintained-repositories), fixes
   are developed in the owner repository. Carried patches remain subject to ADR-0023 under rule 7.6.
3. **Health watch (S3).** The weekly upstream watch reports rename/transfer, archival, deletion, pin reachability from
   the default branch, inactivity over 12 months and new releases; an unreachable pin or a missing repository fails it.
4. **Principle (S4).** Work never waits for upstream: a needed fix is carried as a recorded patch, reported upstream in
   parallel, and removed when upstream releases it. The tree-sitter runtime itself remains unmodified. For a grammar
   under [ADR-0024 rule 7](ADR-0024-grammar-source-tiers.md#amendment-2026-11-18-rule-7-owner-maintained-repositories)
   the needed fix is made in the owner repository and arrives with the next approved pin move.

## Consequences

- Release creation attaches more assets (the archives of that release's pins).
- A growing patch list is visible and bounded by review, which guards against drifting back to a carried fork.
