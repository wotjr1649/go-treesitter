# Third-party notices

This repository's own code is licensed under [MIT](LICENSE), as selected by
the project owner. Third-party works retain their original copyright notices.

| Component | Use | Original notice |
|---|---|---|
| gotreesitter | Pinned Go runtime dependency | [MIT](LICENSES/gotreesitter.txt) |
| tree-sitter | Separate native C oracle tooling | [MIT](LICENSES/tree-sitter.txt) |
| tree-sitter-go | Go grammar | [MIT](LICENSES/tree-sitter-go.txt) |
| tree-sitter-python | Python grammar | [MIT](LICENSES/tree-sitter-python.txt) |
| tree-sitter-javascript | JavaScript and JSX grammar | [MIT](LICENSES/tree-sitter-javascript.txt) |
| tree-sitter-typescript | TypeScript and TSX grammars | [MIT](LICENSES/tree-sitter-typescript.txt) |
| tree-sitter-c-sharp | C# grammar | [MIT](LICENSES/tree-sitter-c-sharp.txt) |
| Newtonsoft.Json | Copied C# validation fixtures | [MIT](testdata/newtonsoft/LICENSE.md) |

[The license manifest](LICENSES/manifest.json) records the exact public source
commit and SHA-256 of each copied notice. The Newtonsoft fixture manifest
separately binds that corpus and its license. No third-party license text was
rewritten as this project's license.

The grammar notices above cover the six grammars in this repository's declared
seven-route validation scope. The upstream default aggregate package embeds a
larger catalog; its other grammars are outside this notice inventory and release
assessment. The selected embedded build documented in the README includes the
declared grammar set. The C oracle is development tooling and is not a C runtime
dependency of the product library.
