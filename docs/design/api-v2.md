# Public API v2 on the W2 engine

Status: accepted design (owner decisions 2026-11-13, ADR-0021). `docs/specs/parser-result.md` is rewritten from this
document during implementation; until then this document is the reference for the new API. The module has no tag.

Inputs: ADR-0020 (engine), ADR-0004 (explicit outcomes), ADR-0010 (limits), ADR-0011 (index); reviews R1 (user and
ecosystem view), R2 (adversarial API review), E1 (engine failure modes), T1 (survey of AST tools), T2 (primitive
measurements) in the local session-15 handoff.

## 1. Engine facts the API is built on

- One translated wasm instance per `Parser`; it is single-threaded and its C trees live in its linear memory.
  Historical per-node calls into the instance cost 240–840 ns; fixed-size byte/symbol/flag reads of an exported
  Go snapshot cost 4–10 ns. Point lookups derive rows and byte columns from the source: the first builds the snapshot's line index (one
  pass, 4 B per line, up to 4x the source size for input made of line feeds), later ones cost O(log lines).
- The C runtime returns a complete tree or `NULL`. A halted parse keeps its state and **resumes on the next call**
  unless `ts_parser_reset` is called (parser.c:2146-2148, 2195, 2234-2235; reproduced in E1).
- Allocation failure aborts (wasm `unreachable`). Grammar scanners may also write through unchecked `NULL`s.
- Accepted trees always end at the input length (1,710/1,710 in E1); the root may start after leading padding
  (whitespace, BOM).

## 2. Packages and modules

```
github.com/wotjr1649/go-treesitter                  package treesitter: Parser, Tree, Snapshot, Node, Index, Query, ...
github.com/wotjr1649/go-treesitter/internal/engine  frozen engine ABI (versioned), host imports, memory providers
github.com/wotjr1649/go-treesitter/grammars/<lang>  one Go module per grammar: func Language() *treesitter.Language
```
The `syntax` package and `internal/gtsadapter` are removed. Each grammar module carries its own translated runtime;
`Language` stays opaque so a later shared runtime is not a breaking change. Generated grammars use
`NewLanguage(*engine.Grammar) *Language` to construct their `Language()` result. See the
[root API inventory](api-stability.md) for the generated-code contract. `NewParser` checks the grammar module's
engine ABI version and snapshot block magic/version and returns `ErrIncompatibleLanguage` on mismatch. Grammar
`go.mod` files require the exact root version they were generated against; CI tests the oldest supported grammar
module against the newest root. Language metadata (symbol/field names, identity) is static Go data; no instance is
needed to read it.

The root accepts engine ABI 1–4 grammar modules. ABI 1 supports parsing. ABI 2 adds queries. ABI 3 adds
single-child field-lookup exceptions through snapshot format 2. ABI 4 adds public symbol IDs to language metadata.
ABI 1/2 modules keep direct-child field lookup for alias-inherited fields.

## 3. Parsing

```go
func NewParser(lang *Language, opts ...Option) (*Parser, error)   // creates the instance now (not lazily)
func (*Parser) Parse(ctx context.Context, src []byte, old *Tree) (*Tree, error)
func (*Parser) ParseOwned(ctx context.Context, src []byte, old *Tree) (*Tree, error)
        // caller transfers ownership of src and must never modify it again; no copy is made
func (*Parser) ParseSnapshot(ctx context.Context, src []byte) (*Snapshot, error)  // parse + export + free the tree
func (*Parser) Reset() error      // drop all trees, recreate the instance; returns committed memory to the OS
func (*Parser) Close() error      // idempotent; invalidates its trees; snapshots stay valid

Option: WithMaxInputBytes(n), WithMaxMemoryBytes(n) (per instance; out-of-range is an error, not clamped),
        WithMaxSnapshotNodes(n), WithRecycleAboveBytes(n) (Reset after a parse that committed more), WithStrictUTF8()
```
Fixed evaluation order: nil/done `ctx` → option/input checks (size, ownership of `old`) → engine. Timeout and
cancellation are taken from `ctx` and checked in the progress callback; stop detection is about 0.2 ms, the return
waits for the parser's cleanup (up to about 6 % of the time already spent; E1 measured up to 97 ms).

**Every path that returns no tree calls `ts_parser_reset` before returning.** Regression test: cancel during
balancing, then parse a different source and require its own tree.

Input is parsed like C tree-sitter: invalid UTF-8 bytes become decode errors and a leading BOM is skipped;
`(*Tree).HasInvalidUTF8()` reports it; `WithStrictUTF8()` turns it into `InvalidInput` instead.

## 4. Outcomes and errors

