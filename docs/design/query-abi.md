# Queries (v0.1): engine ABI v2 and Go API — design draft

Status: draft by the main session, 2026-11-14. Inputs: `docs/design/api-v2.md` §8 (signatures fixed), `engine-abi.md`
(ABI v1), tree-sitter v0.27.2 `api.h` (query and query-cursor API, `ts_query_cursor_exec_with_options` progress
callback, `ts_tree_cursor_current_descendant_index`).

## Goals and non-goals
- Run the unmodified C query engine inside the instance; map every capture to a snapshot `Node` exactly.
- Evaluate text predicates in Go against the snapshot's owned source (C does not evaluate them).
- Non-goals for v0.1: query-level caching across parsers, `#set!`/`#is?` semantics beyond exposing them as properties.

## ABI v2 (additions only; every v1 export keeps its meaning)
`ABIVersion = 2`. The root accepts grammar modules built for ABI 1 **and** 2; queries need 2 (`ErrQueryUnsupported`
otherwise), so grammar `v0.1.0` modules keep working with a newer root.

| Export | Semantics |
|---|---|
| `TSQuery *tsg_query_new(const char *src, uint32_t len)` | `ts_query_new` for the instance's language; `0` on error |
| `uint32_t tsg_query_error_offset(void)`, `uint32_t tsg_query_error_type(void)` | error of the last failed `tsg_query_new` |
| `void tsg_query_delete(TSQuery *q)` | |
| `uint8_t *tsg_query_meta(TSQuery *q)` | malloc'ed block `"TSGQ"`, format 1: pattern, capture and string counts; capture names; string values; per pattern its start byte and predicate steps (`ts_query_predicates_for_pattern`); free with `free` |
| `TSQueryCursor *tsg_qcursor_new(void)`, `void tsg_qcursor_delete(TSQueryCursor *c)` | |
| `int32_t tsg_qcursor_exec(TSQueryCursor *c, TSQuery *q, TSTree *t, uint32_t start_byte, uint32_t end_byte, uint32_t match_limit)` | `ts_query_cursor_exec_with_options` on the root; progress callback = host `should_stop`; `0` ok |
| `uint32_t tsg_qcursor_next(TSQueryCursor *c)` | fill the match buffer with up to its capacity of matches; returns the count (`0` = done or halted; halted is reported by `tsg_qcursor_halted`) |
| `uint32_t tsg_qcursor_halted(TSQueryCursor *c)`, `uint32_t tsg_qcursor_exceeded(TSQueryCursor *c)` | progress-callback halt; `ts_query_cursor_did_exceed_match_limit` |
| `uint8_t *tsg_qcursor_buf(TSQueryCursor *c)` | match buffer, header `"TSGM"` format 1 + capacity |

Match record: `u32 pattern_index, u32 capture_count`, then `capture_count` x `{u32 capture_index, u32 node_index}`.
`node_index` is the node's **preorder descendant index** (the snapshot index), computed in C by descending a tree cursor
from the root to the captured node and reading `ts_tree_cursor_current_descendant_index`. The Go side checks each
mapped node's symbol and byte range against the capture (mismatch = `EngineFault`).

## Go API (signatures from api-v2 §8)
```go
func NewQuery(lang *Language, pattern string) (*Query, error)   // validates once (via a scratch instance) -> *QueryError{Offset, Type}
func (*Query) CaptureNames() []string; PatternCount() int; Properties(pattern int) map[string]string
func (*Tree) Matches(ctx context.Context, q *Query, opts *QueryOptions) iter.Seq2[QueryMatch, error]
type QueryOptions struct{ StartByte, EndByte uint32; MatchLimit uint32 }
type QueryMatch struct{ Pattern int; Captures []QueryCapture }   // Captures[i].Node is a snapshot Node
```
- The query compiles per parser instance on first use and is dropped on instance reset (generation).
- `Matches` takes the tree's snapshot (exporting it if needed), then streams matches; text predicates (`#eq?`, `#match?`
  (Go `regexp`, RE2 — documented difference from Rust regex), `#any-of?`, and their `not-` forms) filter in Go.
- The parser is busy while an iterator is open: `Parse`/`Reset` return an error; breaking the loop releases it.
- Cancellation and limits follow the parse outcome table: ctx halt → `Timeout`/`Cancelled` error from the iterator,
  match limit exceeded → reported on the iterator (`QueryMatch` stream ends with `ErrQueryMatchLimit`), traps →
  `EngineFault` + instance reset.

## Verification (pre-declared)
- Native reference: `tools/native-ref` gains `query <query-file> <source>` printing raw matches (pattern, capture,
  node descendant index, symbol, range) without predicates. Inputs: each grammar repository's own `queries/*.scm`
  (highlights, tags, locals, injections where present; pinned and sha256-verified) over the corpus inputs. Gate: raw
  matches byte-identical to native for every query file and input.
- Predicates: compared with tree-sitter CLI v0.27.2 `query` output (it evaluates text predicates) on the same inputs;
  differences only where RE2 and Rust regex differ, each listed.
- Cancellation-then-parse regression and fuzzing of `NewQuery` (no panics, errors carry offsets).

## Implementation refinements (Q1, 2026-11-14)

