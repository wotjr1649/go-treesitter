# Phase 1 — C producer and identified workload

Scope: formal Windows C producer under ADR-0008; baseline and oracle epoch
unchanged. All files under testdata/oracle/windows-c are C observations only.
No Go/C agreement claim is made in this phase.

Checks: source preparation; six static C builds; 50 fixture executions; a
second 50-fixture execution for deterministic-output checking; exact byte
comparison of every record and the build manifest; CRLF/control-byte input
preservation and rejection above 4 MiB; Python ABI/archive/path/hash/immutable
output tests; existing product smoke and split KR tests. Commands and exits
are in commands.jsonl. No benchmark or performance comparison was performed.

Observed: all C trees cover their full input. F1–F3 have errors in both C
grammars; F4–F7 are clean. C# excerpt has an ERROR node. Comparison with Go is
reserved for Phase 2 and will use its own execution receipts.

Review: bounded, fixed public source URLs; traversal, link, duplicate and
archive-size rejection before extraction; source file-set/hash checks before
compilation; offline compiler/driver invocations with timeouts; no shell-built
commands; iterative cursor walker with node/depth limits; cleanup on every C
return; binary stdin on Windows; immutable evidence outputs. Review caught
and repaired archive member accumulation and unrecorded extra source files.
Relevant checks rerun after that repair. No remaining finding in this unit.

NOT_RUN: Linux/container execution, remote CI, actual CLI regeneration.
The installed generator is absent; checked-in C producer versions are unknown,
explicitly null, and identified by pinned commits and source hashes.
No fork, runtime patch, global installation or remote write occurred.
