# tools/native-ref — native C reference

Builds one native executable per grammar from the sources pinned in `tools/gen/pins.json` (the unmodified tree-sitter
runtime `lib/` plus one grammar's `parser.c` and `scanner.c`, including any approved ADR-0023 patches), linked with this repository's own driver
`c/native_ref.c`. The executables produce the reference results the CGO-free conformance lane compares against
(`docs/specs/oracle.md`). CGO is used only by the C compiler here, never by the Go product lanes.

Scanner calls to all twelve `isw*` classifiers, `towupper`, `towlower` and narrow `isdigit`
use the engine's character semantics: `wctype.h`/`ctype.h` shims route them to namespaced functions compiled from
the wasi-libc sources pinned in `tools/gen/pins.json`. The wasi-libc archive is SHA-256-checked before extraction,
as are the runtime and grammar archives; runtime and wasi-libc C stay unmodified. Private headers compile those sources with
32-bit character types and a 32-bit `wcschr` for musl's whitespace table; `-fno-builtin` prevents host-width compiler
builtins from replacing that lookup. This keeps classification and case conversion independent of the host locale,
glibc or UCRT, including full code points on Windows, whose host `wint_t` is narrower.

```
cd tools/native-ref
go run . -pins ../gen/pins.json -dl <archive dir> -work <empty dir> -out <exe dir> [-fetch] [-cc gcc|clang] [lang ...]
```

- `lang` is a key of `pins.json` `grammars` (`go` is accepted for `golang`, `jsx` for `javascript`); none = all pinned grammars.
- Every archive's sha256 is checked against `archiveSHA256` before extraction (`-fetch` downloads a missing one from the
  pinned `archiveURL`, or the GitHub API tarball of the pinned commit when absent), and every grammar source file against
  the pinned `files` hash. Transient network failures get at most three attempts, with 1/2-second backoff and a two-minute
  timeout per attempt; HTTP errors and hash mismatches are not retried. Failed transfers leave no partial archive. Only
  the required source directories are extracted into `-work` (the pinned grammar/runtime `dirs` and wasi-libc's
  `libc-top-half/musl/src/ctype`). ADR-0027 regeneration additionally extracts the full grammar archive
  into a private directory and executes its grammar JS with the pinned CLI's native JS runtime; only recorded
  generator exceptions retain committed C. Grammar-input patches precede generation, and C patches follow it.
- `-fsigned-char` and `-DNDEBUG` match the engine's plain-char signedness and disabled assertions for the driver,
  grammar and runtime. `-static` on Windows (no MinGW DLLs needed); upstream's `_POSIX_C_SOURCE`/`_DEFAULT_SOURCE`
  flags elsewhere. Windows retains 16-bit `wchar_t`, so C++ raw string delimiters and XML names with supplementary
  characters can differ from the engine; CI native references run on Linux ([oracle contract](../../docs/specs/oracle.md)).
- Needs a C11 compiler (gcc or clang) on `PATH`; large grammars (csharp, tsql, typescript, tsx) take a few GB of RAM to
  compile at `-O2`.

`go test ./...` compiles and runs a C probe through the same shim, including non-ASCII and supplementary code
points. It fetches and verifies the pinned wasi-libc archive; set `TSG_NATIVE_DL` to an existing archive directory
to reuse a local copy, and `CC` to select clang instead of the default gcc.

Two flags serve `tools/compare` (the comparative benchmark): `-driver FILE` links another C driver instead of
`c/native_ref.c` (same shims, flags and pinned sources; use a separate `-out`), and `-stage` fetches and sha256-verifies every
input a build needs (runtime, wasi-libc, grammar archives, the pinned CLI, npm inputs) and extracts `-corpus`, but generates and
compiles nothing, so a later offline run needs no network.

Executable CLI (stdout is binary, `\n` line ends on every platform):

