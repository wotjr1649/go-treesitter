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

## Amendment (2026-11-19): decision 5, how upstream posts are drafted, approved and recorded

This amendment follows the 2026-11-16 one; where they differ ("agents prepare only; the owner writes every post"),
this one governs: agents now draft, the owner still posts. It applies to every upstream organisation alike.

Sessions 18–19 settled the practice below.

- **Agents draft and prepare.** An agent may produce the reproduction, a branch in a local clone or on the owner's fork,
  local runs of the upstream CI steps, a fact sheet (each claim with its source and whether it was reproduced) and the
  full text of a post in English, with a Korean translation. Agents may read public upstream repositories (GitHub API,
  GET only).
- **One recorded owner decision per post.** Each issue, pull request or comment needs the owner's decision for that
  post, recorded in the session notes before posting (date, target, draft path). A decision covers one post; a changed
  draft needs a new decision.
- **The owner reviews, edits as needed and posts** from the owner's account, for example with `gh` run by the owner.
- **Disclosure.** Every post ends with this line, which the owner may extend: "I used an AI coding assistant to
  reproduce the issue and draft this text; I checked the results and edited it." A pull request body also names the
  generator CLI used for regenerated files (2026-11-16 amendment).
- **Agents never** post, comment, react, open or edit issues or pull requests, or push to an upstream repository, and
  never use a credential for that. Pushing to the owner's fork happens only when the owner asks.
- **Records.** Drafts and fact sheets stay in the session's maintainer-local scratch directory; before it is cleaned, copy
  them to `artifacts/handoff/` with hashes (AGENTS.md). Each posted URL is recorded with its date in the session notes
  and in the "Upstream issue" line of the matching known-gap entry (or the `upstreamReport` of the patch record).

### Relation to the tree-sitter AI policy

The [policy](https://tree-sitter.github.io/tree-sitter/6-contributing.html) bans "copy-pasted LLM output in issues, PR
descriptions or comments", asks for concise text from a person who understands the problem, allows translation tools,
and requires disclosure of AI use. Posting an edited agent draft sits closer to that line than the 2026-11-16 wording
did. The practice relies on the owner's edit and understanding and on the disclosure; the upstream maintainers decide
whether a post follows their policy, and a post they close for that reason is recorded, not retried. If the owner
cannot make a draft their own, it is not posted.

## Amendment (2026-11-23): decision 5, disclosure follows the target's policy

This amendment changes only the **Disclosure** item of the 2026-11-19 amendment; the rest of it still governs.

- **Disclosure follows the target project's own policy.** Where that policy requires disclosure of AI use, every post
  ends with a short disclosure in the owner's own words; the 2026-11-19 line stays the default wording. This covers
  every repository of the `tree-sitter` organisation: the
  [AI policy](https://tree-sitter.github.io/tree-sitter/6-contributing.html) lives in `tree-sitter/tree-sitter`, the
  grammar repositories have no contributing guide of their own, and their maintainers overlap. Where the target has no
  such policy, disclosure is the owner's choice.
- Before a draft is posted, the agent states whether the target's policy requires disclosure, with the source it read.

On 2026-11-23 four replies were posted without disclosure. At the owner's direction, an agent appended the line
"I used an AI coding assistant to draft this comment; I checked it before posting." to the three in the `tree-sitter`
organisation (tree-sitter-c-sharp#480, tree-sitter-c-sharp#458, tree-sitter-ruby#306). This edit was a one-time
exception, made at the owner's direction, to "agents never edit". The reply on gmr/tree-sitter-postgres#101 stays as
posted, because that repository's `CONTRIBUTING.md` has no AI clause.
