# Root API stability

The root 1.x contract covers source compatibility of the stable Go API, its documented behavior with a
fixed released grammar, and build and load compatibility for released engine ABI 1–4 grammar modules.
Grammar module releases can change trees and captures as their source pins change. The root RC and 1.0
gates cover the root contract, packaging, and the root-scoped Release-Critical and Release evidence.
[ADR-0028](https://github.com/wotjr1649/go-treesitter/blob/a646e690016a210700830c741399294751e06dd3/docs/design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md)
· [Validation gates](../specs/validation.md#gates)

## Compatibility rules

| Class | Root 1.x rule |
|---|---|
| STABLE | Preserve compiling consumer code and documented results. Removal, renaming, incompatible type or signature changes, and changes to documented behavior require a root major release. This includes changes to method sets, exported fields, constant values, assignability, comparability, and valid struct literals when they break existing uses. A patch preserves or restores the contract. A minor adds compatible API or behavior. Document fixes that change results with the same grammar. |
| EXPERIMENTAL | Keep the API under `experimental/` or explicitly mark it experimental in package or identifier documentation. It may change or be removed outside a root major release. |
| GENERATOR-ONLY | Reserve the entry point for generated grammars. A change that prevents a released ABI 1–4 grammar from building, loading, or retaining the documented behavior of its ABI requires a root major release. A compatible new ABI can ship in a root minor. |
| DEPRECATE-BEFORE-1.0 | Decide the replacement and migration path before freezing the API. Mark the old export deprecated and either remove it before 1.0 or classify and document it explicitly for 1.x. |

The stable behavior includes error classification with `errors.Is` and `errors.As`, the closed
`Outcome` set, edit and changed-range semantics, ownership and lifetimes, concurrency, encoding and
coordinate conversions, and query validation and execution. Atoms are scoped to a snapshot. Field and
symbol IDs are scoped to the active grammar. Compare subtree hashes within the same grammar and compare
structure when a collision matters. When a new Go major changes the Go team's support window, ADR-0028
allows a root minor to raise the Go floor and update minimum-version CI together.
[Parser result contract](../specs/parser-result.md) · [API design](api-v2.md) · [Query contract](query-abi.md)

## Exported identifiers

Inventory at root commit `6674446f03c6004520567683e5b202881deb4559`. Run `GOWORK=off go list ./...`
at the root and `GOWORK=off go doc -all <package>` for each listed path outside `internal/`. The root
module exposes two such packages. The grammar and tool packages belong to separate Go modules.
Struct fields have their own rows, receiver names are omitted, and the table resolves implicit
constant types and values and inferred sentinel types. Use `go doc -u . sentinel` to include the
exported methods available through the error variables.

### github.com/wotjr1649/go-treesitter

The supported consumer entry point for a language is the grammar module's `Language() *treesitter.Language`.
`NewLanguage` accepts a descriptor from the root's internal engine and is reserved for generated code.
The public `Option` type is passed to parser constructors. Consumers obtain values from the exported
`With…` functions because `config` is package-private. Nine sentinel variables have the inferred
unexported type `*sentinel`. Their identity via `errors.Is` is part of the contract.
The method `(*queryRegex).MatchString(text string) bool` is reachable only inside the package.

| Identifier | Go declaration or signature | Class |
|---|---|---|
| `ErrTimeout` | `var ErrTimeout error` | STABLE |
| `ErrCancelled` | `var ErrCancelled error` | STABLE |
| `ErrResourceLimit` | `var ErrResourceLimit *sentinel` | STABLE |
| `ErrInvalidInput` | `var ErrInvalidInput *sentinel` | STABLE |
| `ErrEngineFault` | `var ErrEngineFault *sentinel` | STABLE |
| `ErrIncompatibleLanguage` | `var ErrIncompatibleLanguage *sentinel` | STABLE |
| `ErrTreeInvalidated` | `var ErrTreeInvalidated *sentinel` | STABLE |
| `ErrTreeEdited` | `var ErrTreeEdited *sentinel` | STABLE |
| `ErrParserClosed` | `var ErrParserClosed *sentinel` | STABLE |
| `ErrQueryUnsupported` | `var ErrQueryUnsupported *sentinel` | STABLE |
| `ErrQueryMatchLimit` | `var ErrQueryMatchLimit *sentinel` | STABLE |
| `sentinel.Error` | `func (*sentinel) Error() string` | STABLE |
| `sentinel.Is` | `func (*sentinel) Is(target error) bool` | STABLE |
| `Atom` | `type Atom uint32` | STABLE |
| `ColumnKind` | `type ColumnKind uint8` | STABLE |
| `ByteColumn` | `const ByteColumn ColumnKind = 1` | STABLE |
| `UTF16Column` | `const UTF16Column ColumnKind = 2` | STABLE |
| `RuneColumn` | `const RuneColumn ColumnKind = 3` | STABLE |
| `ColumnKind.String` | `func (ColumnKind) String() string` | STABLE |
| `Encoding` | `type Encoding uint8` | STABLE |
| `UTF8` | `const UTF8 Encoding = 1` | STABLE |
| `UTF16LE` | `const UTF16LE Encoding = 2` | STABLE |
| `UTF16BE` | `const UTF16BE Encoding = 3` | STABLE |
| `CP949` | `const CP949 Encoding = 4` | STABLE |
| `Encoding.String` | `func (Encoding) String() string` | STABLE |
| `FieldID` | `type FieldID uint16` | STABLE |
| `Index` | `type Index struct` | STABLE |
| `NewIndex` | `func NewIndex(s *Snapshot) *Index` | STABLE |
| `Index.NodeAt` | `func (*Index) NodeAt(offset uint32) (Node, bool)` | STABLE |
| `Index.OfSymbol` | `func (*Index) OfSymbol(sym Symbol) iter.Seq[Node]` | STABLE |
| `Index.OfType` | `func (*Index) OfType(kind string) iter.Seq[Node]` | STABLE |
| `InputEdit` | `type InputEdit struct` | STABLE |
| `InputEdit.StartByte` | `StartByte uint32` | STABLE |
| `InputEdit.OldEndByte` | `OldEndByte uint32` | STABLE |
| `InputEdit.NewEndByte` | `NewEndByte uint32` | STABLE |
| `InputEdit.StartPoint` | `StartPoint Point` | STABLE |
| `InputEdit.OldEndPoint` | `OldEndPoint Point` | STABLE |
| `InputEdit.NewEndPoint` | `NewEndPoint Point` | STABLE |
| `EditFor` | `func EditFor(old, new []byte, start, oldEnd, newEnd uint32) InputEdit` | STABLE |
| `Language` | `type Language struct` | STABLE |
| `NewLanguage` | `func NewLanguage(g *engine.Grammar) *Language` | GENERATOR-ONLY |
| `Language.FieldCount` | `func (*Language) FieldCount() int` | STABLE |
| `Language.FieldForName` | `func (*Language) FieldForName(name string) (FieldID, bool)` | STABLE |
| `Language.FieldName` | `func (*Language) FieldName(f FieldID) string` | STABLE |
| `Language.Name` | `func (*Language) Name() string` | STABLE |
| `Language.SymbolCount` | `func (*Language) SymbolCount() int` | STABLE |
| `Language.SymbolForName` | `func (*Language) SymbolForName(name string, named bool) (Symbol, bool)` | STABLE |
| `Language.SymbolName` | `func (*Language) SymbolName(s Symbol) string` | STABLE |
| `LineIndex` | `type LineIndex struct` | STABLE |
| `NewLineIndex` | `func NewLineIndex(src string) *LineIndex` | STABLE |
| `LineIndex.Line` | `func (*LineIndex) Line(off uint32) int` | STABLE |
| `LineIndex.LineStart` | `func (*LineIndex) LineStart(k int) uint32` | STABLE |
| `LineIndex.NumLines` | `func (*LineIndex) NumLines() int` | STABLE |
| `LineIndex.Offset` | `func (*LineIndex) Offset(line int, col uint32, kind ColumnKind) (uint32, bool)` | STABLE |
| `LineIndex.Pos` | `func (*LineIndex) Pos(off uint32) Position` | STABLE |
| `Node` | `type Node struct` | STABLE |
| `Node.AppendText` | `func (Node) AppendText(dst []byte) []byte` | STABLE |
| `Node.Atom` | `func (Node) Atom() Atom` | STABLE |
| `Node.Child` | `func (Node) Child(i int) (Node, bool)` | STABLE |
| `Node.ChildByFieldName` | `func (Node) ChildByFieldName(name string) (Node, bool)` | STABLE |
| `Node.ChildCount` | `func (Node) ChildCount() int` | STABLE |
| `Node.Children` | `func (Node) Children() iter.Seq[Node]` | STABLE |
| `Node.ChildrenByFieldName` | `func (Node) ChildrenByFieldName(name string) iter.Seq[Node]` | STABLE |
| `Node.EndByte` | `func (Node) EndByte() uint32` | STABLE |
| `Node.EndPoint` | `func (Node) EndPoint() Point` | STABLE |
| `Node.Field` | `func (Node) Field() string` | STABLE |
| `Node.FieldID` | `func (Node) FieldID() FieldID` | STABLE |
| `Node.HasError` | `func (Node) HasError() bool` | STABLE |
| `Node.Hash` | `func (Node) Hash() uint64` | STABLE |
| `Node.Index` | `func (Node) Index() int` | STABLE |
| `Node.IsError` | `func (Node) IsError() bool` | STABLE |
| `Node.IsExtra` | `func (Node) IsExtra() bool` | STABLE |
| `Node.IsMissing` | `func (Node) IsMissing() bool` | STABLE |
| `Node.IsNamed` | `func (Node) IsNamed() bool` | STABLE |
| `Node.IsZero` | `func (Node) IsZero() bool` | STABLE |
| `Node.Kind` | `func (Node) Kind() string` | STABLE |
| `Node.NamedChild` | `func (Node) NamedChild(i int) (Node, bool)` | STABLE |
| `Node.NamedChildCount` | `func (Node) NamedChildCount() int` | STABLE |
| `Node.NamedChildren` | `func (Node) NamedChildren() iter.Seq[Node]` | STABLE |
| `Node.NextNamedSibling` | `func (Node) NextNamedSibling() (Node, bool)` | STABLE |
| `Node.NextSibling` | `func (Node) NextSibling() (Node, bool)` | STABLE |
| `Node.Parent` | `func (Node) Parent() (Node, bool)` | STABLE |
| `Node.PrevNamedSibling` | `func (Node) PrevNamedSibling() (Node, bool)` | STABLE |
| `Node.PrevSibling` | `func (Node) PrevSibling() (Node, bool)` | STABLE |
| `Node.ShapeHash` | `func (Node) ShapeHash() uint64` | STABLE |
| `Node.Snapshot` | `func (Node) Snapshot() *Snapshot` | STABLE |
| `Node.StartByte` | `func (Node) StartByte() uint32` | STABLE |
| `Node.StartPoint` | `func (Node) StartPoint() Point` | STABLE |
| `Node.String` | `func (Node) String() string` | STABLE |
| `Node.Symbol` | `func (Node) Symbol() Symbol` | STABLE |
| `Node.Text` | `func (Node) Text() string` | STABLE |
| `Option` | `type Option func(*config) error` | STABLE |
| `WithMaxInputBytes` | `func WithMaxInputBytes(n int64) Option` | STABLE |
| `WithMaxMemoryBytes` | `func WithMaxMemoryBytes(n int64) Option` | STABLE |
| `WithMaxSnapshotNodes` | `func WithMaxSnapshotNodes(n int64) Option` | STABLE |
| `WithRecycleAboveBytes` | `func WithRecycleAboveBytes(n int64) Option` | STABLE |
| `WithStrictUTF8` | `func WithStrictUTF8() Option` | STABLE |
| `Outcome` | `type Outcome uint8` | STABLE |
| `AcceptedClean` | `const AcceptedClean Outcome = 1` | STABLE |
| `AcceptedWithErrors` | `const AcceptedWithErrors Outcome = 2` | STABLE |
| `Timeout` | `const Timeout Outcome = 3` | STABLE |
| `Cancelled` | `const Cancelled Outcome = 4` | STABLE |
| `ResourceLimit` | `const ResourceLimit Outcome = 5` | STABLE |
| `InvalidInput` | `const InvalidInput Outcome = 6` | STABLE |
| `EngineFault` | `const EngineFault Outcome = 7` | STABLE |
| `IncompatibleLanguage` | `const IncompatibleLanguage Outcome = 8` | STABLE |
| `Outcome.String` | `func (Outcome) String() string` | STABLE |
| `ParseError` | `type ParseError struct` | STABLE |
| `ParseError.Outcome` | `Outcome Outcome` | STABLE |
| `ParseError.Reason` | `Reason string` | STABLE |
| `ParseError.InstanceReset` | `InstanceReset bool` | STABLE |
| `ParseError.Error` | `func (*ParseError) Error() string` | STABLE |
| `ParseError.Is` | `func (*ParseError) Is(target error) bool` | STABLE |
| `ParseError.Unwrap` | `func (*ParseError) Unwrap() error` | STABLE |
| `Parser` | `type Parser struct` | STABLE |
| `NewParser` | `func NewParser(lang *Language, opts ...Option) (*Parser, error)` | STABLE |
| `Parser.Close` | `func (*Parser) Close() error` | STABLE |
| `Parser.Parse` | `func (*Parser) Parse(ctx context.Context, src []byte, old *Tree) (*Tree, error)` | STABLE |
| `Parser.ParseOwned` | `func (*Parser) ParseOwned(ctx context.Context, src []byte, old *Tree) (*Tree, error)` | STABLE |
| `Parser.ParseSnapshot` | `func (*Parser) ParseSnapshot(ctx context.Context, src []byte) (*Snapshot, error)` | STABLE |
| `Parser.Reset` | `func (*Parser) Reset() error` | STABLE |
| `ParserPool` | `type ParserPool struct` | STABLE |
| `NewParserPool` | `func NewParserPool(lang *Language, maxParsers int, opts ...Option) (*ParserPool, error)` | STABLE |
| `ParserPool.Close` | `func (*ParserPool) Close()` | STABLE |
| `ParserPool.Get` | `func (*ParserPool) Get(ctx context.Context) (*Parser, error)` | STABLE |
| `ParserPool.Put` | `func (*ParserPool) Put(p *Parser)` | STABLE |
| `Point` | `type Point struct` | STABLE |
| `Point.Row` | `Row uint32` | STABLE |
| `Point.Column` | `Column uint32` | STABLE |
| `Position` | `type Position struct` | STABLE |
| `Position.Line` | `Line int` | STABLE |
| `Position.ByteCol` | `ByteCol uint32` | STABLE |
| `Position.UTF16Col` | `UTF16Col uint32` | STABLE |
| `Position.RuneCol` | `RuneCol uint32` | STABLE |
| `Query` | `type Query struct` | STABLE |
| `NewQuery` | `func NewQuery(lang *Language, pattern string) (*Query, error)` | STABLE |
| `Query.CaptureNames` | `func (*Query) CaptureNames() []string` | STABLE |
| `Query.PatternCount` | `func (*Query) PatternCount() int` | STABLE |
| `Query.Properties` | `func (*Query) Properties(pattern int) map[string]string` | STABLE |
| `QueryCapture` | `type QueryCapture struct` | STABLE |
| `QueryCapture.Index` | `Index uint32` | STABLE |
| `QueryCapture.Node` | `Node Node` | STABLE |
| `QueryError` | `type QueryError struct` | STABLE |
| `QueryError.Offset` | `Offset uint32` | STABLE |
| `QueryError.Type` | `Type QueryErrorType` | STABLE |
| `QueryError.Error` | `func (*QueryError) Error() string` | STABLE |
| `QueryErrorType` | `type QueryErrorType uint32` | STABLE |
| `QueryErrorSyntax` | `const QueryErrorSyntax QueryErrorType = 1` | STABLE |
| `QueryErrorNodeType` | `const QueryErrorNodeType QueryErrorType = 2` | STABLE |
| `QueryErrorField` | `const QueryErrorField QueryErrorType = 3` | STABLE |
| `QueryErrorCapture` | `const QueryErrorCapture QueryErrorType = 4` | STABLE |
| `QueryErrorStructure` | `const QueryErrorStructure QueryErrorType = 5` | STABLE |
| `QueryErrorLanguage` | `const QueryErrorLanguage QueryErrorType = 6` | STABLE |
| `QueryErrorPredicate` | `const QueryErrorPredicate QueryErrorType = 7` | STABLE |
| `QueryMatch` | `type QueryMatch struct` | STABLE |
| `QueryMatch.Pattern` | `Pattern int` | STABLE |
| `QueryMatch.Captures` | `Captures []QueryCapture` | STABLE |
| `QueryOptions` | `type QueryOptions struct` | STABLE |
| `QueryOptions.StartByte` | `StartByte uint32` | STABLE |
| `QueryOptions.EndByte` | `EndByte uint32` | STABLE |
| `QueryOptions.MatchLimit` | `MatchLimit uint32` | STABLE |
| `Range` | `type Range struct` | STABLE |
| `Range.StartByte` | `StartByte uint32` | STABLE |
| `Range.EndByte` | `EndByte uint32` | STABLE |
| `Range.StartPoint` | `StartPoint Point` | STABLE |
| `Range.EndPoint` | `EndPoint Point` | STABLE |
| `Snapshot` | `type Snapshot struct` | STABLE |
| `Snapshot.All` | `func (*Snapshot) All() iter.Seq[Node]` | STABLE |
| `Snapshot.AtomText` | `func (*Snapshot) AtomText(a Atom) string` | STABLE |
| `Snapshot.DescendantForByteRange` | `func (*Snapshot) DescendantForByteRange(start, end uint32, named bool) Node` | STABLE |
| `Snapshot.DescendantForPointRange` | `func (*Snapshot) DescendantForPointRange(start, end Point, named bool) Node` | STABLE |
| `Snapshot.HasInvalidUTF8` | `func (*Snapshot) HasInvalidUTF8() bool` | STABLE |
| `Snapshot.Index` | `func (*Snapshot) Index() *Index` | STABLE |
| `Snapshot.Language` | `func (*Snapshot) Language() *Language` | STABLE |
| `Snapshot.Len` | `func (*Snapshot) Len() int` | STABLE |
| `Snapshot.Lines` | `func (*Snapshot) Lines() *LineIndex` | STABLE |
| `Snapshot.Node` | `func (*Snapshot) Node(i int) Node` | STABLE |
| `Snapshot.NumAtoms` | `func (*Snapshot) NumAtoms() int` | STABLE |
| `Snapshot.Root` | `func (*Snapshot) Root() Node` | STABLE |
| `Snapshot.Source` | `func (*Snapshot) Source() string` | STABLE |
| `Snapshot.Walk` | `func (*Snapshot) Walk(fn func(n Node, depth int) WalkAction)` | STABLE |
| `Source` | `type Source struct` | STABLE |
| `NewSource` | `func NewSource(raw []byte) (*Source, error)` | STABLE |
| `NewSourceCP949` | `func NewSourceCP949(raw []byte) (*Source, error)` | STABLE |
| `NewSourceUTF16` | `func NewSourceUTF16(raw []byte, bigEndian bool) (*Source, error)` | STABLE |
| `Source.Bytes` | `func (*Source) Bytes() []byte` | STABLE |
| `Source.Encoding` | `func (*Source) Encoding() Encoding` | STABLE |
| `Source.Original` | `func (*Source) Original() []byte` | STABLE |
| `Source.OriginalOffset` | `func (*Source) OriginalOffset(off uint32) int` | STABLE |
| `Source.Replacements` | `func (*Source) Replacements() int` | STABLE |
| `Source.UTF8Offset` | `func (*Source) UTF8Offset(orig int) uint32` | STABLE |
| `Symbol` | `type Symbol uint16` | STABLE |
| `ErrorSymbol` | `const ErrorSymbol Symbol = 0xFFFF` | STABLE |
| `Tree` | `type Tree struct` | STABLE |
| `Tree.ChangedRanges` | `func (*Tree) ChangedRanges(old *Tree) ([]Range, error)` | STABLE |
| `Tree.Close` | `func (*Tree) Close()` | STABLE |
| `Tree.Edit` | `func (*Tree) Edit(e InputEdit) error` | STABLE |
| `Tree.HasInvalidUTF8` | `func (*Tree) HasInvalidUTF8() bool` | STABLE |
| `Tree.InputBytes` | `func (*Tree) InputBytes() int` | STABLE |
| `Tree.Matches` | `func (*Tree) Matches(ctx context.Context, q *Query, opts *QueryOptions) iter.Seq2[QueryMatch, error]` | STABLE |
| `Tree.Outcome` | `func (*Tree) Outcome() Outcome` | STABLE |
| `Tree.Snapshot` | `func (*Tree) Snapshot(ctx context.Context) (*Snapshot, error)` | STABLE |
| `WalkAction` | `type WalkAction uint8` | STABLE |
| `Continue` | `const Continue WalkAction = 0` | STABLE |
| `SkipChildren` | `const SkipChildren WalkAction = 1` | STABLE |
| `Stop` | `const Stop WalkAction = 2` | STABLE |

### github.com/wotjr1649/go-treesitter/experimental/match

The package documentation already marks this API experimental.

| Identifier | Go declaration or signature | Class |
|---|---|---|
| `Mapping` | `type Mapping struct` | EXPERIMENTAL |
| `Match` | `func Match(old, new *treesitter.Snapshot) *Mapping` | EXPERIMENTAL |
| `Mapping.Len` | `func (*Mapping) Len() int` | EXPERIMENTAL |
| `Mapping.NewOf` | `func (*Mapping) NewOf(n treesitter.Node) (treesitter.Node, bool)` | EXPERIMENTAL |
| `Mapping.OldOf` | `func (*Mapping) OldOf(n treesitter.Node) (treesitter.Node, bool)` | EXPERIMENTAL |

## Decisions before RC

- Confirm the classification of the full inventory and whether to deprecate any exports before 1.0.
  The current inventory assigns every consumer export in the root package to STABLE and has no
  DEPRECATE-BEFORE-1.0 candidates.
- Confirm the compatibility limits for exact numeric hashes and diagnostics. The current APIs document
  hash equality and comparison scope, error classification, and diagnostic fields. Decide whether any
  additional cross-release hash representation or message text becomes contractual.
- Review the historical wording in API v2 before using it as the 1.x reference. Its header
  describes the pre-tag design. Section 2 still describes lockstep grammar requirements and claims CI for
  the oldest published grammar. ADR-0028 defines independent requirements, and the external consumer job
  below is planned. Reconcile the remaining parsing and lifetime summaries with the parser result
  contract when freezing the behavior. [API v2](api-v2.md) · [Parser result contract](../specs/parser-result.md)

## Compatibility test plan

### Published grammar consumers

Create a disposable Go module outside the root and grammar module paths. Import the root and each
grammar through their public packages. Resolve exact grammar versions from the public Go proxy and
checksum database into empty module caches. Use `CGO_ENABLED=0`, `GOWORK=off`, `GOENV=off`, and
`GOTOOLCHAIN=local`. For PRs and pre-tag rehearsals, serve the candidate root commit from a file
`GOPROXY` at a private test version that sorts above the fixture requirements. Construct its ZIP at that
version with the repository's module ZIP procedure.
Record the local root ZIP and `go.mod` hashes. Authenticate the unchanged published grammar modules
with their public checksums before mirroring them if the rehearsal runs with checksum lookup disabled.
For the published RC and final root, repeat with the exact root tag and public checksums, exercising
both direct and proxy resolution as required by publication. [Module ZIP procedure](../specs/publication.md#multi-module-release)

Require the candidate root in each ABI row. Use separate consumers for different versions of the
same grammar. Each consumer must build and run without `replace` directives. Record
`go list -m -json all` after resolution and assert the selected root, each grammar version, and the
absence of replacements. Check the grammar module's embedded engine ABI and root requirement
against the tagged source when picking the fixtures.

| Engine ABI | Published grammar tags | Root required by those tags | What the external consumer asserts |
|---|---|---|---|
| 1 | `grammars/json/v0.1.0` | `v0.1.0` | A complete parse and snapshot, legacy direct-child field lookup and raw symbol lookup. `NewQuery` returns a nil query and matches `ErrQueryUnsupported` with `errors.Is`. |
| 2 | `grammars/json/v0.2.0`, `grammars/cpp/v0.3.0` | `v0.2.0`, `v0.3.0` | Parse and query successfully. Assert legacy direct-child field lookup and raw symbol lookup. Use the C++ fixture as the legacy control for ABI 3's alias-inherited field. |
| 3 | `grammars/cpp/v0.4.0` | `v0.4.0` | Parse and query successfully. Assert that an alias-inherited field resolves to the recorded descendant and that it leads the field iterator exactly once. Retain raw symbol lookup. |
| 4 | `grammars/golang/v0.6.0` | `v0.6.0` | Parse and query successfully. Assert that `SymbolForName` maps a raw-to-public alias to the recorded public ID and that `Node.Symbol`, `SymbolName` and `Index.OfSymbol` agree for that ID. Preserve ABI 3 field behavior. |

Choose a C++ input with a field exception recorded by the existing field-lookup lane. Pin the input and
the expected legacy and exception results for the two released C++ tags, which share the same
grammar source revision. For ABI 4, use a Go call
such as `g(1)`. The tagged metadata maps the first named `argument_list` raw ID (173) to public ID 175.
Keep symbol expectations scoped to the tag.
For every supported query row, compare pattern number, capture names in order, and captured kinds and
byte ranges on fixed inputs. Require an invalid query to return a `*QueryError` from `NewQuery` and
assert that breaking from the iterator leaves the parser available for another operation.
Reuse the existing root tests for documented behavior such as cancellation and retry, edit and tree
invalidation, snapshot ownership, and coordinate conversion. Include an incremental JSON edit and
changed-range assertion in the consumer to cover the released-module path.

### Mixed versions, minimum Go and rejection

| Case | Test |
|---|---|
| MVS raises the root | Start a consumer with JSON `v0.1.0` and its root minimum. Add Go grammar `v0.6.0` without explicitly upgrading the root. Assert that MVS selects root `v0.6.0` and retains the two exact grammar tags. Check parsing for both, ABI 1 query rejection and ABI 4 query success. |
| MVS with the candidate | In that same dependency graph require the candidate root version explicitly. Assert that it wins over both grammar requirements without changing either grammar version. Repeat the runtime checks. Add each newly released grammar paired with a carried-forward older tag when recording release combinations. |
| Minimum Go | Build and run the ABI and mixed-version consumers on Linux amd64 with Go 1.26.0, the floor in `go.mod`. Disable automatic toolchain switching and record `go version`. Cross-build the release consumer for Linux arm64 as link evidence and record native execution only for the runners that execute it. |
| Too-new ABI or unsupported format | Keep the root's synthetic fixtures for a grammar descriptor at `engine.ABIVersion + 1`, a module ABI mismatch, and unsupported snapshot magic or format. Assert that `NewParser` returns a nil parser and an error matching `ErrIncompatibleLanguage`. For a too-new descriptor assert that no instance was created. Assert release of instances rejected after creation. |

### CI and evidence

The `engine-consumer-compat` job runs the representative ABI rows and mixed-version cases on Linux amd64
at the Go floor for pull requests, pushes to main, and manual candidate runs. It also cross-builds the
mixed candidate for Linux arm64. The existing root jobs run synthetic ABI rejection tests using fake
engine modules. For an RC or final release, also run an empty-cache consumer against
all 25 published `grammars/*/v0.6.0` modules plus any additional grammar tags in the release combination
table. The published-tags run follows the publication procedure. The candidate job is part of the
root Release-Critical gate. [Engine workflow](../../.github/workflows/engine.yml)

The existing `engine`, `engine-go126`, and `grammar` jobs cover the checkout and its embedded grammar
samples. `TestIncompatibleLanguage`, `TestFieldLookupLegacyABI`, `TestFieldLookupCurrentABIRejectsFormat1`,
`TestQueryABI1Compatibility` and `TestSymbolForNamePublicAlias` cover focused cases with synthetic
modules. The release status board records earlier empty-cache packaging runs, including the v0.2.0
rehearsal with an ABI 1 grammar, and the published v0.6.0 sweep. The candidate job extends that coverage
to historical tags and mixed versions against each new root. Its artifact retains the consumer sources,
module graphs and sums, candidate and grammar identities, and execution log even when a consumer fails.
[Parser tests](../../parser_v2_test.go) · [Field tests](../../field_lookup_test.go) · [Query tests](../../query_test.go)
· [Symbol tests](../../node_test.go) · [Release status board](../reports/release-critical-status.md#released)

Record the root commit/version, grammar commits/tags and pin or generator identities, ABI, module ZIP
and `go.mod` sums, selected dependency graph, input and query hashes, expectations, Go version, platform,
and commands and results. Retain failures and their artifacts. The consumer results provide E3 evidence
for the named combinations. Publication and the other root-scoped gates supply the remaining release
evidence. [Evidence levels](../specs/validation.md#evidence-levels)
