# Temporary CSS grammar patches

## 0001: Non-Latin-1 names

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
Review and update the affected corpus expectations and known-gap register in that pin move.
If PR #105 closes without a fix or the patch has been on main for 12 months, the owner reviews it under
[ADR-0023](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md).

## 0002: Non-Latin-1 declaration value

The owner approved this temporary ADR-0023 patch on 2026-11-25 at 20:15 KST.
[The report on upstream PR #105](https://github.com/tree-sitter/tree-sitter-css/pull/105#issuecomment-7140588000) records the bare Hangul value failure.
Apply `0002-non-latin1-value.patch` to `grammar.js` after 0001. This adapts the PR's identifier
extension to the pinned declaration grammar. The added token starts beyond Latin-1, optionally after
a hyphen, and continues with CSS Syntax Level 3 identifier code points. Existing values such as `é`
and `a한` keep their lexer path. Escapes in the added token are outside the patch scope.
`a { b: 한글 }` now contains a `declaration` with a `plain_value`. The separate alternative keeps
`1한` from becoming a number followed by a value, and `한글(abc)` from becoming a value followed by a
parenthesized expression. The retained corpus also covers the terminated form, `!important`,
supplementary-plane characters, and controls for the existing value lexer.

Retire 0002 on an approved pin move that correctly parses the retained declaration fixture. Review
0001 and 0002 independently and update the affected corpus expectations and known-gap records.
If 0001 retires first, adapt 0002 to the new pinned identifier ranges.
If PR #105 closes without the value fix or 12 months pass after the value patch first reaches main,
the owner re-decides it under ADR-0023.
