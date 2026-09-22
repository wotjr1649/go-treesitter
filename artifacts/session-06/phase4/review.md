# Phase 4 — Actual generator execution

The initial missing-tool observation in Phase 2 is retained. This phase reviewed
the official portable CLI release and its published SHA-256, downloaded only
that public asset, and placed its executable in task-local .scratch/tools.
No global installation, PATH change or generator release constant was added.
The acquisition receipt identifies the exact artifact; this is a test input,
not a configuration pin.

Actual tool: tree-sitter 0.27.0. Resolve it through a relative TREE_SITTER_CLI
path, capture its executable hash, regenerate six grammar JSON inputs in copied
build directories and compile them against the unchanged pinned C runtime.
All generated grammars have ABI 15, within that runtime's 13–15 range.
TypeScript/TSX checked-in sources used ABI 14; their new artifacts have new IDs.
Both artifact sets were executed for all 50 inputs within this phase.
Every represented ordered tree and root error state was equal. No result was
copied to the new producer identity, and existing product records were not replaced.

Generator warnings are retained: Go `_type` and Python `_simple_statement` /
`_compound_statement` are both supertype and inline, so supertype metadata is
ignored by this generator. This comparison covers tree fields, not query or
supertype metadata. It is not a release claim for those unassessed dimensions.

Review fixes: resolve executable paths before changing working directory;
capture generator and compiler hashes before execution and reject changes
during execution; add a read-only record-set comparator with inventory, epoch,
source, completeness and digest checks. Its mismatch path is covered by a
retained test using a copied real record. Prepared source sets remain intact.

Checks: actual CLI version/help; six actual generation/builds; two identified
50-input C executions; full record-set comparison; tooling boundary tests;
source-integrity check. Commands and build identities are in this phase.
NOT_RUN: Linux and remote CI. No epoch migration or product patch occurred.

Official sources reviewed:
- https://tree-sitter.github.io/tree-sitter/cli/generate.html
- https://github.com/tree-sitter/tree-sitter/releases/tag/v0.27.0
- https://github.com/tree-sitter/tree-sitter/releases/expanded_assets/v0.27.0
