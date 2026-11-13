# Parser result and consumer contract

Normative for the `treesitter` package (API v2, ADR-0021). Signatures and rationale: `docs/design/api-v2.md`; engine
side: `docs/design/engine-abi.md`. Where this document and the code disagree, the code is a defect or this document
is; fix one of them, never leave both.

Transition: the old `syntax` package, `treesitter.New` and `internal/gtsadapter` remain until their approved removal
(ADR-0020 §7). They follow the previous contract, kept in Git history at commit `9ad82eb`; nothing below applies to them.

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
| `InvalidInput` — nil ctx, invalid option value (never clamped), `old` from another parser or invalidated, non-UTF-8 with `WithStrictUTF8` | no | `ErrInvalidInput` | kept |
| `EngineFault` — any other trap, root-end mismatch, an ABI violation by the module | no | `ErrEngineFault` | recreated (`InstanceReset`) |
| `IncompatibleLanguage` — engine ABI version or export-block magic/format mismatch | no parser | `ErrIncompatibleLanguage` | none created |

Adding a member is a contract change and needs an ADR. Lifetime errors are separate sentinels: `ErrTreeInvalidated`,
`ErrTreeEdited`, `ErrParserClosed`.

## Required evaluation order

nil or already-done `ctx` → option and input checks (size, strict UTF-8, ownership and validity of `old`) → engine.
Checks before the engine never touch the instance. Cancellation during the engine call is detected in the progress
callback; the call returns after the C parser's cleanup.

## Input

Bytes are parsed like C tree-sitter (UTF-8; a leading BOM is skipped; invalid bytes become decode errors in the
tree). `HasInvalidUTF8` reports `!utf8.Valid(src)`. `NewSource`/`NewSourceUTF16` convert UTF-16 or BOM-prefixed input
to UTF-8 and map offsets back to the original bytes.

## Ownership, lifetime, concurrency

- A `Parser` and its `Tree`s are one confinement unit: one goroutine at a time. Concurrent use is detected and
  rejected, not serialized.
- `Close`, `Reset`, an instance reset and `ParserPool.Put` invalidate the parser's live trees (`ErrTreeInvalidated`).
- `Tree.Edit` mutates in place (C semantics); the tree is then only valid as `old` of the next parse, and
  `Snapshot` returns `ErrTreeEdited`. Take snapshots before editing.
- A leaked tree or parser is cleaned up by queueing; only the owning goroutine calls into the engine.
- `Snapshot`, `Node`, `Index`, `LineIndex` are immutable and safe for any number of goroutines. A snapshot owns its
  source (one copy; none with `ParseOwned`, whose caller must never modify the bytes again) and stays valid after the
  parser is closed. `Node.Text` is a zero-allocation substring of that source.
- The zero `Node` is C's null node (`IsZero`); other methods on it panic.

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
