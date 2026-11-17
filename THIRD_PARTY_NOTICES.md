# Third-party notices

This repository's own code is licensed under [MIT](LICENSE), as selected by
the project owner. Third-party works retain their original copyright notices.

Each generated grammar module under `grammars/` contains, in generated Go and data
files, the unmodified tree-sitter runtime C and its grammar's `parser.c`/`scanner.c`
(with any recorded [ADR-0023](docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md) patches)
(translated from wasm by `tools/gen`). Each module ships its grammar's license as its
own `LICENSE`.

| Component | Use | Original notice |
|---|---|---|
| tree-sitter | Runtime C inside every grammar module; native C reference builds | [MIT](LICENSES/tree-sitter.txt) |
| tree-sitter-go | `grammars/golang` | [MIT](LICENSES/tree-sitter-go.txt) |
| tree-sitter-python | `grammars/python` | [MIT](LICENSES/tree-sitter-python.txt) |
| tree-sitter-javascript | `grammars/javascript` (including JSX); base grammar of TypeScript and TSX | [MIT](LICENSES/tree-sitter-javascript.txt) |
| tree-sitter-typescript | `grammars/typescript`, `grammars/tsx` | [MIT](LICENSES/tree-sitter-typescript.txt) |
| tree-sitter-c-sharp | `grammars/csharp` | [MIT](LICENSES/tree-sitter-c-sharp.txt) |
| tree-sitter-json | `grammars/json` | MIT, [grammars/json/LICENSE](grammars/json/LICENSE) |
| tree-sitter-postgres | `grammars/postgres` (SQL only) | [BSD-3-Clause](LICENSES/tree-sitter-postgres.txt) |
| tree-sitter-tsql | `grammars/tsql` | MIT, [grammars/tsql/LICENSE](grammars/tsql/LICENSE) |
| tree-sitter-java | `grammars/java` | [MIT](LICENSES/tree-sitter-java.txt) |
| tree-sitter-c | `grammars/c` | [MIT](LICENSES/tree-sitter-c.txt) |
| tree-sitter-cpp | `grammars/cpp` | [MIT](LICENSES/tree-sitter-cpp.txt) |
| tree-sitter-rust | `grammars/rust` | [MIT](LICENSES/tree-sitter-rust.txt) |
| tree-sitter-bash | `grammars/bash` | [MIT](LICENSES/tree-sitter-bash.txt) |
| tree-sitter-html | `grammars/html` | [MIT](LICENSES/tree-sitter-html.txt) |
| tree-sitter-css | `grammars/css` | [MIT](LICENSES/tree-sitter-css.txt) |
| tree-sitter-yaml | `grammars/yaml` | [MIT](LICENSES/tree-sitter-yaml.txt) |
| tree-sitter-php | `grammars/php` | [MIT](LICENSES/tree-sitter-php.txt) |
| tree-sitter-powershell | `grammars/powershell` | [MIT](LICENSES/tree-sitter-powershell.txt) |
| tree-sitter-r | `grammars/r` | [MIT](LICENSES/tree-sitter-r.txt) |
| tree-sitter-dart | `grammars/dart` | [MIT](LICENSES/tree-sitter-dart.txt) |
| tree-sitter-xml | `grammars/xml` | [MIT](LICENSES/tree-sitter-xml.txt) |
| tree-sitter-ruby | `grammars/ruby` | [MIT](LICENSES/tree-sitter-ruby.txt) |
| tree-sitter-kotlin | `grammars/kotlin` | [MIT](LICENSES/tree-sitter-kotlin.txt) |
| tree-sitter-swift | `grammars/swift` | [MIT](LICENSES/tree-sitter-swift.txt) |
| Newtonsoft.Json | Copied C# test fixtures | [MIT](testdata/newtonsoft/LICENSE.md) |
| regex-syntax 0.8.11 | Generated Unicode character and simple case-fold tables for query regex predicates | [MIT](LICENSES/regex-MIT.txt) or [Apache-2.0](LICENSES/regex-APACHE.txt) |
| Unicode data | Unicode 16.0.0 tables distributed in regex-syntax 0.8.11 | [Unicode data license](LICENSES/Unicode.txt) |

[The license manifest](LICENSES/manifest.json) records the exact public source
commit and SHA-256 of each copied notice. The Newtonsoft fixture manifest
separately binds that corpus and its license. The exact grammar commits and file
hashes of each module are in its `provenance.json`. No third-party license text
was rewritten as this project's license. The native C reference is development
tooling, not a product dependency.

`query_regex_tables.go` derives from the `regex-syntax` crate archive selected by
tree-sitter CLI 0.27.2's `Cargo.lock`. `tools/gen/queryunicode` checks the archive's
SHA-256 before generating the tables and copying its three license files byte for
byte. The generated category/script difference fixture records the Rust source
hash and the Go toolchain's Unicode version; `\p{...}` uses the latter's tables.

## wasi-libc, translated into every grammar module

The wasm that `tools/gen` translates is linked against the libc of wasi-sdk 34
([wasi-libc](https://github.com/WebAssembly/wasi-libc) `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`),
so each module's `engine_gen.go` contains translated wasi-libc code. wasi-libc as a
whole is multi-licensed under Apache-2.0 WITH LLVM-exception, Apache-2.0 and MIT, and
includes works under their own licenses: musl (MIT), cloudlibc (BSD-2-Clause) and
dlmalloc (CC0). The license texts are copied byte for byte into every module under
`LICENSES/wasi-libc/` (for example
[grammars/json/LICENSES/wasi-libc/](grammars/json/LICENSES/wasi-libc/)); `LICENSE`
there is the index. Their source commit and sha256 are pinned in
`tools/gen/pins.json` and recorded in each module's `provenance.json`. `compiler-rt`
builtins are on the link line; generation checks the linker trace and rejects any
linked archive member outside `libc.a`. A change that links compiler-rt needs its
notice (Apache-2.0 WITH LLVM-exception) added.
