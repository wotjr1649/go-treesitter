#!/usr/bin/env bash
# TEST-ONLY runtime rehearsal, measuring half: parse the ciconf inputs with the committed grammar modules (base) and with the
# modules built by tools/gen/runtime-rehearsal.sh (candidate), then classify the per-input differences.
#
#   tools/conformance/runtime-rehearsal.sh <ciconf-data-dir> <gen-work-dir> <new-out-dir> <pins-key> ...
#
# <ciconf-data-dir> is a tools/native-ref/ciconf output (TSG_DATA_DIR); <gen-work-dir> the work dir of the generation half
# (it must hold out/<key>/ for every key; keys are pins.json names, so golang, not go). The candidate runs through a copy of
# tools/conformance/go.mod (go -modfile) whose replace lines point at those modules; the working tree, go.mod and grammars/
# are not changed. TSG_REHEARSAL_LARGE (optional) names a directory with large-<key>/<key>/* inputs.
# Output: <out>/{base.tsv,cand.tsv,summary.md,diff.tsv,cand.mod}; summary.md says INVALID and the script exits 1 when the two
# tables are not over the same inputs. Differences are reported, never a failure. Needs no native executable.
set -euo pipefail

DATA=${1:?usage: runtime-rehearsal.sh <ciconf-data-dir> <gen-work-dir> <new-out-dir> <pins-key> ...}
GEN=${2:?}
OUT=${3:?}
shift 3
[ $# -gt 0 ] || { echo "runtime-rehearsal: no grammar given" >&2; exit 1; }
REPO=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$OUT"
[ -z "$(ls -A "$OUT")" ] || { echo "runtime-rehearsal: $OUT is not empty" >&2; exit 1; }
abs() { (cd "$1" && pwd); }
DATA=$(abs "$DATA") GEN=$(abs "$GEN") OUT=$(abs "$OUT")
if command -v cygpath >/dev/null 2>&1; then
  DATA=$(cygpath -m "$DATA") GEN=$(cygpath -m "$GEN") OUT=$(cygpath -m "$OUT")
fi
export CGO_ENABLED=0 GOENV=off GOWORK=off GOTOOLCHAIN=${GOTOOLCHAIN:-local} TSG_DATA_DIR="$DATA"

# Link only the selected grammars (the table has rows for exactly these); package names of the tags are the conformance names.
langs="" tags=""
for key in "$@"; do
  [ -f "$GEN/out/$key/provenance.json" ] || { echo "runtime-rehearsal: no generated module $GEN/out/$key" >&2; exit 1; }
  name=$key
  [ "$key" != golang ] || name=go
  langs="${langs:+$langs,}$name"
done
for other in csharp go json python tsql tsx typescript javascript html css java c rust bash cpp yaml php powershell r dart xml ruby kotlin postgres swift; do
  case ",$langs," in *",$other,"*) ;; *) tags="${tags:+$tags,}no_$other" ;; esac
done
export TSG_S15_LANGS="$langs"

cd "$REPO/tools/conformance"
cp go.mod "$OUT/cand.mod"
[ ! -f go.sum ] || cp go.sum "$OUT/cand.sum" # none while the module has no external dependency
for key in "$@"; do
  go mod edit -replace "github.com/wotjr1649/go-treesitter/grammars/$key=$GEN/out/$key" "$OUT/cand.mod"
done
dump() { # <table> [go flags]
  local table=$1
  shift
  TSG_REHEARSAL_OUT="$table" go test -count=1 -timeout 120m -tags "$tags" "$@" -run '^TestRuntimeRehearsalDump$' .
}
dump "$OUT/base.tsv"
dump "$OUT/cand.tsv" -modfile "$OUT/cand.mod"
title="base: commit $(git -C "$REPO" rev-parse HEAD); candidate runtime: $(cat "$GEN/runtime-identity.json"); grammars: $*"
go run ./cmd/rehearsal-compare -base "$OUT/base.tsv" -cand "$OUT/cand.tsv" -out "$OUT" -title "$title"
