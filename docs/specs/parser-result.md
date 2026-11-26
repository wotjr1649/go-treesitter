# Parser result and consumer contract

Normative for the `treesitter` package (API v2, ADR-0021). Signatures and rationale: `docs/design/api-v2.md`; engine
side: `docs/design/engine-abi.md`. Where this document and the code disagree, the code is a defect or this document
is; fix one of them, never leave both.

The old `syntax` package, `treesitter.New` and `internal/gtsadapter` were removed (ADR-0022). Their contract is in Git
history at commit `9ad82eb`; nothing below applies to them.

## The traps this contract prevents

1. **A tree that looks complete but is not.** `Parse` returns a non-nil `*Tree` only for a complete tree, and every
   accepted tree is checked: root end == input length, otherwise `EngineFault`.
2. **A halted C parse resuming on the next call.** Every path that returns no tree leaves the C parser reset
   (`ts_parser_reset`) or the instance recreated. Required regression: cancel a parse, then parse a different source
   and require that source's own tree (`tools/conformance` `TestCancelThenReparse`).
3. **Using memory that belongs to a parser after it moved on.** Trees are confined to their parser and invalidated by
   generation; snapshots own their data and never point into the engine.

## Outcome vocabulary (closed set)

| Outcome | Tree | Go error | Instance afterwards |
|---|---|---|---|
| `AcceptedClean` | yes | `nil` | kept |
| `AcceptedWithErrors` (ERROR or MISSING nodes; root `has_error`) | yes | `nil` — syntax errors are not Go errors | kept |
| `Timeout` (ctx deadline) | no | `*ParseError`, `errors.Is` `ErrTimeout` and `context.DeadlineExceeded` | kept, parser reset |
| `Cancelled` (ctx cancel) | no | `*ParseError`, `errors.Is` `ErrCancelled` and `context.Canceled` | kept, parser reset |
| `ResourceLimit` — input over `WithMaxInputBytes`, snapshot over `WithMaxSnapshotNodes` (checked with the descendant count before export) | no | `ErrResourceLimit` | kept |
| `ResourceLimit` — a refused `memory.grow` during the call, even if a tree came back | no | `ErrResourceLimit` | recreated (`InstanceReset`) |
| `InvalidInput` — nil ctx or option, invalid option value (never clamped), `old` from another parser or invalidated, non-UTF-8 with `WithStrictUTF8` | no | `ErrInvalidInput` | kept |
| `EngineFault` — any other trap (including WASI `proc_exit`, which is a trap and never exits the process), root-end mismatch, an ABI violation by the module | no | `ErrEngineFault` | recreated (`InstanceReset`) |
| `IncompatibleLanguage` — engine ABI version or export-block magic/format mismatch | no parser | `ErrIncompatibleLanguage` | none created |

Adding a member is a contract change and needs an ADR. Lifetime errors are separate sentinels: `ErrTreeInvalidated`,
`ErrTreeEdited`, `ErrParserClosed`.

## Required evaluation order

nil or already-done `ctx` → option and input checks (size, strict UTF-8, ownership and validity of `old`) → engine.
Option and input checks never touch the instance. Before them, the parser may do internal housekeeping that
does not depend on the call's input: it recreates an instance discarded by an earlier fault and deletes engine
queries whose `Query` values were garbage-collected (decided 2026-11-25). A fault during that housekeeping is
returned with its usual outcome. Cancellation during the engine call is detected in the progress
callback; the call returns after the C parser's cleanup.

`ParseSnapshot` checks cancellation before copying input into the engine and before copying the snapshot's source.
The descendant count and `WithMaxSnapshotNodes` limit are checked before the snapshot source and node columns are allocated.

## Query results and limits

`NewQueryWithOptions` and `Tree.MatchesWithOptions` use the existing error outcomes.
A malformed query returns `InvalidInput` with a `*QueryError` available through `errors.As`.
`NewQuery` returns that query error directly. Context cancellation or expiration uses `Cancelled`
or `Timeout`. Go-side budget exhaustion returns `ResourceLimit` with `InstanceReset == false`.
Engine growth refusal can require an instance reset. The cursor's in-progress `MatchLimit`
reports `ErrQueryMatchLimit` and discards any result produced by the same engine call.

