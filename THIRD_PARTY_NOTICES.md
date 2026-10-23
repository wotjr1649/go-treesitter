# Third-party notices

This repository's own code is licensed under [MIT](LICENSE), as selected by
the project owner. Third-party works retain their original copyright notices.

| Component | Use | Original notice |
|---|---|---|
| gotreesitter | Pinned sources bundled under `internal/runtime`, with identified patches | [MIT](LICENSES/gotreesitter.txt) |
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

The table above identifies the seven-route C-oracle scope. The complete
[204-grammar inventory](LICENSES/catalog.json) binds each fixed repository,
commit, blob and notice. The base module contains 201 grammars; the separate
[`grammars/gpl`](grammars/gpl/README.md) module contains `caddy`, `disassembly`
and `jq`, including their scanners, generated tables and queries. A build tag
alone does not exclude source from a Go module ZIP, so this is a module boundary.
The optional module retains GPL-3.0 terms; it does not make a GPL-linked
application MIT-only. The C oracle remains development tooling, not a product
C runtime dependency.

The internal runtime also retains the original MIT notice at
`internal/runtime/LICENSE`. Its complete source and transformation inventory is
`internal/provenance/runtime.json`; it does not relabel upstream work as original
project code. Grammar licenses include MIT, Apache-2.0, ISC, CC0-1.0,
Unlicense, WTFPL and MPL-2.0; the base catalog is not uniformly MIT.
`nim` retains MPL-2.0 and its complete pinned source archive under
`LICENSES/sources/`. `wat` declares Apache-2.0 WITH LLVM-exception; both full
standard texts are included in `LICENSES/standard/`.

197 grammars have original license texts. Seven fixed repositories supply SPDX
package declarations without a license file; their unmodified metadata and the
referenced standard texts are included. Those standard texts are reference
terms, not newly invented upstream copyright notices. Elsa's original source
header and author notice are also retained.

Brightscript and Cooklang are not distributed. At their pinned commits,
`package.json` declares ISC and `Cargo.toml` declares MIT, with no license file
([ADR-0017](docs/design/decisions/ADR-0017-exclude-unclear-grammars-and-assurance-tiers.md)).

The TypeScript oracle inputs under `testdata/oracle/typescript-patched/` are
generated from the pinned TypeScript sources and their locked JavaScript base,
with gotreesitter's complete maintained grammar patch. Their manifest records
the upstream patch, generator, source hashes and applicable MIT notices above.
