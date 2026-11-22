# tools/gen

Generates one grammar module (`grammars/<lang>`) from pinned upstream C: clang (wasi-sdk) → wasm → wasm-opt
(recipe-dependent) → `wasm2go` → patched `memory_grow` → Go module. Design: `docs/design/engine-abi.md`, ADR-0020, ADR-0022.

## CP949 decode table

`go run . cp949` (from `tools/gen`) regenerates `../../cp949_table.go` from `testdata/index-euc-kr.txt`, the WHATWG
Encoding Standard `EUC-KR` index (windows-949) pinned by commit URL, sha256 and `Identifier:` header in `cp949.go`.
It needs no network and no `pins.json` entry; `TestCP949TableIsCurrent` compares the committed bytes with a fresh
run. To move the pin, replace the testdata file, update the constants in `cp949.go`, `LICENSES/whatwg-encoding.txt`
and its manifest entry, and regenerate.

## Offline node-type schema commands

From `tools/gen`, use only local `node-types.json` files:

```sh
go run . schema check /path/to/node-types.json
go run . schema diff /path/to/old/node-types.json /path/to/new/node-types.json
```

These commands run before generation flag parsing and do not load pins, fetch inputs, or need a C compiler.
JSON output binds the input filenames, byte counts and SHA-256 hashes. `staticOnly: true` means a static
contract check: it does not establish parse correctness, C agreement, SemVer safety, or release readiness.
Supertypes are compared as declared, without expansion. Entries, alternatives and object-key order do not
change the diff. Output order is node `(type, named)` (false first), then code order below, field and member.

`check` returns `PASS`, `FAIL` for malformed input, or `BLOCKED` for unsupported keys alone. Exact keys are
required at every level; duplicate decoded keys (including escape-equivalent keys), invalid UTF-8 or
unpaired Unicode surrogate escapes, null/missing/wrongly typed required values, duplicate `(type, named)`
identities or alternatives, unresolved subtype references and subtype cycles fail. Empty alias types are
valid. Undeclared field/children alternatives are warnings and can accompany `PASS`. JSON syntax errors
stop at the first error; structural findings are collected. The CLI limits inputs to 16 MiB. JSON nesting
allows at most 64 containers (the outermost array is level 1); decoded object keys are limited to 1024 UTF-8
bytes (`JSON_KEY_LIMIT`). Top-level nodes are decoded and checked one at a time. At most 1000 findings
are retained, followed by one error `FINDINGS_TRUNCATED` with the total number seen. All findings still
contribute to `FAIL`/`BLOCKED`; truncating warnings alone yields `FAIL` rather than an incomplete `PASS`.

`diff` checks both inputs first and emits no differences for invalid/unsupported inputs. A valid equal
pair has assessment `PASS`; a changed pair has assessment `FAIL` (a contract difference, not invalid input).
Both valid outcomes exit **0**, so differences can be reviewed. Invalid/unsupported input, usage or I/O
errors exit **1**. A check with warnings still exits 0. No baseline is inferred from Git or from missing files.

The 20 change codes retain the kit's six review risks, rather than a safety verdict:

| Codes (comparison order within a node) | Risk |
|---|---|
| `NODE_REMOVED`, `NODE_ADDED` | `REMOVAL`, `ADDITION` respectively |
| `NODE_NAMED_CHANGED`, `ROOT_CHANGED` | `IDENTITY` |
| `EXTRA_CHANGED` | `CLASSIFICATION` |
| `FIELDS_PRESENCE_CHANGED` | `IDENTITY` |
| `FIELD_REMOVED`, `FIELD_ADDED` | `REMOVAL`, `ADDITION` respectively |
| `FIELD_REQUIRED_CHANGED`, `FIELD_MULTIPLE_CHANGED` | required false→true or multiple true→false: `CARDINALITY_NARROWED`; inverse: `CARDINALITY_WIDENED` |
| `FIELD_TYPE_REMOVED`, `FIELD_TYPE_ADDED` | `REMOVAL`, `ADDITION` respectively |
| `CHILDREN_PRESENCE_CHANGED` | `ADDITION` when present after, otherwise `REMOVAL` |
| `CHILDREN_REQUIRED_CHANGED`, `CHILDREN_MULTIPLE_CHANGED` | same cardinality rule as fields |
| `CHILDREN_TYPE_REMOVED`, `CHILDREN_TYPE_ADDED` | `REMOVAL`, `ADDITION` respectively |
| `SUBTYPES_PRESENCE_CHANGED` | `ADDITION` when present after, otherwise `REMOVAL` |
| `SUBTYPE_REMOVED`, `SUBTYPE_ADDED` | `REMOVAL`, `ADDITION` respectively |

