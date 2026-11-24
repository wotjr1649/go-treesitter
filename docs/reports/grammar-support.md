# Per-grammar support limits

[ADR-0028](../design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md#decision)
documents these limits at the grammar-module level. They do not gate the root RC or 1.0. Grammar
releases still use the [grammar and pin-move gates](../specs/validation.md#gates). Check the grammar
version selected by Go alongside the root in the [compatibility table](compatibility.md).

- **Published** uses Go module tags `grammars/<dir>/v0.6.0`. All 25 tags point to
  [`a8af436`](https://github.com/wotjr1649/go-treesitter/commit/a8af4360e4964deb6c0bb62461ee8348be1aa738),
  with root `v0.6.0`. The published [pins](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/tools/gen/pins.json)
  and module provenance record those identities.
- **Development** describes main at
  [`de2df14`](https://github.com/wotjr1649/go-treesitter/commit/de2df14b085a3f728acdc37e87057f1bf6f1fd52).
  “Same” carries the published limit forward. “Fixed” applies to the recorded input in the unreleased tree.

The inputs below are illustrative excerpts. Linked records give the exact fixtures and scope. The error
column refers to the root's syntax error flag (`HasError()`). An error-free tree can still have the
wrong structure. The four error-free **registered** cases on main are the C# default constraint,
the HTML end tag and the two Python precedence cases. The support table summarizes recorded evidence
at the linked identities. See the [evidence levels](../specs/validation.md#evidence-levels).

## Registered on main

The [register](../../tools/conformance/testdata/known-gaps/register.json) and the linked grammar pages
record the expected rule, observed dump and source identity. When a new pin or approved patch
changes a case, recheck its exact fixture, neighboring valid inputs and affected captures.
Keep the old tag's limit linked to its published record and retain a regression with the evidence
required by the grammar release gate.

| Grammar module | Input | Published | Development | Root error | Consumer impact | Re-review when | Register ID |
|---|---|---|---|---|---|---|---|
| `csharp` | Override with `where T : default` | `v0.6.0` | Same | No | Type-constraint queries see `default` as a type identifier. | A candidate includes the upstream constraint fix and distinguishes `default` from a type name. | [csharp-default-constraint](../known-gaps/csharp.md#csharp-default-constraint) |
| `csharp` | `#define A` without a final newline | `v0.6.0` | Same | Yes, anonymous missing token | A valid directive can trigger diagnostics or be skipped by error-sensitive tools. | A candidate includes the directive EOF fix. Check the final directive and edit recovery. | [csharp-directive-eof](../known-gaps/csharp.md#csharp-directive-eof) |
| `csharp` | `var s = $"";` | `v0.6.0` | Same | Yes | The interpolation prefix errors and the quotes are parsed as an ordinary string. | A candidate recognizes empty interpolation as an interpolated string. | [csharp-empty-interpolation](../known-gaps/csharp.md#csharp-empty-interpolation) |
| `csharp` | `#if` and `#else` supply different `if` heads for one shared block | `v0.6.0` | Same | Yes | The shared block is detached from its conditional statement. | A candidate changes directive handling. Check the body association, captures and incremental edits. | [csharp-conditional-splits-statement](../known-gaps/csharp.md#csharp-conditional-splits-statement) |
| `css` | `.한글 { a: b }` | `v0.6.0` | Same | Yes | The class selector can put the whole rule into recovery. | A candidate extends selector identifier handling. Check the class and the adjacent declaration. | [css-non-latin1-identifier](../known-gaps/css.md#css-non-latin1-identifier) |
| `css` | `a { b: 한글 }` | `v0.6.0` | Same | Yes | Declaration-value queries lose the bare identifier. | A candidate extends value identifier handling. Check the `plain_value` node independently of selectors. | [css-non-latin1-value](../known-gaps/css.md#css-non-latin1-value) |
| `html` | `<x-걁>body</x-굁>` or an end tag of `</x-A>` | `v0.6.0` | Same | No | The wrong end tag closes the element, misleading structure-sensitive edits and queries. | A scanner candidate compares full names. Check matching and mismatched tags in fresh and incremental trees. | [html-hangul-tag-low-byte-mismatch](../known-gaps/html.md#html-hangul-tag-low-byte-mismatch) |
| `powershell` | `$값 = 1` | `v0.6.0` | Same | Yes | The variable node is lost, affecting navigation and queries. | A candidate changes Unicode variable tokens. Check unbraced names. | [powershell-non-ascii-variable](../known-gaps/powershell.md#powershell-non-ascii-variable) |
| `powershell` | `Get-Item -이름 1` | `v0.6.0` | Same | Yes | Parameter queries lose the name. | A candidate changes parameter tokens. Check the starting character and the following argument. | [powershell-non-ascii-parameter](../known-gaps/powershell.md#powershell-non-ascii-parameter) |
| `powershell` | `@{ 키 = 1 }` | `v0.6.0` | Same | Yes | The hashtable entry goes into recovery. | A candidate changes bare-word keys. Check the key and value association. | [powershell-non-ascii-hash-key](../known-gaps/powershell.md#powershell-non-ascii-hash-key) |
| `powershell` | `$a.속성` | `v0.6.0` | Same | Yes | The member-access node is lost. | A candidate changes bare member names. Check the access tree and captures. | [powershell-non-ascii-member-name](../known-gaps/powershell.md#powershell-non-ascii-member-name) |
| `powershell` | `[한글]$a` | `v0.6.0` | Same | Yes | A cast or type literal goes into recovery. | A candidate changes type names. Check the syntax independently of type resolution. | [powershell-non-ascii-type-name](../known-gaps/powershell.md#powershell-non-ascii-type-name) |
| `python` | `a ^ b & c` | `v0.6.0` | Same | No | The tree groups `(a ^ b) & c`, changing what an analysis infers about the expression. | A candidate groups it as `a ^ (b & c)` and preserves the parenthesized controls. | [python-bitwise-precedence](../known-gaps/python.md#python-bitwise-precedence) |
| `python` | `await f() ** 2` inside an async function | `v0.6.0` | Same | No | The power expression is inside the await operand, changing expression analysis. | A candidate groups it as `(await f()) ** 2` and preserves explicitly parenthesized operands. | [python-await-power](../known-gaps/python.md#python-await-power) |
| `ruby` | `a = :한글` | `v0.6.0` | Same | Yes | The colon errors and the identifier is separate. The recorded `%s(한글)` form parses as a symbol. | A candidate fixes non-ASCII simple symbols. Check ASCII symbols and hash labels as controls. | [ruby-non-ascii-symbol](../known-gaps/ruby.md#ruby-non-ascii-symbol) |
| `swift` | `끝: for _ in 0..<1 { }` | [`v0.6.0` record](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/docs/known-gaps/swift.md#swift-non-ascii-statement-label) | Same on the newer pin | Yes | A valid label definition becomes an error and the loop has no label. | A candidate changes the label production. Check definitions and references. | [swift-non-ascii-statement-label](../known-gaps/swift.md#swift-non-ascii-statement-label) |

## XML limits on the published tag

These IDs remain in the published register. The [development patches](../../MIGRATING.md#xml-names-and-tag-names)
cover the recorded inputs under upstream issue [#40](https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40).
The first grammar tag containing the patches should record the change and its release evidence.

| Grammar module | Input | Published | Development | Root error on published tag | Consumer impact on published tag | Re-review when | Published register ID |
|---|---|---|---|---|---|---|---|
| `xml` | `<a 속성="1"/>` | `v0.6.0` | Fixed, no root error | Yes | An error replaces the attribute node. | A tag includes the name fix. Keep the attribute fixture and recheck it on the next pin. | [xml-non-ascii-attribute-name](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/docs/known-gaps/xml.md#xml-non-ascii-attribute-name) |
| `xml` | `<a xmlns:접두="u"/>` | `v0.6.0` | Fixed, no root error | Yes | The namespace prefix enters recovery. | A tag includes the name fix. Check namespace prefixes and name-start boundaries. | [xml-non-ascii-namespace-prefix](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/docs/known-gaps/xml.md#xml-non-ascii-namespace-prefix) |
| `xml` | `<x걁>body</x굁>` or an end tag of `</xA>` | `v0.6.0` | Fixed, root error | No | A mismatched tag gives a clean element with the wrong boundaries. | A tag includes the scanner fix. Keep both mismatch fixtures and recheck edits restoring scanner state. | [xml-hangul-tag-low-byte-mismatch](https://github.com/wotjr1649/go-treesitter/blob/a8af4360e4964deb6c0bb62461ee8348be1aa738/docs/known-gaps/xml.md#xml-hangul-tag-low-byte-mismatch) |

## Other documented grammar limits

These cases have no active register ID. Numbered references point to the [preview limitations](limitations.md).
“Unrecorded” means the cited record gives no root flag for that example.

| Grammar module | Input or scope | Published | Development | Root error | Consumer impact | Re-review when | Source |
|---|---|---|---|---|---|---|---|
| `bash` | A leading space or tab before the terminator of plain `<<EOF`, or U+3000 before the terminator of `<<-` | `v0.6.0` | Same | No on the retained cases | Heredoc content ends early and later text is parsed outside the body. | A scanner candidate changes leading whitespace. Check plain and tab-stripping forms separately. | [Row 16](limitations.md), [heredoc tests](../../tools/conformance/scanner_patches_test.go) |
| `bash`, `ruby` | Open heredoc end words and metadata exceed the 1,024-byte scanner state buffer | `v0.6.0` | Same | Yes in the Bash fixture. No in the Ruby fixture. | State is lost and the real closing line can disappear from the tree. UTF-8 end words reach the limit sooner. | State storage or serialization changes. Recheck the last fitting and first overflowing inputs. | [Row 16](limitations.md), [heredoc tests](../../tools/conformance/scanner_patches_test.go) |
| `golang` | Bare call to a shadowing `make` | `v0.6.0` | Same | Unrecorded | The first argument is parsed using the type rule even when the name denotes a user function. | A candidate changes `make` call parsing. Check shadowed calls and ordinary builtin calls. | [Row 26](limitations.md) |
| `html` | `R&D` or `&#x26x` in element text | `v0.6.0` | Same | No for the retained `<p>R&D</p>` fixture. Unrecorded for `&#x26x`. | The grammar emits `entity` for the prefixes `&D` and `&#x26`. Entity captures can include text a consumer did not expect. | A candidate changes entity matching. Check named references, numeric reference boundaries and bare ampersands. | [Row 24](limitations.md), [grammar fixtures](../../tools/conformance/testdata/grammar-text-patches.json) |
| `php` | Standalone `php_only` dialect | `v0.6.0` ships the embedded-PHP grammar | Same | Outside scope | There is no separate Go module for the upstream PHP-only dialect. | A PHP-only module is proposed. | [PHP scope](limitations.md#php-v060) |
| `postgres` | Dollar-quote tag of 64 code points, such as 64 copies of `끝` | `v0.6.0` | Same | Yes | PostgreSQL accepts longer tags. The grammar stops recognizing them after 63 code points. | The scanner's tag limit changes. Check both boundary lengths. | [PostgreSQL Korean support](limitations.md#postgresql-korean-support-v060), [retained tests](../../tools/conformance/postgres_patches_test.go) |
| `postgres` | Adjacent tokens in `SELECT 9값;` or `SELECT $9끝$body$9끝$;` | `v0.6.0` | Same | No | The grammar treats trailing junk as an alias where PostgreSQL rejects the statement. | A candidate changes numeric or positional-parameter boundaries. Check adjacent and space-separated controls. | [PostgreSQL Korean support](limitations.md#postgresql-korean-support-v060), [retained tests](../../tools/conformance/postgres_patches_test.go) |
| `postgres` | PL/pgSQL, other SQL dialects or PostgreSQL server-version compatibility | `v0.6.0` covers PostgreSQL SQL | Same | Outside scope | The shipped SQL grammar has no PL/pgSQL support or server-version compatibility claim. | A new dialect or server-version support scope is proposed. | [PostgreSQL scope](limitations.md#postgresql-v060) |
| `powershell` | `$값` inside an expandable here-string | `v0.6.0` | Same | No | The interpolation has no separate variable node for queries. | A candidate changes here-string interpolation. Compare the variable node with PowerShell's parser. | [Korean input evidence](../guides/korean-input.md#which-grammars-take-korean), [fixture](../../tools/conformance/testdata/korean/powershell/str-here-string.in) |
| `python`, `yaml` | Recorded nesting beyond 511 Python indentation levels with an odd number of open string delimiters, or 253 YAML indentation levels | `v0.6.0` | Same | Yes | The scanner drops indentation state. A full-length error tree replaces the expected nested structure. | Serialization bounds or the upstream fix changes. Rerun the stress inputs and boundary controls. | [Row 17](limitations.md) |
| `ruby` | `끝끝` in `<<끝`, or a real closing word at EOF without a final newline | `v0.6.0` | Same | No on the retained cases | An apparent closing line ends the body early, or the real closing word becomes body text. | A scanner candidate changes closing lines. Check repetition and EOF independently, with ASCII and Hangul. | [Row 16](limitations.md), [heredoc tests](../../tools/conformance/scanner_patches_test.go), [EOF fixture](../../tools/conformance/testdata/korean/ruby/str-heredoc-hangul-delimiter-eof-no-newline.in) |
| `ruby` | DEL (U+007F) in a regular global name, or `"#Ĥ!"` at a short interpolation boundary | `v0.6.0` | Same | Unrecorded | The grammar admits an invalid name character or interprets the low byte of a Unicode character as ASCII punctuation. | A candidate changes global-name or short-interpolation scanning. Establish the error flag and exact ranges for each input. | [Row 27](limitations.md) |
| `rust` | Nested block-comment opening at `//*`, as in `/* 是个 //* */ */` | `v0.6.0` | Same | Yes on the recorded input | The comment ends too early and the following close is parsed as code with an error. | A scanner candidate changes overlapping delimiters. Check the outer comment span and following code. | [Row 22](limitations.md) |
| `tsx`, `typescript` | `const x = v as import('x').Y` or the `satisfies` variant, followed by `[a + b];` on the next line | `v0.6.0` | Same | Yes | The array statement goes into recovery in both grammars. | An owner release changes this boundary. Check both grammars and ordinary type index access before a shared pin move. | [Row 29](limitations.md), [owner known-gap record](https://github.com/wotjr1649/tree-sitter-typescript/pull/31) |
| `xml` | Restoring scanner state with a name over 255 UTF-8 bytes or enough open names to exceed the 1,024-byte buffer | — | Limit of the UTF-8 scanner patch after `v0.6.0` | Unrecorded | A long name or many open names can change incremental element boundaries when an end tag fails to match. | Scanner name storage or the state limit changes. Check long names and deep edits against fresh parses. | [Row 30](limitations.md) |
| `xml` | Standalone DTD dialect | `v0.6.0` ships the XML grammar | Same | Outside scope | There is no separate Go module for the DTD dialect. | A DTD module is proposed. | [Row 1](limitations.md) |
| `yaml` | Scanner row beyond `2^31−1` | `v0.6.0` | Same | Outside the root input bound | Scanner row overflow is unreachable through the root API's `2^31−1`-byte input limit. | The root input cap or scanner row representation changes. | [Row 25](limitations.md), [root input limit](../../options.go) |

The other published modules are `c`, `cpp`, `dart`, `java`, `javascript`, `json`, `kotlin`, `r`
and `tsql`. Their `v0.6.0` tags have no case in the tables above. The
[Korean input guide](../guides/korean-input.md) records the specific inputs and positions covered.
