# Self-review checklist

Run before every commit. This is a review of the **diff**, not of the plan.
Skip a dimension only when it is genuinely not touched — and say so in the evidence note.

## Always

| # | Dimension | Ask |
|---|---|---|
| 1 | **Scope control** | Does the diff contain anything the work unit did not require? Unrelated formatting, drive-by renames, "while I was here" fixes — remove them or split the commit. |
| 2 | **Correctness** | For each changed branch: what input reaches it, and what does it produce? Is there a case that reaches it with the opposite expectation? |
| 3 | **Failure paths** | Every error return, every early return, every non-`accepted_clean` outcome: is the tree released, the diagnostic populated, and the state propagated rather than swallowed? |
| 4 | **Contract conformance** | Does the change still satisfy `docs/specs/parser-result.md`? Specifically: full evaluation order, closed `Outcome` set, raw upstream reason strings kept verbatim, no timing in any identity. |
| 5 | **Boundaries** | Does any package outside the adapter import upstream? Did an upstream type leak into `syntax`? Did any language *semantics* appear in this repository? |
| 6 | **Tests** | Does a test fail if the change is reverted? If not, the test does not test the change. |
| 7 | **Known regressions** | Did any `docs/validation/known-regressions.md` signature change? A disappeared failure is as significant as a new one. |
| 8 | **Provenance** | Any version, tag, hash, fixture, or lane touched? If yes, is `docs/specs/baseline-provenance.md` (and `identities.json`) updated in the same commit? |
| 9 | **Evidence** | Does every claim in the commit body and handoff carry its level from `docs/specs/validation.md`? Any use of "parity", "proven", or "verified" without `E5`/`E6`? |
| 10 | **Git hygiene** | One work unit. Tests ran before the commit. No unrelated user work staged. Message states what ran. |

## When applicable

| # | Dimension | Ask |
|---|---|---|
| 11 | **Windows behavior** | Path separators, line endings, file locking, case-insensitive paths. Was this actually exercised on Windows, or only assumed? |
| 12 | **CGO boundary** | Does the product import graph still build with `CGO_ENABLED=0`? Did anything CGO-only reach a non-diagnostic path? |
| 13 | **Resource lifetime** | Trees released exactly once on every path. No parser/scanner retained past its source version. No new global pool or cache without a measurement. |
| 14 | **Concurrency / ownership** | Worker-local unless sharing is proven. Process-global upstream counters are not read under concurrency. |
| 15 | **API shape** | If `syntax` is public: is this change source-compatible? If not, is that intended and recorded? |
| 16 | **Fixtures** | LF, hashed, licensed, recorded in `docs/validation/workloads.md`. Not read live from `_ref` or `code-map-memo`. |

## Second-opinion review

Request an independent review **only** when at least one holds:

- the change alters the `Outcome` set, the evaluation order, or the boundary rule;
- it changes a known-regression record or a gate definition;
- it touches provenance or the oracle contract;
- it introduces concurrency into a previously sequential path.

Do not request one for ordinary implementation, tests, or documentation. Multi-agent review on a
trivial change costs time and adds no signal.

## Output

The review result goes in the commit body and is summarized in the session handoff.
A review that found nothing says so explicitly; silence is not a result.
