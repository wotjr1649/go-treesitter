#!/usr/bin/env bash
# Runs INSIDE the digest-pinned compiler container (no network, read-only root). The measurer (this script and
# compare) is root with a few capabilities; everything that compiles or runs third-party code runs as a per-adapter
# unprivileged user in that adapter's private directory under /work (see lib.sh, docker-run.sh). The measurer alone
# writes /out. Inputs (read-only): /src /trusted /dl /corpus /large /modcache /goroot.
# Environment: LANGK (c go javascript json python), MODE (smoke|full), BUDGET_MIN (minutes for the timing run).
set -uo pipefail
: "${LANGK:?}" "${MODE:?}"; BUDGET_MIN=${BUDGET_MIN:-20}
. /src/tools/compare/scripts/pins.env
. /src/tools/compare/scripts/lib.sh
umask 022
export PATH=/goroot/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOROOT=/goroot GOMODCACHE=/scratch/modcache GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
       GOPROXY=off GOSUMDB=off GOTELEMETRY=off LC_ALL=C
mkdir -p /out/provenance /out/failures /scratch/{bin,refs,prov}
fatal() { printf '%s\n' "$*" > /out/failures/fatal.txt; echo "FATAL: $*" >&2; exit 1; }
fail() { printf '%s\n' "$*" > "/out/failures/$1.txt"; }

