# PR description

Windows product checks previously lacked a reusable C reference set beyond the
small TSX diagnostic. This change adds an identified native C producer and 50
checked-in fixture records. Product tests compare ordered snapshots without a
C toolchain, reject identity drift, and retain exact known-difference signatures.

Generator selection resolves the installed CLI each time. Generation uses
copied grammar JSON and the pinned runtime's ABI range; new sources and binaries
get new identities. The read-only comparator checks new output before adoption.
The actual portable CLI experiment generated and compiled all six grammars.

An explicitly authorized, isolated six-line scanner candidate corrects the four
bare-equals inputs. Its other 46 input snapshots remain unchanged. Four edited
results also agree with their C references. The product dependency remains the
original baseline; a local patch evaluation does not authorize a remote fork.

C# excerpt recovery disagreement is now visible as KR-0002. Development tests
pass with exact records; JSX/TSX and C# still block Release-Critical approval.

Validation: Windows CGO-free build/vet/full tests/module check; Windows ARM64
build; four Python boundary tests; phase-local native C and candidate experiments.
Linux, remote CI, ARM64 execution, race and full release campaigns were not run.

# Reviewer guide

1. ADR-0008 and tools/native-oracle/README.md define the new execution path.
2. Review tools/native-oracle/{driver.c,run.py,identity.py} for byte fidelity,
   bounded execution, artifact identity and failure behavior.
3. Review the case catalog, known-differences.json and oracle_test.go together.
   The generated records are data; hashes, node order and first differences are
   mechanically checked rather than hand-audited line by line.
4. Phase 3's patch-decision.md, candidate.diff and comparison.json are the
   isolated proposal. The opt-in test does not relax the ordinary product gate.
5. Read each phase's own receipts for its claim. Phase 5 is the closing product
   regression, not a combination of results from the earlier phases.

Contract edits under docs/specs and the Korean handoff are intentionally local
and ignored. The public ADR and tooling README describe the relevant behavior.
This is a LARGE branch. It has not been merged, pushed or submitted as a PR.
