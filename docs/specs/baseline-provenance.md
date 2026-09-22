# Baseline and provenance contract

Normative. Owns every version, tag, commit, blob, and hash this repository depends on.
No other document may restate these values; link here instead.

## Two states, never conflated

| State | Meaning |
|---|---|
| **Development baseline** | The exact upstream version implementation work is written against. |
| **Release-approved runtime** | A version that has additionally cleared the Release gate. |

There is currently **no release-approved runtime**. A usable development baseline is not a release.

## Development baseline

```
module   github.com/odvcencio/gotreesitter
version  v0.53.0
tag      f102332cca9dc45e841b9439d34ba5cc59f0b10f   (annotated tag object)
commit   c871b1f576866c40b1695677fe3512243e266d39   (peeled, 2026-09-19 01:40:16 -0700)
```

Selected in `artifacts/handoff/2026-09-22-v053-baseline-reconciliation.md`.
`v0.53.0` was the latest release at selection time; there is no newer tag to prefer.

**Source origin, not product identity.** The original unmodified runtime does not satisfy
Release-Critical correctness for JSX and TSX. The owner-approved internal carrier applies
separately identified patches (ADR-0013); its origin remains this baseline. See `docs/validation/known-regressions.md` (`KR-0001`) and
`docs/reports/release-critical-status.md`. Development may proceed; a release claim may not.

## Grammar identities at the baseline

Bound to `v0.53.0`. These are *not* the values at upstream `main`; do not copy from a checkout tip.

| Language | grammar blob SHA-256 | upstream grammar commit (`grammars/languages.lock`) |
|---|---|---|
| go | `9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08` | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` |
| python | `cde4a67dc6af6e1232dbbd1eab8618478d1d73727020e8a8002542390a452d37` | `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64` |
| javascript | `6706f93890f24d8ea90d6a140df5dde29c02ec8a3213bae16e8cc4df37e33ee0` | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| typescript | `46d8d4f7a0056db32e874500ae5b19170237e1628a63a9e3a401e0ee426d6126` | `75b3874edb2dc714fb1fd77a32013d0f8699989f` (`typescript/src`) |
| tsx | `bf8c490b0bbeb6d4150abce2edc193552e44b093893665dde69bd39e9e940e85` | `75b3874edb2dc714fb1fd77a32013d0f8699989f` (`tsx/src`) |
| c_sharp | `7ad425e89733339dde94e3c03b762ae478fb453b530493f5d62e1ae7537e1784` | `88366631d598ce6595ec655ce1591b315cffb14c` |

`.jsx` resolves to the `javascript` grammar; it is not a separate blob.

## Machine-readable source of truth

Prose drifts from tests. The values above must also exist once, in machine-readable form, and the
tests must read that file rather than re-typing constants:

```
internal/provenance/identities.json     # the single source
internal/provenance/provenance.go       # go:embed + typed accessors
internal/provenance/provenance_test.go  # asserts the source carrier and binary identity
```

This document explains the values. The JSON binds them. If they disagree, the JSON is authoritative
and this document is a defect.

## Internal carrier

The product compiles `internal/runtime`, not a replaced external module.
`identities.json.runtime` records the internal import path and the SHA-256 of
`internal/provenance/runtime.json`. That manifest binds the exact original
archive, source inventory, import relocation tool, patch inventory and every
original/resulting file. The original grammar blob hashes above remain unchanged.
Changing the carrier requires regenerating and reviewing this identity, then
rerunning its affected checks. This is distinct from changing the upstream origin.
See ADR-0013 and `tools/runtime-bundle/README.md` for reproduction and retirement.

ADR-0014 separates 19 GPL-specific files and three aggregate registrations into
the optional grammar module. The main carrier has 1,508 files; its manifest
also binds the separator and excluded original hashes. The optional module's
`provenance.json` binds its generated files, notices and corresponding sources.
The 206 original grammar blobs, upstream origin and C-oracle epoch are unchanged.

The Go recovery-order patch also enables the existing C version transaction
for the exact Go blob above. Its fresh/edit C witnesses and minimized fuzz seed
are retained in the workload register. The carrier manifest includes this
patch; this does not select a different upstream version or grammar.

## Identity rules

1. **Never transfer evidence across identities.** A measurement taken at one version, blob, lane, or
   fixture hash says nothing about another. If any component differs, the comparison is void.
2. **Never label a new result with an old producer.** Re-run or mark `NOT_RUN`.
3. **A checkout tip is not a tag.** Read values from the tag object, never from a working tree that
   happens to be checked out at something else.
4. **Line endings are part of identity.** See "Fixture bytes" below.
5. **Downgrading or upgrading the baseline is a contract change**, requiring an ADR, a re-run of the
   affected gates, and a handoff. It is never a side effect of another task.

## Fixture bytes

`core.autocrlf` is `true` on the development machine and Git normalizes text on commit. The same
fixture read from a checkout and from a module cache can therefore differ in byte count and in every
span it produces. This has already been observed upstream: one C# fixture is 12,408 bytes (LF) in
the Git blob and 12,785 bytes (CRLF) in the Windows working tree.

Required:

- `.gitattributes` must exist at the repository root before the first fixture lands, pinning text to
  LF and marking binary fixtures `-text`.
- Every fixture is recorded in `docs/validation/workloads.md` with its SHA-256 **of the LF bytes**.
- A test verifies recorded hashes at run time. A hash mismatch fails the run; it never gets "fixed"
  by updating the hash without an explanation.

## Upgrade procedure

1. Open an ADR stating why, with the candidate identity.
2. Re-derive every value in this document from the candidate tag object.
3. Re-run the gates whose evidence is identity-bound (all of them, in practice).
4. Record old and new values side by side in a handoff; do not overwrite the old ones silently.
5. Update this document and `internal/provenance/identities.json` in the same commit.

An upgrade is never authorized by "a newer version exists".
