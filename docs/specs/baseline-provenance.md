# Baseline and provenance contract

Normative. Owns every version, tag, commit, blob, and hash this repository depends on.
No other document may restate these values; link here instead.

## Two states, never conflated

| State | Meaning |
|---|---|
| **Development baseline** | The exact upstream version implementation work is written against. |
| **Release-approved runtime** | A version that has additionally cleared the Release gate. |

There is currently **no release-approved runtime**. A usable development baseline is not a release.

## Development baseline candidate

The source identity for the candidate baseline transition under evaluation is
recorded in tools/runtime-bundle/source.json and
internal/provenance/identities.json:

```text
module          github.com/odvcencio/gotreesitter
version         v0.56.0
tag identity    7a1e5770e16d607a1f4dc3a6f8ce28342d0de187
peeled commit   ac4e027e02525e9d1d005c54ed5e54979271e4ed
archive SHA-256 0925d93fadca3ad943e20594d602c3639d79f9ad8f82d466262672890d7592a6
```

Use those pinned identities, not a checkout tip. This section records a
candidate identity; final runtime adoption remains subject to the completion
gate in ADR-0018. A development baseline identifies
the source used by the internal carrier; it is not release approval.

**Source origin, not product identity.** The original upstream module is input
to the product's internal runtime carrier. The product builds its own
internal/runtime and owns its patches, correctness, performance, API and validation
results. The upstream source identity remains separate from the resulting
product identity. See ADR-0013 and ADR-0018.

## Grammar identities for the candidate baseline

The origin blob hashes and grammar commits below are the candidate values in
internal/provenance/identities.json. They identify candidate upstream-origin inputs, not
a blanket correctness result. A derived product artifact is identified
separately by identities.json.runtime.grammar_blobs and its artifact manifest.

| Language | origin grammar blob SHA-256 | upstream grammar commit |
|---|---|---|
| go | `df63fc35604c4e4e7a484abde9eb2110b61640045601c23991723f323a48310d` | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` |
| python | `cde4a67dc6af6e1232dbbd1eab8618478d1d73727020e8a8002542390a452d37` | `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64` |
| javascript | `6706f93890f24d8ea90d6a140df5dde29c02ec8a3213bae16e8cc4df37e33ee0` | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| typescript | `1b6ace3de973196290411c0347013b9f2cc2385171c77dcf375e355f3c2cc90c` | `75b3874edb2dc714fb1fd77a32013d0f8699989f` (`typescript/src`) |
| tsx | `419002e62f0bbaf9a7f7bdc0fbe58799bf6ca94038550ccde33695c16859d984` | `75b3874edb2dc714fb1fd77a32013d0f8699989f` (`tsx/src`) |
| c_sharp | `198db0d7544ae78c6ba533889e972d4371bdbcaba5d438e6f51e6cb95fff9bcf` | `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` |

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

The product compiles internal/runtime, not an external runtime module at
consumer build time. internal/provenance/identities.json binds the carrier
manifest; internal/provenance/runtime.json binds the source archive,
importer/source inventory, patch inputs and original/resulting file hashes.
tools/runtime-bundle/source.json is the source and patch-input record. These
machine-readable files are authoritative; this document does not duplicate
their changing file or patch counts.

identities.json.grammars records upstream-origin grammar blobs.
identities.json.runtime.grammar_blobs records resulting product artifacts.
ADR-0015 keeps the Go origin blob distinct from its C-derived product artifact;
tools/runtime-bundle/grammars/go.json binds that conversion. Each product
artifact and its C comparisons require evidence for the exact current identity.
Evidence from an earlier blob or manifest does not transfer.

The optional GPL module is separate. Its provenance must bind the optional
payload, notices and corresponding-source inventory to the final runtime
manifest before a release claim. ADR-0014 and ADR-0017 continue to define its
distribution boundary and grammar scope.

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
