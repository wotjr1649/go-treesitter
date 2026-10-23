# Release candidate evidence

This directory contains selected historical measurements and commit-bound
candidate checks. It does not assert a Release pass.
`native-e4e6a56/` also retains the fresh native CI and local packaging receipts
for `e4e6a56bc5ee0e6a5a283381e2c929dea48ae2c4`. Both Windows product jobs and
the race diagnostic passed; the two license declarations keep Release blocked.
The current verdict and limitations are in
[the status board](../../docs/reports/release-critical-status.md).
ADR-0017 (2026-10-23) later excluded both grammars; the license gate no longer
reports them.
`native-d6c802a/` retains the receipts of the first fully passing workflow
(run 39949173766): main ZIP `ab7349577b4a93a2f686c3c69072fa1f12cc4d4a35dc02ed878bf62682b7467c`
and GPL ZIP `0d5e64719f69a85f8e7c5f2004f35f04b2c765fe2052887f26bf7754f6922489`,
identical in the local, hosted AMD64 and hosted ARM64 checks.

`historical/identity.json` binds the measured product and benchmark inputs to
`23d6885ebe8709616614c89ffb6511ea815fb23f`. `historical/index.json` records hashes
and original paths for unchanged selected records. `observations.json` and
`cobol-build.json` are explicitly labelled projections; original receipts remain
in the local archive. The Go grammar reproduction receipt retained under
`artifacts/session-07/` is an immutable provenance target, not a public copy of
the session archive.

The selected results cover the 688-record C differential, the 206-entry basic
catalog, performance comparisons, finite memory/fuzz observations and packaging.
Their scope and identities still apply: they are not universal parser guarantees
or observations of a later candidate. Original failing controls and C trees
remain in `testdata/oracle/`.

Fresh candidate checks run in
[the Windows workflow](../../.github/workflows/windows.yml). Its native AMD64
and ARM64 jobs build official module ZIPs and execute empty-cache consumers.
The uploaded packaging receipt identifies the exact commit, toolchain, ZIPs,
executables and module graphs. The separate license job must also pass before
Release can pass. CI artifacts are validation evidence, not published releases.

See [publication boundaries](../../docs/specs/publication.md) for local archives
and the treatment of previously published history. Main merge, tag and release
remain on hold.
