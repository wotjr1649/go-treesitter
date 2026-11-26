# ADR-0031 — Hardware guard pages instead of per-access bounds checks in generated modules

**Status:** Proposed (prototype only; no decision, owner decision 2026-11-26: P3 is a prototype and this draft) · 2026-11-26 · rebased onto the single-copy memory path and wasm2go v0.4.22 by S125 (branch `perf/guard-pages-g1`, local)

## Context

Every wasm load and store in a generated module goes through a Go helper that checks the address in software
(`_ = mem[addr+n]`) and reloads `m.memory` (pointer and length) from the module struct. ADR-0020 §8 kept those
checks on purpose (wasm2go `-unsafe` keeps bounds checks), so a wild access becomes a Go panic, which `engine.Call`
reports as `*Trap` and the parser maps to `EngineFault`.

The S82 analysis found that 99% of parse time is inside translated wasm code and that the checks and the repeated
`m.memory` loads are 18–21% of profile samples. Removing them (throwaway edit) gave −25% to −40% parse time with
identical snapshot hashes. That edit is memory unsafe. This ADR proposes to keep the guarantee "an out-of-range access
ends in `EngineFault`, never in memory corruption" by moving it from a software check to OS memory protection.

The prototype is branch `perf/guard-pages-g1` (local, unpushed, stacked on `g1/dry-run`): `internal/engine`
(`InitGuardedMemory`, which `StartMemory` turns into the 4 GiB + guard reservation, and a scoped `SetPanicOnFault` in
`Call`), `tools/gen/guardpages.go` (post-processing of the wasm2go v0.4.22 output, recipe `v3-guard-O3`) and tests. The
first version (S87, `perf/guard-pages`) targeted wasm2go v0.4.21 and the old lazy memory path; S125 ported it. Measured
evidence is in `.scratch/s23/S87-guard-pages-proto/REPORT.md` (v0.4.21 baseline) and
`.scratch/s23/S125-guard-pages-g1/REPORT.md` (against the G1 modules: v0.4.22 plus single copy). Both are Windows amd64 only.

wasm2go v0.4.22 already caches the memory slice in a local (`mem := m.memory`) and the single-copy change already removes the
heap copy of the initial memory, so part of what S87 measured is now in the baseline: the S125 gain is **16–28% of parse
time** (0.72–0.84 guard/base on large inputs, 0.70–0.78 on small ones), and the NewParser and peak-memory gains of S87 are
gone (they were the single-copy gain).

## Decision (proposed)

### 1. Safety model

| | Today | Proposed |
|---|---|---|
| Linear memory | Reserved by `StartMemory` at the start of `X_initialize` (size = limit), committed as it grows | Reserved by `StartMemory` (after `SetLimit`): **4 GiB + `GuardSize` (64 MiB)**, independent of the limit; only the committed prefix is readable and writable |
| Wasm load/store | Software check against `len(mem)` (`_ = mem[addr+n]`), Go panic when out of range | No check: the helpers dereference `SliceData(mem)+addr`; the base never moves after `StartMemory`, so the module's local `mem` copies stay valid |
| Out of range | `panic` (index out of range) | Hardware fault (`PROT_NONE` / `MEM_RESERVE`), delivered as a Go panic by `debug.SetPanicOnFault(true)` |
| Reporting | `engine.Call` recovers → `*Trap` → `EngineFault` + instance replaced | The same path. `Trap.Value` is a `runtime.Error` with an `Addr()` method |
| Limit semantics | `SetLimit` caps the reservation and the commit | `SetLimit` caps the commit (same refusals, same `Refused` flag, same `ResourceLimit` mapping); legal until the first grow |

The argument for safety is a set of invariants, each with an owner and a check:

1. **Coverage.** A wasm address is a `uint32` base plus a static offset below 2^32 and an access of at most 8 bytes. The
   generator parses every access, fails on any address form it does not recognise, takes the largest static offset and
   emits `const _ = uint(engine.GuardSize - (maxOffset + 8))`, so a generated module does not compile against an engine
   whose guard is too small. Prototype maxima: json 78,140, typescript 2,078,652, ruby 2,122,908, tsql 13,196,876 (of 67,108,864).
2. **Only the committed prefix is accessible.** `StartMemory` commits the initial pages, `MemoryGrow` commits the tail and
   never decommits; the slice handed to host code has `cap == len`, so host-side slicing cannot reach uncommitted bytes.
