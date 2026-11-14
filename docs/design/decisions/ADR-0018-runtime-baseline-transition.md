# ADR-0018 — Evaluate the approved runtime baseline transition

**Status:** Superseded by [ADR-0020](ADR-0020-cgo-free-library-on-unmodified-tree-sitter.md) · 2026-11-13 (accepted 2026-11-06)

This records the approved direction and candidate migration. Runtime adoption remains pending the completion gate; this is not release approval.

## Context

The product is a maintained Go syntax-analysis layer built on a pinned
open-source runtime. ADR-0013 establishes the internal carrier: the repository
owns its adapter, patches, accuracy, performance, API and evidence while
keeping upstream source origin separate.

The project owner approved the direction to evaluate the candidate baseline
recorded in docs/specs/baseline-provenance.md and to continue internal runtime
improvements when evidence supports them. This ADR records the candidate
migration under evaluation. Final runtime adoption is conditional on the
completion gate and source-bound evidence.

The exact source identity remains in tools/runtime-bundle/source.json and
internal/provenance/identities.json. This is an intentional baseline change,
not an automatic update to whatever upstream version is newest.

The C oracle runtime and transport have separate identities in
internal/provenance/identities.json.oracle. The owner approved retaining that
epoch for this upgrade. A grammar or generated-source change still changes an
oracle tuple component and requires identity-bound comparisons.

## Decision

1. Evaluate the pinned source listed in baseline-provenance and source.json as
   the candidate for the development baseline. Runtime adoption remains
   pending the completion gate.
   Keep the source archive, import recipe, patch inputs and resulting files
   bound by the runtime manifest. Do not take runtime code from _ref, a mutable
   checkout or an unverified module cache.
2. Keep the product as the repository-owned internal carrier and adapter.
   Continue to maintain source-bound patches for correctness and performance;
   do not expose upstream parser types as the product API. Production remains
   CGO-free.
3. Keep upstream-origin grammar blobs separate from derived product artifacts.
   In particular, preserve the Go C-derived product identity under ADR-0015 and
   rerun product and C comparisons for the exact resulting blob. Do not transfer
   evidence from the origin blob or an earlier manifest.
4. Keep the C oracle runtime and transport at the pins in identities.json.
   If a grammar pin, C source or generated artifact changes, regenerate and
   attribute comparison records for that exact tuple. This ADR does not move
   the oracle epoch or authorize a move to tree-sitter master.
5. Do not claim release readiness from baseline adoption. The current status
   board records the candidate's open validation work. Performance, complete
   product/platform validation and official packaging remain unfinished.

## Consequences

- The baseline contract and grammar identities must match source.json and
  identities.json; the internal manifest and GPL provenance must bind the
  final candidate.
- Old parity, performance, platform and packaging results remain historical
  unless their exact product, grammar, fixture, toolchain, manifest and lane
  identities are unchanged.
- Release-Critical requires all claimed routes to agree with the pinned C
  oracle and satisfy the incremental invariant. Release additionally requires
  the validation, provenance, performance/resource, packaging, license and
  platform gates in the repository contracts.
- The current development candidate is not release-approved. Exact final
  manifest and test evidence will be attached in the integration handoff.

## Completion gate

The existing work-unit and session completion rules in
`docs/specs/validation.md` apply to this migration. Use the product, oracle,
provenance, performance and packaging checks defined there and in
`docs/specs/publication.md`; record unavailable platform checks explicitly.
The status board must identify the exact candidate and remaining gates before
integration is considered. This ADR adds no separate approval procedure.

## Relationship to ADR-0013

This ADR records evaluation of a candidate origin baseline in place of the origin selected by ADR-0013. ADR-0013 remains Accepted for the carrier architecture and its current baseline until the completion gate is met. The approved direction retains its patch-provenance and CGO-free requirements. ADR-0002's C oracle epoch and ADR-0017's release scope remain unchanged.
