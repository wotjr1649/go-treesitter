# ADR-0023 — Temporary, retirable patches to upstream grammar sources

**Status:** Accepted by the owner · 2026-11-14 · amends ADR-0020 §1 and §4 ("upstream grammar sources are used
unmodified") for grammar sources only. The tree-sitter runtime stays unmodified.

## Context

Wave 2 found two upstream grammar defects that block release gates while the engine agrees with native C:

- **tree-sitter-yaml** (pin `a1c4812a`, upstream master): its generated `src/parser.c` accepts a zero-width
  `end` token in lex state 0 before checking end of input, because the grammar has no internal tokens. When the
  external scanner returns no token mid-input, the parse "ends" there and `recover_eof` drops the rest. Native C and
  CLI 0.27.2 behave the same; 16 of 131 corpus inputs get a root that ends before the input, and inputs with a NUL
  byte can be truncated without any error. Our contract rejects such trees (`EngineFault`). Upstream issues
  tree-sitter-yaml #12, #14, #16 are open without a root cause. Evidence: `.scratch/s15/p24/P24-report.md` (maintainer-local evidence archive 2026-11-16-s17-cleanup,
  s15/p24/P24-report.md; not in this repository).
- **tree-sitter-kotlin** (pin `3dea6dfa`): the external scanner's string-content branch scans hundreds of thousands
  of characters per call in error mode; 2.87x native on a large generated file. Evidence: `.scratch/s15/p23/` (maintainer-local evidence archive 2026-11-16-s17-cleanup, s15/p23/; not in
  this repository).

Waiting for upstream leaves these grammars unavailable indefinitely; relaxing our contract or criteria was rejected.

## Decision

1. A grammar may carry a **temporary patch** to its upstream C (`parser.c`, `scanner.c`, headers) or `grammar.js`
   and related grammar inputs when an upstream
   defect blocks a release gate and the defect is reported upstream. The runtime is never patched.
2. Patches live in `tools/gen/patches/<grammar>/NNNN-<topic>.patch`. `pins.json` records, per patch, the upstream
   file's sha256 before and after, the patch's sha256, and the upstream report URL. `tools/gen` applies them after the pinned source is
   verified and before compiling; a patch that does not apply to the pinned bytes is an error.
3. **Disclosure.** The generated module's `provenance.json` lists every applied patch (file, sha256s, report URL);
   the module README and `docs/reports/limitations.md` say the grammar differs from upstream and why.
4. **Evidence.** The native C reference is built from the same patched source, so C agreement keeps meaning
   "same C as the engine". Each patched grammar also runs an **unpatched comparison** (native unpatched vs native
   patched) over its corpus and fuzz seeds; the report must show which inputs change. A patch must leave every
   error-free tree of the unpatched grammar unchanged unless the change is the documented fix.
5. **Retirement.** When upstream releases a fix, the pin moves to it and the patch file is deleted in the same
   change. A patch whose upstream report is closed without a fix is re-decided by the owner.
6. Every new patch needs the owner's approval; posting the upstream report needs separate publication approval.
7. **Grammar regeneration (owner extension, 2026-11-15).** A grammar-input patch sets the grammar's
   `regenerate: true` and runs the tree-sitter CLI pinned in `pins.json` with `generate --js-runtime native`.
   The per-platform compressed CLI archive and the executable's equality to its decompressed bytes are mandatory
   checks. Two independent copies of the patched source tree are generated and every file is compared; any
   difference fails. The shared `tools/gen/grammarpatch` implementation supplies both the engine generator and
   native reference with those C bytes. Provenance records patch identities, patched input-tree hash, CLI
   version/runtime and generated source hashes; it excludes host paths and host-dependent binary hashes.
   Existing C-only patch records, identities and provenance retain their original format.
8. **Patch burden.** `gen -patch-status` reports every grammar's patch count, including zero. Generation and native
   reference builds print the selected grammar's count and warn above `-patch-alert` (default 3; shell entry points
   accept `PATCH_ALERT_THRESHOLD`). This warning is a review signal, never an approval for another patch.

## Amendment (2026-11-17): decision 1, PostgreSQL Korean support

For the pinned `gmr/tree-sitter-postgres@59d0d8cd7506d68de1229fb4bbce838c83b60c8a`, the owner authorizes temporary,
retirable patches that admit non-ASCII code points in unquoted identifiers and preserve complete code points in
dollar-quote tags. This narrow exception is motivated by Korean support even though these defects do not block a
release gate. The owner posted [upstream issue #101](https://github.com/gmr/tree-sitter-postgres/issues/101) for both
defects; both patch records use that HTTPS URL. All other ADR-0023 requirements remain in force, including recorded
source/patch hashes, disclosures, pinned deterministic regeneration, comparison against unpatched native
corpus/fuzz trees, native agreement on the patched sources, and retirement when upstream incorporates the fixes.
Grammar inputs are patched before generation; the scanner is patched after generation. The tree-sitter runtime
is not patched.

The owner approved this exception on 2026-11-17; merging the pull request carrying this amendment records the
owner's approval of its text.

## Consequences

- YAML can ship with a one-line patch to its generated `parser.c` (do not accept `end` before the end-of-input
  check in lex state 0), after the unpatched comparison and the release gates.
- Kotlin no longer needs a patch: ADR-0024 moved it to `fwcd/tree-sitter-kotlin`, which passes the gates unmodified.
  The fix prepared for the kit's repository was posted upstream as a contribution.
- AGENTS.md's rule "never edit upstream C" now reads "never edit upstream C except through a recorded patch under
  ADR-0023".
