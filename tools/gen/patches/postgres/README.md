# PostgreSQL Korean support patch retirement

The PostgreSQL SQL pin moved from upstream `v19.0.0-beta.4.1`
(`59d0d8cd7506d68de1229fb4bbce838c83b60c8a`) with two ADR-0023 patches to
[upstream v19.0.0](https://github.com/gmr/tree-sitter-postgres/releases/tag/v19.0.0)
(`83c0cc2f4401d5e5944834a09d3f8c24184398fa`). The owner approved the move and
retirement on 2026-11-23. [Upstream PR #102](https://github.com/gmr/tree-sitter-postgres/pull/102)
fixed both defects from [upstream issue #101](https://github.com/gmr/tree-sitter-postgres/issues/101).

- `0001-non-ascii-identifiers` widened unquoted identifiers to all non-ASCII
  code points, preserving ASCII identifier rules. Upstream updated both
  `postgres/grammar.js` and the template in `script/codegen.js`.
- `0002-dollar-tag-code-points` made the scanner compare full `int32_t`
  code points so different tags with the same low byte cannot match.
  The scanner still caps tags at 63 code points.

The patch-specific generator and scanner expectations retired with the patches.
The generator gate still exercises [the Korean identifier corpus](corpus/korean-identifiers.txt).
The [fresh and incremental regression tests](../../../conformance/postgres_patches_test.go)
retain identifier, scanner and lexical boundary cases.

The [dated ADR-0023 amendment](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md#amendment-2026-11-17-decision-1-postgresql-korean-support)
records the original exception. [Current lexical limits](../../../../docs/reports/limitations.md#postgresql-korean-support-v060)
cover U+3000 and numeric or parameter adjacency.
