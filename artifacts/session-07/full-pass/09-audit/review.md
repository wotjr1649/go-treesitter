# Session 07 source and upstream review

Reviewed on 2026-09-23. Public issue status is bound in `public-sources.json`.
An upstream report is a hypothesis until the owned candidate reproduces it.
This review covers the parsing and indexing boundary, not downstream semantic
resolution. No upstream checkout or downstream file was changed.

## External reports

| Source | Finding and disposition |
|---|---|
| [gotreesitter #1064](https://github.com/odvcencio/gotreesitter/issues/1064) | Methodology: bind C runtime, grammar, generated tables, inputs and ordered node fields. Applied to the retained seven-route corpus. Generic compact admission is not a transferred compact certificate. |
| [#1065](https://github.com/odvcencio/gotreesitter/issues/1065) | Methodology: correctness before performance, memory and race gates. Our five-seed local paired measurements are explicitly distinct from upstream's larger release campaign. |
| [#454](https://github.com/odvcencio/gotreesitter/issues/454) | The older Windows ARM64 field report motivates fresh/edit/repeated recovery checks. Owned C# witnesses reproduced R2 ordering and false-clean reconstruction defects; the carried KR-0002/4 patches close those retained witnesses. Native ARM64 remains unrun. |
| [#728](https://github.com/odvcencio/gotreesitter/issues/728) | Closed upstream external-scanner reuse report. Correct fresh fallback is R1, not an AST agreement claim. C# retains its explicit scanner fallback; JS/TSX stateless scanner reuse is measured separately. |
| [#1242](https://github.com/odvcencio/gotreesitter/issues/1242) | Owned `=` text witnesses reproduced R2 and are fixed by KR-0001b. Bare `&` remains erroneous under the pinned C grammar, with exact recovered-tree comparison now required. |
| [#1105](https://github.com/odvcencio/gotreesitter/pull/1105) | Independent public Go C-table migration work corroborates the grammar-artifact boundary. We reproduce the existing pinned C input with the pinned converter; no moving PR branch is imported. Our EOF, keyword, ordering and large-edit witnesses determine applicability. |
| [#1232](https://github.com/odvcencio/gotreesitter/pull/1232) | Hidden missing-node loss is a concrete hazard. Owned Go/C# controls exercise hidden raw cost and error preservation. The external Blade/KDL examples are outside the seven-route E5 scope; their exact C parity is not claimed. |
| [#1221](https://github.com/odvcencio/gotreesitter/pull/1221) | Accepted-version election/culling hypothesis reviewed against the carried C# ordering transaction and retained reduced controls. No additional unexplained owned-corpus mismatch was observed. Rust/CSS/D recovery parity is unassessed. |
| [#1235](https://github.com/odvcencio/gotreesitter/pull/1235) | Perl compact external relex is outside the strict seven-route scope. The 206 basic checks cannot establish malformed Perl C parity. No speculative route widening was adopted. |
| [official Go binding #52](https://github.com/tree-sitter/go-tree-sitter/issues/52), [#36](https://github.com/tree-sitter/go-tree-sitter/issues/36) | RSS retention reports concern another binding, R0 for direct code applicability. They motivated separate live-heap, RSS and explicit tree-lifetime measurements here. |
| [official Go binding #32](https://github.com/tree-sitter/go-tree-sitter/issues/32), [#35](https://github.com/tree-sitter/go-tree-sitter/issues/35), [#43](https://github.com/tree-sitter/go-tree-sitter/issues/43), [#55](https://github.com/tree-sitter/go-tree-sitter/issues/55), [#56](https://github.com/tree-sitter/go-tree-sitter/pull/56), [#58](https://github.com/tree-sitter/go-tree-sitter/pull/58) | cgo callbacks, allocator trampolines and options-handle lifetimes do not occur in this public pure-Go API: R0. No foreign binding code was copied. |

Other recent open reports in the metadata receipt include external-symbol
binding, COBOL recovery and reusable fact buffers. Their existence does not
establish a bug here. The basic catalog and the retained C/C++ lexer and COBOL
completion controls supply only their stated evidence levels.

## Owned source review

| Surface | Evidence, change or limitation |
|---|---|
| Lexer/EOF | Explicit EOF transitions precede rune-zero transitions; cycles are bounded. Contiguous and included-range paths share the rule. Literal NUL terminals are retained, including zero-width EOF. |
| Incremental reuse | Unauthenticated interior lookahead dependencies replay leaves. A source-start skipped-prefix proof must agree. Conflicting leaf action cells return to normal dispatch. The 8,192-declaration witness fails before the latter fix and preserves real reuse after it. |
| Recovery and reconstruction | Ordered C records cover EOF closure, reserved keywords, different version token widths, missing costs, acceptance order and real C# excerpts. Raw ledger information is preserved instead of manufacturing a clean source-derived tree. |
| Cost/cache lifetime | Existing bounded node/version memo is reused. The final child shortcut requires matching arena and captured raw shape as well as node version. A private negative control changes the live node while an older parent retains its earlier shape. |
| Memory | No new global pool. Tree close counts, post-GC heap and process RSS are distinct measurements. The memory budget counts reported parser arena/scratch, not a hard RSS limit or all source/snapshot/cache allocations. |
| Cancellation/concurrency | Independent parser calls and immutable index readers are supported. Trees remain worker-local. Recovery, parsing and snapshot cancellation tests run with separate race diagnostics. |
| Index/search | `Index` builds in O(n log n), owns O(n) offsets/type entries and yields type matches without a result allocation. Position queries prune disjoint intervals; overlapping ranges retain their documented O(n) worst case. The anonymous Go NUL compatibility failure is fixed without relaxing ranges or parent links. CPU profiling confirms position work is in `NodeAt`; no downstream search abstraction was added. |
| Package boundary | Public API uses owned types; only adapter packages import the carrier. Default 203 and optional GPL 3 are separated at module boundaries. Artifact/source/origin hashes remain distinct. |

Read-only downstream review covered `contracts/parser.go`, the complete parser
and source-index implementation, and Go/Python/web/C# parse callers and cleanup
paths in `code-map-memo/internal/codegraph`. Those callers still import the old
runtime directly, maintain source-to-UTF16 indexing and impose downstream
semantic policies. Their migration and semantic query/storage algorithms are
outside this repository. This work does not claim that the downstream product
has adopted or validated this API.

The local reference runtimes, grammar inputs and C implementation were used as
read-only evidence for the lexer/recovery comparisons. Their source identity
is recorded by the conversion, native C build and runtime reproduction receipts.

## Review limits and dispositions

Reproduced owned correctness/resource defects were retained and corrected;
the final candidate must rerun their checks after its last runtime change.
Remaining controlled fallback/performance costs are R1 when complete trees
remain identical. License declarations and native/hosted execution are separate
release blockers. No claim extends the finite E5 corpus to every program or
to all 206 grammars.

The primary agent reviewed the complete production diff, provenance, tests and
failure paths in separate passes. Earlier public-only C# and upstream reviews
provided an independent algorithm review. A second agent did not receive the
private repository diff: available delegate capabilities did not supply the
structural private-data/egress isolation required by S6. That independent
private-diff review is not claimed.
