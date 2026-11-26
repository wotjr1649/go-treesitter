# github.com/wotjr1649/go-treesitter/grammars/python

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/tree-sitter/tree-sitter-python at `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 4
- recipe sha256: `cf1d220ca09e67cf55604e285c81ed2f0e1a936fd2813ac13bab38f4bf037a8e`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

The indentation patch bounds serialization by bytes written, preventing the state-buffer overrun that deep indentation causes when an odd number of string delimiters is open, e.g. inside one string literal (upstream PR #351). The `await` patch binds `await` tighter than `**`, so `await f() ** 2` groups as `(await f()) ** 2`; previously error-free trees change where an `await` operand was followed by `**` (upstream PR #360, see MIGRATING.md).

- `patches/python/0001-bound-serialized-indent-bytes.patch` changes `src/scanner.c`; upstream defect report: https://github.com/tree-sitter/tree-sitter-python/issues/350.

- `patches/python/0002-await-power-precedence.patch` changes `grammar.js`; upstream defect report: https://github.com/tree-sitter/tree-sitter-python/pull/360.
