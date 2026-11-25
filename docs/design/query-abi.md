# Queries — engine ABI 2–4 and Go API

The query contract covers validation, matching, and opt-in budgets.
[Engine ABI layouts](engine-abi.md#6-query-additions-abi-2) · [API inventory](api-stability.md) ·
[ADR-0030](decisions/ADR-0030-query-budgets.md)

## Goals and non-goals
- Run the unmodified C query engine inside the instance; map every capture to a snapshot `Node` exactly.
- Evaluate text predicates in Go against the snapshot's owned source (C does not evaluate them).
- Non-goals for v0.1: query-level caching across parsers, `#set!`/`#is?` semantics beyond exposing them as properties.

## ABI v2 (additions only; every v1 export keeps its meaning)
The root accepts grammar modules built for ABI 1–4. Queries need ABI 2 or later (`ErrQueryUnsupported` for ABI 1).
ABI 3/4 retain these query exports and block formats and use snapshot format 2 for field exceptions.

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
  (the regex subset below), `#any-of?`, and their `not-` forms) filter in Go.
- The parser is busy while an iterator is open: `Parse`/`Reset` return an error; breaking the loop releases it.
- Cancellation and limits follow the parse outcome table: ctx halt → `Timeout`/`Cancelled` error from the iterator,
  match limit exceeded → reported on the iterator (`QueryMatch` stream ends with `ErrQueryMatchLimit`), traps →
  `EngineFault` + instance reset.

## Opt-in Go budgets

`NewQueryWithOptions(ctx, lang, pattern, ...QueryCompileOption)` validates a query in a scratch
instance. `Tree.MatchesWithOptions(ctx, query, opts, ...QueryMatchOption)` resolves the budgets
and copies `QueryOptions` at sequence creation. Each iteration has fresh counters. Query values
can be shared across independent parsers. Both entry points accept zero for an unset limit,
apply repeated options in order, and reject nil options and out-of-range values.

| Option | What it caps and when |
| --- | --- |
| `WithMaxQuerySourceBytes` | Entire pattern in bytes, including comments and whitespace. Checked before scratch allocation, with a maximum of 2 GiB − 1. |
| `WithMaxQueryMemoryBytes` | Scratch linear memory using the parser's memory limit path (32 MiB to 4 GiB for a positive cap). Lazy compilation on the live parser uses its own `WithMaxMemoryBytes`. |
| `WithMaxQueryMetadataBytes` | TSGQ block bytes and, independently, cumulative bytes written into derived property strings. Checked before Go decoding and before constructing derived strings, respectively. The engine already allocated TSGQ. The option accepts at most 2³² − 1. |
| `WithMaxQueryRegexUnits` | Cumulative translation, parsing and expansion cost of all validated regexes, including opaque predicates. Reservations precede variable-size growth or standard-library calls. Units include translated bytes, syntax nodes, instructions and rune entries and can vary with implementation and toolchain. |
| `WithMaxQueryCaptures` | Entries across raw candidates, including rejected candidates. Checked after the engine returns the match buffer and before allocating the Go capture slice. |
| `WithMaxQueryResults` | Accepted matches, including zero-capture matches. At N matches, iteration succeeds if there are no more accepted matches. The first excess match ends the sequence with `ResourceLimit` before yielding it. Early consumer break finishes immediately. |
| `WithMaxQueryPredicateWork` | Cumulative Go candidate, capture and predicate work, including failed alternatives. Charges include length checks, chunks of string bytes compared, and regex VM scratch setup, positions, state pops and character-class lookups. |

Budgets permit exact fit. An observed done context takes precedence at Go budget checkpoints.
The opt-in iterator polls within predicate evaluation and uses the instrumented regex VM, including
for valid UTF-8. The legacy iterator can use a query built with the new constructor via its VM
fallback. The opt-in iterator also accepts queries built with the legacy constructor.

