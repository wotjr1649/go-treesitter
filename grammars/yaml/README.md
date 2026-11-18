# github.com/wotjr1649/go-treesitter/grammars/yaml

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter-grammars/tree-sitter-yaml at `a1c4812a73ec5e089de8e441fdea3a921e8d5079`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `2909e4b8974da4cacb0b9e33cc9e890accdc27e529436acbbf9ac2f30b17939c`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 2 (tree-sitter-grammars), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

YAML patch 0001 prevents premature end-of-input acceptance and silent source truncation.

YAML patch 0002 bounds serialized indentation by bytes written, preventing deep-nesting buffer overruns (upstream PR #47).

YAML patch 0003 widens scanner rows to 32 bits, so documents past 32,767 line breaks parse, counting CR and LF separately: line 16,385 is the boundary with CRLF line ends (adapted from upstream PR #35: the block implicit key row is serialized as a saturating 16-bit distance, keeping 253 indentation levels within the 0002 bound).

- `patches/yaml/0001-no-early-eof.patch` changes `src/parser.c`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-yaml/issues/14.

- `patches/yaml/0002-bound-serialized-indent-bytes.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-yaml/issues/48.

- `patches/yaml/0003-widen-scanner-rows.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-yaml/issues/49.
