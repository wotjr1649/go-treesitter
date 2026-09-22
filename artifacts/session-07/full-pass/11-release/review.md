# Final product review — Session 07

**FINAL VERDICT: BLOCKED_EXTERNAL.** The owned implementation, retained
correctness/resource checks and local package consumption pass. E6 release
approval still requires native Windows ARM64 execution, hosted CI and resolution
of two upstream license declarations. Nothing was pushed, tagged or published.

## Candidate and provenance

The executed product source is commit `23d6885ebe8709616614c89ffb6511ea815fb23f`,
following runtime work unit `4f0d3548d34cda68c38ee5d07a73e41049b82222`.
Closing documentation/evidence commits do not silently inherit a changed
runtime: their source equivalence and final ZIP consumption are recorded in
`../12-closure/`. `identity.json` binds source, tests, benchmarks and origins.

- Runtime origin: `gotreesitter v0.53.0`, commit
  `c871b1f576866c40b1695677fe3512243e266d39`.
- Final internal manifest:
  `5d1f526c35e223341914cbfada8abe1c4c49d52824a09950bde05d331ff41e46`;
  all 1,508 carrier files reproduce from the pinned archive and 11 patches.
- Go derived blob:
  `81f9b19b5886ac11646c713c0c531bd835fc026d363437ba29b28ac2faabea9a`.
  Its pinned C input, converter, source ZIP and MIT notice are bound separately
  from the original upstream Go blob. Grammar base commits did not move.
- C runtime remains v0.25.1, commit
  `f5afe475deb7c0bae6407fb776c76824f717bb61`. The complete approved Candidate C
  TypeScript patch remains SHA-256
  `8d091351f8107d1546a8130d281f0ca8d6d959b11da66b7872d3e10f998340e9`.
- First project version remains planned `v0.0.1`; local versioned ZIPs are
  validation artifacts, not a registry release.

## KR closure and correctness

KR-0001b ships the JSX/TSX text fix in the ordinary internal runtime.
KR-0002 preserves C# missing-token scheduling, shared-history alternatives and
acceptance order. KR-0004 preserves actual recovery errors during reconstruction.
KR-0003 uses the explicitly approved complete Candidate C grammar artifact.
All four records are retired for their retained controls. Original failures and
the original C evidence remain available. The C# changes have independent patch
identities and separate diagnostic evidence; they were integrated in the single
historical product commit `f54eaef`, not two rewritten commits.

`oracle/summary.json` records **688 fresh C records and 439 edited records**,
including the original 94/21 campaign and all added regressions. These are
registered case counts, not a claim of 688 unique source byte strings.
Each case runs 20 clean or 100 error-bearing Go fresh parses, three fresh C
parses, and a native C edit where specified. All represented ordered node fields,
coordinates, error/missing flags, outcomes and completion receipts agree.
No active C-tree exception remains. The additional large Go witness compares
all 57,348 nodes after an edit against both fresh Go and C.

The 206-language basic catalog passes in separate bounded processes: main 203
plus optional GPL 3. Its fresh/determinism/edit/cancel/admission/close checks are
E3 self-consistency, not E5 for all grammars. The strict E5 scope remains Go,
Python, JavaScript, JSX, TypeScript, TSX and C#.

The product run reports 911 passing tests/subtests; the selected-grammar run
reports 895. Go JSON reports nine packages as skipped because they have no
`_test.go` files; no named failing test was skipped. The four private invariant
tests run through the maintained overlay. Vet and oracle-tool tests pass.
Two declared 60-second fuzz runs pass: 16,378 incremental-agreement executions
and 49,059 bounded-parse executions. Earlier minimized failures remain retained.

## Controlled fallback

`fallbacks.json` records every nonempty fallback/reuse reason in the final
corpus. The compact EOF/admission declines and missing authenticated subtree
receipts use the classic dispatcher. Unproved scanner prefixes require fresh
parsing; C#'s unsupported incremental scanner remains an explicit full reparse.
The `JSX-tail-hash` error fixture uses `incremental_parse_full_retry`; its final
tree still equals fresh and C. These are observed route costs, not hidden
semantic divergence. Compact certificates for a different Go blob were not
transferred. No whole-product incremental-disable switch was introduced.

## Cancellation, concurrency and memory

Parsing, recovery and snapshot cancellation propagate explicit outcomes,
release partial trees, and allow the next parse to succeed. Supported independent
workers and immutable index readers pass the separate CGO=1 race diagnostic
(66 tests/subtests). Product results use CGO=0 throughout.

