# ADR-0003 — Windows Tier-1, CGO-free production

**Status:** Accepted · 2026-09-22

## Context

The product target is Windows native. The upstream runtime is pure Go with no external runtime
dependencies, and confines all CGO to a **separate module** — so a CGO-free product build is
structurally guaranteed rather than merely intended.

Verified at the exact baseline: full-module `CGO_ENABLED=0 go build ./...` on Windows/amd64,
all seven target languages parsing cleanly, and `windows/arm64` cross-build linking.

Also verified: upstream's own CI is **entirely Linux** (31 jobs, no `GOOS` matrix). Windows is a gap
we own, not one upstream covers. And the upstream C-oracle harness is POSIX-only (`dlopen`).

## Decision

1. Windows native is **Tier-1**. It is the only platform that decides a product gate.
2. Production and release paths build with `CGO_ENABLED=0`. Nothing in the product import graph may
   require a C toolchain.
3. CGO exists only in diagnostic and oracle lanes. A diagnostic lane **never** decides a product
   gate. `CGO_ENABLED=0` with `-race` is not a valid configuration and must never be planned.
4. Other platforms are **link-checked**, not validated, for the first milestone. `linux/amd64`,
   `darwin/arm64`, and `wasip1/wasm` were confirmed to link at an earlier commit but were
   deliberately not re-verified at the exact baseline; they are out of first-release scope.
5. A race-lane timeout is a diagnostic finding. Measured overhead on the same input was ~14×.

## Consequences

- We must own a Windows CI lane; upstream will not provide one. Contributing one upstream is a
  named opportunity, not a dependency.
- Oracle evidence is generated elsewhere and consumed on Windows as checked-in digests.
- Claims about non-Windows platforms require their own evidence at their own level; the existing
  cross-build result may not be reused for the current baseline.
- `core.autocrlf` is `true` on the development machine. Windows Tier-1 therefore **requires**
  `.gitattributes` before any fixture lands; otherwise fixture bytes and every span derived from
  them differ between a checkout and a module cache. This is not a style preference — the same
  upstream fixture measures 12,408 bytes (LF blob) and 12,785 bytes (CRLF working tree).

## Supersedes

None.
