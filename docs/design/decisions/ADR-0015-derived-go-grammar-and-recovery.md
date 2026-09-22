# ADR-0015 — Bind the Go grammar artifact separately from its origin

Status: Accepted, 2026-09-23, under the owner's approved internal runtime and
correctness-improvement scope. Extends ADR-0013.

The original Go blob combines the pinned C grammar's newline and literal-NUL
terminals into an automatic-semicolon mechanism. Edit-fuzz reductions exposed
different EOF and error-recovery trees. Repairing only the visible result would
hide the grammar distinction and would not restore the parser's state machine.

Use the existing pinned upstream `cmd/ts2go` converter on the exact checked-in
C `parser.c` from the already pinned Go grammar commit. Keep the upstream
module, grammar commit, C runtime and C receipts unchanged. The resulting Go
blob is a derived product artifact, with its own hash in
`identities.json.runtime.grammar_blobs`; `identities.json.grammars` continues
to identify the original upstream blobs. `VerifyRuntime` checks both the origin
and product hashes against the complete runtime manifest.

`tools/runtime-bundle/grammars/go.json` binds the C input, source ZIP, original
MIT notice, converter source and invocation, and resulting blob. The source ZIP
and maintained binary patch are included in runtime reproduction. An independent
conversion reproduced the exact blob bytes. Compact-route certificates for the
old grammar do not transfer. Generic runtime admission can still select a compact
route; each measurement records the route and fallback actually observed.

The C API exposes its literal-NUL token name as an empty string. Preserve that
name and its actual byte/point span. Oracle validation permits only the exact
anonymous Go NUL byte or zero-width EOF terminal and rejects malformed or
non-Go empty names. It does not erase nodes or normalize their ranges.

The separately inventoried recovery patches preserve hidden missing-token
costs, accepted-root raw structure, EOF transitions, version lookahead and
JS-family token widths. Incremental interior nodes require an existing
lookahead-dependency receipt before reuse; otherwise their leaves are replayed.
The source-start skipped prefix is also checked before reuse. These constraints
preserve normal top-level reuse, while avoiding guesses about unseen lookahead.

All affected C records and minimized fuzz failures are retained. Recovered
bare-ampersand tree exceptions are removed after exact C agreement; the grammar's
error-state characterization remains. Correctness precedes performance, and
the final release still needs its resource, packaging and platform gates.

Retire the derived artifact or carried changes only after an explicitly
approved upstream replacement passes the retained EOF, recovery, edit and
negative-control corpus. No result may inherit another artifact's evidence.
