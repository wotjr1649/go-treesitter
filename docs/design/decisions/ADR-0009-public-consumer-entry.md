# ADR-0009 — Public parser construction

Status: Accepted, 2026-09-22, Session 07 consumer integration work.

An external module can import syntax types but cannot construct the internal
adapter. Add treesitter.New at the module root, returning the existing
syntax.Parser interface. No backend registry, upstream types or new interface.

A retained test creates a separate module and runs the public path through
fresh parsing, an incremental edit, old-tree release, error outcomes and
cancellation. README points to its runnable source. Tree ownership stays
worker-local and consumers inspect completion/outcome before using results.

This resolves the construction gap without publishing a version tag or claiming
Release approval. The unreleased API may still evolve under ADR-0004.
