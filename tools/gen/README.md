# tools/gen

Generates one grammar module (`grammars/<lang>`) from pinned upstream C: clang (wasi-sdk) → wasm → wasm-opt
(recipe-dependent) → `wasm2go` → patched `memory_grow` → Go module. Design: `docs/design/engine-abi.md`, ADR-0020, ADR-0022.

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
is recorded in [the schema register](../../docs/validation/grammar-schemas.md#offline-strict-check-inputs). No CI wiring is added;
the first pin move also needs explicit base/head selection and an old/new native tree comparison.

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
bash tools/gen/regen.sh <new-empty-work-dir> [lang ...]      # all 23 modules by default
ROOT_VERSION=v0.1.0 bash tools/gen/regen.sh <work-dir>       # switch a development tree to the release go.mod form
```

`regen.sh` builds `gen`, prints the fetch plan (`gen -plan host`), downloads every archive into the new directory,
verifies its sha256 **before** extracting, builds wasm2go from the pinned version, then runs `gen` per module into
`grammars/<lang>`. `.github/workflows/regen.yml` runs it on Linux and requires `git diff --exit-code -- grammars/`.
Without `ROOT_VERSION`, `regen.sh` derives the go.mod form from the committed `grammars/*/go.mod` (all 23 must be in the same form: development with `replace`, or release `require vX.Y.Z` without `replace`; otherwise it fails), so the drift check regenerates the committed form unchanged.

The modules are `bash`, `c`, `cpp`, `csharp`, `css`, `dart`, `golang`, `html`, `java`, `javascript` (including JSX), `json`,
`kotlin`, `php`, `powershell`, `python`, `r`, `ruby`, `rust`, `tsql`, `tsx`, `typescript`, `xml`, and `yaml`. Each has a grammar-specific generated smoke-test sample.
Downloads allow three attempts with 1s/2s backoff only for transient transport failures. HTTP, certificate and local
write errors end that URL attempt; transient HTTP 408/429/5xx responses also receive bounded retries. A failed upstream download or hash check then tries each configured mirror; sha256 verification remains mandatory before extraction. The retry self-check is
`bash tools/gen/fetch_test.sh`.

Compare raw format-2 field-exception records with `python tools/gen/check-field-export.py --before BEFORE_TREE --after AFTER_TREE --jobs JOBS_JSON --out NEW_DIR`; exact Q13 arguments (`q13/run.py`, `differential-final`), input hashes (`differential-jobs.json`) and results (`differential-final/receipt.json`) are kept in the maintainer-local evidence archive. The source and result identities are recorded in [engine-abi.md](../../docs/design/engine-abi.md#field-lookup-evidence).

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

`bash tools/gen/balance-probe.sh <new-empty-work-dir> [lang ...]` fetches SHA-verified pinned inputs and builds all 23 probes
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
YAML carries recorded C-only patch `0001-no-early-eof`, preventing premature end-of-input acceptance and truncated
roots. Retire it with an approved pin move when upstream fixes it; `native-ref -unpatched` supplies the required
corpus/fuzz baseline.

For `grammar.js` or related grammar inputs, also set the grammar's `regenerate: true`. `gen` and `native-ref` use
the same implementation: verify the full archive, extract a private copy, apply the recorded patches, then run
the pinned CLI twice on independent trees using `generate --js-runtime native`. Every resulting file must match.
The CLI gzip and its decompressed executable are checked against the per-platform pin before execution. The
generated module's optional `grammarGeneration` provenance records CLI version/runtime, patched input-tree hash
and generated source hashes; `native-ref` writes the same record under its `work/patches-<lang>/provenance.json`.
Both compilers read C from this private result. The original extracted source and archive stay intact.

`regen.sh` fetches the CLI only when a grammar requires regeneration; direct `gen` invocations supply `-tree-sitter`
and keep the corresponding pinned gzip in `-dl`. `native-ref -fetch` downloads it on demand. Existing unpatched
modules are unchanged. Patches modifying multiple files are represented by ordered single-file records.

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

The command lists 30 rows for the current pins: 24 `source` archives and six `toolchain` archives. v0.4.0 release
assets are **only the 24 rows with column 4 equal to `source`, plus `SHA256SUMS`**. Check their filenames against `runtime.archive`,
`wasiLibc.archive`, and each grammar's `archive` in `pins.json`, deduplicating TypeScript/TSX's shared archive;
the [publication procedure](../../docs/specs/publication.md#pinned-archive-release-assets) specifies the inventory
and digest checks. The six SDK/binaryen/CLI host archives are excluded until complete bundled notices can be
established. Those tools are fetched from their official releases with pinned SHA-256 checks and have no assets
in this release to provide mirror fallback, even if the fetch plan lists URLs at the configured bases.

After publishing and checking all selected source rows at the new release base, update `mirrors` in a follow-up PR;
for v0.4.0 that PR sets the ordered bases to v0.4.0 then v0.3.0 and restores the development module form. A configured
base can supply only the exact files already published there; a new source archive gains fallback coverage when a
carrying release is published and its base is configured. Published releases are immutable. This command does not
upload anything. `wasm2go` remains a Go module installed through Go's checksum-verified module mechanism
(`wasm2goSum`), not an archive in this list. The Go toolchain and system C compiler are host prerequisites, not
downloads managed by this pin file.
