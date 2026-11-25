#!/usr/bin/env bash
# Runs INSIDE the digest-pinned image that has no C compiler: builds every adapter as a released consumer with
# CGO_ENABLED=0 and -mod=readonly (as the adapter's own unprivileged user), then parses a smoke fixture with each
# executable. Each row records one of four distinct outcomes:
#   ok                    built and the smoke parse returned status ok
#   expected-unsupported  a cgo adapter (official, smacker) that cannot build without a C compiler, as its API requires
#   build-failure         any other adapter that did not build
#   smoke-failure         built, but the smoke parse did not return status ok
# The trusted gate (compare gate-nocc) fails the job unless the product is "ok".
set -uo pipefail
. /src/tools/compare/scripts/pins.env
. /src/tools/compare/scripts/lib.sh
umask 022
export PATH=/goroot/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOROOT=/goroot GOMODCACHE=/scratch/modcache GOENV=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
       GOPROXY=off GOSUMDB=off GOTELEMETRY=off LC_ALL=C CGO_ENABLED=0
mkdir -p /out/provenance /out/failures /scratch/bin
fatal() { printf '%s\n' "$*" > /out/failures/fatal.txt; echo "FATAL: $*" >&2; exit 1; }
# The condition under test: no compiler, no C headers, no linker driver in this image.
present=""; for c in gcc cc c99 c89 clang tcc g++ c++ cpp ld; do command -v "$c" > /dev/null 2>&1 && present="$present $c"; done
for h in /usr/include/stdio.h /usr/include/stdlib.h; do [ -e "$h" ] && present="$present $h"; done
[ -z "$present" ] || fatal "image is not compiler-free:$present"
for d in /src /modcache /goroot; do [ -r "$d" ] || fatal "$d is not readable"; done
{ echo "no C toolchain found (checked gcc cc c99 c89 clang tcc g++ c++ cpp ld, stdio.h, stdlib.h)"; go version; uname -sr; } > /out/provenance/environment.txt 2>&1
cp -r /modcache /scratch/modcache || fatal "copying the module cache failed"
chmod -R a+rX /scratch/modcache
for a in $ADAPTERS empty; do mkwork "$a" || fatal "cannot create the work area of $a"; done
printf 'int main(void) { return 0; }\n' > /scratch/fixture.c; chmod a+r /scratch/fixture.c
echo "adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,smoke_parse,outcome,diagnostics" > /out/builds.csv
empty=0
for a in empty $ADAPTERS; do
  t0=$SECONDS; rc=0; sz=0; smoke=n/a; outcome=ok
  asu "$a" bash /src/tools/compare/scripts/build-adapter.sh "$a" 0 > "/scratch/b-$a.log" 2>&1 || rc=$?
  if [ $rc -eq 0 ] && take "/work/$a/bin" "/scratch/bin/$a"; then
    sz=$(stat -c %s "/scratch/bin/$a")
    if [ "$a" = empty ]; then empty=$sz
    else
      line=$(asu "$a" timeout 120 "/scratch/bin/$a" -lang c -op parse -mode time -src /scratch/fixture.c -reps 1 2>/dev/null | head -c 2000 | head -n 1)
      smoke=$(printf '%s' "$line" | sed -n 's/.*"status":"\([a-z]*\)".*/\1/p'); smoke=${smoke:-none}
      [ "$smoke" = ok ] || outcome=smoke-failure
    fi
  else
    [ $rc -eq 0 ] && rc=99
    case $a in official|smacker) outcome=expected-unsupported;; *) outcome=build-failure;; esac
  fi
  delta=0; [ "$a" != empty ] && [ "$sz" -gt 0 ] && [ "$empty" -gt 0 ] && delta=$((sz-empty))
  d=$(diag "/scratch/b-$a.log")
  echo "$a,no-c-compiler CGO_ENABLED=0,$([ $rc -eq 0 ] && echo ok || echo failed),$rc,$sz,$delta,$((SECONDS-t0)),$smoke,$outcome,$d" >> /out/builds.csv
  [ $rc -eq 0 ] || echo "exit=$rc outcome=$outcome $d" > "/out/failures/build-$a.txt"
done
exit 0
