# Temporary CSS name patch

`0001-non-latin1-names.patch` changes only `grammar.js` at the pinned source. It independently addresses
the name defect reported in [upstream PR #105](https://github.com/tree-sitter/tree-sitter-css/pull/105).
It extends class, ID, type and attribute names, property names, custom property references and keyframe names.
The added code-point ranges follow [CSS Syntax Level 3](https://www.w3.org/TR/2026/CRD-css-syntax-3-20261001/#non-ascii-ident-code-point).
Extended class names require adjacent tokens, including at escapes. The legacy class continuation keeps its
Latin-1 range so that a widened name cannot be consumed across whitespace when the pinned scanner fails to
emit a descendant operator. An input such as `.a 🙂 {}` therefore retains its parse error.
The patch corpus covers combinators, nesting, these scanner limits, mixed-script and escaped names, and
ASCII and Latin-1 controls. The [migration notes](../../../../MIGRATING.md#css-non-latin-1-names) list
positions outside the patch scope.

Retire the patch in the approved pin move to an upstream source containing a fix for the reported name defect.
Review and update the affected corpus expectations and known-gap register in that pin move, including the
[bare Hangul declaration value](../../../../docs/known-gaps/css.md#css-non-latin1-value) if the new pin fixes it.
If PR #105 closes without a fix or the patch has been on main for 12 months, the owner reviews it under
[ADR-0023](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md).
