# Engine ABI v4

Status: ABI 1 parsing contract, ABI 2 queries, ABI 3 snapshot field-lookup exceptions, and ABI 4 public symbol IDs
in language metadata (#77).
Inputs: ADR-0020, ADR-0021, `docs/design/api-v2.md`, `docs/design/query-abi.md`, the session-15 spike
(`.scratch/s15/w2/mem/combo`, maintainer-local archive 2026-11-16-s17-cleanup; shims `tsg_ext.c` and
`tsg_export_s2.c`, archive 2026-11-13-session-15 spike/).

The ABI is the contract between three parts that are versioned separately:

| Part | Location | Owner |
|---|---|---|
| C shim (our own C, compiled into every grammar's wasm with the unmodified runtime and pinned grammar C, including ADR-0023 patches) | `tools/gen/shim/tsg_abi.c` | generator |
| Go side of the ABI, memory provider, trap capture | `internal/engine` (root module) | engine |
| Per-grammar generated module | `grammars/<lang>/` (one Go module each) | generator output |

`ABIVersion = 4`. The root accepts ABI 1 for parsing and ABI 2–4 for parsing and queries. ABI 3/4 use snapshot format
2 for single-child field-lookup exceptions. ABI 1/2 use snapshot format 1 and direct-child lookup for alias-inherited
fields. ABI 4 adds language meta format 2. Any change to an export signature or semantics, the chunk layout or the
meta block bumps the engine ABI.

## 1. C exports (wasm reactor, one instance = one `TSParser`)

All pointers are wasm32 offsets (`int32` in Go). `TSTree*` handles are opaque. One parser per instance, created by
`tsg_init`; an instance may hold several trees.

| Export | Semantics |
|---|---|
| `uint32_t tsg_abi_version(void)` | returns `4` (legacy modules return `1`, `2` or `3`) |
| `int32_t tsg_init(void)` | create the parser, set the language, allocate the export chunk buffer and write its header (section 2); `0` ok, `-1` the runtime rejected the language (language ABI) or allocation failed |
| `TSTree *tsg_parse(const char *src, uint32_t len, TSTree *old)` | `ts_parser_parse_with_options`, UTF-8 input, progress callback = host `should_stop`; `old` may be `0`. **If the result is `NULL` the shim calls `ts_parser_reset` before returning.** |
| `void tsg_parser_reset(void)` | `ts_parser_reset` (host calls it on any no-tree path it creates itself) |
| `uint32_t tsg_root_end(TSTree *t)` | `ts_node_end_byte(root)` |
| `uint32_t tsg_root_has_error(TSTree *t)` | `ts_node_has_error(root)` as `0`/`1` (sets `Outcome` without an export) |
| `uint32_t tsg_descendant_count(TSTree *t)` | `ts_node_descendant_count(root)` (snapshot node cap is checked with this before export) |
| `void tsg_tree_edit(TSTree *t, uint32_t start_byte, old_end_byte, new_end_byte, start_row, start_col, old_end_row, old_end_col, new_end_row, new_end_col)` | `ts_tree_edit` |
| `void tsg_tree_delete(TSTree *t)` | `ts_tree_delete` |
| `uint8_t *tsg_changed_ranges(TSTree *old, TSTree *neu)` | malloc'ed block, never `NULL` (`count = 0` when nothing changed): `u32 count`, then `count` x `{u32 start_byte, end_byte, start_row, start_col, end_row, end_col}`; free with `free` |
| `uint32_t tsg_export_begin(TSTree *t)` | start a chunked preorder export; returns node count `n` (`0` = error) |
| `uint32_t tsg_export_next(void)` | fill the chunk buffer; returns nodes written (`0` = finished) |
| `uint32_t tsg_export_err(void)` | `1` on an interrupted or inconsistent traversal; the host checks context cancellation before interpreting it |
| `uint8_t *tsg_export_buf(void)` | chunk buffer; valid from `tsg_init` on, header already written |
| `void tsg_export_end(void)` | abandon an export in progress (cancellation); idempotent |
| `uint8_t *tsg_lang_meta(void)` | malloc'ed language meta block (section 3); used by the generator, not on the parse path |
| `malloc`, `free` | wasi-libc, for source buffers and returned blocks |

Imports: module `tsg`, `should_stop() i32` (non-zero halts the parse). WASI preview1 imports are stubs returning
`52` (ENOSYS), except `wasi_snapshot_preview1.proc_exit(code)`: it is a trap, never a process exit. `Host.Xproc_exit`
panics, the panic unwinds through generated code to `engine.Call` (`*Trap`), and the parser maps it as any other trap:
`EngineFault`, or `ResourceLimit` when a `memory.grow` was refused before it (a scanner's out-of-memory branch that
calls `exit(1)`), instance discarded and recreated; no code path reaches `os.Exit`. This is the meaning upstream's wasm store
gives it (`lib/src/wasm_store.c` links `proc_exit`, `abort` and `__assert_fail` to a trap). The generator fails if a
grammar needs an import outside `internal/engine`'s stub set (`checkImports`; `proc_exit` is in it).

Adding a host stub leaves the engine ABI unchanged. A module that imports `proc_exit` declares the method in its
generated import interface and requires a root with `Host.Xproc_exit` at compile time. ADR-0022 pins each released
module to the exact root version that supplies its imports.

Build flags (from the spike, fixed): `clang --target=wasm32-wasip1 -mexec-model=reactor -O2 -DNDEBUG
-Wl,--gc-sections -Wl,--no-entry -Wl,--strip-all` + `--export=` for the table above; then `wasm-opt -O3` (binaryen, explicit feature flags) and `wasm2go -unsafe -embed`
(data sections in `engine_gen.dat`, loaded with `go:embed`). Recipe name `v2-O3` in `tools/gen/pins.json`; chosen by the
pre-declared size round of 2026-11-14 (76.0 -> 25.6 MB raw, same conformance, speed within 1 %).

## 2. Export chunk buffer (snapshot block formats 1 and 2)

Header, 16 bytes: `magic = "TSGX"` (4 bytes), `u16 format` (`1` for ABI 1/2, `2` for ABI 3/4), `u16 reserved = 0`,
`u32 KC` (chunk capacity, a multiple of 4), `u32 fieldExceptions` (reserved in format 1; see below for format 2).
Columns follow the header (offsets relative to `buf+16`), as validated in the spike:
`symbol u16[KC] @0`, `field u16[KC] @2KC`, `flags u8[KC] @4KC`, `startByte @5KC`, `endByte @9KC`, `startRow @13KC`,
`startCol @17KC`, `endRow @21KC`, `endCol @25KC` (`u32[KC]` each), `parent i32[KC] @29KC` (global preorder index,
`-1` = root). Flags: `1` named, `2` extra, `4` missing, `8` error, `16` has_error. `symbol 65535` = ERROR.
The header is written by `tsg_init`; `NewParser` checks magic and format once (mismatch = `ErrIncompatibleLanguage`) and the Go side checks them again after every `tsg_export_begin` and nonempty chunk, and derives `descendantEnd`/siblings.

The Go snapshot retains 21 B/node (previously 25, and 41 before that), plus its owned source and sparse field exceptions.
It skips the four exported point columns without changing either block format or engine ABI 1-4.
Node points and point-range lookups derive LF-counted rows and byte columns from byte offsets. The snapshot's
existing `Lines()` index builds once with `sync.Once`, on first use: `[]uint32` line starts occupy exactly
`4 * (LF count + 1)` bytes, excluding slice/object/allocator overhead; there are at most `len(source) + 1` entries.
The source is shared, not copied again. `LineIndex.Pos` additionally builds its existing non-ASCII bitset lazily
(`8 * ceil(lines / 64)` bytes); byte-point lookups do not need it. These Go-heap allocations, like source/node
arrays and other snapshot caches, do not count against `WithMaxMemoryBytes`, which limits instance linear memory.

In format 2, header offset 12 is a wasm32 pointer to the current chunk's field-lookup exception block; `0` means
there are no exceptions in that chunk. The block is little endian: `u32 count`, then `count` records of
`{u32 nodeIndex, u32 fieldID, u32 targetIndex}`. Node and target indices are global visible preorder indices,
not chunk-relative. `targetIndex = UINT32_MAX` means C returned no node. The records are sorted by `(nodeIndex,
fieldID)` and contain only results that differ from the first direct child with that cursor field. The host copies
them into the immutable snapshot before requesting the next chunk and looks them up by binary search; no engine
pointer escapes. Format 1 supplies no exceptions and remains accepted for ABI 1/2 modules. No export or Go module
interface is added for ABI 3.

The shim uses the node's production field map to select field IDs, calls `ts_node_child_by_field_id` and maps its
result through the same preorder index machinery used for query captures. It reads the pinned runtime's private
language/subtree headers inside our shim; upstream C is unchanged. The extra table is sparse, but building it has
an export-time and engine-memory cost in addition to the copied snapshot records.

The export filter precomputes a per-production plan once per instance. It skips empty maps (`plan == 0`),
relying on the generator invariant below. For a nonempty map (`plan > 0`), only when it has no inherited entries,
no different field IDs at the same structural child index, and no decreasing child indices within a field,
it checks that every actual child is visible (a non-extra alias also makes a child visible) and that all mapped
structural indices exist.
For this case, pinned v0.27.2 `node.c:ts_node_child_by_field_id` returns the first non-extra child at a mapped
index; `tree_cursor.c:ts_tree_cursor_current_field_id` returns that child's unique non-inherited field. Extras
have no cursor field and are ignored by field lookup. With no hidden child, no inner field can precede or mask
that mapping. Thus no exception is needed for a field in that nonempty map. All other `plan > 0` nodes use the
unfiltered comparison, including every hidden child, even for a non-inherited map: `ts_node_child(child, 0)`
could return an extra or a descendant with a different innermost cursor field. Missing structural positions,
field collisions and decreasing per-field child indices also keep the comparison. This `plan > 0` compare/skip
decision is a sufficient condition independent of corpus observations. Empty-map skips and fields absent from
the production map instead rely on the generator invariant below. Neither ABI 3 nor format 2 changes.
The plan occupies two bytes per production and lives until the instance is released. Nodes that require a
comparison use one children cursor pass to collect each field's first direct child, reusing a per-instance
`TSNode[field_count + 1]` scratch array; fields in the current map are cleared before each pass. The C lookup,
exception ordering, lazy target index and output records are unchanged.

The generator assumptions behind the field map were inspected in the SHA-256-pinned v0.27.2 runtime archive
(the same release as the CLI pin). `crates/generate/src/build_tables/build_parse_table.rs:get_production_id`
enumerates steps in order, adds at most one direct `step.field()`, and propagates every hidden nonterminal's
field as inherited, including aliased steps. `node_types.rs:inherit_hidden_child_info` computes the transitive
hidden-field summary to a fixed point. A step's field is a single `Option<StrId>` (`grammars.rs` and
`prepare_grammar/flatten_grammar.rs`); `render.rs:add_field_sequences` stably sorts by field name, preserving
each field's increasing step indices and matching field-ID order. The runtime binds the reduce production ID
to the subtree (`parser.c`/`subtree.c`). The invariant needed for empty-map skips and fields absent from a map is
that every hidden child's field is propagated into its parent's production map as inherited; the source inspection
supports this for the pinned generator, not every possible upstream-generated `parser.c`. This filter argument and the
generator invariant describe the pinned v0.27.2 runtime and CLI; a runtime or CLI move must re-inspect and re-measure them. For `plan > 0`, the
filter additionally rejects field collisions, decreasing per-field child indices and absent positions at runtime,
and every hidden child remains in the comparison, including the non-inherited hidden-child case found in the 2026-11-15 independent review of the field-lookup ABI, whose first
visible descendant is an extra.

`ChildByFieldName` consults this table first, then falls back to the direct-child rule. The table records whatever the
module's pinned runtime returns, so a C result need not be a direct child (with the v0.27.x runtimes it can lie inside
a visible alias) and the recorded results follow the runtime, not the root. `ChildrenByFieldName` yields an exception target first, followed
by matching direct children in their original order, omitting a duplicate target; an explicit no-node exception
yields nothing. Without an exception it yields the matching direct children as before. Both methods return
nothing on ERROR nodes, unknown field names and the zero `Node`, for all supported ABIs. Cursor fields exported
in the columns above are unchanged, so `Node.Field()` and field lookup can describe different relationships.
The iterator's first element equals `ChildByFieldName`; `ChildrenByFieldName` is not a C-verified API because
v0.27.2 has no children-by-field API. Its ordering and explicit-null behavior have E3 retained tests.

