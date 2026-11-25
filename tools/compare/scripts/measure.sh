#!/usr/bin/env bash
# One measurement job on its ephemeral VM, as the runner user: native references, adapters, workload, timing, correctness.
# Candidates and the product run as this same user in this same VM (paired timing needs the same machine); the VM is the
# isolation boundary and the results of a malicious candidate could be falsified in its own job (tools/compare/README.md).
#   measure.sh WORKDIR LANG MODE [BUDGET_MIN]       LANG: c go javascript json python; MODE: smoke|full
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
. "$here/pins.env"
T=$(realpath -m "$1"); LANGK=$2; MODE=$3; BUDGET_MIN=${4:-20}
export PATH="$T/go/bin:$PATH" GOMODCACHE="$T/modcache" GOCACHE="$T/gocache" GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
       GOPROXY=off GOSUMDB=off GOTELEMETRY=off LC_ALL=C
out="$T/out"; mkdir -p "$out/provenance" "$out/failures" "$T/prov" "$T/bin-w" "$T/refs"
fatal() { printf '%s\n' "$*" > "$out/failures/fatal.txt"; echo "FATAL: $*" >&2; exit 1; }
fail() { printf '%s\n' "$1" > "$out/failures/$1.txt"; }
diag() { head -c 300 "$1" 2>/dev/null | tr -cd '[:print:]' | tr ',"' '__'; }
case $LANGK in go) pinlang=golang;; *) pinlang=$LANGK;; esac
{ gcc --version | head -1; go version; uname -sr; echo "mode=$MODE lang=$LANGK"; } > "$out/provenance/environment.txt" 2>&1
cd "$repo"

# ---- the product's native reference (native-ref with the comparison driver) and the ciconf conformance build
"$T/bin/native-ref" -pins tools/gen/pins.json -dl "$T/dl" -work "$T/nw-p" -out "$T/exe-product" -cc gcc \
    -driver "$repo/tools/compare/native/sexp_ref.c" "$pinlang" > "$T/prov/native-product.log" 2>&1 || fail native-product
"$T/bin/native-ref" -pins tools/gen/pins.json -dl "$T/dl" -work "$T/nw-c" -out "$T/exe-ciconf" -cc gcc \
    "$pinlang" > "$T/prov/native-ciconf.log" 2>&1 || fail native-ciconf
mkdir -p "$T/refs/product"
[ -f "$T/exe-product/$pinlang" ] && cp "$T/exe-product/$pinlang" "$T/refs/product/$LANGK" || fail native-product-missing
"$T/bin/ciconf" -pins tools/gen/pins.json -corpus "$T/corpus" -exe "$T/exe-ciconf" -out "$T/data" -lang "$LANGK" -seed 1 -j 3 \
    > "$T/prov/ciconf.log" 2>&1 || fail ciconf

