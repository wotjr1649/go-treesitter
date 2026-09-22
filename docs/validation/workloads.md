# Workload and fixture register

Authoritative inventory of every input used to make a claim. Identity rules live in
`docs/specs/baseline-provenance.md`; this file holds the values.

## Rules

- Every fixture has a recorded SHA-256 **of its LF bytes** and a recorded license/provenance.
- A hash mismatch fails the run. Never update a hash without stating why in the same commit.
- Inline fixtures (Go string literals) need no hash — the source file is the hash. Prefer inline for
  anything under ~1 KB.
- External fixtures are copied in with their license file. Never read them live from `_ref` or from
  `code-map-memo`; those are read-only and may move.
- `.gitattributes` must pin `*.cs`, `*.ts`, `*.tsx`, `*.js`, `*.jsx`, `*.py`, `*.go` fixtures to LF
  and mark `*.bin` as `-text`, before the first external fixture lands.

## Tier 1 — smoke (inline, present from the foundation session)

Purpose: prove the grammar loads, the adapter maps outcomes correctly, and the product lane builds.
Not parity. Not performance.

| ID | Language | Shape |
|---|---|---|
| `SM-GO` | go | package + func with a short statement |
| `SM-PY` | python | def + if + comprehension |
| `SM-JS` | javascript | arrow function returning JSX |
| `SM-JSX` | javascript (`.jsx`) | same input routed by extension |
| `SM-TS` | typescript | interface + generic arrow function |
| `SM-TSX` | tsx | component returning JSX |
| `SM-CS` | c_sharp | class with a property and an expression-bodied member |

Each runs: fresh parse → assert outcome; trivial append edit → incremental parse → assert
`incremental == fresh` **and** record the reuse/fallback reason. The equality assertion is
self-consistency, not parity (`docs/specs/oracle.md`).

## Tier 2 — known-regression ratchet (inline)

`KR-0001` fixtures F1–F7, each run against both `tsx` and `javascript`.
See `docs/validation/known-regressions.md`.

## Tier 3 — pinned public corpus (external; lands when a comparison session is authorized)

Newtonsoft.Json @ `4f73e74372445108d2c1bda37b36e6f5e43402e0`, MIT (James Newton-King).
The license file must be copied alongside the sources. Session 05 stores them
under `testdata/newtonsoft/`; `manifest.json` and the nested comparison harness
check all three LF hashes and the license before any measurement.

| File | LF bytes | SHA-256 |
|---|---:|---|
| `JsonPosition.cs` | 5,986 | `4f9e601f9d0be45c4f60f15bf1b6ca8f85dec51e353ebb50e405cc6f8e422c14` |
| `JsonTextReader-excerpt.cs` | 12,408 | `d76fd62cfc90076c11d86cb7d7a0058df181231aa3b34f30e549f650b5294d4a` |
| `MathUtils.cs` | 5,016 | `426587acd7f1f50c3d3dc94f13e9c27726c32c1bbe22d0291d86485afa368640` |

Total 23,410 bytes. `JsonTextReader-excerpt.cs` is also an upstream `gotreesitter` regression
fixture; the LF blob is 12,408 bytes while the same file in a Windows working tree with
`core.autocrlf=true` is 12,785 bytes. **Use the LF bytes.** This is the concrete reason
`.gitattributes` is mandatory.

## Tier 4 — deterministic generators (code, not files)

Generators are reproducible inputs defined in test code, with their parameters recorded.
Never check in their output.

| ID | Shape | Parameters |
|---|---|---|
| `GEN-CS-PATTERN` | C# classes with `catch (…) when (e is T Target)` filter patterns | method count 1 / 4 / 8 / 16 |
| `GEN-CS-BULK` | repeated numbered C# classes with `var x<i> = <i>; return x<i>;` | target ≥ 137 KiB; edit site = first `x0` |
| `GEN-GO-LOOKUP` | `package p` plus numbered `var item<i> = <i>` declarations | 2,048 declarations, LF; `BenchmarkSnapshotLookup`; five fixed samples of 200 operations in Session 07 |

`GEN-CS-BULK` matches the generator upstream itself uses for its C# recovered-structure parity
witness. Keep it aligned with upstream's shape so results stay comparable.

## Tier 5 — oracle digests (generated, checked in)

Produced only in a declared oracle lane (`docs/specs/oracle.md`). One digest file per
(language, fixture, oracle epoch). The Windows product lane compares against these and never needs a
C toolchain. A digest file records the oracle tuple that produced it; a digest whose tuple does not
match the current epoch is `NOT_RUN`, not a failure.

