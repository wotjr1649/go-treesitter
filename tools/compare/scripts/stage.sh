#!/usr/bin/env bash
# Trusted staging step (network allowed, no third-party code executed): fetches and verifies everything the sandbox
# needs, using only this repository's Go code (native-ref -stage, benchci fetch) and 'go mod download'.
#
#   stage.sh WORKDIR LANG[,none]     LANG is a comparison language (c go javascript json python) or "none" (module cache only)
#
# Produces WORKDIR/{go,bin,src,dl,corpus,large,modcache,out}. A missing adapter go.sum switches that adapter to
# bootstrap mode: a copy resolves its module graph with 'go mod tidy' and the proposed go.mod/go.sum go to
# WORKDIR/out/go-sum-proposal/; the checked-in files are never modified and WORKDIR/BOOTSTRAP lists the adapters.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
. "$here/pins.env"
T=$(realpath -m "$1"); lang=$2

case "$(uname -m)" in x86_64) goarch=amd64;; aarch64|arm64) goarch=arm64;; *) echo "unsupported architecture" >&2; exit 1;; esac
want=GO_SHA256_linux_$goarch
mkdir -p "$T"/{bin,dl,corpus,large,modcache,out,stage-work}
chmod 777 "$T/out"

# Go distribution: the official archive, checked against the pinned checksum before it is unpacked.
curl -fsSL -o "$T/go.tgz" "https://go.dev/dl/go$GO_VERSION.linux-$goarch.tar.gz"
echo "${!want}  $T/go.tgz" | sha256sum -c -
tar -C "$T" -xzf "$T/go.tgz"
export PATH="$T/go/bin:$PATH" GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS= GOCACHE="$T/gocache" GOMODCACHE="$T/modcache" CGO_ENABLED=0
[ "$(go env GOVERSION)" = "go$GO_VERSION" ] || { echo "go version mismatch: $(go env GOVERSION)" >&2; exit 1; }

# The sandbox sees a copy of the tree, never the checkout (its credentials are removed by checkout, but the copy is explicit).
rm -rf "$T/src"; mkdir -p "$T/src"
(cd "$repo" && tar --exclude=.git -cf - .) | tar -C "$T/src" -xf -

# Trusted tools: this repository's code only.
(cd "$repo/tools/compare" && go build -trimpath -o "$T/bin/compare" ./cmd/compare)
(cd "$repo/tools/native-ref" && go build -trimpath -o "$T/bin/native-ref" . && go build -trimpath -o "$T/bin/ciconf" ./ciconf)
(cd "$repo/tools/bench" && go build -trimpath -o "$T/bin/benchci" ./ci)

# Unit tests of the orchestrator and the harness (stdlib only; RUN_UNIT=1 in one job per run).
if [ "${RUN_UNIT:-}" = 1 ]; then
  (cd "$repo/tools/compare" && go vet ./... && go test ./...)
  (cd "$repo/tools/compare/harness" && go vet ./... && go test ./...)
fi

# Module graphs of the adapters. Offline from here on: the sandbox gets the populated cache and GOPROXY=off.
: > "$T/BOOTSTRAP"
for a in $ADAPTERS; do
  d="$T/src/tools/compare/adapters/$a"
  cgo=0; case $a in official|smacker) cgo=1;; esac
  if [ ! -f "$d/go.sum" ]; then
    echo "$a: no go.sum -> bootstrap mode" | tee -a "$T/BOOTSTRAP"
    (cd "$d" && CGO_ENABLED=$cgo GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go mod tidy)
    mkdir -p "$T/out/go-sum-proposal/$a"
    cp "$d/go.mod" "$d/go.sum" "$T/out/go-sum-proposal/$a/"
  fi
  (cd "$d" && CGO_ENABLED=$cgo GOPROXY=https://proxy.golang.org GOFLAGS=-mod=readonly go mod download && go mod verify \
     && CGO_ENABLED=$cgo go list -deps ./... > /dev/null)
  if [ -f "$repo/tools/compare/adapters/$a/go.sum" ]; then # frozen: nothing may have changed
    cmp "$repo/tools/compare/adapters/$a/go.mod" "$d/go.mod" && cmp "$repo/tools/compare/adapters/$a/go.sum" "$d/go.sum" \
      || { echo "::error::$a: go.mod or go.sum changed during resolution" >&2; exit 1; }
  fi
done
[ -s "$T/BOOTSTRAP" ] || rm -f "$T/BOOTSTRAP"

[ "$lang" = none ] && exit 0
case $lang in go) pinlang=golang;; *) pinlang=$lang;; esac
# Pinned archives (runtime, wasi-libc, grammar, the CLI, npm inputs), all sha256-verified; the corpus is extracted.
(cd "$repo" && "$T/bin/native-ref" -pins tools/gen/pins.json -stage -dl "$T/dl" -work "$T/stage-work" -corpus "$T/corpus" "$pinlang")
(cd "$repo" && "$T/bin/benchci" fetch -manifest tools/bench/ci/large.json -data "$T/nodata" -out "$T/large" -langs "$pinlang")
echo "staged $lang under $T"
