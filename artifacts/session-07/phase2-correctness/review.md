# Correctness investigation and exact regression receipts

This phase keeps its baseline, candidates, native C observations and rejected
hypotheses together. No runtime patch was adopted. The fixed runtime, grammar
blobs, C epoch and seven-route scope are unchanged. C build metadata is copied
here; source and executable identities accompany each experiment.

## Scanner candidate

The original archive and both pristine runtime copies were checked against all
3,275 files. The isolated scanner candidate differs only by deleting the three
lines rejecting `=` in each of the JavaScript and TSX JSX-text scanners.
source-identities.json records full tree hashes and both file changes.

The initial candidate test failed identity validation because a CLI -modfile
was not inherited by its nested `go list`. That failure remains in
scanner-candidate-01.log. Selecting the same modfile through GOFLAGS for the
nested command repaired the experimental invocation, not a product gate.
The complete rerun passed the 50 original cases and four incremental edits.

The final campaign independently executed the current product adapter with
the baseline and candidate and freshly executed native C for all 94 catalog
inputs. Exactly four KR-0001b inputs changed to the clean C trees; the other
90 Go snapshots and outcomes were unchanged (E5, represented node fields).
All bare ampersands remain errors. KR-0002, KR-0003 and KR-0004 retain their
recorded differences. This is one isolated candidate, not a combined runtime
patch package or a whole-catalog guarantee.

## C# recovery

Ten reduced inputs compare the original, compatibility ablation and EOF
sentinel experiments. A further 37-input reduction matrix separates member
count from comments, namespaces and names. Core and native tracing then covers
three distinguishing controls. Instrumentation was checked not to change the
corresponding trees.

For the 52-byte `members-2-0` witness, the Go missing-token sibling has already
shifted the comma when the absorber processes the real #endif. Its position is
39 and its competing cost is 610 over 19 nodes. The proposed absorber recovery
cost is 702: `(702 - 610) * (1 + 19) = 1840`, above the unchanged threshold 1800.
The native C sibling is still at position 38 and is ineligible for that election.
C retains recovery to state 6874 and consumes the real #endif. This identifies
premature sibling competition caused by differing version-processing order.
Changing the threshold or using fixture offsets would hide that cause.

C# compatibility reconstruction is a separate source of tree/error changes.
Disabling it exposes errors but loses a correct declaration normalization on
the directive-free control. An EOF any-lookahead/Accept ablation changed none
of the ten outcomes, so that hypothesis was rejected for these C# cases.

A diagnostic sibling-competition deferral plus a narrower reconstruction guard
makes the enum-only tree equal C and preserves the directive-free control.
The complete excerpt reaches 2,092 nodes on both sides but still has 19
structural difference blocks. Equal node counts are not tree equality. The
no-enum derivative has 18 difference blocks under that diagnostic variant.
The scheduling shortcut is not a complete implementation of C version order
and could alter other languages. Neither diagnostic change is suitable for
product adoption. Raw overlays stay in ignored scratch storage.

## Newly retained differences

KR-0003: four TypeScript/TSX newline `in`/`in?` inputs are clean in Go and have
errors in the original C grammar. Four binary-in/semicolon controls agree.
Upstream PR #1112 is an ancestor of the baseline and expressly uses a patched
C grammar; our original-source artifacts are different. Making valid Go input
erroneous or silently patching C would be the wrong repair. The first strict
eight-case run failed on exactly four inputs; that log remains. A new exact
record, separate from every previous ratchet, preserves the disagreement.

KR-0004: removing only the enum section from the licensed C# excerpt yields
1,910 Go nodes and accepted_clean, versus 2,051 C nodes with errors. This
false-clean recovery shape is a distinct source and signature, not an extension
of KR-0002. Its retained product test checks both complete ordered digests and
the clean-Go/error-C state. A changed, missing or disappearing signature fails.

Both new records have fresh C receipts, exact catalog/source hashes, the pinned
build/ABI identities and retained product tests (E5). They keep Development
usable while Release-Critical stays blocked. No exception accepts arbitrary
tree differences or skips a failing case.

## Review and closure

The final CGO=0 default and selected-grammar full regressions passed with all
94 records and 21 extended incremental/C comparisons; go vet passed. All older
ratchets remain active. The separate scanner candidate is not used by these
product gates. No C# or EOF ablation is used by any product test.

Self-review covered scope, clean/error outcomes, immutable evidence, exact
source identities, boundary imports and consumer limitations. Independent
reviews found no defect in KR-0003 or the separately reviewed KR-0004 branch;
both confirm existing records were not weakened. Public issue research is
recorded separately in this phase and is never substituted for local execution.
Further runtime correction and any oracle-definition reconciliation remain
explicitly deferred; no release, full-catalog or absence-of-all-defects claim.
