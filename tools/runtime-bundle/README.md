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

The source's MIT notice is retained verbatim in `internal/runtime/LICENSE`.
Grammar and fixture notices are covered separately in `THIRD_PARTY_NOTICES.md`.
