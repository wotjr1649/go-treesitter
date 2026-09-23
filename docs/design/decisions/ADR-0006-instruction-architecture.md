# ADR-0006 — Instruction architecture: AGENTS kernel, docs router, no skill

**Status:** Accepted · 2026-09-22

## Context

Three sessions of investigation produced roughly 240 KB of handoff documents. Without a deliberate
structure, the next implementation session would either re-read all of it (burning context on
material irrelevant to the task) or be handed a giant pasted prompt (the same cost, paid every time).

Design references consulted, pinned in `docs/prompts/references/`:

- OpenAI, *Rethinking skills and prompts for GPT-6 Astra* — contextual routing over mandatory
  reading; progressive disclosure; a skill's root document should be a minimal router; overly
  specific itineraries now hurt; define completion up front; grant explicit permission rather than
  relying on the model to ask.
- Official Codex `AGENTS.md` guidance — files merge root-downward, closer files override, combined
  size capped at 32 KiB by default.
- `agents.md` specification — agents read the nearest file; the closest one wins.

Host facts verified locally: Codex is installed with a global `~/.codex/AGENTS.md`; Claude Code is
at **2.1.278**, and since **2.1.277** Claude Code reads `AGENTS.md` when a folder has no
`CLAUDE.md`.

## Decision

### 1. One minimal root `AGENTS.md`, no nested files yet

Options were: minimal root kernel only (A), root + nested overrides (B), broad root manual (C).

Chosen: **A**. Nested `AGENTS.md` files require directories that exist and have rules that genuinely
differ from root. This repository currently has **no source directories at all**. Creating nested
files now would be drift with no benefit. Option C is exactly the context bloat the Astra guidance
warns about, and would also compete for the 32 KiB budget.

**Trigger for adopting B later:** add a nested `AGENTS.md` only when a directory acquires a rule
that *contradicts or materially narrows* root — the two foreseeable cases are `testdata/`
(fixtures are immutable, LF-pinned, hash-verified) and the adapter package (the only place upstream
may be imported). Add it when that directory exists and the rule has bitten at least once.

### 2. `AGENTS.md` holds only durable, repo-wide rules

It contains: what the repository is and is not; a routing table; hard boundaries; evidence
discipline; autonomy and prohibitions; git, test, and completion behavior.

It deliberately **excludes**: benchmark numbers, issue statuses, the blocker table, fixture details,
runbooks, the oracle tuple, the compatibility matrix, historical corrections, session instructions,
and version numbers. Those live in `docs/` and are read on demand. The rule "the baseline is pinned"
is durable; the value `v0.53.0` is not — so the rule is in the kernel and the value is in
`docs/specs/baseline-provenance.md`.

The routing table follows the Astra pattern directly: *"Use X when doing Y"*, never *"read X, Y, Z
before every edit"*.

### 3. `AGENTS.md` only — no `CLAUDE.md`

Because Claude Code ≥ 2.1.277 falls back to `AGENTS.md`, a single file covers Codex, Claude Code,
and any agents.md-compliant tool. A second file would be a duplicate that silently drifts.

Caveat: the fallback is not available on Bedrock, Vertex, or Foundry deployments. If the project
ever runs there, add a `CLAUDE.md` whose entire content is an import of `AGENTS.md` — never a
second copy of the rules.

### 4. No skill

Evaluated: a `go-treesitter-validation` skill routing to the oracle contract, Windows lane, fixture
manifest, and scripts.

Rejected, for four reasons:

1. **It would be a third routing layer.** `AGENTS.md` already routes, and `docs/README.md` already
   does progressive disclosure. A skill adds indirection without adding information.
2. **Its trigger words are unavoidably generic.** "validation", "test", "parity" would load it
   during ordinary Go edits — precisely the accidental-loading cost the Astra guidance describes.
3. **It would duplicate the contracts**, and a duplicate that can drift is worse than a link.
4. **Skills are host-specific.** Committing to one makes the repository less portable, against the
   host-neutrality goal.

**Prefer scripts to skills.** For any recurring multi-step procedure, write an executable script in
`scripts/`. A script is host-neutral, testable, reviewable in a diff, and cannot drift from what
actually runs — three properties prose does not have.

**Revisit when:** there are ≥ 3 distinct multi-step runnable procedures re-invoked across many
sessions, they already exist as scripts, *and* the project has settled on a primary host. Then a
skill may be worth adding as a **minimal router** over those scripts.

### 5. Session prompts are routers, not payloads

A session prompt states scope, authority, prohibitions, and completion criteria, and **links** to
contracts. It never pastes them. Historical handoffs are never pasted into a prompt at all.

### 6. Language

Agent-facing files (`AGENTS.md`, `docs/`) are written in English for host and model neutrality.
Human-facing session handoffs in `artifacts/handoff/` stay in Korean, matching the existing record.
This is reversible and carries no technical dependency.

## Consequences

- A one-line test fix reads `AGENTS.md` and nothing else.
- Each rule has exactly one canonical owner; everything else links to it. Duplication is treated as
  a defect, because it is the mechanism by which instructions go stale.
- The 32 KiB Codex budget is not a constraint at this size — the kernel is well under 8 KiB.
- If the instruction set grows past roughly two screens, that is the signal to move material into
  `docs/`, not to raise the limit.

## Supersedes

None.