3. **Bulk operations stay checked.** `memory.copy/fill/init`, table operations, `call_indirect` and all host-side access
   (the root package copies source and reads snapshots through the slice) keep their software checks.
4. **Every call into a module runs inside `engine.Call`.** `Call` sets panic-on-fault for the calling goroutine and
   restores the previous value (nested calls keep theirs; other goroutines are unaffected). The root package already
   routes every `Xtsg_*`, `X_initialize` and `New` through `ops.call`. A fault outside `Call` terminates the process
   (`fatal error: unexpected fault address`), so this becomes a rule for any new call site.
5. **No call after `Release`.** The hoisted base outlives `Release`. A late access either faults (unmapped) or, if the
   address range was reused, silently reads another allocation. Today `Release` sets `*mem = nil`, so a late access panics.
   The parser discards the instance after every trap and after `Close`; that must be tested as an invariant, not assumed.
6. **A trapped instance is never reused.** Unchanged from today (`InstanceReset`), and more important now because the
   instance may have stopped in the middle of a C data-structure update.

What the model does not claim: it does not detect an access that is in range but wrong (same as a bounds check), it adds
no protection inside the committed prefix, and the fault location is a Go address, not a wasm location.

### 2. Address-space needs

Each live instance reserves 4 GiB + 64 MiB of virtual address space (the old reservation was the limit, default 1 GiB,
at most 4 GiB). No commit charge, no page tables and no RAM until pages are committed. Consequences:

- 64-bit only (already true: `MaxLimit` is 4 GiB). 32-bit hosts fail in `StartMemory` (`ErrReserve`, inside `X_initialize`) and
  architectures without unaligned access fail at `New` with a panic from the generated code; both surface as a trap
  from `ops.call`. Guard modules still compile for 32-bit targets (checked: linux/386, linux/arm), the cross lane stays link-only.
- 1,000 live parsers reserve about 4 TiB. User address space is 128 TiB on Windows and Linux x86-64 (47-bit), so the
  practical ceiling is about 30,000 live instances, less where an address-space cap exists (`ulimit -v`, a Windows job
  with a process virtual-size limit, some container runtimes, and kernels that configure a 39-bit user address space, such as many Android and some arm64 Linux builds).
- Linux: one reservation is two VMAs once memory is committed; `vm.max_map_count` (65,530 by default) bounds instances to
  about 30,000 as well. Strict overcommit (`vm.overcommit_memory=2`) does not account `PROT_NONE` mappings; the committed
  prefix is accounted exactly as today.
- `ParserPool` sizes and `WithMaxMemoryBytes` keep their meaning, but a program that creates many parsers must now budget
  address space as well as memory.

### 3. Fallback when the reservation fails

