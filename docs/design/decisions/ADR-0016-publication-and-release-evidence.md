# ADR-0016 — Curated publication and release evidence

**Status:** Accepted · 2026-09-23

## Context

The owner requested a stop before main merge and a separation of public product
material from local investigation history, then approved execution of the
six-step closing plan. The existing session history and original receipts must
remain recoverable. Its previously published branch is a separate disclosure
surface from a newly curated candidate.

## Decision

Build a candidate on an independent `validation/release-v0.0.1` history. Copy
only reviewed product, test, license, tool and technical-documentation inputs.
Preserve original session branches and local evidence. A normal push of the
validation branch may execute the approved CI; this does not authorize main
merge, tags, remote history replacement or deletion of earlier public refs.

Keep immutable source-bound measurements as historical observations. Bind any
reuse of those observations to identical product and benchmark inputs, then
run the checks affected by publication or tooling changes. Generate fresh
packaging and native CI receipts for the actual candidate. Public projections
identify their original receipts and omit local machine paths and worktree
status. Do not weaken checks or rewrite failure observations for publication.

The owner-selected closing plan preserves the atomic C# product adoption in
ADR-0013. KR-0002 and KR-0004 remain separate hashed patches with separate
focused comparisons and a retained KR-0002-only failure. The full product
adopts both together in one passing commit, because recovery alone leaves
source reconstruction able to erase errors. This explicitly supersedes the
earlier session prompt's requirement for two separate product-adoption commits;
it does not claim the old requirement was literally met. Correctness, ablation,
provenance and complete product regression requirements remain unchanged.

## Supersedes

The session prompt's C# product-commit separation requirement is superseded.
This decision extends ADR-0005 with the independently curated validation branch.
ADR-0005's verified-commit, preservation and explicit remote authority rules
remain in force. The main-merge stop remains in force.
