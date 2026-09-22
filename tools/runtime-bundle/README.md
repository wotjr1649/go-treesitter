# Internal runtime provenance

The owner-approved carrier is described in
[ADR-0013](../../docs/design/decisions/ADR-0013-bundle-the-approved-runtime.md).
Its original source is the exact archive identified by `source.json`.
It does not depend on `_ref`, a remote fork, a consumer replace, or C code.

Reproduce it in a **new repository-local directory** using a previously acquired
module archive; this command does not download or run dependency code:

```powershell
python tools/runtime-bundle/import.py --archive path/to/v0.53.0.zip --output .scratch/runtime-check --manifest .scratch/runtime-check.json
```

Compare that manifest and directory to `internal/provenance/runtime.json` and
`internal/runtime`. The importer checks the archive identity before creating
output, relocates imports, then checks and applies the inventoried patches.
Existing output is rejected. The bundled file and patch hashes are checked by
`go test ./internal/provenance ./internal/gtsadapter ./internal/boundary`.

`KR-0001b-jsx-text.patch` removes the two non-C equals guards. Its retained
tests cover JSX/TSX fresh C trees and edits; the bare ampersand controls must stay
erroneous. Original known-difference records remain in `testdata/oracle`, while
`active-differences.json` lists only differences present in the current carrier.

`catalog-c-family-lexer-reuse.patch` preserves the C/C++ lexer's preprocessor
state and queued string tokens when a reused span ends at a boundary already
scanned by that lexer. `TestCFamilyLexerStateAcrossReuse` covers includes,
macros, conditionals, escaped strings and ordinary code through the adapter.

`KR-0002-csharp-recovery-transaction.patch` corrects recovery scheduling,
hidden missing-node costs, shared-prefix ambiguity retention and acceptance
ordering. It enables the pinned C# grammar's packed version transaction and
preserves the resulting complete recovery tree instead of widening its retry
policy. `KR-0004-csharp-preserve-recovery-errors.patch` prevents source-based
reconstruction from replacing an error-bearing C# tree.

Run `python tools/runtime-bundle/check.py` with `CGO_ENABLED=0` for the private
merge and acceptance invariants. Its temporary Go test overlay leaves the
carrier's file inventory unchanged. Public adapter tests compare all ordered
nodes against the pinned C records and check fresh/edit equality.

The source's MIT notice is retained verbatim in `internal/runtime/LICENSE`.
Grammar and fixture notices are covered separately in `THIRD_PARTY_NOTICES.md`.

`separate.py` mechanically removes the three GPL grammars from the main carrier
and generates their optional adapter with `--optional-output NEW_DIRECTORY`.
It separates 19 files plus aggregate registry, loader and embed declarations;
the main carrier has 1,508 files. The same pinned blobs, scanners and queries
are preserved in the nested `grammars/gpl` module. The main manifest binds the
separator and excluded origin hashes. `catalog-optional-blob-providers.patch`
lets an explicitly registered blob use the existing cache and scanner binding
when an aggregate catalog is also present. This changes no parser tables.

`go-recovery-version-order.patch` enables the existing C version transaction
for the pinned Go blob. An edit-fuzz witness previously produced a fresh ERROR
subtree with an extra `expression_statement`; incremental and C agreed without
it. `TestGoRecoveryOrderOracleRecords` retains 28 C records and 24 edits, with
the original minimized fuzz input kept in the adapter's fuzz corpus. This
original patch applied to fresh parsing; subsequent recovery patches also
repair incremental lookahead admission.

`recovery-lookahead-and-eof.patch` carries the Go C-table artifact and the
shared EOF, hidden-error-cost, lookahead and edit-dependency corrections.
`recovery-variable-token-width.patch` allows competing JS-family versions to
consume their own internal token widths. The dispatch loop reconciles their
byte positions. Stateful external tokens still require identical spans.
`recovery-cost-memo.patch` caches raw hidden-node costs in the existing bounded
node/version memo, preserving invalidation and avoiding repeated subtree walks.
`recovery-raw-child-memo.patch` also reuses a cached child aggregate during raw
walks, only when its arena, captured shape and node version still match. Older
captured shapes continue to be walked and retain their hidden missing costs.
`incremental-leaf-conflict.patch` leaves conflicting action cells to normal
dispatch. Reusing one shift from such a cell discarded pending reductions and
caused repeated stack copying on the retained 8,192-declaration Go witness.
ADR-0015 explains the origin/product distinction; `grammars/go.json` and its
MIT source ZIP bind the independently reproduced conversion. The 458-record
runtime-hardening corpus includes 295 edits, and the original fuzz failures
remain retained. No active C-tree exception remains.
