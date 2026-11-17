# github.com/wotjr1649/go-treesitter/grammars/postgres

Generated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of
`tools/gen` (docs/design/engine-abi.md, ADR-0020).

- upstream grammar: https://github.com/gmr/tree-sitter-postgres at `59d0d8cd7506d68de1229fb4bbce838c83b60c8a`
- tree-sitter runtime: v0.27.2 (unmodified)
- engine ABI: 3
- recipe sha256: `19868ed294d3d9807462305ce34cdf90fa7f3207778c88bd70f723f6d56692ee`
- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)

`LICENSE` is the upstream grammar's license. `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`;
its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;
every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.

Source tier: owner-designated, per ADR-0024. PostgreSQL SQL only; PL/pgSQL is not included.

This grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):

Owner-approved exception (2026-11-17) to ADR-0023 decision 1 for Korean support; this gap does not block a release gate. Unquoted identifiers accept all non-ASCII code points, and dollar-quote tags compare complete code points. The upstream 63-code-point tag cap remains.

- `patches/postgres/0001-non-ascii-identifiers.patch` changes `postgres/grammar.js`; upstream defect report: https://github.com/gmr/tree-sitter-postgres/issues/101.

- `patches/postgres/0002-dollar-tag-code-points.patch` changes `postgres/src/scanner.c`; upstream defect report: https://github.com/gmr/tree-sitter-postgres/issues/101.
