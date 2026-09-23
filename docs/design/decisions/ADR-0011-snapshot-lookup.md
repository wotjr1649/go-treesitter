# ADR-0011 — Optional snapshot lookup

Status: Accepted, 2026-09-22, Session 07.

The user selected AST position/type lookup as the search workload. Add an
optional immutable index over own snapshot values. A type map yields original
indexes in preorder; an array of byte intervals prunes disjoint ranges for
position lookup. No backend query engine, source retention, global cache or
language semantics is introduced.

NodeAt selects the deepest half-open containing range, breaking equal-depth
overlap ties by preorder. Zero-width nodes and EOF are excluded. Constructor
validation rejects reversed ranges, forward/cyclic parent links and child
ranges outside parents. Construction copies index values; it does not retain
the input slice. An edit requires a new index. Concurrent readers share only
the immutable index, not the parser/tree.

Construction uses O(n log n) time and O(n) extra space. Type lookup takes
O(matches); position lookup can still visit O(n) overlapping ranges. This is
optional because few queries may cost less as a direct scan. The retained
benchmark reports construction cost, query time and allocations separately.
No runtime, grammar, oracle epoch or language scope changes.
