# Phase 4 review and regression evidence

Review scope: the entire branch against its bootstrap root, plus the closing
diff. Checklist: `docs/reviews/review-checklist.md`. Source and test diffs were
read; historical handoff bodies were not used as evidence or instructions.

Two review findings were addressed before the closing regression:

1. Release the native C parser when language assignment fails.
2. Explicitly reject missing C nodes in the fixtures expected to be clean.

Final review has no unresolved implementation findings. Scope, closed outcomes,
full diagnostic observation before classification, error-type-only propagation,
non-clean handle release, owned snapshots, validated edit boundaries, sole
upstream import point, worker-local ownership, and sequential diagnostic counter
reads were checked. The native driver's input set is seven fixed literals.
Generator invocation uses argument arrays, stays inside the repository, and
records a fresh producer/source identity rather than accepting a fixed release.

Phase-local regression: Go 1.27.1, Windows/amd64, CGO_ENABLED=0.
`go build ./...`, `go vet ./...`, `go test ./... -count=1`, and `go mod verify`
passed. Windows/arm64 build passed separately. Python ABI/producer tests passed.
The revised native driver was rebuilt and its TSX differential test passed;
the Phase 4 source/executable identities are in `native-manifest.json`.
These receipts are this phase's regression evidence, not a blend of earlier runs.

The source/document whitespace check passed. The unrestricted branch whitespace
check reports two intentionally retained data details: a final blank compiler
output line in `phase2/wu9-compiler.log`, and a trailing space on line 136 of the
hash-pinned Newtonsoft excerpt. Changing either would rewrite recorded output
or registered fixture bytes. Both are explained; no source/style rule or test
was weakened. The failed setup observations in Phases 0, 2 and 3 remain retained.

Privacy checks: no ignored path is tracked, no reachable commit includes the
four private document directories, and no remote is configured. The user's
authorized root rewrite retained local historical files. Scratch, keys,
environment files, IDE state and Python caches are ignored. All staged changes
belong to this session. Runtime version, grammar commits and C runtime epoch
remain fixed; generator policy changed only by the user's explicit amendment.

NOT_RUN: remote CI, race, ARM64 execution, other OS execution, full Linux oracle
lane, JavaScript/C# C comparisons, proposed runtime patch, actual generator
invocation (no installed CLI), actual timeout/early-stop/resource-limit/
invariant-violation and mid-parse cancellation paths. Resource budgets and
statistical benchmarks are outside this session. Tests for deterministic
supported outcomes and the observed missing-node receipt are retained.

Development/session checks are satisfied with the recorded upstream blockers.
Release-Critical JSX/TSX remain red; no release is approved. The bootstrap merge
exception applies because main has only this session's root commit. Merge only
after this reviewed closing unit is committed and the working tree is clean.
