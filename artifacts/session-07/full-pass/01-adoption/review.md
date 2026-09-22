# Candidate C adoption — scoped closure

The owner explicitly approved the complete Candidate C after the fresh
comparison. The starting project commit was
9e5c90ad4aaf0bcea59d229e6b6f0ab789893a76 on session/07-product-hardening,
with a clean tracked working tree. No product runtime source, dependency
version, Go grammar blob, C runtime commit or grammar base commit changed.

The complete upstream TypeScript patch (SHA-256
8d091351f8107d1546a8130d281f0ca8d6d959b11da66b7872d3e10f998340e9,
source commit 24e1d3b2b4409ee2ffa63e88abba51a40060e76e) applied without a
conflict. Import types, variance annotations, adjacent generic call signatures
and contextual `in` properties were all exercised. The JavaScript grammar
required for generation came from TypeScript's locked 0.23.1 package with its
SHA-512 checked. Package install scripts did not run. The first acquisition
also retained the product's separate JavaScript 0.25.0 grammar as a read-only
input; it was not substituted for this locked generation dependency.

## Observed checks

- 26 TypeScript/TSX feature and control inputs: complete ordered Go/C trees
  equal, including flags, edge fields, byte/point ranges and child order (E5).
- All four compared runtimes produced stable observations for 20 fresh parses
  of each input. The unpatched regenerated C control equaled original C on
  every input, separating generator changes from the semantic patch.
- 26 Go edits equaled fresh trees; 26 edits in each of three C variants did
  likewise (78 C edits). Each C executable also rejected six invalid numeric,
  range, content or UTF-8-boundary requests. The new driver retains bounded
  input and process timeouts and releases old/new C trees on all paths.
- Fresh original-catalog experiment: 94 inputs, 21 Go edit comparisons. Only
  four contextual C differences changed under Candidate C. The scanner
  candidate remained a separate variable; its four corrections are not
  product changes from this commit. C# KR-0002 and KR-0004 remain open.
- All 94 active C receipts were produced anew under `windows-c-v2/` with the
  adopted identity. The eight contextual inputs now require exact clean
  agreement, including the four retired KR-0003 differences. Original C sets
  and old signature files are retained, byte-for-byte.
- CGO=0 full product regression passed (`phase1-product-regression`). The
  final focused oracle/feature tests and all six Python tooling tests passed
  after review changes (`phase1-final-focused`, `phase1-final-tooling`). Eight
  retained feature subtests include 20 fresh repetitions and one edit each.

Command arrays, exit codes, source identities and logs are in this directory,
`../01-candidate-c/`, and the named receipts in the parent directory. Generation
used the resolved tree-sitter 0.27.0 CLI at this execution, ABI 15 accepted by
the unchanged runtime. This is a producer receipt, not a generator version pin.

## Review and limitations

Reviewed source, all new branches, source/patch/manifest binding, semantic
comparison, invalid-edit short circuit order, C ownership, task paths and
license mapping. No test expectation was changed to accept a different Go
tree. Removed only the retired KR-0003 exception requirement; exact tree and
clean-outcome assertions now enforce these cases. The product lane still
needs no C compiler. No unresolved finding remains in this scoped change.

Public upstream research ran independently with public-only input. Repository
diff review and integration were performed locally; no independent private
source delegate review is claimed. This is not a release receipt: KR-0001b,
KR-0002, KR-0004, resource/performance closure, native ARM64, clean distribution
and hosted CI remain separate units. No remote was configured or written.

Preserved failed attempts: PowerShell split an unquoted `-modfile=...` argument
in the scanner experiment; quoting fixed the invocation. A campaign reader
used Windows cp949; explicit UTF-8 fixed it before any comparison. The offline
full module-graph read lacked transitive metadata; a separate task-only modfile
resolved public metadata, leaving go.mod/go.sum unchanged. These are not product
correctness failures and none was hidden or retried without a diagnosis.

Raw Windows command output includes CRLF. To preserve the receipt hashes across
Git checkouts, `command-output.zip` retains those exact bytes and the two large
scanner observation streams. `output-manifest.json` binds every entry and the
archive. The original local files remain untouched and ignored; no failed
attempt was discarded. Source and other text artifacts pass ordinary staged
whitespace checking without changing Git's whitespace rules.
