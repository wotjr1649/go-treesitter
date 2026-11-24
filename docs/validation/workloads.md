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
| Known upstream grammar defects (5 minimal inputs, Python and C#) | [`register.json`](../../tools/conformance/testdata/known-gaps/register.json): exact JSON input strings, input/dump SHA-256, spec citations and source/module identities; adjacent `.dump` files | `TestKnownGaps` (engine and live native reference), `TestKnownGapRatchet`, `TestKnownGapDocs` |
| Published grammar consumers | [Consumer tests](../../testdata/consumer-compat/) | `engine-consumer-compat` |

The published grammar consumers use these input bytes. The job records query hashes and expected captures with the results.

| Grammar | Input | SHA-256 |
|---|---|---|
| JSON | `{"a":1}` | `015abd7f5cc57a2dd94b7590f04ad8084273905ee33ec5cebeae62276a97f862` |
| JSON edit | `{"a":true}` | `5daa0644c4a1c43ea018f7f1ef2944b90e0165ef1f2d5669ca89e5d4f69ac597` |
| C++ | `cpp/expressions-025`, with leading and trailing LF. [Pinned corpus case](https://github.com/tree-sitter/tree-sitter-cpp/blob/c009222808634c1014f82438d4883753516a2c24/test/corpus/expressions.txt) · [MIT license](https://github.com/tree-sitter/tree-sitter-cpp/blob/c009222808634c1014f82438d4883753516a2c24/LICENSE) | `f6b8b31f177bb4196f78e85d17f0cbe7b797d898a12f01ebc4abebb1f739e87e` |
| Go | `package p\nfunc f() { g(1) }\n` (escaped LF) | `2fcb456ae23480a0523865b090f5011876af69cb89becd2d9cb9fef918ba276d` |

The JSON and Go inputs and the queries are original fixtures under this repository's MIT license.

The known-gap inputs are original minimal reproductions written for this repository (repository MIT license),
motivated by [grammar-kit handoff B](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/0a12156/docs/reports/repository-closure-handoff-b.md).
No kit code or expectations are copied. Input bytes come from decoded JSON strings; only the Python await case has
a final LF, and the C# EOF directive deliberately has none. Dumps are LF. Per-grammar consumer documents are generated
from that register with `go generate ./...` in `tools/conformance`; `TestKnownGapDocs` regenerates in memory and
compares all committed bytes, including detecting obsolete documents. CI runs all three tests in every grammar shard;
the entries execute only in the shards of the grammars that have them (Python, C#), so a shard of any other grammar
finishes `TestKnownGaps` with a skip.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from `pins.json`, including patch records;
an unrelated grammar pin move does not invalidate these entries. A changed identity of one grammar fails that
grammar's shard and `TestKnownGapDocs` (not the other shards); the failure prints the recomputed hashes, and the
register identity is updated only after a fresh engine/native reproduction of every entry. `TestKnownGaps` needs no
corpus directory; without `TSG_NATIVE_EXE_DIR` only its native subtests skip (E3). With `TSG_NATIVE_EXE_DIR` set, a
missing executable fails; with `TSG_REQUIRE_FIELD_LOOKUP=1` (CI), an unset variable or a selected but unlinked
grammar fails too, and a shard whose grammar has entries must run at least one. CI builds the reference from the current
pins and sets the variable, so publication evidence is a retained E5 differential, scoped to these exact inputs.
The E5 summary logs input/dump/executable hashes, source/module identities and the executing OS/architecture.

The remaining fixed-oracle catalogs were removed after `be31f49f3e4d645a929a9aebc961be150de16e37`, the last
commit containing them. They remain available in Git history; catalog filenames below refer to that retired
directory. The older C observations and digests remain in Git history at `9ad82eb`.

The identity rules below still apply: bind every result to the pinned runtime and grammar identities.

## Wave-2 batch 1 large inputs (2026-11-14)

`tools/bench/ci/large.json` pins one real source file for each new grammar. The fetch command verifies both the
source SHA-256 and its license SHA-256, and writes the license beside the input as `<file>.LICENSE`.
`-langs` selects the same grammar for fetch and measurement. Files are not concatenated or enlarged.

The batch 1 gate measurements used `tools/gen/pins.json` SHA-256
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
and C++ is nlohmann/json's single-header distribution (the input of the 2026-11-14 large-grammar size/speed spike, retained unchanged). These files represent their
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

## Wave-2 batch 2 large inputs (2026-11-15)

The following unmodified real inputs and their licenses are SHA-256-pinned in `tools/bench/ci/large.json`.
The YAML and Ruby inputs reuse the previously recorded bytes; no input is concatenated or enlarged.

| Grammar | Source | Bytes | SHA-256 | License |
|---|---|---:|---|---|
| yaml | [compute_gapic.yaml](https://raw.githubusercontent.com/googleapis/googleapis/97f6460c3558c1435d7fd001a9251f8df058ae2f/google/cloud/compute/v1beta/compute_gapic.yaml) | 109,374 | `c7d306beaad2221df3676aa6d1933dbb6c8f05f595592aa676929d92b34fdffc` | [Apache-2.0](https://raw.githubusercontent.com/googleapis/googleapis/97f6460c3558c1435d7fd001a9251f8df058ae2f/LICENSE) |
| php | [ContainerBuilder.php](https://raw.githubusercontent.com/symfony/symfony/979969dc83ebb43a7b6cd6447c0945b764af82b2/src/Symfony/Component/DependencyInjection/ContainerBuilder.php) | 72,565 | `d37e4c0a7410237e501f77e787b9c94314cd68989bd7aafa0a6ae468cd50872a` | [MIT](https://raw.githubusercontent.com/symfony/symfony/979969dc83ebb43a7b6cd6447c0945b764af82b2/LICENSE) |
| powershell | [build.psm1](https://raw.githubusercontent.com/PowerShell/PowerShell/cb9cf48b76cf2fdafb2f2a5e4c64dad4da526486/build.psm1) | 187,716 | `9c87bbaceee0f4724983452b5d1c1ace09fa898f955c970b620d89ec0886a6bf` | [MIT](https://raw.githubusercontent.com/PowerShell/PowerShell/cb9cf48b76cf2fdafb2f2a5e4c64dad4da526486/LICENSE.txt) |
| r | [arrowExports.R](https://raw.githubusercontent.com/apache/arrow/ab5b42237dfe14d683bc74ecbddf71109a0c6633/r/R/arrowExports.R) | 64,659 | `1bbbfea18cfb9e64f7f8c1362ddfd264e2057858375eae225fdc415b7ba364dd` | [Apache-2.0](https://raw.githubusercontent.com/apache/arrow/ab5b42237dfe14d683bc74ecbddf71109a0c6633/LICENSE.txt) |
| dart | [ast.dart](https://raw.githubusercontent.com/dart-lang/sdk/f3f72dd63911938effc75feacc86f52022216c67/pkg/analyzer/lib/src/dart/ast/ast.dart) | 1,763,805 | `69c8d401594c049800129b533fd7764d329c3e8df5fd837137915d6b27ea6d8a` | [BSD-3-Clause](https://raw.githubusercontent.com/dart-lang/sdk/f3f72dd63911938effc75feacc86f52022216c67/LICENSE) |
| xml | [wayland.xml](https://raw.githubusercontent.com/wayland-mirror/wayland/f2ecaa61a3a82944ae069de5413104ab7ba2109f/protocol/wayland.xml) | 168,986 | `4831fd1cdbca6319037660bec5da0f3c5636fd81573cb61e01e4709570dac294` | [MIT](https://raw.githubusercontent.com/wayland-mirror/wayland/f2ecaa61a3a82944ae069de5413104ab7ba2109f/COPYING) |
| ruby | [gb18030-tbl.rb](https://raw.githubusercontent.com/ruby/ruby/b7e8fad02273da0947e45fee172b26c6f49e694e/enc/trans/gb18030-tbl.rb) | 1,357,449 | `2755eb2a433e528b1f51213c4b5905a139ceff3d1d2ef5349dbb1f15bc77201f` | [BSD-2-Clause](https://raw.githubusercontent.com/ruby/ruby/b7e8fad02273da0947e45fee172b26c6f49e694e/BSDL) |

Apache Arrow's NOTICE and Ruby's LEGAL are retained with the local batch 2 input evidence; their source identities
and SHA-256 values are in the batch 2 input manifest. Gate results are reported independently of this input register.

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

## Kotlin inputs (JetBrains stdlib and tests)

Eleven inputs from the 2026-11-14 Kotlin speed analysis and scanner-guard verification, reused without byte changes from `JetBrains/kotlin` commit
`e12dc36e784c4c65886009eb622504b3db65b699`. Sources and Apache-2.0 license hashes are pinned in
[`tools/bench/ci/large.json`](../../tools/bench/ci/large.json); each input is checked before measurement.
The grammar source is the unmodified tier-3 `fwcd/tree-sitter-kotlin` pin (ADR-0024).

| File | Bytes | SHA-256 |
|---|---:|---|
| _Arrays.kt | 985805 | `85d0cf599441b69ed6f836ba588e0f54617a807f63f0e6b57195124ee6bc0849` |
| _UArrays.kt | 413285 | `cb2dc50d5296585e863da1a8efbce997c0db3f14e7e0d434809b0f12675da741` |
| _Collections.kt | 143165 | `ceef966fca1973a351e85c6bedc4a14d43386b26b25682fb6efaa2c9e79cc842` |
| _Sequences.kt | 120825 | `66906380cffa34e53b7c65d780879c5d49930d46ed3c8788f59656a93102a0f0` |
| _Strings.kt | 93867 | `5b6fe03540506b6191bd097c0166ba85b83da38bb0689f213eeada3621259a44` |
| _Maps.kt | 25235 | `ff9e7a624131f00164563bd126404d61a8487998441e42aaf41262d71b9b7e0e` |
| Collections.kt | 22139 | `6534b218d083d0a59b12d4f1c2276a92ee23cf3c1ec8d9a1e9157ae744dcf06d` |
| Strings.kt | 70480 | `12be6c955d7ab31a705abe296b4733f3883468c8566ca823537a2642932e6b82` |
| StringTest.kt | 80759 | `dfae8bb84a5eb8aa80d12e76395f2d80a5ea1b943aae2313728031d482fa8d4d` |
| CollectionTest.kt | 49180 | `6ef51d3628a53e461d75c29c2d99786e697c209c3b37aa7a2afbe47c2649cfa9` |
| ArraysTest.kt | 128682 | `bfc65c890312121479e786d5dd05d25f85bbc42e8ac5b10ca95947caa2e8bb03` |

The license is `JetBrains/kotlin` `license/LICENSE.txt`, SHA-256
`cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`.

## TSX large input (2026-11-16)

`tools/bench/ci/large.json` pins Excalidraw's unmodified `App.tsx` at v0.18.0 commit
`817d8c553c3389650f8b4503984a6d4a5d2f0c11`. It is one real application source file,
not a concatenated or enlarged fixture; results describe this input's shape.

| Grammar | Source | Bytes | SHA-256 | License | License SHA-256 |
|---|---|---:|---|---|---|
| tsx | [App.tsx](https://raw.githubusercontent.com/excalidraw/excalidraw/817d8c553c3389650f8b4503984a6d4a5d2f0c11/packages/excalidraw/components/App.tsx) | 353,813 | `7b3e614cf314d29d0285b69079cc3e67986b75c56f57088c17f9bc01b3b96dbe` | [MIT](https://raw.githubusercontent.com/excalidraw/excalidraw/817d8c553c3389650f8b4503984a6d4a5d2f0c11/LICENSE) | `1352d204fdb90d5e482c13139d848bc2c9300a9a7de358a0a558a23e72d0d8be` |

The fetch lane checks both hashes and writes `App.tsx.LICENSE` beside the input.

## Bounded grammar-kit inputs

`tools/conformance/testdata/kit/manifest.json` records 165 source cases from
[tree-sitter-grammar-kit at `0a12156`](https://github.com/wotjr1649/tree-sitter-grammar-kit/tree/0a12156).
The archive SHA-256 is
`83d792c6453a5f316e101a76885f637b4397ba7a601cfebbd7a388f23cab07d3`.
The manifest embeds the full MIT notice, also retained in `testdata/kit/LICENSE`.
Only `source_utf8` and `edits` are projected; kit expectations, queries, native
outputs and product code are not imported. Source JSON file hashes and case IDs
identify every selected case.

Selection is independent of parser results. For each of csharp, go, json,
python, tsql, tsx and typescript, take at most 24 cases by round-robin over
`routes/edited`, `routes/unedited`, `gaps/edited`, `gaps/unedited`, omitting
missing/exhausted buckets. Each bucket sorts by `(SHA256(UTF-8 case ID), ID)`;
duplicate `(source SHA-256, edits)` cases do not consume quota. Exclude every
case with a `sample` field. Only routes/gaps are read: `n461` is the separate
.NET Framework 4.6.1/C# 7.3 WinForms/DevExpress/WCF business workload, and is
excluded in full. JSON exhausts its unique cases at 21.

| Grammar | Source cases | Fresh inputs | Pair edits |
|---|---:|---:|---:|
| csharp | 24 | 40 | 26 |
| go | 24 | 41 | 28 |
| json | 21 | 33 | 21 |
| python | 24 | 43 | 35 |
| tsql | 24 | 48 | 32 |
| tsx | 24 | 35 | 15 |
| typescript | 24 | 39 | 20 |
| Total | 165 | 279 | 177 |

Projection preserves UTF-8 bytes, including CRLF, without normalization. Each
edit replaces the first `find` in the preceding step; empty `find` appends,
and absent targets fail. Offsets for `EditFor(old,new,start,oldEnd,newEnd)` are
`start=byte index`, `oldEnd=start+len(find)`, `newEnd=start+len(replace)`;
`EditFor` derives byte-column points. Steps are generated sequentially, but each
edit test freshly parses its immediate old source and applies exactly one edit.
This tests pairs, not tree reuse through the entire edit history.

Fresh inputs are deduplicated by `(grammar, SHA-256)`; identical bytes across
grammars still run for each grammar. Edit pairs are deduplicated separately by
`(grammar, old SHA-256, new SHA-256, start, oldEnd, newEnd)`, preserving distinct
edits of a shared source. The content-addressed store has 275 unique blobs,
79,260 bytes, with a fixed 256 KiB source budget. `.gitattributes` disables
newline conversion for them. Every initial and projected output hash is in the
manifest; its own SHA-256 identifies the complete selection and projection.

Reproduce into a new directory using Python's standard library only:

```text
python tools/native-ref/ciconf/select_kit.py KIT_ARCHIVE NEW_DIRECTORY
```

In review, regenerate into a new directory and `diff -r` it against
`tools/conformance/testdata/kit`: the loader checks hashes, chains and budget but
cannot prove the selection rules were followed, and it rejects files under
`sources/` that the manifest does not reference.

The generator verifies the archive hash before reading it. CI needs no kit
checkout or Python: `ciconf -kit tools/conformance/testdata/kit` validates the
committed manifest, MIT notice, every source hash, edit chain and budget, then
adds rows to the existing conformance corpus/edit manifests. No seeded edits
are added to kit inputs. The five existing corpus, edits, native-query, fresh
field and edited-field tests compare them with freshly built native references.
Kit native incremental/fresh differences are counted separately in
`kitNativeIncrementalDiffersFromFresh`; every engine/native difference still
fails. `ci.json`/`inputs.json` record `kitManifestSHA256`; nightly summaries, the
job summary table and repro JSON include that identity plus `inputSetSHA256` (the
complete generated corpus manifest hash, including seeds from the pinned grammar corpus). `ciconf`
rejects an empty `-kit` value, and in the seven kit grammars' shards
`TestCorpusKitInputs` (also the nightly test) fails when the manifest identity,
kit fresh inputs or (for full CI data) kit pair edits are missing.

Before widening, measure the current subset per affected shard locally and on
a hosted PR run, retaining before/after native-data preparation and all five
comparison timings. Existing limits remain 20 minutes per comparison step and
60 minutes per job. A next proposal is at most 48 source cases per grammar
(336 cases maximum), at most 600 fresh inputs, 400 pairs and the same 256 KiB
source budget. Proceed only after measured full shards use less than half each
existing step/job timeout; do not infer hosted timing from local measurements.
This change does not widen to that proposal.

## PostgreSQL candidate

`postgres` is SQL only; PL/pgSQL is excluded. The large input is real Pagila INSERT data, not repeated padding.
It does not represent every PostgreSQL statement or application workload. Both files are used in the local PostgreSQL paired lane;
the hosted bench fetch manifest includes the Pagila input and the upstream corpus supplies its small-file lane.

| Input | Bytes | SHA-256 | License |
|---|---:|---|---|
| [Pagila insert data](https://github.com/devrimgunduz/pagila/blob/6ea53f812254c91a602f61c0ef62beca3fa676b1/pagila-insert-data.sql) | 19,526,610 | `14c39f6e24822177d4cf8b09390fd1bec35c95ded626d7a71c083f429d73182a` | [PostgreSQL](https://github.com/devrimgunduz/pagila/blob/6ea53f812254c91a602f61c0ef62beca3fa676b1/LICENSE.txt), SHA-256 `dcc10ac50b94172e627f7a5ec9c08f9e4568e6dd51a952f96ec09506f70163fb` |
| [Open-rental report SQL](../../tools/bench/testdata/postgres/rentals.sql) | 351 | `60832cb4371298e2c5cb3026e5bb70e80747da40e56bd6507087a1e2420c937e` | Repository MIT; original PostgreSQL example, LF, UTF-8 |

The grammar corpus and queries are extracted from the SHA-pinned upstream archive by `native-ref`; seeded edits use seed 1.

## Swift candidate

`swift` is the Swift grammar of `alex-pinkus/tree-sitter-swift` (first released in v0.6.0). The large inputs are
real project files, not padding: one machine-generated file and two hand-written ones. They do not represent all
Swift syntax, and no Swift language-version coverage is claimed. The hosted bench fetch manifest includes them; the
upstream corpus supplies the small-file lane and one original small file adds a named small workload.

| Input | Bytes | SHA-256 | License |
|---|---:|---|---|
| [swift-syntax `SyntaxNodes.swift`](https://github.com/swiftlang/swift-syntax/blob/0688b9cfc4c3dd234e4f55f1f056b2affc849873/Sources/SwiftSyntax/gyb_generated/syntax_nodes/SyntaxNodes.swift) (tag `0.50200.0`, generated code) | 425,161 | `337b909b59ed6b7ec3c295e30b5dfd8a2954b164e56660fd62c46906c4ec6016` | [Apache-2.0 with Runtime Library Exception](https://github.com/swiftlang/swift-syntax/blob/0688b9cfc4c3dd234e4f55f1f056b2affc849873/LICENSE.txt), SHA-256 `770af8291f708538d8ff885a0bbc4e045cd700531741c4f99528d435c14d7f55` |
| [Alamofire `Session.swift`](https://github.com/Alamofire/Alamofire/blob/513364f870f6bfc468f9d2ff0a95caccc10044c5/Source/Core/Session.swift) (tag `5.10.2`) | 76,405 | `88f520308efecbc71b168f3f2cd45d14b0326a82451460d9cf7a86adbb698c7b` | [MIT](https://github.com/Alamofire/Alamofire/blob/513364f870f6bfc468f9d2ff0a95caccc10044c5/LICENSE), SHA-256 `3ef1fbd5fd9cb08a9e071d6c0c6f68322deda2b93a814e5135a7829495dae698` |
| [Alamofire `Request.swift`](https://github.com/Alamofire/Alamofire/blob/513364f870f6bfc468f9d2ff0a95caccc10044c5/Source/Core/Request.swift) (tag `5.10.2`) | 44,513 | `bc44fbbe62aa29c96b2d9821cb67f8cb48215f9875769ee539b03f081d0d556c` | same Alamofire license |
| [Open-rental store](../../tools/bench/testdata/swift/rentals.swift) | 570 | `1a0a9a11f18cfaeab99eaeaceb05feb101054119dd2ebaa8d31fca977b96a5a6` | Repository MIT; original Swift example, LF, UTF-8 |

The grammar corpus and queries are extracted from the SHA-pinned upstream archive by `native-ref`; seeded edits use seed 1.


## Grammar-input patch regression corpus

`tools/gen/patches/postgres/corpus/korean-identifiers.txt` is an original repository MIT fixture, UTF-8 with LF
line endings. The generator gate adds its `SELECT 한글;` input to both native variants and derives three seed-1
edits. It exercises ADR-0023's non-ASCII unquoted identifier fix without changing the upstream corpus or its tests.
Corpus-format file: 520 bytes, SHA-256 `5ddf7ad1aafcf144351bab7ffef3352897b01cc51ef5e4378d148d57048ad0f3`.

## T-SQL v2.1.0 Korean regression inputs (development tree)

The Korean [manifest](../../tools/conformance/testdata/korean/manifest.csv) retains 11 T-SQL inputs,
all requiring clean trees and explicit node-span expectations. Five former known-gap rows become clean;
`tsql/id-non-ascii-regression.in` preserves the exact 67-byte former register input (SHA-256
`c472521c07e5373fcf69a734e9a3acc72b6e1efa4a9552b2bb56f399eb384aa3`, final LF).
These original repository MIT inputs exercise aliases, variables, columns and a statement label;
`TestCorpusKorean` checks the product and, when configured, a fresh native reference.
The fixed T-SQL entry and obsolete strict-failure dump/document are retired rather than accepting a changed wrong tree.
