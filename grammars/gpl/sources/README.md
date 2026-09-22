# Source and reproduction

The three grammar archives contain the complete fixed repositories:

| Grammar | Repository | Commit |
|---|---|---|
| caddy | opa-oz/tree-sitter-caddy | 9b3fde99d3d74345b85b655a6d8065e004fbe26f |
| disassembly | ColinKennedy/tree-sitter-disassembly | 0229c0211dba909c5d45129ac784a3f4d49c243a |
| jq | nverno/tree-sitter-jq | 1e139eba1fd3a9c34a36f0f0f47ed8b73c9b4636 |

The distributed blobs, Go scanners and external scanner tables are unchanged
artifacts from gotreesitter v0.53.0, commit
`c871b1f576866c40b1695677fe3512243e266d39`. The parent repository's
`tools/runtime-bundle/import.py --optional-output NEW_DIRECTORY` reproduces
the eight generated adapter files from that exact module archive, relocates
imports and registers the three entries. `provenance.json` in this module
binds the resulting files, source archives and notices.

`ts2go-source.zip` includes the unmodified conversion package, its required
runtime packages/assets, MIT notice and original module files.
`ts2go-source.json` records the archive of origin and every extracted file hash.
`golang-x-sync-v0.11.0.zip` supplies the converter's build dependency and its
original BSD notice. Extract both into separate directories, create a temporary
Go workspace using those two directories, and build `./cmd/ts2go` in the
converter directory. No dependency installation is needed when the workspace
supplies this dependency. The product module does not import this tool.

For a grammar archive extracted into `GRAMMAR_DIRECTORY`, run:

```text
ts2go -input GRAMMAR_DIRECTORY/src/parser.c -output OUTPUT_DIRECTORY/grammar.go -name LANGUAGE
```

The upstream v0.53.0 converter emits a newer Gob schema and metadata than its
three bundled blobs. A native Windows rebuild succeeded for all three, but
its bytes are not the upstream artifact bytes. A complete exported-field
comparison identified only these decoded differences:

- all three rebuilt blobs record their C ABI, while the bundled field is zero;
- disassembly embeds the same external lex-state table that the bundle supplies
  through its Go sidecar and marks recovery-cost capability;
- jq marks recovery-cost capability and adds three converter conflict policies.

The actual lexer and parser tables otherwise compared equal. The optional
module deliberately preserves the pinned upstream artifacts and behavior;
it does not silently adopt the converter's newer policy metadata. Therefore
artifact import reproduction and C-source regeneration are separate claims.
The raw failed byte/field equality checks are retained in the parent
repository's `artifacts/session-07/full-pass/05-licenses` evidence and associated
command receipts. No exact C-source-to-blob byte reproduction is claimed.
