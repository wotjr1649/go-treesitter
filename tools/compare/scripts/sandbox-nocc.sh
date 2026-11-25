#!/usr/bin/env bash
# Runs INSIDE the digest-pinned image that has no C compiler: builds every adapter as a released consumer with
# CGO_ENABLED=0 and -mod=readonly, then parses a smoke fixture with each resulting executable.
# Expected: the pure-Go adapters build and parse; the cgo adapters fail with bounded diagnostics. Both are recorded.
set -uo pipefail
. /src/tools/compare/scripts/pins.env
export PATH=/goroot/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export HOME=/scratch/home GOROOT=/goroot GOCACHE=/scratch/gocache GOMODCACHE=/scratch/modcache GOENV=off GOWORK=off \
       GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off GOTELEMETRY=off LC_ALL=C CGO_ENABLED=0
mkdir -p "$HOME" /scratch/bin /out/provenance /out/failures
# The condition under test: no compiler, no C headers, no linker driver in this image.
present=""; for c in gcc cc c99 c89 clang tcc g++ c++ cpp ld; do command -v "$c" > /dev/null 2>&1 && present="$present $c"; done
for h in /usr/include/stdio.h /usr/include/stdlib.h; do [ -e "$h" ] && present="$present $h"; done
if [ -n "$present" ]; then echo "image is not compiler-free:$present" > /out/failures/image.txt; exit 0; fi
{ echo "no C toolchain found (checked gcc cc c99 c89 clang tcc g++ c++ cpp ld, stdio.h, stdlib.h)"; go version; uname -sr; } > /out/provenance/environment.txt 2>&1
cp -r /modcache /scratch/modcache && chmod -R u+w /scratch/modcache
printf 'int main(void) { return 0; }\n' > /scratch/fixture.c
echo "adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,smoke_parse,diagnostics" > /out/builds.csv
empty=0
for a in empty $ADAPTERS; do
  t0=$SECONDS; rc=0; sz=0; smoke=n/a
  (cd "/src/tools/compare/adapters/$a" && go build -trimpath -buildvcs=false -ldflags="-s -w" -o "/scratch/bin/$a" .) > "/scratch/b-$a.log" 2>&1 || rc=$?
  if [ $rc -eq 0 ]; then
    sz=$(stat -c %s "/scratch/bin/$a")
    if [ "$a" = empty ]; then empty=$sz
    else smoke=$(timeout 120 "/scratch/bin/$a" -lang c -op parse -mode time -src /scratch/fixture.c -reps 1 2>/dev/null | head -c 2000 | sed -n 's/.*"status":"\([a-z]*\)".*/\1/p'); smoke=${smoke:-none}; fi
  fi
  delta=0; [ "$a" != empty ] && [ "$sz" -gt 0 ] && [ "$empty" -gt 0 ] && delta=$((sz-empty))
  diag=$(head -c 300 "/scratch/b-$a.log" | tr '\n,"' '  _')
  echo "$a,no-c-compiler CGO_ENABLED=0,$([ $rc -eq 0 ] && echo ok || echo failed),$rc,$sz,$delta,$((SECONDS-t0)),$smoke,$diag" >> /out/builds.csv
done
exit 0
