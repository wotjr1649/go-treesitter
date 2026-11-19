# github.com/wotjr1649/go-treesitter/grammars/xml

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter-grammars/tree-sitter-xml at `5000ae8f22d11fbe93939b05c1e37cf21117162d`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `8954d1ea0ab716c7cfc6fe6ddd8b382723316a4dd703e92c1bcb0b77ebe2e2c0`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 2 (tree-sitter-grammars), per ADR-0024.