# ---- inputs: fail fast when a staged input cannot be read or copied
for d in /src /trusted /dl /corpus /large /modcache /goroot; do [ -r "$d" ] || fatal "$d is not readable"; done
case $LANGK in go) pinlang=golang;; *) pinlang=$LANGK;; esac
cp -r /modcache /scratch/modcache || fatal "copying the module cache failed"
chmod -R a+rX /scratch/modcache
for a in $ADAPTERS empty; do mkwork "$a" || fatal "cannot create the work area of $a"; done
cp -r /dl /work/product/dl || fatal "copying the pinned archives failed"
chown -R "$(uidof product):$(uidof product)" /work/product/dl
for f in /work/product/dl/*; do asu product test -r "$f" || fatal "the sandbox user cannot read $f"; done
{ gcc --version | head -1; go version; uname -sr; echo "mode=$MODE lang=$LANGK"; } > /out/provenance/environment.txt 2>&1

# ---- the product's native reference (native-ref with the comparison driver) and the ciconf conformance build
cd /src
asu product /trusted/native-ref -pins tools/gen/pins.json -dl /work/product/dl -work /work/product/nw-p -out /work/product/exe-product \
    -cc gcc -driver /src/tools/compare/native/sexp_ref.c "$pinlang" > /scratch/prov/native-product.log 2>&1 || fail native-product
asu product /trusted/native-ref -pins tools/gen/pins.json -dl /work/product/dl -work /work/product/nw-c -out /work/product/exe-ciconf \
    -cc gcc "$pinlang" > /scratch/prov/native-ciconf.log 2>&1 || fail native-ciconf
mkdir -p /scratch/refs/product
take "/work/product/exe-product/$pinlang" "/scratch/refs/product/$LANGK" || fail native-product-missing
asu product /trusted/ciconf -pins tools/gen/pins.json -corpus /corpus -exe /work/product/exe-ciconf -out /work/product/data \
    -lang "$LANGK" -seed 1 -j 3 > /scratch/prov/ciconf.log 2>&1 || fail ciconf
[ -z "$(find /work/product/data -type l 2>/dev/null)" ] || fatal "ciconf output holds a symbolic link"
cp -r --no-dereference /work/product/data /scratch/data || fatal "copying the ciconf data failed"
chmod -R a+rX /scratch/data

# ---- candidate native references from each candidate's own vendored runtime and grammar C
for a in official smacker; do
  mkdir -p "/scratch/refs/$a"
  if asu $a bash /src/tools/compare/scripts/build-ref.sh $a "$LANGK" > "/scratch/prov/native-$a.log" 2>&1; then
    take "/work/$a/ref" "/scratch/refs/$a/$LANGK" || fail "native-$a-missing"
  else
    echo "reference build did not produce a binary for $LANGK (see failures/ and provenance/)" > "/scratch/refs/$a/$LANGK.unavailable"
  fi
done
for a in gotreesitter bonsai malivvan; do mkdir -p "/scratch/refs/$a"; done
echo "the Go tables' C-runtime source identity is not established by this implementation; diagnostic comparison only" > /scratch/refs/gotreesitter/$LANGK.unavailable
echo "the embedded wasm is built from build.env tags; regenerating its source tree is not implemented; diagnostic comparison only" > /scratch/refs/bonsai/$LANGK.unavailable
echo "the vendored C snapshot behind the embedded wasm is not rebuilt by this implementation; diagnostic comparison only" > /scratch/refs/malivvan/$LANGK.unavailable

# ---- adapters: built as their own users with the same flags; the build record keeps only bounded diagnostics
echo "adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,diagnostics" > /out/builds.csv
build() { # name cgo baseline -> prints the size
  local a=$1 cgo=$2 base=${3:-0} t0=$SECONDS rc=0 sz=0
  asu "$a" bash /src/tools/compare/scripts/build-adapter.sh "$a" "$cgo" > "/scratch/prov/build-$a.log" 2>&1 || rc=$?
  if [ $rc -eq 0 ]; then
    if take "/work/$a/bin" "/scratch/bin/$a"; then sz=$(stat -c %s "/scratch/bin/$a"); else rc=99; fi
  fi
  local d; d=$(diag "/scratch/prov/build-$a.log")
  echo "$a,cgo-image cgo=$cgo,$([ $rc -eq 0 ] && echo ok || echo failed),$rc,$sz,$((sz>0 && base>0 ? sz-base : 0)),$((SECONDS-t0)),$d" >> /out/builds.csv
  [ $rc -eq 0 ] || echo "exit=$rc $d" > "/out/failures/build-$a.txt"
  echo $sz
}
empty=$(build empty 0 | tail -1)
for a in $ADAPTERS; do cgo=0; case $a in official|smacker) cgo=1;; esac; build "$a" $cgo "$empty" > /dev/null; done
rm -f /scratch/bin/empty

# ---- freeze the workload (the expected cells never depend on which builds succeeded), measure, check
/trusted/compare select -data /scratch/data -large /large -large-manifest /src/tools/bench/ci/large.json -out /scratch/workload -langs "$LANGK" \
    || fatal "select failed"
chmod -R a+rX /scratch/workload /scratch/bin /scratch/refs
cp /scratch/workload/workload.json /out/workload.json
smoke=(); [ "$MODE" = smoke ] && smoke=(-smoke)
adapters=$(echo $ADAPTERS | tr ' ' ',')
taskset -c 2 /trusted/compare run -workload /scratch/workload -bin /scratch/bin -out /out -langs "$LANGK" -adapters "$adapters" \
    -uid-base $UID_BASE -work-root /work -budget "${BUDGET_MIN}m" "${smoke[@]}" || fail run
/trusted/compare check -workload /scratch/workload -bin /scratch/bin -refs /scratch/refs -out /out -langs "$LANGK" -adapters "$adapters" \
    -uid-base $UID_BASE -work-root /work -j 3 "${smoke[@]}" || fail check
for f in native-product native-ciconf ciconf native-official native-smacker; do
  [ -f /scratch/prov/$f.log ] && tail -n 40 /scratch/prov/$f.log | head -c 4000 | tr -cd '[:print:]\n' > /out/provenance/$f.txt
done
for f in /scratch/refs/*/$LANGK; do [ -f "$f" ] && echo "$(sha256sum "$f" | cut -d' ' -f1)  $(basename "$(dirname "$f")")/$LANGK"; done > /out/provenance/reference-binaries.txt
for f in /scratch/bin/*; do [ -f "$f" ] && echo "$(sha256sum "$f" | cut -d' ' -f1)  worker/$(basename "$f")"; done > /out/provenance/worker-binaries.txt
exit 0
