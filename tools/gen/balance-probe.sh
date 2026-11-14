#!/usr/bin/env bash
# Build test-only balancing-cancel modules and native references; never write grammars/.
# The fetch plan and archive verification match regen.sh. The output directory must be new or empty.
set -euo pipefail

W=${1:?usage: balance-probe.sh <new-empty-work-dir> [lang ...]}
shift
LANGS=("$@")
if [ ${#LANGS[@]} -eq 0 ]; then
  LANGS=(bash c cpp csharp css golang html java javascript json python rust tsql tsx typescript)
fi
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
mkdir -p "$W"
[ -z "$(ls -A "$W")" ] || { echo "balance-probe: $W is not empty" >&2; exit 1; }
W=$(cd "$W" && pwd)
if command -v cygpath >/dev/null 2>&1; then
  W=$(cygpath -m "$W")
  REPO=$(cygpath -m "$REPO")
fi
export CGO_ENABLED=0 GOWORK=off
EXE=""
[ "$(go env GOOS)" = windows ] && EXE=.exe
mkdir -p "$W/bin" "$W/dl" "$W/src" "$W/work"
(cd "$REPO/tools/gen" && go build -trimpath -o "$W/bin/gen$EXE" .)
GEN="$W/bin/gen$EXE"

"$GEN" -root "$REPO/tools/gen" -plan host >"$W/plan.tsv"
while IFS=$'\t' read -r kind file url sum dest; do
  case "$kind" in
    goinstall)
      (cd "$W" && GOBIN="$W/bin" go install -trimpath "$file")
      ;;
    tool | source)
      fetch_archive "$W/dl/$file" "$url"
      echo "$sum  $W/dl/$file" | sha256sum -c -
      mkdir -p "$W/$dest"
      (cd "$W" && tar -xzf "dl/$file" --strip-components=1 -C "$dest")
      ;;
    *) echo "balance-probe: unknown plan line kind $kind" >&2; exit 1 ;;
  esac
done <"$W/plan.tsv"

cd "$REPO"
sha256sum tools/gen/shim/tsg_abi.c >"$W/shim-before.sha256"
for lang in "${LANGS[@]}"; do
  echo "== test-only $lang"
  "$GEN" -balance-probe -lang "$lang" -root tools/gen -src "$W/src" -dl "$W/dl" -sdk "$W/sdk" \
    -wasm2go "$W/bin/wasm2go$EXE" -wasm-opt "$W/binaryen/bin/wasm-opt$EXE" \
    -out "$W/probes/$lang" -work "$W/work" -repo .
done
sha256sum -c "$W/shim-before.sha256"
(cd tools/native-ref && go build -trimpath -o "$W/bin/native-ref$EXE" .)
"$W/bin/native-ref$EXE" -pins tools/gen/pins.json -cc "${CC:-gcc}" \
  -dl "$W/dl" -work "$W/native-work" -out "$W/native" "${LANGS[@]}"
