# ADR-0013 — Distribute the approved runtime inside the module

**Status:** Accepted by the project owner · 2026-09-23

## Context

The exact pinned upstream runtime reproduces KR-0001b, KR-0002 and KR-0004.
The owner explicitly approved including the required runtime and grammar sources
under `internal/`, retaining provenance and MIT notices, and carrying their
minimum fixes. The first project release version will be `v0.0.1`.

A `replace` directive in this repository would not propagate to consumers.
An internal source bundle lets an ordinary versioned module carry the same
implementation that its product tests exercise, without a remote fork.

## Decision

`internal/runtime` contains an identified subset of the upstream module archive:
the production packages, their build-tag alternatives, embedded data and origin
license. Upstream's baseline version, tag, commit and grammar blobs remain the
origin identity; the internal carrier has a separate manifest and patch identity.
No source is read from `_ref` or a mutable module cache at product run time.

Only `internal/gtsadapter`, and packages inside the carrier, may import the
carrier. Consumer-facing types remain repository-owned. Product builds remain
CGO-free. The root module has no external runtime dependency or replace directive.

`tools/runtime-bundle/import.py` reproduces the bundle from a hash-checked local
archive into a new directory. Import paths are relocated mechanically. Runtime
changes are maintained as separate reviewed patches with regression witnesses;
generated files are not edited as an alternative to maintaining their patch.
`internal/provenance/runtime.json` binds every original and resulting file hash,
the importer, its source inventory and each patch. `identities.json` binds the
manifest. Missing, changed or additional runtime files fail provenance checks.

The initial patch is the six-line KR-0001b scanner correction. KR-0002 and
KR-0004 have separate patches, focused C comparisons and ablation evidence.
Their product adoption is atomic: core recovery alone leaves source-based
reconstruction free to clear errors on reduced inputs, so the intermediate
state does not pass the product suite. A rejected experiment is not a runtime patch.
Further improvements require matching evidence and must not change the pinned
origin, oracle epoch or Release-Critical scope as a side effect.

Retire a carried patch when a separately approved upstream baseline supplies the
same behavior and the relevant positive and negative tests pass without it.

The external consumer test installs a local versioned `v0.0.1` module ZIP through
a file module proxy, with an empty module cache and no replace directives. This
proves local packaging and consumption, not registry publication or release
approval. Version tags, remote writes and actual releases remain separate acts.

## Supersedes

ADR-0001's prohibition on an owned runtime and its replace-based distribution
topology, for this explicitly approved internal carrier. Its origin-pinning and
evidence requirements remain applicable.