A stopped match sequence emits at most one terminal error and releases the parser's busy guard.
Previously yielded capture nodes retain their snapshots. An iteration that ends cleanly, stops
on a Go limit, observes cooperative cancellation, or breaks early leaves the live tree reusable.
See the [query contract](../design/query-abi.md#opt-in-go-budgets) for enforcement points.

## Input

Bytes are parsed like C tree-sitter (UTF-8; a leading BOM is skipped; invalid bytes become decode errors in the
tree). `HasInvalidUTF8` reports `!utf8.Valid(src)`. `NewSource`/`NewSourceUTF16` convert UTF-16 or BOM-prefixed input
to UTF-8 and map offsets back to the original bytes. `NewSourceCP949` does the same for CP949 (the WHATWG `EUC-KR`
index = windows-949); `NewSource` never detects it. A conversion writes U+FFFD for invalid input, and a U+FFFD is valid
UTF-8: neither `HasInvalidUTF8` nor `WithStrictUTF8` reports it, so `Source.Replacements()` is the only count of them
(0 for UTF-8 input, whose invalid bytes `HasInvalidUTF8` does report). `Source.UTF8Offset` is the inverse of
`OriginalOffset`; for converted sources (UTF-16, CP949) both floor an offset inside a character to its start, while a
UTF-8 source is not converted and both return the offset unchanged (clamped). See the
[Korean and CP949 input guide](../guides/korean-input.md).

## Ownership, lifetime, concurrency

- A `Parser` and its `Tree`s are one confinement unit: use them from one goroutine at a time. Operations that need
  the parser's busy guard, including cached `Tree.Snapshot`, reject overlapping operations with `ErrInvalidInput`
  when the guard is held. `Tree.Close` marks the tree closed once and queues its handle if the guard is held.
- `Tree.Matches` applies `QueryOptions.MatchLimit` to in-progress states on each call and reports
  `ErrQueryMatchLimit` on overflow. See the [query contract](../design/query-abi.md) for cursor and memory lifetimes.
- `Close`, `Reset`, an instance reset and `ParserPool.Put` invalidate the parser's live trees (`ErrTreeInvalidated`).
- `NewParserPool` copies its options and rejects a nonzero memory limit below the engine minimum (32 MiB).
  Closing a pool wakes waiting `Get` calls with `ErrParserClosed` and prevents a parser still being created from
  being lent. A parser already lent is closed when returned with `Put`.
- `Tree.Edit` mutates in place (C semantics); the tree is then only valid as `old` of the next parse, and
  `Snapshot` returns `ErrTreeEdited`. Take snapshots before editing.
- A leaked tree or parser is cleaned up by queueing; only the owning goroutine calls into the engine.
- `Snapshot`, `Node`, `Index`, `LineIndex` are immutable and safe for any number of goroutines. A snapshot owns its
  source and stays valid after the parser is closed. `ParseOwned` transfers ownership of the input bytes to avoid
  a copy. The caller must never modify them again, including through any alias. `Node.Text` is a zero-allocation
  substring of that source.
- `Source.Bytes` and `Source.Original` return read-only views. The caller must leave their bytes and the original
  input unchanged, including through any alias.
- Snapshot arrays retain 21 B/node (previously 25, and 41 before that), plus sparse field exceptions and source. Node points derive
  LF-counted rows and byte columns from source offsets using a shared, lazy `sync.Once` line index:
  `[]uint32` line starts cost `4 * (LF count + 1)` bytes plus fixed object/allocator overhead, with at most
  `len(source) + 1` entries and no extra source copy. Only `LineIndex.Pos` adds its lazy non-ASCII bitset
  (`8 * ceil(lines / 64)` bytes). Snapshot/source/cache Go memory is outside `WithMaxMemoryBytes`, whose
  instance wasm linear-memory semantics are unchanged.
- `Tree.Edit` validates byte bounds and point ordering only. Points inconsistent with bytes are passed to C
  unchanged. Snapshot node points and `ChangedRanges` points derive from the result
  source instead; changed-range offsets beyond its end use the EOF point. `EditFor` computes consistent points
  from bytes. For example, inserting LF at byte 3 into `[1,2,3]\n` with all edit points zero can leave C
  changed range `[3,4)` ending at `0:4`; this API derives `1:0`. Query captures store byte offsets and node
  indices, with no point columns.
- The zero `Node` is C's null node (`IsZero`). Every method on it returns its zero value (false, 0, `""`, the zero `Node`, an
  empty iterator, `(Node{}, false)` for the `(Node, bool)` methods) and none panics.

## Field lookup and grammar compatibility

The root accepts engine ABI 1-4 grammar modules. ABI 1 supports parsing, ABI 2 adds queries, ABI 3 adds snapshot
field-lookup exceptions and ABI 4 adds public symbol IDs to language metadata (`SymbolForName`). With ABI 3 or 4, `Node.ChildByFieldName` follows the `ts_node_child_by_field_id` of the grammar module's pinned
runtime (`identity.Runtime` in its `provenance.json`), which can return a descendant rather than a direct child.
With the v0.27.x runtimes an inherited field can resolve inside a visible alias; the contract does not promise that
shape, and a module on another runtime can resolve such a field differently. For a given tree the result is thus
fixed by the grammar module version, not by the root version. The snapshot owns the exception table; lookup remains
valid after parser closure and never calls into the engine.

