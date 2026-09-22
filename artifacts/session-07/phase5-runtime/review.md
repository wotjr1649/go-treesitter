# Duplicate recovery-cost calculation experiment

Completed isolated experiment; the product dependency is unchanged. The single
candidate diff reuses the first `ac` in `reduceForkWindowPreference`. Between
the two former calls only `dynamicPrecedence` fields were read. The cost walker
uses local state. All four callers of the preference function were inspected.
No persistent memo, parser pool, algorithm threshold or grammar rule was added.

The runtime copy was compared file-by-file with the pinned module archive:
3,275 files matched before the overlay. `plan.json` records the original tree,
archive, edited file and owned source hashes. Both binaries use that same local
module replacement; only the candidate uses the one-file overlay. Module cache,
reference clones, go.mod, go.sum and product identity remain unchanged.

E4: five paired processes, alternating order, one cold parse plus three warm
parses each; C# 128-class input, 4,644 bytes, 2,305 nodes. Go 1.27.1,
windows/amd64, CGO=0, GOMAXPROCS=1, GOMEMLIMIT=512MiB, GOGC=100. The plan preceded
execution. No sample was replaced or rerun.

| Per parse | Baseline | Candidate |
|---|---:|---:|
| Warm median | 1,292.606 ms | 830.128 ms |
| Warm min–max | 970.753–2,293.375 ms | 726.232–1,007.923 ms |
| Cold median | 1,289.550 ms | 981.697 ms |
| Allocated bytes, median | 24,425,554 | 19,978,053 |

The observed warm median is 35.8% lower and allocation is 18.2% lower for this
workload. The wide time spread and overlapping warm ranges prevent a stable
speed guarantee. Retained Go heap is essentially unchanged at about 39.4 MB;
this is allocation reduction, not a demonstrated RSS reduction. The two tree
hashes match in every process.

This phase independently ran both Go variants and pinned native C on the exact
85-input original/extended catalogs. All 85 baseline/candidate ordered snapshots
agree; all 11 recorded C differences, including KR-0002, retain their exact
digests. The other 74 agree with C for represented node fields (E5).
`comparisons.json` and the copied C build receipt bind this observation. New
catalogs added elsewhere after this experiment are not included in its count.

The candidate also passed the retained seven-route incremental self-consistency,
input/snapshot/memory/work-budget, tiny-timeout release and independent-worker
tests with its replacement explicitly selected. This is an experimental lane,
not a replacement of the product identity gate. No claim covers upstream's
entire language catalog or all of its own tests.

Self-review found no semantic state mutation or new dependency. Independent
review found no correctness defect in the diff or evidence and required the
time-spread limitation above. The candidate is suitable for a small upstream
performance submission after maintainer review; nothing has been sent. Product
adoption remains outside the earlier experiment-only authorization. A downstream
library's local `replace` is not a distributable solution for its consumers.