### Field-lookup evidence

Historical E5 native agreement for single-child lookup (2026-11-15 field-export-cost run) covers fresh parses of the reproducible pinned
1,761-input corpus on Windows amd64 (`tools/native-ref/ciconf`, seed 1). That run also checked 2,779 additional local
inputs; they are recorded separately and are not part of this reproducible-corpus E5 claim.
The evidence archive for that run (maintainer-local `2026-11-15-q13-field-export-cost`, not in this repository) binds the result to base
`35233d365418147ee8cac8bd7c54f5f617bfe753`, T13 `9531f2e5aa7b1cef78fc140a86230a17ab7c6e20`,
pins SHA-256 `f67b89640c3880226aeec44bbe44f135b9017ba193241168cd88ec71f25d7571` and the Windows platform.
Its `q13/generation-identity.json` records module/native identities, `q13/differential-jobs.json` lists input hashes,
and `q12-prerequisite/` preserves the input archives. The 2026-11-15 wave-2 batch 2 ABI 3 run reran the field-export cost-filter guard (independently reviewed 2026-11-15) on the same reproducible corpus with E5 agreement. Its ABI-v3 evidence archive (maintainer-local `2026-11-15-q14-batch2`) records the new shim/pins hashes, generation and test commands, exact differential arguments and input hashes.
E5 native agreement from the 2026-11-15 edited-tree field-lookup run extends ABI 3 `ChildByFieldName` to all 5,283 seeded incremental edits of that
reproducible 15-grammar corpus (seed 1) on Windows amd64, plus the five separately retained T-SQL
`knownIncrementalDiffs` cases. `TestFieldLookupEdits` compares with C's **incremental** tree, never a fresh parse,
and checks that `ChildrenByFieldName` has the same first result (or no result). This is not a C oracle for the
iterator's remaining elements or arbitrary inputs. CSS and HTML have no field names; their edit headers are
compared, with zero field-result rows. That run's report and identities (maintainer-local archive `2026-11-15-q16-edited-fields`)
bind the result to base `2a1d01d19f4e9be7bc49425f467fd464e56f9629`, unchanged pins SHA-256
`53fca7937e25f52437925aa4a58f5a249171cd0fbaad5fdfc79bffb8c1477ac9`, input manifests,
the native driver and executable hashes, and the delivered patch/index tree. The existing conformance shard's
`TestFieldLookup` selector includes `TestFieldLookupEdits`; `TSG_REQUIRE_FIELD_LOOKUP=1` makes missing native
results, missing data and zero selected edit cases fail. Older local data may use the native `edit-fields` command
directly; new `ciconf` data records each `.fields` result's exit code and SHA-256 in `edits/results-native.csv`.

