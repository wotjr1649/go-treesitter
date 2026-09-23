# ADR-0012 — Adopt the complete maintained TypeScript oracle patch

Status: Accepted, 2026-09-22. The owner explicitly approved Candidate C in the
full-pass execution turn after reviewing its identity and comparison results.

The pinned Go grammar already includes upstream's maintained TypeScript patch.
Comparing it with the original C grammar made KR-0003 a comparison between
different language definitions. Making the Go result erroneous would regress
valid syntax. A partial scanner-only C patch would leave three other grammar
features without the same definition.

Adopt the complete upstream patch for the TypeScript and TSX C artifacts.
The C runtime, grammar base commits, product dependency and six Go blobs stay
at their existing identities. `oracle.typescript_patch` in identities.json is
the authoritative patch identity and binds a manifest of generated grammar JSON
and scanner sources. Builds validate those inputs and record the actual current
generator and compiler. No npm lifecycle script or C library enters the product.

The comparison used an unpatched regenerated control with the same generator.
Across 26 feature/control inputs, that control matched the original C trees,
and Candidate C matched the product's complete ordered snapshots. Each runtime
repeated each input 20 times. Go edits and all three C variants' edits matched
their respective fresh results. On the original 94-input set, only the four
registered contextual C differences changed. Other known failures stayed visible.

The original C records and KR-0003 definition remain available as historical
evidence. Active records are freshly produced under `windows-c-v2/`; no old
receipt is relabeled. This decision retires KR-0003 only, not the scanner or
C# correctness blockers, and does not constitute Release approval.

Supersedes: the original-source-only TypeScript artifact choice in the oracle
contract. ADR-0002's runtime epoch and ADR-0007's generator policy remain in force.
