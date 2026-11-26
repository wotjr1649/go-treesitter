#!/usr/bin/env bash
# Staging step of a job on its ephemeral VM: fetches and verifies the pinned inputs with this repository's own Go code
# (native-ref -stage, benchci fetch) and 'go mod download'. It runs before any candidate code does, but the VM, not this
# script, is the isolation boundary (tools/compare/README.md).
#
#   stage.sh WORKDIR LANG|none     normal mode: needs a committed go.mod and go.sum for every adapter, else stops here
#   STAGE_MODE=bootstrap stage.sh WORKDIR none
#                                  proposal only: resolves each adapter's module graph on a copy and writes the proposed
#                                  go.mod/go.sum to WORKDIR/out/go-sum-proposal/; nothing else is staged or run
#   STAGE_MODE=tools stage.sh WORKDIR none
#                                  installs the pinned Go and builds the 'compare' tool only (the aggregate job)
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

# Go distribution: the official archive, checked against the pinned checksum before it is unpacked.
curl -fsSL -o "$T/go.tgz" "https://go.dev/dl/go$GO_VERSION.linux-$goarch.tar.gz"
echo "${!want}  $T/go.tgz" | sha256sum -c -
tar -C "$T" -xzf "$T/go.tgz"
export PATH="$T/go/bin:$PATH" GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS= GOCACHE="$T/gocache" GOMODCACHE="$T/modcache" CGO_ENABLED=0
[ "$(go env GOVERSION)" = "go$GO_VERSION" ] || { echo "go version mismatch: $(go env GOVERSION)" >&2; exit 1; }

# The orchestrator: this repository's code, standard library plus the bench helper only.
(cd "$repo/tools/compare" && go build -trimpath -o "$T/bin/compare" ./cmd/compare)
[ "$mode" = tools ] && exit 0

if [ "$mode" = bootstrap ]; then
  # Proposal only: no candidate is built, run or measured. 'go mod tidy' needs the module sources but executes none.
  rm -rf "$T/boot"; mkdir -p "$T/boot"
  (cd "$repo" && tar --exclude=.git -cf - tools/compare tools/bench/proc) | tar -C "$T/boot" -xf -
  for a in $ADAPTERS; do
    d="$T/boot/tools/compare/adapters/$a"
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

# Module graphs of the adapters (modgraph.sh): verified downloads, the full graph's go.mod files, and an offline proof.
bash "$here/modgraph.sh" "$T/identity"

[ "$lang" = none ] && exit 0
case $lang in go) pinlang=golang;; *) pinlang=$lang;; esac
# Pinned archives (runtime, wasi-libc, grammar, the CLI, npm inputs), all sha256-verified; the corpus is extracted.
(cd "$repo" && "$T/bin/native-ref" -pins tools/gen/pins.json -stage -dl "$T/dl" -work "$T/stage-work" -corpus "$T/corpus" "$pinlang")
(cd "$repo" && "$T/bin/benchci" fetch -manifest tools/bench/ci/large.json -data "$T/nodata" -out "$T/large" -langs "$pinlang")
echo "staged $lang under $T"
