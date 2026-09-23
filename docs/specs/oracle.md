# Oracle contract

Normative. Defines what "correct syntax" means here, and what it costs to change that definition.

## The oracle is a tuple, not a repository

A syntax oracle result is only meaningful when every component below is fixed. Changing any one of
them produces a different oracle and invalidates prior comparisons.

```
C runtime      tree-sitter v0.25.1
               f5afe475deb7c0bae6407fb776c76824f717bb61

transport      official Go lane: github.com/tree-sitter/go-tree-sitter v0.25.0
               adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3   (statically links the runtime)
               native C lane: tools/native-oracle/driver.c, identified by SHA-256

grammars       the upstream grammar commits pinned by gotreesitter v0.53.0
               see docs/specs/baseline-provenance.md

generator      selected at execution time; record the actual producer per artifact
               ABI must fall within the pinned runtime's advertised supported range

build          -std=c11 -fPIC -O2, plus the compiler identity and the artifact SHA-256
```

The runtime and grammar epoch is inherited from upstream `gotreesitter`, which encodes it in
`cgo_harness/parity_c_loader_cgo.go`. The constants there were verified identical at `v0.52.0`,
`v0.53.0`, and upstream `main`. The generator policy was revised by the user's
Session 05 instruction; see ADR-0007.

## Generator selection and artifact identity

Do not pin a generator release in configuration. Resolve `TREE_SITTER_CLI` or
`tree-sitter` from PATH on each generation, record its reported version, and
generate for an ABI supported by the pinned runtime. An updated installed tool
is used on the next invocation without editing a version constant. This policy
does not install tools globally or silently update the C runtime or grammars.

Checked-in generated C sources may be used directly. Record the grammar commit,
parser/scanner hashes, ABI, and `generator_mode=checked-in`; the historic generator
version may be unknown and must never be invented. The immutable C source bytes
identify that input. A dependency version range is not a producer version.

Every build records the actual compiler version, flags and artifact hash. A
different generator, generated source, ABI or compiler yields a new artifact
identity: regenerate the comparison records instead of reusing old results.
Generator updates alone do not migrate the runtime/grammar epoch. They require
fresh comparisons before a new artifact can support a claim.

The active C artifacts use the pinned base sources with the explicitly approved
TypeScript/TSX patch identified by `oracle.typescript_patch` in identities.json.
ADR-0012 records the owner's Candidate C adoption. Its complete import-type,
variance, adjacent-call-signature and contextual-`in` changes are inseparable.
`testdata/oracle/typescript-patched/manifest.json` binds the generated grammar
JSON and scanner inputs to that patch and its producer. The build validates
this identity and generates C with the currently selected CLI. Other grammars
retain their original sources. The C runtime and grammar base commits did not move.

Active records are under `testdata/oracle/windows-c-v2/`. The older unpatched
record sets and KR-0003 signatures remain historical evidence. They identify
different grammar semantics and cannot be relabeled as current comparisons.

The retained `testdata/oracle/runtime-abi.json` binds the pinned runtime header
hash and supported ABI range to its commit. The comparison tool checks each
build against this independent anchor, each receipt against its build, and its
input byte length against the catalog source. Mutually consistent stale
metadata does not authorize a new epoch or ABI range.

The root span must be ordered and end at input EOF. Its start need not be zero:
the C grammar may exclude leading hidden text, including in C# recovery roots
and fixed-format COBOL. The reported root range must exactly match the first
canonical node's range. Preserve these coordinates rather than widening them.

## Why not current `tree-sitter/master`

At the time of selection, upstream `master` was **958 commits** ahead of `v0.25.1`, of which
**56 touched the semantic core** — including error-recovery child nesting, error-cost accumulation
through hidden nodes, state on missing nodes, and a large number of query anchor/quantifier changes.
Moving the oracle there would not make results "more correct"; it would make every existing parity
board **incomparable**. Newness is not a reason.

## Comparison dimensions

A tree comparison that claims oracle agreement must compare all of:

```
node type / symbol · child order · named · extra · missing · error
field name per child · byte range · point range · alias
root completeness · parse stop reason · truncation flag
```

The retained comparator enforces every represented `syntax.Node` field and the
completion/error receipt:

```
node type · parent/child order · named · extra · missing · error
field name · byte range · point range · completeness · result outcome
```

Aliases are compared through the public node type and field projection; numeric
symbol IDs and separate grammar alias metadata are not exposed by this API.
The Go grammar's anonymous literal-NUL terminal has an empty C name; only its
exact byte/point form is admitted (ADR-0015). Query, supertype, tags, and
highlight dimensions are **adopted but not enforced**; they become
enforced when a consumer actually depends on them. Adopting a requirement and satisfying it are
different states — say which one you mean.

## Separate invariant

```
incremental(final source) == fresh(final source)
```

This is a **self-consistency** property. It is not oracle agreement. A run may satisfy it while both
sides disagree with the C runtime. Never report it as parity.

## Execution reality

The upstream dynamic-loader harness is POSIX-only. ADR-0008 adds a native C
driver with static grammar linking on Windows, explicitly authorized by the
user in Session 06. Both routes retain the same runtime/grammar epoch and
comparison requirements; every artifact states its actual transport and OS.

- Oracle execution produces deterministic ordered tree records checked into
  this repository. Compiler identity and binary hash belong to the build receipt.
- The Windows product lane compares against those records with CGO disabled and
  needs no C toolchain. Only the separate oracle producer invokes a C compiler.
- Windows execution does not imply Linux execution. An unavailable environment
  stays NOT_RUN for that environment; no result is transferred between them.

## Changing the epoch

An oracle upgrade requires **all** of:

1. Every current ratchet is green at the old epoch. Never migrate with an open unexplained failure —
   attribution becomes impossible.
2. Upstream `gotreesitter` has itself moved its oracle constants. We do not lead upstream here.
3. The semantic delta between epochs is enumerated from the runtime's own history, not summarized.
4. Both epochs run side by side over the same corpus, and **every** digest difference is attributed
   to a named upstream commit. One unattributed difference blocks the migration.
5. An ADR, a handoff recording old and new digests, and retention of the old digest set.

Until all five hold, the epoch does not move.
