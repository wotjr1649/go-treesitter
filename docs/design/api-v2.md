# Public API v2 on the W2 engine

**Status:** Historical design, accepted on 2026-11-13 in
[ADR-0021](decisions/ADR-0021-public-api-v2.md). The current contracts are the
[parser result contract](../specs/parser-result.md), [root API stability policy](api-stability.md),
[query contract](query-abi.md), and [ADR-0028](decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md)
for root and grammar versioning.

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
- In the original engine investigation, all 1,710 accepted trees ended at the input length. A root can start
  after leading padding (whitespace, BOM). Section 4 describes the current acceptance check.

## 2. Packages and modules

```
github.com/wotjr1649/go-treesitter                  package treesitter: Parser, Tree, Snapshot, Node, Index, Query, ...
github.com/wotjr1649/go-treesitter/internal/engine  frozen engine ABI (versioned), host imports, memory providers
github.com/wotjr1649/go-treesitter/grammars/<lang>  one Go module per grammar: func Language() *treesitter.Language
```
API v2 replaced the `syntax` package and `internal/gtsadapter`. Each grammar module carries its own translated
runtime. Its public entry point is `Language() *treesitter.Language`. Generated code constructs that opaque
value with `NewLanguage(*engine.Grammar) *Language`, the generator-only entry point in the
[root API inventory](api-stability.md#exported-identifiers). Symbol and field metadata can be read without an
engine instance. `NewParser` rejects an unsupported engine ABI or snapshot block magic/version with
`ErrIncompatibleLanguage`.

Each grammar module has independent tags under `grammars/<dir>/v0.x.y`. Its release `go.mod` requires the
oldest published root that supplies its API, ABI, and generated host imports. Go's minimal version selection
can select a newer root for a consumer with mixed grammar versions. A root release alone leaves existing grammar
tags and requirements in place. ADR-0028 calls for testing the minimum and recommended roots and publishing
the tested combinations. The [checkout CI](../../.github/workflows/engine.yml) exercises the root and grammar
modules in the checkout. The [compatibility test plan](api-stability.md#compatibility-test-plan) describes
the planned checks for consumers using published ABI 1–4 modules and mixed grammar versions.

The root accepts engine ABI 1–4 grammar modules. ABI 1 supports parsing. ABI 2 adds queries. ABI 3 adds
single-child field-lookup exceptions through snapshot format 2. ABI 4 adds public symbol IDs to language metadata.
ABI 1/2 modules keep direct-child field lookup for alias-inherited fields.

## 3. Parsing

```go
func NewParser(lang *Language, opts ...Option) (*Parser, error)   // creates the instance eagerly
func (*Parser) Parse(ctx context.Context, src []byte, old *Tree) (*Tree, error)
func (*Parser) ParseOwned(ctx context.Context, src []byte, old *Tree) (*Tree, error)
        // caller transfers ownership of src and must never modify it again
func (*Parser) ParseSnapshot(ctx context.Context, src []byte) (*Snapshot, error)  // parse + export + free the tree
func (*Parser) Reset() error      // invalidate live trees, release and recreate the instance
func (*Parser) Close() error      // idempotent; invalidates its trees; snapshots stay valid
```

`NewParser` rejects invalid options with `InvalidInput`. `WithMaxInputBytes` caps input bytes,
`WithMaxSnapshotNodes` caps exported nodes, and `WithMaxMemoryBytes` caps an instance's wasm linear memory.
`WithRecycleAboveBytes` allows an instance above its committed-memory threshold to be recreated once it has
no live trees.

Parse checks a nil or already-done context first, then admits the input (size, strict UTF-8, and ownership
and validity of `old`) before starting an engine parse. A running parse checks for cancellation through the
progress callback and returns after cleanup. A failed call leaves no suspended parse to resume. A halted C
parse is reset or its instance is recreated. Preflight rejections start no parse. The required
[cancel-then-reparse regression](../../tools/conformance/cancel_test.go) uses a different source for the next parse.

A leading UTF-8 BOM is skipped by the parser. Invalid UTF-8 bytes become decode errors, and
`Tree.HasInvalidUTF8` reports their presence. `WithStrictUTF8` rejects such bytes with `InvalidInput`.
When an encoding constructor converts invalid input to U+FFFD, count replacements with
`Source.Replacements`. The result is valid UTF-8. [Input contract](../specs/parser-result.md#input)

## 4. Outcomes and errors

`Parse` returns a non-nil tree only for a complete result whose root ends at the input length.
`Tree.Outcome` is `AcceptedClean` or `AcceptedWithErrors`. A complete tree with ERROR or MISSING nodes
still returns a nil Go error. Use `errors.As` to inspect a `*ParseError` and `errors.Is` for sentinel matching.
`ParseError.Outcome` classifies the failure, `InstanceReset` reports that the instance was discarded and
live trees invalidated, and `Reason` provides human-readable detail. Error and diagnostic text may change
in any release.
[Root compatibility rules](api-stability.md#compatibility-rules)

| Cause | Outcome | Sentinel matching with `errors.Is` | Instance handling |
|---|---|---|---|
| Context deadline or cancellation | `Timeout` / `Cancelled` | `ErrTimeout` and `context.DeadlineExceeded` / `ErrCancelled` and `context.Canceled` | An already-done context starts no parse. After a callback halt, the C parser is reset. |
| Input or snapshot node cap exceeded | `ResourceLimit` | `ErrResourceLimit` | Kept. The descendant count is checked before export. |
| Refused `memory.grow` during a parse, even if it returned a tree | `ResourceLimit` | `ErrResourceLimit` | Recreated. |
| Other engine trap (including WASI `proc_exit`), root-end mismatch, or module ABI violation during use | `EngineFault` | `ErrEngineFault` | Recreated. |
| Nil context, invalid input or old tree, or invalid constructor options | `InvalidInput` | `ErrInvalidInput` | Preflight rejection starts no parse. |
| Unsupported ABI or snapshot export magic/format during construction | `IncompatibleLanguage` | `ErrIncompatibleLanguage` | No parser is returned. An instance rejected after allocation is released. |

Lifetime sentinels include `ErrTreeInvalidated`, `ErrTreeEdited`, and `ErrParserClosed`.
The complete, closed outcome set is defined by the [parser result contract](../specs/parser-result.md#outcome-vocabulary-closed-set).

## 5. Trees, edits, lifetimes, concurrency

```go
func (*Tree) Outcome() Outcome
func (*Tree) HasInvalidUTF8() bool
func (*Tree) InputBytes() int
func (*Tree) Snapshot(ctx context.Context) (*Snapshot, error)   // cached; cancellable chunked export
func (*Tree) Edit(e InputEdit) error      // in place (C semantics): afterwards the tree is old-only,
                                          // Snapshot() returns ErrTreeEdited; take snapshots before editing
func (*Tree) ChangedRanges(old *Tree) ([]Range, error)          // one call into the instance
func (*Tree) Close()
func EditFor(old, new []byte, start, oldEnd, newEnd uint32) InputEdit  // O(n); byte columns; rows count '\n'
```
- Use a `Parser` and all its trees from one goroutine at a time. The parser rejects detected concurrent use.
  `Snapshot`, `Node`, `Index`, and `LineIndex` are immutable and can be shared by concurrent readers.
- `Parser.Close`, `Parser.Reset`, an instance reset and `ParserPool.Put` invalidate live trees. Operations
  requiring a live tree then report `ErrTreeInvalidated`. `Tree.Close` is idempotent and metadata accessors
  read stored Go values. A leaked tree queues its handle for cleanup by the parser's owner.
- Take snapshots before applying `Tree.Edit` in place. An edited tree is suitable as `old` for incremental
  parsing, and a snapshot request on it returns `ErrTreeEdited`. Incremental and fresh trees may differ.
- `Edit` validates byte bounds and point ordering. Use `EditFor` to calculate points consistent with the bytes.
  The engine receives the supplied edit points. Snapshot points and `ChangedRanges` points derive from the
  result source. Changed-range offsets beyond its end use the EOF point. A change can yield an empty range
  list under the runtime's syntactic comparison. Query captures store byte offsets and node indices.
  [Ownership and edit contract](../specs/parser-result.md#ownership-lifetime-concurrency)

## 6. Snapshot and Node

A snapshot owns its Go data and stays valid after the tree or parser closes. `Parse` and `ParseSnapshot`
make one copy for the snapshot's source. `ParseOwned` avoids that copy by taking ownership of the input.
The snapshot's struct-of-arrays layout retains 21 B/node, plus source and sparse ABI 3/4 field exceptions.
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
func (*Snapshot) DescendantForByteRange(start, end uint32, named bool) Node
func (*Snapshot) DescendantForPointRange(start, end Point, named bool) Node

func (Node) Kind() string; Symbol() Symbol; Field() string; FieldID() FieldID; Index() int
func (Node) IsNamed() bool; IsExtra() bool; IsMissing() bool; IsError() bool; HasError() bool
func (Node) StartByte() uint32; EndByte() uint32; StartPoint() Point; EndPoint() Point
func (Node) Parent() (Node, bool); ChildCount() int; Child(i int) (Node, bool); NamedChildCount() int; NamedChild(i int) (Node, bool)
func (Node) NextSibling() (Node, bool); PrevSibling() (Node, bool); NextNamedSibling() (Node, bool); PrevNamedSibling() (Node, bool)
func (Node) Children() iter.Seq[Node]; NamedChildren() iter.Seq[Node]
func (Node) ChildByFieldName(name string) (Node, bool); ChildrenByFieldName(name string) iter.Seq[Node]
func (Node) Text() string          // substring of the owned source without allocating
func (Node) AppendText(dst []byte) []byte
func (Node) String() string        // S-expression, for tests and debugging
func (*Language) SymbolForName(name string, named bool) (Symbol, bool)
```

The zero `Node` is the null node. Its methods return documented zero results, including empty iterators.

`SymbolForName` selects the first visible or supertype raw symbol in raw ID order with the requested name and
namedness. With ABI 4 grammar modules it returns that symbol's public ID, in the same ID space as `Node.Symbol()`,
`SymbolName` and `Index.OfSymbol` (ADR-0011). Named `ERROR` resolves to `ErrorSymbol`. ABI 1–3 grammar modules
retain the raw ID lookup. A name can have multiple public IDs or be shared by named and anonymous nodes.
`Index.OfType` yields all nodes with the requested `Kind()`, in preorder.

With an ABI 3 or 4 grammar module, `ChildByFieldName` follows the `ts_node_child_by_field_id` of the module's pinned
runtime: the result may be a descendant rather than a direct child (with the v0.27.x runtimes, inside a visible alias),
and a module on another runtime can resolve such a field differently, so the result is fixed by the grammar module
version, not the root version. The snapshot stores only the exceptions
to selecting the first direct child with the matching cursor field. ABI 1/2 snapshots have no exception table and
retain that direct-child rule, so they can differ from C on alias-inherited fields. `Node.Field()` continues to
report the cursor field in the visible parent; it does not imply that C field lookup selects that node.
Both field-lookup methods return nothing on an ERROR node, for an unknown field name or on the zero `Node`,
including when the grammar module uses ABI 1 or 2.
`ChildrenByFieldName` yields the C exception target first, then matching direct children in order without
duplicating that target; an explicit no-node exception yields nothing. Without an exception it retains the
direct-child iteration. Thus the first result remains `ChildByFieldName`, but it need not be a direct child.
The iterator order and explicit-null behavior are Go semantics covered by retained tests (E3).
The [parser result contract](../specs/parser-result.md#field-lookup-and-grammar-compatibility) records the
scope of the historical E5 comparisons for single-child lookup.

## 7. Primitives that set this library apart (v0.0.1)

The original design evaluated the snapshot primitives in T2. The measurements below describe that
investigation.

- **Positions and encodings.** `NewSourceUTF16` converts UTF-16 to UTF-8. `NewSource` detects UTF-16 by BOM
  and preserves UTF-8 bytes including a UTF-8 BOM. `NewSourceCP949` uses a generated table from the WHATWG
  `EUC-KR` index for windows-949 (ADR-0021 amendment). Converted sources map byte offsets in both directions,
  flooring offsets inside a character to its start. UTF-8 offsets pass through with clamping.
  `Snapshot.Lines` returns a `LineIndex` for byte offsets and UTF-8, UTF-16, and rune columns.
  `LineIndex.Pos` floors `UTF16Col` and `RuneCol` to the character's start. `ByteCol` retains the byte
  distance from the line start. [Korean and CP949 input guide](../guides/korean-input.md)
- **Merkle subtree hashes.** `Node.Hash()` includes symbol, field, flags, leaf text and ordered children.
  `Node.ShapeHash()` ignores extra descendants and the extra flag. Use the hashes for equality checks within
  one grammar and release, and compare structure when a collision matters. Numeric values may change in
  any release. [Root compatibility rules](api-stability.md#compatibility-rules)
- **Atoms.** `Node.Atom()` and `(*Snapshot).AtomText(a)` intern leaf-token text per snapshot. The text is
  a substring of the owned source. T2 recorded 6–11 ns/node and about 10 % more snapshot memory.
- **Match (experimental, separate package).** `match.Match(old, new *treesitter.Snapshot) *match.Mapping` in
  `github.com/wotjr1649/go-treesitter/experimental/match` (same module, no compatibility promise, built on the public
  `Node` API) pairs nodes across an edit using equal-hash subtrees, longest common subsequences, and a
  bottom-up similarity score. T2 recorded 100 % on its large single-token edits and 77–97 % matched
  on its small edits, with 0.1–0.3 % wrong matches (Go 3.7 %). Error-recovery edits remained a weak case.

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
func (Node) Hash() uint64; ShapeHash() uint64          // compare within one grammar and release
func (Node) Atom() Atom; (*Snapshot) AtomText(Atom) string; NumAtoms() int   // named non-empty leaves only
// package experimental/match (not part of the stable treesitter API):
func Match(old, new *treesitter.Snapshot) *Mapping    // EXPERIMENTAL; NewOf/OldOf(Node) (Node, bool), Len; other grammar -> empty
func NewParserPool(lang *Language, maxParsers int, opts ...Option) (*ParserPool, error) // Get(ctx), Put, Close
func NewIndex(*Snapshot) *Index; (*Snapshot) Index() *Index  // OfType, OfSymbol -> iter.Seq[Node]; NodeAt(off uint32) (Node, bool)
```

<a name="8-queries-v01-signatures-fixed-now"></a>

## 8. Queries

Queries shipped after the initial API design. The [query contract](query-abi.md) defines validation,
execution, and predicate behavior. ABI 1 grammars still parse. `NewQuery` returns `ErrQueryUnsupported` for them.

`NewQuery(lang, pattern) (*Query, error)` validates the pattern in a scratch instance before returning.
Each parser instance compiles the query on first use and discards its compiled cache on reset.
`(*Tree).Matches(ctx, q, opts) iter.Seq2[QueryMatch, error]` runs the C engine and maps captures to snapshot
nodes by preorder index (equal to C's descendant index). Text predicates (`#eq?`, `#match?`, `#any-of?`) are
evaluated in Go against the snapshot's owned source. Iteration holds the parser busy until it ends or
the caller breaks early.

In v0.6.0, regex predicates default to Unicode 16.0.0 shorthand classes, word boundaries
and simple case folding from CLI 0.27.2's pinned `regex-syntax` 0.8.11 tables. `\p{...}` categories/scripts instead
follow the consumer's Go toolchain. Scoped `(?-u:...)` retains ASCII shorthand classes and boundaries; unsupported
Rust or Go-only syntax returns a query error. The operators `#any-eq?`, `#any-not-eq?`, `#any-match?`, and
`#any-not-match?` are validated and exposed as properties without filtering.
See the [regex subset](query-abi.md#regex-predicate-subset-v060) and
[migration guidance](../../MIGRATING.md#upgrading-from-v050-to-v060-engine-abi-4).

## 9. Out of scope for v0.0.1

The original design excluded live per-node navigation over the instance, included ranges, shared-runtime linking,
legacy encodings beyond the documented converters, and a compatibility package for other Go tree-sitter APIs.
Included ranges are proposed in [ADR-0029](decisions/ADR-0029-included-ranges.md); implementation is deferred until after 1.0.
See the [migration name map](../../MIGRATING.md) and [current limitations](../reports/limitations.md).
