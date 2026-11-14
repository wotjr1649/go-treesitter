# Public API v2 on the W2 engine

Status: accepted design (owner decisions 2026-11-13, ADR-0021). `docs/specs/parser-result.md` is rewritten from this
document during implementation; until then this document is the reference for the new API. The module has no tag.

Inputs: ADR-0020 (engine), ADR-0004 (explicit outcomes), ADR-0010 (limits), ADR-0011 (index); reviews R1 (user and
ecosystem view), R2 (adversarial API review), E1 (engine failure modes), T1 (survey of AST tools), T2 (primitive
measurements) in the local session-15 handoff.

## 1. Engine facts the API is built on

- One translated wasm instance per `Parser`; it is single-threaded and its C trees live in its linear memory.
  Per-node calls into the instance cost 240–840 ns; reads of an exported Go snapshot cost 4–10 ns.
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
github.com/wotjr1649/go-treesitter/encoding         (v0.1, optional) legacy decoders via golang.org/x/text
```
The `syntax` package and `internal/gtsadapter` are removed. Each grammar module carries its own translated runtime;
`Language` stays opaque so a later shared runtime is not a breaking change. `NewParser` checks the grammar module's
engine ABI version and snapshot block magic/version and returns `ErrIncompatibleLanguage` on mismatch. Grammar
`go.mod` files require the exact root version they were generated against; CI tests the oldest supported grammar
module against the newest root. Language metadata (symbol/field names, identity) is static Go data; no instance is
needed to read it.

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

## 6. Snapshot and Node

The snapshot owns the source: by default one copy (about 0.07 ms per MB; +5–22 % of the snapshot's own memory, no
disk use); with `ParseOwned` no copy. Struct-of-arrays, about 41 B/node plus `descendantEnd` and `childCount`
(+8 B/node) for O(1) subtree skip, child access and range lookup.

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
`Index` (ADR-0011) remains for repeated position/kind lookups and gains `OfSymbol`.

## 7. Primitives that set this library apart (v0.0.1)

All are lazy (computed on first use, once, safe for concurrent readers) and measured in T2:

- **Positions and encodings.** `NewSourceUTF16`/BOM detection convert UTF-16 or BOM-prefixed input to UTF-8 and keep
  an offset map to the original bytes; `(*Snapshot).Lines()` returns a `LineIndex` with byte → line and UTF-8,
  UTF-16 and rune columns (build 0.2–1 ns/byte, lookup 10–27 ns; 15 M positions verified). Standard library only.
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
func (*Source) Bytes() []byte; Original() []byte; Encoding() Encoding; OriginalOffset(utf8Off uint32) int
func NewLineIndex(src string) *LineIndex; (*Snapshot) Lines() *LineIndex
func (*LineIndex) NumLines() int; LineStart(line int) uint32; Line(off uint32) int; Pos(off uint32) Position // {Line, ByteCol, UTF16Col, RuneCol}
func (Node) Hash() uint64; ShapeHash() uint64          // comparable within one grammar only (symbol ids are hashed)
func (Node) Atom() Atom; (*Snapshot) AtomText(Atom) string; NumAtoms() int   // named non-empty leaves only
// package experimental/match (not part of the stable treesitter API):
func Match(old, new *treesitter.Snapshot) *Mapping    // EXPERIMENTAL; NewOf/OldOf(Node) (Node, bool), Len; other grammar -> empty
func NewParserPool(lang *Language, maxParsers int, opts ...Option) (*ParserPool, error) // Get(ctx), Put, Close
func NewIndex(*Snapshot) *Index; (*Snapshot) Index() *Index  // OfType, OfSymbol -> iter.Seq[Node]; NodeAt(off uint32) (Node, bool)
```

## 8. Queries (v0.1; signatures fixed now)

`NewQuery(lang, pattern) (*Query, error)` compiles per instance on first use (cache invalidated by reset);
`(*Tree).Matches(ctx, q, opts) iter.Seq2[Match, error]` runs the C engine and maps captures to snapshot nodes by
preorder index (equal to C's descendant index); text predicates (`#eq?`, `#match?`, `#any-of?`) are evaluated in Go
against the snapshot's owned source. A parser is busy while an iterator is open (`Parse` returns an error).

## 9. Out of scope for v0.0.1

Live per-node navigation over the instance, included ranges, shared-runtime linking, legacy encodings (v0.1 `encoding`
package), a compatibility package for other Go tree-sitter APIs (a `MIGRATING.md` name map is written instead).
