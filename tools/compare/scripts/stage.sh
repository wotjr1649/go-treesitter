#!/usr/bin/env bash
# Trusted staging step (network allowed, no third-party code executed): fetches and verifies everything the sandbox
# needs, using only this repository's Go code (native-ref -stage, benchci fetch) and 'go mod download'.
#
#   stage.sh WORKDIR LANG|none            normal mode: needs a committed go.mod and go.sum for every adapter, else stops here
#   STAGE_MODE=bootstrap stage.sh WORKDIR none
#                                         proposal only: resolves each adapter's module graph on a copy and writes the proposed
#                                         go.mod/go.sum to WORKDIR/out/go-sum-proposal/; nothing is staged for a sandbox run
#
# Produces (normal mode) WORKDIR/{go,bin,src,dl,corpus,large,modcache,out,identity}. All staged files are made readable
# by every user, because the sandbox's candidate users are not the user that downloaded them.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
. "$here/pins.env"
T=$(realpath -m "$1"); lang=$2; mode=${STAGE_MODE:-normal}

# Normal mode runs nothing from a module graph that nobody approved: every adapter needs its committed files first.
if [ "$mode" = normal ]; then
  for a in $ADAPTERS; do
    for f in go.mod go.sum; do
      [ -s "$repo/tools/compare/adapters/$a/$f" ] || { echo "::error::tools/compare/adapters/$a/$f is not committed; run the workflow once with bootstrap=true, review the proposal and commit it" >&2; exit 1; }
    done
  done
fi

case "$(uname -m)" in x86_64) goarch=amd64;; aarch64|arm64) goarch=arm64;; *) echo "unsupported architecture" >&2; exit 1;; esac
want=GO_SHA256_linux_$goarch
mkdir -p "$T"/{bin,dl,corpus,large,modcache,out,stage-work,identity}
chmod 777 "$T/out"

# Go distribution: the official archive, checked against the pinned checksum before it is unpacked.
curl -fsSL -o "$T/go.tgz" "https://go.dev/dl/go$GO_VERSION.linux-$goarch.tar.gz"
echo "${!want}  $T/go.tgz" | sha256sum -c -
tar -C "$T" -xzf "$T/go.tgz"
export PATH="$T/go/bin:$PATH" GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS= GOCACHE="$T/gocache" GOMODCACHE="$T/modcache" CGO_ENABLED=0
[ "$(go env GOVERSION)" = "go$GO_VERSION" ] || { echo "go version mismatch: $(go env GOVERSION)" >&2; exit 1; }

# The sandbox sees a copy of the tree, never the checkout.
rm -rf "$T/src"; mkdir -p "$T/src"
(cd "$repo" && tar --exclude=.git -cf - .) | tar -C "$T/src" -xf -

# Trusted tools: this repository's code only.
(cd "$repo/tools/compare" && go build -trimpath -o "$T/bin/compare" ./cmd/compare)

if [ "$mode" = bootstrap ]; then
  # Proposal only: no candidate is built, run or measured. 'go mod tidy' needs the module sources but executes none.
  for a in $ADAPTERS; do
    d="$T/src/tools/compare/adapters/$a"
    cgo=0; case $a in official|smacker) cgo=1;; esac
    (cd "$d" && CGO_ENABLED=$cgo GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go mod tidy)
    mkdir -p "$T/out/go-sum-proposal/$a"
    cp "$d/go.mod" "$d/go.sum" "$T/out/go-sum-proposal/$a/"
  done
  exit 0
fi

(cd "$repo/tools/native-ref" && go build -trimpath -o "$T/bin/native-ref" . && go build -trimpath -o "$T/bin/ciconf" ./ciconf)
(cd "$repo/tools/bench" && go build -trimpath -o "$T/bin/benchci" ./ci)

# Unit tests of the orchestrator and the harness (stdlib only; RUN_UNIT=1 in one job per run).
if [ "${RUN_UNIT:-}" = 1 ]; then
  (cd "$repo/tools/compare" && go vet ./... && go test ./...)
  (cd "$repo/tools/compare/harness" && go vet ./... && go test ./...)
fi

# Module graphs of the adapters: the committed go.mod/go.sum are the approval. Download verifies them, 'go mod verify'
# re-hashes the cache, 'go list -deps' proves go.mod needs no change, and the files must still be byte-identical.
for a in $ADAPTERS; do
  d="$T/src/tools/compare/adapters/$a"
  cgo=0; case $a in official|smacker) cgo=1;; esac
  (cd "$d" && CGO_ENABLED=$cgo GOPROXY=https://proxy.golang.org GOFLAGS=-mod=readonly go mod download && go mod verify \
     && CGO_ENABLED=$cgo go list -deps ./... > /dev/null)
  cmp "$repo/tools/compare/adapters/$a/go.mod" "$d/go.mod" && cmp "$repo/tools/compare/adapters/$a/go.sum" "$d/go.sum" \
    || { echo "::error::$a: go.mod or go.sum changed during resolution" >&2; exit 1; }
  # identity of the resolved graph, kept with the results
  (cd "$d" && GOPROXY=off CGO_ENABLED=$cgo go list -m all) > "$T/identity/modules-$a.txt"
done
(cd "$repo/tools/compare/adapters" && sha256sum */go.mod */go.sum) > "$T/identity/gomod-gosum-sha256.txt"

[ "$lang" = none ] && { chmod -R a+rX "$T/modcache" "$T/identity"; exit 0; }
case $lang in go) pinlang=golang;; *) pinlang=$lang;; esac
# Pinned archives (runtime, wasi-libc, grammar, the CLI, npm inputs), all sha256-verified; the corpus is extracted.
(cd "$repo" && "$T/bin/native-ref" -pins tools/gen/pins.json -stage -dl "$T/dl" -work "$T/stage-work" -corpus "$T/corpus" "$pinlang")
(cd "$repo" && "$T/bin/benchci" fetch -manifest tools/bench/ci/large.json -data "$T/nodata" -out "$T/large" -langs "$pinlang")
# The sandbox's users are not this user: staged files must be readable (and directories searchable) by everyone.
chmod -R a+rX "$T/dl" "$T/corpus" "$T/large" "$T/modcache" "$T/bin" "$T/src" "$T/go" "$T/identity"
find "$T/dl" "$T/corpus" "$T/large" -type f ! -perm -o=r -print | grep . && { echo "::error::staged files are still unreadable" >&2; exit 1; } || true
echo "staged $lang under $T"
