# Grammar-owned root spans and EOF completion

The first 206-language basic campaign rejected a complete COBOL parse because
the adapter required every root to start at byte zero. The pinned C runtime and
COBOL grammar exclude the fixed-format leading area: the three recorded roots
start at byte 7, byte 7, and byte 6. All ordered node properties match the C
oracle after preserving these coordinates.

Completion now checks a valid ordered root span ending at EOF, both runtime EOF
receipt fields, accepted stop, no truncation or early stop, and a completed
snapshot. The closed Outcome set, priority order, raw reasons, non-clean tree
lifetime and public types are unchanged. The extra EOF requirements prevent a
root that merely covers the input from masking an incomplete token stream.

Windows amd64, Go 1.27.1, CGO_ENABLED=0 evidence:

- `cobol-baseline-witness.json` retains the old adapter's `early_stop` rejection
  under a Go overlay. The wrapper requires that specific failure.
- `../cobol-completion-witness.json` records the three retained C comparisons and
  the completion receipt's negative controls passing.
- `../eof-coverage-tests.json` records `go test ./... -count=1 -timeout=150s -json`
  passing after the completion change.
- `after-fixes.json` binds the exact adapter, completion source, runtime carrier
  and 206 unchanged input hashes. All 206 passed three fresh repeats, deterministic
  observations, newline edit vs fresh nodes, pre-cancellation, input admission,
  and repeated Close. This is basic coverage, not 206-language C parity.

`catalog-cobol.json` binds all three C records to their source hashes, grammar
commit/blob, runtime epoch, driver and executable. Native C source compilation
and results are retained in `cobol-c-oracle.json` and `build-cobol-oracle.json`.
The source is an authored minimal COBOL sample with a fixed-format variation.

Diff review covered both acceptance and rejection branches, all completion
callers, snapshot failure propagation, tree ownership and compatibility. No
unresolved finding remains in this bounded change. Private source was not sent
to a delegate; independent native C observations and retained negative controls
provide the additional review evidence. Native Windows ARM64 is unrun here.
Raw command outputs are archived byte-for-byte in `completion-command-output.zip`
with SHA-256 hashes in its companion manifest. Original failures remain intact.
