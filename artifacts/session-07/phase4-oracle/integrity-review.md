# Oracle comparison integrity review

The comparator previously accepted two mutually consistent sets from a stale
epoch, or records with an ABI/byte length unbound to their actual producer/input.
Six mutations now exercise stale epoch, receipt ABI mismatch, ABI outside the
range, coherently replaced ABI and range, changed runtime header, and incorrect
input bytes. Every mutation recomputes file/set hashes so checksum validation
alone cannot make the test pass.

integrity-red-01 retained the initial four failing negative cases. The first
broader run then found an existing test incorrectly assumed TEMP was outside
the repository. The contained runner puts TEMP under .scratch, so the test was
corrected to use an explicitly external path without creating or writing it.
No generator boundary was changed. integrity-green-02 passed all five tests.

Independent review found the ABI range was still defined only by each build's
own metadata. A separate runtime-abi.json anchor was derived from the pinned
runtime header after checking its source-lock hash and runtime commit. Its
header hash is a363e2f2120178702374d8e34f8af9d3b7b2124a2c9e0cc33e57ded13db59b48,
and that header advertises ABI 13 through 15. The comparator checks the anchor's
commit against current pins and build range/header against the anchor. No
generator version is fixed. Coherent-range/header mutations were added.
integrity-green-03 passed the final five tests with all six negative mutations.

existing-records-01 loaded the retained 50-record set on both sides successfully.
This is a reader compatibility check, not a new C execution or a new Go/C claim.
Evidence level is E3 for the retained tooling tests. The original epoch,
go.mod, go.sum, identities.json, fixtures and existing C records are unchanged.

Self-review: input metadata only drives bounded local reads; comparison launches
no process and performs no writes/network calls. Current epoch, ABI, inventory,
input hashes/lengths and node digests are checked before interpreting results.
Failures propagate; no assertion or gate was weakened. No unresolved finding
in the scoped diff. Authenticity still relies on reviewed repository inputs;
this is not a signature system against an attacker who can rewrite the whole
repository and its trust anchors. Product parser code is not part of this unit.
