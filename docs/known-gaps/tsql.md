<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->

# T-SQL known grammar gaps

These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test
passes only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.
Remove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.

Initial E5 evidence: Windows native reference differential (windows/amd64), base commit `c138aaa1053cf714569428ee598130a904c48c83`, with this register applied on top of it.
Only the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built
`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.

Identity: grammar `2e34d86ec58f3d1a771efc0ca1e71c536690a0b7`; runtime `v0.27.2`; grammar patches: 0.
The source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),
including source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/tsql/provenance.json).

- Source identity SHA-256: `f92a08a6f2faad79363b0e32bcb4a533918cd512f35c81e247ba3497c3db4080`
- Module provenance SHA-256: `17b1492ca3d9493efd249c466c776a36283d6d50be25e0390f735ce50c96002c`

## tsql-non-ascii-identifier

Input (JSON string; escapes and final newline are significant):

```json
"SELECT 1 AS 별칭;\nDECLARE @변수 INT;\nCREATE TABLE t (열 INT);\n"
```

Observed: The root has errors: ERROR nodes cover the Hangul alias after AS [12,18), the @Hangul variable [28,35), and the Hangul column name [57,60). The three identifiers are not recognized as regular identifiers.

Expected rule: SQL Server regular identifiers may begin with a Unicode 3.2 letter, including Hangul, and may contain Unicode letters thereafter. A leading @ denotes a local variable. The unquoted alias 별칭, variable @변수 and column 열 in this input satisfy these identifier rules and should not cause syntax errors.

- Specification: [Microsoft Learn: Database identifiers — Rules for regular identifiers](https://learn.microsoft.com/en-us/sql/relational-databases/databases/database-identifiers?view=sql-server-ver17#rules-for-regular-identifiers)
- Upstream issue: none recorded.
- Input SHA-256: `c472521c07e5373fcf69a734e9a3acc72b6e1efa4a9552b2bb56f399eb384aa3`
- [Recorded native dump](../../tools/conformance/testdata/known-gaps/tsql-non-ascii-identifier.dump), SHA-256: `58866d9108f250a422d7b3df33c50221237cd7814591502acf0eff762e164a14`
