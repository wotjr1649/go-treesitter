# ADR-0023 — Temporary, retirable patches to upstream grammar C

**Status:** Accepted by the owner · 2026-11-14 · amends ADR-0020 §1 and §4 ("upstream grammar sources are used
unmodified") for grammar sources only. The tree-sitter runtime stays unmodified.

## Context

Wave 2 found two upstream grammar defects that block release gates while the engine agrees with native C:

- **tree-sitter-yaml** (pin `a1c4812a`, upstream master): its generated `src/parser.c` accepts a zero-width
  `end` token in lex state 0 before checking end of input, because the grammar has no internal tokens. When the
  external scanner returns no token mid-input, the parse "ends" there and `recover_eof` drops the rest. Native C and
  CLI 0.27.2 behave the same; 16 of 131 corpus inputs get a root that ends before the input, and inputs with a NUL
  byte can be truncated without any error. Our contract rejects such trees (`EngineFault`). Upstream issues
  tree-sitter-yaml #12, #14, #16 are open without a root cause. Evidence: `.scratch/s15/p24/P24-report.md`.
- **tree-sitter-kotlin** (pin `3dea6dfa`): the external scanner's string-content branch scans hundreds of thousands
  of characters per call in error mode; 2.87x native on a large generated file. Evidence: `.scratch/s15/p23/`.

Waiting for upstream leaves these grammars unavailable indefinitely; relaxing our contract or criteria was rejected.

## Decision

1. A grammar may carry a **temporary patch** to its upstream C (`parser.c`, `scanner.c`, headers) when an upstream
   defect blocks a release gate and the defect is reported upstream. The runtime is never patched.
2. Patches live in `tools/gen/patches/<grammar>/NNNN-<topic>.patch`. `pins.json` records, per patch, the upstream
   file's sha256 before and after, and the upstream report URL. `tools/gen` applies them after the pinned source is
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

## Consequences

- YAML can ship with a one-line patch to its generated `parser.c` (do not accept `end` before the end-of-input
  check in lex state 0), after the unpatched comparison and the release gates.
- Kotlin stays held for now (owner decision: upstream PR first); this ADR allows a later decision to patch it.
- AGENTS.md's rule "never edit upstream C" now reads "never edit upstream C except through a recorded patch under
  ADR-0023".
