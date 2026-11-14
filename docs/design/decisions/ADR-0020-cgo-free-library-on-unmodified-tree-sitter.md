# ADR-0020 — A public CGO-free tree-sitter library built from unmodified upstream C

**Status:** Accepted by the owner · 2026-11-13 · supersedes ADR-0002, ADR-0013 and ADR-0018,
and ADR-0017 in part (grammar scope)

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
8. **Build recipe details fixed by the spike.** wasm2go runs with `-unsafe` (bounds checks kept; unaligned
   loads/stores on amd64/arm64). The generated `memory_grow` is replaced in a deterministic, hash-checked
   post-processing step of our own generator with a reserve-then-commit linear memory (`VirtualAlloc` on
   Windows, `mmap` on Unix), because wasm2go's `append`-based growth left 4–5x garbage in the Go heap.
   A custom memory-provider hook is proposed to wasm2go upstream; if accepted, the post-processing step is
   removed. Publishing that proposal needs the owner's separate authority.

## Evidence (session 15 spike, E2/E5 as noted, identity-bound)

Identity: tree-sitter v0.27.2 unmodified; wave-1 grammars at the pins above; wasi-sdk 34 (clang 23.1.0-rc3),
wasm2go v0.4.21, Go 1.27.4, `CGO_ENABLED=0`, Windows amd64; reference = gcc 16.2.0 native build of the same
sources. Raw results are kept in the local session-15 handoff, not in this public record.

- Correctness vs the same-release native C (canonical cursor-preorder dump): fresh 1,586/1,586 and
  incremental edits 5,919/5,919 identical; the Go snapshot renders the same 1,586 dumps (C agreement on that
  corpus only, not on arbitrary input).
- Speed: large-input parse geomean 1.46x native; faster than gotreesitter v0.57.0 (8.1x native) and bonsai
  v0.4.1 (3.4x native) on every same-grammar input; small inputs within 1 ms or 2.5x native per file.
- Memory: peak working set within 100 x input + 64 MB on all 12 large inputs for parse and for parse + snapshot
  export (worst 53 % of the limit); released to within 16 MB of start after discard.
- Build: largest grammar (C#) builds in about 16 s with under 1 GB compiler memory; binary growth up to
  about 12 MB per grammar. Cold start about 32 ms; cancellation through the progress callback about 0.2 ms.
- Update drill v0.27.1 → v0.27.2: no manual edits beyond the version parameter; native and W2 agree at both
  releases; the release changed only `has_error` on ERROR nodes (57/1,586 dumps), as its release notes state.
- Not yet run: Linux, macOS and Windows arm64 execution; memory implementation exists for Windows only.

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
