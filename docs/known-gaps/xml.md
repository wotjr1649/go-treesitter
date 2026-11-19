<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# XML known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `5000ae8f22d11fbe93939b05c1e37cf21117162d`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/xml/provenance.json).

- Source identity SHA-256: `0367e2b839cccbb01756ff178acf69b44f6d7cda95ce8f91f8799726d65095bb`
- Module provenance SHA-256: `f5e7357dbd13bc4a377bfff038a17794cb3b7ca69fc7d16eea8a97d8df0f7833`

## xml-non-ascii-attribute-name

Input (JSON string; escapes and final newline are significant):

```json
"<a 속성=\"1\"/>\n"
```

Observed: The root has errors: an ERROR over the attribute [3,13) replaces the Attribute node. The Name regex is [a-zA-Z_][a-zA-Z0-9_:.·-]*; element names with Hangul parse because the external scanner reads them, attribute names do not.

Expected rule: An Attribute whose Name is 속성: NameStartChar includes U+3001-U+D7FF and NameChar continues with the same ranges.

- Specification: [XML 1.0 (Fifth Edition): Names and Tokens](https://www.w3.org/TR/xml/#NT-NameStartChar)
- [Upstream issue](https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40)
- Input SHA-256: `139be011e1c15c050238fa61f3dec2db1ddcb0203211c724545850540e6d92e3`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/xml-non-ascii-attribute-name.dump), SHA-256: `4eb87bb3ff40d1b9dd8e8e84a4bfd34fc20ab6fd5e17fb1e87e3747e80baf849`

## xml-non-ascii-namespace-prefix

Input (JSON string; escapes and final newline are significant):

```json
"<a xmlns:접두=\"u\"/>\n"
```

Observed: The root has errors: an ERROR over the namespace declaration [9,15) replaces the prefix after xmlns:. The prefix is matched by the ASCII-only Name regex.

Expected rule: A namespace declaration attribute xmlns:접두: a prefix is an NCName, whose first character may be any NameStartChar except the colon, including Hangul.

- Specification: [Namespaces in XML 1.0: Attribute Names for Namespace Declaration](https://www.w3.org/TR/xml-names/#NT-NSAttName)
- Specification: [Namespaces in XML 1.0: NCName](https://www.w3.org/TR/xml-names/#NT-NCName)
- [Upstream issue](https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40)
- Input SHA-256: `fc57295ceda51575cf2bd7a888411a60d7ed9cdb4ee414cb5021760f5889596e`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/xml-non-ascii-namespace-prefix.dump), SHA-256: `fa9d64054a422ab64209949368e3b2a2efb1a08412b8e52bed029f9c1762530e`

## xml-hangul-tag-low-byte-mismatch

Input (JSON string; escapes and final newline are significant):

```json
"<x걁>body</x굁>\n"
```

Observed: The root has no error: </x굁> (U+AD41) closes <x걁> (U+AC41) and the document parses as one element. The scanner stores each tag-name code point as one signed byte, so names whose code points share the low byte 0x41 compare equal; </xA> closes <x걁> as well. The ASCII twin <xa>body</xb> is an ERROR, as is <x한>body</x글>.

Expected rule: An end-tag name must equal the name in the start tag. </x굁> does not match <x걁>, so the input is not well-formed and must produce an error, as the ASCII twin does.

- Specification: [XML 1.0 (Fifth Edition): Element Type Match](https://www.w3.org/TR/xml/#GIMatch)
- [Upstream issue](https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40)
- Input SHA-256: `b6c286fbc013a3b8c3158dd57033888565e62ac4856bcf97cc21ff2eb0f42c94`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/xml-hangul-tag-low-byte-mismatch.dump), SHA-256: `45328da4fb016cacd52269648048f6e555e633b8686405f1768013d6de5a7c72`
