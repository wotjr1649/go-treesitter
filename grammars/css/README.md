# github.com/wotjr1649/go-treesitter/grammars/css

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-css at `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `61273387a9631d4b962b34c727c2e528cd448589fa4302fd3601b087719fdc05`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

An independently implemented name patch addresses the defect reported in upstream PR #105. It extends class, ID, type and attribute names, property names, custom property references, and keyframe names. The patch leaves bare identifier attribute values, unquoted URLs, bare declaration values, units, and at-rule names and arguments outside its scope. See MIGRATING.md for the descendant selector limit and retirement criteria.

- `patches/css/0001-non-latin1-names.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-css/pull/105.