`Parse` returns a non-nil `*Tree` only for a complete tree; `Outcome()` is `AcceptedClean` or `AcceptedWithErrors`
(syntax errors are not Go errors). Every accepted tree is checked at run time: root end == input length, else
`EngineFault`. Other results return `nil` and `*ParseError{Outcome, Reason, InstanceReset bool}`; sentinels for
`errors.Is`: `ErrTimeout` (also `context.DeadlineExceeded`), `ErrCancelled` (also `context.Canceled`),
`ErrResourceLimit`, `ErrInvalidInput`, `ErrEngineFault`, `ErrIncompatibleLanguage`, `ErrTreeInvalidated`, `ErrTreeEdited`.

| Cause | Outcome | Instance |
|---|---|---|
| ctx deadline / cancel (callback halt) | Timeout / Cancelled | kept (after reset) |
| input over cap, snapshot node cap (checked with `ts_node_descendant_count` before export) | ResourceLimit | kept |
| refused `memory.grow` during the parse (host flag), even if a tree came back | ResourceLimit | reset |
| any other trap (`unreachable`, out-of-bounds) | EngineFault | reset |
| non-UTF-8 with `WithStrictUTF8`, negative options, `old` from another parser, nil ctx | InvalidInput | kept |
| grammar ABI/version mismatch | IncompatibleLanguage | none created |

## 5. Trees, edits, lifetimes, concurrency

```go
func (*Tree) Outcome() Outcome; HasInvalidUTF8() bool; InputBytes() int
func (*Tree) Snapshot(ctx context.Context) (*Snapshot, error)   // cached; cancellable chunked export
func (*Tree) Edit(e InputEdit) error      // in place (C semantics): afterwards the tree is old-only,
                                          // Snapshot() returns ErrTreeEdited; take snapshots before editing
func (*Tree) ChangedRanges(old *Tree) ([]Range, error)          // one call into the instance
func (*Tree) Close()
func EditFor(old, new []byte, start, oldEnd, newEnd uint32) InputEdit  // O(n); byte columns; rows count '\n'
```
- A `Parser` and all its `Tree`s are one confinement unit: one goroutine at a time. `Snapshot`, `Node`, `Index`,
  `LineIndex` are immutable and safe for any number of goroutines.
- Every `Tree` carries the parser's generation; after `Close`, `Reset` or an instance reset its methods return
  `ErrTreeInvalidated`. A leaked tree's cleanup only enqueues a free; the owner goroutine drains the queue on the next
  `Parse`, `Reset` or `Close` (never from the GC goroutine).
- `ParserPool` (`Get(ctx)`, `Put`): `Put` invalidates all live trees of that parser before it can be lent again.
- Incremental parsing is not guaranteed to equal a fresh parse (upstream behavior; 5 of 5,919 edit cases differed in
  both native and W2). This is documented, not hidden.
- `Edit` checks byte bounds and point ordering, not whether points match bytes. Use `EditFor` for consistent
  points. C trusts supplied edit points; snapshot points instead derive from the new source bytes.
  `ChangedRanges` also derives points from the result source (offsets past EOF are clamped), reusing its snapshot
  index if present or otherwise building a temporary line index on every call (O(source) each time). For example, inserting LF at byte 3 into `[1,2,3]\n`
  with all edit points zero can make C report range
  `[3,4)` as `0:3`–`0:4`; this API reports `0:3`–`1:0`. Query captures contain byte offsets and node indices,
  not stored points.

## 6. Snapshot and Node

The snapshot owns the source: by default one copy (about 0.07 ms per MB; +5–22 % of the snapshot's own memory, no
disk use); with `ParseOwned` no copy. Struct-of-arrays, 25 B/node (previously 41) including `descendantEnd` and `childCount`
for O(1) subtree skips and child counts, plus sparse field-lookup exceptions in ABI 3/4 snapshots.
`StartPoint`, `EndPoint` and point-range lookups derive LF rows and byte columns from the byte offsets using the
snapshot's shared `Lines()` index: O(source bytes) on first use, O(log lines) per point. Its lazy `[]uint32` line
starts cost `4 * (LF count + 1)` bytes (at most `len(source) + 1` entries), plus fixed object/allocator overhead;
the owned source is shared. `LineIndex.Pos` alone needs the additional lazy non-ASCII bitset
(`8 * ceil(lines / 64)` bytes). `WithMaxMemoryBytes` still limits only instance wasm linear memory; source,
snapshot arrays, line starts and other Go-heap caches are excluded.

