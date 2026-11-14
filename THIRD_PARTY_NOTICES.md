# Third-party notices

This repository's own code is licensed under [MIT](LICENSE), as selected by
the project owner. Third-party works retain their original copyright notices.

Each generated grammar module under `grammars/` contains, in generated Go and data
files, the unmodified tree-sitter runtime C and its grammar's `parser.c`/`scanner.c`
(translated from wasm by `tools/gen`). Each module ships its grammar's license as its
own `LICENSE`.

| Component | Use | Original notice |
|---|---|---|
| tree-sitter | Runtime C inside every grammar module; native C reference builds | [MIT](LICENSES/tree-sitter.txt) |
| tree-sitter-go | `grammars/golang` | [MIT](LICENSES/tree-sitter-go.txt) |
| tree-sitter-python | `grammars/python` | [MIT](LICENSES/tree-sitter-python.txt) |
| tree-sitter-javascript | Base grammar of the TypeScript and TSX grammars | [MIT](LICENSES/tree-sitter-javascript.txt) |
| tree-sitter-typescript | `grammars/typescript`, `grammars/tsx` | [MIT](LICENSES/tree-sitter-typescript.txt) |
| tree-sitter-c-sharp | `grammars/csharp` | [MIT](LICENSES/tree-sitter-c-sharp.txt) |
| tree-sitter-json | `grammars/json` | MIT, [grammars/json/LICENSE](grammars/json/LICENSE) |
| tree-sitter-tsql | `grammars/tsql` | MIT, [grammars/tsql/LICENSE](grammars/tsql/LICENSE) |
| Newtonsoft.Json | Copied C# test fixtures | [MIT](testdata/newtonsoft/LICENSE.md) |

[The license manifest](LICENSES/manifest.json) records the exact public source
commit and SHA-256 of each copied notice. The Newtonsoft fixture manifest
separately binds that corpus and its license. The exact grammar commits and file
hashes of each module are in its `provenance.json`. No third-party license text
was rewritten as this project's license. The native C reference is development
tooling, not a product dependency.

## wasi-libc, translated into every grammar module

The wasm that `tools/gen` translates is linked against the libc of wasi-sdk 34
([wasi-libc](https://github.com/WebAssembly/wasi-libc) `2e6fb9d8ee0cdf9e431fbcabe8af3115de000a13`),
so each module's `engine_gen.go` contains translated wasi-libc code: 59 `libc.a`
members plus `crt1-reactor.o`, the same set for all seven grammars. wasi-libc as a
whole is multi-licensed under Apache-2.0 WITH LLVM-exception, Apache-2.0 and MIT, and
includes works under their own licenses: musl (MIT), cloudlibc (BSD-2-Clause) and
dlmalloc (CC0). The license texts are copied byte for byte into every module under
`LICENSES/wasi-libc/` (for example
[grammars/json/LICENSES/wasi-libc/](grammars/json/LICENSES/wasi-libc/)); `LICENSE`
there is the index. Their source commit and sha256 are pinned in
`tools/gen/pins.json` and recorded in each module's `provenance.json`. `compiler-rt`
builtins are on the link line, but a linker trace of the pinned recipe shows that no
member of it is linked into any of the seven modules; a change that links one needs
its notice (Apache-2.0 WITH LLVM-exception) added.
