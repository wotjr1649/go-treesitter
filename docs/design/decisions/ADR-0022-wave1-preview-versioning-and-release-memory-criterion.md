# ADR-0022 — Wave-1 preview release, multi-module versioning, release-memory criterion, carrier removal

**Status:** Accepted by the owner · 2026-11-14 · amends ADR-0020 §4 (first-release scope) and the resource budget in
`docs/specs/validation.md`

## Context

Session 15 step 2 passed the wave-1 gates on the W2 engine (C agreement on the corpus and edit cases, speed, memory,
reproducible generation, execution on five CI platforms). Three gaps remained: grammar modules depend on the root via
`replace ../..` and cannot be consumed outside this repository; the release-memory criterion counts executable image
pages (6–10 MB, grammar-size dependent) that are not memory the engine holds; the old carrier still lives beside the
new engine.

## Decision

1. **First public release is a v0.1.0 preview with the wave-1 grammars** (C#, T-SQL, TypeScript, TSX, Python, Go,
   JSON). Wave-2 routes follow in later minor releases. ADR-0020 §4's "first release = all routes" is replaced by this.
2. **Versioning:** the root module is tagged `vX.Y.Z`; every grammar module `require`s that exact root version (no
   `replace` in a released commit) and is tagged `grammars/<dir>/vX.Y.Z`. Each tag and release still needs the owner's
   separate authority.
3. **Release-memory criterion:** after `Close`, every engine region is returned to the OS (100 %), and process
   private bytes are within start + 16 MB. Previous criterion: working set within start + 16 MB (worst measured
   15.2 MB; the remainder was mostly image pages). The peak criterion (100 x input + 64 MB) is unchanged.
   **macOS amendment (owner, 2026-11-14):** private bytes are the vmmap physical footprint, which keeps counting heap
   pages the Go runtime has already returned (`MADV_FREE_REUSABLE`; `GODEBUG=madvdontneed` has no effect on darwin).
   Bench run 43577876668 showed it on five snapshot cases (delta up to 97 MB) while the snapshot had been collected,
   the live heap was under 1 MB and `heap/released` covered the delta. On darwin the gate is therefore
   footprint delta minus the growth of the runtime's released bytes (`/memory/classes/heap/released:bytes`) within
   16 MB, and for snapshot cases the snapshot must have been collected. The raw footprint delta is still reported. Other
   platforms are unchanged.
4. **Carrier removal now:** `internal/runtime` (gotreesitter carrier and its patches), `syntax`,
   `internal/gtsadapter`, the old `treesitter.New`, `grammars/gpl` and tooling that only serves them are removed on the
   session-15 branch. They remain in Git history; their evidence stays in `artifacts/`.

## Consequences

- The generator must emit a release form of `go.mod` (exact root version, no `replace`).
- Memory results are re-measured under the new criterion and reported next to the old numbers.
- ADR-0002, ADR-0013, ADR-0014 and ADR-0018 describe removed code and stay as history.
