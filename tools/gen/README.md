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
ROOT_VERSION=v0.1.0 bash tools/gen/regen.sh <work-dir>       # release go.mod form
```

`regen.sh` builds `gen`, prints the fetch plan (`gen -plan host`), downloads every archive into the new directory,
verifies its sha256 **before** extracting, builds wasm2go from the pinned version, then runs `gen` per module into
`grammars/<lang>`. `.github/workflows/regen.yml` runs it on Linux and requires `git diff --exit-code -- grammars/`.

## go.mod forms

- default (development): `require github.com/wotjr1649/go-treesitter v0.0.0` and `replace … => ../..`;
- `-root-version vX.Y.Z` (release, ADR-0022): `require github.com/wotjr1649/go-treesitter vX.Y.Z`, no `replace`.

The release form cannot be built before the root tag exists; the generator's meta runner always uses its own local
`replace`, so generating in release mode works before then.
