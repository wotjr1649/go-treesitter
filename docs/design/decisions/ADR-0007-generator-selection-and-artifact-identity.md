# ADR-0007 — Dynamic generator selection, immutable evidence inputs

Status: Accepted · 2026-09-22 · explicit user instruction in Session 05.

The user requested a generator policy that accepts updates without editing a
fixed version. The original native probe stopped because the checked-in parser
used ABI 14 while configuration listed only ABI 15 and 13.

Resolve the installed generator for each generation, optionally through
`TREE_SITTER_CLI`. Capture its actual version and generated source hashes.
Validate ABI against the pinned C runtime's minimum and maximum, rather than
a hand-maintained list. A checked-in parser needs no generator execution; its
commit and source hash identify it, with historic producer explicitly unknown.

The native probe uses `tools/native-oracle/identity.py` for both modes. New
generator versions are accepted immediately when they produce a compatible ABI.
Evidence is always regenerated and bound to the actual artifacts. This changes
generator selection only; runtime/grammar pins, release scope, no-fork policy
and all comparison checks remain in force.

Supersedes the fixed generator/ABI preference in the original oracle tuple.
No general plugin system, version resolver or automatic tool installer is needed.
