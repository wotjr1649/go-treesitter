# ADR-0008 — Windows native C oracle execution

Status: Accepted · 2026-09-22 · explicit user decision during Session 06.

The user accepted Windows native C as a formal oracle execution path. The
available machine has GCC, but no Linux container runtime. Session 05 already
exercised the pinned C runtime and TSX grammar through a static C driver.

Use the same runtime/grammar epoch and comparison dimensions on Windows.
Build one static executable per grammar and retain its source, ABI, compiler,
flags and executable identities. The driver transports C API observations;
it does not normalize or repair trees. The existing official Go transport pin
remains recorded for its own lane; a static C driver does not claim to use it.

Only source preparation uses the network. Builds and comparisons consume
hash-checked local inputs. Generated records are immutable, ordered, and bound
to a build manifest. The product lane reads checked-in records with CGO off.
Every receipt identifies the OS; Windows execution supplies no Linux claim.

Generator selection stays dynamic under ADR-0007. New artifacts require new
comparison records. Neither a generator update nor this execution-path change
changes runtime/grammar pins or the intended Release-Critical scope.

Supersedes the Linux-only execution restriction in the oracle contract, not
ADR-0002's epoch decision. Linux execution remains available when its environment
exists and otherwise is explicitly NOT_RUN.
