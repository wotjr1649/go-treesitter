# ADR-0014 — Preserve 206 grammars with a separate GPL module

**Status:** Implements the owner's selected MIT-centered distribution policy · 2026-09-23

The pinned catalog contains GPL caddy, disassembly and jq grammars. Replacing
them would change parser identities and behavior; a comparable permissive
disassembly grammar was not found, and the jq alternative has conflicting
license metadata. Keep the existing grammar artifacts in a separate Go module.

The base module contains 203 grammars. `grammars/gpl` contains the other three,
their Go scanners, external lex-state tables and queries. A side-effect import
registers them before parsing, using the existing registry and grammar cache.
The optional module depends on base v0.0.1; the base has no reverse dependency.
All 206 continue through the same public parser/result contract and basic gate.
The seven Release-Critical C-oracle paths are unchanged.

Each module confines runtime imports to its own `internal/gtsadapter` and the
runtime carrier itself. The GPL module exports no runtime types. Registration
happens only during package initialization; parsing introduces no registry
writes. The main module's existing import-boundary check remains unchanged.

A separate go.mod excludes the optional module from the base module ZIP under
the [Go module rules](https://go.dev/ref/mod#vcs-dir). Build tags alone do not.
The evidence directory is also a separate module so diagnostic archives do not
enter the product ZIP. Its local evidence remains tracked and preserved.

The GPL source archives, source notices, conversion tool and dependency source
are included with the optional module. Main-catalog notices bind all 206 fixed
commits and artifacts. Nim retains MPL-2.0 and its pinned source archive.
Brightscript/Cooklang's contradictory ISC/MIT metadata remains an explicit
release licensing question, not an invented license grant or a reduced
technical support scope. No remote publication is part of this decision.
