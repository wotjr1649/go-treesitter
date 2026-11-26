# Baseline and provenance contract

Normative. Owns which sources and tools the product is built from and how that identity is bound.
No other document may restate these values; link here instead. Engine and policy: ADR-0020. ABI: `docs/design/engine-abi.md`.

## What the product is built from

Every grammar module is generated, never hand-edited, from:

| Input | Rule |
|---|---|
| tree-sitter runtime C (`lib/src/lib.c` tree) | an upstream **release**, unmodified |
| grammar inputs, `scanner.c`, headers | full commit in an upstream grammar repository or in an owner repository designated under [ADR-0024 rule 7](../design/decisions/ADR-0024-grammar-source-tiers.md#amendment-2026-11-18-rule-7-owner-maintained-repositories); `parser.c` is regenerated with the single pinned CLI under ADR-0027, except recorded behaviour-gate exceptions; owner-approved temporary source patches follow ADR-0023 |
| `tools/gen/shim/tsg_abi.c` | this repository's own C, versioned by the engine ABI |
| toolchain | wasi-sdk (clang) and binaryen `wasm-opt` pinned by upstream release archive sha256 per platform; wasm2go pinned by module version + `go.sum` hash and built with `go install -trimpath`; versions and the recipe name recorded per generation (host binary hashes are not identity) |
| post-processing | the hash-checked `memory_grow` replacement of `tools/gen` (ADR-0020 §8) |

The runtime stays unmodified. Temporary grammar patches carried by go-treesitter follow the recorded, hash-checked patch
and retirement mechanism of ADR-0023. Fixes released by a designated owner repository arrive through an approved
ADR-0024 rule 7 pin move. Grammar-input changes regenerate C with the pinned CLI, identically for the engine and native reference.
The YAML early-EOF fix, Bash/Ruby heredoc comparisons, Ruby symbol-name comparison, YAML/Python serialization bounds, YAML scanner row widening, Rust block comment
code-point fix are C-only patches. HTML patches `grammar.js` to accept `>` and
bare `&` in text, Go patches it to accept Go 1.26 expression arguments to `new`, Ruby patches it to accept
non-ASCII global-variable names, CSS patches it to accept non-Latin-1 names and single bare declaration identifiers, XML patches `common/common.mjs` to accept non-ASCII names (its scanner patch is C-only), and PowerShell patches `grammar.js` to accept non-ASCII names. The pins and generated provenance record original and result source hashes and each patch identity.
PostgreSQL retired the two temporary Korean-support patches with the upstream v19.0.0 pin, which includes
[upstream PR #102](https://github.com/gmr/tree-sitter-postgres/pull/102). See the
[retirement record](../../tools/gen/patches/postgres/README.md) for both patch identities and retained regressions.
The unpatched native comparison is separate from the product comparison.
Under ADR-0027 every grammar sets `regenerate: true` unless its recorded exception retains the reference C.
Both builders use the shared, pinned CLI path: independent generation twice, byte comparison, input-tree hash and
generated-file hashes in provenance. A grammar's optional `generateFrom: "grammar.json"` selects its archived,
unmodified `<grammarDir>/grammar.json` instead of evaluating `grammar.js`; its original hash and selection enter
provenance, identically for both builders. Swift uses this input and has a narrowly ratcheted CLI upstream-test
exception accepted by the owner; see [ADR-0027](../design/decisions/ADR-0027-one-pinned-generator-cli.md#swift-amendment-owner-2026-11-17).
Grammar-input patches apply before generation and C/header patches after it.
External npm grammar inputs come from the upstream lockfile, are integrity-checked and extracted without install
scripts, and are recorded in provenance. The original archive/tree/file pins continue to identify the archived
bytes; generated C hashes are additional identities. CLI or grammar moves require the ADR-0027 native behaviour
gate, including the grammar repository's own tests with isolated compiled-language caches; schema changes enter the schema register.

## Machine-readable source of truth

```
tools/gen/pins.json                 runtime release + archive/tree sha256, grammar repos + full commits, temporary patch records, tool versions
grammars/<lang>/provenance.json     identity of one generated module: inputs and their sha256, recipe sha256, outputs
grammars/<lang>/language.go         the same identity compiled in (engine.Identity)
```

The JSON is authoritative; prose that disagrees is a defect. A test in each grammar module checks that
`provenance.json` and the compiled `engine.Identity` agree, and `tools/gen` refuses to generate from an input whose
sha256 differs from the pin. CI (`.github/workflows/regen.yml`) regenerates every module on Linux and requires the
committed bytes.

## Versions

- **Runtime:** follows the latest stable tree-sitter release. A release is adopted when the generated build agrees
  with a native C build of the same release on the reference corpus (`docs/specs/oracle.md`); target within four
  weeks of the release. Adoption updates `tools/gen/pins.json`, regenerates every grammar module and records old and
  new results side by side in a handoff.
- **Grammars:** the grammar set and commits are fixed per library release. A grammar change ships with a static
  node-type (schema) diff.
- **Engine ABI:** `internal/engine.ABIVersion` is the newest version; supported older ABIs are listed in
  `docs/design/engine-abi.md`. Unsupported versions are rejected at `NewParser`.

## Previous runtime (history)

The previous product runtime — the `internal/runtime` carrier of `gotreesitter` v0.56.0 with its patches and its
provenance files — was removed by ADR-0022 and remains in Git history at `9ad82eb`. No result is carried between the
two engines.

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
