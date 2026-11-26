# github.com/wotjr1649/go-treesitter/grammars/ruby

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-ruby at `ad907a69da0c8a4f7a943a7fe012712208da6dee`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `bfcca938dca35cad51e79cc208e180a32bd422c6ddf31b6bb0b7bf06cba91804`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 1 (core), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The heredoc patch stores and compares UTF-8 end words, preserving ASCII bytes and distinguishing code points that share a low byte. Invalid lookaheads all compare as 0xFF, distinct from U+FFFD. The 1024-byte scanner state buffer includes all open end words and metadata; overflow loses state (about 330 Hangul syllables total, fewer with other state).

The global-variable patch accepts non-ASCII names such as `$한글`. Previously error-free trees change where an ASCII global name is directly followed by non-ASCII characters (for example `$a한 = 1` or `"#$a한"`): the whole name becomes one `global_variable` (see MIGRATING.md).

The symbol patch compares whole code points when the scanner reads a plain `:symbol` name (no `@`, `@@` or `$` prefix and no operator), so non-ASCII names such as `:한글` parse as `simple_symbol`. Every non-ASCII code point (including non-ASCII white space such as U+3000) continues such a name, as in Ruby; before, only its low byte was tested. Variable and operator symbols and the name after `#@` in a string still use the low byte (for example `:@한글`, `"#@일"` and `"#Ĥ!"`).

- `patches/ruby/0001-heredoc-utf8.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-ruby/issues/305.

- `patches/ruby/0002-global-variable-non-ascii.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-ruby/issues/306.

- `patches/ruby/0003-non-ascii-symbol.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-ruby/issues/292.
