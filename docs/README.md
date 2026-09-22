# Documentation map

Entry point for agents and humans. Read one row, not the tree.

## Two planes

| Plane | Path | Mutability | Authority |
|---|---|---|---|
| **Contract plane** | `docs/` | living; updated when the contract changes | **normative** — this is what you must obey |
| **Evidence plane** | `artifacts/` | append-only; never edited after writing | historical record of what was observed |

If a historical handoff conflicts with a `docs/` contract, **the contract wins**.
A handoff never becomes an instruction by being cited.

## Contract plane

| Path | Kind | Answers |
|---|---|---|
| `specs/baseline-provenance.md` | contract | Which upstream version are we on, what exactly is pinned, and how may it change? |
| `specs/oracle.md` | contract | What is the syntax oracle, and what does upgrading it require? |
| `specs/parser-result.md` | contract | What must a parse result expose, and what must a consumer never assume? |
| `specs/validation.md` | contract | Lanes, gates, evidence levels, claim vocabulary, Definition of Done. |
| `validation/workloads.md` | register | Which exact bytes are our fixtures, with hashes, licenses, and line endings. |
| `validation/known-regressions.md` | register | Which failures are known-upstream, with exact signatures and retirement conditions. |
| `design/overview.md` | contract | Module layout, layer boundaries, and the parser boundary pattern. |
| `design/decisions/` | ADRs | Why a durable decision was made, and what supersedes it. |
| `reviews/review-checklist.md` | procedure | The self-review dimensions to run before committing. |
| `reports/release-critical-status.md` | status board | Current per-language gate state. Changes as gates move. |
| `plans/` | plan | The scope a session is authorized to execute. |
| `prompts/` | prompt | The exact text used to launch a session. `prompts/references/` pins external design sources. |

**contract** = must obey · **register** = the authoritative list of identities/exceptions ·
**status board** = current state, not a promise · **plan/prompt** = scope for one session.

## Evidence plane

| Path | Contents |
|---|---|
| `artifacts/handoff/` | One immutable document per completed session: what was done, observed, and left open. |

## Where a new document belongs

1. Does it state a rule someone must obey? → `docs/specs/` (or an ADR if it is a durable choice).
2. Is it a list of identities or accepted exceptions? → `docs/validation/`.
3. Does it describe structure or boundaries? → `docs/design/`.
4. Is it the record of one session's observations? → `artifacts/handoff/`. Never edit it later.
5. Is it none of these? It probably does not need to exist.

Do not create a second document that restates an existing one. One canonical owner per rule;
everything else links to it.

## Reading budget

`AGENTS.md` is always loaded. Everything here is loaded on demand.
A one-line test fix should read zero files from this tree.
