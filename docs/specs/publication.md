# Publication boundary

The public candidate contains the API and implementation, the engine ABI, generated
grammar modules, pinned source identities (`tools/gen/pins.json`), licenses, retained
tests and fixtures, reproducible tools (including the native C reference,
`tools/native-ref`), CI, technical contracts and selected evidence. The upstream runtime C is unmodified. Grammar C comes from the pinned grammar
sources under the [baseline and provenance contract](baseline-provenance.md). The seven-route C oracle gate and the 204-entry
basic catalog of ADR-0017 belonged to the removed carrier (ADR-0022; Git history at
`9ad82eb`); the current gate is the conformance lane (`docs/specs/validation.md`).

Session prompts, private handoffs, raw investigation output, temporary checkouts,
build caches, profiles, dumps and personal machine/account configuration stay
outside the public candidate. This applies to this project's own material.
Third-party corresponding-source archives are distributed unmodified, as
upstream publishes them. Build files that upstream committed inside such an
archive are third-party content, not this project's configuration, and are
disclosed where the archive is described. Keep original failures in the local archive and
their regression controls in the public test suite. A public projection of a
receipt identifies the original hash and states which fields were omitted;
it must not pretend to be an unmodified original execution receipt.

Public evidence uses repository-relative paths and binds source, commands,
environment, results and artifact hashes. Human-readable status is maintained
in `docs/reports/release-critical-status.md`. Machine-readable release results
are under `artifacts/release-candidate/` and in the corresponding CI run.

A candidate is checked from a fresh checkout, including official Go module
ZIPs and empty-cache consumers without replace directives. Product execution
uses CGO=0. Windows AMD64 and ARM64 must execute natively. Diagnostic race
results remain separate. License failures block the Release gate.

Publishing a curated branch does not remove previously published branches,
commits or cached copies. Review reachable history as well as the tip before
publication. Remote history replacement, branch deletion, visibility changes,
main merges and release tags each require explicit authorization for that act.
This includes a local merge into main, whatever the branch size (ADR-0019).
No publication workflow may infer those permissions from a passing test, and
no one acting under this contract uses a ruleset administrator bypass.

Rulesets (2026-11-21): `main-protection` (no deletion or non-fast-forward; 12 required CI checks), `tag-protection`
(no tag update or deletion) and `tag-creation` (tags other than `v*` and `grammars/*/v*` only by the owner). Task
branches need no ruleset; what is pushed is checked before the push (`AGENTS.md`), because the always-on owner bypass
made the former `branch-creation` ruleset a warning, not a guard. Version tags therefore need authority, not a bypass, and stay immutable.

Accepted ADRs preserve their original reasoning. Historical references in them
may identify local records; current public contracts and selected evidence must
be sufficient for the current product and its stated claims. ADR-0016 records
the publication and commit-boundary decision.

## Grammar pin-move evidence

