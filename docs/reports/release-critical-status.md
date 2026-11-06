# Release-Critical status board

Updated 2026-11-07. The runtime baseline candidate is described in
[baseline-provenance](../specs/baseline-provenance.md). Its carrier applies 28
patches (runtime manifest `b54a07c1…`, committed as `0eee29e`). It passed the
local Windows AMD64 product checks below. It is not release-approved. PR #1
integrated it into main as merge commit `402b52c`.

[The candidate's verification projection](../../artifacts/release-candidate/runtime-upgrade/validation-0eee29e.json)
binds commit, tree, commands, log hashes and omitted private fields. The checks ran
on a clean checkout of a commit with exactly the tree of `0eee29e`, before
`0eee29e` itself was created. The full product run passed 889
terminal tests, the selected-grammar run passed 874, and the private runtime
checker passed 6. None of those selections skipped a test. The 703-case core
bundle includes ordered C comparisons and regression controls; it is not a count
of unique C input records. All previously retained candidate C mismatches,
including GO-third, are resolved for this identity. An earlier working-tree run
of the same manifest is projected in
[validation-28.json](../../artifacts/release-candidate/runtime-upgrade/validation-28.json).

| Gate | Current candidate state |
|---|---|
| Windows AMD64 product | PASS (E3) on `b52f8ce` ([projection](../../artifacts/release-candidate/runtime-upgrade/validation-b52f8ce.json)) and earlier on the tree of `0eee29e`: build, vet, full and selected-grammar tests, private invariants and native consumer execution. |
| Retained core C comparisons | PASS (E5) on `b52f8ce` and on the tree of `0eee29e` for their pinned inputs and represented node/result fields. |
| Runtime and grammar provenance | Product provenance checks pass. The importer reproduces all 1,561 inventoried carrier files. |
| Local optional GPL module | Checks pass using a local go.work; official ZIP consumers remain a separate gate. |
| Performance | A [controlled whole-product comparison](../../artifacts/release-candidate/runtime-upgrade/runtime-comparison.json) of v0.53.0 against a working tree with this candidate's runtime manifest `b54a07c1…` completed. It binds the manifest, benchmark inputs and binaries, but not the full source tree, so it is an observation for that manifest rather than an E4 result for `0eee29e`. Allocation volume falls in major full-parse workloads. Edit latency is mixed, and host load varied. No speed claim is made. |
| C# grammar-kit pin check (ADR-0017 Decision 5) | Run with tsgk v0.2.0 (E2, local receipts). Origin: the commit-addressed GitHub tar.gz of tree-sitter-c-sharp `9150f7d5`, downloaded again, is byte-identical to the archive the oracle lane recorded, and its 8 grammar inputs match. `verify --archive` of the same commit's GitHub ZIP against an expected document built from those inputs: PASS, no differences (`parser.c` read under the kit's registered large-file exception; the expected document is caller-declared from the oracle snapshot). Product blob identity: observed (NOT_ASSESSED). Update review against the previous pin: verify FAIL on four changed files and schema diff FAIL with 28 static node-types changes (27 additions; `lambda_expression.parameters` widened to multiple). Both outcomes are expected for a pin update and do not approve the change. The previous pin's `parser.c` exceeds the kit file limit, so its old-to-new difference is not kit-reported. Fingerprints identify bytes, not authorship. |
| C# constructs added by the new pin | `csharp-constructs-cases.json` adds 16 authored records at the new pin: lambdas with several or typed parameters, `async` and `static` lambdas, collection expressions with spreads, an extension block, list and slice patterns, compound-assignment operator declarations, null-conditional assignment, 7 edits and 2 erroneous lambdas. All agree with C (E5, these inputs only). A third erroneous lambda, `(a, b => a + b;`, differs and is open as KR-0005. Those records give each lambda a single `parameters` child. `csharp-lambda-modifier-cases.json` adds 7 records (2 edits, 2 erroneous) whose implicit parameters carry `ref`, `out`, `in`, `scoped` or `readonly`. C emits the multi-child field (modifiers, `,` and `)`) in 6 of them, including the missing-body error; it parses the unclosed variant as a tuple argument holding a single-parameter lambda, so no unclosed multi-child list is covered. Go agrees exactly with all 7, including both edits, which C# handles by full reparse (E5, these inputs only). |
| C# Release-Critical | RED: KR-0005 open for this candidate. Error flags agree, but the ordered tree of that unclosed lambda differs from C; an exact-signature ratchet holds it. Not observed at, and not attributed to, the earlier carrier. |
| Official module ZIPs and empty-cache consumers | PASS locally for commit `b52f8ce` ([receipts](../../artifacts/release-candidate/local-b52f8ce/index.json)), from a fresh regular clone: the 201+3 split, consumers without replace and with empty caches, CGO-free graphs, and all 204 basic catalog cases. Hosted CI on `650d133` also passed this check on both AMD64 and ARM64, with identical module ZIPs ([receipts](../../artifacts/release-candidate/native-650d133/index.json)). |
| Release license gate | PASS locally for commit `b52f8ce` and in hosted CI on `650d133`: 204 grammars, no unresolved declarations. |
| Windows ARM64 native | PASS in hosted CI on `650d133` ([run 42259446839](https://github.com/wotjr1649/go-treesitter/actions/runs/42259446839), job `product (windows-11-arm, arm64)`): build, vet, full and selected-grammar tests, tool tests, native consumer, private invariants, official module ZIPs and empty-cache consumers. A local cross-link of `b52f8ce` also passed. |
| Race diagnostic | PASS locally for commit `b52f8ce` and in hosted CI on `650d133`, using the workflow's selection (diagnostic lane; never a product gate). |
| Remote/hosted CI | PASS on `650d133` ([run 42259446839](https://github.com/wotjr1649/go-treesitter/actions/runs/42259446839)): licenses, product AMD64, product ARM64 and race. |
| Release decision | Not made. |

Commit `b52f8ce` differs from `0eee29e` in documentation (including README files),
evidence, `.gitattributes`, the GPL provenance entry for its sources README, and
the license manifest source URLs; it changes no Go code or test.
The product gate, packaging, license, cross-link and race results above were run
on `b52f8ce` itself. The commit that records them changes only this board and
`artifacts/`. It was not itself packaged, and those results do not transfer to
it.

The hosted CI results above belong to `650d133`. The commit that records them
changes only this board, `artifacts/` and `.gitattributes`, and its own CI run is
separate.

The C# construct records and KR-0005 were added after that. Their commit passed
a local CGO-free `go vet` and full product test run on its own tree; it was not
itself packaged, and the gate rows above do not transfer to it.

The exact current source and patch inventory remain in
`internal/provenance/identities.json` and `tools/runtime-bundle/source.json`.
No allocation optimization beyond the 28 inventoried patches is part of this
candidate. Existing historical native CI results do not apply to this new
identity. Current release requirements are in
[validation](../specs/validation.md) and [publication](../specs/publication.md).

## Historical evidence at the pre-upgrade identity

Everything below is retained as historical evidence from the earlier
v0.53.0 carrier and its candidate commits. It is not the status of the current
candidate and does not satisfy its product or release gates.

Updated 2026-10-23 (ADR-0017). **Release is not yet executed.** Brightscript
and Cooklang are no longer distributed. At `d6c802a` the whole Windows workflow
passed for the first time ([run 39949173766](https://github.com/wotjr1649/go-treesitter/actions/runs/39949173766)):
native AMD64 and ARM64 product jobs, the license gate (204 grammars) and the race
diagnostic. On 2026-10-23 the owner made `main` (this curated history) the
default branch and removed the old `session/07-product-hardening` ref; its
commits stay reachable by SHA and in earlier copies. Tags remain held. Development, Integration and the retained
seven-route Release-Critical correctness gates pass on this carrier
(`0f32d229…`): a local Windows AMD64 CGO=0 rerun passed all 911 product
test/subtest results, including the recorded C comparisons. This is finite-corpus
E5 evidence for those routes only, not proof for all source programs or for the
other grammars, which have the basic check. [Selected evidence](../../artifacts/release-candidate/README.md).

The historical measured product source commit is `23d6885ebe8709616614c89ffb6511ea815fb23f`.
At `e4e6a56` the curated candidate preserved all 1,807 files of that production
identity. ADR-0017 deletes 15 of them and changes 11 (carrier, provenance,
catalog and tooling), so the historical measurements below stay bound to
`23d6885` and are not transferred to this candidate.
Runtime origin remains `gotreesitter v0.53.0`; the internal carrier manifest is
`0f32d22943e7e636f1ccdf03ec677871f67f81bc22db106a739d88a950ffa6e6` (ADR-0017;
the receipts below at `e4e6a56` bind the earlier
`5d1f526c35e223341914cbfada8abe1c4c49d52824a09950bde05d331ff41e46`).
The Go product blob now derives from the already pinned C tables (ADR-0015).
C runtime v0.25.1 and grammar base commits remain pinned. Candidate C's complete
TypeScript patch remains the approved C artifact (ADR-0012).

| Route | Retained Release-Critical gate | Limitation |
|---|---|---|
| Go | PASS, fresh/edit/C agreement | C-derived Go artifact has its own identity; generic compact admission is not a transferred certificate. |
| Python | PASS, fresh/edit/C agreement | Scanner-prefix checks may require an explicit fresh fallback. |
| JavaScript | PASS, fresh/edit/C agreement | Recovery can decline compact admission. |
| JSX | PASS, fresh/edit/C agreement | Bare `&` remains a C grammar error; the recovered tree now agrees exactly. |
| TypeScript | PASS, fresh/edit/C agreement | Maintained Candidate C patch is required for the pinned oracle artifacts. |
| TSX | PASS, fresh/edit/C agreement | Conservative reuse and recovery checks have a measured deletion cost. |
| C# | PASS, fresh/edit/C agreement | KR-0002/4 retired; the external scanner still uses an explicit full reparse on edits. |

KR-0001b, KR-0002, KR-0003 and KR-0004 are retired for their retained controls.
Original failures remain recorded. No active C-tree exception or unexplained
comparison mismatch remains in the executed final corpus. Controlled fallback
reasons are enumerated in `artifacts/release-candidate/historical/fallbacks.json`.

## Executed evidence

- 688 C records, 439 edited records; repeated fresh recovery observations and
  three native C executions per record. One additional 8,192-declaration Go
  edit agrees with C over all 57,348 ordered nodes.
- 206 basic grammar checks at `e4e6a56`, before ADR-0017: 203 in the base
  module and 3 in the optional GPL module; three fresh runs, edit/fresh equality, cancellation, admission and
  close. This does not certify C parity for the entire catalog.
- Product and selected-grammar suites, vet, oracle-tool tests, four private
  runtime invariants, 66 race test/subtest results and two 60-second fuzz runs.
- 193 memory cycles / 3,281 closed trees. Post-GC heap stayed within
  83,966,168–83,967,832 bytes; peak RSS was 295,403,520 bytes on the measured
  Windows AMD64 host. This finite observation is not a hard RSS guarantee.
- Five randomized paired performance seeds plus longer lookup samples and
  profiles. Correctness has costs: TSX delete median +41.8%, Go replace +23.8%
  versus the earlier `2e18f90` baseline. Profiled avoidable raw-cost walks and
  conflict-driven stack copying were fixed; all samples remain recorded.
- Official Go module ZIP creation and checks, empty-cache consumers without
  replace, 100 external-consumer C comparisons, and separate GPL consumption.

## Platform and remaining release blockers

| Gate | State |
|---|---|
| Windows AMD64 native | PASS: CGO=0 build/tests/consumer execution, rerun locally for the ADR-0017 carrier. |
| Product dependency/PE audit | PASS: 137 dependency packages, zero CgoFiles, no runtime/cgo; ordinary PE imports kernel32.dll only. |
| Windows ARM64 | PASS natively at `d6c802a` and earlier at `e4e6a56`: all native CGO=0 product, recorded-C, selected-grammar, oracle-tool, runtime-invariant, official-ZIP consumer and 206-entry catalog steps completed. |
| Linux AMD64, Darwin ARM64, wasip1/wasm | CGO=0 cross-link PASS; execution NOT_RUN and outside first-release native scope. |
| Hosted CI | PASS at `d6c802a`: all four jobs ([receipts](../../artifacts/release-candidate/native-d6c802a/index.json)). At `e4e6a56` it failed only on the license gate. |
| License release gate | PASS for 204 grammars, local and hosted at `d6c802a` (ADR-0017). |

Own code remains MIT. The optional GPL module contains caddy, disassembly and
jq with source archives and notices. All 204 inventory rows have identified
license evidence. A local candidate ZIP is not publication approval.

The initial version remains `v0.0.1`. The owner authorized publication of
`session/07-product-hardening` at `2494b35499a3b87281e6837de0f9820a28a6f7ec`.
[The first hosted run](https://github.com/wotjr1649/go-treesitter/actions/runs/35799931220)
records the native checks and the retained tooling failure. The independent
`validation/release-v0.0.1` candidate excludes the raw session/experiment history
and publishes technical contracts and selected evidence. Its local pre-commit
checks passed 911 product and 895 selected-grammar test/subtest results, six
oracle-tool tests, one packaging boundary test, four private runtime invariants
and vet. See [the receipt](../../artifacts/release-candidate/precommit.json).

The Windows workflow runs official module ZIP and empty-cache consumer checks
on both native architectures, including every basic catalog case against the
packaged modules (206 at `e4e6a56`; 204 after ADR-0017). [Run 35804758357](https://github.com/wotjr1649/go-treesitter/actions/runs/35804758357)
completed these checks at `e4e6a56bc5ee0e6a5a283381e2c929dea48ae2c4`.
[Its retained receipts](../../artifacts/release-candidate/native-e4e6a56/index.json)
bind the candidate, toolchains, executables, module graphs and downloadable CI
artifacts. The local, hosted AMD64 and hosted ARM64 ZIP hashes match exactly:
main `e914918acd95eb85e58eae6adca26fcf0853e53b93f41d49bd610259c620a239`,
GPL `ec4e23ff5684793cc9a0e2b11a37923e11450fda8bd96dee8cb72411724ce56d`.
These are that commit's ZIP identities; documentation-only commits produce new
ZIPs and their own workflow receipts.

No gate is waived by another job passing. Existing published
session history is a separate disclosure surface; the curated branch does not
erase it. No main merge, tag or release was performed.

MemoryBudgetBytes covers parser-managed accounting, not total process
RSS; grammar caches, source copies, snapshots and concurrent workers require
separate application budgets. The independent private-diff review limitation is
retained in the local audit: the primary agent reviewed the owned diff;
independent public-source reviews did not review the private repository diff.
