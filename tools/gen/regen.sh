#!/usr/bin/env bash
# Regenerate grammar modules from the pinned upstream inputs on this host (CI: .github/workflows/regen.yml).
#
#   tools/gen/regen.sh <work-dir> [lang ...]
#
# <work-dir> must not exist or must be empty: every archive is downloaded and extracted only into it, its sha256 is
# checked before extraction. REGEN_RESUME=1 continues a stopped run in a non-empty <work-dir>: an archive already there whose
# sha256 matches its pin is not downloaded again (any other file is), every extraction directory is replaced by a fresh
# extraction of the verified archive, and the language list continues the run (the "== <lang>" lines of its log say which
# were reached; a language is regenerated from scratch, never half-continued). All non-exception grammars execute grammar JS with the pinned CLI's native
# JS runtime and a twice-generated determinism check (ADR-0027). The fetch plan
# (URLs, sha256, destinations) comes from tools/gen/pins.json via `gen -plan`; wasm2go is built from its pinned
# module version with `go install -trimpath`. Output goes to grammars/<lang> in this repository.
# Each selected grammar keeps the form and minimum root from its own go.mod. ROOT_VERSION=vX.Y.Z (or vX.Y.Z-prerelease, as `gen -root-version`)
# overrides that form only for the selected grammars (all grammars if no language list is given).
# REGEN_KEEP_GOING=1 (CI: a regen.yml group of grammars shares one toolchain setup) regenerates the remaining grammars after one failed,
# names every failed grammar and then exits 1; without it the first failure stops the run.
#
# REGEN_PHASE splits a run at its network boundary (the owner-local evidence workflow, .github/workflows/upstream-autopr.yml).
# fetch: the only phase that uses the network. It builds gen, downloads and sha256-verifies every archive and the npm inputs of
# the selected grammars, and builds wasm2go (`go install` compiles; nothing downloaded is extracted or run). run: needs the
# fetch phase's work directory, downloads and builds nothing (an archive missing or differing from its pin is an error), extracts
# the verified archives and regenerates; run it under tools/gen/sandbox-run.sh with the work directory writable, <work-dir>/dl
# read-only and grammars/<lang> writable. Unset: both in one go, as before.
# RUNTIME_REHEARSAL_{PINS,ARCHIVE,TREE,OUT} (all four, set by tools/gen/runtime-rehearsal.sh) make it a TEST-ONLY runtime
# rehearsal: the runtime source comes from ARCHIVE/TREE (hashed by `gen runtime-pins` into the pins copy PINS), every other
# pin is unchanged, and the modules go to OUT/<lang> instead of grammars/<lang>. With RUNTIME_REHEARSAL_CLI_GZ (a candidate CLI
# archive, named like the pinned asset) and RUNTIME_REHEARSAL_CLI_SANDBOX (tools/gen/nightly-sandbox.sh) the candidate CLI replaces
# the pinned one: it is only decompressed here, and gen runs it solely through the sandbox wrapper (gen -cli-sandbox).
set -euo pipefail

W=${1:?usage: regen.sh <work-dir> [lang ...]}
shift || true
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
mkdir -p "$W"
[ -z "${REGEN_RESUME:-}" ] || [ "$REGEN_RESUME" = 1 ] || { echo "regen: REGEN_RESUME must be 1 or unset" >&2; exit 1; }
PHASE=${REGEN_PHASE:-}
case "$PHASE" in '' | fetch | run) ;; *) echo "regen: REGEN_PHASE must be fetch, run or unset" >&2; exit 1 ;; esac
[ "$PHASE" != run ] || export REGEN_RESUME=1 # keeps the fetched archives; fetch.sh downloads nothing in this phase
[ -n "${REGEN_RESUME:-}" ] || [ -z "$(ls -A "$W")" ] || { echo "regen: $W is not empty (REGEN_RESUME=1 continues a stopped run)" >&2; exit 1; }
W=$(cd "$W" && pwd)
if command -v cygpath >/dev/null 2>&1; then # Git Bash: native paths for the Go tools
  W=$(cygpath -m "$W")
  REPO=$(cygpath -m "$REPO")
fi
EXE=""
[ "$(go env GOOS)" = windows ] && EXE=.exe

ALL=(postgres bash c cpp csharp css dart golang html java javascript json php powershell python r ruby rust tsql tsx typescript xml yaml kotlin swift)
module_form=(-keep-module-form)
if [ -n "${ROOT_VERSION:-}" ]; then
  module_form=(-root-version "$ROOT_VERSION")
fi

