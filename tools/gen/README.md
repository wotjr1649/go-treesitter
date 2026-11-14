# tools/gen

Generates one grammar module (`grammars/<lang>`) from pinned upstream C: clang (wasi-sdk) → wasm → wasm-opt
(recipe-dependent) → `wasm2go` → patched `memory_grow` → Go module. Design: `docs/design/engine-abi.md`, ADR-0020, ADR-0022.

## Pins (`pins.json`)

Everything a generation consumes is pinned, and the identity of a generated module never depends on the host:

| Input | Pinned as |
|---|---|
| tree-sitter runtime, grammars | full commit, GitHub API tarball sha256 (`api.github.com/repos/<owner>/<repo>/tarball/<commit>`), tree and file sha256 |
| wasi-sdk, binaryen | version plus the upstream release archive sha256 **per host platform** (`<GOOS>-<GOARCH>`; windows-amd64 and linux-amd64) |
| wasi-libc license texts | `wasiLibc`: the commit named by the sdk's `VERSION` file (the sdk ships no license text), tarball sha256, and each copied file's sha256; copied to `LICENSES/wasi-libc/` in every module and listed in its `provenance.json` outputs |
| wasm2go | module version and its `go.sum` `h1:` hash; built per host with `go install -trimpath` |
| host binaries | `localBinarySHA256`, checked only with `-verify-binaries`; never part of an identity |

`provenance.json` and the compiled `engine.Identity` record tool versions (`wasi-sdk`, `clang`, `binaryen`, `wasm2go`,
`wasm2go-sum`, `recipe`), not host binary hashes. Another host must reproduce the same `engine_gen.go` / `engine_gen.dat`.

## Regenerate

```
bash tools/gen/regen.sh <new-empty-work-dir> [lang ...]      # all seven modules by default
ROOT_VERSION=v0.1.0 bash tools/gen/regen.sh <work-dir>       # switch a development tree to the release go.mod form
```

`regen.sh` builds `gen`, prints the fetch plan (`gen -plan host`), downloads every archive into the new directory,
verifies its sha256 **before** extracting, builds wasm2go from the pinned version, then runs `gen` per module into
`grammars/<lang>`. `.github/workflows/regen.yml` runs it on Linux and requires `git diff --exit-code -- grammars/`.
Without `ROOT_VERSION`, `regen.sh` derives the go.mod form from the committed `grammars/*/go.mod` (all seven must be in the same form: development with `replace`, or release `require vX.Y.Z` without `replace`; otherwise it fails), so the drift check regenerates the committed form unchanged.

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

`bash tools/gen/balance-probe.sh <new-empty-work-dir>` fetches SHA-verified pinned inputs and builds all seven probes
under `<work-dir>/probes/` and native references under `<work-dir>/native/`, without regenerating product modules.
Then, from `tools/conformance`:

```sh
CGO_ENABLED=0 GOWORK=off TSG_BALANCE_PROBE_DIR=<absolute-work-dir>/probes \
  TSG_NATIVE_EXE_DIR=<absolute-work-dir>/native TSG_CONF_OUT=<absolute-work-dir>/out \
  go test -count=1 -timeout 90m -v -run '^TestBalancingCancel$' .
```

The nightly/manual Linux job `.github/workflows/balancing-cancel.yml` runs this lane. It is not a PR-required check.
