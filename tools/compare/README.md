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

## Pins and the one-time go.sum bootstrap

The versions are in each adapter's `go.mod`. Their `go.sum` files must not be written by hand or on a machine that
has not verified them: they come from the workflow.

1. First run (a pull request touching this directory, or *Run workflow* with `bootstrap=true`): an adapter without
   `go.sum` runs in **bootstrap mode**. On a copy, `go mod tidy` resolves the module graph with the default checksum
   database, and the proposal is uploaded as `go-sum-proposal/<adapter>/go.{mod,sum}`. The measurements still run on that
   copy. The run **fails** unless dispatched with `bootstrap=true`; nothing is committed by the workflow.
2. Review the proposal (module list, versions against the plan, sums against the sum database), and commit the files.
3. From then on `stage.sh` downloads with `-mod=readonly`, runs `go mod verify` and `go list -deps`, and fails when
   `go.mod` or `go.sum` would differ from the committed bytes. The sandbox builds with `GOPROXY=off` and the staged cache.

## Isolation

Trusted steps (network): checkout (no persisted credentials), the pinned Go archive (sha256), this repository's own
Go tools, `native-ref -stage` and `benchci fetch` (pinned archives, sha256-checked, nothing generated or compiled),
`go mod download`. Untrusted steps (grammar generation, C compilation, every adapter and worker) run in a digest-pinned
container: `--network none`, read-only root, all capabilities dropped, `no-new-privileges`, unprivileged user, PID/memory/CPU
limits, read-only inputs, a tmpfs scratch and one result volume. The trusted collector (`compare collect`) accepts only
allowlisted names, regular files, schema-valid bounded content and 25 MB in total; anything else is rejected and the
job fails, an oversized file is recorded by size and hash and leaves the run incomplete.

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
  (Go heap only). Peak RSS is the worker's own `VmHWM` over three fresh processes, minus a no-parse control, for the large inputs.
* Correctness: each candidate's public S-expression against `ts_node_string` of its own native reference, for fresh, edited-fresh and incremental parses.
  Levels: `sexp-exact`, `profile-marks` (formatting only), `profile-kinds`, `mismatch`, `unparsed`. Mismatches are rows, not failures.
* Build: every adapter in the compiler image (size, delta from the empty consumer) and in a compiler-free image (`CGO_ENABLED=0`, with a parse smoke test).

## Evidence

Nothing here is an E4 speed claim or an E5 C-agreement claim (docs/specs/validation.md): each library ships different grammar bytes, runtimes and
scanners, and S-expression agreement is at most E2. The report states this in its header. Any README statement needs a separate review of the results.

## Known gaps (also listed in the implementation report)

* The identical-grammar experiment (official ABI-15 pseudo-version linked with the product's regenerated C) is not implemented.
* Native references exist for the product, the official binding and Smacker. The other three are compared with the product's reference as a labelled diagnostic.
* Tree comparison covers the small inputs; large inputs are timed but not tree-compared.
* Size is one binary per adapter with all its grammars linked, not one per grammar.
