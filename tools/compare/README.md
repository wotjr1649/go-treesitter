# tools/compare: comparative benchmark

Compares this library's released modules with other Go tree-sitter libraries on identified inputs: full parse,
incremental edit, Go-heap allocations, peak RSS, consumer size, the no-C-compiler build, and S-expression agreement with
a native C reference. The design is the approved plan (S7); the only runner is
[`.github/workflows/compare.yml`](../../.github/workflows/compare.yml).

**Do not build, test or run the adapters (`adapters/*`) on a developer machine or in any other workflow.** They import
third-party code (some with C runtimes). The orchestrator and the harness import none and are tested locally:

```
cd tools/compare && go vet ./... && go test ./...      # orchestrator, fake-worker end-to-end test
cd tools/compare/harness && go test .                  # worker protocol, edits, S-expression profiles
```

## Layout

| Path | Module | Role |
| --- | --- | --- |
| `harness/` | own | Worker command line and `Result` schema, edit construction (`MakeEdit`), S-expression comparison profiles. No candidate imports. |
| `cmd/compare/` | `tools/compare` | `select` freezes inputs and edits, `run` measures, `check` compares trees with the native references, `collect` validates a result directory, `report` writes the summary. |
| `adapters/<name>/` | one each | One worker binary per candidate (`product`, `official`, `smacker`, `gotreesitter`, `bonsai`, `malivvan`), each pinned by its own `go.mod`/`go.sum`. Separate modules because the two cgo bindings carry colliding C symbols. `empty` is the size baseline. |
| `native/sexp_ref.c` | | One C driver (`sexp`, `edit-sexp`: `ts_node_string` of a fresh or incremental parse) linked with each candidate's own runtime and grammar C. The product's build uses `native-ref -driver`. |
| `scripts/` | | `pins.env` (Go version and checksums, image digests), `stage.sh` (trusted fetch), `docker-run.sh` (sandbox flags), `sandbox-measure.sh`, `sandbox-nocc.sh`. |

## Running it

The workflow runs on `workflow_dispatch` only, and every job refuses any ref but `main`: there is no pull-request or scheduled
trigger, so a change to this directory is measured only after it has landed. Inputs: `mode` (`full` = the plan's repetitions,
`smoke` = one round, two repetitions, a few inputs) and `bootstrap`.

## Pins and the one-time go.sum bootstrap

The versions are in each adapter's `go.mod`. Their `go.sum` files must not be written by hand or on a machine that has not
verified them: they come from the workflow.

1. Dispatch once with `bootstrap=true`. Only the proposal job runs: on a copy it resolves each module graph with `go mod tidy`
   (default checksum database) and uploads `go-sum-proposal/<adapter>/go.{mod,sum}`. No candidate is built, run or measured.
2. Review the proposal (module list, versions against the plan, sums against the sum database) and commit the files to `main`.
3. A normal run **stops before any sandbox step** unless a committed `go.mod` and `go.sum` exist for every adapter. It then
   downloads with `-mod=readonly`, runs `go mod verify` and `go list -deps`, fails if either file would change, and records
   `go list -m all` and the file hashes with the results. The sandbox builds with `GOPROXY=off` and the staged cache.

## Isolation

Trusted steps (network): checkout (no persisted credentials), the pinned Go archive (sha256), this repository's own Go tools,
`native-ref -stage` and `benchci fetch` (pinned archives, sha256-checked, nothing generated or compiled), `go mod download`.
All staged files are made readable by every user. Untrusted steps (grammar generation, C compilation, every adapter and worker)
run in a digest-pinned container: `--network none`, read-only root, `no-new-privileges`, PID/memory/CPU limits, read-only inputs,
tmpfs areas, one result volume.

