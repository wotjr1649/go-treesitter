<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# Swift known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `fef1236022ccde2d2bc5c32d4db8947c87a7f848`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/swift/provenance.json).

- Source identity SHA-256: `9b2bbb6f6acb4150147c21ccd2ddb2e38a4c24aaa189432f0ce1442b6ec2731f`
- Module provenance SHA-256: `b145c841cd9af3d53ce6a2dba8d87ef8c86801a82b244755402d41c9f4f55c04`

## swift-non-ascii-statement-label

Input (JSON string; escapes and final newline are significant):

```json
"끝: for _ in 0..<1 { }\n"
```

Observed: The root has errors: an ERROR over [0,4) holds the Hangul word as a simple_identifier and the colon, followed by an unlabeled for_statement. statement_label is matched by /[a-zA-Z_][a-zA-Z_0-9]*:/; abc: for _ in 0..<1 { } parses as a labeled statement, and break 끝 and Hangul identifiers elsewhere parse.

Expected rule: A for_statement labeled 끝: a label is an identifier, and Swift identifiers may contain Hangul.

- Specification: [The Swift Programming Language: Labeled Statement](https://docs.swift.org/swift-book/documentation/the-swift-programming-language/statements/#Labeled-Statement)
- [Upstream issue](https://github.com/alex-pinkus/tree-sitter-swift/issues/658)
- Input SHA-256: `55ced9ae4acaaecd7b2e10bea37f181ed942a9f63fc0bfeaeef551c54bf60d79`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/swift-non-ascii-statement-label.dump), SHA-256: `f33ff90486aa2acb14d5f8b389cdce7ee35b97e5794506ceaf7ef1341fa97942`
