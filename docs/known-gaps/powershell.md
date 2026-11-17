<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# PowerShell known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `65ea6919e816a7ac142acec7ee72757f89db8439`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/powershell/provenance.json).

- Source identity SHA-256: `65eb15e64f6ee61fbb99ddf4ef30daf04969251c00e790c739fe94d63ae573d0`
- Module provenance SHA-256: `88599b11c325f80c683a593cdbfb6fa989132fad357f2f1d1d91c2cb45b4a0a9`

## powershell-non-ascii-variable

Input (JSON string; escapes and final newline are significant):

```json
"$값 = 1\n"
```

Observed: The root has errors: an ERROR over [0,6) holds a nested ERROR over the Hangul name [1,4); $값 is not a variable node. Variable names are matched by [a-zA-Z0-9_]+; $abc = 1 parses without errors.

Expected rule: A variable node for $값: a variable name is made of Unicode letters (Lu, Ll, Lt, Lm, Lo), digits and _. PowerShell 7 accepts this assignment.

- Specification: [PowerShell language specification 2.3.2: Variables](https://learn.microsoft.com/en-us/powershell/scripting/lang-spec/chapter-02#232-variables)
- [Upstream issue](https://github.com/airbus-cert/tree-sitter-powershell/issues/70)
- Input SHA-256: `db49aa30808854327648da38cc72ba6ff061d6b94ffd85f676cc7ed73007b305`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/powershell-non-ascii-variable.dump), SHA-256: `ff2553c1e47efffd60e016e45fe33395cb23277545d31828077d2bcf51d6a960`

## powershell-non-ascii-parameter

Input (JSON string; escapes and final newline are significant):

```json
"Get-Item -이름 1\n"
```

Observed: The root has errors: an ERROR over [10,16) (with a nested ERROR over the same bytes) covers the Hangul part of the parameter name after the dash; -abc parses without errors.

Expected rule: A command parameter -이름 (first character a Unicode letter of class Lo) followed by the argument 1. PowerShell 7 binds such a parameter.

- Specification: [PowerShell language specification 2.3.4: Parameters](https://learn.microsoft.com/en-us/powershell/scripting/lang-spec/chapter-02#234-parameters)
- [Upstream issue](https://github.com/airbus-cert/tree-sitter-powershell/issues/70)
- Input SHA-256: `abd09ce235e8bf26fbda31ed9ba8e71dc98fb11e4419079023c3563971bd8501`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/powershell-non-ascii-parameter.dump), SHA-256: `2c005039026c270d0236b4ae5ba2e59e0ff32ad2e28a531b14b77d9cf6feaf97`

## powershell-non-ascii-hash-key

Input (JSON string; escapes and final newline are significant):

```json
"@{ 키 = 1 }\n"
```

Observed: The root has errors: inside the hash literal an ERROR over [2,11) holds a nested ERROR over the bare-word key [2,6); the ASCII key in @{ abc = 1 } parses without errors.

Expected rule: A hash entry whose key-expression is the simple name 키, with value 1. PowerShell 7 creates a hashtable with key 키.

- Specification: [PowerShell language specification 7.9: Hash literal expression](https://learn.microsoft.com/en-us/powershell/scripting/lang-spec/chapter-07#719-hash-literal-expression)
- [Upstream issue](https://github.com/airbus-cert/tree-sitter-powershell/issues/70)
- Input SHA-256: `36d8389a9207140ff42e58070fb6acfbe34a3e379f3a042645a38baf7713be1e`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/powershell-non-ascii-hash-key.dump), SHA-256: `3dfeaf6bbfa19ccb41d7f3f3bf4dc87e0b7902130ba89d4a9424abc992c4a0bb`

## powershell-non-ascii-member-name

Input (JSON string; escapes and final newline are significant):

```json
"$a.속성\n"
```

Observed: The root has errors: the root is an ERROR holding the variable $a [0,2), the dot [2,3) and an ERROR over the Hangul member name [3,10); $a.abc parses without errors.

Expected rule: A member access on $a with the member name 속성. PowerShell 7 reads such a property.

- Specification: [PowerShell language specification 7.1.2: Member access](https://learn.microsoft.com/en-us/powershell/scripting/lang-spec/chapter-07#712-member-access)
- [Upstream issue](https://github.com/airbus-cert/tree-sitter-powershell/issues/70)
- Input SHA-256: `d2957ffd9cf127b90abe9e4501222c83cb046282b2cd92849890c41a91b988e9`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/powershell-non-ascii-member-name.dump), SHA-256: `0b112ae9830abee51162581a268b68aa5c736d39d43836c49ccf586b1e332436`

## powershell-non-ascii-type-name

Input (JSON string; escapes and final newline are significant):

```json
"[한글]$a\n"
```

Observed: The root has errors: an ERROR over [0,8) holds a nested ERROR over the Hangul type name [1,7). Type names are matched by [a-zA-Z0-9_]+.

Expected rule: A cast or type literal whose type identifier is 한글: type characters are Unicode letters (Lu, Ll, Lt, Lm, Lo), digits and _. Whether the type exists is outside this syntax test.

- Specification: [PowerShell language specification 2.3.5.7: Type names](https://learn.microsoft.com/en-us/powershell/scripting/lang-spec/chapter-02#2357-type-names)
- [Upstream issue](https://github.com/airbus-cert/tree-sitter-powershell/issues/70)
- Input SHA-256: `457f66f792b63d6f3b42fe25797be4b4f3cd9a8a69efc579a9273cd8d622606e`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/powershell-non-ascii-type-name.dump), SHA-256: `df14b91b3f302cf2fe155dab67530176c8d691a3c85cd94bf142d394ce57721b`
