# github.com/wotjr1649/go-treesitter/grammars/html

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-html at `73a3947324f6efddf9e17c0ea58d454843590cc0`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `186066f60a6a0514e34b8ab830bbe77e95aa08387fad2578148d3d1484ccbb90`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

Patch 0001 accepts `>` in element text (upstream PR #141). Patch 0002 is this project's own fix for upstream issue #143, which has no upstream pull request. The upstream `entity` token matches bounded prefixes with an optional `;`. The forms are `&` followed by 1–30 ASCII letters, `&#` followed by 1–5 decimal digits, and `&#x` or `&#X` followed by 1–6 hexadecimal digits. For example, `R&D` gives `text R` and `entity &D`, and `&#x26x` gives `entity &#x26` and `text x`. Other `&` characters are text. A bare `&` followed by whitespace can join a `text` token when text continues after the whitespace (`Users & roles` is one `text`). If a tag, an entity or end of input follows the whitespace, the `&` can be a separate `text` node.

- `patches/html/0001-greater-than-text.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-html/pull/141.

- `patches/html/0002-bare-ampersand-text.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-html/issues/143.
