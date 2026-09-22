# Public upstream review, 2026-09-22

Public-only research used original GitHub issue/release pages. Reports at other
versions, architectures or API surfaces are not observations of this product.
The release page still identifies [v0.53.0 as latest](https://github.com/odvcencio/gotreesitter/releases).

| Source | Finding and disposition |
|---|---|
| [gotreesitter #1242](https://github.com/odvcencio/gotreesitter/issues/1242) | Open report combines `&` and `=` and compares to TypeScript's parser. Our pinned C experiments separate them: preserve bare `&` errors; evaluate the `=` candidate independently. |
| [gotreesitter #1108 / PR #1112](https://github.com/odvcencio/gotreesitter/pull/1112) | Merged September 16; local ancestry check places the merge in the baseline. The report expressly compares patched C. Newline-sensitive local probes expose KR-0003 against our original C sources. |
| [gotreesitter #454](https://github.com/odvcencio/gotreesitter/issues/454) | Windows ARM64 field report uses v0.47.0. Its C# nondeterminism and timing reports are not transferred to v0.53.0. Native ARM64 execution remains required separately. |
| [gotreesitter #1056](https://github.com/odvcencio/gotreesitter/issues/1056) | EOF sentinel/Accept handling has a Scala witness at the same C epoch. Local EOF ablation did not change the ten C# controls, so it is not the demonstrated cause here. Scala is outside the fixed release scope. |
| [gotreesitter #1057](https://github.com/odvcencio/gotreesitter/issues/1057) | Compact reduced-head scheduling work uses Scala witnesses. It is relevant architecture context, not an executed seven-route regression or authorization to replace the scheduler. |
| [tree-sitter #5953](https://github.com/tree-sitter/tree-sitter/issues/5953) | Generator memory report concerns v0.27/current master and a large grammar. Keep generator processes bounded and identify each generated artifact; no corresponding product runtime defect was established. |
| [tree-sitter #5951](https://github.com/tree-sitter/tree-sitter/issues/5951) | C query capture-pool lookup cost. This adapter does not expose that C query engine; it is not evidence about the new Go snapshot index. |
| [tree-sitter #5950](https://github.com/tree-sitter/tree-sitter/issues/5950) | Backward cursor sibling boundaries. Our API exposes forward snapshots and index lookups. Wide forward checks do not claim to repair the backward C API. |
| [tree-sitter #5948](https://github.com/tree-sitter/tree-sitter/issues/5948) | Direct field lookup can cross a visible alias. Snapshot edge fields and direct child_by_field APIs are distinct; do not claim that this adapter repairs the latter. |
| [tree-sitter #5925](https://github.com/tree-sitter/tree-sitter/issues/5925) | Keyword tokenization under lexical conflicts is a newer grammar/generator discussion, not grounds for migrating the pinned epoch. |
| [tree-sitter #5810](https://github.com/tree-sitter/tree-sitter/issues/5810) | Closed Rust QueryCursor callback lifetime issue at 0.26.11. It is not a deep-recursion report and does not describe this Go adapter. An earlier research summary misclassified it; the primary page corrects that classification. |

Compact graduation tracking tickets and Swift-specific work do not establish
new defects in the seven claimed routes by their titles. A follow-up fetch of
#1061 timed out; no conclusion here depends on its unexamined body. No public
issue was filed, commented on, closed or otherwise modified.
