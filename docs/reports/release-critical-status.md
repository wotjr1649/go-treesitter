# Release-Critical status board

Updated 2026-11-24 for v0.6.0 Preview (25 grammar modules, engine ABI 4).
Historical gate tables retain their original commits, pins and scope.
The previous board for the `gotreesitter` carrier is in Git history at main `9ad82eb`.

## v0.6.0 release scope

- **Release-Critical scope.** All 25 grammar pins, including the first Swift module and PostgreSQL SQL (`postgres`).
  The [README](../../README.md#use) names the source tiers and the [migration notes](../../MIGRATING.md#upgrading-from-v050-to-v060-engine-abi-4)
  describe consumer-visible changes.
- **Release policy (ADR-0022).** The [publication procedure](../specs/publication.md#multi-module-release) governs
  this Preview pre-release. Tags and publication require separate owner authority.

## v0.6.0 preparation gates

Candidate: [`8c14a15083c38fbc692cd08bb10e4fed90d4c46c`](https://github.com/wotjr1649/go-treesitter/commit/8c14a15083c38fbc692cd08bb10e4fed90d4c46c).
The release-preparation commit differs from this candidate only in documentation. Every hosted run below
reports the candidate as its head SHA. The pins SHA-256 is
`66f7fda502664f8104f42d001b7bb63cf24a8bef51841f3e38ddee3074d8f7e2`. The candidate uses tree-sitter
runtime and CLI 0.27.2, pins schema 3, recipe `v2-O3` and engine ABI 4. Product results use `CGO_ENABLED=0`.

| Gate | v0.6.0 candidate result | Scope |
|---|---|---|
| Windows product lane | PASS, E3 product, 4/4 jobs | Native Windows amd64/arm64 product execution and licence checks. The separate race job contributes diagnostic evidence. [45311382045](https://github.com/wotjr1649/go-treesitter/actions/runs/45311382045) |
| Grammar pin move evidence | PASS, 1/1 push job, diff skipped | The push supplied no explicit comparison refs. C# v0.24.0 has separate old/new pin evidence on PR [#104](https://github.com/wotjr1649/go-treesitter/pull/104). [45311382097](https://github.com/wotjr1649/go-treesitter/actions/runs/45311382097) |
| Reproducible generation | PASS, E3, attempt 2, 26/26 jobs | All 25 Linux amd64 grammar shards matched committed bytes, including regenerated C# v0.24.0. The aggregate passed. The first attempt is recorded below. [45311381991, attempt 2](https://github.com/wotjr1649/go-treesitter/actions/runs/45311381991/attempts/2) |
| Conformance | PASS, E5, 52/52 jobs | 25 grammar shards and one aggregate per Linux architecture (amd64 and arm64). Pinned corpora and bounded kit inputs cover native dumps, seeded edits and ranges, raw query matches, strict fresh/edited fields, symbol lookups and retained known gaps. [45311382111](https://github.com/wotjr1649/go-treesitter/actions/runs/45311382111) |
| Engine CI | PASS, E3, 156/156 jobs | 150 grammar jobs and six engine jobs. Native Linux amd64/arm64, macOS arm64 and Windows amd64/arm64 on Go 1.27.x, plus native Linux amd64 on supported Go 1.26.x. [45311382038](https://github.com/wotjr1649/go-treesitter/actions/runs/45311382038) |
| Speed / memory bench | PASS, E4, 75/75 grammar/platform jobs | Paired product/native speed and memory on hashed large inputs and the pinned small corpora, on Linux amd64/arm64 and macOS arm64. macOS uses the ADR-0022 private-byte adjustment and reports engine-region counts as `NOT_RUN`. [45318803258](https://github.com/wotjr1649/go-treesitter/actions/runs/45318803258) |
| Balancing cancellation | PASS, E5 probe builds, 25/25 jobs | Linux amd64. Across the pinned big and follow-up inputs, 6,175 stops landed in balancing and all 8,618 next parses matched the native reference. The JSON no-reset control disagreed in all three comparisons. [45318808525](https://github.com/wotjr1649/go-treesitter/actions/runs/45318808525) |
| Nightly diff | PASS, E5 for sampled cases, 50/50 jobs | All 25 grammars on Linux amd64/arm64. Seed `20261124`, four minutes per grammar, 30 seconds per Go fuzz target and a 30-second watchdog per Go case/native operation. [45318813099](https://github.com/wotjr1649/go-treesitter/actions/runs/45318813099) |
| Upstream watch | PASS, E1, 2/2 jobs | One Swift warning, accepted in this PR until 2027-02-24 ([#109](https://github.com/wotjr1649/go-treesitter/issues/109)), with existing tier-review notices for CSS, Go, HTML and Python. Bash was below the age threshold in this run. [45318817150](https://github.com/wotjr1649/go-treesitter/actions/runs/45318817150) |
| Candidate module ZIP rehearsal | PASS, local E3 packaging | Windows amd64, Go 1.27.4, file `GOPROXY`, empty module cache and no `replace`. The root and all 25 grammar ZIPs resolved at v0.6.0, all 25 samples parsed cleanly with `CGO_ENABLED=0`, and the consumer linked for `linux/arm64`. |
| Candidate source-row inventory | PASS, local E1 | 34 fetch-plan rows: all 26 selected source archives matched their pins after download and eight toolchain rows were excluded. Draft upload inventory is the 26 source archives plus `SHA256SUMS`. |
| Tag/proxy resolution and published mirror assets | Pending | Root and grammar tags, direct/public proxy resolution, draft asset digests, attestation and published download hashes are publication steps. |

- **Regeneration attempt 1.** Twenty-four grammar shards passed. The C shard stopped before comparing generated
  bytes after the official WASI SDK 34 Linux x86-64 URL and the v0.5.0 and v0.4.0 mirror requests each
  returned HTTP 403. The aggregate also failed. A single rerun on the same candidate passed every shard.
  [First C job](https://github.com/wotjr1649/go-treesitter/actions/runs/45311381991/job/138167676328) ·
  [First aggregate job](https://github.com/wotjr1649/go-treesitter/actions/runs/45311381991/job/138173733682)
- **Conformance.** Each Linux platform recorded 8,462 fresh corpus inputs, 24,726 seeded edit/range comparisons,
  115,938 query/input pairs (stride 1), 76,405 fresh-field rows and 179,919 edited-field native result rows, with
  zero differences against the native reference. Native and Go incremental trees differ from fresh in the same
  30 cases per platform. The shard inputs include the [bounded kit register](../validation/workloads.md#bounded-grammar-kit-inputs).
  [Conformance run](https://github.com/wotjr1649/go-treesitter/actions/runs/45311382111)
- **Bench.** The highest all-paired-round large-input geomean was 2.347x native (Python, Linux amd64). TSX
  `App.tsx` geomeans were 1.783x / 1.838x / 1.892x on Linux amd64 / Linux arm64 / macOS arm64. All 8,183
  eligible small files passed on the first pass on each platform (24,549 file/platform decisions). The ratios
  describe the paired measurement windows on these shared runners. [Bench run](https://github.com/wotjr1649/go-treesitter/actions/runs/45318803258)
- **Nightly inputs.** Each shard mutates its pinned `test/corpus` and [bounded kit](../validation/workloads.md#bounded-grammar-kit-inputs)
  seeds, generates edits and compares dumps, incremental dumps and changed ranges with the pinned native
  executable. The 50 job summaries record per-shard input-set hashes and kit manifest SHA-256
  `2e694d893c5cfd74cbb46f2c9b1837bccd75c8da101cac7f2da9491e82afed8c`. Under seed `20261124`, the
  summaries report 670,491 cases on Linux amd64 and 563,508 on Linux arm64 with zero mismatches.
  [Nightly run](https://github.com/wotjr1649/go-treesitter/actions/runs/45318813099)
- **Source watch.** nvim-treesitter is ahead of the Swift pin at `fef1236022cc`, two commits that add
  multi-pattern catch clauses. The owner accepted the warning until 2027-02-24: v0.6.0 ships the gated pin, and the
  pin moves after the release ([#109](https://github.com/wotjr1649/go-treesitter/issues/109)). CSS, Go (`golang`), HTML and Python have source-age tier-review notices
  covered by existing acceptances expiring 2027-05-16 UTC (exclusive). Bash matched the reference pin and had
  no age warning on 2026-11-24. [Watch run](https://github.com/wotjr1649/go-treesitter/actions/runs/45318817150) ·
  [Bash follow-up #107](https://github.com/wotjr1649/go-treesitter/issues/107)

The local rehearsal packaged the tested candidate tree. The publication procedure calls for a rehearsal of the
final release commit so the root ZIP includes the final document bytes. The release decision includes owner
review of the [known limitations](limitations.md).

## v0.5.0 release scope

- **v0.5.0 Release-Critical scope:** the 24 release pins, including `postgres` (SQL only, no PL/pgSQL). Swift is excluded from v0.5.0 (its first release is v0.6.0, [limitations](limitations.md#swift-v060)); the T-SQL pin stays unchanged. Sources and tiers are in the [README](../../README.md#use).
- **Release policy (ADR-0022):** Preview (GitHub pre-release), following [publication.md](../specs/publication.md) step by step. Tags and publication require separate owner authority.
- Identity: tree-sitter v0.27.2 and the grammar commits in `tools/gen/pins.json`; Go 1.27.4; `CGO_ENABLED=0`.

## v0.5.0 preparation gates

Preparation base: `a6391abf7746aa216d9f4fdc86151bc6948d1dcf` = main `0372fb53a200272a8ba2eac077f739c41d25e259`
plus one C# known-gap documentation correction. Pins SHA-256:
`5467afce00a76529acf5dd13243a51d8218fa164c595ce171fc7fe6d78ea9913`; ABI 3, recipe `v2-O3`.
C now uses `tree-sitter-grammars/tree-sitter-c@f3202ac`; four new ADR-0023 scanner patches join YAML's retained
`0001`. Snapshot arrays use 25 rather than 41 bytes per node, plus source, sparse exceptions and a lazy line index.
The release commit candidate is `02d10aa18cf70baea757d17e9cac39070c774fd9` (main after PR #50).
All nine hosted runs below completed successfully on 2026-11-16; each run's `headSha` equals this candidate.
The pins hash above is unchanged. Counts and measurements below bind this commit, these pins and the named
inputs/platforms; they do not establish E6 or publication approval. Retain and classify failures rather than retrying until green.

| Gate | v0.5.0 state | Scope / required evidence |
|---|---|---|
| Conformance | PASS, E5; [43835484209](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484209): 48 grammar/platform shards + two aggregate jobs; per Linux platform 4,616 inputs, 13,188 edits/range comparisons, 58,574 query/input pairs (stride 1), 46,401 fresh-field and 103,966 edited-field native result rows, zero differences | All 24 grammars, native dumps, seeded edits/ranges, queries, fresh/edited fields and retained known gaps; include bounded grammar-kit inputs; E5 only on the inputs/platforms run |
| Engine CI | PASS, E3; [43835484351](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484351): 150/150 jobs (144 grammar jobs + six engine jobs) | Hosted Linux/macOS/Windows amd64 and arm64 execution, per-platform identities |
| Reproducible generation | PASS, E3; [43835484231](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484231): 24/24 grammar drift shards + aggregate job; no committed-byte differences | All 24 committed grammar modules; PostgreSQL unpatched CLI regeneration and provenance |
| Windows product lane | PASS, E3 product; [43835484319](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484319): amd64/arm64 product jobs, licenses and diagnostic race job, 4/4 total | Native Windows amd64/arm64 execution, CGO=0 |
| Speed / memory bench | PASS, E4; [43835556351](https://github.com/wotjr1649/go-treesitter/actions/runs/43835556351): 72/72 grammar/platform combinations, highest large-input geomean 2.385x (HTML, Linux amd64); no small-file confirmation needed | All 24 grammars on configured platforms; TSX large workload; preserve first-pass small-file failures and their single paired confirmation results |
| Balancing cancellation | PASS, E5 probe builds; [43835560830](https://github.com/wotjr1649/go-treesitter/actions/runs/43835560830): 24/24 grammars on Linux amd64; 5,929 balancing stops, 8,266 next-parse comparisons with zero differences; JSON no-reset control mismatches 3/3 | All 24 grammar probe builds; input-specific recovery evidence |
| Nightly diff | PASS, E5 only for sampled cases; [43835565191](https://github.com/wotjr1649/go-treesitter/actions/runs/43835565191): 48/48 grammar/platform jobs, Linux amd64/arm64, seed 20261116; no mismatches | One release-commit run over every pinned grammar; retain/classify failures, seeds and reproducers |
| Upstream watch | PASS, E1 source-health observation; [43835569820](https://github.com/wotjr1649/go-treesitter/actions/runs/43835569820): zero unaccepted warnings, nine accepted warnings (expiry 2027-05-16 UTC, exclusive); two jobs passed | Pin reachability/source warnings, accepted warnings with six-month expiry; no automatic pin moves |
| Local module ZIP rehearsal | PASS, local E3 packaging | Windows amd64 empty-cache file GOPROXY, root + 24 grammars at v0.5.0, CGO=0 run and linux/arm64 build; packaging evidence only |
| Source-row inventory | PASS, local static check | 33 rows: 25 pin-matching source rows, eight toolchain rows excluded; publication requires those 25 archives plus SHA256SUMS |
| Tag/proxy resolution and published mirror assets | PASS, read-only verification | Root tag on `a53a506`, grammar tags on `84c20bd`; `GOPROXY=direct` and proxy.golang.org with the checksum database resolve root + 24 grammars; uploaded digests 26/26 matched; `gh release verify` passed; all 25 source archives downloaded from the published release match their pins |

Hosted scope on `02d10aa18cf70baea757d17e9cac39070c774fd9`:

- Conformance [43835484209](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484209): the per-platform totals include the bounded kit subset (279 fresh
  inputs and 177 edit pairs across seven grammars); native and engine incremental trees differ from fresh in
  the same 22 cases per platform, with zero engine/native differences. Strict fresh/edit fields and all seven
  retained known-gap inputs passed on each Linux platform. PostgreSQL contributes 71 inputs, 213 edits/range
  comparisons and 923 query/input pairs per platform; its fresh/edit named-field result counts are zero because
  the grammar has no named fields. Input scope is the pinned upstream corpora plus the
  [bounded kit register](../validation/workloads.md#bounded-grammar-kit-inputs), not arbitrary inputs.
- Engine [43835484351](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484351) executes Linux amd64/arm64, macOS arm64 and Windows amd64/arm64 on Go
  1.27.x; Linux amd64 also executes the oldest supported Go 1.26.x. Product jobs use `CGO_ENABLED=0`.
  Regeneration [43835484231](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484231) includes PostgreSQL's two independent unpatched C regenerations
  using pinned CLI 0.27.2 and compares generated modules/provenance with committed bytes on Linux amd64.
  Pin-move [43835484205](https://github.com/wotjr1649/go-treesitter/actions/runs/43835484205) also passed on this candidate; pins were not changed by these runs.
- Bench [43835556351](https://github.com/wotjr1649/go-treesitter/actions/runs/43835556351) covers all 24 grammars on Linux amd64/arm64 and macOS arm64. TSX
  `App.tsx` large-input geomeans are 1.579x / 1.882x / 1.790x on those platforms respectively.
  All 4,337 eligible small files per platform passed their first pass (13,011 file/platform decisions total);
  no file entered the single paired confirmation pass, so there are no second-pass measurements.
  Parameters were five large rounds x five repetitions, three corpus rounds x seven repetitions, and three
  fresh memory processes per input/operation. Ruby `gb18030-tbl.rb` snapshot maxima from raw `mem.csv`
  are 175.602 / 175.480 / 180.375 MiB against the calculated 193.456 MiB limit (rounded), leaving
  17.855 / 17.976 / 13.081 MiB on Linux amd64 / Linux arm64 / macOS arm64. Thus Ruby headroom ranges
  from 13.081 to 17.976 MiB. Memory rows passed peak, private-byte and region-return checks on these runs;
  macOS private bytes use the ADR-0022 adjustment (raw deltas remain reported). Ratios describe shared
  runners in the paired measurement windows, not absolute performance on other hardware.
- Nightly [43835565191](https://github.com/wotjr1649/go-treesitter/actions/runs/43835565191) used default four-minute differential budgets per grammar, 30-second
  Go fuzz-target budgets and a 30-second per-operation watchdog, with seed 20261116 on Linux amd64/arm64.
  The 48 differential summaries record 1,202,392 cases and no mismatches; this is sampled E5 evidence,
  not universal parity. Earlier failed nightly runs remain in the historical sections below.
- Upstream watch [43835569820](https://github.com/wotjr1649/go-treesitter/actions/runs/43835569820) accepted CSS, Go, HTML, Java, JavaScript, Python, TSX and
  TypeScript source-age warnings plus the newer T-SQL stable release warning under the recorded 2026-11-16
  decisions; each expires 2027-05-16 UTC (exclusive). Zero unaccepted warnings does not mean zero warnings.

Publication (2026-11-16): the publication-step-1 rehearsal was rerun on the release commit `a53a506`
(file GOPROXY, empty caches, root + 24 grammar ZIPs, 24/24 clean parses with `CGO_ENABLED=0`, linux/arm64 build);
the rehearsal below on the preparation base is kept as history. Tags, proxy resolution and mirror assets are in
the gate table above and under Released.

Local rehearsal on the preparation base above (Windows amd64, Go 1.27.4, CGO=0) used
`golang.org/x/mod/zip` to create/check 25 v0.5.0 module ZIPs from a tracked snapshot with documentation edits.
Only the separate packaging copy receives release-form grammar module files; committed generated engine bytes
are unchanged. Initially empty module/build caches resolve root + 24 grammars from a file GOPROXY without
replacements; all 24 embedded samples parse cleanly, and linux/arm64 builds. This is packaging/link evidence,
not native C agreement, hosted platform execution or completion of the release-commit gates. The final release
commit still needs step 1 repeated under the publication procedure. Source inventory was compared with pin
filenames and hashes; it does not establish uploaded/downloaded asset digests or a new license audit.

## Wave 1 gates (session 15, Windows amd64, working-tree grammar modules generated by `tools/gen`)

| Gate | State | Evidence level |
|---|---|---|
| Fresh corpus vs native C (1,586 inputs) | PASS, 0 differences | E5 (that corpus only) |
| Edit cases vs native C (5,919) incremental and fresh | PASS, 0 differences; 5 `incremental != fresh` T-SQL cases also differ in native C (ratcheted) | E5 |
| `ChangedRanges` vs native `ts_tree_get_changed_ranges` (5,919) | PASS | E5 |
| Invalid UTF-8 inputs (189) | PASS | E5 |
| Cancel then reparse a different source (7 grammars x large inputs) | PASS | E3 |
| Cancel during balancing, then reparse (deterministic k-th callback stop; json 5 MB, typescript.js; test-only probe build with the product shim) | PASS: 494 of 682 stops landed in balancing; 682/682 next parses equal native; without the reset 6/6 fail | E5 (probe build, those inputs) |
| C agreement in CI from the grammars' own `test/corpus` (943 inputs, 2,829 seeded edits) | PASS on Linux x64 and arm64 and Windows x64, 0 differences | E5 (those inputs) |
| Fuzz (7 grammars, ~2.6M execs) | 0 crashers | E3 |
| Large-input speed (12 inputs) | PASS, geomean 1.356x native on the committed bytes `3bb05a4` (limit 2.5x; host load 71-78 %) | bench lane |
| Small-file speed (1,586) | PASS, 1,586/1,586 on `3bb05a4` | bench lane |
| Memory peak / release (ADR-0022 criterion) | PASS 72/72: worst peak 0.53 of limit; engine regions returned 72/72; worst private delta 6.0 MB of 16 MB | bench lane |
| Reproducible generation | PASS for the modules checked (byte-identical regeneration) | E3 |
| `-race` (diagnostic) | all 7 grammars: no race (sampled edits, quick cancel) | diagnostic |
| Linux amd64/arm64, macOS arm64, Windows amd64/arm64 execution | PASS: engine CI run 43499796838 on `1c9a204`, 10/10 packages per platform (engine, root, generator, 7 grammar modules with embedded samples). C-reference conformance ran on Windows amd64 only | E3 per platform |

## Released

- **v0.5.0 preview** (2026-11-16, immutable GitHub pre-release, attestation verified): twenty-four grammars, engine
  ABI 3. Root tag `v0.5.0` on `a53a506` (root ZIP `h1:JYGKNCLcI4vGNR9UBjjPfd7J/86hKcnWGy91hcv2348=`, equal to the
  step-1 rehearsal ZIP), grammar tags `grammars/<lang>/v0.5.0` on `84c20bd`. Verified after tagging: `GOPROXY=direct`
  and proxy.golang.org, with the checksum database, resolve all twenty-five modules; an empty-cache external module
  installs root + all twenty-four grammars without `replace`, parses each grammar's sample cleanly with
  `CGO_ENABLED=0` and cross-builds linux/arm64 (Windows amd64 host, Go 1.27.4). The release carries its twenty-five
  pinned source archives with `SHA256SUMS`; uploaded digests matched before publication and all twenty-five
  downloaded from the published release match their pins. `tools/gen/pins.json` now lists the mirror bases
  `v0.5.0`, `v0.4.0`.
- **v0.4.0 preview** (2026-11-15, immutable GitHub pre-release, attestation verified): twenty-three grammars, engine
  ABI 3. Root tag `v0.4.0` on `09f4c2f` (root ZIP `h1:swsqEf42qDrpBltRaE1rQ7cTxuQqrWDJcpLJAAZABs0=`, precomputed),
  grammar tags `grammars/<lang>/v0.4.0` on `eb1fbd5`. Verified after tagging: `GOPROXY=direct` and proxy.golang.org,
  with the checksum database, resolve all twenty-four modules with the precomputed root checksum; an empty-cache
  external module installs root + all twenty-three grammars, parses with each, runs six queries and cross-builds
  linux/arm64 without `replace`. The release carries its twenty-four pinned source archives (grammars, runtime,
  wasi-libc) with `SHA256SUMS`; uploaded digests matched before publication and all twenty-four downloaded from the
  published release match their pins. Toolchain archives are not mirrored (ADR-0026 amendment). `tools/gen/pins.json`
  now lists the mirror bases `v0.4.0`, `v0.3.0`.

- **v0.3.0 preview** (2026-11-15, immutable GitHub pre-release, attestation verified): fifteen grammars. Root tag
  `v0.3.0` on `60f1d33`, grammar tags `grammars/<lang>/v0.3.0` on `ee25291`. Verified after tagging: `GOPROXY=direct`
  and proxy.golang.org resolve all sixteen modules with the precomputed root checksum; an empty-cache external module
  installs root + all fifteen grammars, parses with each, runs a query and cross-builds linux/arm64 without `replace`.
  The release carries its sixteen pinned source archives (grammars, runtime, wasi-libc) with `SHA256SUMS`.
- **v0.2.0 preview** (2026-11-14, immutable GitHub pre-release, attestation verified): queries. Root tag `v0.2.0` on
  `bd4d01e`, grammar tags `grammars/<lang>/v0.2.0` on `b4af8ce`. Verified after tagging: `GOPROXY=direct` and
  proxy.golang.org resolve all eight modules with the rehearsal checksums; an empty-cache external module installs
  root + all seven grammars, builds, runs (parse and query) and cross-builds linux/arm64 without `replace`. The
  release carries the pinned T-SQL grammar source archive (its commit is not on the upstream default branch).
- **v0.1.0 preview** (2026-11-14, GitHub pre-release): root tag `v0.1.0` on `98165ce`, grammar tags
  `grammars/<lang>/v0.1.0` on `2b0c9ec`. Verified after tagging: `GOPROXY=direct` and proxy.golang.org resolve all
  eight modules; an empty-cache external module with default Go settings installs root + csharp + tsql, builds, runs
  and cross-builds linux/arm64 without `replace`.

## v0.2.0 gates (queries; CI on `acbb8da` unless stated)

| Gate | State | Evidence level |
|---|---|---|
| Raw query matches vs native C (grammars' own `queries/*.scm` + probe queries over the corpora) | PASS, 0 differences: 22,047 query/input pairs on Windows amd64 (local); 12,584 per platform in CI on Linux x64 and arm64 | E5 (those queries and inputs) |
| Query predicates vs tree-sitter CLI 0.27.2 | Historical v0.2.0 behavior: agree except the RE2/Rust regex difference (Unicode `\w`); 20/20 acceptance probes agree. This predates the [v0.6.0 regex change](../design/query-abi.md#regex-predicate-subset-v060). | E3 (retained CLI probes; not native C evidence) |
| Cancel during balancing, then reparse (all 7 grammars, test-only probe builds) | PASS: 1,729 stops in balancing, 2,411/2,411 next parses equal native, no-reset control 3/3 mismatches (Windows); Linux x64 run 43583147260 PASS with identical callback counts | E5 (probe builds, those inputs) |
| Large-input speed (bench run 43584133627) | PASS: geomean 1.705x (Linux x64), 1.959x (Linux arm64), 1.818x (macOS arm64); limit 2.5x | bench lane |
| Memory peak / release (same run) | PASS on all three; macOS private bytes under the ADR-0022 §3 macOS amendment | bench lane |
| Engine, conformance, regeneration, licenses, race (required checks) | PASS on `acbb8da` | E3-E5 per lane |
| Release rehearsal (module ZIPs from a file proxy, empty-cache consumer, Go 1.26 and 1.27, linux/arm64 build, v0.1.0 grammar on root v0.2.0 returns `ErrQueryUnsupported`) | PASS | local |

## v0.3.0 gates (wave-2 batch 1; CI on `2f9e000`/`3212a5a`)

| Gate | State | Evidence level |
|---|---|---|
| Corpus + seeded edits + changed ranges vs native C (15 grammars, 1,761 inputs, 5,283 edits) | PASS, 0 differences on Linux x64/arm64 (CI) and Windows (local); 13 native-identical incremental != fresh cases recorded | E5 (those inputs) |
| Raw query matches vs native C (each grammar's `queries/*.scm` + probes) | PASS, 0 differences (CI, stride 1) | E5 |
| Native reference character semantics | scanners built with the pinned wasi-libc `isw*`/`tow*` on every platform (PR #25; glibc's C locale differed on U+00E9 in html) | definition |
| Cancel during balancing, then reparse (15 grammars) | PASS, nightly run 43603462204 | E5 (probe builds) |
| Nightly differential fuzzing (15 grammars, Linux x64/arm64) | PASS, run 43603459593 | E5 (those cases) |
| Large-input speed (bench run 43606388464, per grammar) | PASS: 1.46x-2.38x native; highest HTML 2.375x (Linux x64), Python 2.23x (macOS); TSX has no large input | bench lane |
| Memory peak / release (same run) | PASS on Linux x64/arm64 and macOS arm64 | bench lane |

## Historical batch-2 measurements

The following wave-2 batch 2 and fwcd Kotlin observations (2026-11-14/15) predate this ABI 3 integration. They retain their original identities and do not
pass any ABI 3 integration gate or extend the fifteen-grammar ABI 3 field-lookup evidence.

## Wave-2 batch 2 candidates (archive `2026-11-15-q9-batch2`)

YAML, PHP (the `php` parser only), PowerShell, R, Dart, XML (the `xml` parser only), and Ruby passed the local
Windows amd64 gates on the batch 2 detached working tree based on `2f9e0005f11c8520a37cf2878cc30219aac1950f`.
Grammar identities are those recorded in the batch 2 artifacts and preserved by the fwcd Kotlin patch
`da79b3cc909c93f8d05ebc545bc0966f750d4b8590400ba8b8e771c2f11325fa`, not the current ABI 3 module identities.
The native reference used the same pinned sources and wasi-libc character functions. These results do not claim hosted platforms or release approval.

| Grammar | Corpus / edits (E5) | Raw query pairs (E5, stride 1) | Balancing stops / trials (E5 probe) | Large speed geomean / median (E4) |
|---|---:|---:|---:|---:|
| yaml | 131 / 393 | 1441 | 253 / 343 | 1.159x / 1.218x |
| php | 143 / 429 | 2002 | 258 / 350 | 1.283x / 1.239x |
| powershell | 147 / 441 | 1764 | 266 / 355 | 0.301x / 0.312x |
| r | 89 / 267 | 1157 | 241 / 340 | 1.266x / 1.270x |
| dart | 187 / 561 | 2431 | 250 / 351 | 1.377x / 1.353x |
| xml | 16 / 48 | 176 | 236 / 340 | 1.268x / 1.293x |
| ruby | 291 / 873 | 3783 | 255 / 349 | 1.046x / 1.065x |

Corpus (1,004 inputs), seeded edits (3,012, including changed ranges), raw query pairs (12,754) and every next
parse after a sampled balancing cancellation agree with native C, with zero mismatches. All 22 module test suites
passed. Memory passed all 42 fresh-process cases under ADR-0022: peak at most 100 x input + 64 MB, private delta
within 16 MB, and all engine regions returned. The seven large inputs and their licenses are SHA-256-pinned in
[`tools/bench/ci/large.json`](../../tools/bench/ci/large.json). No benchmark was repeated after observing a gate result.
YAML and R each matched two independent regenerations; the existing 15 modules regenerated byte-identically.

At that measurement YAML carried the owner-approved temporary [ADR-0023](../design/decisions/ADR-0023-temporary-upstream-grammar-patches.md)
patch `0001-no-early-eof` (Bash, Ruby and Python patches and YAML `0002` came later; see
[limitations](limitations.md)). Native unpatched/patched comparison (E4) covered 159 inputs: 18 existing error trees changed, ten first-NUL
truncations became full-length error trees, and every other error-free tree stayed identical. Patched root-end faults: zero.
PHP four, Dart one and Ruby one seeded edits differ between incremental and fresh in native and engine alike;
these are [native characterizations](../validation/known-regressions.md#wave-2-batch-2-seed-1-native-incremental-vs-fresh-characterization),
not engine/native mismatches. See [limitations](limitations.md). Kotlin was evaluated separately on fwcd in the next section.

## Kotlin from fwcd (2026-11-15)

Kotlin uses the unmodified tier-3 `fwcd/tree-sitter-kotlin` default-branch commit
`d6af96d2e23ef0a8ff5ab512864378cf08fba311` (2026-10-31; selected 2026-11-15), per ADR-0024.
Its module uses the development go.mod form. The old kit grammar and its scanner patch are absent.
These are local Windows amd64 results on the detached fwcd Kotlin tree based on `2f9e000`, not hosted or release approval.

| Gate | Kotlin result | Level |
|---|---|---|
| Module + root/generator/native-ref/conformance/bench tests | PASS, all 23 grammar modules (CGO disabled) | E3 |
| Corpus / edits / ChangedRanges vs native | 1,462 / 4,386 / 4,386; zero differences | E5, those inputs |
| Raw query comparisons (upstream highlights.scm and tags.scm + ten probes, stride 1) | 17,544, zero differences | E5 |
| Balancing stop / trials | 258 / 350, every next parse matches native | E5, test-only probe |
| Kotlin hang/minimized/variant inputs (2026-11-14 scanner-guard verification) with an outer 60s watchdog on each side | 55 / 55 finish and match | E5, those inputs |
| Eleven JetBrains inputs, large speed geomean / per-file median geomean | 1.023x / 1.041x native (limit 2.5x) | E4 |
| Small corpus speed | 1462 / 1462 pass (<=1ms or <=2.5x native) | E4 |
| Memory (11 inputs x parse/snapshot x 3 fresh processes) | 66 / 66 pass, regions returned, private delta <=16 MiB, peak <=100 x input +64 MiB | E4 |
| Regeneration | 17 / 17 files byte-identical in one independent regeneration | E3 |

Three seeded incremental trees differ from fresh exactly as native C does; their complete signatures are
in the [native characterization register](../validation/known-regressions.md#fwcd-kotlin-seed-1-native-incremental-vs-fresh-characterization).
No new grammar patch, weakened criterion or benchmark retry was used. The original batch-2 seven grammar
engine bytes and the 256 wave-1/batch-1 files are unchanged. The nightly differential lane now isolates its Go
case and native commands with the [30s wall-clock watchdog](../specs/validation.md); timeout inputs are retained
and fail that grammar while the supervisor continues. Hosted Linux/arm64 nightly execution has not run.

## Wave-2 batch 2 ABI 3 integration (2026-11-15)

Candidates: YAML (patched), PHP, PowerShell, R, Dart, XML, Ruby and unpatched fwcd Kotlin. The baseline is
`35233d365418147ee8cac8bd7c54f5f617bfe753` plus the ABI 3 field-lookup patch (2026-11-15 field-lookup ABI and field-export-cost runs). Existing fifteen-module
results above remain bound to their recorded identities; they do not transfer to the eight new modules.

| Gate | ABI 3 integration state | Measured scope |
|---|---|---|
| Module tests | PASS 23/23, E3 | Fifteen ABI-v3 modules plus eight new modules, CGO disabled; unchanged root/engine code |
| Root / generator / native-ref / full conformance | PASS, E3 | Root and fifteen tests in ABI-v3 phase; final generator/native-ref and integrated full conformance pass |
| Corpus / edits / raw native queries (stride 1) | PASS, E5 | All 23: 4,227 inputs, 12,681 edits, 53,371 query pairs, zero differences from fresh native builds |
| Strict field lookup | PASS, E5 | 4,227 fresh corpus inputs, 36,117 native/Go result rows, zero differences; `TSG_REQUIRE_FIELD_LOOKUP=1` |
| YAML root ends and unpatched comparison | PASS, E5 | 159 inputs; 28 allowed changes, including 10 NUL fixes; product dump/field/root-end differences zero |
| Balancing cancellation | PASS 8/8, E5 | Test-only builds; nonzero balancing stops and zero subsequent-parse differences for each grammar |
| Large speed / ADR-0022 memory | PASS, E4 | Eight geomeans <=2.5x native, small corpus passes, 108/108 memory runs pass; Ruby minimum peak margin 1.40 MiB |
| Regeneration | PASS, E4 bytes | YAML and R twice each, 17/17 files identical every run; original fifteen 256 files byte-identical to T13b |
| Patch / mirror inventory and actionlint | PASS, E3/static | YAML one patch, others zero; 30 archive candidate rows (historical inventory; v0.4.0 publishes 24 source rows only); workflow lint passed |

T13b is `60ccaf3ab36aff760e6633aa99bd0d1f6d1469f9`: base plus the ABI 3 field-lookup patch and the F1–F4 fixes from the 2026-11-15 independent review of the field-export cost filter. Its shim SHA-256 is
`b8ee914f19412a811ee7f83cca555d6e95e2e84310bf02c3afb267d1fbe1ca05`; the 23-grammar pins SHA-256 is
`8ce3944348352142818521d96d99785f3dc720cc9a1b5f732c4cebaf5560a6e9`. The maintainer-local evidence archive (`2026-11-15-q14-batch2` handoff)
binds the Windows/Go/tool identities, input hashes, original failures and interrupted attempts, raw results and patches.
The nine recorded native incremental/fresh differences for PHP/Dart/Ruby/Kotlin exactly match their old input,
edit-coordinate, dump and range signatures. Existing fifteen-grammar differences remain unchanged. No criterion was relaxed.

| Grammar | Corpus | Edits | Query pairs | Native field results | Large speed ratio | Memory runs |
|---|---:|---:|---:|---:|---:|---:|
| yaml | 131 | 393 | 1441 | 367 | 1.124x | 6/6 |
| php | 143 | 429 | 2002 | 2093 | 1.250x | 6/6 |
| powershell | 147 | 441 | 1764 | 230 | 0.263x | 6/6 |
| r | 89 | 267 | 1157 | 2299 | 1.166x | 6/6 |
| dart | 187 | 561 | 2431 | 759 | 1.355x | 6/6 |
| xml | 16 | 48 | 176 | 16 | 1.282x | 6/6 |
| ruby | 291 | 873 | 3783 | 2054 | 0.997x | 6/6 |
| kotlin | 1462 | 4386 | 17544 | 533 | 1.015x | 66/66 |

Hosted checks subsequently ran on the earlier pin set: conformance run
[43707969139](https://github.com/wotjr1649/go-treesitter/actions/runs/43707969139) on `dc3b23d` passed 48/48 jobs;
engine run [43707969156](https://github.com/wotjr1649/go-treesitter/actions/runs/43707969156) on `dc3b23d` passed
144 jobs; bench run [43705487569](https://github.com/wotjr1649/go-treesitter/actions/runs/43705487569) on `d44273c`
passed 69/69 grammar/platform combinations (23 grammars, Linux x64/arm64 and macOS arm64). These are historical
results, not gates for the later PHP/PowerShell/Dart pin moves or for the release commit.

## v0.4.0 preparation after the pin refresh and schema-tool fixes

Baseline for this preparation is `506e4f7`: the 2026-11-15 nvim-treesitter pin refresh moved PHP to `58d5643086ae`, PowerShell to `65ea6919e816` and
Dart to `0038280b774b`; the 2026-11-15 schema-tool fixes added offline schema commands and ADR-0025. The v0.4.0 preparation pins SHA-256 was
`18a8497b6e0fa028b4d7d1607ff2505a51fc3a182a4989f61dd55b114255c3a1`. The wave-2 batch 2 ABI 3 integration's aggregate counts above keep their
old pins identity. The pin refresh's maintainer-local evidence archive (`2026-11-15-q17-pins-nvim`) records Windows amd64 / Go 1.27.4 measurements on the
new three pins (base `dc3b23d`, candidate tree `a0f905db15810e380dac3256317931eaa6c70b80`):

| Grammar | Corpus / seeded edits | Raw query pairs | Fresh-field result rows | Scope |
|---|---:|---:|---:|---|
| PHP | 146 / 438 | 2,044 | 2,122 | E5; zero product/native differences |
| PowerShell | 165 / 495 | 1,980 | 271 | E5; zero product/native differences |
| Dart | 205 / 615 | 2,665 | 864 | E5; zero product/native differences |

The 2026-11-15 edited-tree `ChildByFieldName` E5 evidence covers the original fifteen grammars (5,283 seeded edits and five
retained T-SQL cases, Windows amd64, base `2a1d01d`); it does not cover the eight additions. CI now has the strict
edited-field shard for every grammar; release-commit conformance run [43718543939](https://github.com/wotjr1649/go-treesitter/actions/runs/43718543939) (`f8a9f29`) passed it for all 23 grammars on Linux x64 and arm64 (per platform: 12,798 edits, 97,858 edited-field native result rows, zero differences).

Release commit `f8a9f29`: engine [43718543915](https://github.com/wotjr1649/go-treesitter/actions/runs/43718543915) 144/144 jobs, regeneration [43718543860](https://github.com/wotjr1649/go-treesitter/actions/runs/43718543860) 24/24, Windows
[43718543945](https://github.com/wotjr1649/go-treesitter/actions/runs/43718543945) 4/4, bench [43718548275](https://github.com/wotjr1649/go-treesitter/actions/runs/43718548275) 69/69 (highest large-input ratio 2.334x, YAML Linux x64),
balancing-cancel [43718551120](https://github.com/wotjr1649/go-treesitter/actions/runs/43718551120) 23/23, upstream watch [43718596677](https://github.com/wotjr1649/go-treesitter/actions/runs/43718596677) pass (T-SQL pin reachable).
Packaging and compatibility (v0.4.0 local release rehearsal on `f8a9f29`, Windows amd64): release-form regeneration leaves all 23
`engine_gen.*` byte-identical; empty-cache consumers on Go 1.26.0 and 1.27.4 parse every grammar and run six
queries; linux/arm64 builds; `grammars/json` v0.1.0 (ABI 1) and v0.3.0 (ABI 2) work with root v0.4.0.
Ruby headroom: 2.0 MiB (macOS arm64) to 8.3 MiB (Linux x64).

The prior fifteen-grammar nightly run
[43699872727](https://github.com/wotjr1649/go-treesitter/actions/runs/43699872727) (`a9c0077`, seed 20261115)
failed on Bash linux/arm64 at iteration 43175. The maintainer-local evidence archive of the failure classification classifies it as native C
`char` signedness: signed/unsigned probes on Windows reproduce the runner's Go/native dumps respectively (E2 probe,
that input); Ruby also shows a native signedness difference. Baseline `506e4f7` does not yet force signed `char` in
the native reference. The reference now builds with `-fsigned-char` (PR #36, `3213f3c`); conformance run
[43718543939](https://github.com/wotjr1649/go-treesitter/actions/runs/43718543939) passed, and the required 23-grammar nightly-diff run [43718545692](https://github.com/wotjr1649/go-treesitter/actions/runs/43718545692) (`f8a9f29`) passed
46/46 jobs. Owner disposition (2026-11-15): native-reference platform difference, not an engine defect. Do not erase the failed run.

Mirror coverage follows the configured bases and their immutable contents. After publication and first-base
asset checks, one PR sets `mirrors` to the v0.4.0 base followed by v0.3.0 and restores the development module form,
as specified in [publication.md](../specs/publication.md). Those v0.4.0 publication steps were subsequently completed (see Released above).
The owner's 2026-11-15 ADR-0026 amendment limits v0.4.0 assets to 24 source archives plus `SHA256SUMS`.
The six wasi-sdk/binaryen/tree-sitter CLI host archives are fetched from official releases with pinned SHA-256
checks; this release supplies no toolchain fallback assets until complete bundled notices are established.
The source-only release scope removes the SDK redistribution-notice blocker; collected texts stay in the
maintainer-local evidence archive for later SBOM work.


## PostgreSQL adoption evidence (Windows amd64, local, before the rebase)

This historical adoption run added a 24th grammar candidate, outside the immutable v0.4.0 release. Base `45f9071328d66cd665fd20fb89991b589cf0fa23`,
Go 1.27.4, `CGO_ENABLED=0`, ABI 3 / recipe v2-O3; pins SHA-256
`ec6fac800e2821eb50d22f84ee7828fc230906c653232b4ab4973c9c6c73f773`.
Module `postgres` uses owner-designated `gmr/tree-sitter-postgres@59d0d8cd7506d68de1229fb4bbce838c83b60c8a`,
BSD-3-Clause, SQL only (PL/pgSQL excluded). Unmodified grammar inputs regenerate with CLI 0.27.2 / native JS;
the engine and a fresh native build have identical generated-source provenance. No patches are applied.

Local E5: 71 corpus inputs, 213 edits (fresh, incremental and changed ranges), 923 raw-query comparisons
(stride 1, three upstream files plus ten structural probes), zero differences. Strict field tests cover the
71 fresh and 213 incremental trees; this grammar has no named fields, so both sides emit zero field-result rows.
The retained module tests, PostgreSQL fuzz seeds and the hashed realistic small SQL workload pass (E3).

On the [registered Pagila input](../validation/workloads.md#postgresql-candidate), E4 three paired windows
(warmup 1, timed 3 each) give Go/native 1.127257x (range 1.102759–1.160900); the 351-byte SQL report gives
1.702076x and 0.052673 ms Go median geomean. The shared Windows host does not establish absolute performance.
ADR-0022 memory: parse peak WS 905,232,384 bytes, snapshot 1,276,252,160 (limit 2,019,769,864); Close+GC private
deltas 2,367,488 / 3,993,600 bytes; engine regions and committed bytes after Close are zero in both fresh processes.
These are local input-specific results, not E6 or hosted platform evidence.

Two independent full PostgreSQL module generations matched all 17 files; regeneration of the other 23 modules
matched their complete committed file sets and hashes. Archive strict schemas (24), generated PostgreSQL schema,
patch-status, mirror inventory and actionlint passed.

The first generation attempt stopped at the unchanged 2.5 GiB free-physical-memory guard; its failure and partial
outputs are retained. One declared recovery after a sustained headroom check completed. The Korean PostgreSQL adoption report,
patch digest, logs and identity-bound receipts are in the maintainer-local PostgreSQL adoption handoff archive. Hosted drift,
platform, nightly, balancing, bench and release/packaging gates remain pending; no release approval is inferred.
