# Workload and fixture register

Authoritative inventory of every input used to make a claim. Identity rules live in
`docs/specs/baseline-provenance.md`; this file holds the values.

## Current workloads (engine, ADR-0020/0022)

The tiers below this section record the removed `gotreesitter` carrier's inputs and oracle records (`testdata/`,
`windows-c*`); they are **historical** (Git history at `9ad82eb`) and no current test or gate reads the oracle
digests they describe. The current lanes use:

| Workload | Where | Used by |
|---|---|---|
| Fresh corpus (1,586 inputs, 7 grammars), edit cases (5,919), large inputs (12), UTF-8 cases | local tree `$TSG_S15_DIR` (`corpus/`, `edits/`, `large/`, `native/dumps/`); not tracked | `tools/conformance`, `tools/bench` |
| Native C reference dumps, edit dumps and changed ranges for those inputs | built and recorded with `tools/native-ref` from `tools/gen/pins.json` | the same |
| Built-in fuzz seeds (one small sample per grammar) | `tools/conformance/fuzz_test.go` (`FuzzParse<Lang>`); crashers go to `tools/conformance/testdata/fuzz/` | `go test -fuzz` |
| Embedded samples | each `grammars/<lang>` module | its own tests |

The remaining fixed-oracle catalogs were removed after `be31f49f3e4d645a929a9aebc961be150de16e37`, the last
commit containing them. They remain available in Git history; catalog filenames below refer to that retired
directory. The older C observations and digests remain in Git history at `9ad82eb`.

The identity rules below still apply: bind every result to the pinned runtime and grammar identities.

## Wave-2 batch 1 large inputs (Q7)

`tools/bench/ci/large.json` pins one real source file for each new grammar. The fetch command verifies both the
source SHA-256 and its license SHA-256, and writes the license beside the input as `<file>.LICENSE`.
`-langs` selects the same grammar for fetch and measurement. Files are not concatenated or enlarged.

Q7 gate measurements used `tools/gen/pins.json` SHA-256
`34bb8edcd9bd17c44aabad17e20d1836f0f527e890fd491d918133a3c21cb514`, before YAML was removed.
After that removal the pins file is `c17b1b5e7b0c6f024284ee68f33bbbf3622cdef7810e9c0ee9b9e58807902af4`;
the eight grammars listed below retain identical pins and generated module bytes. The recorded gate results
keep their original measurement identity.

