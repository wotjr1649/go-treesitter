<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# Ruby known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `b5d00506d20ad65ddd84a8e5b9195b0fe7a32e2f`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `ad907a69da0c8a4f7a943a7fe012712208da6dee`; runtime `v0.27.2`; grammar patches: 1.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/ruby/provenance.json).

- Source identity SHA-256: `5bade487c23b69aa12c533a7f99c3f0cd0f5353f713672995fa40b03ecca877f`
- Module provenance SHA-256: `5e35a40f7732dd36521090e57f56be114a4f99a42972c6623c7e3ad432418bce`

## ruby-non-ascii-symbol

Input (JSON string; escapes and final newline are significant):

```json
"a = :한글\n"
```

Observed: The root has errors: the colon [4,5) is an ERROR node and the Hangul word [5,11) is parsed as a separate identifier, so there is no simple_symbol. :abc parses as a simple_symbol; %s(한글) and { 한글: 1 } parse without errors.

Expected rule: A simple_symbol :한글 on the right of the assignment.

- Specification: [Ruby 3.3 syntax: Literals (Symbol Literals)](https://docs.ruby-lang.org/en/3.3/syntax/literals_rdoc.html#label-Symbol+Literals)
- [Upstream issue](https://github.com/tree-sitter/tree-sitter-ruby/issues/292)
- Input SHA-256: `ab172f39384a073ea728126cdf4c99a6abcea56ea9d597046ce156aa5fa9a0c3`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/ruby-non-ascii-symbol.dump), SHA-256: `3e1608561ae0e0f94fd804caed90a3774f5e586c37cb93afdb85a1ebdd425b36`

## ruby-non-ascii-global-variable

Input (JSON string; escapes and final newline are significant):

```json
"$전역 = 1\n"
```

Observed: The root has errors: the dollar sign [0,1) is an ERROR node (nested ERROR) and the Hangul name [1,7) starts a separate assignment with an identifier on its left. $abc = 1 parses as a global_variable assignment; @필드 and @@클래스 parse.

Expected rule: A global_variable $전역 assigned 1.

- Specification: [Ruby syntax: Assignment (Global variables)](https://docs.ruby-lang.org/en/3.3/syntax/assignment_rdoc.html#label-Global+Variables)
- Upstream issue: none recorded.
- Input SHA-256: `9f198f0b9205eae6a2276ed454de726e3823017f9f451141782a85ead409fa0b`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/ruby-non-ascii-global-variable.dump), SHA-256: `3d49ee51af0fbdff6eec0d5903c9a85a18305cc4ceabf6d828f9abb3014c69e4`
