# tools/compare: comparative benchmark

Compares this library's released modules with other Go tree-sitter libraries on identified inputs: full parse,
incremental edit, Go-heap allocations, peak RSS, consumer size, the no-C-compiler build, and S-expression agreement with
a native C reference. The design is the approved plan (S7); the only runner is
[`.github/workflows/compare.yml`](../../.github/workflows/compare.yml).

**Do not build, test or run the adapters (`adapters/*`) on a developer machine or in any other workflow.** They import
third-party code (some with C runtimes). The orchestrator and the harness import none and are tested locally:

```
cd tools/compare && go vet ./... && go test ./...      # orchestrator, fake-worker end-to-end test, no-compiler classifier on stub adapters
cd tools/compare/harness && go test .                  # worker protocol, edits, S-expression profiles
```

## Threat model

The isolation boundary is the **ephemeral GitHub-hosted runner VM of each job**. The workflow protects the repository and
its credentials; it does **not** protect the integrity of the numbers against a malicious candidate.

* The workflow runs on `workflow_dispatch` only and every job refuses any ref but `main`. There is no pull-request or scheduled
  trigger, so a change to this directory is measured only after it has landed.
* Top-level `permissions` are empty and no job asks for any: no `contents: read`, no secrets, no `id-token`, no caches, no
  environment. `actions/checkout` would need `contents: read` only for its default token (its README recommends it), so the jobs
  fetch the commit with `git fetch` instead (anonymous; the repository is public). If the repository ever became private, add
  `contents: read` to those jobs and use the pinned checkout action. Actions that remain (artifact upload/download) are pinned by SHA.
* Candidates and the product run as the runner user in the same VM, one after the other, because paired timing needs the same
  machine. A malicious candidate could therefore falsify the measurements and files of **its own job**, or interfere with the
  product's process in that job. Nothing it can reach carries a credential, and nothing it writes is executed afterwards: the
  aggregate job downloads the artifacts, parses them with bounded, link-refusing reads and strict schemas, and runs no code
  from them; no later privileged step consumes them.
* What is still checked: every candidate's tree is compared with a native C reference (built from that candidate's own vendored
  runtime and grammar where this implementation can, else from the product's reference as a labelled diagnostic), only
  tree-equal results enter the ratios, and the aggregate rejects artifacts that do not match their own plan or schema.
* Evidence: timings are at most **E4**, and only for a pair whose grammar bytes, runtime and API work are controlled
  (docs/specs/validation.md); the libraries here ship different grammars and runtimes, so the rows are descriptive observations
  of identified consumer artifacts. S-expression agreement is at most E2. No E5 claim is made anywhere.

## Layout

| Path | Module | Role |
| --- | --- | --- |
| `harness/` | own | Worker command line and `Result` schema, edit construction (`MakeEdit`), S-expression comparison profiles. No candidate imports. |
| `cmd/compare/` | `tools/compare` | `select` freezes inputs and edits, `run` measures, `check` compares trees with the native references, `nocc` builds without a C compiler, `collect`/`report`/`seal`/`aggregate` validate and summarize. |
| `adapters/<name>/` | one each | One worker binary per candidate (`product`, `official`, `smacker`, `gotreesitter`, `bonsai`, `malivvan`), each pinned by its own `go.mod`/`go.sum`. Separate modules because the two cgo bindings carry colliding C symbols. `empty` is the size baseline. |
| `native/sexp_ref.c` | | One C driver (`sexp`, `edit-sexp`: `ts_node_string` of a fresh or incremental parse) linked with each candidate's own runtime and grammar C. The product's build uses `native-ref -driver`. |
| `scripts/` | | `pins.env` (Go version and checksums), `stage.sh` (pinned downloads, module graphs), `measure.sh` (one measurement job), `identity.sh`. |

## Running it

