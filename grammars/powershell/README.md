# github.com/wotjr1649/go-treesitter/grammars/powershell

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/airbus-cert/tree-sitter-powershell at `65ea6919e816a7ac142acec7ee72757f89db8439`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `a138cc1dca3286f6a7c8d96d3606b2a1820fe438e675f7aab00fb89ed786870c`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: 3 (third party), per ADR-0024.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The name patch widens the ASCII-only name patterns of variables (including scope prefixes), command parameters, simple names (hash keys, member names, class and enum names) and type identifiers to Unicode letters (`\p{L}`), decimal digits (`\p{Nd}`) and `_`. ASCII names match as before; a name after `$` in an expandable string can now continue over non-ASCII letters, and a `$` directly followed by one now starts a variable there (see MIGRATING.md). Statement labels stay ASCII-only: `:한글` at the start of a statement is still read as a command name. Supplementary-plane letters are accepted as the specification says, although PowerShell 7.6.10 reads names by UTF-16 code unit and rejects them (one-off check, E2).

- `patches/powershell/0001-non-ascii-names.patch` changes `grammar.js`; upstream defect report: https://github.com/airbus-cert/tree-sitter-powershell/issues/70.