LANGS=("$@")
if [ ${#LANGS[@]} -eq 0 ]; then
  LANGS=("${ALL[@]}")
fi

mkdir -p "$W/bin" "$W/dl" "$W/src" "$W/work"
GEN="$W/bin/gen$EXE"
if [ "$PHASE" = run ]; then
  [ -x "$GEN" ] || { echo "regen: REGEN_PHASE=run needs $GEN from the fetch phase" >&2; exit 1; }
else
  (cd "$REPO/tools/gen" && CGO_ENABLED=0 go build -trimpath -o "$GEN" .)
fi

"$GEN" -root "$REPO/tools/gen" -plan host >"$W/plan.tsv"
OUT_ROOT=grammars
rehearsal=()
if [ -n "${RUNTIME_REHEARSAL_PINS:-}" ]; then
  : "${RUNTIME_REHEARSAL_ARCHIVE:?}" "${RUNTIME_REHEARSAL_TREE:?}" "${RUNTIME_REHEARSAL_OUT:?}"
  cp "$RUNTIME_REHEARSAL_ARCHIVE" "$W/dl/"
  cp -R "$RUNTIME_REHEARSAL_TREE" "$W/src/"
  rehearsal=(-runtime-rehearsal "$RUNTIME_REHEARSAL_PINS")
  if [ -n "${RUNTIME_REHEARSAL_CLI_GZ:-}" ]; then
    : "${RUNTIME_REHEARSAL_CLI_SANDBOX:?a candidate CLI runs only through the sandbox wrapper}"
    rehearsal+=(-cli-sandbox "$RUNTIME_REHEARSAL_CLI_SANDBOX")
  fi
  module_form=()
  OUT_ROOT=$RUNTIME_REHEARSAL_OUT
fi
while IFS=$'\t' read -r kind file url sum dest mirrors; do
  case "$kind" in
    goinstall)
      # Built from the pinned module version; Go verifies it against the checksum database, and gen
      # compares the build info's go.sum hash with pins.json before using the binary.
      [ "$PHASE" = run ] || (cd "$W" && GOBIN="$W/bin" CGO_ENABLED=0 go install -trimpath "$file")
      ;;
    tool | source)
      fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors"
      [ "$PHASE" != fetch ] || continue
      rm -rf -- "${W:?}/${dest:?}" # a stopped run may have left a partial extraction (no-op in a new directory)
      mkdir -p "$W/$dest"
      (cd "$W" && tar -xzf "dl/$file" --strip-components=1 -C "$dest") # relative paths: GNU tar reads "D:/x" as a remote host
      ;;
    cli)
      if [ -n "${RUNTIME_REHEARSAL_CLI_GZ:-}" ]; then # the candidate replaces the pinned CLI (its hash is in the pins copy)
        cp "$RUNTIME_REHEARSAL_CLI_GZ" "$W/dl/$file"
      else
        fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors"
      fi
      [ "$PHASE" != fetch ] || continue
      gzip -dc "$W/dl/$file" >"$W/$dest"
      chmod +x "$W/$dest"
      ;;
    *) echo "regen: unknown plan line kind $kind" >&2; exit 1 ;;
  esac
done <"$W/plan.tsv"

if [ "$PHASE" = fetch ]; then
  # The npm lockfile inputs of the regenerate: true grammars are the one download the plan does not list; native-ref -stage
  # fetches and verifies them (and the other archives, already present) into the same dl directory and runs nothing.
  (cd "$REPO/tools/native-ref" && CGO_ENABLED=0 go build -trimpath -o "$W/bin/native-ref$EXE" .)
  "$W/bin/native-ref$EXE" -pins "$REPO/tools/gen/pins.json" -stage -dl "$W/dl" -work "$W/stage-work" "${LANGS[@]}"
  rm -rf -- "${W:?}/stage-work"
  exit 0
fi
fetch_npm=(-fetch-npm)
[ "$PHASE" != run ] || fetch_npm=() # offline: the npm archives are in dl/npm from the fetch phase
cd "$REPO"
failed=()
for lang in "${LANGS[@]}"; do
  echo "== $lang"
  # gen creates $W/work/<lang>/patches with os.Mkdir: the half-done directory of the stopped run must go (a resumed language runs from scratch)
  [ "${REGEN_RESUME:-}" != 1 ] || rm -rf -- "${W:?}/work/${lang:?}"
  rc=0
  "$GEN" -lang "$lang" -root tools/gen -src "$W/src" -dl "$W/dl" -sdk "$W/sdk" \
    -wasm2go "$W/bin/wasm2go$EXE" -wasm-opt "$W/binaryen/bin/wasm-opt$EXE" \
    -tree-sitter "$W/bin/tree-sitter$EXE" "${fetch_npm[@]}" -patch-alert "${PATCH_ALERT_THRESHOLD:-3}" \
    -out "$OUT_ROOT/$lang" -work "$W/work" -repo . "${rehearsal[@]}" "${module_form[@]}" || rc=$?
  if [ "$rc" -ne 0 ]; then
    [ -n "${REGEN_KEEP_GOING:-}" ] || exit "$rc"
    echo "regen: $lang failed (exit $rc)" >&2
    failed+=("$lang")
  fi
done
[ ${#failed[@]} -eq 0 ] || { echo "regen: failed: ${failed[*]}" >&2; exit 1; }
