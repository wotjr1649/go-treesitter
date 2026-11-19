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
   The [2026-11-17](#amendment-2026-11-17-decision-1-postgresql-korean-support) and
   [2026-11-18](#amendment-2026-11-18-decision-1-scope-patch-limit-and-review) amendments widen which defects
   qualify; the 2026-11-18 amendment also limits the patches per grammar and adds a 12-month review.
2. Patches live in `tools/gen/patches/<grammar>/NNNN-<topic>.patch`. `pins.json` records, per patch, the upstream
   file's sha256 before and after, the patch's sha256, and the upstream report URL. `tools/gen` applies them after the pinned source is
   verified and before compiling; a patch that does not apply to the pinned bytes is an error.
3. **Disclosure.** The generated module's `provenance.json` lists every applied patch (file, sha256s, report URL);
   the module README and `docs/reports/limitations.md` say the grammar differs from upstream and why.
4. **Evidence.** The native C reference is built from the same patched source, so C agreement keeps meaning
   "same C as the engine". Each patched grammar also runs an **unpatched comparison** (native unpatched vs native
   patched) over its corpus and fuzz seeds; the report must show which inputs change. A patch must leave every
   error-free tree of the unpatched grammar unchanged unless the change is the documented fix.
   For each post-generation scanner patch, the conformance lane checks a reviewed
   `tools/gen/patches/<grammar>/scanner-expectations.json` entry. Both native builds use the same generated
   parser, grammar-input patches, and preceding C patches. The comparison adds shared regression inputs to
   the pinned corpus and fixed fuzz inputs, and requires exact input and fresh-dump signatures, including
   the complete set of changed cases. A change to a previously error-free tree needs a reason tied to the
   individual case. A crash of the original scanner is accepted only as a reviewed case recorded with its
   platform and crash class, and the patched build must still match its signature. A candidate crash,
   timeout, incomplete dump, or mismatched signature fails the check. The YAML early-EOF parser patch keeps
   the existing whole-stack comparison. The scanner check covers fresh parsing; native edits retain their
   conformance coverage.
   Review expectation updates with the affected patch, pin, or fixture change. Pin moves rerun the comparison
   for each affected patch. Remove the associated entry when the patch retires and review any remaining entries
   whose build or inputs changed. CI reads the file and fails on stale or missing entries.
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
   The 2026-11-18 amendment (item 3) caps a grammar at five patch records.

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

## Amendment (2026-11-18): decision 1, scope, patch limit and review

Several pinned upstreams have had no commit for a year or more while known defects and their fixes wait in open
issues and pull requests. The owner extends decision 1 so that such fixes are carried without waiting for upstream
(ADR-0026 S4), within a bounded burden.

1. **Qualifying defects.** Besides a defect that blocks a release gate, a temporary patch may fix an error-free
   misparse (a tree without `ERROR` or `MISSING` nodes whose structure is wrong), Korean or other non-ASCII input
   the grammar rejects or mis-scans, common input of the language that the grammar rejects (`ERROR` or `MISSING`
   nodes), or newer language syntax that the grammar does not accept. The pull request states why the input is
   common (for example the language's documentation or widely used code) and, for a misparse, the specification or
   reference behaviour it follows.
2. **Upstream report.** When a new patch is added, an open upstream issue or pull request, by any author, must
   describe the defect; its HTTPS URL is the patch record's `upstreamReport`. If none exists, a report is posted
   first (decision 6: separate publication approval). The patch may carry an upstream pull request's change adapted
   to the pinned source or, when no such pull request exists, our own fix.
3. **Limit.** A grammar carries at most five patch records, as `gen -patch-status` counts them. Each record changes
   exactly one file; by default it addresses one upstream report. Bundling several reports that change the same
   file into one patch needs the owner's explicit approval of the bundling, stated in that patch's pull request. A
   bundled patch records one of its reports as `upstreamReport`; `tools/gen/patches/<grammar>/README.md` and the
   grammar's `docs/reports/limitations.md` row list every bundled report URL, and decision 5 and item 4 apply to
   each of them (when upstream incorporates one, the pin move reduces the patch to the remaining reports). The
   `-patch-alert` default stays 3: adding a fourth or fifth patch record requires the owner's source-tier review
   (ADR-0026 S2), recorded in the pull request that adds the record. More than five requires amending this ADR.
4. **Review.** A patch that upstream has not incorporated within 12 months after a patch for its upstream report
   first reached `main` is re-decided by the owner, for example removing it and recording the defect as a known
   gap or moving the grammar's source (ADR-0024); if the owner keeps it, the next review is 12 months later.
   Renaming, rewriting or replacing the patch file does not restart the period. Decision 5 still applies when
   upstream releases a fix or closes the report without one.
5. **Breaking changes.** Adding, changing or removing a patch (including retirement) so that node types or fields,
   or the trees of inputs that parsed without errors, change is a breaking change for users. Besides decision 4, its
   pull request updates the schema register (`docs/validation/grammar-schemas.md`) and `MIGRATING.md`, and checks
   that the upstream queries the conformance lane uses still compile and pass that lane with the patched grammar;
   that lane does not show unchanged captures, so `MIGRATING.md` describes query-visible changes.
6. **Evidence.** Decisions 2–4 and 7 apply unchanged. The ADR-0027 generator gate applies a C or header patch to
   both of its sides and so does not test the patch's effect; that evidence is the decision 4 unpatched comparison,
   also run over regression inputs that exercise the fix (the patch corpus or retained conformance tests) so that
   the documented fix appears among the changed inputs, and native agreement on those inputs. A grammar-input patch
   also runs the generator gate; its reviewed differences, if any, are recorded in
   `tools/gen/patches/<grammar>/generator-expectations.json` (ADR-0027).
7. **Approval.** This amendment approves no individual patch; decision 6 applies to each. The 2026-11-17 PostgreSQL
   amendment remains the record of those two patches, which also meet items 1–3 of this amendment.

The owner set this direction and the limit on 2026-11-18; merging the pull request carrying this amendment records
the owner's approval of its text.

## Consequences

- YAML can ship with a one-line patch to its generated `parser.c` (do not accept `end` before the end-of-input
  check in lex state 0), after the unpatched comparison and the release gates.
- Kotlin no longer needs a patch: ADR-0024 moved it to `fwcd/tree-sitter-kotlin`, which passes the gates unmodified.
  The fix prepared for the kit's repository was posted upstream as a contribution.
- AGENTS.md's rule "never edit upstream C" now reads "never edit upstream C except through a recorded patch under
  ADR-0023".
