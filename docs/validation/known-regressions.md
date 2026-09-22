# Known regressions register

Authoritative list of failures that are known, upstream-owned, and signature-matched.
Semantics and gate behavior: `docs/specs/validation.md` § Known-regression ratchet.

A record here keeps the **Development gate green** and the **Release-Critical gate red** for the
languages it names. It never loosens an assertion.

## Record format

```
ID
upstream issue
baseline identity          (the exact version the signature was observed at)
fixtures                   (exact bytes, hashed)
expected affected set      (which fixtures fail, and which must NOT fail)
expected observable state  (outcome, stop reason, completeness, node count)
release impact             (which gate it blocks, for which languages)
retirement condition       (what must be true to delete this record)
```

---

## Upstream issue `#1242` splits into two records

Reported as one issue ("a bare `&` or `=` in JSX text produces an ERROR node"). Root-cause analysis
against the pinned C sources shows **two unrelated causes with opposite verdicts**. They must not
share a record, because "fixing" the first would *break* C agreement.

| Record | Symptom | Cause | Verdict |
|---|---|---|---|
| `KR-0001a` | bare `&` | grammar requires `&…;`; the C scanner stops at `&` too | **C-faithful.** Not a defect of this runtime. |
| `KR-0001b` | bare `=` after a leading identifier | a heuristic that exists **only** in the Go port | **Confirmed TSX divergence (E5).** JavaScript remains source-level E1. |

The original report compared against `tsc` 5.9.3, **not** against C tree-sitter. Comparing to a
different oracle and reporting the difference as a runtime defect is exactly the evidence error
`docs/specs/validation.md` § Claim vocabulary forbids. Both records below carry their own oracle.

### Shared fixtures

Seven single-line sources, each parsed as `x.tsx` (grammar `tsx`) and as `x.jsx` (grammar
`javascript`). Store as LF, inline in the test — no external files.

```
F1  const a = <p>Org & Team</p>;      KR-0001a
F2  const b = <p>AT&T</p>;            KR-0001a
F3  const c = <p>&</p>;               KR-0001a
F4  const d = <p>a = b</p>;           KR-0001b
F5  const e = <code>k=v</code>;       KR-0001b
F6  const f = <p>x &amp; y</p>;       control — must stay clean
F7  const g = <p>plain</p>;           control — must stay clean
```

Observed at the baseline (`v0.53.0` @ `c871b1f5`) and also at upstream `main` @ `095a1876`:
F1–F5 produce an error tree in **both** grammars; F6–F7 are clean in both.
Every failing case reports itself as successful — that shape is the defining signature:

```
outcome        accepted_with_errors      (NOT timeout, NOT early_stop)
stop reason    accepted
stopped early  false
root span      covers the full input
has error      true
```

Reference node counts at the baseline (same-walker comparison only):

| Fixture | `tsx` | `javascript` |
|---|---:|---:|
| F1 | 16 | 18 |
| F2 | 16 | 18 |
| F3 | 18 | 17 |
| F4 | 17 | 15 |
| F5 | 17 | 15 |
| F6 | 19 | 19 |
| F7 | 17 | 17 |

Witness shape for `const a = <p>a & b</p>;` under `tsx`:

```
(program (ERROR (identifier) (ERROR (jsx_opening_element (identifier)) (jsx_text)) (identifier) (regex_pattern)))
```

---

## KR-0001a — bare `&` in JSX text (C-faithful, **not** a runtime defect)

| Field | Value |
|---|---|
| **Upstream issue** | `odvcencio/gotreesitter#1242` (partially) — and, properly, a `tree-sitter-javascript` grammar question |
| **Fixtures** | F1, F2, F3 |
| **Verdict** | TSX F1–F3 have errors in both runtimes (E5). Their recovered-tree shapes differ; only the error-state characterization agrees. |

### Root cause

Two facts, read from the pinned sources:

