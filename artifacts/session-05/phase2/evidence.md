# Phase 2 evidence

Product lane: Go 1.27.1, Windows/amd64, CGO_ENABLED=0. Phase-local build-info
check passes; the tuple is `internal/provenance/identities.json`.

- WU7: all 14 F1–F7 × TSX/JavaScript receipts match the register, including
  ordered-walker node counts (E3). Ampersand remains erroneous; controls clean.
- WU8: minimum reproducer, unsent issue draft and a six-line deletion proposal
  retained. The proposal was parsed as a diff, never applied. Its source hashes
  and the product reproduction are local to this phase. Root-cause level: E1.
- WU9: NOT_RUN at the first structural obstacle in identity preflight.
  `gcc.exe (Rev3, Built by MSYS2 project) 16.2.0` is present, but the pinned
  [TSX parser source](https://raw.githubusercontent.com/tree-sitter/tree-sitter-typescript/75b3874edb2dc714fb1fd77a32013d0f8699989f/tsx/src/parser.c)
  declares ABI 14. The contracted generator tuple specifies ABI 15 preferred,
  13 fallback. Its pinned package.json declares `tree-sitter-cli ^0.24.4`, not
  the identity of the generator that produced parser.c. No tree-sitter CLI is
  installed on PATH. Both downloads had 40/20-second bounds, respectively.
  Exact SHA-256 values are in `wu9-identity-preflight.log`.
  Runtime/grammar compilation and the C comparison were not attempted. No
  substitute epoch, regenerated source, tool installation or weaker comparison.

KR-0001b therefore remains E1 for its Go-only cause; the failing Go receipt is
retained at E3. These are different claims. No E5 result was produced, and no
fork-trigger completion claim is made. The open issue page alone does not prove
absence of a tagged fix; recheck that condition when escalation is considered.

Review: no remaining findings. Static signatures, two distinct failure policies,
controls, source identities and the unapplied proposal inspected. The conditional
probe is explicitly deferred on an identity obstacle, not a failed product gate.

NOT_RUN: WU9 C execution (identity obstacle), patch execution (out of scope),
Linux oracle, race, remote CI, upstream publication. Release-Critical JSX/TSX
remain blocked by KR-0001b; KR-0001a stays a grammar usability limitation.
