# Query regex Unicode tables

Run from `tools/gen`:

```sh
go run ./queryunicode -root ../..
go run ./queryunicode -root ../.. -check
```

`-archive /path/to/regex-syntax-0.8.11.crate` uses a previously downloaded archive;
its SHA-256 is checked exactly as for a fresh download. No Rust compiler or Go
dependency is required. `tools/gen/pins.json` is independent of this generator.

The source is the crates.io `regex-syntax` 0.8.11 archive, SHA-256
`d6f6ff9a378485b298a5286656da665ba74413d36db0979633275d2e708145d4`, matching
[tree-sitter CLI 0.27.2's Cargo.lock](https://github.com/tree-sitter/tree-sitter/blob/v0.27.2/Cargo.lock).
Its four generated source headers identify Unicode 16.0.0. The embedded crate
VCS metadata points to `rust-lang/regex` commit
`140167995737fa11dfe11b8af8b9aa143b790b4e`.

The generator emits `query_regex_tables.go`, `testdata/query-regex-unicode.json`,
and the three copied license files. It also checks all exact Go category/script
names against Rust's property aliases for binary-first name collisions, respecting
Rust's `Cf`, `Sc`, and `LC` exceptions. No such collisions occur in the recorded
Go Unicode 17.0.0 list.

The JSON fixture records every scalar-value difference between Rust's Unicode
16.0.0 general-category/script tables and the generating Go toolchain's tables,
including empty Rust tables for scripts absent from that Unicode version.
`GoOnly` and `RustOnly` endpoints are inclusive decimal code points. Each Rust
table hash is over the sorted scalar values formatted as uppercase six-digit
hexadecimal, one per line. The product test reconstructs those Rust sets from
Go and the recorded differences, then checks the hashes. On another Unicode
version it skips this version-specific evidence check; query `\p{...}` still
uses that toolchain's own tables. Regenerate the fixture intentionally when
recording a new Go Unicode version.