## 3. Language meta block (formats 1 and 2)

`magic "TSGM"`, `u32 format = 2`, `u32 language_abi` (`ts_language_abi_version`), `u32 nSym`, `u32 nField` (ids
`0..field_count`, id 0 = ""), `u32 poolBytes`; then `symOff u32[nSym+1]`, `fldOff u32[nField+1]`,
`publicSymbols u16[nSym]`, `symFlags u8[nSym]` (`1` named, `2` visible, `4` supertype), then the name pool.
Each public symbol entry is `ts_language_public_symbol(lang, rawID)`. The flags are the underlying
`ts_language_symbol_metadata` bits. The generator emits these tables as static Go data. ABI 1–3 used format 1,
which has no public symbol column. Their Go metadata leaves `PublicSymbols` nil, so the root retains raw ID lookup
for those modules.

## 4. Go side (`internal/engine`)

```go
const ABIVersion = 4

type Memory = interface{ Slice() *[]byte; Grow(delta, max int64) int64 } // alias: identical to wasm2go's type

type Module interface { // implemented by the generated *Module
    X_initialize(); Xmemory() Memory; Xmalloc(int32) int32; Xfree(int32)
    Xtsg_abi_version() int32; Xtsg_init() int32
    Xtsg_parse(src, n, old int32) int32; Xtsg_parser_reset()
    Xtsg_root_end(t int32) int32; Xtsg_root_has_error(t int32) int32; Xtsg_descendant_count(t int32) int32
    Xtsg_tree_edit(t, sb, oeb, neb, sr, sc, oer, oec, ner, nec int32); Xtsg_tree_delete(t int32)
    Xtsg_changed_ranges(old, neu int32) int32
    Xtsg_export_begin(t int32) int32; Xtsg_export_next() int32; Xtsg_export_err() int32
    Xtsg_export_buf() int32; Xtsg_export_end()
}

type Host struct{ Stop atomic.Int32 } // implements the tsg and WASI import interfaces

type Grammar struct {
    ABI      uint32               // literal ABI version the module was generated for
    Name     string
    New      func(h *Host) Module // generated: return New(h, h)
    Meta     LangMeta             // static
    Identity Identity             // runtime release, grammar repo+commit, source sha256s, recipe sha256, tool versions
}

func MemoryGrow(mem *[]byte, delta, max int64) int64 // replaces generated memory_grow (same contract)
func SetLimit(mem *[]byte, bytes int64) error        // per instance; before X_initialize; out of range = error
func Refused(mem *[]byte) bool                       // a grow was refused since the last ClearRefused
func ClearRefused(mem *[]byte)
func Committed(mem *[]byte) int64                    // for WithRecycleAboveBytes
func Release(mem *[]byte)                            // return the region to the OS; idempotent
func InitMemory(mem *[]byte, size int, segs ...Segment) // records an initial memory (no copy); for a generated New that calls it
func StartMemory(mem *[]byte)                        // creates the region and writes the segments once; first statement of X_initialize
func Call(fn func()) error                           // runs fn; a trap (any panic from generated code) becomes *Trap
```

