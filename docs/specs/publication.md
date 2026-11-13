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

Accepted ADRs preserve their original reasoning. Historical references in them
may identify local records; current public contracts and selected evidence must
be sufficient for the current product and its stated claims. ADR-0016 records
the publication and commit-boundary decision.
