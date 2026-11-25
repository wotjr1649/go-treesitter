#!/usr/bin/env bash
# Runs INSIDE the digest-pinned compiler container (no network, read-only root, unprivileged). Everything that
# compiles or executes third-party code happens here: native references, the adapters, the workers.
# Inputs (read-only): /src /trusted /dl /corpus /large /modcache /goroot. Result volume: /out (flat, allowlisted names).
# Environment: LANGK (c go javascript json python), MODE (smoke|full), BUDGET_MIN (minutes for the timing run).
set -uo pipefail
: "${LANGK:?}" "${MODE:?}"; BUDGET_MIN=${BUDGET_MIN:-20}
. /src/tools/compare/scripts/pins.env
export PATH=/goroot/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export HOME=/scratch/home GOROOT=/goroot GOCACHE=/scratch/gocache GOMODCACHE=/scratch/modcache GOENV=off GOWORK=off \
       GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off GOTELEMETRY=off LC_ALL=C
mkdir -p "$HOME" /scratch/{bin,refs,prov} /out/provenance /out/failures
cp -r /modcache /scratch/modcache && chmod -R u+w /scratch/modcache
cp -r /dl /scratch/dl
case $LANGK in go) pinlang=golang;; *) pinlang=$LANGK;; esac
smoke=(); [ "$MODE" = smoke ] && smoke=(-smoke)
{ gcc --version | head -1; go version; uname -sr; echo "mode=$MODE lang=$LANGK"; } > /out/provenance/environment.txt 2>&1
fail() { echo "$*" > "/out/failures/$1.txt"; }

# ---- native references (the product's, through native-ref with the comparison driver; and the ciconf conformance build)
cd /src
/trusted/native-ref -pins tools/gen/pins.json -dl /scratch/dl -work /scratch/nw-p -out /scratch/exe-product -cc gcc \
    -driver /src/tools/compare/native/sexp_ref.c "$pinlang" > /scratch/prov/native-product.log 2>&1 || fail native-product
/trusted/native-ref -pins tools/gen/pins.json -dl /scratch/dl -work /scratch/nw-c -out /scratch/exe-ciconf -cc gcc \
    "$pinlang" > /scratch/prov/native-ciconf.log 2>&1 || fail native-ciconf
mkdir -p /scratch/refs/product
[ -f "/scratch/exe-product/$pinlang" ] && cp "/scratch/exe-product/$pinlang" "/scratch/refs/product/$LANGK"
/trusted/ciconf -pins tools/gen/pins.json -corpus /corpus -exe /scratch/exe-ciconf -out /scratch/data -lang "$LANGK" -seed 1 -j 3 \
    > /scratch/prov/ciconf.log 2>&1 || fail ciconf

# ---- candidate native references, built from each candidate's own vendored runtime and grammar C (its cgo flags)
modl() { (cd "/src/tools/compare/adapters/$1" && go list -m -f '{{.Dir}}' "$2"); }
mkdir -p /scratch/refs/official /scratch/refs/smacker /scratch/refs/gotreesitter /scratch/refs/bonsai /scratch/refs/malivvan
case $LANGK in c) gmod=tree-sitter-c; sub=c;; go) gmod=tree-sitter-go; sub=golang;; javascript) gmod=tree-sitter-javascript; sub=javascript;;
               json) gmod=tree-sitter-json; sub=;; python) gmod=tree-sitter-python; sub=python;; esac