Memory provider: reserve-then-commit, `VirtualAlloc` on Windows, `mmap` on Unix (standard library `syscall` first;
`golang.org/x/sys` only with a stated reason). The generated `memory_grow` body is replaced by
`return engine.MemoryGrow(mem, delta, max)` in a deterministic, sha256-checked step (expected original body hash
recorded in the generator; mismatch = generation failure). `InitMemory`/`StartMemory` are additive provider API for a
generator step that copies the data segments once, straight into the reserved region (after `SetLimit`, with no
initial-size Go-heap slice); no generated module calls them yet, and `MemoryGrow` keeps its Go-heap-slice path, so modules
generated before this API still work unchanged. `StartMemory` does nothing when no `InitMemory` is pending; an initial
size above the limit traps with `Refused` set (a refused grow).

Instance sequence (the `treesitter` package): `m := g.New(h)` → `SetLimit(m.Xmemory().Slice(), limit)` →
`engine.Call(m.X_initialize)` → `Xtsg_abi_version() == g.ABI` (accepted: 1–4) → `Xtsg_init() == 0` → export header magic/format (format 1 for ABI 1/2, format 2 for ABI 3/4); any failure =
`ErrIncompatibleLanguage` (version) or `EngineFault`. A trap with `Refused` set maps to `ResourceLimit`; any other trap
to `EngineFault`; both discard the instance (`Release`) and create a new one.

