# Phase 1 evidence

Lane: Windows/amd64 product, Go 1.27.1, CGO_ENABLED=0, pinned baseline from
`internal/provenance/identities.json`. Source hashes and raw receipts are in
the smoke logs. Commands and module hashes are in `commands.jsonl`.

- WU2: nine outcomes, completion receipt tests, no upstream imports (E3).
- WU3: unsupported UTF-8/grammar, pre-cancelled, clean, error-tree, and invalid
  edit paths exercised (E3). Every non-clean backend handle is released; an
  error tree retains an explicit owned snapshot. Error messages are not exposed.
- WU4: boundary test passes; injected syntax import fails; restored test passes.
- WU5: 7/7 fresh clean, 7/7 incremental/fresh ordered snapshots equal (E3,
  self-consistency). Source bytes, nil flags, lengths and first differences logged.
  The first observation lacked reuse reasons; WU5 connected the existing profiled
  incremental API and reran the affected tests. Timings are discarded.
  Python reports `external_scanner_prefix_frontier_unproven`; C# reports
  `external_scanner_unsupported`. Both use a full reparse for this edit.
  A compact C# fresh smoke result was observed on this small fixture; no inference
  about larger corpora is made from that observation.
- WU6: workflow uses JSON syntax, a YAML subset, to permit stdlib structural
  validation. Its build/vet/test and Windows ARM64 build commands run locally.
  Action inputs checked against the official
  [checkout documentation](https://github.com/actions/checkout) and
  [setup-go documentation](https://github.com/actions/setup-go).

Review: no remaining findings after reuse attribution was corrected. Branch
scope, ownership, cancellation flag lifetime, UTF-8/edit validation, outcome
priority, snapshots, import boundary and CI permissions inspected.

NOT_RUN: remote CI (no push), race, Linux oracle, actual timeout/early-stop/
resource-limit/invariant-violation outcomes and mid-parse cancellation.
Fresh fallback detail is unavailable per parse upstream, explicitly indicated
by the diagnostic availability field. No global route counters enter the adapter.
ARM64 is build-only, not execution evidence. No release gate claim.
