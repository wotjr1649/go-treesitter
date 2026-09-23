# Parser result and consumer contract

Normative. Defines what a parse must report and what a consumer may never assume.
Derived from observed downstream failures recorded in
`artifacts/handoff/2026-09-22-code-map-memo-findings-and-adoption-review.md`.
Transfer the contract, not any downstream product logic.

## The three traps this contract exists to prevent

1. **A returned `error == nil` does not mean the parse completed.** The upstream runtime can return
   a tree *and* a nil error on a timeout.
2. **`root.EndByte() == len(source)` does not mean the parse completed.** A real observation: root
   span covered the full input while the last consumed token was at byte 3233 of 5986, and the stop
   reason was `timeout`.
3. **`stopReason == accepted` does not mean the tree is clean.** A real observation: `accepted`,
   not stopped early, full root span, and `HasError() == true`.

Any of these three checked alone will silently publish a wrong result.

## Outcome vocabulary

A parse returns exactly one outcome. The set is closed; adding a member is a contract change.

| Outcome | Meaning |
|---|---|
| `accepted_clean` | Parse reached input EOF, valid root span ending at EOF, no error and no missing nodes. |
| `accepted_with_errors` | Parse finished, but the tree contains error or missing nodes. |
| `early_stop` | Runtime reported it stopped before natural completion. |
| `timeout` | The parse deadline was reached. |
| `cancelled` | The caller's context or cancellation flag fired. |
| `resource_limit` | An iteration, stack-depth, node-count, or memory budget was hit. |
| `invariant_violation` | The runtime reported an internal invariant failure. |
| `unsupported_input` | Invalid UTF-8, or no grammar registered for the input. |
| `not_run` | Nothing was attempted (precondition failed, or the lane is unavailable). |

Only `accepted_clean` may be treated as a complete, trustworthy tree.
`accepted_with_errors` is a *usable* tree with an explicit caveat; it is never silently equivalent
to clean.

## Required evaluation order

Request admission precedes runtime work: a nil context or negative timeout/limit
is `not_run`; an already cancelled context takes precedence over source checks.
A positive `Limits.MaxInputBytes` rejects oversized input as `resource_limit`
before UTF-8 scanning, grammar loading or source copying. No raw runtime stop
reason is invented for a request that never entered the runtime.

Evaluate in this order. Stopping early at any step and skipping the rest is the defect class above.

```
1  caller context cancelled / deadline exceeded      -> cancelled
2  input not valid UTF-8, or no grammar              -> unsupported_input
3  returned error != nil                             -> record the error TYPE (never the message)
4  returned tree == nil                              -> not_run / invariant_violation
5  runtime stop reason                               -> timeout | cancelled | resource_limit |
                                                        invariant_violation
6  runtime reported "stopped early"                  -> early_stop
7  root == nil                                       -> invariant_violation
8  root has error or missing nodes                   -> accepted_with_errors
9  otherwise                                         -> accepted_clean
```

On any non-`accepted_*` outcome the tree must be released and not handed to a consumer as if it
were a result.

## Required diagnostic surface

Every parse produces a diagnostic record alongside the outcome. Minimum fields:

```
outcome                    input bytes             grammar / language id
stop reason (raw)          stopped-early flag      root present / root span
has-error / has-missing    error type (not text)   deadline applied
tokens consumed            last token end byte     expected EOF byte
iterations + limit         nodes + limit           peak stack depth + limit
arena / scratch bytes      memory budget           truncated flag
route taken                fallback reason (raw upstream string, verbatim)
snapshot complete          snapshot node count / cap
input byte cap             adapter-owned limit reason
```

Rules:

- **Store raw upstream reason strings verbatim.** They are the stable join key to upstream code and
  to `docs/validation/known-regressions.md`. Do not paraphrase or normalize them.
- **Never put timing values into anything that forms a deterministic identity** (digests, semantic
  IDs, golden files). Timing goes in the evidence record, never in the result identity.
- Error *messages* may embed source text. Record the error **type** only. Prefer
  the upstream error type when present; otherwise record the adapter error type.
  Cancellation preserves the context error for `errors.Is` checks.
- `ExpectedEOFByte` is uint64 so a rejected input beyond the runtime's uint32
  range can still report its length without wrapping. Node offsets remain uint32.

