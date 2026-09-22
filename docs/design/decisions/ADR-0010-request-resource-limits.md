# ADR-0010 — Explicit request resource limits

Status: Accepted, 2026-09-22, Session 07.

The adapter previously exposed a timeout but no runtime work/memory budget and
always materialized the entire snapshot. Add own Limits fields that map to the
existing pinned runtime APIs, plus input/snapshot caps owned by this adapter.
No upstream types or process-global settings become public options.

Reject negative options and nil contexts as not_run. A cancelled context wins
admission; oversized source is rejected before scanning/copying it. Preserve raw
stop reasons and record an adapter limit reason separately. Complete requires
the snapshot walk to finish. Do not return a partial snapshot as a usable tree.

The snapshot traversal uses a stack of active ancestors, not all siblings.
Cancellation is checked during traversal. Independent workers keep private
parsers/trees; this does not permit sharing or editing one tree concurrently.

Runtime growth budgets exclude retained slabs. Also check the returned arena
plus scratch receipt before snapshot construction and reject an over-budget
result with an adapter-owned reason. Runtime budgets remain checkpoint
thresholds, not hard process-memory caps.
All error returns carry a type receipt; raw upstream error types take precedence.
Context cancellation retains errors.Is identity. ExpectedEOFByte becomes uint64
to describe rejected oversized inputs without truncation; node offsets stay
uint32. This is an intentional change to the still-unreleased diagnostic API.
Zero options retain previous runtime behavior. This decision does not move the
baseline, grammar identities, oracle epoch, language scope or gate definitions.
