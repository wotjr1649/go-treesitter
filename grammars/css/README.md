# github.com/wotjr1649/go-treesitter/grammars/css

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-css at `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `541d8a46a449d3dfa29a9f632a31f73b71b4dbaf4d6964a6ff31d33a302f8d12`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The name patch accepts identifiers beyond Latin-1 in selectors, custom property names and references, and keyframe names. Bare Hangul declaration values still produce an error (see MIGRATING.md).

- `patches/css/0001-non-latin1-names.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-css/pull/105.
