# github.com/wotjr1649/go-treesitter/grammars/golang

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-go at `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `bb49b3aa0a93195a2f1806331415a3d435c8871607b8a5806ba4c0cdf6689c30`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The Go 1.26 patch accepts expressions as the first argument to `new` and prefers the type interpretation when ambiguous. The first argument to `make` still uses the type rule (adapted from upstream PR #202).

- `patches/golang/0001-new-value-argument.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-go/issues/201.