C query compilation observes cancellation at Go call boundaries. The capture and metadata caps
apply on the Go side after ABI 2–4 shim allocation. Engine traversal, cursor advances, snapshot
export, and the first whole-tree index remain subject to parser limits and cooperative engine
cancellation. See [query outcomes](../specs/parser-result.md#query-results-and-limits) for errors and lifetimes.

### Running untrusted queries

Use the parser input, snapshot-node and linear-memory limits together with the cursor's
`MatchLimit`, query budgets and a context deadline. For a hard deadline or total memory bound,
run the request in an independently killable worker. Cap query, source, and IPC frame sizes
before buffering, and limit concurrency and response bytes. Apply an OS memory limit to the
worker and all its descendants. The supervisor deadline covers startup, compile, parse,
iteration and output. On expiry, terminate and reap the process tree and discard the partial
response. Replace reusable workers after forced stops and bound retained results and caches.

The worker's memory limit includes Go heap, snapshots, results and engine memory. The parser's
linear-memory limit covers one engine instance. The supervisor enforces process-wide memory and
elapsed time independently of cooperative checks and Go heap targets.

## Verification (pre-declared)
- Native reference: `tools/native-ref` gains `query <query-file> <source>` printing raw matches (pattern, capture,
  node descendant index, symbol, range) without predicates. Inputs: each grammar repository's own `queries/*.scm`
  (highlights, tags, locals, injections where present; pinned and sha256-verified) over the corpus inputs. Gate: raw
  matches byte-identical to native for every query file and input. `TestQueryNative` runs this comparison
  with both query APIs and compares the two Go-filtered result streams under nonbinding budgets.
- Predicates: retained CLI v0.27.2 `query` outputs record pattern/input boolean results with CLI identity and hashes;
  product-lane tests compare against that matrix (E3). This is separate from native C raw-query evidence (E5).
- Cancellation-then-parse regression and fuzzing of `NewQuery` (no panics, errors carry offsets).

## Implementation refinements (2026-11-14)

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
- #eq? compares a capture to a literal or pairs of equally sized capture lists. #match? uses the regex subset below;
  #any-of? checks literal membership. Each occurrence must satisfy the predicate; not- forms negate
  each occurrence. Optional captures with no occurrences satisfy their text predicate. No additional filtering is
  inferred for opaque directives or property assertions.


## Review corrections (follow-up to the 2026-11-14 implementation, F1–F5)

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
  1 second (the per-exec rebuild measured in the session-15 independent query review took 9.65 seconds). This is a regression bound, not a platform speed guarantee.
- Predicate validation follows CLI v0.27.2: #any-of? and #not-any-of? accept a capture with an empty alternative list.
  #set!, #is? and #is-not? accept one to three arguments containing one or two literals and at most one capture, in
  any order; a key literal is required. Properties exposes #set!'s key/value; capture qualification is validated but
  does not add property semantics to the query filter. Unknown predicates/directives remain opaque properties.
- #any-eq?, #any-not-eq?, #any-match?, #any-not-match? validate the corresponding eq/match argument shapes (and regex)
  but do not filter, matching the CLI 0.27.2 behavior recorded in the session-15 independent query review. They are exposed as properties. This statement
  concerns that CLI version; it is not a claim about all Rust binding consumers.
- The per-instance cache now uses weak Query keys. An unreachable Query's runtime cleanup enqueues a retirement with
  weak owner/instance identities; no generated engine call runs on the GC goroutine. The next owner engine operation
  deletes the corresponding TSQuery. Retirements from a replaced instance are ignored. Reset/Close release the entire
  instance. Reuse Query values to avoid repeatedly allocating validation instances and compiling the same source.
- MatchLimit bounds in-progress states, not returned results. On overflow the iterator reports ErrQueryMatchLimit
  and discards any match produced by that same next call. Previously yielded matches remain valid; small limits may
  yield no matches at all. The public doc comments state this explicitly.

## Regex predicate subset (v0.6.0)

`#match?` and `#not-match?` use boolean search semantics. Shorthand classes `\w`, `\d`, `\s` (and their negations),
`\b`/`\B` word membership and `(?i)` simple case folding use generated Unicode 16.0.0 tables from `regex-syntax`
0.8.11, pinned by the tree-sitter CLI 0.27.2 lockfile. These tables are embedded in the standard-library-only root
module. Case folding precedes class negation, including for computed shorthand sets. Literal and class folding do
not use Go's own Unicode fold tables.

The translator emits canonical Go regex syntax with explicit rune ranges. Go `regexp` handles patterns without
word-boundary assertions on valid UTF-8. A bounded-state matcher over `regexp/syntax.Prog` handles word boundaries
and invalid UTF-8 text, using per-call state. No Unicode character-consuming instruction advances over invalid
bytes; an encoded U+FFFD is an ordinary valid character. Unicode assertions use regex-automata 0.4.18 forward
decoding and its four-byte reverse-decoding window, including beside lone continuation bytes. The reverse decoder
can recognize a word character ending before the assertion; this affects assertions only, never byte consumption.
Matching does not return regex captures or submatch priorities. Query captures continue to come from the C query engine.

Scoped `(?-u:...)` supports ASCII shorthand classes and word boundaries. Consuming classes are accepted only when
their final computed set is ASCII: `(?-u:[^\W_])` works, while `(?-u:\W)`, `(?-u:\D)`, `(?-u:\S)` and `(?-u:[^a])`
are rejected. Its `\s` preserves Go's old ASCII set without vertical tab, unlike CLI 0.27.2's ASCII `\s`; this is a
recorded residual difference. It does not enable byte matching: `(?-u:.)`, non-ASCII byte matching and `(?i-u)` are
rejected. `\p{...}` accepts Go categories and scripts using the consumer's Go toolchain tables, so those properties
can differ from the pinned Unicode 16 shorthand/fold tables. Rust-only properties, including `Alphabetic`, `Emoji`,
`scx`, `age`, `gcb`, `wb` and `sb`, are rejected.

Unsupported constructs return a query predicate error naming the construct: `\<`, `\>`, extended `\b{...}` assertions,
class set operations (`&&`, `--`, `~~`), nested classes other than POSIX classes, verbose `(?x)` and CRLF `(?R)` modes,
`\Q...\E`, octal escapes, `\0`, unrecognized letter escapes, repetitions above 1000 and dotted capture-group names.
Use hex escapes for octal/NUL literals and individually escaped punctuation instead of `\Q...\E`.
ASCII punctuation escapes such as `\_`, `\%` and `\!` are accepted. POSIX classes inside brackets, including
`[[:alpha:]]` and `[[:^digit:]]`, use Rust's ASCII sets; negated sets complement within the active Unicode/byte mode.
Only recognized lowercase POSIX names are supported; unknown or case-variant forms such as `[[:Alpha:]]` are
rejected, even though the CLI can interpret them as nested classes.
These acceptance limits remain even where the CLI accepts the pattern. This is a supported subset, not the full
Rust regex language. `#any-match?` and `#any-not-match?` validate this syntax but remain non-filtering properties.

Compilation budgets also differ: a repetition count is limited to 1000 here, while the CLI's UTF-8 byte-automaton
size limit can reject a valid Unicode pattern such as `\w{1000}` that this implementation accepts. The recorded
acceptance probes retain both directions of this difference. Group nesting is limited to 99 nested groups
(the outer expression counts toward a depth limit of 100). Nested repetition (`a**`, `a*+`, `a{2}{3}`, `a*??`),
spaces inside repetition counts (`a{ 1 }`, `a{1 ,2}`), and hex escapes longer than eight digits are rejected.
Property names must match Go names exactly: Rust aliases and loose names such as `Letter`, `lu`, `LU`, `Latn`,
`Zyyy`, `han`, `Hira`, `gc=Lu`, `Math`, `Any`, `ASCII`, `Assigned` and ` L ` are rejected. Go's `\p{Cs}`
and `\P{Cs}` are accepted as empty/all-scalar sets, although the CLI rejects the surrogate category.
Group names are restricted to ASCII letters, digits and underscores, with a letter or underscore first; regex
captures are not returned by boolean predicates.

The retained E3 matrix records where behavior agrees with tree-sitter CLI 0.27.2 on its named patterns and inputs,
including the ASCII-space and Go Unicode-property residuals described above. CLI/library identities, input bytes,
results and acceptance differences are retained in [`testdata/query-regex-cli.json`](../../testdata/query-regex-cli.json).
It does not establish native C agreement or general regex equivalence.
