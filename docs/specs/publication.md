# Publication boundary

The public candidate contains the API and implementation, the engine ABI, generated
grammar modules, pinned source identities (`tools/gen/pins.json`), licenses, retained
tests and fixtures, reproducible tools (including the native C reference,
`tools/native-ref`), CI, technical contracts and selected evidence. The runtime and
grammar C are unmodified upstream. The seven-route C oracle gate and the 204-entry
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

Rulesets (2026-11-14): `main-protection` (no deletion or non-fast-forward; 12 required CI checks), `branch-creation`
(new branches only by the owner), `tag-protection` (no tag update or deletion) and `tag-creation` (tags other than
`v*` and `grammars/*/v*` only by the owner). Version tags therefore need authority, not a bypass, and stay immutable.

Accepted ADRs preserve their original reasoning. Historical references in them
may identify local records; current public contracts and selected evidence must
be sufficient for the current product and its stated claims. ADR-0016 records
the publication and commit-boundary decision.

## Multi-module release

The root module and each grammar module are versioned and tagged separately (ADR-0022 §2). Go versions are
immutable once a proxy has served them, so the order below is fixed. Every numbered step that writes to the remote
needs the owner's explicit authority for that act (above); one grant does not cover the next step.

| # | Step | Owner authority |
|---|---|---|
| 1 | On the release commit, run the release gates (`docs/specs/validation.md`) and the rehearsal: module ZIPs for the root and every generated grammar listed in `tools/gen/pins.json` (currently 15 grammar modules) at `vX.Y.Z` served from a file `GOPROXY`, built by an empty-cache consumer with `CGO_ENABLED=0`, plus `GOOS=linux GOARCH=arm64 go build`. | no (local) |
| 2 | Tag the root `vX.Y.Z` on the release commit and push the tag. | **yes** |
| 3 | Verify the root: `GOPROXY=direct go list -m github.com/wotjr1649/go-treesitter@vX.Y.Z` from an empty module; then the same through `GOPROXY=https://proxy.golang.org`. | no (read-only) |
| 4 | Regenerate the grammar modules in release form: `ROOT_VERSION=vX.Y.Z bash tools/gen/regen.sh <work-dir>` (`-root-version`). Only `go.mod` and the generated `go.sum` (the root's two sum lines, verified through the checksum database) change; `engine_gen.*` must stay byte-identical. Raise the root `require` of the in-repository tool modules (`tools/conformance`, `tools/bench`) to the same version. Commit through a pull request. No `replace` may remain in a grammar module; `tools/gen/regen.sh` derives the release version from the committed `go.mod` files. | no (local commit) |
| 5 | Tag each grammar `grammars/<dir>/vX.Y.Z` on that commit and push the tags. | **yes** |
| 6 | Verify every grammar with `GOPROXY=direct`, then through `proxy.golang.org`: an empty consumer with no `replace` runs `go get` for the root and two grammars, builds with `CGO_ENABLED=0`, runs, and also builds for `linux/arm64`. | no (read-only) |
| 7 | Create the release notes/GitHub release, if wanted. | **yes** |

Notes:

- The root tag (step 2) points at a commit whose grammar `go.mod` files still use the development form; that is
  fine because the root ZIP excludes nested modules. Grammar tags point at the later step-4 commit. Do not move
  the root tag to it.
- The root ZIP is built by `golang.org/x/mod/zip` from the tagged tree: it contains the root package, `internal/engine`,
  `testdata/`, `LICENSE`/`LICENSES/`, `docs/`, `.github/`, `AGENTS.md` and `tools/lane` (no `go.mod` there). It omits
  every nested module: `grammars/*`, `tools/gen`, `tools/bench`, `tools/conformance`, `tools/modulezip`, `artifacts`.
  Local-only paths (`docs/plans`, `docs/prompts`, `artifacts/handoff`, `.scratch`) must not be tracked, because a
  tracked file is published by the tag; check `git ls-files` before step 2.
- Between releases `main` keeps the grammar modules in the development form (`require` root `v0.0.0` with
  `replace ../..`, no `go.sum`), so they build against the working root, including engine ABI changes the released
  root cannot load. The release form exists only in the release commit of step 4; the next change after the grammar
  tags returns to the development form.
- **Rollback.** A pushed tag and any version a proxy has served are never deleted, moved or re-tagged. A bad
  version is retracted: add `retract vX.Y.Z // reason` to the `go.mod` of a newer version of the same module and tag
  that (`vX.Y.(Z+1)`). A bad grammar is retracted in that grammar's module only; the root is unaffected. A bad
  release before step 2 is fixed by a new commit, nothing to roll back.
- A grammar `require`s one exact root version. If the root needs a fix, release root `vX.Y.(Z+1)`, then regenerate
  every grammar `go.mod` against it (steps 4-6); a grammar is not re-tagged at the old number.
