# Phase 2 — Product comparisons and generator integration

The retained Windows CGO-free tests compare 50 inputs against identified C
records. This phase also executes all 50 C inputs once and checks byte equality
against the checked-in set; c-receipts.json records this phase's C observations.
No result is assembled from Session 05 or Phase 1 test conclusions.

Initial strict comparison failed on the C# excerpt. Preserved in compare-initial
with the first differing nodes. Raw pinned runtime execution reproduces the Go
digest, excluding adapter translation as the source. Register KR-0002 separately:
Go missing-node recovery versus C ERROR recovery; exact deeper cause remains open.
C# Release-Critical is blocked. Nothing was changed in the product runtime.

E5 observations, limited to syntax.Node's represented fields: 39 equal ordered
snapshots; 6 bare-ampersand error-state characterizations with different recovery
trees; 4 bare-equals divergences; 1 C# recovery difference. Numeric symbols,
separate alias metadata, queries and incremental C comparisons are not assessed.
Eleven exact source/Go/C digest triples ratchet the recorded differences.

Review: no backend types cross the existing adapter boundary; real module and
all six blob hashes are checked before comparisons; inventory/file hashes detect
missing or modified fixtures; completion precedes comparison; ordering retained;
first differences logged before assertions; clean outcomes explicitly required.
Negative checks reject changed ABI/build/source/tree and incomplete receipts.
Runtime/grammar identities and Release-Critical scope are unchanged.

The existing Windows workflow now includes the ordinary retained C comparisons.
CLI regeneration resolves the current installed tool, records its executable
identity, consumes grammar JSON, and writes only build-local source copies.
The CLI is absent: actual regeneration is NOT_RUN, never simulated. Its missing
tool preflight and ABI/path policy checks are exercised. Python and Go focused
checks pass after review fixes. No remaining implementation finding in this unit.

NOT_RUN: Linux, remote CI, actual generator execution. Product release remains
blocked by the registered JSX/TSX and C# differences. No fork/product patch applied.
