# Engine ABI v2 (session 15 queries)

Status: ABI 1 parsing contract plus the additive ABI 2 query implementation (Q1, 2026-11-14).
Inputs: ADR-0020, ADR-0021, `docs/design/api-v2.md`, `docs/design/query-abi.md`, the session-15 spike
(`.scratch/s15/w2/mem/combo`, shims `tsg_ext.c` and `tsg_export_s2.c`).

The ABI is the contract between three parts that are versioned separately:

| Part | Location | Owner |
|---|---|---|
| C shim (our own C, compiled into every grammar's wasm with unmodified upstream C) | `tools/gen/shim/tsg_abi.c` | generator |
| Go side of the ABI, memory provider, trap capture | `internal/engine` (root module) | engine |
| Per-grammar generated module | `grammars/<lang>/` (one Go module each) | generator output |

`ABIVersion = 2`. The root accepts ABI 1 for parsing and ABI 2 for parsing and queries. Any change to an export signature or semantics, the chunk layout or the meta block bumps it.

## 1. C exports (wasm reactor, one instance = one `TSParser`)

All pointers are wasm32 offsets (`int32` in Go). `TSTree*` handles are opaque. One parser per instance, created by
`tsg_init`; an instance may hold several trees.

| Export | Semantics |
|---|---|
| `uint32_t tsg_abi_version(void)` | returns `2` |
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
| `uint32_t tsg_export_err(void)` | `1` if the traversal disagreed with the descendant count |
| `uint8_t *tsg_export_buf(void)` | chunk buffer; valid from `tsg_init` on, header already written |
| `void tsg_export_end(void)` | abandon an export in progress (cancellation); idempotent |
| `uint8_t *tsg_lang_meta(void)` | malloc'ed language meta block (section 3); used by the generator, not on the parse path |
| `malloc`, `free` | wasi-libc, for source buffers and returned blocks |

Imports: module `tsg`, `should_stop() i32` (non-zero halts the parse). WASI preview1 imports are stubs returning
`52` (ENOSYS); the generator fails if a grammar needs an import outside `internal/engine`'s stub set.

Build flags (from the spike, fixed): `clang --target=wasm32-wasip1 -mexec-model=reactor -O2 -DNDEBUG
-Wl,--gc-sections -Wl,--no-entry -Wl,--strip-all` + `--export=` for the table above; then `wasm-opt -O3` (binaryen, explicit feature flags) and `wasm2go -unsafe -embed`
(data sections in `engine_gen.dat`, loaded with `go:embed`). Recipe name `v2-O3` in `tools/gen/pins.json`; chosen by the
pre-declared size round of 2026-11-14 (76.0 -> 25.6 MB raw, same conformance, speed within 1 %).

## 2. Export chunk buffer (snapshot block format 1)

Header, 16 bytes: `magic = "TSGX"` (4 bytes), `u16 format = 1`, `u16 reserved = 0`, `u32 KC` (chunk capacity, a
multiple of 4), `u32 reserved`. Columns follow the header (offsets relative to `buf+16`), as validated in the spike:
`symbol u16[KC] @0`, `field u16[KC] @2KC`, `flags u8[KC] @4KC`, `startByte @5KC`, `endByte @9KC`, `startRow @13KC`,
`startCol @17KC`, `endRow @21KC`, `endCol @25KC` (`u32[KC]` each), `parent i32[KC] @29KC` (global preorder index,
`-1` = root). Flags: `1` named, `2` extra, `4` missing, `8` error, `16` has_error. `symbol 65535` = ERROR.
The header is written by `tsg_init`; `NewParser` checks magic and format once (mismatch = `ErrIncompatibleLanguage`) and the Go side checks them again after every `tsg_export_begin`, and derives `descendantEnd`/`childCount`/siblings.

## 3. Language meta block (format 1)

`magic "TSGM"`, `u32 format = 1`, `u32 language_abi` (`ts_language_abi_version`), `u32 nSym`, `u32 nField` (ids
`0..field_count`, id 0 = ""), `u32 poolBytes`; then `symOff u32[nSym+1]`, `fldOff u32[nField+1]`,
`symFlags u8[nSym]` (`1` named, `2` visible, `4` supertype), then the name pool. The generator turns it into static
Go data; nothing at run time needs an instance to read names.

## 4. Go side (`internal/engine`)

```go
const ABIVersion = 2

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
func Call(fn func()) error                           // runs fn; a trap (any panic from generated code) becomes *Trap
```

Memory provider: reserve-then-commit, `VirtualAlloc` on Windows, `mmap` on Unix (standard library `syscall` first;
`golang.org/x/sys` only with a stated reason). The generated `memory_grow` body is replaced by
`return engine.MemoryGrow(mem, delta, max)` in a deterministic, sha256-checked step (expected original body hash
recorded in the generator; mismatch = generation failure).

Instance sequence (the `treesitter` package): `m := g.New(h)` → `SetLimit(m.Xmemory().Slice(), limit)` →
`engine.Call(m.X_initialize)` → `Xtsg_abi_version() == g.ABI` (accepted: 1 or 2) → `Xtsg_init() == 0` → export header magic/format; any failure =
`ErrIncompatibleLanguage` (version) or `EngineFault`. A trap with `Refused` set maps to `ResourceLimit`; any other trap
to `EngineFault`; both discard the instance (`Release`) and create a new one.

## 5. Grammar module layout

`grammars/<dir>/go.mod`: `module github.com/wotjr1649/go-treesitter/grammars/<dir>`, requires the root module (local
development: `replace github.com/wotjr1649/go-treesitter => ../..`). Wave 1 directories / packages: `csharp`, `tsql`,
`typescript`, `tsx`, `python`, `golang` (package `golang`), `json`. Files: generated `engine_gen.go` (wasm2go output,
patched), `language.go` (static `engine.Grammar` + `func Language() *treesitter.Language`), `LICENSE` of the grammar,
`provenance.json` (identity + hashes), `README.md`. Regenerating twice must give identical bytes.

`treesitter` exposes `func NewLanguage(g *engine.Grammar) *Language` for generated modules only (the argument type is
internal, so other modules cannot construct one).

## 6. Query additions (ABI 2)

ABI 1's `Module` interface and all export/snapshot layouts above stay unchanged. ABI 2 additionally implements
`engine.QueryModule`, an interface embedding `Module`; instance creation rejects ABI 2 without these exports.
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

`.github/workflows/balancing-cancel.yml` runs every generated grammar (currently 15 modules) nightly and manually on Linux amd64. Its separate
toolchain build and sweep cost stay outside PR-required checks; changing required checks remains a separate decision.
