# Selected embedded grammar build

Reuse the pinned upstream's grammar_subset build tags; add no loader, registry,
dependency or runtime patch. The optional set contains all six grammars for the
seven assessed routes. Both default and selected builds remain CGO-free and
embed their own blobs. Default behavior and Release-Critical scope are unchanged.

E4, Windows/amd64, Go 1.27.1: the same retained tools/measure program was built
twice with only the build tags changed. Default executable: 35,230,208 bytes;
selected executable: 17,077,760 bytes (51.5% smaller for this program).
Initial Go heap after GC was about 912–917 KB versus 252–261 KB across the
workloads. Retained post-run heap was roughly 0.75 MB lower. These are Go heap
and executable-size measurements, not process RSS or a size guarantee for every
consumer. Per-parse allocation was effectively unchanged. Warm time ranges
overlap, so no general parsing-speed improvement is claimed from the tags.

Five paired process samples per route and variant were declared in advance.
Each process parsed one 128-item source once cold and 20 times warm. Run order
alternated default/subset then subset/default. GOMAXPROCS=1, GOMEMLIMIT=512MiB,
GOGC=100. Every process compared complete ordered snapshots internally, and
source/tree hashes matched across both builds for all seven routes.

The initial campaign completed 61 of 70 samples before a selected C# process
hit the 20-second outer watchdog. The immediately preceding default C# sample
already measured 0.939 seconds per warm parse, so 20 parses plus startup were
too close to that watchdog. The failure and all completed samples were kept.
The timed-out process had stopped and no measurement child remained. Only the
nine missing samples were collected under a 45-second outer limit; source,
binaries, iterations and measurement environment did not change. There was no
retry-until-fast or replacement of completed slow samples. continuation-plan
and both sample streams identify this measurement-procedure correction.

The C# input is only 4,644 bytes and 2,305 nodes. Median warm parse time remained
about 1.04 seconds by default and 1.02 seconds in the selected build, allocating
about 24.4 MB per parse. This is an unresolved runtime performance finding,
not a packaging fix. A separate three-warm-parse CPU profile attributed 52.5%
cumulative samples to rawStackEntryErrorCost and 17.8% to canReach. Profiling
numbers are diagnostic and are not merged into the unprofiled measurements.

E3: complete default and subset product regressions passed, including all 85
C records and known signatures. Tool option checks reject zero iterations and
unknown routes before parsing. go vet passed. Self-review found no new native
dependency, source-bearing diagnostic output, external download, global cache,
parser sharing or change to a completion gate. The tool has bounded fixed
inputs and per-parse timeouts. No second opinion was required for this additive
measurement tool and use of an existing build option.
