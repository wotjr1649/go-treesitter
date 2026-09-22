# Draft: split JSX ampersand behavior from the equals scanner guard

This is a local, unsent report for
[gotreesitter #1242](https://github.com/odvcencio/gotreesitter/issues/1242).
Baseline identities are in `internal/provenance/identities.json`.

## Minimal reproducer

Parse these exact strings (no final newline) as both `x.tsx` and `x.jsx`:

```tsx
const d = <p>a = b</p>;
const e = <code>k=v</code>;
```

Treat each line as a separate source. The retained reproducer is:

```powershell
$env:CGO_ENABLED = '0'
go test ./internal/gtsadapter -run TestKR0001b -v -count=1
```

Actual: `accepted_with_errors`, stop `accepted`, not stopped early, full root
span, `HasError=true`. F4/F5 have respectively 17/17 TSX nodes and 15/15
JavaScript nodes with the same preorder walker. Source hashes and receipts are
in `wu7-ratchets.log` (Phase 2 product lane, E3).

Expected from source inspection (E1): the C scanner should consume `a = b` and
`k=v` as JSX text. End-to-end C confirmation is a separate WU9 result, if run.

## Two causes, two expectations

The original report uses `tsc` 5.9.3 as its comparison. That is a different
syntax implementation; it does not establish disagreement with C tree-sitter.

In the pinned TypeScript
[scan_jsx_text](https://github.com/tree-sitter/tree-sitter-typescript/blob/75b3874edb2dc714fb1fd77a32013d0f8699989f/common/scanner.h#L257)
loop, this exact condition is present:

```c
lexer->lookahead != '}' && lexer->lookahead != '&'
```

The loop also stops at EOF, angle brackets and braces. The grammar's
`html_character_reference` pattern is:

```text
&(#([xX][0-9a-fA-F]{1,6}|[0-9]{1,5})|[A-Za-z]{1,30});
```

A bare ampersand lacks the required terminator. F1–F3 must retain their error
behavior; F6 (`&amp;`) and F7 (plain text) must remain clean. KR-0001a is a
grammar usability limitation, not a runtime correction target.

The Go-only leading-identifier guard in `tsxScanJsxText` and `jsScanJsxText`
returns false when the next character is `=`. The pinned C function has no
corresponding equals guard. This is the KR-0001b source-level explanation (E1).

## Proposed change, not applied

`proposed-equals-guard.diff` removes only the three-line equals rejection in
each scanner. The bare-ampersand stop and adjacent slash guard are untouched.
This is a review proposal, not a tested correction. Attribute parsing, malformed
elements, multiline text, scanner recovery and the full grammar corpus need
differential checks before anyone applies it. No runtime or reference source
was modified, and no fork or replace directive was created.

## Upstream status observed for this report

On 2026-09-22 the public issue page is open, unassigned, without labels or a
linked pull request. This is an observation of that page, not a guarantee that
no maintainer is working privately. A fresh tagged-fix search is needed before
any fork decision. Nothing was submitted upstream.
