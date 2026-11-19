<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# CSS known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/css/provenance.json).

- Source identity SHA-256: `b9a0f4f0a51a022cf9c5dbb58c3a53b812db256ef13e389c253021fade559c07`
- Module provenance SHA-256: `ab58ffc15e92df2b6f9efb5633085657f6de894e2cf8edfeb33ffdd5e9426432`

## css-non-latin1-identifier

Input (JSON string; escapes and final newline are significant):

```json
".한글 { a: b }\n"
```

Observed: The root has errors: the rule is an ERROR over [0,16) with nested ERROR nodes over the Hangul class name [1,7) and the declaration characters a [10,11) and b [13,14). The identifier regex accepts ASCII letters, _ and U+00A0-U+00FF only, so no Hangul syllable can start or continue an identifier; the ASCII twin .abc { a: b } parses without errors.

Expected rule: The Hangul syllables in this input are in the allowed non-ASCII ident range U+3001-U+D7FF, so .한글 is a class selector and the rule parses without errors.

- Specification: [CSS Syntax Module Level 3 (2026-10-01): ident-start code point](https://www.w3.org/TR/2026/CRD-css-syntax-3-20261001/#ident-start-code-point)
- Upstream issue: none recorded.
- Input SHA-256: `c44e56c8e90aefcabb5524f92a06c1bd411fa417c98a5097f873bc68a28ffa6a`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/css-non-latin1-identifier.dump), SHA-256: `72e0bc04a41301b1575bbd9466831ff1c2a0a3941c070cc5b4b40f85c9a07095`

## css-non-latin1-value

Input (JSON string; escapes and final newline are significant):

```json
"a { b: 한글 }\n"
```

Observed: The root has errors: the declaration is an ERROR over [4,13) and the Hangul value [7,13) is a nested ERROR. The value identifier uses the same Latin-1-limited regex as selectors; a { b: abc } parses without errors.

Expected rule: 한글 is an identifier value (plain_value) of the declaration b, with no error.

- Specification: [CSS Syntax Module Level 3 (2026-10-01): ident-start code point](https://www.w3.org/TR/2026/CRD-css-syntax-3-20261001/#ident-start-code-point)
- Upstream issue: none recorded.
- Input SHA-256: `9e72dd72130d55d7d5294e143ea1fbd1656ec0c3a7879e306db4fb46d9c730a1`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/css-non-latin1-value.dump), SHA-256: `1e2f75f20d41affef8fa61e82f8570e8b5432cabbd307af2b2b93048e5ea58f3`