```go
func (*Snapshot) Root() Node; Len() int; Node(i int) Node; Language() *Language; Source() string
func (*Snapshot) Walk(fn func(n Node, depth int) WalkAction)   // WalkAction: Continue, SkipChildren, Stop
func (*Snapshot) All() iter.Seq[Node]
func (*Snapshot) DescendantForByteRange(start, end uint32, named bool) Node   // C semantics, checked against C
func (*Snapshot) DescendantForPointRange(start, end Point, named bool) Node

func (Node) Kind() string; Symbol() Symbol; Field() string; FieldID() FieldID; Index() int
func (Node) IsNamed() bool; IsExtra() bool; IsMissing() bool; IsError() bool; HasError() bool
func (Node) StartByte() uint32; EndByte() uint32; StartPoint() Point; EndPoint() Point
func (Node) Parent() (Node, bool); ChildCount() int; Child(i int) (Node, bool); NamedChildCount() int; NamedChild(i int) (Node, bool)
func (Node) NextSibling() (Node, bool); PrevSibling() (Node, bool); NextNamedSibling() (Node, bool); PrevNamedSibling() (Node, bool)
func (Node) Children() iter.Seq[Node]; NamedChildren() iter.Seq[Node]
func (Node) ChildByFieldName(name string) (Node, bool); ChildrenByFieldName(name string) iter.Seq[Node]
func (Node) Text() string          // zero-allocation substring of the owned source (T2: 1.9 ns)
func (Node) AppendText(dst []byte) []byte
func (Node) String() string        // S-expression, for tests and debugging
func (*Language) SymbolForName(name string, named bool) (Symbol, bool)
```
`SymbolForName` selects the first visible or supertype raw symbol in raw ID order with the requested name and
namedness. With ABI 4 grammar modules it returns that symbol's public ID, in the same ID space as `Node.Symbol()`,
`SymbolName` and `Index.OfSymbol` (ADR-0011). Named `ERROR` resolves to `ErrorSymbol`. ABI 1–3 grammar modules
retain the raw ID lookup. A name can have multiple public IDs or be shared by named and anonymous nodes.
`Index.OfType` yields all nodes with the requested `Kind()`, in preorder.

