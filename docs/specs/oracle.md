# Oracle contract

Normative. Defines what "correct syntax" means here (ADR-0020 §3).

## The reference is the same C, built natively

The correctness reference for a generated grammar module is a **native C build of the same inputs**: the same
tree-sitter runtime release and the same grammar `parser.c`/`scanner.c` bytes (`docs/specs/baseline-provenance.md`),
compiled unmodified by this repository's own tooling (`tools/native-ref`: our own dump/edit/changed-ranges driver,
sources verified against `tools/gen/pins.json`). Scanner character classification and case conversion use the
engine's same pinned wasi-libc sources through `wctype.h`/`ctype.h` shims, rather than the host libc's semantics;
the wasi-libc archive is SHA-256-checked against the pin before its sources are compiled. The upstream runtime and
grammar sources stay unmodified. There is no long-lived pinned oracle epoch: the reference moves with the runtime
release and is rebuilt for it.

A reference result is identified by the tuple

```
runtime release + tree hash · grammar commit + source sha256s · wasi-libc commit + archive sha256
· driver/shim sha256 · compiler identity + flags · OS/arch
```

and a generated-module result by its `provenance.json` identity. A comparison is valid only when both sides share the
runtime release, grammar source hashes and pinned wasi-libc character semantics. A native build using glibc or UCRT
character functions can differ for non-ASCII input and is not this reference. Other C builds (for example
grammar-kit's patched runtime) are not a reference.

## Comparison dimensions

The canonical dump is a cursor-preorder listing that the native driver and the Go snapshot renderer both produce.
Oracle agreement requires byte-identical dumps, which compare:

```
symbol name · child order · named · extra · missing · error · has_error
field name per child · byte range · point range (rows, byte columns)
root presence (the tree was returned) · root end == input length
```

The root may start after leading padding (whitespace, BOM); its range must equal the first dumped node's range.

## Lanes

- **Corpus:** fresh parses of the reference corpus and incremental edit cases, per grammar.
- **Large inputs:** the large-input set (also the speed and memory gate inputs).
- Reference dumps are produced by the native lane (C compiler required) and compared by the product lane
  (`CGO_ENABLED=0`, no compiler). Windows results do not imply Linux, macOS or arm64 results; an environment that did
  not run stays `NOT_RUN`.

## Separate invariant

```
incremental(final source) == fresh(final source)
```

This is **self-consistency**, not oracle agreement, and upstream does not guarantee it (5 of 5,919 session-15 edit
cases differ in native C as well). The oracle comparison for an edit case compares the incremental result with the
native incremental result.

## Adopting a new runtime release

1. Build the native reference and the generated modules from the new release with unchanged grammar inputs.
2. Run all comparison lanes. Every difference is attributed to a named upstream change (release notes or commit);
   an unattributed difference blocks adoption.
3. Record old and new results side by side in a handoff; keep the old reference results.