Session 06 adds `testdata/oracle/cases.json`: 50 hashed inputs covering the seven
smoke routes, both grammars' F1–F7, 24 JSX boundary controls, two generic-arrow
controls, and the three licensed Tier-3 C# files. Inline sources are authored in
this repository; C# sources retain their existing MIT origin and license.
The JSON identifies each exact UTF-8 byte sequence without newline conversion.

`testdata/oracle/windows-c/` holds one ordered C record per input plus its build
manifest. These are oracle observations, not assertions that the Go runtime
agrees. Every comparison must check identities and completeness before trees.

Session 07 adds a separate `testdata/oracle/extended-cases.json` catalog with
35 repository-authored fixed inputs (30,072 UTF-8 bytes in total). For each of
the seven routes it contains a UTF-8/CRLF base, length-changing identifier
rename, UTF-8 comment-row insertion, trailing-CRLF deletion, and a fixed
513-comment sibling boundary input. CRLF is intentional source content encoded
inside LF JSON; source SHA-256 checks include those CR bytes. The TypeScript
base includes `type T = typeof a.b` to exercise field/alias edges. The three
edit variants identify their exact previous fixture and byte ranges.

`testdata/oracle/windows-c-extended/` stores the independently executed Session
07 C records and their own build identity. Product tests compare 35 fresh
results and 21 incremental results against this set. This catalog does not
change or widen any known-regression record. Bulk performance generators remain
code-defined under Tier 4 rather than checked-in generated corpora.

Session 07's separate `typescript-contextual-cases.json` adds eight authored LF
inputs under this repository's MIT license. Each route (`.ts`, `.tsx`) contains
newline-separated `in`, optional `in?`, binary `in` across a newline, and a
semicolon-separated control. Each exact source has a SHA-256 in the catalog.
`windows-c-ts-contextual/` retains fresh C observations from Phase 2. Four
differences are independently bound by KR-0003; the other four must agree.

After explicit Candidate C adoption (ADR-0012), `windows-c-v2/` contains new
native C executions for all four existing catalogs. The contextual cases now
require exact agreement; original observations remain historical. The inline
`TestTypeScriptMaintainedGrammarFeatures` covers the four patch features in
both routes with 20 fresh repeats and one edit each. The separate 26-input
Candidate C manifest, source hashes, full trees and runtime comparisons are
retained under `artifacts/session-07/full-pass/01-candidate-c/`.

`csharp-recovery-cases.json` holds the single `CS-excerpt-no-enum` derivative:
remove the original excerpt's `internal enum` section through the next summary
comment, retaining all other LF bytes. It retains the Newtonsoft MIT notice in
`testdata/newtonsoft/LICENSE.md` and has its own exact source hash. The native C
record is in `windows-c-cs-recovery/`; KR-0004 owns its distinct signature.

## C# full-pass hardening

The full-pass hardening adds `csharp-order-cases.json` (33 records, 32 edits)
and `csharp-preservation-cases.json` (41 records, 40 edits). Each catalog includes
the same clean edit origin and records exact source hashes and edit bytes.
The cases cover conditional enum members, closed recovery prefixes, accept
ordering, pointer/declaration ambiguity, comments, renaming, Korean identifiers,
CRLF and leading hidden whitespace. C records are in the corresponding
`windows-c-v2/cs-order` and `windows-c-v2/cs-preservation` directories. The 72
target inputs were compared against three native C runs and repeated in Go
100 times for recovery inputs or 20 times for clean inputs. Native C edits were
also compared against fresh C trees for all 72 targets.

Minimal reductions and the original real-world inputs derive from the existing
Newtonsoft MIT fixtures, whose notice remains in `testdata/newtonsoft/LICENSE.md`.
Other inline cases and transformations were authored in this repository. The
JSON file itself uses LF; intentional CRLF source bytes are encoded in strings.

## Go edit-fuzz recovery

The Go edit-fuzz regression adds `go-recovery-cases.json`: four clean edit
origins and 24 recovery variants (plain, leading newline, header comment and
CRLF, each with six ASCII/UTF-8 corruptions of `package`). All are authored
under this repository's MIT license, with exact source hashes and edit ranges.
`windows-c-v2/go-recovery/` contains all 28 native C records. The minimized
`FuzzIncrementalAgreement/7596da024a0d8a5b` input remains in the adapter corpus.
Twenty Go repeats and three C executions per recovery variant agreed after
the Go version-order patch; 24 native C edits also agreed with fresh C.

## What is deliberately absent

- No fixture is copied from `code-map-memo`'s `.work/` run directories at runtime. If bytes are
  needed, they are copied in once, hashed, and licensed here.
- No repository-wide corpus. Every workload is named, bounded, and justified by a claim it supports.