The Windows AMD64 retention workload covers seven clean routes, a large Go file
and the real C# recovery excerpt. Over 193 cycles and 3,281 parses, every opened
tree is closed, the observed goroutine count stays at one, and the 12 post-GC
heap checkpoints range from 83,966,168 to 83,967,832 bytes. Peak RSS is
295,403,520 bytes (281.7 MiB); after explicit scavenging it is 102,858,752 bytes
(98.1 MiB). Peak reported arena/scratch are 23,646,312 / 8,272,920 bytes.

The earlier conflict bug reached 3.27 GiB peak RSS in the same finite harness.
A leaf reuse path selected a shift while discarding a competing reduction,
growing stacks and copying them until a memory retry. Normal conflict dispatch
fixes that root cause and preserves real reuse. The retained 64 MiB admission
test fails before the fix and passes after it. This finite run does not prove
the absence of all possible leaks. Runtime memory limits still are not RSS caps;
grammar caches, snapshots, input copies and aggregate worker memory are separate.

## Performance and index audit

`performance/plan.json` declares five randomized paired seeds, GOMAXPROCS=4,
40 timed operations per mode, 59 modes, allocation metrics and full sample
retention. The baseline is `2e18f90`; both use the same benchmark inputs.
Paired log-ratio t intervals assume approximate normality; with five pairs the
exact two-sided sign test cannot reach p below 0.0625. These local measurements
are not upstream's full release-performance certificate.

Profiled raw recovery-cost walks and conflict-driven copying were corrected.
In an isolated five-pair comparison, child memo reuse cuts TSX delete median
from 7.81 to 3.80 ms and C# recovery from 12.20 to 10.91 ms. Memo reuse requires
matching arena, captured shape and node version; the negative control retains
an older hidden missing subtree after its live node changes.

The final candidate still has correctness costs versus the earlier baseline:
TSX delete 2.694 → 3.819 ms (+41.8%, paired t interval 1.319–1.518), Go replace
0.880 → 1.089 ms (+23.8%), Go insert +18.0% and no-edit +18.4%. Profiles place
the remaining work in parsing/dispatch and allocation, after the repeated raw
cost walk was removed from the dominant paths. The C-derived Go tables and
conservative lookahead/leaf admission preserve results that the cheaper earlier
candidate did not. No absolute latency SLO exists; the measured cost is retained
as an explicit correctness tradeoff, not described as a universal speedup.

The short query/index series was noisy, so a separate predeclared five-pair
series uses 1,000,000 query operations and 1,000 index constructions. TS query
medians are 79.46 / 80.23 ns (ratio 1.010, interval 0.931–1.038); JSX index
69.048 / 67.259 microseconds. Query allocations remain zero. Position profiling
is in `Index.NodeAt`; no semantic or storage index was imported from downstream.
Public NUL-node indexing has both a before-failure receipt and a passing retained
test. Source/algorithm audit and independent-review limits are in
`../09-audit/review.md`.

## Packaging, platform and license

Official Go `x/mod/zip.CreateFromVCS` and `CheckZip` produce the complete main
and optional module ZIPs. The main ZIP excludes the nested GPL module and the
separate evidence module. It contains all 203 main blobs, notices and the Go
conversion inputs. Empty-cache external consumers install version `v0.0.1`
through a local file proxy with no replace and no outside source reference.
Module graphs contain only two or three modules. A consumer-built probe matches
100 C records, including C# recovery/preservation and the adopted TypeScript
features. Closing ZIP identities are in `../12-closure/packaging/artifacts.json`.

Windows AMD64 builds/tests/consumer execution pass natively. The product graph
has 137 packages, zero CgoFiles and no runtime/cgo; ordinary Windows PE imports
contain only kernel32.dll. CGO=0 cross-links pass for Windows ARM64, Linux AMD64,
Darwin ARM64 and wasip1/wasm. Cross-linking does not establish native execution.
The latter non-Windows targets are outside first-release native scope.

The owner-approved GPL split retains caddy/disassembly/jq in the optional module
with notices and complete pinned source/converter archives. All 206 grammar
inventory rows are source-bound: 197 original notice texts and nine metadata
declarations. Brightscript and Cooklang disagree between ISC and MIT. The
release license checker deliberately exits 1 and reports BLOCKED_EXTERNAL;
neither license nor rights-holder permission was invented.

## Remaining external work

1. Obtain rights-holder clarification and applicable notices for the two
   conflicting grammar declarations, while preserving the approved 206 scope.
2. Supply the GitHub repository target and explicit branch-push/CI authority;
   no origin is configured. Native AMD64/ARM64 workflow jobs and a separate
   AMD64 race job are prepared. Official runner labels were checked against
   [GitHub's runner documentation](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
3. Execute native Windows ARM64 and hosted CI on that final published branch,
   preserve logs/architecture/job identity, and resolve any resulting failure.

The approval request is separate from completed local implementation. No
internal failing gate is being relabeled as an external blocker. The final
evidence/handoff records the actual Git state and code/ZIP equivalence.
