<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# C# known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `c138aaa1053cf714569428ee598130a904c48c83`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `9150f7d56bb47f1a809fa23623f1ba1413e93fa9`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/csharp/provenance.json).

- Source identity SHA-256: `742374675380713a022e58e7c92055ca80095b454a2284ee77d20288fcf2807c`
- Module provenance SHA-256: `16408d4d74390f4287b34fadf53248d954a87ef4b95aff38fe7777e013ed1981`

## csharp-default-constraint

Input (JSON string; escapes and final newline are significant):

```json
"class C : B { override void M<T>() where T : default {} }"
```

Observed: The error-free type_parameter_constraint contains type: identifier over [45,52), whose text is default.

Expected rule: The default keyword is a primary constraint in its own right, not a class/type identifier constraint. The input uses an override; name/type resolution is outside this syntax test.

- Specification: [C# specification 15.2.5: Type parameter constraints](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/classes#1525-type-parameter-constraints)
- Specification: [C# 9 specification: default constraint](https://github.com/dotnet/csharplang/blob/main/proposals/csharp-9.0/unconstrained-type-parameter-annotations.md#default-constraint)
- Upstream issue: none recorded.
- Input SHA-256: `e450432afe015af75c5a4af54439ce6ca792b177ffe4396304ae322f1e18cda1`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/csharp-default-constraint.dump), SHA-256: `bb2cf264e2d5097bc483b0285a3e3ccafa53a1888bc862366b61a7407d72c442`

## csharp-directive-eof

Input (JSON string; escapes and final newline are significant):

```json
"#define A"
```

Observed: The root and preproc_define over [0,9) have has_error=1 without an ERROR node. The C API string shows an anonymous zero-width MISSING preproc_if_token2 at the end of preproc_define, which the dump does not list.

Expected rule: A nonempty compilation unit receives an implicit final line terminator, so #define A at EOF must parse without errors.

- Specification: [C# specification 6.3.2: Line terminators](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure#632-line-terminators)
- Specification: [C# specification 6.5.4: Definition directives](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure#654-definition-directives)
- [Upstream issue](https://github.com/tree-sitter/tree-sitter-c-sharp/issues/458)
- Input SHA-256: `e4342cebc1c4b162457213203ce862cac93c73b402f3ac7a004b6c2be418629c`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/csharp-directive-eof.dump), SHA-256: `0422ba44c497c2475466bbbecc090af5aa3251adc3ec6e21963b4a02ff9fc33e`

## csharp-empty-interpolation

Input (JSON string; escapes and final newline are significant):

```json
"var s = $\"\";"
```

Observed: The root has errors: an ERROR over [8,9) contains interpolation_start ($), followed by an ordinary string_literal over [9,11).

Expected rule: The interpolated regular string production permits zero content and zero interpolations. $"" is a valid empty interpolated string and must parse without errors.

- Specification: [C# specification 12.8.3: Interpolated string expressions](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/expressions#1283-interpolated-string-expressions)
- Upstream issue: none recorded.
- Input SHA-256: `55e49735e3373f85215a805a55fa39d41b3330363c66c8cd842737f26668e7b6`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/csharp-empty-interpolation.dump), SHA-256: `44b803dd1798e548026b8f1e848b7768ffb1065838a23f8424f56b79489f7579`

## csharp-conditional-splits-statement

Input (JSON string; escapes and final newline are significant):

```json
"class C { void M() {\n#if A\nif (true)\n#else\nif (false)\n#endif\n{ }\n} }\n"
```

Observed: The root has errors: preproc_if contains nested if_statement nodes, an ERROR over #else [37,42), and a missing semicolon at byte 53. The shared block [61,64) is parsed as a separate sibling instead of the body of the if head that either branch selects.

Expected rule: C# conditional compilation operates during lexical analysis: only the selected section supplies ordinary tokens, and unselected sections are skipped source text. Whether A is defined or undefined, this input selects one complete if head followed by the shared block. Either selection is a valid program, and so is the source with the directive lines removed (two nested if statements), so this input is a grammar defect: the grammar models #if/#else/#endif as a node holding whole statements. Inputs whose branches cannot be combined without evaluating symbols (for example a { opened in each branch) are an inherent limit of parsing every branch and are not claimed here. The register records only this input; it does not require a particular tree for inactive text.

- Specification: [C# specification 6.5.5: Conditional compilation directives](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure#655-conditional-compilation-directives)
- Upstream issue: none recorded.
- Input SHA-256: `b0e13a41549542013633c8b08a00593220c6413d0704e4bfc2d0fc3e4db07b97`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/csharp-conditional-splits-statement.dump), SHA-256: `8149d9af64945eb54e95a9594d60b0bd1dc426efff639cd07af41092929bfdb5`
