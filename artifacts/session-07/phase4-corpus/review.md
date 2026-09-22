# Expanded fresh/incremental C checks

This phase built all six pinned C grammars against the pinned C runtime with
GCC 16.2.0 on Windows/amd64, then executed 35 separately cataloged inputs.
The build identity is
a1efb8b41812e3326bc5d3a41082063f1fac2f0d113d3ba2a23160f91555bce7.
testdata/oracle/windows-c-extended retains its complete build manifest, set
inventory and each ordered tree. No generator ran; mode is checked-in.

E5 for these observations: all 35 product fresh trees and all 21 incremental
results matched the new C snapshots in the currently enforced dimensions:
ordered node types, edge fields, flags, byte/point ranges and root/error state.
Completeness and producer/input identities were checked first. Inputs cover
each of Go, Python, JS, JSX, TS, TSX and C#: UTF-8/CRLF, identifier replacement
with a different byte length, multi-byte comment-row insertion, CRLF deletion,
and 513 sibling comments. The TS source also includes typeof a.b.

Incremental API use does not imply subtree reuse: the receipts explicitly show
fresh fallback for C# external-scanner cases and include raw reuse/fallback
information. C# small clean cases do not resolve the separate KR-0002 recovery
blocker. These are bounded authored cases, not a claim over arbitrary source,
query execution, backward cursors, other languages or other platforms.

The Go reader now shares the independent pinned runtime-ABI anchor used by the
Python comparator. Retained negative tests reject coherent ABI-range/header
changes. Existing 50-record wrappers/ratchets remain unchanged. focused-02 and
regression-01 passed on Windows/amd64 with CGO_ENABLED=0, including every old
signature. No new exception was added to accept a difference.

Independent review found no defect in identities, edit comparisons, lifetime
or preservation of the original ratchets. Self-review confirmed source hashes,
LF catalog encoding with intentional CRLF payloads, module/grammar bindings,
failure propagation and exactly-once release of each parse result. No baseline,
oracle epoch, Release-Critical scope or gate definition changed. Linux and
native Windows/ARM64 execution remain separate, unrun environments.
