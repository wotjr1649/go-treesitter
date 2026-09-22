# C# recovery product adoption

Base commit: `388f50cb156e13ff2aad0b32e69f102f422199d4`.
Runtime manifest: `cc6eb7a4f2426f66c296c09faefbaa9830ee5a9af7d11af558442eca0915e6c5`.
Upstream v0.53.0, C runtime v0.25.1, grammar commits and blobs remain pinned.

## Change and causal evidence

The KR-0002 patch preserves pending recovery lookahead, C version order,
hidden missing-leaf costs, shared closed error history and acceptance order.
The C# profile is bound to its existing blob hash. Clean suffixes may merge
only when every path reaches the same error-prefix pointer; distinct histories,
open recovery and bypass paths remain separate. The helper polls the existing
stop signal. The pending-token loop uses the existing reduction bound.

The independent KR-0004 patch prevents source reconstruction from replacing
an error-bearing tree. The KR-0002-only experiment fixes the large original
witness but still loses errors on reduced cases, as recorded in
`../03-csharp-kr2-only/comparison.json` and `../kr2-product-tests.json`.
The two patches therefore land in one passing product commit. Neither a
temporary exception nor a failing intermediate commit is introduced.

The original KR signatures and C records remain available. Active C#
exceptions are removed; the six bare-ampersand KR-0001a characterization
records remain. Ordered node comparison, error/missing flags and result
classification now follow the pinned C receipt on every adopted case.

## Observed checks

| Evidence | Result and scope |
|---|---|
| E5 `../csharp-product-campaign.json`, `repeated.json` | 72/72 inputs match C; 100 recovery or 20 clean Go repetitions; 72 Go edit/fresh comparisons |
| E5 `../csharp-native-new-edits.json` | 72 C edit/fresh comparisons and six invalid-edit admission controls pass |
| E3 `../csharp-final-product-tests.json` | `CGO_ENABLED=0 go test ./... -count=1 -timeout=150s -json` passes, including consumer and provenance checks |
| E3 `../csharp-final-vet.json` | `go vet ./...` passes |
| E3 `../csharp-product-catalog.json` | 206/206 basic grammar checks pass after adoption |
| E3 `../kr2-runtime-invariants.json`, `../csharp-instrumented-invariants.json` | Shared-prefix negative controls and acceptance order pass in ordinary and `gts_workcount` builds |
| E3 `../csharp-oracle-tool-tests-v2.json` | Six Python oracle-boundary tests pass |
| E4 `../03-kr2-product/reproduce.json`, `../03-kr4-product/reproduce.json` | Both adoption stages reproduce all 1,527 carrier files from the pinned archive and maintained patches |

The two new catalogs retain 74 fresh records, including a duplicated clean
origin, and 72 target edits. C# edits currently use the documented
`external_scanner_unsupported` fresh fallback; these results do not claim old
subtree reuse. Whole-language or release-wide parity is not implied.

Some C roots begin after leading hidden whitespace. Oracle receipt validation
now requires an ordered root range, an EOF end, and exact agreement with the
first canonical node. It rejects inconsistent range metadata. No canonical
node or error evidence is discarded.

## Review and preservation

Reviewed scope, branch behavior, failure propagation, resource ownership,
public API boundaries, fixture licensing, Windows execution and provenance.
No public API types or outcome ordering change. No external dependency,
network access, shared mutable parser or new pool is introduced. Runtime
checks run with CGO disabled and local caches. Untrusted source bytes stay
inside bounded local parser processes. Full resource and platform campaigns
remain separate release requirements.

The prior public-source C recovery review informed the trace comparison.
Repository changes were reviewed locally; no private diff was sent to a
delegate. No additional unresolved finding was identified in this review.

`command-output.zip` preserves 80 original command logs, including failed
experiments. `diagnostic-sources-v2.zip` contains candidate patches and scripts
with entry hashes. Its base is the commit above. The first archive stopped on
a pre-bundle scratch target; its partial bytes and status are retained in
`diagnostic-sources.zip` and `diagnostic-sources-partial.json`. The completed
archive explicitly excludes that differently based overlay. Earlier raw
observations and original scratch sources remain untouched.

Unified diff context in the maintained `.patch` files contains literal leading
spaces before tabs and blank context lines. These are patch syntax, not product
source whitespace. Ordinary changed sources pass `git diff --check`.
