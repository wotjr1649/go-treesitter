# Closing review and evidence

Scope completed: Windows C oracle producer and product comparisons; actual
dynamic generator integration; isolated scanner candidate evaluation and a
bounded adoption recommendation. Product implementation and pins are unchanged.

This phase's product build, vet, full tests, module-integrity check and
Windows/arm64 cross-build pass with CGO disabled. Four Python tooling tests pass.
Commands, environment, main identity hashes and exits are in commands.jsonl.
These are closing regression receipts, not substitutes for the other phases'
C or candidate observations.

Final diff review against the repository checklist:

- Scope: adapter implementation, syntax API, go.mod/go.sum and identities.json
  unchanged. New Go behavior is retained tests and explicit experimental tests.
- Correctness: inventory/build/source/blob binding precedes comparisons; empty,
  incomplete or changed records fail; child order retained; first difference
  logged before assertions; known differences use exact source/Go/C digests.
- Failure paths: C parser/tree/cursor/buffer cleanup inspected; producer rejects
  existing outputs; timeouts and size/depth/node limits retained; partial sets
  cannot be adopted. Real CLI invocation and negative boundary tests exercised.
- Architecture: only the existing adapter imports upstream; no new product
  dependencies, plugin layer, concurrency, query API or process-global counters.
- Windows: binary input, LF fixtures, relative CLI path resolution and native C
  builds exercised. Product tests consume JSON without invoking C or Python.
- Evidence: initial C# failure, missing-generator attempt and expected negative
  replacement check retained. Generator warnings retain their metadata limitation.
- Privacy: tracked files and reachable history contain none of docs/prompts,
  docs/specs, docs/plans, artifacts/handoff or .scratch. Ignore checks pass.
  There are no remotes. Historical handoff contents were never read.
- Git: four scoped implementation/evaluation commits precede the closing commit.
  The branch is LARGE under ADR-0005 (contract and patch proposal). No merge.

The unrestricted branch whitespace check returns 2 for exactly twelve context
lines in candidate.diff: a unified-diff context marker precedes existing Go tabs.
The raw artifact stays intact. All other branch files pass the whitespace check;
no setting was changed to suppress it. This is explained in Phase 3's review.

Residual blockers: product KR-0001b remains active for JSX/TSX; KR-0002 records
the C# excerpt's differing recovery. Its deeper internal cause remains open.
The isolated candidate is a recommendation, not an adopted product dependency.

NOT_RUN: Linux/container execution, remote CI, ARM64 execution, race/resource
campaign, full release corpus and query/supertype metadata gates. They are
outside this completed Windows foundation/evaluation scope and carry no claim.
The actual generator gap from Phase 2 was resolved in Phase 4; the earlier
attempt remains historical evidence rather than being edited to success.
