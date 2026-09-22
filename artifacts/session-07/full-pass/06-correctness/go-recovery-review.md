# Go recovery-order fuzz finding

The seven-route edit fuzzer found a real divergence after 2,319 executions.
The minimized retained input edits `package` to `packag#i#000`. Both requests
complete with errors, but fresh Go adds an `expression_statement` inside ERROR;
incremental Go matches the pinned C tree without it. The same witness still
fails with the seven pre-C#-patch runtime files overlaid, so this is not a new
C# regression. Original failure receipts and all compared trees are retained.

The smallest passing change enables the existing C physical-version transaction
for the exact pinned Go blob. The feature's admission requires no old tree and
no reuse cursor; incremental parsing is not replaced by fresh parsing. Import
reproduction shows precisely one changed carrier file, among 1,508 files.
The maintained patch, carrier manifest, main identity and optional-module
parent identity were updated together. No grammar, source origin or C epoch
changed. This is a carrier improvement under the existing owner approval.

E5 evidence: 28 native C records, comprising four clean origins and 24 malformed
targets; 20 Go repeats and three C runs per target; all 24 Go edit/fresh/C
comparisons and 24 C edit/fresh comparisons agree. Six invalid native edit
controls are rejected. The first native-edit CLI invocation used an unsupported
flag and did not execute cases; its failed receipt remains beside the corrected
invocation. No failure was silently retried or treated as success.

E3 checks after adoption: full `go test ./... -count=1 -timeout=150s -json`,
`go vet ./...`, optional-module tests and all 206 basic catalog cases pass.
`go-recovery-adoption.json` records exact import reproduction. The Go-only
30-second fuzz run completed 149,150 executions before this patch; the first
60-second edit campaign failed and must be resumed after this work unit.
Neither receipt establishes final-candidate fuzz closure.

The retained hardening harness also exercises real in-flight cancellation on
all seven routes, snapshot cancellation, subsequent parser use and bounded
goroutine counts. A diagnostic race run passed before the Go profile change;
it is not CGO-free product evidence or final-candidate race closure.

Self-review found no remaining issue in this scoped diff: failure ownership,
UTF-8 admission before edit mutation, complete-result receipts, worker-local
trees, context cleanup, source/patch hashes and package boundaries are retained.
The new test fails on the pre-patch fresh tree. The original public-source
review raised old-root framing as a possible alternative; the actual witness
uses a clean `source_file` old root and differs inside ERROR, and the isolated
version-order experiment resolves it. No speculative root-normalization or
retry-comparator change was added. The public reviewer received no private
repository material and executed no local product tests.

This closes this particular fuzz defect. It does not claim whole-language
equivalence, final resource/performance gates, native ARM64 or release approval.
