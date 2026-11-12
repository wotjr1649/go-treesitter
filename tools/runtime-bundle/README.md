# Internal runtime provenance

The product builds the identified source carrier under internal/runtime; it
does not use a consumer replace or a runtime dependency on a mutable checkout.
ADR-0013 defines the carrier boundary. ADR-0018 records the approved candidate
baseline transition; final runtime adoption remains subject to its completion
gate.

## Authoritative identities

tools/runtime-bundle/source.json identifies the upstream module version, tag,
commit, source archive, imported packages/assets and patch inputs.
internal/provenance/runtime.json binds the importer, original/resulting file
hashes and patch inventory. internal/provenance/identities.json binds the
baseline candidate and resulting runtime manifest. Treat those files as
authoritative; do not copy volatile file or patch counts into this guide.

## Reproduce the carrier

Use a previously acquired archive whose SHA-256 matches source.json. Import
into a new repository-local output directory; existing output is rejected:

```powershell
python tools/runtime-bundle/import.py --archive path/to/pinned-upstream.zip --output .scratch/runtime-check --manifest .scratch/runtime-check.json
```

Compare the resulting manifest and directory with
internal/provenance/runtime.json and internal/runtime. The importer checks
archive identity before creating output, relocates imports, then verifies and
applies the inventoried patches.

Pass `--optional-output NEW_DIRECTORY` to the importer to write the optional
GPL adapter produced by `separate.py`. The source and resulting files are identified by the
central source and runtime manifests; ADR-0014 and ADR-0017 define the
distribution boundary.

Run python tools/runtime-bundle/check.py with CGO_ENABLED=0 as part of the
CGO-free product lane. Its temporary Go test overlay checks private merge and
acceptance invariants without changing the carrier file inventory.

catalog-c-family-lexer-reuse.patch preserves C/C++ preprocessor state and
queued string tokens when a reused span ends at a boundary already scanned by
that lexer. TestCFamilyLexerStateAcrossReuse covers includes, macros,
conditionals, escaped strings and ordinary code through the adapter.

incremental-leaf-conflict.patch leaves conflicting action cells to normal
dispatch. Reusing one shift from such a cell discarded pending reductions and
caused repeated stack copying. The patch keeps conflicting cells on normal
dispatch; `TestLargeGoIncrementalConflict` retains the regression witness.

c-oracle-recovery-summary-all-links.patch records the error-recovery stack
summary over every link of a merged version, as C's stack__iter does. A
reduction merged into an earlier version survives only as a packed link, and
the link-0 walk hid it from the strategy-1 election (KR-0005).
`TestKR0005CSharpLambdaRecovery` retains the regression witness. The private
`testdata/recovery_summary_links_test.go`, run by check.py, compares the walk
with a literal port of C's iterator loop and checks that the election sees
every recorded depth.

c-oracle-packed-version-order-no-promotion.patch stops promoting a primary
stack to slot 0 between iterations for grammars with packed GSS version order.
C keeps its stack-version order, and a later merge keeps the earlier version's
link; the promotion changed which C# and TypeScript generic-call reading survived (KR-0006).
`TestCSharpGenericCallOracleRecords` and `TestTypeScriptGenericCallOracleRecords`
retain the regression witnesses.

c-oracle-packed-version-order-opt-out-promotion.patch exempts grammars that opt
out of the default C recovery port (cRecoveryDefaultOptOut; C++ among the
packed-order grammars) from that change. By default the port's condense swap
does not run for them, and on the traced C++ inputs the promotion orders versions as C's
condense does; removing it chose the template reading over C's comparison
(KR-0007). `TestCppPackedVersionOrderControls` retains the witnesses.

The upstream source MIT notice is retained verbatim in internal/runtime/LICENSE.
Grammar and fixture notices are covered separately in THIRD_PARTY_NOTICES.md.
The Go artifact keeps separate origin and C-derived product identities under
ADR-0015. Current candidate results and unrun gates are in
docs/reports/release-critical-status.md. Historical results remain bound to
their original source, grammar, manifest, fixture and lane identities.

## When to replay the importer

A full replay writes a complete carrier (about 26 MiB, including grammar blobs) into a new directory.
Routine cause experiments use `check.py` overlays or focused tests instead. Replay the importer only at
patch-registration and integration-candidate boundaries, into a new `.scratch/<task>/<run>` directory, and
keep the comparison receipt: file list, hashes, differences, and exit status.

A replay tree whose result matches may be deleted after the session, under `docs/specs/validation.md`
§ Scratch retention. A tree whose result does not match stays until its difference is explained and
recorded.

Do not change `import.py` to save space: `internal/provenance/runtime.json` binds its hash. Some patches
modify grammar blobs (`GIT binary patch`), so the blobs must stay in the replay.
