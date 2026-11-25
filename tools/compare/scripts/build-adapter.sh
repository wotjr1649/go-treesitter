#!/usr/bin/env bash
# build-adapter.sh ADAPTER CGO: builds one adapter as the adapter's own user; the binary stays in its area (/work/ADAPTER/bin).
set -uo pipefail
a=$1; cgo=$2
cd "/src/tools/compare/adapters/$a" && CGO_ENABLED=$cgo go build -trimpath -buildvcs=false -ldflags="-s -w" -o "/work/$a/bin" .
