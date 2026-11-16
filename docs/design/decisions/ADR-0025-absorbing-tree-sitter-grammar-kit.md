# ADR-0025 — Absorbing tree-sitter-grammar-kit

**Status:** Accepted by the owner · 2026-11-15 · amends ADR-0020 (Context and decision 6: grammar-kit as a separate
supply-chain checker) and ADR-0024 (Consequences: "being absorbed").

## Context

tree-sitter-grammar-kit was closed on 2026-11-14
([closure](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/0a12156/docs/reports/repository-closure.md),
handoffs [A](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/0a12156/docs/reports/repository-closure-handoff-a.md)
and [B](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/0a12156/docs/reports/repository-closure-handoff-b.md)).
It was built for a patched pure-Go runtime; most of its machinery judged patched grammars. This repository now uses
unmodified upstream C with its own native reference, so only part of the kit is useful here. Two independent reviews
of the absorption plan found the handoff's proposal sound in direction but its evidence and acceptance criteria
incomplete.

## Decision

1. **Reference, not dependency.** The kit is referenced by GitHub URL at commit `0a12156`; its archive (sha256
   `83d792c6453a5f316e101a76885f637b4397ba7a601cfebbd7a388f23cab07d3`) is preserved with this repository's release
   evidence. No product module imports or copies kit code (ADR-0020 decision 6 stays: supply-chain tooling lives outside
   the product module, now in `tools/`).
2. **Absorbed now:** a node-type schema register (`docs/validation/grammar-schemas.md`); a strict `node-types.json`
   check and diff reimplemented in `tools/gen/schema` (`go run . schema check|diff`; kit diff/risk code names and
   invalid-input cases reused as test data), wired into CI at the first real pin move together with an old-pin vs
   new-pin native tree diff; kit case sources and
   edits (never its expectations) as extra native-vs-engine inputs, after measurement; a defect register in
   `tools/conformance` whose entries are strict expected-failure tests (removed when fixed), from which per-grammar
   known-gap documents are generated.
3. **Not absorbed:** native execution, oracle and incremental comparison (covered by `tools/native-ref` and
   `tools/conformance`); qualification, query-semantics verification, cost ledgers, G1 approval machinery (built for
   patched grammars); `identity`/`verify` (covered by `pins.json` hashing); kit expectations as gates.
4. **Later, separately decided:** T-SQL engine verdicts (726 SQL Server PARSEONLY cases) as an independent oracle;
   kit queries (124) as query-lane inputs; `SECURITY.md`.
5. **Upstream contributions** follow a fixed procedure: reproduce on the current default branch, search existing
   issues and pull requests, prefer a pull request with a corpus test on an existing issue, one repository at a time,
   owner approval per post.
6. The kit's GitHub archive is decided by the owner after this absorption is confirmed complete.

## Consequences

- ADR-0020's "grammar-kit only for supply-chain checks" becomes "the useful checks live in `tools/`".
- Known gaps carry E3 evidence (retained tests bound to pins), not one-off probes.

## Amendment (2026-11-16): decision 4

- `SECURITY.md` is added at the repository root.
- The kit's 124 queries stay deferred as query-lane inputs.
- T-SQL verdicts from SQL Server (the 726 PARSEONLY cases) are not absorbed here. The owner's grammar repository
  `wotjr1649/tree-sitter-mssql` takes them over as its own oracle. This repository keeps the pinned T-SQL grammar
  (ADR-0024) and its known gaps.

## Amendment (2026-11-16): decision 5, how upstream contributions are made

The tree-sitter [AI policy](https://tree-sitter.github.io/tree-sitter/6-contributing.html) forbids direct coding-agent
interaction with its repositories and pasted LLM text, and requires disclosure of AI use. Three issues were posted by
an agent from the owner's account without disclosure; the owner disclosed this on each afterwards.

- Agents prepare only: reproduction, the patch on a branch of the owner's fork, local runs of the upstream CI steps,
  and a fact sheet. The owner writes and posts every issue, pull request and comment, and discloses AI use. A
  translation of the owner's own text is allowed.
- Regenerated files in an upstream pull request use that repository's own pinned CLI (its `package.json`), as its
  contributors do, and the pull request names the CLI used.
- When testing several variants locally, give each its own `TREE_SITTER_LIBDIR`; a shared compiled-parser cache
  makes `tree-sitter test` run an older parser.
