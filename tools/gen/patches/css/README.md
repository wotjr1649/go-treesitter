# Temporary CSS name patch

`0001-non-latin1-names.patch` changes only `grammar.js` at the pinned source and follows
[upstream PR #105](https://github.com/tree-sitter/tree-sitter-css/pull/105). It accepts non-Latin-1 names in
selectors, property names, custom property references and keyframe names. The added code-point ranges follow
[CSS Syntax Level 3](https://www.w3.org/TR/2026/CRD-css-syntax-3-20261001/#non-ascii-ident-code-point).
The existing identifier token continues to serve ASCII and Latin-1 names. The patch corpus covers Hangul,
mixed-script names, an escape followed by Hangul, and ASCII and Latin-1 controls. The
[bare Hangul declaration value](../../../../docs/known-gaps/css.md#css-non-latin1-value) has a separate record.

Retire this patch with an approved pin that carries the same name fix. If PR #105 closes without a fix or
the patch has been on main for 12 months, the owner reviews it under
[ADR-0023](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md).