fn=tree_sitter_$LANGK
off=$(modl official github.com/tree-sitter/go-tree-sitter) && gd=$(modl official "github.com/tree-sitter/$gmod") && {
  srcs=("$off/src/lib.c" "$gd/src/parser.c"); [ -f "$gd/src/scanner.c" ] && srcs+=("$gd/src/scanner.c")
  gcc -std=c11 -O2 -fPIC -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE -I"$off/include" -I"$off/src" -I"$gd/src" -DTSG_LANG_FN=$fn \
      "${srcs[@]}" /src/tools/compare/native/sexp_ref.c -o /scratch/refs/official/$LANGK > /scratch/prov/native-official.log 2>&1
} || { echo "official reference build failed (see failures/)" > /scratch/refs/official/$LANGK.unavailable; fail native-official; }
if [ -n "$sub" ] && sm=$(modl smacker github.com/smacker/go-tree-sitter); then
  srcs=(); for f in "$sm"/*.c; do [ "$(basename "$f")" = bindings.c ] || srcs+=("$f"); done
  srcs+=("$sm/$sub/parser.c"); [ -f "$sm/$sub/scanner.c" ] && srcs+=("$sm/$sub/scanner.c")
  gcc -O2 -fPIC -I"$sm" -I"$sm/$sub" -DTSG_API_H='"api.h"' -DTSG_LANG_FN=$fn "${srcs[@]}" /src/tools/compare/native/sexp_ref.c \
      -o /scratch/refs/smacker/$LANGK > /scratch/prov/native-smacker.log 2>&1 \
    || { echo "smacker reference build failed (see failures/)" > /scratch/refs/smacker/$LANGK.unavailable; fail native-smacker; }
else
  echo "the module bundles no $LANGK grammar package" > /scratch/refs/smacker/$LANGK.unavailable
fi
echo "the Go tables' C-runtime source identity is not established by this implementation; diagnostic comparison only" > /scratch/refs/gotreesitter/$LANGK.unavailable
echo "the embedded wasm is built from build.env tags; regenerating its source tree is not implemented; diagnostic comparison only" > /scratch/refs/bonsai/$LANGK.unavailable
echo "the vendored C snapshot behind the embedded wasm is not rebuilt by this implementation; diagnostic comparison only" > /scratch/refs/malivvan/$LANGK.unavailable

# ---- adapters (one binary each; same flags for every one) and the build record
echo "adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,diagnostics" > /out/builds.csv
build() { # name cgo
  local a=$1 cgo=$2 t0=$SECONDS rc=0 sz=0 base=${3:-0}
  (cd "/src/tools/compare/adapters/$a" && CGO_ENABLED=$cgo go build -trimpath -buildvcs=false -ldflags="-s -w" -o "/scratch/bin/$a" . ) > "/scratch/prov/build-$a.log" 2>&1 || rc=$?
  [ $rc -eq 0 ] && sz=$(stat -c %s "/scratch/bin/$a")
  local diag; diag=$(head -c 300 "/scratch/prov/build-$a.log" | tr '\n,"' '  _')
  echo "$a,cgo-image cgo=$cgo,$([ $rc -eq 0 ] && echo ok || echo failed),$rc,$sz,$((sz>0 && base>0 ? sz-base : 0)),$((SECONDS-t0)),$diag" >> /out/builds.csv
  [ $rc -eq 0 ] || cp "/scratch/prov/build-$a.log" "/out/failures/build-$a.txt"
  echo $sz
}
empty=$(build empty 0 | tail -1)
have=(); for a in $ADAPTERS; do cgo=0; case $a in official|smacker) cgo=1;; esac
  build $a $cgo "$empty" > /dev/null; [ -x "/scratch/bin/$a" ] && have+=("$a"); done
rm -f /scratch/bin/empty

# ---- freeze the workload, measure, check
/trusted/compare select -data /scratch/data -large /large -large-manifest /src/tools/bench/ci/large.json -out /scratch/workload -langs "$LANGK" \
    || { fail select; exit 0; }
cp /scratch/workload/workload.json /out/workload.json
IFS=,; adapters="${have[*]}"; unset IFS
taskset -c 2 /trusted/compare run -workload /scratch/workload -bin /scratch/bin -out /out -langs "$LANGK" -adapters "$adapters" \
    -budget "${BUDGET_MIN}m" "${smoke[@]}" || fail run
/trusted/compare check -workload /scratch/workload -bin /scratch/bin -refs /scratch/refs -out /out -langs "$LANGK" -adapters "$adapters" \
    -j 3 "${smoke[@]}" || fail check
for f in native-product native-ciconf ciconf native-official native-smacker; do
  [ -f /scratch/prov/$f.log ] && tail -n 40 /scratch/prov/$f.log | head -c 4000 > /out/provenance/$f.txt
done
for f in /scratch/refs/*/$LANGK; do [ -f "$f" ] && echo "$(sha256sum "$f" | cut -d' ' -f1)  $(basename "$(dirname "$f")")/$LANGK"; done > /out/provenance/reference-binaries.txt
exit 0
