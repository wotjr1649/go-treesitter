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
