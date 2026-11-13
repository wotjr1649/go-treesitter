# Baseline and provenance contract

Normative. Owns which upstream sources and tools the product is built from and how that identity is bound.
No other document may restate these values; link here instead. Engine and policy: ADR-0020. ABI: `docs/design/engine-abi.md`.

## What the product is built from

Every grammar module is generated, never hand-edited, from:

| Input | Rule |
|---|---|
| tree-sitter runtime C (`lib/src/lib.c` tree) | an upstream **release**, unmodified |
| grammar `parser.c`, `scanner.c`, headers | upstream grammar repository at a full commit, unmodified (wave 1: the six repositories of ADR-0020) |
| `tools/gen/shim/tsg_abi.c` | this repository's own C, versioned by the engine ABI |
| toolchain | wasi-sdk (clang), binaryen `wasm-opt`, wasm2go with `-unsafe -embed`, Go; versions and the recipe name recorded per generation |
| post-processing | the hash-checked `memory_grow` replacement of `tools/gen` (ADR-0020 §8) |

Upstream files are never edited. A fix to an upstream file goes upstream as a contribution; until it lands, the
difference is a published known gap.

## Machine-readable source of truth

```
tools/gen/pins.json                 runtime release + archive/tree sha256, grammar repos + full commits, tool versions
grammars/<lang>/provenance.json     identity of one generated module: inputs and their sha256, recipe sha256, outputs
grammars/<lang>/language.go         the same identity compiled in (engine.Identity)
```

The JSON is authoritative; prose that disagrees is a defect. A test in each grammar module checks that
`provenance.json` and the compiled `engine.Identity` agree, and `tools/gen` refuses to generate from an input whose
sha256 differs from the pin.

## Versions

- **Runtime:** follows the latest stable tree-sitter release. A release is adopted when the generated build agrees
  with a native C build of the same release on the reference corpus (`docs/specs/oracle.md`); target within four
  weeks of the release. Adoption updates `tools/gen/pins.json`, regenerates every grammar module and records old and
  new results side by side in a handoff.
- **Grammars:** the grammar set and commits are fixed per library release. A grammar change ships with a static
  node-type (schema) diff.
- **Engine ABI:** `internal/engine.ABIVersion`; a grammar module built for another ABI is rejected at `NewParser`.

## Transition (until the carrier is removed)

The previous product runtime — the `internal/runtime` carrier of `gotreesitter` v0.56.0 with its patches, bound by
`internal/provenance/identities.json`, `internal/provenance/runtime.json` and `tools/runtime-bundle/source.json` —
remains in the tree only until the new engine passes its gates and the owner approves its removal (ADR-0020 §7).
Those files keep binding the old code while it exists; they are not identities of the new engine, and no result is
carried between the two.

## Identity rules

1. **Never transfer evidence across identities.** A measurement taken at one runtime release, grammar commit,
   toolchain, recipe, lane or fixture hash says nothing about another. If any component differs, the comparison is
   void.
2. **Never label a new result with an old producer.** Re-run or mark `NOT_RUN`.
3. **A checkout tip is not a release.** Read runtime and grammar sources from the pinned release/commit, verified by
   hash, never from a working tree that happens to be checked out at something else.
4. **Line endings are part of identity.** See "Fixture bytes" below.
5. **Changing a pin is a recorded change**, not a side effect of another task: regenerate, re-run the identity-bound
   gates, record old and new values.

## Fixture bytes

`core.autocrlf` is `true` on the development machine and Git normalizes text on commit. The same fixture read from a
checkout and from a module cache can therefore differ in byte count and in every span it produces.

Required:

- `.gitattributes` pins text to LF and marks binary fixtures `-text`; generated Go sources are LF.
- Every fixture is recorded in `docs/validation/workloads.md` with its SHA-256 **of the LF bytes**.
- A test verifies recorded hashes at run time. A hash mismatch fails the run; it never gets "fixed" by updating the
  hash without an explanation.
