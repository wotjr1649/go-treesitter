# github.com/wotjr1649/go-treesitter/grammars/rust

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-rust at `77a3747266f4d621d0757825e6b11edcbf991ca5`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `c54f36dc2b7488b778bc730b584bd93ea89b103e46e67913442d5ac264c5a80f`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The block comment patch compares full code points for comment delimiters and documentation markers (upstream PR #316). The unpatched scanner read a code point whose low byte is `/`, `*` or `!` as that character, so previously error-free trees change: ordinary comments lose a false `inner`/`outer` marker and `doc` child; after a false nested opening is removed, the comment ends at the `*/` that closes every opening the scanner counts, and the rest is parsed as code (the scanner still does not count a `/*` that directly follows `/`, for example `//*`, an unchanged upstream limitation; where a false opening used to offset it, valid Rust such as `/* 是个 //* */ */` followed by code was error-free before and now ends the comment at the first `*/` with an `ERROR` over the second); after a false nested closing is removed or a previously hidden real `/*` is recognized, the block comment can extend to a later real `*/` and absorb nodes and query captures even in an error-free tree. When real closes run out, the tree can contain a `MISSING` `*/` or an `ERROR` that absorbs the following code. A documentation comment's `doc_comment` range can also change (see MIGRATING.md).

- `patches/rust/0001-block-comment-code-points.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-rust/issues/315.
