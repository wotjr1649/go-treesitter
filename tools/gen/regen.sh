#!/usr/bin/env bash
# Regenerate grammar modules from the pinned upstream inputs on this host (CI: .github/workflows/regen.yml).
#
#   tools/gen/regen.sh <work-dir> [lang ...]
#
# <work-dir> must not exist or must be empty: every archive is downloaded and extracted only into it, its sha256 is
# checked before extraction. All non-exception grammars execute grammar JS with the pinned CLI's native
# JS runtime and a twice-generated determinism check (ADR-0027). The fetch plan
# (URLs, sha256, destinations) comes from tools/gen/pins.json via `gen -plan`; wasm2go is built from its pinned
# module version with `go install -trimpath`. Output goes to grammars/<lang> in this repository.
# Each selected grammar keeps the form and minimum root from its own go.mod. ROOT_VERSION=vX.Y.Z
# overrides that form only for the selected grammars (all grammars if no language list is given).
set -euo pipefail

W=${1:?usage: regen.sh <work-dir> [lang ...]}
shift || true
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
mkdir -p "$W"
[ -z "$(ls -A "$W")" ] || { echo "regen: $W is not empty" >&2; exit 1; }
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

mkdir -p "$W/bin" "$W/dl" "$W/src" "$W/work"
(cd "$REPO/tools/gen" && CGO_ENABLED=0 go build -trimpath -o "$W/bin/gen$EXE" .)
GEN="$W/bin/gen$EXE"

"$GEN" -root "$REPO/tools/gen" -plan host >"$W/plan.tsv"
while IFS=$'\t' read -r kind file url sum dest mirrors; do
  case "$kind" in
    goinstall)
      # Built from the pinned module version; Go verifies it against the checksum database, and gen
      # compares the build info's go.sum hash with pins.json before using the binary.
      (cd "$W" && GOBIN="$W/bin" CGO_ENABLED=0 go install -trimpath "$file")
      ;;
    tool | source)
      fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors"
      mkdir -p "$W/$dest"
      (cd "$W" && tar -xzf "dl/$file" --strip-components=1 -C "$dest") # relative paths: GNU tar reads "D:/x" as a remote host
      ;;
    cli)
      fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors"
      gzip -dc "$W/dl/$file" >"$W/$dest"
      chmod +x "$W/$dest"
      ;;
    *) echo "regen: unknown plan line kind $kind" >&2; exit 1 ;;
  esac
done <"$W/plan.tsv"

LANGS=("$@")
if [ ${#LANGS[@]} -eq 0 ]; then
  LANGS=("${ALL[@]}")
fi
cd "$REPO"
for lang in "${LANGS[@]}"; do
  echo "== $lang"
  "$GEN" -lang "$lang" -root tools/gen -src "$W/src" -dl "$W/dl" -sdk "$W/sdk" \
    -wasm2go "$W/bin/wasm2go$EXE" -wasm-opt "$W/binaryen/bin/wasm-opt$EXE" \
    -tree-sitter "$W/bin/tree-sitter$EXE" -fetch-npm -patch-alert "${PATCH_ALERT_THRESHOLD:-3}" \
    -out "grammars/$lang" -work "$W/work" -repo . "${module_form[@]}"
done
