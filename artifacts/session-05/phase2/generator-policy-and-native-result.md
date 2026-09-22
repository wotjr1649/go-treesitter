# Phase 2 addendum after the user's generator-policy change

This supplements, and does not erase, the earlier NOT_RUN observation. The user
explicitly requested dynamic generator selection after the first identity check.
ADR-0007 and the local oracle specification record the revised policy.

`identity.py` resolves the installed generator at generation time, accepts its
actual version, and validates generated ABI against the pinned runtime header.
No release number is embedded in configuration. Checked-in C uses source hashes
and a null historic generator version. Its unchanged ABI 14 is compatible with
the pinned runtime's actual 13–15 range. Boundary tests reject 12 and 16 and
accept 13/14/15; version metadata is not restricted to a release allow-list.

Native build: MSYS2 gcc 16.2.0, Windows/amd64, `-std=c11 -fPIC -O2`, static driver.
The runtime archive came from a read-only Git object at the pinned runtime
commit; TypeScript parser/scanner/header came from the pinned public commit.
Compiler, source/archive and executable hashes are in `native-manifest.json`.
Direct C transport is specific to this diagnostic probe, not the full oracle lane.

| TSX input | C HasError | Go HasError | Claim scope |
|---|---|---|---|
| F1–F3 | true | true | Error-state characterization only; recovered trees differ |
| F4–F5 | false | true | KR-0001b differential, E5 |
| F6–F7 | false | false | Ordered snapshot digests agree for these two controls, E5 |

The diagnostic retains both digests, completeness, source hashes, node counts
and first differences. This is solely Phase 2 evidence. It does not incorporate
Phase 1 smoke results or Phase 3 timings. No patch was applied or tested.

Fork-trigger evaluation: the baseline reproduction and CGO-free product relevance
are retained here. A public `git ls-remote --tags --refs` query on 2026-09-22
returned latest version tags v0.51.0, v0.52.0 and v0.53.0; the last tag object was
`f102332cca9dc45e841b9439d34ba5cc59f0b10f`. The public issue is open, unassigned,
without a maintainer reply or linked pull request. All three recorded trigger
conditions are met for the TSX finding at this observation time. A fork still
requires the user's separate decision; this session creates none.

Review: outcome signatures unchanged, unsupported ABI rejected, shell-free
generator invocation, generation confined to the repository, source identities
retained, raw strings preserved. The C driver accepts only seven fixed literals.
Its malformed-input recovery differences are recorded, not asserted equivalent.

NOT_RUN: actual generator invocation (CLI absent; checked-in C was used),
JavaScript C differential, proposed patch execution, full Linux oracle lane,
race and remote CI. Upgrading a future generator still requires new artifact
receipts and comparisons; evidence never moves to a new identity automatically.