| Command | Output |
|---|---|
| `<exe> dump <file>` | `#tsg-dump/1 root_has_error=<0\|1> nodes=<n>` then one tab-separated line per node in cursor preorder: `depth field type named extra missing is_error has_error start_byte end_byte row:col row:col` |
| `<exe> edit <old> <new> <start> <old_end> <new_end>` | the same dump of `<new>` reparsed incrementally from the edited `<old>` tree |
| `<exe> changed-ranges <old> <new> <start> <old_end> <new_end>` | `ts_tree_get_changed_ranges(old, new)`, one `start_byte end_byte start_row start_col end_row end_col` line per range |
| `<exe> query <query-file> <source>` | raw matches of a fresh parse, text predicates not evaluated (the C engine never does): `#tsg-query/1 patterns=<n> captures=<n>`, then per match `m<TAB><pattern><TAB><n>` and per capture `c<TAB><capture><TAB><node preorder index><TAB><symbol><TAB><start byte><TAB><end byte>` (the index comes from a full cursor walk by node id, independent of the engine's descendant index; symbol is `ts_node_symbol`). A rejected query prints `#tsg-query/1 error offset=<o> type=<t>` and exits 0. |
| `<exe> fields <file>` | `ts_node_child_by_field_name` of a fresh parse for every node (cursor preorder) and every field name: `#tsg-fields/1 nodes=<n> fields=<f>`, then `<node index><TAB><field><TAB><result preorder index>` per non-null result (`?<symbol>:<start>:<end>` when the result is not a visible node). The runtime has no `ts_node_children_by_field_name` (v0.27.2). `tools/conformance` `TestFieldLookup` compares it with `Node.ChildByFieldName`. |
| `<exe> edit-fields <old> <new> <start> <old_end> <new_end>` | the same `#tsg-fields/1` lookup output for `<new>` reparsed incrementally from the edited `<old>` tree, using the same tree construction as `edit`. `TestFieldLookupEdits` compares this with the Go incremental tree. |
| `<exe> symbols` | `#tsg-symbols/1` followed by one tab-separated row per visible or supertype raw ID in ascending order. Each row gives the raw ID, its public ID, the result of `ts_language_symbol_for_name`, namedness (0 or 1), and the symbol name in hex. `TestSymbolLookupNative` checks the Go lookup against these rows. |

Points are byte columns with `\n` rows (the same as `treesitter.EditFor`). Exit codes: 0 ok, 2 usage / edit out of
range, 3 unreadable input, 4 parse failure, 5 write failure.

## CI conformance data (`ciconf`)

`.github/workflows/conformance.yml` needs no local data. Inputs are the pinned grammar repositories' own
`test/corpus/*.txt` (under each grammar's license; the archive is sha256-verified before anything is read):

```
native-ref -pins tools/gen/pins.json -fetch -dl <dl> -work <work> -out <exe dir> -corpus <corpus dir>   # build + extract corpora
#   -queries <dir>: also extract each archive's queries/ into <dir>/<extractDir>/queries (sha256-verified; TSG_QUERY_DIR for the query lane)
#   -nobuild: verify and extract only (no compiler); GITHUB_TOKEN, when set, authenticates -fetch against API rate limits
cd tools/native-ref && go run ./ciconf -pins ../gen/pins.json -corpus <corpus dir> -exe <exe dir> -out <data dir> -kit ../conformance/testdata/kit [-seed N] [-j N] [-lang javascript,html,...]
cd tools/conformance && TSG_DATA_DIR=<data dir> TSG_NATIVE_EXE_DIR=<exe dir> go test -run 'TestCorpus|TestEdits|TestParserPoolRealEngine' .
```

In CI the build output (`exe`, `corpus`, `queries`, `scanner-stages`) is cached per group by a push to main and restored by pull requests;
the key holds the runner image, compiler and Go versions, the group's grammars and the content of `pins.json`, these sources, the
patches and the packages this tool imports. `cachesums.sh` records the sha256 of every cached file; a restore that does not match
it is discarded and the build runs as usual.

`-kit` is what CI passes; data made without it has no kit inputs, so `TestCorpusKitInputs` fails for the seven kit grammars
(a dropped flag must not leave a lane green). `go test ./...` in `tools/conformance` skips what lacks a prerequisite:

| Test | Needs | Without it |
|---|---|---|
| `TestCorpus*`, `TestEdits`, `TestQuerySmoke` ... | `TSG_DATA_DIR` (ciconf output) or `TSG_S15_DIR` | skip |
| `TestCorpusKitInputs` | ciconf data made with `-kit` (or no ciconf data) | **fails** (the guard) |
| `TestFieldLookup`, `TestSnapshotPointsNative`, `TestInvalidUTF8VsNative`, native subtests | `TSG_NATIVE_EXE_DIR` (the `-out` directory above); `TestInvalidUTF8VsNative` also accepts `<data>/native/exe2/<lang>.exe` of the session-15 tree | skip |
| `TestQuerySmoke`, `TestQueryNative` | queries: `TSG_QUERY_DIR` (`native-ref -queries`) or `<data>/src` | skip (`TestQueryNative` fails when its executable exists but the queries do not) |

`TSG_REQUIRE_FIELD_LOOKUP=1` (CI) turns the skips of the field, snapshot-point, invalid-UTF-8 and query-smoke tests into failures.
CI runs `TestSnapshotPointsNative` and `TestInvalidUTF8VsNative` in the Go conformance step of every group and `TestQuerySmoke` in its query step (`.github/workflows/conformance.yml`).

`-lang` limits corpus preparation to the comma-separated conformance language names (default: all 25). JSX inputs use the JavaScript corpus and parser. PHP selects the PHP parser; XML selects XML, not DTD.
Nested corpus directories follow each pin's `grammarDir`; XML upstream queries are under `queries/xml`.

`-inputs-only` writes only corpus inputs, manifests and `inputs.json`; it needs no `-exe` and executes no parser.
Nightly uses it so all parsing happens under its per-input watchdog. It writes no reference dumps, edits or `ci.json`
and cannot substitute for conformance preparation.

`ciconf` is its own `main` (stdlib only) because `native-ref` itself only fetches and compiles; it parses the corpus
format (`===` header with name and `:attribute` lines, input, `---`, expected tree), writes each input once as
`corpus/<lang>/<file stem>-<nnn>.in`, runs the native executables, and writes the data directory read by
`tools/conformance` (same layout as the local session-15 tree, plus `ci.json`):

- Every test is an input except `:skip` entries and `:language(x)` entries for another language (typescript and tsx
  share one corpus); both groups go to `corpus/skipped.csv` with the reason. `:error` entries are kept (error recovery
  is exactly what the comparison should cover); the attributes are recorded in the manifest's `attrs` column. The
  expected trees are not used: the reference is the native C dump of the same bytes.
- Three seeded edits per input (`insert`, `delete`, `replace`; `insert` only for an empty input), positions snapped to
  UTF-8 boundaries, inserted text ASCII or a slice of the input. The seed is `-seed` mixed with the input id, so the
  same pins and seed give the same data on every host; the per-edit seed is in `edits/manifest.csv`.
- Native results: `native/dumps/<lang>/<name>.dump` (fresh dump), `edits/results-native.csv` (sha256 of the native
  incremental dump and of the native fresh dump of the new text), `edits/<lang>/<name>.ranges` (native
  `changed-ranges`), and `edits/<lang>/<name>.fields` (native incremental field lookups). `edits/results-native.csv`
  also records `edit_fields_exit` and `edit_fields_sha256` for each edit. A native run that fails is listed in `native-failures.csv` and `ciconf` exits 1.

## Timing modes (`tools/bench/ci`)

| Command | Output |
|---|---|
| `<exe> speed <file> <reps>` | `reps+1` lines, each the milliseconds of one `ts_parser_parse_string` + `ts_tree_delete` on a reused parser (the first is the warm-up). |
| `<exe> corpus <list> <reps>` | `speed` for every path in `<list>`, one line each: `name,bytes,clean\|errors,ms0;ms1;...`. |

Timer: `QueryPerformanceCounter` on Windows, `mach_absolute_time` on macOS, `CLOCK_MONOTONIC` elsewhere. Used by
`.github/workflows/bench.yml` through `tools/bench/ci` (`benchci`).

## Upstream watch (`upstreamwatch`)

`cd tools/native-ref && go run ./upstreamwatch -pins ../gen/pins.json` compares the pins with the newest stable
runtime/tool releases (exit 1 when one is newer). For every grammar, runtime, wasi-libc and tool pin it reports:

- API redirects / changed `full_name` (rename or transfer), archive status, and missing/deleted repositories;
- which default branch, published release tag, or other branch/tag contains the pin (`identical` or `ahead` from
  pin to ref); only a pin unreachable from every branch and tag fails, including an orphan still downloadable as an archive;
- last default-branch commit older than 12 calendar months (tier-review warning), and latest stable release vs pin.

Rename, archive and age are warnings. Exit 2 means an unreachable pin, missing repository or API failure; GitHub's
404 cannot distinguish deletion from an inaccessible repository. Requests are cached by path (shared grammar repos
are fetched once); comparisons request one commit per page. The default head and latest published release are tried
first; fallback enumerates all branches and public repository tags in pages of 100, stopping at the first containing ref.
Public tags count as published release refs even without a GitHub Release entry. API/listing errors or quota exhaustion
fail as incomplete checks, rather than being reported as an orphan. `GITHUB_TOKEN` is recommended: the complete inventory
can exceed the anonymous 60-request quota. The report records request count and remaining quota, and stops requests
when it reaches zero. It reads `pins.json`, the checked-in acceptance list and public API answers; it never changes pins.
Run by `.github/workflows/upstream-watch.yml` with its read-only token.

Two sections follow the health table and the nvim comparison. **Patch-carrying grammars** (ADR-0023) lists
each `pins.json` patch with its `upstreamReport` and the date that report first reached the first-parent history of HEAD
(main in the workflow; `git log -S` on `pins.json`, so renaming or rewriting the patch file does not restart it). Over
12 months is a warning per report (kind `patch older than 12 months: <report URL>`, so accepting one report does not hide
another); more than 3 patches (`-patch-alert`) or 5 (the ADR limit) is flagged in the table only. Unreadable `patches`
metadata or unusable git history (no repository, shallow checkout; the workflow uses `fetch-depth: 0`) is a diagnostic in
that section, never a warning or a failure. Bundled reports listed only in READMEs are not tracked (a pins schema
change, an owner decision). **Grammar repository activity** (one GraphQL query per grammar repository, so it needs `GITHUB_TOKEN`
and is skipped without it) comes last: merged pull requests in the last 12 months and how many of the 20 newest issues
have a comment by an OWNER, MEMBER or COLLABORATOR among their first 20 comments (`>=` when more comments were not
read; `?` for data the response lacked). It never warns, runs inside a 2-minute budget and stops at the first GraphQL
rate limit or `Retry-After`, marking the remaining rows `unavailable`, so it cannot keep the issue body or state file
from being written. Errors print as a fixed kind (and HTTP status), never as remote text. The workflow updates the
tracking issue only for runs on the default branch.

Owner-accepted warnings live in [upstreamwatch/accepted-warnings.json](upstreamwatch/accepted-warnings.json),
embedded at build time. Each entry matches a component and exact warning kind (a newer release kind includes its
version), with an accepted date, exclusive UTC expiry date and a linked reason. Active matches appear in **Accepted
warnings** with their expiry; only unaccepted warnings count at the top or prevent automatic issue closure. On the
expiry date they become new warnings again. Entries without any current match are shown as **Stale acceptances**
for review, even when expired; they do not count as current warnings. Tool/runtime release failures remain failures.

`-issue FILE` writes the complete issue body even for zero unaccepted warnings, preserving accepted/stale entries
when the workflow closes the issue. `-state FILE` writes `warn`, `clean` or `error`; hard errors remove the body and
leave the issue unchanged. The issue job uses that state rather than inferring warnings from the body's existence.

## Recorded grammar patches

Both builders support `regenerate: true` without patches (for example PostgreSQL LFS source) and use the ADR-0023 patch records in `pins.json`; grammar-input patches with `regenerate: true` use
`tools/gen/grammarpatch` to compare two independent generated source trees with the pinned CLI. `-fetch` also
retrieves the required CLI gzip upstream-first, with the release mirror as fallback and the original SHA-256
still mandatory. A generation record is kept at `<work>/patches-<lang>/provenance.json`.
`-unpatched` omits recorded patches for the before/after comparison; a `regenerate: true` pin still generates
its original grammar inputs, since its archived C may be absent or an LFS pointer. It never changes the pin set.
`-patch-alert N` sets the patch-count warning threshold (default 3). See `tools/gen/README.md` for the record schema.

## Pinned generator behaviour gate (ADR-0027)

Build `native-ref`, `ciconf`, and `tools/bench/ci` from this checkout, then run (Python 3.11+):

```
python tools/native-ref/generatorgate.py --lang json --native-ref <native-ref> --ciconf <ciconf> \
  --benchci <benchci> --cli <pinned-tree-sitter> --dl <verified-archive-cache> --out <new-result-directory>
```

The current pin set contains 25 grammar modules. A grammar with `reference: "none"` (Swift) has no committed
upstream parser C: `native-ref -reference swift` returns `NOT_APPLICABLE_NO_COMMITTED_PARSER` as an error.
The gate records that status and omits only the committed-C variant. It still requires candidate determinism,
strict generated-schema validation, product-vs-fresh-native corpus/seeded-edit/field and pinned large-input
comparisons, plus the upstream repository's own tests. Swift's `generateFrom: "grammar.json"` selects the same
unmodified upstream `src/grammar.json` as the product generator, with the input hash in provenance.
Its owner-approved `upstreamTestException` covers only CLI 0.27.2 passing the Swift input as the outline query path:
all 313 corpus cases and eight highlight cases must pass, the full-suite exit 1 and exact query error remain in the
report/log, and `tree-sitter query queries/outline.scm test/outline/outline.swift` must succeed. Any new failure or
a disappearing signature fails. On a CLI pin move, recheck the full suite and retire the exception when fixed.
This exception does not waive parser correctness or constitute release approval. Any other failed check fails the gate.

The tool builds committed-C (`native-ref -reference`) and regenerated-C (`-candidate`) variants independently,
compares native corpus dumps, seeded incremental/fresh trees, changed ranges and field lookups, and compares
the SHA-256-pinned large inputs in `tools/bench/ci/large.json`. It runs the pinned grammar repository's own tests once
per variant, with separate `TREE_SITTER_LIBDIR` directories; existing failures must keep their output signature.
If a corpus failure prevents the CLI from reaching auxiliary tests, a declared supplemental invocation excludes
exactly those failing corpus names and collects the remaining stages; the original failures remain in the comparison.
Grammar repositories with no test cases record `NOT_RUN_ZERO_CASES`. A committed LFS pointer is a reference only
when regeneration reproduces its recorded SHA-256 OID. When that OID equals the candidate parser, there are no
patches, and all other grammar C/header inputs are byte-identical, the reference shares the candidate's native
build, data and grammar test results. The report explicitly records `LFS_OID_IDENTICAL_TO_CANDIDATE` and the OID;
this avoids regenerating and compiling PostgreSQL a second time rather than claiming an independent comparison.
The PostgreSQL shard budgets 30 minutes locally (generation plus one native build and tests), with a 90-minute
hosted limit allowing a 2x slowdown plus 30 minutes of margin. This is an estimate, not a hosted measurement.
Initial generator exceptions still exercise the candidate
and must match exactly the `generatorException.failingInputs` IDs; a disappearing or expanding exception fails.
The new output directory retains source/build identities, commands, input bytes, native results and grammar test
summaries. CI uploads these results and GNU `time -v` wall-time/max-RSS records; hosted measurements describe
the runner that ran them, not a local estimate. No pin is changed by the gate.
Text identities for pins, the gate script and the large-input manifest hash CRLF-normalized-to-LF bytes; binary
tools, archives, parser sources and input/output dumps retain exact byte hashes. Each subprocess is supervised,
including child-tree cleanup on exceptions, with bounded cleanup waits. `--large-dump-timeout SECONDS` controls
each pinned large native dump (default 120 seconds); a timeout remains a retained failure.

## Unpatched comparison (ADR-0023)

For a patched grammar, build separate executables into separate work/output directories, once normally and once
with `-unpatched`. For YAML, compare the corpus and the retained fuzz seed literals with:

```sh
cd tools/native-ref
go run ./patchcompare -before <unpatched-yaml-exe> -after <patched-yaml-exe> \
  -corpus <data>/corpus/yaml -fuzz-source ../conformance/fuzz_test.go -fuzz-target FuzzParseYAML \
  -allow-nul-truncation -out <new-comparison-dir>
```

The report lists changed inputs and binds input/executable hashes. Error-free unpatched trees must remain identical;
YAML's documented NUL-truncation exception must become a full-length error tree. Mismatches retain both dumps and
fail the command. The `-allow-nul-truncation` exception is only for this recorded YAML fix. YAML patch 0003 changes
no corpus or fuzz-seed input, so this comparison passes; its documented fix (error-free explicit-key trees past row
32,767 becoming correct) appears only in the retained inputs of `tools/conformance/scanner_patches_test.go`, which this
command reports as `pass=false` when given them as a corpus. The PR description records those rows.

`go test -run '^TestQueryIndexInvalidation$' .` also exercises the retained `query_index` invalidation probe
against the current shim and pinned JSON/runtime C. It asserts cache invalidation directly and checks a negative
mutation without the invalidation assignments; grammar regeneration is unnecessary for this test.

The CI and nightly lanes also pass `-kit ../conformance/testdata/kit` (relative
to this module; from the repository root use `tools/conformance/testdata/kit`).
This adds the bounded, hash-checked source/edit projection documented in
[`workloads.md`](../../docs/validation/workloads.md#bounded-grammar-kit-inputs).
Kit inputs get their explicit pair edits, not three additional seeded edits.
`-inputs-only` includes their fresh step sources without running native parsers.

ADR-0027 regenerates every grammar except the recorded `generatorException` entries in `pins.json`.
`grammar.js` patches apply before generation; recorded C patches apply afterwards, using generated
before/after hashes where necessary. Selected `npmDependencies` are unpacked directly from the upstream
`package-lock.json` tarball after SHA-512 integrity verification; install scripts never run.
QuickJS receives these packages in the grammar working directory's `node_modules`. TypeScript and TSX use
`grammar.js` vendored in their shared owner archive. No host Node installation participates. Lockfile, tarball
and dependency identities are recorded in provenance when npm inputs are used.


Grammar-input patches are omitted only for the gate's `-reference` build. Its C/header patches remain common
with the candidate. LFS reference regeneration must equal the committed OID before those C/header fixes apply.
Intentional grammar-input behaviour changes use a reviewed `tools/gen/patches/<lang>/generator-expectations.json`
with exact input identities and both result hashes; new, changed or disappeared differences fail. The report retains
all differences and the sidecar hash even on success. The record format and canonical hash rules are owned by
[ADR-0027](../../docs/design/decisions/ADR-0027-one-pinned-generator-cli.md#grammar-input-patch-comparisons-adr-0023).
