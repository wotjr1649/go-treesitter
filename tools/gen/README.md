# tools/gen

Generates one grammar module (`grammars/<lang>`) from pinned upstream C: clang (wasi-sdk) → wasm → wasm-opt
(recipe-dependent) → `wasm2go` → patched `memory_grow` → Go module. Design: `docs/design/engine-abi.md`, ADR-0020, ADR-0022.

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
bash tools/gen/regen.sh <new-empty-work-dir> [lang ...]      # all 15 modules by default
ROOT_VERSION=v0.1.0 bash tools/gen/regen.sh <work-dir>       # switch a development tree to the release go.mod form
```

`regen.sh` builds `gen`, prints the fetch plan (`gen -plan host`), downloads every archive into the new directory,
verifies its sha256 **before** extracting, builds wasm2go from the pinned version, then runs `gen` per module into
`grammars/<lang>`. `.github/workflows/regen.yml` runs it on Linux and requires `git diff --exit-code -- grammars/`.
Without `ROOT_VERSION`, `regen.sh` derives the go.mod form from the committed `grammars/*/go.mod` (all 15 must be in the same form: development with `replace`, or release `require vX.Y.Z` without `replace`; otherwise it fails), so the drift check regenerates the committed form unchanged.

The modules are `bash`, `c`, `cpp`, `csharp`, `css`, `golang`, `html`, `java`, `javascript` (including JSX), `json`,
`python`, `rust`, `tsql`, `tsx`, and `typescript`. Each has a grammar-specific generated smoke-test sample.
Downloads allow three attempts with 1s/2s backoff only for transient transport failures. HTTP, certificate and local
write errors end that URL attempt; transient HTTP 408/429/5xx responses also receive bounded retries. A failed upstream download or hash check then tries each configured mirror; sha256 verification remains mandatory before extraction. The retry self-check is
`bash tools/gen/fetch_test.sh`.

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

`bash tools/gen/balance-probe.sh <new-empty-work-dir> [lang ...]` fetches SHA-verified pinned inputs and builds all 15 probes
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

List the exact release attachments from all pinned platforms (deduplicated filename, SHA-256, upstream URL):

```sh
(cd tools/gen && CGO_ENABLED=0 go run . -mirror-assets) > release-assets.tsv
```

The list includes the runtime, every grammar archive, wasi-libc, both host SDK/binaryen archives, and the pinned
CLI executables' gzip archives, including when no real grammar patch uses the CLI yet. Download each listed upstream
URL into a file named by column one, check column two, and attach these unchanged files to each immutable `vX.Y.Z`
release. Before releasing a new pin set, update `mirrors` to that release and retain older bases only when useful.
The configured `v0.3.0` mirror becomes available when its assets are published; this command does not upload anything.
`wasm2go` remains a Go module installed through Go's checksum-verified module mechanism (`wasm2goSum`), not an archive
in this list. The Go toolchain and system C compiler are host prerequisites, not downloads managed by this pin file.
