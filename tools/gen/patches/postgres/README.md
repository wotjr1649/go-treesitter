# PostgreSQL temporary Korean support patches

Owner decision, 2026-11-17: explicit exception to ADR-0023 decision 1 for Korean
support, although this upstream gap does not block a release gate. No runtime or
source-version pin changes. Retire these patches with an approved upstream fix.

- `0001` patches `postgres/grammar.js` before CLI generation: the byte-oriented
  high-bit identifier range is represented as all non-ASCII Unicode code points.
  ASCII start/continuation rules are unchanged.
- `0002` patches `postgres/src/scanner.c` after generation: retain full `int32_t`
  lookaheads so matching tags close and different code points cannot alias by low byte.
  The existing 63-code-point tag cap and partial-match scanner algorithm remain.

Both patch records link to [upstream issue #101](https://github.com/gmr/tree-sitter-postgres/issues/101),
posted by the owner for the identifier range and dollar-tag code-point defects.
The [dated ADR-0023 amendment](../../../../docs/design/decisions/ADR-0023-temporary-upstream-grammar-patches.md#amendment-2026-11-17-decision-1-postgresql-korean-support)
records the owner-approved exception.
Upstream must also update the identifier template in `script/codegen.js` so a
subsequent upstream generation does not restore the old regex.

PostgreSQL permits all high-bit bytes in UTF-8 identifiers, including combining
characters and U+3000; this is not a Unicode letter/category restriction.
The grammar's `\s+` extras still skip only ASCII whitespace; U+3000 was a lexing
error before and is an identifier character now. Other upstream SQL grammar/scanner limits remain.