# ---- candidate native references from each candidate's own vendored runtime and grammar C (its own cgo flags)
modl() { (cd "$repo/tools/compare/adapters/$1" && go list -m -f '{{.Dir}}' "$2"); }
drv="$repo/tools/compare/native/sexp_ref.c"; fn=tree_sitter_$LANGK
mkdir -p "$T/refs/official" "$T/refs/smacker" "$T/refs/gotreesitter" "$T/refs/bonsai" "$T/refs/malivvan"
build_official() {
  case $LANGK in c) g=tree-sitter-c;; go) g=tree-sitter-go;; javascript) g=tree-sitter-javascript;; json) g=tree-sitter-json;; python) g=tree-sitter-python;; esac
  off=$(modl official github.com/tree-sitter/go-tree-sitter) && gd=$(modl official "github.com/tree-sitter/$g") || return 1
  local srcs=("$off/src/lib.c" "$gd/src/parser.c"); [ -f "$gd/src/scanner.c" ] && srcs+=("$gd/src/scanner.c")
  gcc -std=c11 -O2 -fPIC -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE -I"$off/include" -I"$off/src" -I"$gd/src" -DTSG_LANG_FN=$fn "${srcs[@]}" "$drv" -o "$T/refs/official/$LANGK"
}
build_smacker() {
  case $LANGK in c) sub=c;; go) sub=golang;; javascript) sub=javascript;; python) sub=python;; *) echo "no $LANGK package"; return 2;; esac
  sm=$(modl smacker github.com/smacker/go-tree-sitter) || return 1
  local srcs=() f; for f in "$sm"/*.c; do [ "$(basename "$f")" = bindings.c ] || srcs+=("$f"); done
  srcs+=("$sm/$sub/parser.c"); [ -f "$sm/$sub/scanner.c" ] && srcs+=("$sm/$sub/scanner.c")
  gcc -O2 -fPIC -I"$sm" -I"$sm/$sub" -DTSG_API_H='"api.h"' -DTSG_LANG_FN=$fn "${srcs[@]}" "$drv" -o "$T/refs/smacker/$LANGK"
}
for a in official smacker; do
  build_$a > "$T/prov/native-$a.log" 2>&1 || echo "reference build did not produce a binary for $LANGK (see provenance/)" > "$T/refs/$a/$LANGK.unavailable"
done
echo "the Go tables' C-runtime source identity is not established by this implementation; diagnostic comparison only" > "$T/refs/gotreesitter/$LANGK.unavailable"
echo "the embedded wasm is built from build.env tags; regenerating its source tree is not implemented; diagnostic comparison only" > "$T/refs/bonsai/$LANGK.unavailable"
echo "the vendored C snapshot behind the embedded wasm is not rebuilt by this implementation; diagnostic comparison only" > "$T/refs/malivvan/$LANGK.unavailable"

# ---- adapters: the same flags for every one; the build record keeps only bounded diagnostics
echo "adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,diagnostics" > "$out/builds.csv"
build() { # name cgo baseline -> prints the size
  local a=$1 cgo=$2 base=${3:-0} t0=$SECONDS rc=0 sz=0
  (cd "$repo/tools/compare/adapters/$a" && CGO_ENABLED=$cgo go build -trimpath -buildvcs=false -ldflags="-s -w" -o "$T/bin-w/$a" .) > "$T/prov/build-$a.log" 2>&1 || rc=$?
  [ $rc -eq 0 ] && sz=$(stat -c %s "$T/bin-w/$a")
  local d; d=$(diag "$T/prov/build-$a.log")
  echo "$a,compiler-image cgo=$cgo,$([ $rc -eq 0 ] && echo ok || echo failed),$rc,$sz,$((sz>0 && base>0 ? sz-base : 0)),$((SECONDS-t0)),$d" >> "$out/builds.csv"
  [ $rc -eq 0 ] || echo "exit=$rc $d" > "$out/failures/build-$a.txt"
  echo $sz
}
empty=$(build empty 0 | tail -1)
for a in $ADAPTERS; do cgo=0; case $a in official|smacker) cgo=1;; esac; build "$a" $cgo "$empty" > /dev/null; done
rm -f "$T/bin-w/empty"

# ---- freeze the workload (the expected cells never depend on which builds succeeded), measure, check
"$T/bin/compare" select -data "$T/data" -large "$T/large" -large-manifest "$repo/tools/bench/ci/large.json" -out "$T/workload" -langs "$LANGK" \
    || fatal "select failed"
cp "$T/workload/workload.json" "$out/workload.json"
smoke=(); [ "$MODE" = smoke ] && smoke=(-smoke)
adapters=$(echo $ADAPTERS | tr ' ' ',')
taskset -c 2 "$T/bin/compare" run -workload "$T/workload" -bin "$T/bin-w" -out "$out" -langs "$LANGK" -adapters "$adapters" \
    -budget "${BUDGET_MIN}m" "${smoke[@]}" || fail run
"$T/bin/compare" check -workload "$T/workload" -bin "$T/bin-w" -refs "$T/refs" -out "$out" -langs "$LANGK" -adapters "$adapters" \
    -j 3 "${smoke[@]}" || fail check
for f in native-product native-ciconf ciconf native-official native-smacker; do
  [ -f "$T/prov/$f.log" ] && tail -n 40 "$T/prov/$f.log" | head -c 4000 | tr -cd '[:print:]\n' > "$out/provenance/$f.txt"
done
for f in "$T"/refs/*/$LANGK; do [ -f "$f" ] && echo "$(sha256sum "$f" | cut -d' ' -f1)  $(basename "$(dirname "$f")")/$LANGK"; done > "$out/provenance/reference-binaries.txt"
for f in "$T"/bin-w/*; do [ -f "$f" ] && echo "$(sha256sum "$f" | cut -d' ' -f1)  worker/$(basename "$f")"; done > "$out/provenance/worker-binaries.txt"
exit 0