A grammar pin move includes the evidence required by [ADR-0024 rule 6](../design/decisions/ADR-0024-grammar-source-tiers.md#amendment-2026-11-16-rule-6-following-a-maintained-fork).
The `pin-move.yml` workflow compares a PR's head with the merge base of its fetched/verified base and head, and publishes strict old/new
schema checks, schema diff codes/risks and native old/new tree comparisons over both pins' corpora plus the
grammar's SHA-256-pinned large inputs. Added grammars receive a schema check only; removed grammars are listed.
Push/manual runs without explicit comparison refs state **diff skipped**. Missing baseline/inputs, invalid
schema/dump evidence, native process failures and timeouts fail; schema removal/narrowing risks and changed
trees are review findings. An ok→error result also fails unless the PR body names its exact input on a visible
`PIN-MOVE-ALLOW-OK-TO-ERROR <input-id>` line. Overrides are retained with the result and cannot excuse missing
or invalid evidence. Existing error trees remain reported as errors.

The job summary and evidence artifact bind the selected commits, pins, input hashes, native results and overrides.
Keep the relevant artifact with the pin-review record; it contains source corpus and large-input bytes and tree
dumps, so review the artifact before any separate publication. The workflow has read-only repository permissions,
posts no PR comments and changes no pins. It does not replace conformance, bench, licence comparison or owner
approval. Commands, limits and input-ID syntax are documented in [tools/gen](../../tools/gen/README.md#pin-move-ci-evidence).

## Multi-module release

The root module and each grammar module are versioned and tagged independently under
[ADR-0028](../design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md#decision).
Go versions are immutable once served by a proxy. Each remote write below needs separate owner authority.

- **Root release.** Run steps 1–3 and 6–11. Carry forward the published grammar tags and their minimum root
  requirements. If selected grammars are also ready, publish any required new root first, then run steps 4–6
  for that subset. For root RC and 1.0, use ADR-0028's root scope, including actual released grammars for
  engine ABIs 1–4. Document grammar known gaps in the per-grammar support records.
- **Grammar-only release.** Run step 1 and steps 4–11 using the already published roots. Keep the existing
  grammar and pin-move gates. Record old and new pins, schema and tree/query changes, and migration examples,
  citing the owner release and owner CHANGELOG where applicable.
- **Combination table.** Publish the [root/grammar combination table](../reports/compatibility.md) and link it
  from the release notes. Bind each tested combination to its release commits, new and carried-forward tags,
  source revisions, generator identity, ABI, minimum roots and evidence scope.

Select modules for a new grammar tag when their source pin moves or they have an approved same-pin fix. A pin
move takes a grammar minor release (both modules for a shared TypeScript/TSX move). A same-pin fix that preserves
the declared contract may take a patch.

| # | Step | Owner authority |
|---|---|---|
| 1 | Run the applicable [release gates](validation.md#gates) on the candidate, including the scoped nightly-diff run, and retain and classify failures. Before tagging each candidate, rehearse the exact root and grammar versions in the combination table from a file `GOPROXY`. Build candidate ZIPs from their release commits using the `-version` flag in [`tools/modulezip`](../../tools/modulezip) and use the released ZIPs for carried-forward modules. Run the consumer checks below. For a selected grammar, do the ZIP rehearsal once its step-4 commit is ready. | no (local) |
| 2 | Tag the root `vX.Y.Z` on the release commit and push the tag. | **yes** |
| 3 | Verify the root: `GOPROXY=direct go list -m github.com/wotjr1649/go-treesitter@vX.Y.Z` from an empty module; then the same through `GOPROXY=https://proxy.golang.org`. | no (read-only) |
| 4 | Select the oldest published root that supplies each grammar's API, engine ABI and generated host imports. Put only the selected grammars into release form with `ROOT_VERSION=<minimum-root> bash tools/gen/regen.sh <new-empty-work-dir> <lang> [<lang> ...]`. Group grammars by minimum root and use a separate invocation per group. On this metadata conversion only the selected `go.mod` and `go.sum` change, and `engine_gen.*` stays byte-identical. Check that each selected module has no `replace`, and that unselected modules stay in development form. Commit through a pull request. | no (local commit) |
| 5 | Tag each selected module `grammars/<dir>/v0.x.y` on its step-4 release commit after its ZIP rehearsal. Push only those grammar tags. | **yes** |
| 6 | Repeat the consumer checks with `GOPROXY=direct`, then through `GOPROXY=https://proxy.golang.org`, using the published versions. A root-only release runs this check after step 3 using its new root and carried-forward grammar tags. | no (read-only) |
| 7 | Create a **draft** GitHub release and release notes for the root or grammar tag. While the project is a Preview, mark it pre-release. Upload every archive row with column 4 equal to `source` in `gen -mirror-assets` as described below, plus `SHA256SUMS`. | **yes** |
| 8 | Before publication, compare the uploaded asset digests (`gh release view <tag> --json assets`) with local `SHA256SUMS`. The inventory is exactly the source rows in `gen -mirror-assets` for the release pins, plus `SHA256SUMS`. Stop on missing, extra or differing assets. | no (read-only) |
| 9 | Publish the draft after step 8 passes. Publication makes the assets immutable. Then run `gh release verify <tag>` and retain its attestation result. | **yes** (publish); verification is read-only |
| 10 | From the published new release base (the intended first mirror base), download **every row with column 4 equal to `source`** in `gen -mirror-assets` by its exact filename into an empty directory and check its SHA-256 against the row and `SHA256SUMS`. Listing assets alone is insufficient. Record the base, pins identity and every result. | no (read-only) |
| 11 | In **one PR** after step 10, set `pins.json` `mirrors` to `[new release base, previous release base]` (for v0.6.0: `[v0.6.0 base, v0.5.0 base]`). Restore the selected grammar `go.mod` files from before step 4 and remove their release-generated `go.sum` files. After a root release, keep the new root `require` with `replace ../..` in `tools/conformance` and `tools/bench`, as in v0.5.0 and v0.6.0. Set it here if the root release preparation did not already set it. Confirm the paths and engine-byte hashes, and update the status board. | local preparation; remote integration needs authority |

**Consumer checks.** For each selected grammar release, test its declared minimum root with that grammar alone
and test the recommended root in a mixed consumer using the new tags and carried-forward tags. For root releases,
test the candidate root with the carried-forward grammar tags, including mixed-version combinations. In each case,
start with an empty module and module cache, use the explicit module versions and no `replace`, record
`go list -m all`, build with `CGO_ENABLED=0`, run a parse smoke test for each imported grammar and cross-build for
`linux/arm64`. A grammar's `require` sets its minimum root. Under [Go minimal version selection](https://go.dev/ref/mod#minimal-version-selection),
the highest root requirement in the graph wins. Upgrading one grammar can raise the root used by the entire consumer.

- The root tag (step 2) points at its tested commit. The root ZIP excludes the nested grammar modules. Only
  the selected grammar tags point at their step-4 commit.
- The root ZIP is built by `golang.org/x/mod/zip` from the tagged tree: it contains the root package, `internal/engine`,
  `testdata/`, `LICENSE`/`LICENSES/`, `docs/`, `.github/`, `AGENTS.md` and `tools/lane` (no `go.mod` there). It omits
  every nested module: `grammars/*`, `tools/gen`, `tools/bench`, `tools/conformance`, `tools/modulezip`, `artifacts`.
  Local-only paths (`docs/plans`, `docs/prompts`, `artifacts/handoff`, `.scratch`) must not be tracked, because a
  tracked file is published by the tag; check `git ls-files` before step 2.
- Between releases `main` keeps grammar modules in development form (`require` root `v0.0.0` with `replace ../..`
  and no `go.sum`) so they build against the working root. Without `ROOT_VERSION`, `regen.sh` reads each selected
  module's `go.mod`, including on a mixed release commit.
- **Retraction.** Fix a candidate before its tag is pushed with a new commit. Keep pushed and proxy-served
  tags immutable. Retract a bad version by adding `retract <bad-version> // reason` to that module's `go.mod`
  and tagging a newer version of the same module.

## Pinned archive release assets

Under the [2026-11-15 ADR-0026 amendment](../design/decisions/ADR-0026-never-block-on-upstream.md#amendment-2026-11-15),
immutable releases mirror source archives plus `SHA256SUMS` (v0.4.0: 24 source archives; v0.6.0: 26 source archives).
`(cd tools/gen && CGO_ENABLED=0 go run . -mirror-assets)` lists the candidate rows for the current pins (34) as
headerless TSV: filename, SHA-256, primary download URL, kind (`source`/`toolchain`). Select column 4 equal to `source`
(for example, `awk -F '\t' '$4 == "source"' release-assets.tsv`), excluding the `toolchain` rows.
Check the selected filenames equal `runtime.archive`, `wasiLibc.archive`, or a
`grammars` entry's `archive` in the release `pins.json`, deduplicating shared filenames: one archive per grammar
(TypeScript and TSX share one), the runtime and wasi-libc. Check that every selected row is present, that every
row's hash equals its pin, and that no other row is attached.

The excluded rows are `binaryen-version_133-x86_64-linux.tar.gz`, `binaryen-version_133-x86_64-windows.tar.gz`, `tree-sitter-linux-arm64.gz`, `tree-sitter-linux-x64.gz`, `tree-sitter-macos-arm64.gz`, `tree-sitter-windows-x64.gz`, `wasi-sdk-34.0-x86_64-linux.tar.gz`, `wasi-sdk-34.0-x86_64-windows.tar.gz`, because the exact toolchain archives' bundled licence/notice inventory is incomplete.
Toolchains continue to be fetched from their official releases with pinned SHA-256 checks; releases supply no
toolchain fallback assets until the complete notices can be established, for example from an SBOM of the exact
archives. Preserve the collected upstream licence texts as maintainer-local evidence for that later work.

Download each selected source row unchanged, check its listed SHA-256, and attach it with `SHA256SUMS` covering
those source files. The command emits filenames, SHA-256 pins, primary download URLs and kinds; it does not publish or filter the
output to the release scope. Existing source notices remain in the repository and grammar modules; this policy
adds no redistributed-toolchain licence bundle. Go modules (including wasm2go) continue to use Go's module checksum
mechanism; host toolchains remain prerequisites.

The draft digest comparison happens before publication; the anonymous first-base download check happens after
publication and covers every selected source row. Until step 11 is integrated, generation uses the previously
configured mirror bases. Do not point a base at a draft or imply that an older immutable release contains a newly
pinned archive. After step 11, repeat the source-row download/hash check against the **configured first base** to
confirm the effective configuration. Fetch plans may still list toolchain URLs at these bases; that does not mean
the releases contain those files or provide toolchain fallback coverage.