With an ABI 3 or 4 grammar module, `ChildByFieldName` follows C's `ts_node_child_by_field_id`: an inherited field may
refer to a descendant inside a visible alias rather than a direct child. The snapshot stores only the exceptions
to selecting the first direct child with the matching cursor field. ABI 1/2 snapshots have no exception table and
retain that direct-child rule, so they can differ from C on alias-inherited fields. `Node.Field()` continues to
report the cursor field in the visible parent; it does not imply that C field lookup selects that node.
Both field-lookup methods return nothing on an ERROR node, for an unknown field name or on the zero `Node`,
including when the grammar module uses ABI 1 or 2.
`ChildrenByFieldName` yields the C exception target first, then matching direct children in order without
duplicating that target; an explicit no-node exception yields nothing. Without an exception it retains the
direct-child iteration. Thus the first result remains `ChildByFieldName`, but it need not be a direct child.
`ChildrenByFieldName` is not a C-verified API: v0.27.2 has no children-by-field API. Its first element equals
`ChildByFieldName`; the iterator order and explicit-null behavior are this Go API's semantics (E3 tests).
Historical E5 agreement for `ChildByFieldName` (2026-11-15 field-export-cost run) is limited to fresh parses of the reproducible pinned
1,761-input corpus on Windows amd64. That run also checked 2,779 additional local inputs, recorded separately in the
[archived evidence and identities](engine-abi.md#field-lookup-evidence). The 2026-11-15 wave-2 batch 2 ABI 3 run reran the field-export cost-filter guard (independently reviewed 2026-11-15) on the same reproducible corpus with E5 agreement; its input and source identities are archived with that evidence.
The 2026-11-15 edited-tree field-lookup run adds E5 agreement on Windows amd64 for all 5,283 reproducible seeded incremental edits of the 15 grammars
and five retained T-SQL incremental-difference cases. `ChildByFieldName` is compared with C's incremental tree, and the iterator's
first result is checked to equal it; the remaining iterator results have no C oracle. See the
[scope and identities](engine-abi.md#field-lookup-evidence).

## 7. Primitives that set this library apart (v0.0.1)

All are lazy (computed on first use, once, safe for concurrent readers) and measured in T2:

- **Positions and encodings.** `NewSourceUTF16`/BOM detection convert UTF-16 or BOM-prefixed input to UTF-8, and
  `NewSourceCP949` converts CP949 (standard library only; the WHATWG `EUC-KR` index = windows-949, decoded by a
  generated table; ADR-0021 amendment), keeping an offset map to the original bytes in both directions;
  `(*Snapshot).Lines()` returns a `LineIndex` with byte → line and UTF-8, UTF-16 and rune columns, and back
  (`Offset`). In a converted source (UTF-16, CP949) an offset inside a character maps to that character's start; a UTF-8
  source is not converted, so its offsets pass through clamped. `LineIndex.Pos` floors `UTF16Col` and `RuneCol` to the
  character's start, while `ByteCol` stays the byte distance from the line start. See the
  [Korean and CP949 input guide](../guides/korean-input.md). Line starts build lazily in O(source bytes); byte points use binary search, while
  UTF-16/rune columns can additionally scan the non-ASCII line prefix. Standard library only.
- **Merkle subtree hashes.** `Node.Hash()` (kind, field, leaf text, children) and `Node.ShapeHash()` (ignores extras
  such as comments). 7–11 ns/node, 8 B/node; equal-hash groups verified by full canonical serialization. Use cases:
  subtree-level caching (instead of per-file), duplicate and clone detection, change detection.
- **Atoms.** `Node.Atom()` and `(*Snapshot).AtomText(a)` intern leaf-token text per snapshot (6–11 ns/node, +10 %
  memory, texts are substrings of the owned source). Frequency/equality work is 6–13x faster than with strings.
- **Match (experimental, separate package).** `match.Match(old, new *treesitter.Snapshot) *match.Mapping` in
  `github.com/wotjr1649/go-treesitter/experimental/match` (same module, no compatibility promise, built on the public
  `Node` API) pairs nodes across an edit (top-down equal-hash
  subtrees + LCS, bottom-up dice). Measured: 100 % on large single-token edits; 77–97 % matched with 0.1–0.3 % wrong
  on small edits (Go 3.7 % wrong), weak when the edit produces ERROR recovery. Marked experimental; accuracy work
  continues before it is promoted.

Signatures as implemented (session 15 step 2):

```go
func NewSource(raw []byte) (*Source, error)                  // BOM detection; UTF-8 (with or without BOM) is kept as is
func NewSourceUTF16(raw []byte, bigEndian bool) (*Source, error)
func NewSourceCP949(raw []byte) (*Source, error)             // no detection; EF BB BF / FF FE / FE FF prefix -> ErrInvalidInput
func (*Source) Bytes() []byte; Original() []byte; Encoding() Encoding; OriginalOffset(utf8Off uint32) int
func (*Source) UTF8Offset(orig int) uint32; Replacements() int   // Encoding values: UTF8, UTF16LE, UTF16BE, CP949
func NewLineIndex(src string) *LineIndex; (*Snapshot) Lines() *LineIndex
func (*LineIndex) NumLines() int; LineStart(line int) uint32; Line(off uint32) int; Pos(off uint32) Position // {Line, ByteCol, UTF16Col, RuneCol}
func (*LineIndex) Offset(line int, col uint32, kind ColumnKind) (uint32, bool)  // ByteColumn, UTF16Column, RuneColumn
func (Node) Hash() uint64; ShapeHash() uint64          // comparable within one grammar only (symbol ids are hashed)
func (Node) Atom() Atom; (*Snapshot) AtomText(Atom) string; NumAtoms() int   // named non-empty leaves only
// package experimental/match (not part of the stable treesitter API):
func Match(old, new *treesitter.Snapshot) *Mapping    // EXPERIMENTAL; NewOf/OldOf(Node) (Node, bool), Len; other grammar -> empty
func NewParserPool(lang *Language, maxParsers int, opts ...Option) (*ParserPool, error) // Get(ctx), Put, Close
func NewIndex(*Snapshot) *Index; (*Snapshot) Index() *Index  // OfType, OfSymbol -> iter.Seq[Node]; NodeAt(off uint32) (Node, bool)
```

## 8. Queries (v0.1; signatures fixed now)

`NewQuery(lang, pattern) (*Query, error)` validates the pattern in a scratch instance before returning.
Each parser instance compiles the query on first use and discards its compiled cache on reset.
`(*Tree).Matches(ctx, q, opts) iter.Seq2[QueryMatch, error]` runs the C engine and maps captures to snapshot
nodes by preorder index (equal to C's descendant index). Text predicates (`#eq?`, `#match?`, `#any-of?`) are
evaluated in Go against the snapshot's owned source. Iteration holds the parser busy until it ends or
the caller breaks early.

In v0.6.0, regex predicates default to Unicode 16.0.0 shorthand classes, word boundaries
and simple case folding from CLI 0.27.2's pinned `regex-syntax` 0.8.11 tables. `\p{...}` categories/scripts instead
follow the consumer's Go toolchain. Scoped `(?-u:...)` retains ASCII shorthand classes and boundaries; unsupported
Rust or Go-only syntax returns a query error. `#any-*` predicates still validate and expose properties without
filtering. See the [regex subset](query-abi.md#regex-predicate-subset-v060) and
[migration guidance](../../MIGRATING.md#upgrading-from-v050-to-v060-engine-abi-4).

## 9. Out of scope for v0.0.1

Live per-node navigation over the instance, included ranges, shared-runtime linking, legacy encodings other than UTF-16
and CP949, a compatibility package for other Go tree-sitter APIs (a `MIGRATING.md` name map is written instead).