Inputs: `mode` (`full` = the plan's repetitions, `smoke` = one round, two repetitions, a few inputs) and `bootstrap`.

## Pins and the one-time go.sum bootstrap

The versions are in each adapter's `go.mod`. Their `go.sum` files must not be written by hand or on a machine that has not
verified them: they come from the workflow.

1. Dispatch once with `bootstrap=true`. Only the proposal job runs: on a copy it resolves each module graph with `go mod tidy`
   (default checksum database) and uploads `go-sum-proposal/<adapter>/go.{mod,sum}`. No candidate is built, run or measured.
2. Review the proposal (module list, versions against the plan, sums against the sum database) and commit the files to `main`.
3. A normal run **stops before any candidate step** unless a committed `go.mod` and `go.sum` exist for every adapter. It then
   downloads with `-mod=readonly`, runs `go mod verify` and `go list -deps`, fails if either file would change, and records
   `go list -m all` and the file hashes with the results. Builds use `GOPROXY=off` and the staged cache.

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
* Correctness: each candidate's public S-expression against `ts_node_string` of its native reference, for fresh, edited-fresh and incremental
  parses of the small inputs and the fresh parse of the large ones. Only a worker that exited cleanly, answered `ok` for exactly this call and left an output file
  matching its reported length and hash is graded. Levels compare parent/child structure: `sexp-exact`, `tree-equal` (same tree, formatting differs), `tree-lossy`
  (same tree after discarding UNEXPECTED token text), `shape-only` (same kinds and nesting, fields or marks differ), `mismatch`, `unparsed`. Where a candidate has no native
  reference of its own, the product's is used and the row says `diagnostic-product`. `compat` flags a candidate that reports an error for bytes the product's
  reference accepts cleanly, using the product reference's result for the same edited bytes and operation.
* Build: every adapter in the normal job (size, delta from the empty consumer), and in the `nocc` job with `CGO_ENABLED=0` and only the Go toolchain's bin directory on
  `PATH`, then a smoke parse whose exit status and JSON answer are checked. Outcomes: `ok`, `expected-unsupported` (a cgo adapter whose build says it needs cgo),
  `build-failure`, `smoke-failure`. The job fails unless the product is `ok`. The VM image has a C compiler installed elsewhere: the claim is that these builds need none and
  see none, not that the image has none.

## Which cells are summarized

Expected cells (grammar x library x operation x input) are generated from the workload and the adapter list, independent of which builds or
preflights succeeded. A cell is `measured` only when both passes (time, allocation) finished every scheduled round with complete samples, clean exits and results that
describe their own call; otherwise it is `partial`, `failed`, `incomplete`, `unsupported` or `unavailable`. The summary's ratios include only measured cells whose own operation
(the fresh parse, or the timed incremental token edit) produced a tree equal (`sexp-exact` or `tree-equal`) to the reference's on both the product and the candidate. Everything
else is counted in the coverage table, and every raw row stays in `measurements.jsonl`. Peak RSS differences appear only with the full number of fresh-process samples for both the
operation and the control.

## Known gaps (also listed in the implementation report)

* A malicious candidate can falsify numbers in its own job (see the threat model); only the tree comparison with the native reference is independent of its answers.
* The check does not compare root coverage, byte ranges or points (only the S-expression structure), so a binding that gets a range wrong but prints the same tree is not caught. The snapshot operation is graded through the live-tree fresh parse of the product.
* A single operation has a 30 s watchdog inside the worker (it prints a partial result with status `timeout`); the measurer adds a per-process ceiling. No allocation baseline for the harness itself is subtracted from the Go-heap counters.
* The grammar C, ABI and patch identity of each library is not recorded beyond the module graph, the native references' hashes and each library's self-reported dependency versions.
* The large JavaScript input is pinned by the sha256 of the extracted member; the npm tarball itself is not digest-pinned (`tools/bench/ci/large.json`).
* The ciconf step runs the pinned native build; its dump headers decide which inputs count as clean.
* The identical-grammar experiment (official ABI-15 pseudo-version linked with the product's regenerated C) is not implemented.
* Native references exist for the product, the official binding and Smacker. The other three are compared with the product's reference as a labelled diagnostic.
* Large inputs are tree-graded on their fresh parse only (no edits); small inputs get fresh, edited-fresh and incremental parses.
* Size is one binary per adapter with all its grammars linked, not one per grammar. Malivvan's early/late samples are not summarized separately (the raw samples are kept).