ABI 1/2 snapshots have no exception table: `ChildByFieldName` keeps the first direct child with the requested
cursor field, which can differ from C on alias-inherited fields. Regenerate or upgrade the grammar module to get
ABI 3 semantics. `ChildByFieldName` and `ChildrenByFieldName` return nothing on ERROR nodes, unknown field names
and the zero `Node` for every supported ABI. `Node.Field()` still reports cursor fields, including those attached
to ordinary visible children; it makes no promise of a nonempty field on children of ERROR nodes. The lookup rule
does not change the exported tree or cursor metadata.

`ChildrenByFieldName` yields a recorded C target first, then matching direct children in order without duplicating
that target. A recorded C no-node result yields nothing; without an exception it retains the direct-child
iteration. Its first result is `ChildByFieldName`, even when that result is not a direct child.
This iterator is not a C-verified API: v0.27.2 has no children-by-field API. Its ordering and explicit-null behavior
are Go API semantics covered by retained tests (E3).

Historical E5 native C agreement for `ChildByFieldName` (2026-11-15 field-export-cost run) covers fresh parses of the reproducible pinned
1,761-input corpus on Windows amd64. That run also checked 2,779 additional local inputs, recorded separately in the
[archived evidence and identities](../design/engine-abi.md#field-lookup-evidence). The 2026-11-15 wave-2 batch 2 ABI 3 run reran the field-export cost-filter guard (independently reviewed 2026-11-15) on the same reproducible corpus with E5 agreement; its input and source identities are archived with that evidence.
The 2026-11-15 edited-tree field-lookup run extends E5 agreement on Windows amd64 to all 5,283 reproducible seeded incremental edits of the 15 grammars
and five retained T-SQL incremental-difference cases. `TestFieldLookupEdits` compares `ChildByFieldName` with C's incremental tree and checks that
`ChildrenByFieldName`'s first result (including no result) equals it, including cases where C's
incremental and fresh parses differ. The [scope and identities](../design/engine-abi.md#field-lookup-evidence)
do not cover the iterator's remaining elements or arbitrary inputs.

## Navigation differs from C at zero-width nodes

`Snapshot.DescendantForByteRange`, `DescendantForPointRange`, `Node.NextSibling` and `Node.NextNamedSibling` are
not claimed equal to C's `ts_node_(named_)descendant_for_{byte,point}_range` and `ts_node_(next_)(named_)sibling`.
The difference is structural: a snapshot holds visible nodes only, while C navigates the full tree, including hidden
nodes (zero-width scanner marker tokens, hidden container rules). Two cases follow, both at zero-width positions:

- A zero-width range at a node boundary: C can stop at a hidden zero-width leaf, so it returns an ancestor of the
  snapshot's result, or it steps over a zero-width visible node (for example MISSING) at the end of a hidden
  container where the snapshot returns that node.
- `NextSibling` / `NextNamedSibling`: C skips a zero-width sibling directly after the node but returns it from
  `ts_node_prev_sibling`. The snapshot keeps next and prev inverse to each other and includes the zero-width sibling.

This subsection records what is known and its level (`docs/specs/validation.md`): E1, a reading of C v0.27.2 `node.c` and
of grammar scanner sources; and the report-only navigation-comparison data of the open #156 CI run, which compared 8,463
pinned-corpus and kit inputs and showed differing cells only at zero-width nodes (10,890 cells, none in `parent`,
`prev` or `prevNamed`: no difference observed there, not a guarantee). That comparison is not a retained gate and no E3-E5 level is claimed. C's hidden zero-width
leaves are not exported by the engine, so the hidden-leaf cause is inferred from source, not observed. Arbitrary
ranges, edited trees and `DescendantForPointRange` are not covered by that data. Code that needs C's exact result at a
zero-width position must not rely on these methods.

## Comparison discipline

- C agreement means byte-identical canonical dumps against a native C build of the same runtime release and grammar
  sources (`docs/specs/oracle.md`). It holds for the corpus it was measured on, not for arbitrary input.
- `incremental == fresh` is self-consistency, not C agreement, and is not guaranteed (upstream behavior).
- `ChangedRanges` is compared with native `ts_tree_get_changed_ranges`; an empty result for a changed dump is valid C
  behavior.
- Result evidence is bound to the identity in `docs/specs/baseline-provenance.md`; it is not carried to another runtime
  release, grammar commit, toolchain, recipe or platform.

## Scope boundary

Syntax only: parse trees, positions, hashes and structural matching over one grammar. No semantic resolution, code
graph, persistence or application logic. Queries arrive in v0.1 with the signatures fixed in api-v2 §8.
