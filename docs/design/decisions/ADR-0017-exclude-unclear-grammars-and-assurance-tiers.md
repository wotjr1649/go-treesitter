# ADR-0017 — Exclude two unclear grammars and state assurance by release scope

**Status:** Accepted by the owner · 2026-10-23 · amends ADR-0014

## Context

Brightscript (`ajdelcimmuto/tree-sitter-brightscript@253fdfaa`) and Cooklang
(`addcninblue/tree-sitter-cooklang@4ebe237c`) declare ISC in `package.json` and
MIT in `Cargo.toml`, with no notice file. The license gate cannot pass without
the rights holders, and neither grammar is used by the first consumer. Only
the seven Release-Critical routes (six grammars) have the C-oracle gate; the
other grammars have the basic catalog check and a few retained regression
controls (for example COBOL).

## Decision

1. Neither grammar is distributed. `tools/runtime-bundle/separate.py` excludes
   their 11 carrier files and their registry, embed and scanner registrations;
   the GPL separation is unchanged. The catalog has 204 grammars: 201 in the
   base module and 3 in `grammars/gpl`. Upstream origin records that carry only
   names (`languages.lock`, extension and language-name maps, the Core100 list,
   smoke-sample strings) and the upstream runtime's name-gated Cooklang result
   normalization stay, as for the GPL grammars; without a registration they
   resolve to an unregistered grammar.
2. Assurance is a property of a release's evaluated scope, never of an
   individual result. Release-Critical routes have the release's C-oracle (E5)
   gate; the other grammars have the basic catalog check, not that gate.
   Results carry no assurance field. Consumers that need only the core routes
   link them with the existing `grammar_subset` build tags.
3. Before v1, which grammars outside the Release-Critical routes are available,
   and whether `New()` parses them by default, may change and is not covered by
   the compatibility promise (`docs/specs/parser-result.md`).
4. Reconsider the scope at these triggers, before doing the repair work:
   - a request for a non-core grammar or a second consumer: decide an explicit
     per-language opt-in, where refusal is `not_run` with a policy reason;
   - the start of an upstream runtime upgrade: first measure the basic-check
     failures outside the core, then decide whether to keep or reduce the
     catalog.
5. A grammar is promoted to C-oracle evidence on demand: grammar-kit
   verification when its pin changes, then this repository's own oracle lane.
   Promotion does not by itself change default inclusion. A permissively
   licensed Cooklang grammar can be proposed in a new ADR if demand appears.

## Alternatives

Shipping only the core routes, inverting the build tags (about 461 carrier
constraints) and adding the opt-in API now were rejected for v0.0.1: each costs
more now than removing two grammars, and none is required by a current consumer.
The basic catalog gate stays release-blocking for all 204 grammars.
