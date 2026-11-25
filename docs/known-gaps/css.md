<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# CSS known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f`; runtime `v0.27.2`; grammar patches: 1.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/css/provenance.json).

- Source identity SHA-256: `5396081629b52157f4813d97914bc1955341e8ed8dc277b2b2a3eeb63f0bac2e`
- Module provenance SHA-256: `df7598196c394f7766c8ecd563bbf5823a22421d2362c3705ec44f8a2ce3c953`

## css-non-latin1-value

Input (JSON string; escapes and final newline are significant):

```json
"a { b: 한글 }\n"
```

Observed: The root has errors: the declaration is an ERROR over [4,13) and the Hangul value [7,13) is a nested ERROR. The value token for bare identifiers retains the pinned Latin-1 range; a { b: abc } parses without errors.

Expected rule: 한글 is an identifier value (plain_value) of the declaration b, with no error.

- Specification: [CSS Syntax Module Level 3 (2026-10-01): ident-start code point](https://www.w3.org/TR/2026/CRD-css-syntax-3-20261001/#ident-start-code-point)
- [Upstream pull request](https://github.com/tree-sitter/tree-sitter-css/pull/105#issuecomment-7140588000)
- Reported as a comment on open pull request [#105](https://github.com/tree-sitter/tree-sitter-css/pull/105); no maintainer response as of 2026-11-25.
- Input SHA-256: `9e72dd72130d55d7d5294e143ea1fbd1656ec0c3a7879e306db4fb46d9c730a1`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/css-non-latin1-value.dump), SHA-256: `1e2f75f20d41affef8fa61e82f8570e8b5432cabbd307af2b2b93048e5ea58f3`
