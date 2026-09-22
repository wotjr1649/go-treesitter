# Phase 3 review and checks

Scope: request limits, cancellation/error receipts, bounded snapshots, tree
ownership and independent workers. Runtime dependency, grammar blobs, C epoch,
known-regression signatures and seven Release-Critical routes are unchanged.

Independent review found missing ErrorType on local early failures and a uint32
conversion before oversized-input rejection. Both were fixed: one shared error
return helper preserves upstream types or records the local type, and the own
ExpectedEOFByte field is uint64. Context errors are returned directly. Negative
options and local-limit tests now require a nonempty error type. No 4 GiB input
was allocated; overflow prevention was reviewed from the widened type and cast.

The reviewer also clarified that Request.Timeout bounds only the parse API.
The concurrency test's ten-second timeout is not a total worker deadline;
go test -timeout supplies the campaign's outer bound. Grammar initialization
remains cold, and snapshot cancellation is governed by the request context.

After these fixes focused-04 and regression-03 passed with CGO_ENABLED=0 on
Windows/amd64. regression-03 includes all existing 50 C records and exact known
signatures. Earlier bounded fuzz and separate race checks are retained in this
phase; they preceded the final error-type/diagnostic-width edits. Those edits
do not change the traversal or introduce shared state, so no additional fuzz
or race campaign was run solely for them. See diagnosis.md for failed attempts.

Self-review against the repository checklist: scope and own-type boundary
preserved; all nonaccepted paths release raw trees; partial snapshots are not
returned; raw runtime reasons remain intact; source ownership and invalid edits
are exercised; no process-global pool/counter manipulation was added. All six
limit fields have positive and negative tests. Windows product build/test
coverage remains CGO-free. Existing known failures still match their records.
No external fixture or license changed. No unresolved finding in this diff.

Evidence: E3 retained request/lifetime tests. Memory receipts are thresholds,
not an OS process cap. Independent trees were tested; shared-tree concurrency
remains unsupported. No platform result is transferred to ARM64 or Linux.