There is no silent fallback. Code generated for guard pages has no bounds checks and cannot run on heap memory.
`StartMemory` panics with `engine.ErrReserve` and sets the refused flag; `X_initialize` runs inside `ops.call`, so
`NewParser` returns `ResourceLimit` today, with the text "initialize: memory.grow refused" (the message should name address
space, because the caller's remedy is fewer or smaller instances, not a bug report; not implemented). Considered and rejected:

- *Smaller reservation (limit + guard).* Addresses between the limit and 4 GiB would be unmapped by construction but may
  belong to another mapping, which turns a trap into silent corruption.
- *Heap memory without the check.* Unsafe by definition.
- *Second, checked variant of every module* selected by a build tag. Possible (the helpers are the only difference) but
  doubles the verification matrix and the module size if both are shipped; worth it only if a platform without guard
  support (see 4) must stay supported. Not part of this proposal.

### 4. Platform support

| Platform | Mechanism | State of the prototype |
|---|---|---|
| Windows amd64 | `VirtualAlloc(MEM_RESERVE)`, commit by `MEM_COMMIT`; Go raises the access violation as a panic when panic-on-fault is set (`signal_windows.go`) | Executed: engine tests, 4 grammar modules, parser-level fault tests, conformance tests that need no native C |
| Windows arm64 | same API | Not executed (compiled by cross-build only) |
| Linux amd64/arm64 | `mmap(PROT_NONE, MAP_NORESERVE)`, `mprotect` to commit; `SIGSEGV` → `sigpanic` → panic (`signal_unix.go`, `SEGV_ACCERR`) | **Not executed** (no WSL distribution on the authoring host); cross-compiled only |
| macOS arm64/amd64 | `mmap(PROT_NONE)`, no `MAP_NORESERVE`; the fault may arrive as `SIGBUS` or `SIGSEGV`, both honoured by `sigpanic` | **Unverified** |
| Others (FreeBSD, wasm, 32-bit) | no provider | unsupported, as today |

### 5. Compatibility

Additive engine API (`InitGuardedMemory`, `GuardSize`, `ErrReserve`; `StartMemory` refuses a second memory for an instance);
`engine.ABIVersion` stays 4 because the module/engine interface is unchanged. A guard module calls
`engine.InitGuardedMemory`, so its `go.mod` must require a root version that has it (the generator already records the
minimum root version at release; single-copy modules already need `InitMemory`). Existing released modules keep working on a newer
root: they use the legacy lazy reservation path, which the prototype keeps and tests (all non-guard modules ran
unchanged against the new engine). The post-processing pins the sha256 of wasm2go's load/store helper text. S125 re-derived the patch for v0.4.22
(call sites changed from `load32(m.memory, a)` to a local `mem`; the helper text and its sha256 did not change) and checked
all 25 committed v0.4.22 outputs: the helper block is identical and every access has a recognised address form. The recipe
hash does not cover the single-copy patch (`patchNewMemory`); the guard patch is bound by its own recipe lines.

## Evidence required before acceptance

Under `docs/specs/validation.md` levels, with results bound to commit, pins, inputs and platform:

1. Executed Linux amd64 and arm64 and macOS arm64 runs of `internal/engine` tests, the out-of-range grammar test and the
   parser-level fault tests (a CI lane; the authoring host cannot run them). A failure on a platform means that platform
   stays on checked modules or is dropped (decision 4).
2. All 25 grammars: committed snapshot hashes equal the baseline's on the standard corpus plus the large inputs
   (prototype: 4 grammars, equal), and the conformance and fuzz seeds pass.
3. Paired benchmark on the standard runners (not a loaded workstation), Parse and ParseSnapshot, large and small, with the
   run bound to commits. Prototype (Windows amd64, loaded machine, interleaved pairs): guard/base = 0.65–0.69 large parse,
   0.64–0.73 large snapshot, 0.64–0.76 small.
4. Fault soak: at least 100,000 create / fault / release cycles and the same with parallel parsers, no growth in address
   space (Windows `VirtualQuery`, Linux `/proc/self/maps`) or handles; `-race` and `-d=checkptr` runs on Linux.
5. Many-instance test: the largest pool the supported range promises (for example 1,000 live parsers) on each platform and in
   a container with an address-space limit; the failure mode there is the documented `ResourceLimit`.
6. A test that every public entry on a closed or reset parser returns an error without calling the old module (invariant 5).
7. Fuzzing with the new trap path enabled long enough to cover the existing fuzz targets (`tools/conformance/fuzz_test.go`).
8. Owner review of the security statement (SECURITY.md and `docs/design/engine-abi.md` §4) and of the changed support
   promise (address space per instance, platforms).
9. A regeneration PR for all 25 modules with the recipe change as its only generated change, on top of the wasm2go v0.4.22
   and single-copy PRs (S125 derived 4 modules by post-processing the committed output, without running the toolchain; `regen.yml`
   on the branch must reproduce those bytes).

## Consequences

- Parse and snapshot time drops by about 16–28% against the G1 modules (S125; 25–36% against v0.4.21 in S87) on the measured grammars; the remaining gap to C is smaller.
- The safety argument is now a platform property plus six invariants instead of a local check; reviewers must keep
  invariants 4 and 5 in mind for every new call site.
- Faults carry a Go address only; debugging a wild access needs the wasm stack from the `Trap`'s Go stack, not the check's message.
- Address space becomes a resource that users can exhaust; documentation must say so next to the memory limit.
- The single-copy change (`InitMemory`/`StartMemory`) is now the base of this decision, so it gives no NewParser or peak-memory gain of its
  own (S125: NewParser +0–15%, peak commit unchanged), and creating a parser costs a little more (a 4 GiB reservation; 4,000 live
  json parsers took 4.0 s instead of 0.87 s on Windows).
