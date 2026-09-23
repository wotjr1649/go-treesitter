# Publication boundary

The public candidate contains the API and implementation, maintained runtime
patches, grammar artifacts, source/patch identities, licenses, retained tests
and fixtures, reproducible tools, CI, technical contracts and selected evidence.
Keep the seven-route C oracle gate and the complete 206-entry basic catalog.

Session prompts, private handoffs, raw investigation output, temporary checkouts,
build caches, profiles, dumps and personal machine/account configuration stay
outside the public candidate. Keep original failures in the local archive and
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
No publication workflow may infer those permissions from a passing test.

Accepted ADRs preserve their original reasoning. Historical references in them
may identify local records; current public contracts and selected evidence must
be sufficient for the current product and its stated claims. ADR-0016 records
the publication and commit-boundary decision.
