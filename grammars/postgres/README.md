# github.com/wotjr1649/go-treesitter/grammars/postgres

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/gmr/tree-sitter-postgres at `83c0cc2f4401d5e5944834a09d3f8c24184398fa`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `55418e401717da436bca22c5361308f570e503e7706d465f040c72bbef73136f`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: owner-designated, per ADR-0024. PostgreSQL SQL only; PL/pgSQL is not included.
