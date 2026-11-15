# github.com/wotjr1649/go-treesitter/grammars/yaml

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter-grammars/tree-sitter-yaml at `a1c4812a73ec5e089de8e441fdea3a921e8d5079`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `33253f2997d1c560dd21e6cedada35f70fa15081eb34c2e428521fd18ba49e5c`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 2 (tree-sitter-grammars), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

YAML patch 0001 prevents premature end-of-input acceptance and silent source truncation.

- `patches/yaml/0001-no-early-eof.patch` changes `src/parser.c`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-yaml/issues/14.
