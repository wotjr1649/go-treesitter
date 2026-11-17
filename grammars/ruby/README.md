# github.com/wotjr1649/go-treesitter/grammars/ruby

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-ruby at `ad907a69da0c8a4f7a943a7fe012712208da6dee`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `91df26f8934cb2cb7041effcd032580bcc383efdd4998981ab5c6c00cf6f17a1`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 1 (core), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The heredoc patch compares an ASCII delimiter byte exactly and a non-ASCII one by its stored low byte, so non-ASCII delimiters close; two non-ASCII code points with the same low byte still match each other.

- `patches/ruby/0001-heredoc-lookahead-char.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-ruby/issues/305.
