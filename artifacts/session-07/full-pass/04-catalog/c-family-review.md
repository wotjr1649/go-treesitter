# C/C++ incremental lexer state

The 206-language basic campaign found that appending a newline to the C++
smoke input turned its unchanged `preproc_include` into an ERROR. The lexer had
already consumed a reused leaf, but `SkipToByte` reset its preprocessor state.
For quoted tokens, its cursor was ahead of the token currently delivered and
resetting also destroyed the remaining token queue.

The shared C-family lexer now preserves state at the live cursor or a pending
token boundary, discarding only queued tokens covered by the reused span.
Other offsets keep the existing repositioning path. This changes one runtime
file through an inventoried patch and exact archive reproduction. Original
grammar blobs, runtime origin and the C oracle epoch are unchanged.

Evidence on Windows amd64, Go 1.27.1, CGO_ENABLED=0:

- `c-family-baseline-witness.json` records the retained test failing before the
  patch, including C and C++ include, macro and string cases.
- `c-family-product-tests.json` records all adapter and provenance tests passing
  after the patch, including 14 retained C/C++ fresh-vs-edit cases and the
  existing seven-route recorded oracle comparisons.
- `c-family-skip.json` retains the first partial candidate's failures.
- `c-family-skip-v2.json` records 15 isolated-process cases passing three fresh
  repeats, exact edit-vs-fresh nodes, pre-cancellation and input admission.
- `c-family-adoption.json` binds the final patch and runtime manifest. The final
  code additionally checks that a queued-token target is an exact boundary.

These C/C++ checks establish retained engine consistency (E3), not C parity for
the whole languages. No failure was reclassified or skipped. The new test's
build constraint includes it exactly when both grammars are compiled.

Diff review covered both target branches, queue lifetime, parser-state
preservation, ordinary repositioning, profile independence and source identity.
No unresolved issue was found in this bounded change. No API or Outcome change.
Raw logs are preserved in `c-family-command-output.zip` and bound by its manifest.
The six `git diff --cached --check` messages are literal unified-diff context
markers inside the patch file, including its blank context line. Source has no
whitespace finding; the valid patch bytes and validation rules are preserved.
