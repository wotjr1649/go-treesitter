#!/usr/bin/env bash
# TEST-ONLY runtime rehearsal, generation half: build grammar modules against a tree-sitter runtime other than the pinned
# one, with every other pin unchanged, into a new directory outside grammars/. Nothing in the repository changes.
#
#   tools/gen/runtime-rehearsal.sh <new-empty-work-dir> <runtime-ref> [lang ...]
#
# <runtime-ref> is a tag of tree-sitter/tree-sitter (v0.27.3) or a full 40-hex commit (a release-0.27 head). RUNTIME_LABEL
# overrides the identity label (default: the tag, or rehearsal-<12 hex>). The runtime source is the same kind of archive the pin
# uses (GitHub API tarball of the commit); its sha256 and tree hash are computed here and written, with nothing else
# changed, into <work-dir>/pins.runtime.json by `gen runtime-pins`. The rest of the fetch and the generation are regen.sh
# (pinned CLI, wasm2go, wasi-sdk, grammar archives, all sha256-verified). Output: <work-dir>/out/<lang>/ (provenance.json
# says testOnly), <work-dir>/runtime-identity.json, <work-dir>/pins.runtime.json. No token is needed or used.
# The measuring half is tools/conformance/runtime-rehearsal.sh.
set -euo pipefail

W=${1:?usage: runtime-rehearsal.sh <new-empty-work-dir> <runtime-ref> [lang ...]}
REF=${2:?usage: runtime-rehearsal.sh <new-empty-work-dir> <runtime-ref> [lang ...]}
shift 2
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
mkdir -p "$W"
[ -z "$(ls -A "$W")" ] || { echo "runtime-rehearsal: $W is not empty" >&2; exit 1; }
W=$(cd "$W" && pwd)
if command -v cygpath >/dev/null 2>&1; then # Git Bash: native paths for the Go tools
  W=$(cygpath -m "$W")
  REPO=$(cygpath -m "$REPO")
fi
EXE=""
[ "$(go env GOOS)" = windows ] && EXE=.exe
UPSTREAM=tree-sitter/tree-sitter # the repo of the pinned runtime

if [[ "$REF" =~ ^[0-9a-f]{40}$ ]]; then
  COMMIT=$REF
  LABEL=${RUNTIME_LABEL:-rehearsal-${REF:0:12}}
elif [[ "$REF" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
  # annotated tags list the commit on the peeled line
  COMMIT=$(git ls-remote "https://github.com/$UPSTREAM" "refs/tags/$REF^{}" "refs/tags/$REF" | awk '/\^\{\}$/ { peeled = $1 } !/\^\{\}$/ { tag = $1 } END { print peeled ? peeled : tag }')
  LABEL=${RUNTIME_LABEL:-$REF}
else
  echo "runtime-rehearsal: runtime-ref must be a vX.Y.Z tag or a full commit sha" >&2
  exit 1
fi
[[ "$COMMIT" =~ ^[0-9a-f]{40}$ ]] || { echo "runtime-rehearsal: could not resolve $REF to a commit" >&2; exit 1; }

EXTRACT=tree-sitter-rehearsal-${COMMIT:0:12}
ARCHIVE=$EXTRACT.tar.gz
mkdir -p "$W/runtime/dl" "$W/runtime/src/$EXTRACT" "$W/bin"
fetch_archive "$W/runtime/dl/$ARCHIVE" "https://api.github.com/repos/$UPSTREAM/tarball/$COMMIT"
(cd "$W/runtime" && tar -xzf "dl/$ARCHIVE" --strip-components=1 -C "src/$EXTRACT") # relative paths: GNU tar reads "D:/x" as a remote host

(cd "$REPO/tools/gen" && CGO_ENABLED=0 go build -trimpath -o "$W/bin/gen$EXE" .)
"$W/bin/gen$EXE" runtime-pins -root "$REPO/tools/gen" -src "$W/runtime/src" -extract-dir "$EXTRACT" \
  -archive "$W/runtime/dl/$ARCHIVE" -release "$LABEL" -commit "$COMMIT" -out "$W/pins.runtime.json" >"$W/runtime-identity.json"
cat "$W/runtime-identity.json"

RUNTIME_REHEARSAL_PINS="$W/pins.runtime.json" RUNTIME_REHEARSAL_ARCHIVE="$W/runtime/dl/$ARCHIVE" \
  RUNTIME_REHEARSAL_TREE="$W/runtime/src/$EXTRACT" RUNTIME_REHEARSAL_OUT="$W/out" \
  bash "$REPO/tools/gen/regen.sh" "$W/regen" "$@"