1. `tree-sitter-typescript` @ `75b3874edb2dc714fb1fd77a32013d0f8699989f`, `common/scanner.h`,
   `scan_jsx_text` — the C loop terminates on exactly:

   ```c
   while (lexer->lookahead != 0 && lexer->lookahead != '<' && lexer->lookahead != '>' &&
          lexer->lookahead != '{' && lexer->lookahead != '}' && lexer->lookahead != '&') {
   ```

   **`&` is in the C stop set.** The Go port's identical condition is faithful.

2. The only grammar rule that can consume the `&` is `html_character_reference`:

   ```
   &(#([xX][0-9a-fA-F]{1,6}|[0-9]{1,5})|[A-Za-z]{1,30});
   ```

   It **requires a trailing `;`**. A bare `&` matches nothing, and `jsx_text` has already stopped,
   so nothing consumes it.

That `&amp;` (F6) parses cleanly is independent behavioural confirmation: the clean path exists only
for a well-formed character reference.

### Consequence

The C implementation produces an error here too. A "fix" that makes bare `&` parse cleanly would be
a **divergence from the oracle**, not a correction. The reporter's bisect (good at `v0.20.9`, bad
from `v0.21.0`) is best explained by a grammar bump that introduced `html_character_reference` and
the `&` stop — a change that moved the runtime *toward* C, not away from it.

### Required test behaviour

Assert that F1–F3 produce `accepted_with_errors`. This is a **characterization test**, not a
ratchet on a defect. If these ever become clean without an oracle change, that is a **divergence
from C** and must fail.

### Release impact

Blocks nothing on correctness grounds. It is a **usability limitation inherited from the grammar**:
real-world JSX containing `&` will not produce extractable top-level definitions.
Whether that is acceptable for a release is a product decision, not a correctness one.

### Retirement condition

`tree-sitter-javascript` / `tree-sitter-typescript` change the grammar so a bare `&` is valid JSX
text, gotreesitter adopts that grammar commit, and our baseline moves to it. Not retirable by any
change in this repository or in gotreesitter's runtime.

---

## KR-0001b — bare `=` after a leading identifier (TSX divergence)

| Field | Value |
|---|---|
| **Upstream issue** | `odvcencio/gotreesitter#1242` (partially) |
| **Fixtures** | F4, F5 |
| **Verdict** | **Confirmed for TSX at E5.** The native C probe parses F4/F5 cleanly; pinned Go reports errors. JavaScript's cause remains E1; its Go failures are retained at E3. |

### Root cause

`grammars/runtime/tsx_scanner.go` `tsxScanJsxText` (and the identical
`grammars/runtime/javascript_scanner.go` `jsScanJsxText`) contain a block with **no counterpart in
the C scanner**:

```go
if onlyWhitespace && (lexer.Lookahead() == '_' || unicode.IsLetter(lexer.Lookahead())) {
    for { /* consume identifier chars */ }
    for unicode.IsSpace(lexer.Lookahead()) { lexer.Advance(false) }
    if lexer.Lookahead() == '=' {
        return false          // <-- rejects jsx_text
    }
    ...
}
```

The C `scan_jsx_text` quoted under `KR-0001a` is the complete function: it has **no `=` logic and no
`/` logic**, and `tsx/src/scanner.c` merely delegates to it. So for `<p>a = b</p>` the C scanner
consumes `a = b` as `jsx_text` and stops at `<`, while the Go port returns false and the element
falls apart.

This maps exactly onto the reporter's second bisect (good at `v0.35.0`, bad from `v0.36.0`).

The heuristic appears to be a JSX-attribute guard (`<div foo=…>`) that also fires inside element
*content*. The adjacent `/` + `onlyWhitespace` block is a second Go-only addition; whatever it
compensates for is unknown, so neither block may be removed without evidence.

### Evidence status — read this before acting

Session 05's native diagnostic compares pinned TSX C and Go parses of F1–F7.
F4/F5 are clean in C and erroneous in Go (E5); F6/F7 ordered tree digests agree.
F1–F3 remain erroneous on both sides, with different recovered trees.
The user revised generator selection in ADR-0007 before this probe: checked-in
ABI 14 sources are identified by commit/hash and accepted by the pinned runtime's
13–15 ABI range. No generator ran and its historic version is not asserted.

