# AST lookup implementation and measurement

The user selected node position/type lookup. An optional syntax.Index copies
offsets, parent-derived depth and snapshot IDs into an immutable interval array
and a type map. It retains no source or backend handle. NodeAt uses half-open
ranges, excludes zero-width nodes/EOF, and breaks equal-depth overlap ties by
preorder. OfType yields IDs without making a result slice. Rebuild after edits.

E3: malformed ranges/parents, adjacent boundaries, overlap ties, zero-width
nodes, early iterator stop, empty snapshots and caller mutation have retained
tests. All byte positions of all 50 registered snapshots matched a simple
linear reference walk, and type IDs matched the snapshot order. Four readers
shared the immutable index. The separate race run passed. The external consumer
also exercised both lookup APIs. regression-01 and vet-01 passed in the CGO-free
Windows/amd64 product lane, including original C records and known signatures.

E4: one executable compared linear scans with the index, using the same parsed
Go snapshot of 2,048 declarations. Five fixed samples of 200 iterations per
operation were declared before measurement. GOMAXPROCS=1, GOMEMLIMIT=512MiB,
Go 1.27.1, Intel Core Ultra 9 285H. No measurement was repeated to improve it.
Source and implementation hashes plus all samples are in measurements.json.

| Operation | Median | Range across five samples | Allocation per operation |
|---|---:|---:|---:|
| Build index | 871,842 ns | 831,409–973,574 ns | 888,320 bytes, 109 allocations |
| Linear position | 13,626 ns | 13,280–15,295 ns | 0 |
| Indexed position | 520 ns | 477.5–594.5 ns | 0 |
| Linear type | 13,256 ns | 12,987–14,942 ns | 0 |
| Indexed type | 5,566 ns | 5,196–6,242 ns | 0 |

For this workload, median position lookup is about 26.2 times faster and type
lookup about 2.38 times faster. Including construction, position queries need
roughly 67 repetitions to amortize that cost under these measurements. The
build allocation figure is total allocated bytes, not retained heap/RSS. This
does not describe arbitrary syntax shapes: heavily overlapping ranges can still
take O(n) position work, and a few queries may be cheaper as direct scans.

Independent review found no defect in range pruning, tie semantics, validation,
ownership or concurrent-read behavior. Final self-review found no upstream type
leak, global state, semantic resolution or unbounded recursion (tree traversal
is balanced). No new dependency, grammar/epoch change or gate change. Fixtures
are existing registered inputs plus repository-authored benchmark generation.