## 5. Grammar module layout

`grammars/<dir>/go.mod`: `module github.com/wotjr1649/go-treesitter/grammars/<dir>`, requires the root module (local
development: `replace github.com/wotjr1649/go-treesitter => ../..`). The layout applies to every entry in `tools/gen/pins.json`. Files: generated `engine_gen.go` (wasm2go output,
patched), `language.go` (static `engine.Grammar` + `func Language() *treesitter.Language`), `LICENSE` of the grammar,
`provenance.json` (identity + hashes), `README.md`. Regenerating twice must give identical bytes.

`treesitter` exposes `func NewLanguage(g *engine.Grammar) *Language` for generated modules only (the argument type is
internal, so other modules cannot construct one).

## 6. Query additions (ABI 2)

ABI 1's `Module` interface stays unchanged. ABI 2 additionally implements `engine.QueryModule`, an interface
embedding `Module`. Instance creation rejects ABI 2–4 modules without these exports. ABI 2–4 use the same query
block formats.
This separate extension is required so old grammar modules still compile against the new root.

| C export (wasm32 opaque handles) | Go method | Meaning |
|---|---|---|
| `tsg_query_new(src, len) -> q` | `Xtsg_query_new(src, n int32) int32` | Compile for the instance language; NULL on query error |
| `tsg_query_error_offset() -> u32` | `Xtsg_query_error_offset() int32` | Last compilation error byte offset |
| `tsg_query_error_type() -> u32` | `Xtsg_query_error_type() int32` | C TSQueryError (1 syntax, 2 node type, 3 field, 4 capture, 5 structure, 6 language) |
| `tsg_query_delete(q)` | `Xtsg_query_delete(q int32)` | Free query |
| `tsg_query_meta(q) -> ptr` | `Xtsg_query_meta(q int32) int32` | Allocated metadata block; host frees it |
| `tsg_qcursor_new() -> c` | `Xtsg_qcursor_new() int32` | Allocate opaque shim cursor wrapping TSQueryCursor, persistent callback options and match buffer |
| `tsg_qcursor_delete(c)` | `Xtsg_qcursor_delete(c int32)` | Free cursor and buffer |
| `tsg_qcursor_exec(c,q,t,start,end,limit) -> i32` | `Xtsg_qcursor_exec(c,q,t,start,end,limit int32) int32` | Root execution with byte intersection range and host should_stop callback; 0 success, -1 invalid range; limit 0 uses UINT32_MAX |
| `tsg_qcursor_next(c) -> u32` | `Xtsg_qcursor_next(c int32) int32` | Fill buffer, return match count (currently capacity 1); 0 means done/halted |
| `tsg_qcursor_halted(c) -> u32` | `Xtsg_qcursor_halted(c int32) int32` | Progress callback halted execution |
| `tsg_qcursor_exceeded(c) -> u32` | `Xtsg_qcursor_exceeded(c int32) int32` | C did_exceed_match_limit |
| `tsg_qcursor_buf(c) -> ptr` | `Xtsg_qcursor_buf(c int32) int32` | Current buffer; read only after a nonzero next result |

