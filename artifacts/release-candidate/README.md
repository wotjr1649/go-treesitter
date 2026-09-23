# Release candidate evidence

These records are selected historical observations, not a new Release pass.
The current verdict and limitations are in
[the status board](../../docs/reports/release-critical-status.md).

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