Inside the container the **measurer** (this repository's `compare`, root with only SETUID/SETGID/CHOWN/DAC_OVERRIDE/FOWNER) is
the only writer of results. Each adapter's builds, native reference and workers run as that adapter's own user (20000+index,
no capabilities) in its own directory under `/work`; binaries, inputs and `/out` are not writable by those users, so a
candidate can neither forge a measurement row nor change another candidate's files. The measurer reads what a candidate leaves
behind with bounded, link-refusing reads and keeps only sanitized printable text; build logs reach the results only as an exit
code plus up to 300 printable characters.

The trusted collector (`compare collect`) accepts only allowlisted names and regular files, checks every measurement row against
the measurer's own cell plan and workload (cell, adapter, language, operation, input, round; no duplicates), requires printable ASCII,
and bounds each file and the set. Oversize files are fingerprinted from a bounded prefix and leave the run incomplete. After the
report is added, `compare seal` enforces the total cap on the final artifact set and writes `SHA256SUMS`; the aggregate job re-checks it.

## What is measured

* Inputs: the nine large inputs of `tools/bench/ci/large.json` for the five languages (JSON generated, 3 Go, 3 Python, 1 JavaScript, 1 C) and,
  per language, up to 24 small inputs (at most 25 KiB, unique, valid UTF-8) from the pinned corpora, chosen by sha256 then id:
  the first 18 clean and the first 6 with syntax errors according to the product's native reference. Shortages and exclusions are in `workload.json`.
* Edits: a single-line, equal-length ASCII token replacement per input (frozen before any candidate runs; old and new end
  points equal, the timing workload), one space-to-newline edit per language (end points differ; correctness only) and the `ciconf`
  insert/delete/replace edits (correctness only).
* Timing: fresh process per block, one untimed warm-up, then 5 (large) or 7 (small) timed operations; the product and the candidate
  run one after the other in the same window, AB/BA alternating by a manifest seed; 6 large and 4 small rounds. The paired ratio is
  candidate/product of block medians. Fixture ratios are summarized by their median, IQR and range; a spread above 20% of the median or a range
  crossing 1x is **inconclusive** and is never rerun. Allocation-only runs repeat the same counts and report `runtime.ReadMemStats` deltas
  (Go heap only). Peak RSS is the worker's own `VmHWM` over three fresh processes (one in smoke mode), minus a no-parse control, for the large inputs.
* Correctness: each candidate's public S-expression against `ts_node_string` of its own native reference, for fresh, edited-fresh and incremental
  parses of the small inputs and the fresh parse of the large ones. Only a worker that exited normally, answered `ok` and left an output file matching its
  reported length and hash is graded. Levels compare parent/child structure: `sexp-exact`, `tree-equal` (same tree, formatting differs), `tree-lossy`
  (same tree after discarding UNEXPECTED token text; a lossy normalization), `shape-only` (same kinds and nesting, fields or marks differ), `mismatch`,
  `unparsed`. Where a candidate has no native reference of its own, the product's is used and the row says `diagnostic-product`. `compat` flags a candidate that
  reports an error for bytes the product's reference accepts cleanly, using the product reference's result for the same edited bytes and operation.
* Build: every adapter in the compiler image (size, delta from the empty consumer) and in a compiler-free image (`CGO_ENABLED=0`, with a parse smoke test).

## Evidence

Nothing here is an E4 speed claim or an E5 C-agreement claim (docs/specs/validation.md): each library ships different grammar bytes, runtimes and
scanners, and S-expression agreement is at most E2. The report states this in its header. Any README statement needs a separate review of the results.

## Which cells are summarized

Expected cells (grammar x library x operation x input) are generated from the workload and the adapter list, independent of which builds or
preflights succeeded. A cell is `measured` only when both passes (time, allocation) finished every scheduled round with complete samples and
normal exits; otherwise it is `partial`, `failed`, `incomplete`, `unsupported` or `unavailable`. The summary's ratios include only measured cells whose input
has a correctness grade (a compared fresh parse) for both the product and the candidate. Everything else is counted in the coverage table, and every
raw row stays in `measurements.jsonl`. Peak RSS differences appear only with the full number of fresh-process samples for both the operation and the control.

## Known gaps (also listed in the implementation report)

* The check does not compare root coverage, byte ranges or points (only the S-expression structure), so a binding that gets a range wrong but prints the same tree is not caught.
* A single operation has a 30 s watchdog inside the worker (it prints a partial result with status `timeout`); the measurer adds a per-process ceiling. Tmpfs space is shared by all candidates, and the container's PID limit is shared with the measurer.
* The grammar C, ABI and patch identity of each library is not recorded beyond the module graph, the native references' hashes and each library's self-reported dependency versions.
* The large JavaScript input is pinned by the sha256 of the extracted member; the npm tarball itself is not digest-pinned (`tools/bench/ci/large.json`).
* No allocation baseline for the harness itself is subtracted from the Go-heap counters.
* The ciconf step runs the pinned native build as the product's user; its dump headers decide which inputs count as clean.
* The identical-grammar experiment (official ABI-15 pseudo-version linked with the product's regenerated C) is not implemented.
* Native references exist for the product, the official binding and Smacker. The other three are compared with the product's reference as a labelled diagnostic.
* Large inputs are tree-graded on their fresh parse only (no edits); small inputs get fresh, edited-fresh and incremental parses.
* Malivvan's early/late samples are not summarized separately (the raw samples are kept).
* Size is one binary per adapter with all its grammars linked, not one per grammar.
