#!/usr/bin/env bash
# Module step of staging (stage.sh) and its local check: the committed go.mod/go.sum are the approval, and the offline
# measuring steps (GOPROXY=off, measure.sh and the nocc build) must find every module file they need in the cache.
#   modgraph.sh IDENTITY_DIR     go on PATH and GOMODCACHE set; nothing from the module graph is executed
# Local check (needs network, no CI): GOOS=linux GOMODCACHE=<new empty dir> bash modgraph.sh <new empty dir>
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
. "$here/pins.env"
: "${GOMODCACHE:?set GOMODCACHE (an empty throwaway directory when checking locally)}"
idn=$(realpath -m "$1"); mkdir -p "$idn"
export GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly

# Online: download verifies the committed sums, 'go mod verify' re-hashes the cache, 'go list -deps' proves go.mod needs no
# change, and 'go list -m all' fetches the go.mod of every module of the full graph (download alone leaves those out
# when go.mod does not require them, and the offline steps then fail with "module lookup disabled"). The files must stay
# byte-identical. Offline: the same listing and package load must now succeed from the cache alone.
for a in $ADAPTERS; do
  d="$repo/tools/compare/adapters/$a"
  cgo=0; case $a in official|smacker) cgo=1;; esac
  (cd "$d" && export CGO_ENABLED=$cgo && GOPROXY=https://proxy.golang.org go mod download && go mod verify \
     && GOPROXY=https://proxy.golang.org go list -deps ./... > /dev/null \
     && GOPROXY=https://proxy.golang.org go list -m all > "$idn/modules-$a.txt")
  (cd "$repo" && git diff --exit-code -- "tools/compare/adapters/$a/go.mod" "tools/compare/adapters/$a/go.sum") \
    || { echo "::error::$a: go.mod or go.sum changed during resolution" >&2; exit 1; }
  (cd "$d" && export CGO_ENABLED=$cgo GOPROXY=off GOSUMDB=off && go list -m all | cmp - "$idn/modules-$a.txt" && go list -deps ./... > /dev/null) \
    || { echo "::error::$a: the staged cache is not enough for GOPROXY=off" >&2; exit 1; }
done
(cd "$repo/tools/compare/adapters" && sha256sum */go.mod */go.sum) > "$idn/gomod-gosum-sha256.txt"