`Result.Complete` additionally requires a completed snapshot walk. An accepted
runtime receipt alone cannot make a snapshot stopped by cancellation or a node
cap appear complete. `LimitReason` identifies `input_bytes`, `snapshot_nodes`
or `runtime_memory`; it never replaces or normalizes the raw `StopReason`.

Completion checks both `LastTokenEndByte == len(source)` and
`ExpectedEOFByte == len(source)`, plus an ordered root span ending at input EOF.
The root need not start at byte zero: some grammars exclude leading hidden
extras from that node. The pinned COBOL C grammar starts at byte 7 after seven
spaces, or at byte 6 after fixed-format sequence numbers. Preserve those raw
coordinates. Do not widen or relabel the root to satisfy a completion test.
Root coverage alone still cannot establish completion; the stop, EOF, snapshot,
truncation, cancellation and error/missing observations all remain required.

## Request limits

`syntax.Limits` exposes an input byte cap, snapshot node cap, runtime memory
budget, and runtime iteration/node/stack-depth limits. Zero preserves upstream
defaults and imposes no additional adapter cap; negative values are rejected.
The snapshot cap is exact for returned snapshot nodes. Traversal state grows
with tree depth, not the number of pending siblings.

The runtime budget checks growth beyond retained arena and scratch slabs, so a
warm parse can report accepted with a footprint above that budget. The adapter
also rejects a result when reported arena plus scratch bytes exceed the explicit
budget, before constructing its snapshot. The raw accepted stop remains intact
and `LimitReason=runtime_memory` explains this local result-admission failure.
It is not a hard process-RSS cap and excludes grammar caches, input
copies, snapshots and other workers. A parser step can exceed work thresholds.
Configured runtime limits may change the selected parsing route. Callers needing
a total process-memory boundary must manage the process and worker concurrency.
Context cancellation also interrupts snapshot construction. The existing
`Timeout` continues to bound the parse API only, with grammar loading outside it.

## Failure propagation

- A non-clean outcome must reach the caller as an explicit state. It must never be published as an
  authoritative result with the failure dropped.
- A consumer that keeps a previous good state must be able to distinguish
  "legitimately empty input" from "empty because the parse failed". Those are different outcomes,
  not the same empty result.
- Whether to publish, keep the previous state, or mark something stale is a **downstream** policy.
  This repository's job is to make the distinction unambiguous, not to choose the policy.

## Comparison discipline

When comparing two trees or two derived fact sets:

1. **Check completeness on both sides first.** Never compare a complete result against an incomplete
   one — that produces a false "drift" report. This exact mistake cost a downstream session a
   misattributed failure.
2. **Capture both sides and the first difference *before* asserting.** An assertion that fires
   without having recorded the two inputs destroys the only evidence that could diagnose it.
   Record at least: both lengths, nil-ness, the first differing index, and both values at that index.
3. **Do not sort before comparing.** Ordering defects are real defects; sorting hides them.
4. Compare the dimensions listed in `docs/specs/oracle.md`, not an ad-hoc subset.

## Timing and lane separation

- Phases are distinct and separately budgeted: grammar load, parser construction, token-source
  setup, **parse API**, tree validation, and everything after. A parse deadline applies to the parse
  API only; grammar loading happens outside it.
- Cold and warm are different conditions. Profiler-on and profiler-off are different conditions.
  Never mix them in one number.
- The CGO-free product lane and the CGO+race diagnostic lane are different lanes.
  A race-lane timeout is **not** a product-gate failure. See `docs/specs/validation.md`.

## Ownership and lifetime

- Parser, query, and scanner objects are **worker-local** unless stronger sharing has been proven by
  a test. Shared-parser thread safety is not assumed.
- Trees are explicitly released. Release exactly once, on every path including failure paths.
- Do not introduce a global pool or cache without a measurement that shows it is needed.
- Upstream exposes some diagnostics as **process-global counters**. Deltas around a parse are only
  valid when parses are sequential within the process. Any concurrent use must not read them.
  If per-parse routing information is required under concurrency, that is an upstream request, not a
  local workaround.

## Scope boundary

This layer resolves **syntax**. It does not resolve names, scopes, overloads, types, imports, or any
language semantics. Those belong to a downstream consumer. A fixture may be shared between the two,
but the judgments must not be mixed: "the parser produced this node" and "this identifier binds
there" are separate claims with separate oracles.