- ABI 2 query exports live in `engine.QueryModule`, embedding the unchanged ABI 1 `Module`. Adding required methods
  directly to Module would prevent released ABI 1 grammar code from compiling, contradicting backward compatibility.
- The abbreviated capture record above is extended to five u32 values: capture_index, node_index, symbol, start_byte,
  end_byte. Without the last three fields the required independent symbol/range validation is impossible. Exact
  metadata and buffer layouts are recorded in `engine-abi.md` section 6. The cursor handle is an opaque shim wrapper
  holding the TSQueryCursor, callback options and buffer. The first implementation streams one match per call
  (capacity 1), growing only the capture buffer; callers must still validate capacity and payload length.
- `QueryCapture` is `{Index uint32; Node Node}`; Index selects CaptureNames. QueryError.Type uses C error IDs 1–6;
  7 is a Go predicate validation error and its Offset is the pattern start. Accessors return copies. Properties returns
  nil for an invalid pattern. #set! exposes its name/value; other directives and assertions expose their operator and
  space-separated arguments (capture arguments retain @). They do not change filtering or capture text.
- QueryOptions.EndByte=0 means no end limit, including zero-width EOF captures; an explicit EndByte equal to
  source length excludes those EOF nodes; MatchLimit=0 uses UINT32_MAX, matching C's default. Byte ranges outside
  the owned source, reversed ranges and queries from another Language return InvalidInput. The options are copied
  when creating the sequence. Query validation checks predicate arity and compiles regexes before returning Query.
- #eq? compares a capture to a literal or pairs of equally sized capture lists. #match? uses Go regexp (RE2), not the
  CLI's Rust regex; #any-of? checks literal membership. Each occurrence must satisfy the predicate; not- forms negate
  each occurrence. Optional captures with no occurrences satisfy their text predicate. No additional filtering is
  inferred for opaque directives or property assertions.


## Review corrections (Q1 follow-up, F1–F5)

- The range is validated against the input length, but EndByte is forwarded unchanged to C. In particular, zero
  retains C's unlimited-end semantics. StartByte still must not exceed the effective end (source length for validation).
- Capture indices are now resolved through a cursor-owned, half-full hash table of TSNode IDs. One preorder traversal
  records each node's descendant index on the first returned match; later captures, including backward captures, use
  expected constant-time lookups. Mapping costs expected O(nodes + captures), with 8 bytes per slot in wasm32 and fewer
  than four slots per node. The table is freed with the query cursor. Building it polls should_stop every 256 nodes.
  The host lazily allocates one cursor per parser instance and reuses it across Matches calls, queries and ranges.
  Its index covers the whole tree: the first match on a different tree costs O(nodes), even for a narrow range;
  consecutive execs on the same tree reuse it. Switching trees frees the old table. The shim registers its cursors
  and drops tables referencing a tree before deleting that tree, so allocator reuse of a TSTree address cannot reuse
  stale indices. A cancelled index build frees its partial table before the next exec. Tree edits are rejected by
  Matches. Reset, Close and traps release the whole instance, including its cursor and table.
  For N nodes, the index allocation is `8 * 2^ceil(log2(2*N))` bytes in wasm32: less than 32 bytes per
  node, excluding allocator overhead. At most the last queried live tree's index is retained per instance,
  until that tree is deleted (Tree.Close, leak drain or ParserPool.Put), a different tree is queried, or the
  instance is discarded. The query-dependent TSQueryCursor arrays and match buffer remain allocated until
  instance discard (Parser.Reset, Parser.Close, recycling or a trap), even after the tree is deleted or switched.
  These retained allocations count against WithMaxMemoryBytes, leaving less room for subsequent parsing;
  neither a narrow byte range nor MatchLimit bounds the whole-tree index. Freeing the index makes its space
  reusable inside the instance; linear memory is returned to the OS only on instance discard.
  TestQueryNarrowRangeReuse retains a 2.28 MB JSON regression: after warming, 20 narrow-range calls must finish within
  1 second (P18's per-exec rebuild took 9.65 seconds). This is a regression bound, not a platform speed guarantee.
- Predicate validation follows CLI v0.27.2: #any-of? and #not-any-of? accept a capture with an empty alternative list.
  #set!, #is? and #is-not? accept one to three arguments containing one or two literals and at most one capture, in
  any order; a key literal is required. Properties exposes #set!'s key/value; capture qualification is validated but
  does not add property semantics to the query filter. Unknown predicates/directives remain opaque properties.
- #any-eq?, #any-not-eq?, #any-match?, #any-not-match? validate the corresponding eq/match argument shapes (and regex)
  but do not filter, matching the CLI 0.27.2 behavior recorded in P16. They are exposed as properties. This statement
  concerns that CLI version; it is not a claim about all Rust binding consumers.
- The per-instance cache now uses weak Query keys. An unreachable Query's runtime cleanup enqueues a retirement with
  weak owner/instance identities; no generated engine call runs on the GC goroutine. The next owner engine operation
  deletes the corresponding TSQuery. Retirements from a replaced instance are ignored. Reset/Close release the entire
  instance. Reuse Query values to avoid repeatedly allocating validation instances and compiling the same source.
- MatchLimit bounds in-progress states, not returned results. On overflow the iterator reports ErrQueryMatchLimit
  and discards any match produced by that same next call. Previously yielded matches remain valid; small limits may
  yield no matches at all. The public doc comments state this explicitly.