Evidence: `artifacts/session-05/phase2/native-manifest.json`, native output and
differential logs. The earlier identity-blocked attempt is retained separately.
JavaScript C execution and the effects of changing the slash/whitespace guards
remain untested. **No patch or fork is authorized by this result.**

### Fork-trigger evaluation (as of this record)

| Condition | Status |
|---|---|
| Reproduces at the exact pinned baseline | **yes** (`v0.53.0`, retained Phase 2 product test) |
| Appears in the CGO-free product lane | **yes** (a plain fresh parse; not race-dependent) |
| Upstream has no fix in a tagged release, and is not already tracking it | **no tagged fix observed**; public tag query on 2026-09-22 ends at v0.53.0; issue open without maintainer response or linked PR |

All three conditions currently read as satisfied. That makes `KR-0001b` the **first and only**
candidate that has ever reached the trigger — and precisely why the `E5` requirement matters before
anyone acts on it. Creating a fork remains a **user-owned decision**.

### Required test behaviour

Assert that F4–F5 currently produce `accepted_with_errors`, tagged as a **divergence ratchet**
rather than as accepted behaviour. If they become clean, that is the fix landing — investigate and
retire, do not silently pass.

### Release impact

Release-Critical gate **red** for JSX and TSX until resolved.

### Retirement condition

Either (a) upstream ships a tagged release in which F4–F5 are clean in both grammars and our
baseline moves to it, or (b) an authorized patch is proven against the C oracle at `E5` and carries
its own retirement condition. Move the record to a retired section; do not delete it.

---

## Known characteristics (not regressions, not defects)

These are measured, upstream-documented properties. Reporting them as defects is a reporting error.

### KC-0001 — C# compact declines on the recorded real corpus

At the baseline, the recorded C# real-corpus parses decline the compact scheduler and fall back
to the classic GLR path. Upstream's pinned matrix records `c_sharp: 0 PASS / 2 FALLBACK`, the worst of
the Release-Critical set (`go`, `python` 2/0; `javascript` 1/0; `typescript`, `tsx` 1/1).

Consequences to expect, not to file: non-linear parse cost on catch-pattern-dense input
(input ×3.38 → nodes ×12.06, time ×18.88 at four sizes); a stable decline reason string worth
recording verbatim; no incremental reuse for C#, so every edit is a full reparse — still correct.

Widening compact admission is upstream work. Because upstream documents and tracks it, this does
**not** count toward the fork trigger.

Do not generalize the corpus observation to every C# input. Session 05 Phase 1's
small SM-CS fixture took the compact route; its incremental edit used a full
reparse. Phase 3 independently recorded declines on its three larger inputs.

### KC-0002 — the race lane is roughly 14× the product lane

Measured on the same C# input: 253 ms (product) vs 3,687 ms (race), same deadline.
A race-lane timeout is a diagnostic observation. It never fails a product gate.

---

## Open investigations (no ratchet yet)

### OI-0001 — `accepted` + `HasError` on the upstream C# excerpt fixture

Upstream's own C-parity test asserts `HasError() == false` for `jsontextreader_excerpt.cs` on the
classic production route with the compact route explicitly disabled. A downstream consumer observed
`HasError() == true` on the same bytes with the compact route enabled (and therefore declined).

**Not yet a ratchet** — not reproduced at the current baseline. The experiment that would settle it
is a 2×2 over `{v0.52.0, v0.53.0} × {default route, compact disabled}`.
Until reproduced at the baseline, do not cite it as a defect of `v0.53.0`.

Session 05 Phase 3 executed all four cells once per fixture with the same C#
grammar blob and LF corpus. The excerpt's `HasError` was false in all cells:
the reported route-dependent error flag was not reproduced. All four instead
contained a missing `;` at byte 335 (row 6, column 26), yielding
`accepted_with_errors` under this repository's contract. A separate retained
adapter test covers that receipt (E3). This is not a new ratchet or a claim of
C# oracle agreement; C# differential diagnosis remains open. Node/token counts
and outcome were unchanged across cells. See Phase 3 evidence for conditions.
