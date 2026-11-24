# github.com/wotjr1649/go-treesitter/grammars/xml

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter-grammars/tree-sitter-xml at `5000ae8f22d11fbe93939b05c1e37cf21117162d`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `83886b4c40d156151e3c0f428baad2091c2f223af2dc9758f144213174684334`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 2 (tree-sitter-grammars), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The name patch lets Name tokens (attribute names including namespace declarations such as `xmlns:접두`, DOCTYPE, DTD and entity names) use the XML 1.0 NameStartChar and NameChar ranges instead of ASCII only; `:` still cannot start a name. The scanner patch compares complete UTF-8 tag names, so names that share a low byte stay distinct. Names are serialized as UTF-8 bytes in the 1024-byte scanner state with a 255-byte limit per name (85 Hangul syllables); beyond it, or with deeply nested tags, an end tag after an edit-time state restore can fail to match (see MIGRATING.md).

- `patches/xml/0001-non-ascii-names.patch` changes `common/common.mjs`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40.

- `patches/xml/0002-tag-name-code-points.patch` changes `xml/src/scanner.c`; upstream defect report: https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40.
