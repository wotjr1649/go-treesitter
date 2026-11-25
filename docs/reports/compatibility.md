# Root and grammar compatibility

Add a row for each tested combination when publishing a root or grammar release. Record the root requested and the
root selected by Go, the exact grammar tags and commits, and whether each tag is new or carried forward. For each
grammar record its declared minimum root and engine ABI, and link to the source revision and generator identity in
the release's pins and module provenance. Group modules when their versions, commits, minimum root, ABI and
evidence scope match. Record minimum-root and recommended-root consumer results with their platform, commands and
evidence links. The [publication procedure](../specs/publication.md#multi-module-release) defines the checks and
the [per-grammar support table](grammar-support.md) tracks known grammar limits by tag. The
[limitations report](limitations.md) covers the wider preview scope.

| Requested / selected root (commit) | New grammar tags (commit) | Carried-forward tags | Minimum root | Engine ABI | Source and generator identities | Consumer evidence |
|---|---|---|---|---|---|---|
| `v0.6.0` / `v0.6.0` ([`f723d8e`](https://github.com/wotjr1649/go-treesitter/commit/f723d8ea61d2077ad907ddd6a21921476affacf2)) | `grammars/<dir>/v0.6.0` for all 25 modules in the release pins, including Swift's first tag ([`a8af436`](https://github.com/wotjr1649/go-treesitter/commit/a8af4360e4964deb6c0bb62461ee8348be1aa738)) | None | `v0.6.0` for all 25 | 4 | [Release pins](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/tools/gen/pins.json) record each `grammars.<dir>.commit` and the tool identities (CLI and runtime 0.27.2, recipe `v2-O3`). Each `grammars/<dir>/provenance.json` on the grammar commit records the module's generation identity. | [Published v0.6.0 checks](release-critical-status.md#released): empty-cache Windows amd64 consumers on Go 1.27.4, direct and public proxy, no `replace`, `CGO_ENABLED=0`, 25 clean parses and a `linux/arm64` cross-build. All module versions in this historical run were `v0.6.0`. |
