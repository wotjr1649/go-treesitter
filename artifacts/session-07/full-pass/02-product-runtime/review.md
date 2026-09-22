# Approved internal runtime and KR-0001b product delivery

The carrier preserves the v0.53.0 origin and all 206 grammar blobs. Its 1,527
files are mechanically selected and import-relocated from the identified module
archive. The only behavior change is the six-line JSX/TSX text scanner patch.
`internal/provenance/runtime.json` binds every source/result byte and the importer.
ADR-0013 records the owner's approval; no remote fork, push or publication occurred.

Observed Windows amd64, Go 1.27.1, CGO_ENABLED=0 checks:

| Check | Result and evidence |
|---|---|
| Full product `go test ./... -count=1 -timeout=180s -json` | PASS, `../bundled-final-product.json` |
| Seven-route selected build | PASS, `../bundled-subset.json` |
| `go vet ./...` | PASS, `../bundled-vet.json` |
| Native oracle Python tooling | 6 tests PASS, `../bundled-tooling.json` |
| Runtime tamper checks | Modified, additional and missing files rejected, `../bundled-tamper.json` |
| Archive reproduction | All file bytes and manifest identical; wrong archive and existing output rejected, `reproduction.json` |
| Versioned external consumer | Empty module cache, local file proxy, v0.0.1, no replace; public fresh/edit/error/cancel/lifetime behavior and scanner fix PASS, `../bundled-consumer-fixed.json` |
| Retained JSX/TSX C comparison | Fresh and edits pass (E5 for those inputs), `../bundled-oracle-fixed.json` |
| Fresh 94-input campaign | All observations match the isolated scanner candidate; 86 exact C trees, 6 characterized bare-ampersand shapes, 2 unresolved C# differences; 21 edits, ordinary repeats 20 and C# regression repeats 100, `campaign.json` |

The initial product and focused failures remain recorded. They exposed that the
C input comparison included the newly added Go-only carrier identity, and that
the consumer test mixed Go download diagnostics into stdout. The fixes compare
all actual C inputs, verify the Go carrier independently, and preserve exact
consumer stdout while capturing stderr separately. No C comparison was loosened.

Review covered the entire owned diff, public/import boundaries, provenance,
error/lifetime behavior, local ZIP delivery, Windows paths, and CGO-free builds.
No unresolved defect was found in this work unit. Source selection and the
six-line behavior delta were also independently checked by archive reproduction
and the previous isolated candidate campaign. A delegated private-diff review
was not used because its information isolation could not be established.

The staged runtime bytes were checked against all manifest entries. `git diff
--cached --check` reports one original generated-file EOF blank and twelve
unified-patch context lines (the required leading space before Go tabs).
`index-review.json` records and classifies those exact warnings; the origin
bytes and valid patch format are preserved, and no whitespace rule was disabled.

Remaining work is explicit: KR-0002/KR-0004, expanded catalog failures, full
notice/distribution treatment, integrated resource/performance/determinism gates,
native Windows ARM64 and hosted CI receipts. The owner chose all 206 basic
checks and seven strict C-oracle routes, with an MIT-centered distribution.
The current unpublished carrier still includes the original GPL grammar inputs;
their replacement or separate distribution remains a release blocker.

`command-output.zip` preserves raw command logs and the full campaign node
streams without Git newline conversion. `output-manifest.json` binds each byte.
Earlier failed receipts are retained. This commit retires KR-0001b only; it is
not a FULL_PASS or a release approval.