`NODE_NAMED_CHANGED` applies only to one removed and one added identity of the same spelling. Absence,
explicit false and empty fields are distinct. Added/removed whole nodes or fields emit the whole change;
children/subtype presence changes also list their added/removed alternatives. Before/after values use
canonical key and alternative order. Findings carry JSON Pointers; diff changes identify node, field and member.
Definitions and reused test data are attributed in [schema/NOTICE.md](schema/NOTICE.md); implementation uses
the Go 1.26 standard library only.

To exercise every pin against SHA-256-checked archives without downloading or modifying them:

```sh
TSG_SCHEMA_ARCHIVES=/absolute/regen-work/dl CGO_ENABLED=0 GOWORK=off go test -count=1 -v -run '^TestPinnedSchemas$' .
# Optional explicit pin file for reviewing a pin change:
TSG_SCHEMA_ARCHIVES=/absolute/regen-work/dl TSG_SCHEMA_PINS=/absolute/review/tools/gen/pins.json CGO_ENABLED=0 GOWORK=off go test -count=1 -v -run '^TestPinnedSchemas$' .
```

Without `TSG_SCHEMA_ARCHIVES`, this archive integration test is explicitly skipped; ordinary unit tests stay
offline. Archive hash mismatches and strict schema failures fail the test, with no allowlist. Schema provenance
is recorded in [the schema register](../../docs/validation/grammar-schemas.md#offline-strict-check-inputs).

## Pin-move CI evidence

`.github/workflows/pin-move.yml` fetches and verifies the PR `base.sha`, then compares the **merge base** of base and
head with the head commit (`--merge-base`), so a branch that is behind the base is never judged against pins it does
not contain. Full history is checked out; a missing merge base fails the step. The cache, Go setup and native work
are skipped when no grammar entry differs (`--detect`). It runs again when the PR body changes. Dispatch requires explicit `base` and `head`
refs; a push or dispatch without refs reports **diff skipped**, never an inferred baseline. Local replay uses
the same driver from the repository root (the named commits must already be available locally):

```sh
python3 tools/gen/pin-move.py --base OLD_COMMIT --head NEW_COMMIT \
  --out /new/empty/pin-move-out --cache /reusable/pin-move-cache \
  --pr-body-file /path/to/pr-body.txt
python3 -m unittest discover -s tools/gen -p pin_move_test.py
```

Only changed `grammars` entries in `tools/gen/pins.json` run. `ADDED` entries receive a strict new-schema check;
`REMOVED` entries are listed. Changed entries receive strict old and new schema checks and a diff with its
codes/risks, then fresh `tools/native-ref` builds for both pins using the current wasi-libc ctype shims,
`-fsigned-char` and `-DNDEBUG`. Downloads are cached by SHA-256 and checked before archive/schema use.
The current native driver is used on both sides; this compares grammar sources, not historical host binaries.

Each native executable dumps every extracted old corpus input, every new corpus input and each selected grammar's
SHA-256-pinned large benchmark input from `tools/bench/ci/large.json`. The driver retains input hashes, both
dumps and per-input classifications: identical, changed, ok→error and error→ok. Existing error trees are retained
and reported; they are not successful clean parses. Corpus extraction uses `ciconf -inputs-only`; upstream `:skip`
and other-language exclusions are listed with reasons in the report and counted in the summary. A changed grammar
without a large manifest entry fails. A grammar whose large input is a corpus concatenation (T-SQL)
also needs that input's `sha256` in `tools/bench/ci/large.json` updated by the same PR whenever the move changes the
corpus, otherwise the hash check fails and cannot be overridden. The concatenation skips the corpus's `errors-*` inputs
(invalid on purpose; T-SQL v2.1.0's unterminated comment would otherwise turn 88% of the file into one ERROR node). A missing baseline/input, extraction/schema failure, process
failure, timeout or malformed dump fails the job. Schema `REMOVAL`/narrowing risks and changed trees alone do not.
Native runs have a 30-second per-input limit, build commands a 15-minute limit, and the CI job a 60-minute limit.

An ok→error regression fails unless the PR body contains a separate exact override line naming that input:

```text
PIN-MOVE-ALLOW-OK-TO-ERROR c/new/declarations-001.in
```

Corpus IDs are `<grammar>/<old|new>/<name>.in`; large IDs are `<grammar>/large/<file>`. Copy the exact ID from
the failed report; there is no wildcard or blanket override. The report shows the override beside the regression
so it remains visible in review. An override applies only to that ok→error result, not missing/invalid evidence,
schema failures or a native process failure. A reviewer still decides whether the regression is acceptable;
the override does not grant pin-change or release authority.

`summary.md` is appended to the job summary even after a comparison failure. The artifact retains `report.json`,
per-grammar schemas/results, inputs, dumps and logs; extracted sources, executable binaries and download caches
are excluded. This is native-before/native-after evidence, not Go/native C agreement or release readiness.

### Later T-SQL releases

A later release from `wotjr1649/tree-sitter-mssql` (for example, v2.2.0) can use one T-SQL-only pin-move PR:
resolve its full commit, download and hash its archive, update only the `tsql` pin with the new archive/tree/file/
licence/notice hashes (`go run . -hash -lang tsql -src <sources> -dl <archives>` from `tools/gen`), then run the
schema checks and `bash tools/gen/regen.sh <new-empty-work-dir> tsql`. Keep the CLI/runtime pins unchanged.
If the upstream corpus changes, compute the T-SQL `large.json` hash with `benchci fetch -print -langs tsql`
over `ciconf -inputs-only` data from that corpus, without the optional kit inputs. Run the generator gate,
fresh native/product conformance (including the repository's queries and Korean fixtures), and the commit-bound
pin-move replay above. The generator gate compares generated C with committed C; it does not replace product
conformance for T-SQL.

The PR updates `grammars/tsql`, the schema register, migration notes for node-kind changes, and any affected
Korean expectations or known-gap identities/entries; regenerate known-gap documents rather than editing them.
Do not refresh a changed gap signature without investigating it. Document the new archive's mirror status here;
repository/tier documents need another amendment only if that assignment changes. An ok-to-error result still
requires the owner's input-specific decision. Grammar-module versioning and publication remain separate decisions
under [publication.md](../../docs/specs/publication.md).

## Nightly CLI rehearsal (report only)

`.github/workflows/nightly-cli-rehearsal.yml` (weekly, Wednesday 04:23 UTC, and manual dispatch; Linux amd64 only) runs
`tools/gen/nightly-rehearsal.sh`, which regenerates every grammar with the pinned CLI and with the CLI of the `nightly`
release of `tree-sitter/tree-sitter` and compares the generated files. It is **not a gate**: nothing is written under
`grammars/`, no pin changes, no PR is opened or blocked ([ADR-0027](../../docs/design/decisions/ADR-0027-one-pinned-generator-cli.md#nightly-rehearsal-2026-11-20-report-only),
[validation.md](../../docs/specs/validation.md)). Locally (needs `gh`, `curl`, `gzip`; Linux amd64 or Windows amd64 with Git Bash;
a heavy job under the machine's one-slot rule):

```sh
bash tools/gen/nightly-rehearsal.sh all <new-empty-work-dir> [lang ...]     # all 25 grammars by default; Linux needs bwrap
NIGHTLY_REHEARSAL_UNSANDBOXED=1 bash tools/gen/nightly-rehearsal.sh all <dir> json   # Windows: explicit opt-in, no isolation
```

The workflow runs the phases as separate steps: `fetch` (pinned archives and CLI, fetched and SHA-256-verified exactly as
`regen.sh` does), `nightly` (the only step with a token: release metadata and the asset download; nothing is executed)
and `run`. The nightly CLI has no pin; its identity is recorded in `<work-dir>/nightly-meta.json` (asset URL, API size and
`digest`, commit of the `nightly` tag before and after the download) and in `out/report.json` (SHA-256 of the asset and
of the binary that ran, the `--version` line and its build commit), and cross-checked: API size and digest against the
file before decompressing it, the tag unchanged, the build commit equal to the tag commit. Any mismatch, and any failure
to read the asset or the metadata, to parse the metadata or to decompress the asset, makes `nightly.identity` `held`: the
problems and whatever was observed are recorded, no grammar is compared, there is no `digest.txt` and the command fails.
Without `-nightly-meta` the identity is `unchecked`.

`gen nightly-rehearsal` (the Go side, `rehearse.go`) first runs every grammar's common input check,
`grammarpatch.CheckInputs`: Prepare's own pre-generation steps (archive hash and extraction, input patches, npm inputs on
the patched tree, fetched and verified, and the generation input: the explicit `grammar.json`, or by default
`grammar.js` in the directory `generate` runs in). A failure there is `input-failed` and neither CLI runs. Then the same
`grammarpatch.Prepare` runs for both sides independently; C patches on generated files stay with each side. The nightly
side sets `Config.Unpinned`, which exists for this command only and refuses pin fields; it generates with `--output`
into a fresh directory beside each tree, and Prepare imports only the files that mode generates (regular `parser.c`,
`node-types.json`, `tree_sitter/{alloc,array,parser}.h`, and `grammar.json` only when generating from `grammar.js`; with
`generateFrom: grammar.json` the input stays and an output `grammar.json` fails the run). An expected file the CLI did
not write is removed, not left as the archive's copy, so a missing `node-types.json` reports as missing; a missing
`parser.c` fails the generation. Then the C patches and hashes follow. The nightly `--version` and `generate` processes
get the caller's environment without `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN` and
the mount variables; only `generate` gets the two trusted mount paths. On
Linux every nightly execution goes through `nightly-sandbox.sh` (bubblewrap with unprivileged user namespaces: no network,
empty environment, read-only `/usr`, that call's tree, including npm inputs and Swift's `grammar.json`, read-only, its
output directory the only writable path, nothing else of the host mounted); the report records `"sandbox": "bwrap"`.
Without the wrapper `gen` stops before running anything downloaded, unless `-nightly-unsandboxed` is given; the script
passes it only with `NIGHTLY_REHEARSAL_UNSANDBOXED=1` (Windows has no sandbox), and the report then records
`"sandbox": "none"`. The `run` phase also starts `gen` itself without those token variables, also in `all`.

Output in `<work-dir>/out/`: `summary.md` (also the job summary and the issue comment), `report.json`, `digest.txt`, and gzipped copies
of the differing `parser.c`/`grammar.json`/`node-types.json` under `files/<lang>/<side>/`, for each side that has the file. Per
grammar the result is `identical`, `headers-only` (only other generated files such as `tree_sitter/*.h` differ), `differs`
(one of the three main files differs), `pinned-failed`, `nightly-failed`, `both-failed`
(each side's error and time are reported; files are compared only when both succeed) or `input-failed`. When both sides
generated, the `node-types.json` schema verdict is always included, also when no file differs: `identical` only for equal
hashes, `BLOCKED` with a reason when one or both sides lack it or it cannot be read. Unreadable files and failed copies
are `artifactErrors` and fail the command. A grammar with a generator exception (csharp) is regenerated
as well. The conformance corpus is not run: that needs the native build lane and belongs to the pin-move pull request.
The digest covers the results (status, each differing file's change kind and both SHA-256s, the schema verdict and its
per-code and per-risk counts) and not the CLI identity, error text or timings, so the weekly job comments on issue #67
only when a result changed since the last comment.

## Pins (`pins.json`)

Everything a generation consumes is pinned, and the identity of a generated module never depends on the host:

| Input | Pinned as |
|---|---|
| tree-sitter runtime, grammars | canonical `https://github.com/...` repository and full commit, archive sha256 (GitHub API tarball by default; grammar `archiveURL` preserves a pinned codeload URL), tree and file sha256 |
| wasi-sdk, binaryen | version plus the upstream release archive sha256 **per host platform** (`<GOOS>-<GOARCH>`; windows-amd64 and linux-amd64) |
| wasi-libc license texts | `wasiLibc`: the commit named by the sdk's `VERSION` file (the sdk ships no license text), tarball sha256, and each copied file's sha256; copied to `LICENSES/wasi-libc/` in every module and listed in its `provenance.json` outputs |
| wasm2go | module version and its `go.sum` `h1:` hash; built per host with `go install -trimpath` |
| host binaries | `localBinarySHA256`, checked only with `-verify-binaries`; never part of an identity |

`provenance.json` and the compiled `engine.Identity` record tool versions (`wasi-sdk`, `clang`, `binaryen`, `wasm2go`,
`wasm2go-sum`, `recipe`), not host binary hashes. Another host must reproduce the same `engine_gen.go` / `engine_gen.dat`.

## Regenerate

```
bash tools/gen/regen.sh <new-empty-work-dir> [lang ...]      # all 25 modules by default
ROOT_VERSION=v0.1.0 bash tools/gen/regen.sh <work-dir>       # switch a development tree to the release go.mod form
```

`regen.sh` builds `gen`, prints the fetch plan (`gen -plan host`), downloads every archive into the new directory,
verifies its sha256 **before** extracting, builds wasm2go from the pinned version, then runs `gen` per module into
`grammars/<lang>`. `.github/workflows/regen.yml` runs it on Linux and requires `git diff --exit-code -- grammars/`.
Without `ROOT_VERSION`, `regen.sh` derives the go.mod form from the committed `grammars/*/go.mod` (all 25 must be in the same form: development with `replace`, or release `require vX.Y.Z` without `replace`; otherwise it fails), so the drift check regenerates the committed form unchanged.

The modules are `bash`, `c`, `cpp`, `csharp`, `css`, `dart`, `golang`, `html`, `java`, `javascript` (including JSX), `json`,
`kotlin`, `php`, `postgres`, `powershell`, `python`, `r`, `ruby`, `rust`, `swift`, `tsql`, `tsx`, `typescript`, `xml`, and `yaml`. Each has a grammar-specific generated smoke-test sample.
Downloads allow three attempts with 1s/2s backoff only for transient transport failures. HTTP, certificate and local
write errors end that URL attempt; transient HTTP 408/429/5xx responses also receive bounded retries. A failed upstream download or hash check then tries each configured mirror; sha256 verification remains mandatory before extraction. The retry self-check is
`bash tools/gen/fetch_test.sh`.

Compare raw format-2 field-exception records with `python tools/gen/check-field-export.py --before BEFORE_TREE --after AFTER_TREE --jobs JOBS_JSON --out NEW_DIR`; exact arguments of the 2026-11-15 field-export-cost run (`q13/run.py`, `differential-final`), input hashes (`differential-jobs.json`) and results (`differential-final/receipt.json`) are kept in the maintainer-local evidence archive. The source and result identities are recorded in [engine-abi.md](../../docs/design/engine-abi.md#field-lookup-evidence).

## go.mod forms

- default (development): `require github.com/wotjr1649/go-treesitter v0.0.0` and `replace … => ../..`;
- `-root-version vX.Y.Z` (release, ADR-0022): `require github.com/wotjr1649/go-treesitter vX.Y.Z`, no `replace`, and a `go.sum` with the root module's two `h1:` lines taken from
  `go mod download -json` through the default GOPROXY/GOSUMDB (generation fails if they are unavailable). The development
  form writes no `go.sum` and removes a stale one. `go.mod` and `go.sum` are not listed in `provenance.json`.

The release form cannot be built before the root tag exists; the generator's meta runner always uses its own local
`replace`, so generating in release mode works before then.

## Test-only balancing cancellation

`-balance-probe` adds reset observations and a no-reset negative control by including the unmodified pinned runtime
and product shim in `probe/balance.c`. It uses the normal recipe and memory provider patch. It requires a new empty
output outside `grammars/`, rejects `-root-version`, and marks its provenance `testOnly: "balancing-cancel"`; its wrapper
hash and extra exports are bound into its identity. These modules are test artifacts, never product grammar modules.

`bash tools/gen/balance-probe.sh <new-empty-work-dir> [lang ...]` fetches SHA-verified pinned inputs and builds all 25 probes
under `<work-dir>/probes/` and native references under `<work-dir>/native/`, without regenerating product modules.
With language arguments it builds only those probes and native executables, for example `... <work-dir> javascript`.
Then, from `tools/conformance`:

```sh
CGO_ENABLED=0 GOWORK=off TSG_BALANCE_PROBE_DIR=<absolute-work-dir>/probes \
  TSG_NATIVE_EXE_DIR=<absolute-work-dir>/native TSG_CONF_OUT=<absolute-work-dir>/out \
  go test -count=1 -timeout 90m -v -run '^TestBalancingCancel$' .
```

The nightly/manual Linux job `.github/workflows/balancing-cancel.yml` runs this lane. It is not a PR-required check.

## Recorded grammar patches (ADR-0023)

Each grammar's optional `patches` list keeps the existing single-file records: `file` (under `patches/<grammar>/`),
`source` (archive-root-relative), `sha256` (patch bytes), `beforeSHA256`, `afterSHA256`, and `upstreamReport` (HTTPS).
Only owner-approved patches may be added. C-only patches keep their existing identities and disclosure format.
No real grammar patch is introduced by the regeneration mechanism itself.
Recorded C-only patches exist for bash and ruby (`0001-heredoc-utf8`), yaml (`0001-no-early-eof`,
`0002-bound-serialized-indent-bytes`, `0003-widen-scanner-rows`), python (`0001-bound-serialized-indent-bytes`), rust
(`0001-block-comment-code-points`). HTML applies
`0001-greater-than-text` and `0002-bare-ampersand-text` before generation. PostgreSQL applies
`0001-non-ascii-identifiers` before generation and `0002-dollar-tag-code-points` after it.
Go applies `0001-new-value-argument` before generation to accept Go 1.26 expression arguments to `new`.
Ruby applies `0002-global-variable-non-ascii` before generation to accept non-ASCII global-variable names.
`gen -patch-status` lists 13 records in total, including two for Ruby. Retire each with an approved pin move to a source that contains the fix (for an owner-maintained repository, an owner release; ADR-0024 rule 7.6).
JavaScript retired `0001-jsx-bare-ampersand` with its owner v4.4.0 pin (`e5ada307a6f1741a573d5c8d4dafee9234330b1f`).
The fix first shipped in [owner v2.0.0](https://github.com/wotjr1649/tree-sitter-javascript/releases/tag/v2.0.0)
([fix commit](https://github.com/wotjr1649/tree-sitter-javascript/commit/4765cd79603fa478da221da50500162757859e20)).
`native-ref -unpatched` supplies the required corpus, fuzz-seed and patch-regression baseline.

ADR-0027 sets `regenerate: true` for every grammar except entries with a recorded `generatorException`
(reason and exact failing input IDs). An exception retains the committed reference C. The original archive,
tree and file pins always identify upstream bytes; generation adds identities instead of replacing them.
`gen` and `native-ref` share one implementation: verify the full archive, extract a private copy, apply grammar-input
patches, then run the pinned CLI twice on independent trees using `generate --abi 15 --js-runtime native`.
C/header patches apply after generation. Every resulting file must match. When generated C differs from committed C,
a patch records `generatedBeforeSHA256`/`generatedAfterSHA256` alongside the original reference hashes; the same
hash-checked patch bytes apply to each variant (YAML `0001-no-early-eof` uses this).

Selected `npmDependencies` are unpacked from the upstream `package-lock.json` tarball after SHA-512 integrity
verification; npm and install scripts never run. QuickJS resolves these packages from the grammar working
directory's `node_modules`. TypeScript and TSX load `third_party/tree-sitter-javascript/grammar.js` from their
shared owner archive via `common/define-grammar.js`. The vendored directory is covered by each pin's tree hash.
The pinned CLI uses its native JavaScript runtime for both forms.
npm tarballs are cached in `<dl>/npm` by integrity identity. Direct `gen` calls require explicit `-fetch-npm`
for a missing tarball; `native-ref` requires `-fetch`. `regen.sh` and `balance-probe.sh` opt in with `-fetch-npm`.
Without that flag, generation uses only verified cached tarballs and fails if one is missing. npm inputs are selected
from the verified upstream lockfile after source extraction, outside `-plan`; this explicit opt-in keeps generation
from silently adding network access. Corrupt cached tarballs fail instead of being replaced.

The CLI gzip and decompressed executable are checked against the per-platform pin before execution.
`grammarGeneration` provenance records CLI/runtime, input-tree hash (after grammar-input patches and npm supply),
lockfile/tarball/dependency identities, and generated source hashes. `native-ref` writes the same record under
`work/patches-<lang>/provenance.json`. Both compilers read C from this private result; the extracted source and
archive remain intact. `regen.sh` fetches the CLI; direct `gen` calls supply `-tree-sitter` and its pinned gzip
in `-dl`. `native-ref -fetch` downloads it on demand. Ordered single-file records represent multi-file patches.

`gen -patch-status [-patch-alert N]` lists all grammar patch counts, including zero; counts greater than `N` warn
(default 3). Both builders print the selected grammar's count. `PATCH_ALERT_THRESHOLD=N` configures the shell
entry points; `native-ref` accepts `-patch-alert N`. The synthetic integration test is enabled with
`TREE_SITTER_CLI_ARCHIVE=<pinned-host-gzip> CGO_ENABLED=0 go test ./...` from this module.

## Immutable release source mirror

`pins.json` has an ordered `mirrors` array of release-asset base URLs. The fetch plan's sixth TSV column contains
those bases with the exact archive filename appended (comma-separated; `-` means none). Upstream always runs first;
transport/HTTP failure or a wrong SHA-256 tries the next mirror. Both `regen.sh` and `balance-probe.sh` check the original
archive hash before extraction. `native-ref -fetch` follows the same rule. Existing cached native archives are still
checked and a corrupt cache fails, rather than being silently replaced.

List the pinned archive candidates from all platforms (deduplicated TSV: filename, SHA-256, upstream URL,
kind `source`/`toolchain`; no header):

```sh
(cd tools/gen && CGO_ENABLED=0 go run . -mirror-assets) > release-assets.tsv
awk -F '\t' '$4 == "source"' release-assets.tsv > release-source-assets.tsv
```

The command lists 34 rows for the current pins: 26 `source` archives and eight `toolchain` archives. The immutable v0.4.0 release
contains its original 24 source archives plus `SHA256SUMS`; the new C and PostgreSQL archives have no fallback there.
The v0.5.0 release mirrors the 25 rows with column 4 equal to `source` that existed then, plus `SHA256SUMS`.
The Swift archive, the T-SQL v2.1.0 archive `tree-sitter-mssql-80037156b044.tar.gz`, the Java v3.0.3 archive
`tree-sitter-java-358048c1c6f9.tar.gz`, the shared TypeScript/TSX v1.0.0 archive
`tree-sitter-typescript-59fc4edb0c0b.tar.gz`, and the JavaScript v4.4.0 archive
`tree-sitter-javascript-e5ada307a6f1.tar.gz` were added afterwards and
have no mirror fallback until a release carrying them is published and its base is configured. Check their filenames against `runtime.archive`,
`wasiLibc.archive`, and each grammar's `archive` in `pins.json`, deduplicating TypeScript/TSX's shared archive;
the [publication procedure](../../docs/specs/publication.md#pinned-archive-release-assets) specifies the inventory
and digest checks. The eight SDK/binaryen/CLI host archives are excluded until complete bundled notices can be
established. Those tools are fetched from their official releases with pinned SHA-256 checks and have no assets
in this release to provide mirror fallback, even if the fetch plan lists URLs at the configured bases.

The pins use v0.5.0 and then v0.4.0 as mirror bases. After publishing and checking all selected source rows at a
new release base, update `mirrors` in a follow-up PR. A configured base can supply only the exact files already
published there. A new source archive gains fallback coverage when a
carrying release is published and its base is configured. Published releases are immutable. This command does not
upload anything. `wasm2go` remains a Go module installed through Go's checksum-verified module mechanism
(`wasm2goSum`), not an archive in this list. The Go toolchain and system C compiler are host prerequisites, not
downloads managed by this pin file.

## Reviewed scanner patch expectations

The conformance matrix checks tracked `patches/<grammar>/scanner-expectations.json` files under ADR-0023.
The comparison uses fresh native dumps for the pinned corpus and kit inputs, fixed fuzz inputs, and the
same regression bytes used by the product tests. Native builds share the pinned generator run and C patch
prefixes. The unchanged YAML early-EOF step runs the existing whole-stack comparison.

When adding or changing a scanner patch, review a candidate file and its raw comparison before updating
the tracked expectations. For example, with the pinned archives in an absolute `$dl` directory, set `$work`
to a new absolute directory and run these commands from the repository root.

```sh
lang=yaml
mkdir -p "$work"
(cd tools/native-ref && go build -o "$work/native-ref" . && go build -o "$work/ciconf" ./ciconf && go build -o "$work/patchcompare" ./patchcompare)
"$work/native-ref" -pins tools/gen/pins.json -dl "$dl" -work "$work/build" -out "$work/exe" \
  -corpus "$work/corpus" -scanner-stages "$work/stages" "$lang"
"$work/ciconf" -pins tools/gen/pins.json -corpus "$work/corpus" -out "$work/data" \
  -lang "$lang" -kit tools/conformance/testdata/kit -inputs-only
ulimit -c 0
"$work/patchcompare" -scanner-build "$work/stages/$lang.json" -corpus "$work/data/corpus/$lang" \
  -fuzz-source tools/conformance/fuzz_test.go -regressions tools/conformance/testdata \
  -out "$work/evidence" -write-candidate "$work/candidate.json"
```

Match each changed case to the patch's documented effect. Fill in `reason` and `retireWhen` for each patch
and `review` for every changed clean baseline and recorded baseline crash. A baseline crash row lists its
class under `baselineCrashes` per `GOOS/GOARCH`; add each platform's class from that platform's run. Keep the candidate's exact
signatures and preserve unexplained failures for investigation. Re-run with a fresh `-out` directory and
`-expectations tools/gen/patches/$lang/scanner-expectations.json` in place of `-write-candidate`.
Check the Linux amd64 and arm64 conformance results before accepting the update. CI never writes the
sidecar. Pin changes require re-review and retirement deletes the affected entry.

## PostgreSQL regeneration cost

`postgres` uses pinned `gmr/tree-sitter-postgres` grammar inputs with the temporary Korean-support
patches disclosed in [patches/postgres/README.md](patches/postgres/README.md). Its archived `postgres/src/parser.c` is an
LFS pointer; the original pointer and input tree remain pinned, and both builders compile the twice-generated C.
PL/pgSQL is excluded. The CLI is pinned for Windows x64, Linux x64/arm64 and macOS arm64.

Windows feasibility measurement: one CLI generation took about 180 seconds and 4.52 GiB peak commit. `grammarpatch`
allows 10 minutes per generation and runs the two independent generations sequentially. Regen and balancing
jobs have 60 minutes; conformance, nightly and bench native builds also remain one grammar per job with 60 minutes.
The balancing job generates both a probe and a fresh native reference, so it performs four sequential CLI runs.
No generated-C cache or skip bypasses the drift check. Other grammar shards fetch the PostgreSQL archive but do not generate it.

[GitHub's hosted-runner specifications](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
list 16 GB RAM for public Ubuntu x64/arm64 and 7 GB for macOS arm64 (private Ubuntu: 8 GB). Each CLI run precedes
C compilation and no other language regenerates within the job. macOS has the least headroom; hosted results
remain pending and an OOM/timeout must fail visibly. Windows measurements do not establish hosted memory or timing.

The PostgreSQL regen shard also runs the small patched and unpatched CLI integration tests using the
same verified CLI archive. A missing archive fails the step before Go can skip those tests. Pin-move CI
selects grammars from `pins.json`: adding `postgres` checks its new schema, and later pin changes require
the old/new native comparison. Its unpatched regeneration flag also prepares the pinned CLI for that job.
CLI errors (including OOM), compiler errors and timeouts propagate as job failures; no memory-dependent
skip or `continue-on-error` is used for PostgreSQL generation.

## Swift regeneration

`swift` uses unmodified `alex-pinkus/tree-sitter-swift` grammar inputs; upstream commits no `parser.c`, so
`regenerate: true`, `generateFrom: "grammar.json"` with the pinned CLI 0.27.2 produces `parser.c` and `node-types.json` twice
independently, for both engine and fresh native C builds. There are zero patches. Of the current 25 grammar
modules, Swift alone pins `reference: "none"`: no committed-parser comparison exists, and the gate explicitly
records `NOT_APPLICABLE_NO_COMMITTED_PARSER`. Direct `native-ref -reference swift` fails with that status.
The gate still requires candidate determinism, a strict generated-schema check, product-vs-fresh-native corpus,
seeded edits and field lookups, pinned large-input comparisons, and the upstream repository's own tests.
Swift reads the unmodified upstream-committed `src/grammar.json` (including emoji patterns); it never evaluates
`grammar.js`. Other grammars keep the default JS input. `grammarGeneration.generateFrom` and
`generateFromSHA256` identify the selected input before generation; the full input-tree hash is retained.
The sole owner-approved upstream-test exception is the CLI 0.27.2 outline query-path defect, recorded in
`pins.json` separately from generator exceptions. The original full-suite exit 1 and raw log are retained.
The gate requires all 312 corpus cases and eight highlight cases to pass, matches the exact query error, then
runs `tree-sitter query queries/outline.scm test/outline/outline.swift`; any other failure fails the gate.
A CLI pin move invalidates the exception; retire it after the fixed CLI passes the full suite.
The absent reference is not a pass for the remaining checks. The CLI generation takes about a second
on the Windows measurement host. The scanner's allocation-failure branch calls `fprintf(stderr, ...)` and `exit(1)`; the engine
host maps the resulting WASI `proc_exit` import to a trap (EngineFault), so `checkImports` allows it. See
[limitations](../../docs/reports/limitations.md#swift-development-tree-after-v050).