All words below are little endian u32; no alignment padding. Query meta header: `"TSGQ", format=1, totalBytes,
patternCount, captureCount, stringCount` (24 bytes). Capture names then literal strings, each `{byteLength, bytes}`;
then each pattern `{startByte, stepCount, stepCount * {TSQueryPredicateStepType, valueID}}`. The host validates all
lengths, counts, step types and capture/string IDs before evaluation. Type 0 terminates one predicate.

Match buffer header (16 bytes): `"TSGM", format=1, matchCapacity, payloadBytes`. Each match is `{patternIndex,
captureCount}`, followed by captures `{captureIndex, nodeIndex, symbol, startByte, endByte}` (20 bytes each).
`nodeIndex` is read from a cursor-owned hash table keyed by TSNode ID (the identity used by ts_node_eq within a
single tree). On the first returned match, a single preorder traversal records ts_tree_cursor_current_descendant_index
for every node. Subsequent forward or backward captures use expected O(1) lookups; building the table polls should_stop
every 256 nodes. A missing ID yields UINT32_MAX, which the host rejects. The table has power-of-two capacity at least
twice the node count and uses 8 bytes per slot on wasm32 (fewer than 32 bytes per node). Exec retains the table for
the same tree and frees it when switching trees. The shim tracks cursors and frees their tables before deleting a
referenced tree, clearing the tree pointer so a reused allocator address cannot preserve stale indices. An interrupted
build frees its partial table. Cursor deletion or release of the entire instance also releases the table.
Symbol and byte range are included so the host can validate the mapping against its snapshot, including zero-width
nodes. This extends the abbreviated two-word capture in the design draft.

