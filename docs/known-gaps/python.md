<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# Python known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64`; runtime `v0.27.2`; grammar patches: 2.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/python/provenance.json).

- Source identity SHA-256: `8fb2d425ba28d52f392ecc16852ccb8d71075d48455b39be5dc2b3bdfbcdf1c2`
- Module provenance SHA-256: `be4a924f1af1217319b3139723df00a25accb334d2af92814b5e6adc47a2e640`

## python-bitwise-precedence

Input (JSON string; escapes and final newline are significant):

```json
"a ^ b & c"
```

Observed: The error-free outer binary_operator is & over [0,9), with a left ^ subtree over [0,5): (a ^ b) & c.

Expected rule: & binds tighter than ^, so the expression must group as a ^ (b & c).

- Specification: [Python reference 6.9: Binary bitwise operations](https://docs.python.org/3/reference/expressions.html#binary-bitwise-operations)
- [Upstream issue](https://github.com/tree-sitter/tree-sitter-python/issues/358)
- Issue [#358](https://github.com/tree-sitter/tree-sitter-python/issues/358) was closed on 2026-11-21 by its reporter, who pointed to a community fork; the fix is not in upstream `master`.
- Input SHA-256: `03545f6a4d6bce1014bb6e30b243104f137f8ee3ad2c3656e7980e5a66a3cb39`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/python-bitwise-precedence.dump), SHA-256: `dcf314e5282f04508b5d8e8fd8d1d4e689aee0fc4471e959e04a0acffb4d6b5b`
