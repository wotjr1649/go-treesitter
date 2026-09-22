# Packaging and native product audit

The owner selected MIT for this library's own code. The root LICENSE carries
that notice; seven copied dependency/grammar/oracle notices retain their exact
original bytes and commit/hash identities. The existing Newtonsoft fixture
notice is separately checked against its manifest. The initial audit script
used the wrong JSON key (`upstream` instead of `baseline`); the failed receipt
is preserved, the script was corrected and the full audit passed on run 02.
No license text or dependency identity was changed to satisfy a check.

The notice inventory covers the declared six grammars/seven routes. Upstream's
larger default aggregate catalog has not received a complete transitive grammar
license inventory. A release of that broader bundle needs that work, or an
explicitly selected assessed bundle. This note is not a broader license claim.

The .gitignore additions exclude profiles, memory dumps, traces, local config
and private artifacts. Sixteen representative paths are ignored. Requested
docs/prompts, docs/specs, docs/plans and handoff paths are absent from both the
index and reachable Git history. Existing local files are preserved. No history
rewrite, remote addition, push, publication or tag occurred in this unit.

The CI definition adds selected-grammar tests, Python oracle boundary tests,
explicit Go test timeouts and an ordinary Windows ARM64 consumer link. It keeps
read-only contents permission and checkout credentials disabled. Its exact
local test commands passed; hosted workflow execution remains NOT_RUN.

E3, Windows/amd64, Go 1.27.1, CGO=0: ordinary consumer build/run, full product
regression, selected-grammar regression, go vet and module integrity passed.
The five retained Python tests passed. Both product builds used the unchanged
85-record catalogs present at the time. Subsequent fixtures are not part of
this phase's claim. The consumer exercises construction, fresh/edit/error
results, cancellation, lifetime and position/type lookup.

The 140-package product import graph has zero CgoFiles and no runtime/cgo.
Both ordinary Windows AMD64 and ARM64 executables record CGO_ENABLED=0 and the
pinned runtime in build information; PE imports are kernel32.dll only. A C
compiler or tree-sitter DLL is not required for these product paths. The
separate native C oracle and race diagnostics still use C tooling.

CGO-free ordinary consumer linking also passed for Windows ARM64, Linux AMD64,
Darwin ARM64 and wasip1/wasm. Binary hashes/formats are in receipt.json. Only
Windows AMD64 was executed. Cross-linking does not replace native platform
tests; ARM64 execution and remote CI are NOT_RUN. No release claim is made.

Self-review: own license choice matches the user's answer; copied notices and
link destinations match their manifests; no product/runtime source change;
no new dependency, credential handling, lifecycle or concurrency behavior.
Provenance hashes are unchanged and both reference checkouts are clean at their
original heads. No second opinion was required for this packaging-only diff.
