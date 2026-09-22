# Phase 0 evidence

Windows/amd64, Go 1.27.1, CGO_ENABLED=0. Commands and exit codes are in
`commands.jsonl`; nonempty command output is retained in the adjacent logs.
The module/version/tag/commit, six grammar identities and oracle tuple are
bound by `internal/provenance/identities.json`.

- WU0: module syntax, LF attributes and private-path exclusions checked (E1).
  The user additionally excluded local handoffs, configuration, keys and IDE state.
  A user-authorized rewrite removed historical handoffs from reachable history.
  The original historical whitespace finding was outside the new implementation;
  those local files were preserved without editing or reviewing their contents.
- WU1: retained build-info probe accepts v0.53.0 (E3). A separate `-modfile`
  selected v0.52.0 and the same test failed with the expected identity mismatch.
  The main go.mod was never switched. The subsequent v0.53.0 check passed.
- The first negative-check setup used incorrectly transmitted PowerShell flags;
  `go mod edit` rejected the setup and the check still saw v0.53.0. Its output is
  preserved as `wu1-negative.log`. Explicit argument arrays corrected the setup;
  `wu1-negative-fixed.log` contains the actual mismatch demonstration.
- Review: no remaining implementation findings. Scope, identities, fail-closed
  replacement rejection, LF and explicit staging inspected. Parser/lifetime and
  regression dimensions do not apply yet. No contract, epoch or scope changed.

NOT_RUN: parser tests (later phase), remote CI, race diagnostics, Linux oracle.
This phase makes no syntax or performance claim.
