# Temporary Python patches

`0001-bound-serialized-indent-bytes.patch` changes `src/scanner.c` after generation; see
[upstream issue #350](https://github.com/tree-sitter/tree-sitter-python/issues/350).

`0002-await-power-precedence.patch` changes only `grammar.js` at the pinned source. It applies the grammar change of
[upstream PR #360](https://github.com/tree-sitter/tree-sitter-python/pull/360) to the pinned commit, without that pull
request's regenerated files: `await` gets its own precedence between the power operator and calls, attributes and
subscripts. `await f() ** 2` therefore groups as `(await f()) ** 2`, as the Python reference reads it
([6.4 await](https://docs.python.org/3/reference/expressions.html#await-expression),
[6.5 power](https://docs.python.org/3/reference/expressions.html#the-power-operator)). The patch changes no node type or field.
Previously error-free trees change where an `await` operand was followed by `**`; the
[migration notes](../../../../MIGRATING.md#python-await-and-the-power-operator) list them. The patch corpus and the
generator expectations record the changed inputs.

Retire `0002` in the approved pin move to an upstream source containing the fix for the reported defect. Review and
update the patch corpus expectations in that pin move.
If PR #360 closes without a fix or the patch has been on main for 12 months, the owner reviews it under
[ADR-0023](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md).
