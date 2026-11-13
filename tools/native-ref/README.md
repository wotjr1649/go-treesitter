# tools/native-ref — native C reference

Builds one native executable per grammar from the sources pinned in `tools/gen/pins.json` (the unmodified tree-sitter
runtime `lib/` plus one unmodified grammar, `parser.c` and `scanner.c`), linked with this repository's own driver
`c/native_ref.c`. The executables produce the reference results the CGO-free conformance lane compares against
(`docs/specs/oracle.md`). CGO is used only by the C compiler here, never by the Go product lanes.

```
cd tools/native-ref
go run . -pins ../gen/pins.json -dl <archive dir> -work <empty dir> -out <exe dir> [-fetch] [-cc gcc|clang] [lang ...]
```

- `lang` is a key of `pins.json` `grammars` (`go` is accepted for `golang`); none = all seven.
- Every archive's sha256 is checked against `archiveSHA256` before extraction (`-fetch` downloads a missing one from the
  GitHub API tarball of the pinned commit, as `tools/gen/regen.sh` does), and every grammar source file against the
  pinned `files` hash. Only the pinned `dirs` are extracted into `-work`; nothing in them is run.
- `-static` on Windows (no MinGW DLLs needed); upstream's `_POSIX_C_SOURCE`/`_DEFAULT_SOURCE` flags elsewhere.
- Needs a C11 compiler (gcc or clang) on `PATH`; large grammars (csharp, tsql, typescript, tsx) take a few GB of RAM to
  compile at `-O2`.

Executable CLI (stdout is binary, `\n` line ends on every platform):

| Command | Output |
|---|---|
| `<exe> dump <file>` | `#tsg-dump/1 root_has_error=<0\|1> nodes=<n>` then one tab-separated line per node in cursor preorder: `depth field type named extra missing is_error has_error start_byte end_byte row:col row:col` |
| `<exe> edit <old> <new> <start> <old_end> <new_end>` | the same dump of `<new>` reparsed incrementally from the edited `<old>` tree |
| `<exe> changed-ranges <old> <new> <start> <old_end> <new_end>` | `ts_tree_get_changed_ranges(old, new)`, one `start_byte end_byte start_row start_col end_row end_col` line per range |

Points are byte columns with `\n` rows (the same as `treesitter.EditFor`). Exit codes: 0 ok, 2 usage / edit out of
range, 3 unreadable input, 4 parse failure, 5 write failure.
