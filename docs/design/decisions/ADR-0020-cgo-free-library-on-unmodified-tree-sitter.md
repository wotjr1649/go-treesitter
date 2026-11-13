# ADR-0020 — A public CGO-free tree-sitter library built from unmodified upstream C

**Status:** Proposed · 2026-11-13 · on acceptance supersedes ADR-0002, ADR-0013 and ADR-0018,
and ADR-0017 in part

## Context

The repository was built as a syntax layer for one consumer over an internal carrier of the pure-Go
`gotreesitter` runtime with 31 carried patches (ADR-0013, ADR-0018). Most effort went into making that
runtime agree with C tree-sitter, measuring it against a pinned C oracle, and binding provenance. The
directly written product code is about 1,200 lines; the carrier is 1,561 files.

The owner has since restated the goal and constraints:

- The consumer that motivated the carrier is no longer developed. This repository is a **public library**
  that lets Go programs use tree-sitter **CGO-free**, with optimization as an explicit aim.
- Product builds stay `CGO_ENABLED=0` (ADR-0003), and a developer machine without a C compiler must be able
  to use the library without installing one.
- Do not vendor another project's source into this repository and carry patches against it. Borrowing ideas
  is fine.
- The repository must own original technology, not only validate someone else's engine.
- Track the latest tree-sitter release instead of pinning one version indefinitely.
- Use the owner's tree-sitter-grammar-kit for grammar supply-chain checks.

The tree-sitter parsing runtime is C. Public precedents compile unmodified C to WebAssembly and translate
the module to Go source (`ncruces/go-sqlite3` for SQLite; `msuozzo/bonsai` and `cgofree/tree-sitter` for
tree-sitter), which keeps builds pure Go and needs no compiler on the consumer side.

## Decision

1. **Engine (W2 first, measured gate).** Build the unmodified tree-sitter runtime and each grammar's
   unmodified `parser.c`/`scanner.c` to wasm32 with wasi-sdk and translate it to Go source. Code this
   repository adds in C (for example a progress-callback shim for cancellation, or a tree export helper) is
   a separate, named file of our own; upstream files are never edited. A wazero-hosted build of the same
   wasm is the fallback if the W2 gates fail. The gates and thresholds are pre-declared in the session plan
   before measurement and confirmed by the owner.
2. **Versions.** The runtime follows the latest stable tree-sitter release: a new release is adopted when the
   translated build matches a native build of the same release on the reference corpus (target: within four
   weeks). Grammar sets are fixed per library release; a grammar change ships with a static node-type diff.
3. **Reference.** The correctness reference is a native C build of the *same* runtime release and grammar
   sources, built by this repository's own tooling. There is no long-lived pinned oracle epoch any more.
4. **Grammars.** Start from the grammar-kit's 26 routes (25 parsers; all MIT or BSD-3). Wave 1 (engine gate):
   C#, T-SQL, TypeScript/TSX, Python, Go, JSON. Wave 2: the remaining routes. Each grammar is its own Go
   module. Upstream grammar sources are used unmodified; grammar-kit's fixes go upstream as contributions,
   and remaining gaps are published as measured known gaps.
5. **Original work this repository owns.** An idiomatic Go API (context cancellation, resource limits,
   explicit lifetimes) and a one-call export of a parse tree into an immutable Go snapshot that many
   goroutines can read; the reproducible build recipe and update pipeline are means, not the product.
6. **grammar-kit** runs only in CI or a separate tools module. The product module never imports it.
7. **Transition.** This repository is restructured in place (module path kept; no tag exists). The carrier and
   its 31 patches are removed only after the new engine passes its gates. Prior ADRs and evidence stay as
   history.

## Consequences

- No vendored runtime and no carried runtime patches; generated Go is derived from unmodified upstream C and
  regenerated mechanically per release.
- Build size and compile memory of large grammars (C# `parser.c` is 31 MB) become first-class risks measured
  by the gate.
- Results remain identity-bound (`docs/specs/validation.md`): a result for one runtime release, grammar
  commit, toolchain or recipe is not carried to another.
- `docs/specs/baseline-provenance.md`, `docs/specs/oracle.md` and the Release-Critical scope must be rewritten
  when this ADR is accepted.

## Alternatives considered

- Keep the pure-Go carrier (status quo): fails the no-fork requirement and self-reports 3x to 4.8x slower
  than C on full parses.
- Depend on an existing translated build (bonsai, cgofree): fastest start, but no C# grammar, older runtime
  versions, and update speed controlled by a third party.
- Prebuilt native libraries loaded without cgo: native speed, but per-platform binaries, signing and
  antivirus friction.
- A new pure-Go engine: most original, but months of work and unlikely to match C behavior or speed.
