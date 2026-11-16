<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# Python known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `c138aaa1053cf714569428ee598130a904c48c83`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64`; runtime `v0.27.2`; grammar patches: 1.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/python/provenance.json).

- Source identity SHA-256: `541729622eee59fa4885980a139ec10badbc43394468e0aa6d327149f6925932`
- Module provenance SHA-256: `99acbb437ac979fa8f0d30b074d11396b03357f9febe25ce275bb5c8eef80756`

## python-bitwise-precedence

Input (JSON string; escapes and final newline are significant):

```json
"a ^ b & c"
```

Observed: The error-free outer binary_operator is & over [0,9), with a left ^ subtree over [0,5): (a ^ b) & c.

Expected rule: & binds tighter than ^, so the expression must group as a ^ (b & c).

- Specification: [Python reference 6.9: Binary bitwise operations](https://docs.python.org/3/reference/expressions.html#binary-bitwise-operations)
- [Upstream issue](https://github.com/tree-sitter/tree-sitter-python/issues/358)
- Input SHA-256: `03545f6a4d6bce1014bb6e30b243104f137f8ee3ad2c3656e7980e5a66a3cb39`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/python-bitwise-precedence.dump), SHA-256: `dcf314e5282f04508b5d8e8fd8d1d4e689aee0fc4471e959e04a0acffb4d6b5b`

## python-await-power

Input (JSON string; escapes and final newline are significant):

```json
"async def g():\n    await f() ** 2\n"
```

Observed: The named await node spans [19,33), containing the ** binary_operator over [25,33): await (f() ** 2), without errors.

Expected rule: await_expr takes a primary; power takes await_expr on its left. Group as (await f()) ** 2; the await operand ends at byte 28.

- Specification: [Python reference 6.4: Await expression](https://docs.python.org/3/reference/expressions.html#await-expression)
- Specification: [Python reference 6.5: The power operator](https://docs.python.org/3/reference/expressions.html#the-power-operator)
- Upstream issue: none recorded.
- Input SHA-256: `cd893c4c0e887b6f60b5b21dc8d9b61bdd2b8c01787f314ae4ad69b45e3e065e`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/python-await-power.dump), SHA-256: `28cca2fdede80ce83a60920a29c26db0f0f2f8d29e1e6f7f212e3cdfbc87e021`
