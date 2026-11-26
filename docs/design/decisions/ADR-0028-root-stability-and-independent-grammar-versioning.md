# ADR-0028 — Root 1.x stability and independent grammar versioning

**Status:** Accepted · 2026-11-24

## Context

The v0.6.0 Preview ships the root and 25 grammar modules. They have separate tags but share a version
number and root requirement. A grammar pin move can change trees and captures with the same root and
query text. Independent versions let consumers upgrade the root while keeping the grammars their
queries depend on. [ADR-0022 decision 2](ADR-0022-wave1-preview-versioning-and-release-memory-criterion.md#decision)

## Decision

1. **Release path (D).** Ship `v0.7.0` Preview with the proposed stable contract, version policy and
   support tables, then root `v1.0.0-rc.N` for migration against named grammar tags, then root
   `v1.0.0`. Plan about six weeks. The applicable release gates and owner approval determine when
   each release ships. Changes to the root candidate contract, code or inputs require a new RC and
   evidence for the affected claims. Grammar modules stay on `v0.x` throughout this transition.

2. **Root 1.x contract.** Preserve source compatibility of the stable public Go API and its
   documented behavior with a fixed released grammar. This covers parsing, errors, edits and changed
   ranges, ownership, lifetimes and concurrency, encodings and coordinates, and query validation,
   iteration, predicates and results. Before RC, classify the public exports, align the API
   documentation with implementation, mark experimental packages, and document `NewLanguage` as
   the generated-code entry point. [Public API design](../api-v2.md)

   Keep released engine ABI 1–4 modules buildable and loadable. ABI 1 parses and returns
   `ErrQueryUnsupported` for queries. ABI 2 adds queries, ABI 3 adds single-child field-lookup
   exceptions, and ABI 4 adds public symbol IDs. Preserve legacy field lookup in ABI 1/2, raw symbol
   IDs in ABI 1–3, and the documented rejection of unsupported formats and ABIs.
   [Engine ABI](../engine-abi.md) · [Parser result contract](../../specs/parser-result.md)

   A root patch preserves or restores the documented contract. A root minor adds compatible API or
   behavior, including new ABI support. Breaking a stable signature or documented behavior, or
   dropping a supported ABI, requires a root major. Document fixes that change results with a
   fixed grammar.

3. **Root release gates and grammar limits.** Gate the root RC and 1.0 solely on the root's Go API
   and documented behavior, engine ABI 1–4, packaging, and the Release-Critical and Release gates
   for that root scope. Exercise actual released modules for each ABI and mixed-version, CGO-free
   consumers. Grammar known gaps do not block either root release. Publish each gap as a documented
   limit in a per-grammar support table with the affected tags, input and impact. Keep existing
   grammar release and pin-move gates for grammar releases. Ship known-gap fixes in the affected
   grammar's independent minor releases. For Korean-input gaps, carry temporary patches in
   separate PRs under ADR-0023, with its approval, evidence and retirement requirements.
   [Validation gates](../../specs/validation.md#gates) · [ADR-0023](ADR-0023-temporary-upstream-grammar-patches.md)

4. **Independent grammar releases.** Keep each import path and `Language() *treesitter.Language`.
   Tag each module independently as `grammars/<dir>/v0.x.y`. A grammar source pin move takes a minor
   for each affected module, including both TypeScript and TSX when their shared pin moves.
   Report old and new pins, changes to CSTs, node types, fields, query compilation, matches and
   captures, and examples for consumers who need to migrate. Other same-pin fixes that preserve
   the declared contract may take a patch. Carry forward unaffected tags across root releases.
   For owner-maintained sources, cite the owner release and owner CHANGELOG.
   [ADR-0024 rule 7](ADR-0024-grammar-source-tiers.md#amendment-2026-11-18-rule-7-owner-maintained-repositories)

   Each new grammar tag requires the oldest published root that supplies its API, ABI and generated
   host imports. Its release module files record that minimum and its checksums without `replace`.
   A root release alone does not raise grammar requirements or trigger grammar tags. Test the
   minimum and recommended roots. Go selects the highest requirement for a given module path, so
   upgrading one grammar can raise the root selected for the whole application.
   [Go minimal version selection](https://go.dev/ref/mod#minimal-version-selection)

   Publish a root/grammar combination table with each root or grammar release. Record tested root
   versions, new and carried-forward grammar tags, source revisions, generator identities, ABI,
   minimum roots and evidence scope. Bind the results to the matching release commits.

5. **Go minimum.** Follow the oldest Go major release that the Go team supports. The current floor
   is Go 1.26 (`go 1.26.0` in `go.mod`, exercised by `engine-go126`). When a new Go major release
   changes that support window, raise the `go.mod` floor and update the CI job for the minimum
   Go version together in the next root minor release. Treat this floor increase as a minor change.
   [Go release policy](https://go.dev/doc/devel/release#policy) · [Engine CI](../../../.github/workflows/engine.yml)

6. **Tree-sitter toolchain.** Root 1.0 uses the pinned CLI and runtime 0.27.2. Handle the 0.28 move in
   a separate PR under #67. For that move, only grammars whose trees change get a grammar
   minor release. A compatible change that affects the root contract takes a root minor.
   [ADR-0027](ADR-0027-one-pinned-generator-cli.md) · [Issue #67](https://github.com/wotjr1649/go-treesitter/issues/67)

## Consequences

- Update [publication](../../specs/publication.md#multi-module-release) for independent tags, root-only
  and selected-grammar releases, combination and support tables, and per-module retraction. Publish
  a required root before tagging dependents. Preserve separate owner approvals and immutable tags.
  Exercise independent module ZIPs with empty-cache consumers and the public proxy.
- Update [`regen.sh`](../../../tools/gen/regen.sh), generator and packaging tools, and drift CI to handle
  development and release module files and minimum roots per grammar. A metadata-only conversion
  must leave generated engine files byte-identical.
- On acceptance, append a dated amendment to [ADR-0022](ADR-0022-wave1-preview-versioning-and-release-memory-criterion.md)
  replacing the lockstep tags and shared root requirement in decision 2. Decisions 1, 3 and 4
  continue to apply, including the resource criteria in decision 3. Update the index at that time.

## Amendment (2026-11-25): decisions 1 and 3, root scope, RC bench, RC exit and Windows resources

This defines "that root scope" in decision 3 and the RC to 1.0 conditions in decision 1. It replaces no other
text. [Validation contract](../../specs/validation.md#root-rc-and-10-scope) holds the gate rules.

1. **Root scope.** The Release-Critical and Release gates for the root RC and 1.0 cover all 25 grammar
   modules. Hosted engine and bench CI cover Linux amd64/arm64, macOS arm64 and Windows amd64/arm64.
   Hosted conformance covers Linux amd64/arm64. A grammar's resource-budget failure (bench) fails the root gate.
   A smaller scope exists only as a per-case exception that the owner records.
2. **RC bench.** Run the hosted bench exactly once on the RC candidate commit, and declare the run before it
   starts. If it fails, fix the cause and cut a new RC, or the owner decides a new per-case exception. The
   criteria do not change. The v0.7.0 Preview macOS kotlin exception does not carry into an RC or 1.0.
3. **RC exit.** Any change to root code, contract or inputs during an RC cuts a new RC. Without such a
   change, 14 days without root defect reports lead to 1.0.
4. **Windows resources.** The hosted Windows bench required before 1.0 is in place for amd64 and arm64.
   For the RC, also run the Windows amd64 local bench for all 25 grammars on the candidate commit.