| Grammar | Source | Bytes | SHA-256 | License |
|---|---|---:|---|---|
| javascript | [typescript.js](https://registry.npmjs.org/typescript/-/typescript-6.0.3.tgz#package/lib/typescript.js) | 9,144,216 | `569177652966bd528c319171c7dd22860dbf72bde116cbc4f644f1d02bb12e39` | [Apache-2.0](https://raw.githubusercontent.com/microsoft/TypeScript/050880ce59e30b356b686bd3144efe24f875ebc8/LICENSE.txt) |
| html | [dom_sort.html](https://raw.githubusercontent.com/DataTables/DataTables/f60c41290255c3fcab38999161f9529226b655c9/examples/plug-ins/dom_sort.html) | 57,738 | `8eed4937dc497a478d12199dfbe0fa2aa070ae5104ccc5f18e92c4d5235c9c53` | [MIT](https://raw.githubusercontent.com/DataTables/DataTables/f60c41290255c3fcab38999161f9529226b655c9/license.txt) |
| css | [bootstrap.css](https://raw.githubusercontent.com/twbs/bootstrap/f9a7a8941e7c1c91b0cae8a0c906537c26086ae2/dist/css/bootstrap.css) | 496,025 | `ad0d5123040e043dad1451a6d4483dad4df4e2835e6687d6cf5af8a757283371` | [MIT](https://raw.githubusercontent.com/twbs/bootstrap/f9a7a8941e7c1c91b0cae8a0c906537c26086ae2/LICENSE) |
| java | [Proto3GencodeTestProto.java](https://raw.githubusercontent.com/protocolbuffers/protobuf/1633873a4e9a6cf3df2e0f0d69cb2693b8f006b5/compatibility/smoke/v25.8/legacy_gencode_test/proto3/Proto3GencodeTestProto.java) | 909,661 | `80413d76f6b02e978a4d5a569008371b192f1e219a2802c53bdb1083f4d8dfbe` | [BSD-3-Clause](https://raw.githubusercontent.com/protocolbuffers/protobuf/1633873a4e9a6cf3df2e0f0d69cb2693b8f006b5/LICENSE) |
| c | [openssl.c](https://raw.githubusercontent.com/curl/curl/4f2b5fba5212a32c4e4a889492cc153e1220e619/lib/vtls/openssl.c) | 175,997 | `c542b52edee1dbd827ede82f10fb21519674ebe93ec335c56d2cec20f39d81aa` | [curl](https://raw.githubusercontent.com/curl/curl/4f2b5fba5212a32c4e4a889492cc153e1220e619/COPYING) |
| rust | [ast.rs](https://raw.githubusercontent.com/rust-lang/rust/5f4effd579500175af844241eb179e756190f1e7/compiler/rustc_ast/src/ast.rs) | 154,408 | `09609e4e241bc21662722f6423437b53fc2fabe9f711621806b94c9cde0f84f5` | [MIT](https://raw.githubusercontent.com/rust-lang/rust/5f4effd579500175af844241eb179e756190f1e7/LICENSE-MIT) |
| bash | [nvm.sh](https://raw.githubusercontent.com/nvm-sh/nvm/09e620c401382f4b3d3147333dd64c1e09188fdb/nvm.sh) | 243,120 | `caca749b75b598322d6f03674f3440e99a7efb8c4f60265750b6d867b3a1d65b` | [MIT](https://raw.githubusercontent.com/nvm-sh/nvm/09e620c401382f4b3d3147333dd64c1e09188fdb/LICENSE.md) |
| cpp | [json.hpp](https://raw.githubusercontent.com/nlohmann/json/81888ff9f005c022e60f2de572327b2e50222222/single_include/nlohmann/json.hpp) | 1,929,376 | `686e19239b9135b5822f96876fd59de90bde335d598c2e71ded84201d6f5e584` | [MIT](https://raw.githubusercontent.com/nlohmann/json/81888ff9f005c022e60f2de572327b2e50222222/LICENSE.MIT) |

The Java input is real generated protobuf compatibility code,
and C++ is nlohmann/json's single-header distribution (the Q6 input, retained unchanged). These files represent their
specific shapes, not all applications in each language. HTML, CSS, Java, C, Rust, and Bash are below 1 MB; each
exceeds the small-file lane's 25 KB boundary. All source bytes are LF and their hashes refer to the downloaded bytes.
CSS, C, Bash, and C++ inputs contain syntax the pinned grammars recover from on both native C and Go; their outcomes
must remain visible in the evidence.

## Rules

- Every fixture has a recorded SHA-256 **of its LF bytes** and a recorded license/provenance.
- A hash mismatch fails the run. Never update a hash without stating why in the same commit.
- Inline fixtures (Go string literals) need no hash — the source file is the hash. Prefer inline for
  anything under ~1 KB.
- External fixtures are copied in with their license file. Never read them live from `_ref` or from
  `code-map-memo`; those are read-only and may move.
- `.gitattributes` must pin `*.cs`, `*.ts`, `*.tsx`, `*.js`, `*.jsx`, `*.py`, `*.go` fixtures to LF
  and mark `*.bin` as `-text`, before the first external fixture lands.

## Tier 1 — smoke (inline, present from the foundation session)

Purpose: prove the grammar loads, the adapter maps outcomes correctly, and the product lane builds.
Not parity. Not performance.

| ID | Language | Shape |
|---|---|---|
| `SM-GO` | go | package + func with a short statement |
| `SM-PY` | python | def + if + comprehension |
| `SM-JS` | javascript | arrow function returning JSX |
| `SM-JSX` | javascript (`.jsx`) | same input routed by extension |
| `SM-TS` | typescript | interface + generic arrow function |
| `SM-TSX` | tsx | component returning JSX |
| `SM-CS` | c_sharp | class with a property and an expression-bodied member |

Each runs: fresh parse → assert outcome; trivial append edit → incremental parse → assert
`incremental == fresh` **and** record the reuse/fallback reason. The equality assertion is
self-consistency, not parity (`docs/specs/oracle.md`).

## Tier 2 — known-regression ratchet (inline)

`KR-0001` fixtures F1–F7, each run against both `tsx` and `javascript`.
See `docs/validation/known-regressions.md`.

## Tier 3 — pinned public corpus (external; lands when a comparison session is authorized)

Newtonsoft.Json @ `4f73e74372445108d2c1bda37b36e6f5e43402e0`, MIT (James Newton-King).
The license file must be copied alongside the sources. Session 05 stores them
under `testdata/newtonsoft/`; `manifest.json` and the nested comparison harness
check all three LF hashes and the license before any measurement.

| File | LF bytes | SHA-256 |
|---|---:|---|
| `JsonPosition.cs` | 5,986 | `4f9e601f9d0be45c4f60f15bf1b6ca8f85dec51e353ebb50e405cc6f8e422c14` |
| `JsonTextReader-excerpt.cs` | 12,408 | `d76fd62cfc90076c11d86cb7d7a0058df181231aa3b34f30e549f650b5294d4a` |
| `MathUtils.cs` | 5,016 | `426587acd7f1f50c3d3dc94f13e9c27726c32c1bbe22d0291d86485afa368640` |

Total 23,410 bytes. `JsonTextReader-excerpt.cs` is also an upstream `gotreesitter` regression
fixture; the LF blob is 12,408 bytes while the same file in a Windows working tree with
`core.autocrlf=true` is 12,785 bytes. **Use the LF bytes.** This is the concrete reason
`.gitattributes` is mandatory.

## Tier 4 — deterministic generators (code, not files)

Generators are reproducible inputs defined in test code, with their parameters recorded.
Never check in their output.

| ID | Shape | Parameters |
|---|---|---|
| `GEN-CS-PATTERN` | C# classes with `catch (…) when (e is T Target)` filter patterns | method count 1 / 4 / 8 / 16 |
| `GEN-CS-BULK` | repeated numbered C# classes with `var x<i> = <i>; return x<i>;` | target ≥ 137 KiB; edit site = first `x0` |
| `GEN-GO-LOOKUP` | `package p` plus numbered `var item<i> = <i>` declarations | 2,048 declarations, LF; `BenchmarkSnapshotLookup`; five fixed samples of 200 operations in Session 07 |

`GEN-CS-BULK` matches the generator upstream itself uses for its C# recovered-structure parity
witness. Keep it aligned with upstream's shape so results stay comparable.

## Tier 5 — historical fixed-oracle catalogs and digests (Git history only)

These digests were produced only in a declared oracle lane (`docs/specs/oracle.md`), one per
(language, fixture, oracle epoch). The retired carrier's Windows product lane compared against them
without a C toolchain. Each digest recorded its oracle tuple; a tuple mismatch was `NOT_RUN`.
The current lanes build a native C reference from the same pins, as listed above.

Session 06 adds `cases.json`: 50 hashed inputs covering the seven
smoke routes, both grammars' F1–F7, 24 JSX boundary controls, two generic-arrow
controls, and the three licensed Tier-3 C# files. Inline sources are authored in
this repository; C# sources retain their existing MIT origin and license.
The JSON identifies each exact UTF-8 byte sequence without newline conversion.

`windows-c/` holds one ordered C record per input plus its build
manifest. These are oracle observations, not assertions that the Go runtime
agrees. Every comparison must check identities and completeness before trees.

Session 07 adds a separate `extended-cases.json` catalog with
35 repository-authored fixed inputs (30,072 UTF-8 bytes in total). For each of
the seven routes it contains a UTF-8/CRLF base, length-changing identifier
rename, UTF-8 comment-row insertion, trailing-CRLF deletion, and a fixed
513-comment sibling boundary input. CRLF is intentional source content encoded
inside LF JSON; source SHA-256 checks include those CR bytes. The TypeScript
base includes `type T = typeof a.b` to exercise field/alias edges. The three
edit variants identify their exact previous fixture and byte ranges.

`windows-c-extended/` stores the independently executed Session
07 C records and their own build identity. Product tests compare 35 fresh
results and 21 incremental results against this set. This catalog does not
change or widen any known-regression record. Bulk performance generators remain
code-defined under Tier 4 rather than checked-in generated corpora.

Session 07's separate `typescript-contextual-cases.json` adds eight authored LF
inputs under this repository's MIT license. Each route (`.ts`, `.tsx`) contains
newline-separated `in`, optional `in?`, binary `in` across a newline, and a
semicolon-separated control. Each exact source has a SHA-256 in the catalog.
`windows-c-ts-contextual/` retains fresh C observations from Phase 2. Four
differences are independently bound by KR-0003; the other four must agree.

After explicit Candidate C adoption (ADR-0012), `windows-c-v2/` contains new
native C executions for all four existing catalogs. The contextual cases now
require exact agreement; original observations remain historical. The inline
`TestTypeScriptMaintainedGrammarFeatures` covers the four patch features in
both routes with 20 fresh repeats and one edit each. The separate 26-input
Candidate C manifest, source hashes, full trees and runtime comparisons are
retained in the local investigation archive. The approved public inputs and hashes
are under `typescript-patched/` and `internal/provenance/identities.json`.

`csharp-recovery-cases.json` holds the single `CS-excerpt-no-enum` derivative:
remove the original excerpt's `internal enum` section through the next summary
comment, retaining all other LF bytes. It retains the Newtonsoft MIT notice in
`testdata/newtonsoft/LICENSE.md` and has its own exact source hash. The native C
record is in `windows-c-cs-recovery/`; KR-0004 owns its distinct signature.

## C# full-pass hardening

The full-pass hardening adds `csharp-order-cases.json` (33 records, 32 edits)
and `csharp-preservation-cases.json` (41 records, 40 edits). Each catalog includes
the same clean edit origin and records exact source hashes and edit bytes.
The cases cover conditional enum members, closed recovery prefixes, accept
ordering, pointer/declaration ambiguity, comments, renaming, Korean identifiers,
CRLF and leading hidden whitespace. C records are in the corresponding
`windows-c-v2/cs-order` and `windows-c-v2/cs-preservation` directories. The 72
target inputs were compared against three native C runs and repeated in Go
100 times for recovery inputs or 20 times for clean inputs. Native C edits were
also compared against fresh C trees for all 72 targets.

Minimal reductions and the original real-world inputs derive from the existing
Newtonsoft MIT fixtures, whose notice remains in `testdata/newtonsoft/LICENSE.md`.
Other inline cases and transformations were authored in this repository. The
JSON file itself uses LF; intentional CRLF source bytes are encoded in strings.

## C# constructs of the new pin

`csharp-constructs-cases.json` (16 records, 7 edits) covers syntax that
tree-sitter-c-sharp `9150f7d5` added or widened: lambda parameter lists,
`async`/`static` lambdas, collection expressions and spreads, extension
blocks, list and slice patterns, compound-assignment operator declarations
and null-conditional assignment, plus two erroneous lambdas. C emits one
`parameters` child per lambda in every record. Each edit changes
the construct itself. `csharp-kr0005-cases.json` holds the erroneous lambda
that first differed from C (KR-0005, now retired). `csharp-lambda-modifier-cases.json` (7 records, 2 edits,
2 erroneous) covers implicit lambda parameters with `ref`, `out`, `in`,
`scoped` and `readonly`. In the grammar only that rule gives
`lambda_expression.parameters` several children; C does so in 6 of the 7
records and parses the unclosed variant with a single parameter. C records
are in `windows-c-v2/cs-constructs`, `windows-c-v2/cs-kr0005` and
`windows-c-v2/cs-lambda-modifiers`, recorded once with the retained C build
`e00a14ff…`. All inputs were authored in this repository under its MIT
licence; no third-party text is copied. The JSON uses LF.

## Generic-call ambiguity

`csharp-generic-call-cases.json` (11 records, 1 edit) and
`typescript-generic-call-cases.json` (5 records) hold clean inputs where
`<...>(` can be a generic call or two comparisons: shapes that differed from C
(KR-0006, now retired), the edit, and controls that already agreed, including a
declaration expression and parenthesized comparisons. C records are in
`windows-c-v2/cs-generic-call` and `windows-c-v2/ts-generic-call`, recorded once
with the retained C build `e00a14ff…`. All inputs were authored in this
repository under its MIT licence. The JSON uses LF.

## Go edit-fuzz recovery

The Go edit-fuzz regression adds `go-recovery-cases.json`: four clean edit
origins and 24 recovery variants (plain, leading newline, header comment and
CRLF, each with six ASCII/UTF-8 corruptions of `package`). All are authored
under this repository's MIT license, with exact source hashes and edit ranges.
`windows-c-v2/go-recovery/` contains all 28 native C records. The minimized
`FuzzIncrementalAgreement/7596da024a0d8a5b` input remains in the adapter corpus.
Twenty Go repeats and three C executions per recovery variant agreed after
the Go version-order patch; 24 native C edits also agreed with fresh C.

## Shared EOF, token-width and edit hardening

`go-keyword-cases.json` retains 34 authored inputs and 27 edits for reserved-word
recovery. `runtime-hardening-cases.json` retains 458 authored inputs, including
163 distinct clean origins and 295 edits. They cover Go EOF/newline/NUL,
ASCII and Unicode leading prefixes, JS-family pending lookahead, malformed
strings, JSX text punctuation and bare-ampersand controls. Deduplication uses
the full filename/source/previous/edit tuple; source hashes remain per input.
All authored inputs use the project's MIT license. Encoded CRLF and NUL bytes
are intentional; JSON itself uses LF.

The corresponding `windows-c-v2/go-keyword` and `runtime-hardening` sets contain
new native C executions. `TestGoRecoveryKeywordOracleRecords` and
`TestRuntimeHardeningOracleRecords` enforce exact ordered fresh/edit/C trees.
The final nine-catalog campaign covers 788 fresh inputs and 439 edits, with
20 Go repeats for clean inputs, 100 for error inputs, three C fresh runs per
input, and one native C edit per catalog edit. Receipts are under
the local investigation archive; selected source-bound observations are in
`artifacts/release-candidate/historical/` and bind their actual carrier identity.
The minimized failures remain in `FuzzIncrementalAgreement`'s corpus.

`BenchmarkRelease` generates 128 declarations for each of the seven paths and
also uses the existing C# recovery excerpt. It separates full parse, replace,
insert, delete, no-edit, snapshot, index construction and position lookup.
Recovery snapshot materialization is included in full parse because the adapter
releases non-clean backend handles before returning. Five fixed ordering seeds
and 40 operations per benchmark are declared before execution in the performance
evidence; these are measurements, not universal latency guarantees.

`TestLargeGoIncrementalConflict` generates 8,192 numbered Go variable declarations
and appends one newline. It requires exact incremental/fresh snapshots under a
64 MiB runtime footprint budget, retaining actual old-tree reuse and bounded
stack depth. Before the leaf-conflict correction, the old-tree attempt exhausted
its scratch budget and retried fresh. The same generator participates in the
193-cycle retention workload alongside the seven 128-declaration workloads and
the existing C# recovery excerpt. Process RSS and post-GC live heap are recorded
separately from parser arena/scratch receipts.

## What is deliberately absent

- No fixture is copied from `code-map-memo`'s `.work/` run directories at runtime. If bytes are
  needed, they are copied in once, hashed, and licensed here.
- No repository-wide corpus. Every workload is named, bounded, and justified by a claim it supports.
