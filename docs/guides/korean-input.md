# Korean and legacy-encoded input

How to get Korean text, and text in CP949, through the parser and back to the right bytes and columns. The parser
itself only reads UTF-8; everything here is about the layer in front of it, plus what Korean text does inside the
grammars and queries. Rules owned elsewhere are linked, not repeated: [invalid UTF-8 and
`HasInvalidUTF8`](../specs/parser-result.md#input), [the `Source` and `LineIndex`
signatures](../design/api-v2.md#7-primitives-that-set-this-library-apart-v001), [known limits](../reports/limitations.md).

This guide describes v0.6.0. The published v0.5.0 has no CP949 decoder, no
`Source.UTF8Offset`, no `LineIndex.Offset`, the earlier query regex meaning and the earlier heredoc end-word comparison;
[MIGRATING](../../MIGRATING.md#upgrading-from-v050-to-v060-engine-abi-4) lists the changes.

## Choose the entry point

| Your bytes | Call | Notes |
|---|---|---|
| UTF-8, with or without a BOM | `Parse` / `ParseSnapshot`, or `NewSource` | Used unchanged. A BOM stays in the text. |
| UTF-16 with a BOM | `NewSource` | Detects `FF FE` / `FE FF`, converts, drops the BOM. |
| UTF-16 without a BOM | `NewSourceUTF16(raw, bigEndian)` | You say the byte order. |
| CP949 (Korean Windows, `.txt`/`.c`/`.java` from older tools) | `NewSourceCP949(raw)` | Built in, standard library only. |
| Anything else (Shift_JIS, GBK, ...) | convert to UTF-8 yourself | Not built in. |

**There is no auto-detection, by design.** `NewSource` keeps its BOM-only contract. A CP949 file has no marker, and
heuristics fail silently: a short file can be valid in two encodings, and a wrong guess yields wrong text with no
error. Detect at the edge, where you know more than the library can (file extension, project setting, a charset
declaration), and call the matching constructor. `NewSourceCP949` refuses input that starts with `EF BB BF`, `FF FE` or
`FE FF` with `ErrInvalidInput`: that is a byte order mark, so the input is UTF-8 or UTF-16.

Passing CP949 bytes to `Parse` without decoding does not fail: the parser reads them as invalid UTF-8, a clean tree is
returned (`HasError()` can be false, for example when the Korean text is inside a comment or string) and
`HasInvalidUTF8()` is true. Check `HasInvalidUTF8` if you cannot tell what you were given.

## CP949 means windows-949 here

`NewSourceCP949` implements the WHATWG Encoding Standard's `EUC-KR` decoder and index: **windows-949 (also called UHC)**.
It decodes plain EUC-KR (a subset) and the Unified Hangul Code extension, so all 11,172 Hangul syllables, 94
compatibility jamo and 4,888 CJK ideographs are available (17,048 double-byte mappings in all). It is **not IBM-949**,
which Java calls `Cp949` and ICU `cp949`. The table is generated from the pinned index (identity in the header of
[`cp949_table.go`](../../cp949_table.go)); its licence is in [`THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md).

Invalid input becomes U+FFFD exactly as the standard says:

- a lone `0x80` or `0xFF`, a lead byte at the end of the input, and an unmapped pair whose second byte is not ASCII
  (including the user-defined areas with lead `0xC9` or `0xFE`) are one replacement for the unit;
- a lead byte followed by an ASCII byte that completes no mapped pair is one replacement, and the ASCII byte is then read
  on its own. A stray lead byte therefore never swallows a newline, quote or backslash.

**Replacements are invisible to strict mode.** U+FFFD is valid UTF-8, so `WithStrictUTF8` and `HasInvalidUTF8` never see
a decoded replacement. `Source.Replacements()` counts them (also unpaired UTF-16 surrogates; it is 0 for UTF-8, whose
invalid bytes `HasInvalidUTF8` does report). Do not treat strict mode as a safeguard for decoded input: check
`Replacements() == 0` where it matters, and always before writing anything back.

## Offsets between the original and the UTF-8 text

`Source.Bytes()` is what you parse; every node offset is into it. `Source.Original()` is what you were given.

- `OriginalOffset(off)`: an offset in `Bytes()` to the offset in `Original()`.
- `UTF8Offset(orig)`: an offset in `Original()` to the offset in `Bytes()`.

For converted sources (UTF-16, CP949) both floor an offset that is inside a character to that character's start, clamp
past the end (to the end of the other text), and map UTF-16 BOM bytes to 0 / past the BOM. They round-trip only at
character boundaries. For UTF-8 sources the two texts are the same bytes and the offset is returned unchanged (clamped),
so neither function checks or moves it to a character boundary. A CP949 character is 1 or 2 original bytes and 1 to 3
UTF-8 bytes; a replacement can be 1 or 2 original bytes, so never compute widths from the decoded text.

`UTF8Offset` is for positions in the original (an editor selection, a byte offset from another tool). Do not build an
`InputEdit` from it; see below.

## Columns, lines and the reverse conversion

`Snapshot.Lines()` (or `NewLineIndex`) gives a `LineIndex`. `Pos(off)` returns the line and three columns:

| Field | Counts | LSP `positionEncoding` (3.17) |
|---|---|---|
| `ByteCol` | bytes (tree-sitter's `Point.Column`) | `utf-8` |
| `UTF16Col` | UTF-16 code units | `utf-16`, the default every server must support |
| `RuneCol` | Unicode code points | `utf-32` |

`Offset(line, col, kind)` converts back, with `kind` one of `ByteColumn`, `UTF16Column`, `RuneColumn`. It returns
`(0, false)` for a line outside `[0, NumLines())` (a line is never clamped, so a wrong line is not silently moved) and
clamps the column the way LSP does: past the end of the line's content gives the end of the content, which is before the
line's LF and before the CR of a CRLF; a column inside a character (between the two units of an emoji, or inside the
bytes of a Hangul syllable) gives that character's start. `Pos` is monotonic the same way: an offset inside a character
reports the `UTF16Col` and `RuneCol` of the character's start, while `ByteCol` stays the plain distance from the line start.

Where this differs from an LSP client (see [Position](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#position)
in the 3.17 specification):

- **Line terminators.** LSP lines end at `\n`, `\r\n` or `\r`. Here lines end at LF only, like tree-sitter rows, so a
  lone `\r` is a character of the line, not a break.
- **Invalid bytes.** Each byte that is not valid UTF-8 counts as one rune and one UTF-16 unit. A client that decoded the
  file with replacement characters may count an invalid sequence differently.
- **BOM.** A UTF-8 BOM is text here: on line 0 the columns include it (`Pos(3)` of `BOM + "a"` is `UTF16Col` 1, `ByteCol` 3),
  where an LSP client usually counts from after it. Subtract the BOM's columns on line 0 when your client does. A UTF-16
  BOM is dropped by `NewSource`, so there is nothing to subtract.

## Normalization and display width

The library does not normalize. A syllable can arrive precomposed (NFC, `한`, one rune) or as conjoining jamo (NFD,
`ᄒ` `ᅡ` `ᆫ`, three runes), and the two are different byte strings with different `RuneCol` counts. macOS file names and
some clipboards produce NFD. **CP949 cannot encode NFD jamo** (nor emoji, nor typographic marks such as the em dash),
so normalize to NFC before encoding. Two names that differ only in normalization are different names to the parser and
to queries.

A rune is not a screen column. Hangul syllables and CJK ideographs are wide (two cells) and conjoining jamo, combining
marks and emoji sequences have their own widths: compute display width with a width library, not from `RuneCol`.

## Korean in queries

In the development tree, `#match?` and `#not-match?` evaluate regular expressions the way the tree-sitter CLI does: `\w`,
`\d`, `\s`, `\b`/`\B` and the case folding of `(?i)` are Unicode. `\w` matches Hangul syllables and jamo, so
the pattern `^\w+$` accepts `한글`, and `\b` finds word edges next to Hangul. Write plain `\w` for Hangul; a class
such as `\p{L}` is not needed. A node's text is a substring of the UTF-8 text that was parsed, so the predicates see
decoded Hangul even when the file was CP949.

To get the old ASCII meaning of a shorthand class or a boundary, scope it: `(?-u:\w+\b)` matches ASCII word characters
only. What the regex subset accepts and where it differs from the CLI (`\p{...}` follows your Go toolchain, a few
Rust-only forms are query errors) is owned by [limitations, row 5](../reports/limitations.md), the
[query contract](../design/query-abi.md#regex-predicate-subset-v060) and the
[migration notes](../../MIGRATING.md#upgrading-from-v050-to-v060-engine-abi-4); a pattern that Go accepted before can now be a query error.

## Which grammars take Korean

Some pinned grammars match names with ASCII-only or Latin-1 patterns, so a Hangul name can
produce `ERROR` nodes even though the language allows it. The table says what was shown, per grammar and kind of text.

- **yes**: every retained input of that kind parses with no `ERROR` or `MISSING` node. For each Hangul run, and each
  other non-ASCII run such as an emoji, the deepest covering named node has the kind the manifest records; when that
  node is wider than the run, its extent is recorded too. This describes the recorded tree of each input, not whether
  the tree matches the language's own parser. One recorded tree is known to differ from it: in
  `ruby/str-heredoc-hangul-delimiter-eof-no-newline` the closing `끝` at end of file without a newline is part of
  `heredoc_content` (Ruby ends the heredoc there). The same EOF boundary occurs with an ASCII end word.
- **no** / **partly**: a defect in the pinned grammar, reproduced by the engine and by native C. The linked page has the exact
  input and the observed wrong tree. A tree is still returned; check `HasError()`.
- **not recorded**: there is no retained input for it, so nothing is claimed either way.

| Grammar | Identifiers and names | Strings and text | Comments |
|---|---|---|---|
| `bash` | yes (words, function names) | yes (quoted forms, heredocs) | yes |
| `c` | yes | yes | yes |
| `cpp` | yes | yes | yes |
| `csharp` | yes | yes | yes |
| `css` | no: fails on class, id and type selectors, custom properties, attribute selectors, identifier values ([CSS](../known-gaps/css.md)) | yes (a `content` string) | yes |
| `dart` | not recorded | yes | yes |
| `go` | yes | yes | yes |
| `html` | yes (tag and attribute names; see note 1) | yes | yes |
| `java` | yes | yes | yes |
| `javascript` | yes | yes | yes |
| `json` | yes (object keys) | yes | JSON has no comments |
| `kotlin` | yes | yes | yes |
| `php` | yes | yes | yes |
| `postgres` | yes (unquoted and quoted; see note 2) | yes (string literals and `$태그$` dollar-quote tags and bodies; see note 2) | yes |
| `powershell` | partly: variable, parameter, hash key, member name and type name parse (development tree after v0.6.0; v0.6.0 fails on them); fails on statement labels: `:한글` is read as a command name, not a label (no `ERROR`; see [limitations](../reports/limitations.md), row 31) | yes | yes |
| `python` | yes | yes | yes |
| `r` | yes | yes | yes |
| `ruby` | partly: local, instance and global variable names parse (global names in v0.6.0); fails on symbols ([Ruby](../known-gaps/ruby.md)) | yes | yes |
| `rust` | yes | yes | yes |
| `swift` | partly: variable, function, parameter, type and field names parse; fails on statement labels ([Swift](../known-gaps/swift.md)) | yes | yes |
| `tsql` | yes on the retained bracketed, double-quoted and regular identifiers, variables and label cases (development tree, owner release v2.8.0, as since v2.1.0; v0.5.0 rejects regular identifiers) | yes | yes |
| `tsx` | yes (includes JSX element names) | yes (includes JSX text) | yes |
| `typescript` | yes | yes | yes |
| `xml` | yes: element, attribute and namespace-prefix names (attribute names and prefixes in the development tree after v0.6.0; v0.6.0 fails on them)<sup>1</sup> | yes | yes |
| `yaml` | yes | yes | yes |

**Notes.**

1. The HTML scanner stores each tag-name character in one byte, so names whose characters share a low byte close each other:
   `<x-걁>body</x-굁>` parses without an error. This pinned upstream defect is not exercised by the recorded rows: see
   [HTML](../known-gaps/html.md#html-hangul-tag-low-byte-mismatch), which shows that an ASCII mismatch is still an `ERROR`.
   The XML scanner had the same defect; the development tree (after v0.6.0) compares complete tag names, so
   `<x걁>body</x굁>` is an `ERROR` there ([limitations](../reports/limitations.md), row 30).
2. The development tree uses upstream v19.0.0, which incorporates both Korean-support fixes. The published v0.5.0
   `postgres` fails on Hangul identifiers and dollar-quote tags. The development scanner caps tags at 63 code points.
   U+3000 and other non-ASCII spaces are identifier characters. See
   [limitations, PostgreSQL Korean support](../reports/limitations.md#postgresql-korean-support-v060).

**Evidence.** `TestCorpusKorean` in [`tools/conformance`](../../tools/conformance/korean_test.go) runs the retained
inputs listed in [`manifest.csv`](../../tools/conformance/testdata/korean/manifest.csv) through the product. In every
product lane it is a retained test (`E3`): "parses these recorded inputs cleanly". In the CI conformance shards (Linux
amd64 and arm64, with a freshly built native reference) it also compares each dump byte for byte with native C (`E5`),
for those inputs, on those platforms and the pinned grammar identity. A "no" row is an expected failure: the test passes
only while the recorded wrong tree is unchanged. For clean inputs, the test checks every Hangul and other non-ASCII run,
including emoji in the edge rows. The result applies to the recorded inputs and pinned grammar versions; other positions
of the same upstream patterns are not recorded ([limitations, rows 18, 20 and 21](../reports/limitations.md)). Do not read the table as "this grammar fully supports
Korean": "yes" means "these inputs".

## Heredocs with Korean end words (Bash and Ruby)

Upstream stores each end-word character in one byte, so a Korean end word never closed. The development tree stores end
words as UTF-8 and compares whole characters: `<<끝` closes on a line holding `끝`, and a different syllable, or an ASCII
byte that equals its low byte, no longer closes it. The published v0.5.0 keeps the earlier low-byte comparison, where
two syllables with the same low byte close each other.

- **Bash** closes only on a line that holds the end word and then a line ending (LF or CRLF) or the end of the input
  (a CR directly before the end of input also closes it): `끝났다` does not close `<<끝`, and trailing text stays in the body.
  In CRLF input the `heredoc_end` span includes the CR.
- **Remaining limits.** Bash still ignores spaces and tabs before the end word for every heredoc (` 끝` closes a plain
  `<<끝`, where Bash keeps that line in the body), and `<<-` strips every `iswspace` character, including U+3000, not only
  tabs. Ruby still closes on a repeated end word (`끝끝` closes `<<끝`).
- **Scanner state.** All open end words and the scanner's own state share one 1024-byte buffer. About 337 Hangul syllables
  in total fit (a single end word of 337 is the last that fits in Bash, 338 in Ruby), fewer with other state. Past that the
  state is lost without a signal from the parser: Bash then returns an error tree, and Ruby can return a tree with no
  error, so a very long end word is not safe to rely on.

The patches, the tests and the exact bounds are owned by [limitations, row 16](../reports/limitations.md) and
[ADR-0023](../design/decisions/ADR-0023-temporary-upstream-grammar-patches.md).

## Writing edited text back to a CP949 file

Re-encoding the whole `Bytes()` is lossy (a replaced byte is lost) and rewrites parts of the file nobody edited.
Splice instead, and refuse when you cannot prove the result:

1. Refuse if `Replacements() > 0`.
2. Find the original span with `OriginalOffset` and check it with `UTF8Offset` (both ends must be character boundaries).
3. Normalize the new text to NFC and encode it with a **strict** encoder. Never use `encoding.ReplaceUnsupported`: it
   silently writes `0x1A` for a character the code page lacks.
4. Splice into the original bytes: `Original()[:start] + encoded + Original()[end:]`.
5. Decode the result again and require it to equal `Bytes()[:a] + newText + Bytes()[b:]` with no replacements; otherwise
   do not write.

Step 5 is what stops a lead byte before the splice point from pairing with inserted bytes. For example `x` `B0` decodes
to `x` U+FFFD; inserting `a` after it gives `x` `B0` `61`, which decodes to `x캻`. The replacement turned into a
different character outside the edit, and the inserted `a` is gone. Step 1 refuses such a file; step 5 catches it if you
override step 1.

```go
// spliceCP949 replaces s.Bytes()[a:b] with text and returns the new CP949 bytes of the whole file.
func spliceCP949(s *treesitter.Source, a, b uint32, text string) ([]byte, error) {
	if s.Replacements() > 0 {
		return nil, errors.New("the original has bytes that are not CP949; refusing to rewrite it")
	}
	oa, ob := s.OriginalOffset(a), s.OriginalOffset(b)
	if a > b || s.UTF8Offset(oa) != a || s.UTF8Offset(ob) != b {
		return nil, errors.New("the span does not start and end on character boundaries")
	}
	text = norm.NFC.String(text)                       // golang.org/x/text/unicode/norm
	enc, err := korean.EUCKR.NewEncoder().String(text) // golang.org/x/text/encoding/korean: strict, errors on an unencodable rune
	if err != nil {
		return nil, err
	}
	out := slices.Concat(s.Original()[:oa], []byte(enc), s.Original()[ob:])
	back, err := treesitter.NewSourceCP949(out)
	if err != nil {
		return nil, err
	}
	want := string(s.Bytes()[:a]) + text + string(s.Bytes()[b:])
	if back.Replacements() != 0 || string(back.Bytes()) != want {
		return nil, errors.New("the splice would change text outside the edit")
	}
	return out, nil
}
```

The encoder lives in your program, not in this library. `korean.EUCKR` from `golang.org/x/text` is windows-949 too; a
one-off comparison during development (not a test in this repository) found the same 17,048 pairs as this library's table.

## Incremental editing

CP949 decoding is not local: one changed byte can change how its neighbours pair, so an offset map for the old original
does not describe the new one. After any change to the original:

1. Decode the whole new original again with `NewSourceCP949` (O(n)).
2. Find the edit from the common prefix and suffix of the old and new `Bytes()`.
3. Build the `InputEdit` with `EditFor(old, new, start, oldEnd, newEnd)`, call `tree.Edit`, then `Parse(ctx, new, tree)`.

```go
func editBetween(old, new []byte) treesitter.InputEdit {
	p := 0
	for p < min(len(old), len(new)) && old[p] == new[p] {
		p++
	}
	q := 0
	for q < min(len(old), len(new))-p && old[len(old)-1-q] == new[len(new)-1-q] {
		q++
	}
	return treesitter.EditFor(old, new, uint32(p), uint32(len(old)-q), uint32(len(new)-q))
}
```

Do not use `UTF8Offset` to build the edit; it is for positions in the original, not for describing what changed in the
decoded text.

## CRLF

Lines end at LF, so a CRLF file has a `\r` at the end of each line's content as far as `Pos` and tree-sitter points are
concerned, but `Offset` stops before it: a column past the content of a CRLF line gives the offset of the `\r`. Decoding
never changes line breaks: `0x0D` and `0x0A` are ASCII and always read on their own.
