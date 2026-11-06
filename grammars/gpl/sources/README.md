# Source and reproduction

The three grammar archives contain the complete fixed repositories:

| Grammar | Repository | Commit |
|---|---|---|
| caddy | opa-oz/tree-sitter-caddy | 2b0dd9066900568a3d6b33dc51d2e271cb48bd92 |
| disassembly | ColinKennedy/tree-sitter-disassembly | 0229c0211dba909c5d45129ac784a3f4d49c243a |
| jq | nverno/tree-sitter-jq | 1e139eba1fd3a9c34a36f0f0f47ed8b73c9b4636 |

The Caddy corresponding-source archive is pinned to commit `2b0dd9066900568a3d6b33dc51d2e271cb48bd92` and is downloaded from `https://codeload.github.com/opa-oz/tree-sitter-caddy/zip/2b0dd9066900568a3d6b33dc51d2e271cb48bd92`. The archive is 107,192 bytes, SHA-256 `81de66ca77125a8b8594547cdafd2aa07d3d66bd179cae1f8b8ff79e5fa61e55`; its 87 ZIP entries passed path/root/symlink checks. It is the unmodified GitHub archive of that commit. It includes build files that upstream committed (for example `build/config.gypi`), which contain the upstream author's local build paths. The existing Caddy license notice remains byte-identical at SHA-256 `3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986`.

**Caddy binding:** LICENSES/catalog.json now binds the current candidate Caddy blob SHA-256 94b81aa106461eaa9ef1ed8b3d25abdae36ac10b7b659ed3ec9bc21634c277db to repository commit 2b0dd9066900568a3d6b33dc51d2e271cb48bd92, with the license source pinned to that commit. The verified source archive above and preserved license notice (3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986) match this source identity. The GPL candidate's Caddy payload is one of nine optional files imported from gotreesitter v0.56.0, commit ac4e027e02525e9d1d005c54ed5e54979271e4ed, archive SHA-256 0925d93fadca3ad943e20594d602c3639d79f9ad8f82d466262672890d7592a6. provenance.json binds the central runtime manifest and the hashes of every GPL module file. Packaging and consumer results for a candidate are recorded in docs/reports/release-critical-status.md.

The current candidate's optional adapter payload (three grammar blobs, scanners, external scanner tables, catalog and binding helper) contains nine files matching the importer output for gotreesitter v0.56.0, commit ac4e027e02525e9d1d005c54ed5e54979271e4ed, source archive SHA-256 0925d93fadca3ad943e20594d602c3639d79f9ad8f82d466262672890d7592a6. The parent importer tools/runtime-bundle/import.py --optional-output NEW_DIRECTORY produces the optional source output. The payload remains in the GPL-specific grammars/gpl module. This local output identity does not establish official module-consumer validation or product adoption.

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

The historical comparison below used the v0.53.0 blobs. It found that the v0.53.0 converter emitted newer Gob schema and metadata; a native Windows rebuild succeeded for all three, but its bytes differed from the upstream artifacts. These observations describe that old identity only. v0.56.0 C-source regeneration has not been validated.

- all three rebuilt blobs record their C ABI, while the bundled field is zero;
- disassembly embeds the same external lex-state table that the bundle supplies
  through its Go sidecar and marks recovery-cost capability;
- jq marks recovery-cost capability and adds three converter conflict policies.

The actual lexer and parser tables otherwise compared equal in the historical v0.53.0 comparison. For the v0.53.0 module, the imported upstream artifacts were preserved rather than replaced with converter output. Artifact import reproduction and C-source regeneration therefore remained separate claims. The raw failed byte/field equality checks are retained in the parent repository's artifacts/session-07/full-pass/05-licenses evidence and associated command receipts; no exact C-source-to-blob byte reproduction is claimed for that historical comparison. For the v0.56.0 candidate, the GPL payload is established by importer output hashes, and official ZIP consumer results are recorded per candidate commit in docs/reports/release-critical-status.md. C-source regeneration of these three blobs has not been validated.
