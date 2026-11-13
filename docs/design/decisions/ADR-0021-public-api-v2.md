# ADR-0021 — Public API v2: owned immutable snapshots on a confined parser

**Status:** Accepted by the owner · 2026-11-13 · supersedes ADR-0004 in part (package layout, outcome set) and
ADR-0009 (public constructor); ADR-0010 and ADR-0011 continue in the forms described in `docs/design/api-v2.md`

## Context

ADR-0020 replaced the engine: one single-threaded translated wasm instance per parser, trees in its linear memory,
cheap reads only after a one-call export into a Go snapshot. Three reviews and two prototypes (session 15) found that
the first draft was missing lifetime rules, failure mappings and everyday node operations, and that AST tools mostly
suffer from encoding/position errors, unsafe node lifetimes and repeated per-call text copies.

## Decision

1. One public package `treesitter`, one Go module per grammar behind a frozen, versioned engine ABI.
2. `(*Tree, error)` with `*ParseError` and sentinels; a tree exists only when complete. ADR-0004's lesson stays: there
   is no accepted-but-unusable state, and the root-end check runs on every accepted tree.
3. Every no-tree path resets the C parser before returning (a halted C parse otherwise resumes on the next call).
4. A parser and its trees are one confinement unit; snapshots, nodes and indexes are immutable and shareable.
   Edited trees are old-only. Leaked trees are freed by the owner goroutine, never by the GC goroutine.
5. The snapshot owns its source: one copy by default, no copy with `ParseOwned`; `Node.Text()` is a zero-allocation
   substring.
6. Invalid UTF-8 is parsed like C and reported (`HasInvalidUTF8`); strict rejection is opt-in. Legacy encodings
   move to an optional `encoding` package in v0.1.
7. v0.0.1 includes the position/encoding layer, Merkle hashes, atoms and an experimental `Match`.
8. Queries ship in v0.1 with the signatures fixed in `docs/design/api-v2.md`.

## Consequences

- `syntax`, `internal/gtsadapter` and the gotreesitter-specific diagnostics disappear; `docs/specs/parser-result.md`
  is rewritten from `docs/design/api-v2.md`.
- The ABI version check, cancel-then-reparse regression test, trap/grow-refusal tests and the per-reason outcome
  table become required tests.
- Memory cost of the owned source is +5–22 % of a snapshot; documented with the measurements.