The match buffer currently holds one match and grows to fit its captures; each next call replaces it. The host copies
records into Go values before yielding; no engine pointer or memory slice escapes. The cursor options live in the
shim cursor (never on a returned C stack frame). Allocation failures abort and become host traps.

The root validates NewQuery using a scratch instance. Queries compile once per parser instance. The cache uses weak Query keys; runtime cleanup only queues a retirement
with weak owner/instance identities. The next owner engine operation deletes that TSQuery, ignoring entries for an
already replaced instance. The cache is also released with the instance. Matches holds the existing busy guard through snapshot export, compilation, streaming,
predicate filtering; break/panic releases it. The host lazily allocates one cursor per instance and re-execs it for
each Matches call, including different queries; the next exec resets C's match state and options. Context cancellation retains the instance, while
refused growth (even without a trap) becomes ResourceLimit with instance reset; other traps, malformed blocks and
capture mapping mismatches become EngineFault with instance reset. A match-limit error is ErrQueryMatchLimit and
retains the instance. Reset/Close and traps release the entire instance with its cursor. Go predicate validation
errors use QueryError type 7 and the pattern start byte.

## Test-only balancing-cancel lane

`tools/gen -balance-probe` builds a separate instrumented module into a new temporary output directory outside
`grammars/`; it cannot emit a release dependency form. `tools/gen/probe/balance.c` includes the unchanged pinned
`lib.c` and product `tsg_abi.c` in one translation unit. Only the shim's calls to `ts_parser_reset` are redirected to
an observation wrapper: it records `canceled_balancing` before calling the real reset. Five extra exports expose the
live flag, last pre-reset flag, reset count, observation clear and a deliberately no-reset parse for negative controls.
These exports are not part of the product engine ABI. The probe uses the same recipe and memory_grow patch, carries
`testOnly: "balancing-cancel"` provenance, and binds its wrapper hash and exports to its distinct recipe identity.

`tools/conformance`'s `TestBalancingCancel` runs the generated tests through `balanceprobe`. A custom test host stops
at the kth progress callback; it counts an uncancelled baseline, locates the balancing boundary and samples the full
parse, boundary and tail (including callbacks beyond completion). Each trial is followed on the same instance by a
different input whose dump must equal the native reference. Every grammar must observe at least one balancing stop.
JSON additionally requires the no-reset negative control to produce a mismatching next parse, then recover correctly.
The test artifacts retain per-trial results and source/native identities. This is evidence for the sampled probe
builds and inputs, not direct observation inside product module bytes or an exhaustive sweep of every callback.

`.github/workflows/balancing-cancel.yml` runs every generated grammar (currently 25 modules, including `swift`) nightly and manually on Linux amd64. Its separate
toolchain build and sweep cost stay outside PR-required checks; changing required checks remains a separate decision.
