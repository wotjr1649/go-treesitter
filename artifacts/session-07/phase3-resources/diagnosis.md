# Request-limit observations, Session 07 Phase 3

Environment: Windows/amd64, Go 1.27.1. Product checks use CGO_ENABLED=0;
race checks use CGO_ENABLED=1 and GCC 16.2.0 in a separate diagnostic lane.
Exact commands and dependency identities are retained in commands.jsonl.

The initial focused checks passed. regression-01 failed because a one-byte
memory budget still returned accepted_clean after other tests had warmed the
upstream pools. The receipt reported arena=7,664,904 and scratch=7,506,096 bytes.
The original failure is retained. Source inspection found that arena.setBudget
and parserScratch.setBudget charge allocation growth beyond existing slabs;
parser initialization also occurs before this baseline. This is the pinned
runtime's budget semantics, not a hard footprint cap.

The adapter now rejects receipts above the requested arena+scratch budget
before building a snapshot, including an upstream accepted receipt. It retains
the raw stop and reports LimitReason=runtime_memory. No pool is drained and no
global accounting is read. This admission check does not prevent an allocation
spike and does not include source copies, grammar caches, snapshots or other
workers. TestMemoryReceiptAfterWarmup observed three upstream accepted receipts
with arena=2,938,912 and scratch=3,866,656, all rejected by the adapter.

focused-02, regression-02 and vet-01 passed after this change. The second fixed
20-second fuzz run passed 7,685 executions with two workers. This finite Go-only
fuzz run does not establish safety for arbitrary inputs or other grammars.

race-02, run concurrently with fuzz, hit the newly written concurrency test's
two-second request watchdog for cold C# work. It reported timeouts, not a data
race. The test checks independent-worker correctness, not latency; its watchdog
was changed to ten seconds while retaining cold initialization and all 112
requests over seven routes. race-03 then ran alone and passed; focused-03 passed
the same worker test in the product lane. race-02 remains recorded and is not a
product-gate result. Product timeouts have a separate retained stop test.

Evidence level: E3 for retained product tests. The existing 50-record C checks
ran within regression-02; no claim here transfers results from another phase.
