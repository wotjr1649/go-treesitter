<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# HTML known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `73a3947324f6efddf9e17c0ea58d454843590cc0`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/html/provenance.json).

- Source identity SHA-256: `7779ebc983293964f36db015ba0d3899633512e43f793df325e96af59d7998e6`
- Module provenance SHA-256: `6f76108a55d89c60b1a5700a184651725aed3a5e5d74e90cbbe5534519e6b84f`

## html-hangul-tag-low-byte-mismatch

Input (JSON string; escapes and final newline are significant):

```json
"<x-걁>body</x-굁>\n"
```

Observed: The root has no error: </x-굁> (U+AD41) closes <x-걁> (U+AC41) and the dump equals that of the matching pair <x-걁>body</x-걁>. The scanner stores each upper-cased tag-name code point in one byte, so names whose code points share the low byte 0x41 compare equal; </x-A> closes <x-걁> as well. The ASCII twin <x-a>body</x-b> is an ERROR.

Expected rule: An end tag whose name differs from the start tag does not close the element. The custom element name x-걁 is valid (PCENChar includes U+3001-U+D7FF), and so is the different name x-굁, which must not be treated as its end tag.

- Specification: [HTML Standard: Valid custom element name](https://html.spec.whatwg.org/multipage/custom-elements.html#valid-custom-element-name)
- Specification: [HTML Standard: End tags](https://html.spec.whatwg.org/multipage/syntax.html#end-tags)
- Upstream issue: none recorded.
- Input SHA-256: `8c47babd119327f3687226049a3a3bc8ec5a3501d467dc490c0b4fad118f92bb`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/html-hangul-tag-low-byte-mismatch.dump), SHA-256: `71b903e64d0654a58d3d97fb924ffa548093162c8c29f538c8951a289affec77`
