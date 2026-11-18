# github.com/wotjr1649/go-treesitter/grammars/javascript

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/wotjr1649/tree-sitter-javascript at `d6501de34e9aabad1f49b860fcfcc23066766223`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `4dded19a1e467ebfcc6aa305a26576cdb811302fa7521d9cd3284b9e165e86e1`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` and `NOTICE` are the upstream grammar's license files. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The JSX text patch accepts a bare `&` in JSX text; character references still produce `html_character_reference` (upstream PR #388). TSX has the same defect and is not patched.

- `patches/javascript/0001-jsx-bare-ampersand.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-javascript/pull/388.
