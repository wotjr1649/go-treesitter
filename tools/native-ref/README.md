# tools/native-ref — native C reference

Builds one native executable per grammar from the sources pinned in `tools/gen/pins.json` (the unmodified tree-sitter
runtime `lib/` plus one grammar's `parser.c` and `scanner.c`, including any approved ADR-0023 patches), linked with this repository's own driver
`c/native_ref.c`. The executables produce the reference results the CGO-free conformance lane compares against
(`docs/specs/oracle.md`). CGO is used only by the C compiler here, never by the Go product lanes.

Scanner calls to `iswalnum`, `iswalpha`, `iswdigit`, `iswspace`, `towupper`, `towlower` and narrow `isdigit`
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
  `libc-top-half/musl/src/ctype`). Approved grammar-input patches additionally extract the full grammar archive
  into a private directory and execute its grammar JS with the pinned CLI's native JS runtime (ADR-0023).
- `-static` on Windows (no MinGW DLLs needed); upstream's `_POSIX_C_SOURCE`/`_DEFAULT_SOURCE` flags elsewhere.
- Needs a C11 compiler (gcc or clang) on `PATH`; large grammars (csharp, tsql, typescript, tsx) take a few GB of RAM to
  compile at `-O2`.

`go test ./...` compiles and runs a C probe through the same shim, including non-ASCII and supplementary code
points. It fetches and verifies the pinned wasi-libc archive; set `TSG_NATIVE_DL` to an existing archive directory
to reuse a local copy, and `CC` to select clang instead of the default gcc.

Executable CLI (stdout is binary, `\n` line ends on every platform):

| Command | Output |
|---|---|
| `<exe> dump <file>` | `#tsg-dump/1 root_has_error=<0\|1> nodes=<n>` then one tab-separated line per node in cursor preorder: `depth field type named extra missing is_error has_error start_byte end_byte row:col row:col` |
| `<exe> edit <old> <new> <start> <old_end> <new_end>` | the same dump of `<new>` reparsed incrementally from the edited `<old>` tree |
| `<exe> changed-ranges <old> <new> <start> <old_end> <new_end>` | `ts_tree_get_changed_ranges(old, new)`, one `start_byte end_byte start_row start_col end_row end_col` line per range |
| `<exe> query <query-file> <source>` | raw matches of a fresh parse, text predicates not evaluated (the C engine never does): `#tsg-query/1 patterns=<n> captures=<n>`, then per match `m<TAB><pattern><TAB><n>` and per capture `c<TAB><capture><TAB><node preorder index><TAB><symbol><TAB><start byte><TAB><end byte>` (the index comes from a full cursor walk by node id, independent of the engine's descendant index; symbol is `ts_node_symbol`). A rejected query prints `#tsg-query/1 error offset=<o> type=<t>` and exits 0. |
| `<exe> fields <file>` | `ts_node_child_by_field_name` of a fresh parse for every node (cursor preorder) and every field name: `#tsg-fields/1 nodes=<n> fields=<f>`, then `<node index><TAB><field><TAB><result preorder index>` per non-null result (`?<symbol>:<start>:<end>` when the result is not a visible node). The runtime has no `ts_node_children_by_field_name` (v0.27.2). `tools/conformance` `TestFieldLookup` compares it with `Node.ChildByFieldName`. |

Points are byte columns with `\n` rows (the same as `treesitter.EditFor`). Exit codes: 0 ok, 2 usage / edit out of
range, 3 unreadable input, 4 parse failure, 5 write failure.

## CI conformance data (`ciconf`)

`.github/workflows/conformance.yml` needs no local data. Inputs are the pinned grammar repositories' own
`test/corpus/*.txt` (under each grammar's license; the archive is sha256-verified before anything is read):

```
native-ref -pins tools/gen/pins.json -fetch -dl <dl> -work <work> -out <exe dir> -corpus <corpus dir>   # build + extract corpora
#   -queries <dir>: also extract each archive's queries/ into <dir>/<extractDir>/queries (sha256-verified; TSG_QUERY_DIR for the query lane)
#   -nobuild: verify and extract only (no compiler); GITHUB_TOKEN, when set, authenticates -fetch against API rate limits
cd tools/native-ref && go run ./ciconf -pins ../gen/pins.json -corpus <corpus dir> -exe <exe dir> -out <data dir> [-seed N] [-j N] [-lang javascript,html,...]
cd tools/conformance && TSG_DATA_DIR=<data dir> TSG_NATIVE_EXE_DIR=<exe dir> go test -run 'TestCorpus|TestEdits|TestParserPoolRealEngine' .
```

`-lang` limits corpus preparation to the comma-separated conformance language names (default: all 23). JSX inputs use the JavaScript corpus and parser. PHP selects the PHP parser; XML selects XML, not DTD.
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
  `changed-ranges`). A native run that fails is listed in `native-failures.csv` and `ciconf` exits 1.

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
when it reaches zero. It reads only `pins.json` and public API answers and changes nothing.
Run by `.github/workflows/upstream-watch.yml` with its read-only token.

## Recorded grammar patches

Both builders use the ADR-0023 patch records in `pins.json`; grammar-input patches with `regenerate: true` use
`tools/gen/grammarpatch` to compare two independent generated source trees with the pinned CLI. `-fetch` also
retrieves the required CLI gzip upstream-first, with the release mirror as fallback and the original SHA-256
still mandatory. A generation record is kept at `<work>/patches-<lang>/provenance.json`.
`-unpatched` builds the original C for the required before/after comparison; it never changes the pin set.
`-patch-alert N` sets the patch-count warning threshold (default 3). See `tools/gen/README.md` for the record schema.

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
fail the command. The `-allow-nul-truncation` exception is only for this recorded YAML fix.

`go test -run '^TestQueryIndexInvalidation$' .` also exercises the retained `query_index` invalidation probe
against the current shim and pinned JSON/runtime C. It asserts cache invalidation directly and checks a negative
mutation without the invalidation assignments; grammar regeneration is unnecessary for this test.
