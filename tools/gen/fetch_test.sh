#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/fetch.sh"
work=$(mktemp -d)
trap 'rm -rf -- "$work/cache"; rm -f -- "$work/calls" "$work/archive"; rmdir -- "$work"' EXIT

curl() {
  local call result
  call=$(cat "$work/calls")
  result=${codes[$call]}
  echo $((call + 1)) >"$work/calls"
  echo partial >"$work/archive"
  printf '%s' "${result#*:}"
  return "${result%%:*}"
}
sleep() { delays+="$1 "; }
check() {
  local want_status=$1 want_calls=$2 want_delays=$3 status calls
  shift 3
  codes=("$@") delays=""
  echo 0 >"$work/calls"
  if fetch_archive "$work/archive" https://example.invalid; then status=0; else status=$?; fi
  calls=$(cat "$work/calls")
  [ "$status" = "$want_status" ] && [ "$calls" = "$want_calls" ] && [ "$delays" = "$want_delays" ] || {
    echo "unexpected fetch status=$status calls=$calls delays=$delays" >&2
    exit 1
  }
  if [ "$status" = 0 ]; then [ -f "$work/archive" ]; else [ ! -e "$work/archive" ]; fi
}
check 0 1 "" 0:200
check 0 3 "1 2 " 28:000 56:000 0:200
check 7 3 "1 2 " 7:000 7:000 7:000
for http in 408 429 500 502 503 599; do
  check 0 2 "1 " "22:$http" 0:200
  check 22 3 "1 2 " "22:$http" "22:$http" "22:$http"
done
for http in 400 401 403 404 409 499; do check 22 1 "" "22:$http"; done
for code in 22 23 60; do check "$code" 1 "" "$code:000"; done
echo "fetch retry checks passed"

# Simulate blocked upstream and corrupt mirrors without depending on a real outage.
curl() {
  local url="${@: -1}"
  echo "$url" >>"$work/calls"
  case "$url" in
    https://upstream.invalid/blocked) printf 404; return 22 ;;
    https://mirror.invalid/corrupt) printf corrupt >"$work/archive" ;;
    *) printf 'pinned archive' >"$work/archive" ;;
  esac
  printf 200
}
sum=$(printf 'pinned archive' | sha256sum | cut -d' ' -f1)
: >"$work/calls"
fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/blocked \
  https://mirror.invalid/corrupt,https://mirror.invalid/good
[ "$(wc -l <"$work/calls")" = 3 ]
[ "$(cat "$work/archive")" = 'pinned archive' ]
: >"$work/calls"
fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/good https://mirror.invalid/good
[ "$(wc -l <"$work/calls")" = 1 ]
if fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/blocked https://mirror.invalid/corrupt; then
  echo "accepted corrupt mirror" >&2; exit 1
fi
[ ! -e "$work/archive" ]
echo "fetch mirror checks passed"

# TSG_DL_CACHE: a hit needs no download, a wrong cached file is replaced by a verified download, and a corrupt download is never stored.
export TSG_DL_CACHE="$work/cache"
: >"$work/calls"
fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/good
[ "$(wc -l <"$work/calls")" = 1 ] && [ "$(cat "$work/cache/archive")" = 'pinned archive' ]
rm -f -- "$work/archive"
: >"$work/calls"
fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/blocked
[ ! -s "$work/calls" ] && [ "$(cat "$work/archive")" = 'pinned archive' ]
printf 'poisoned' >"$work/cache/archive"
rm -f -- "$work/archive"
: >"$work/calls"
fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/good
[ "$(wc -l <"$work/calls")" = 1 ] && [ "$(cat "$work/archive")" = 'pinned archive' ] && [ "$(cat "$work/cache/archive")" = 'pinned archive' ]
rm -rf -- "$work/cache"
if fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/blocked https://mirror.invalid/corrupt; then
  echo "accepted corrupt mirror with cache" >&2; exit 1
fi
[ ! -e "$work/archive" ] && [ ! -e "$work/cache/archive" ]
unset TSG_DL_CACHE
echo "fetch cache checks passed"

# REGEN_RESUME=1 keeps a file that matches its pin and downloads any other one; without it nothing is kept.
resume() { # <want calls> <want content> <REGEN_RESUME value or empty> <file content or - for none>
  : >"$work/calls"
  if [ "$4" = - ]; then rm -f -- "$work/archive"; else printf '%s' "$4" >"$work/archive"; fi
  REGEN_RESUME=$3 fetch_verified_archive "$work/archive" "$sum" https://upstream.invalid/good -
  [ "$(wc -l <"$work/calls")" = "$1" ] && [ "$(cat "$work/archive")" = "$2" ] || {
    echo "unexpected resume: calls=$(wc -l <"$work/calls") content=$(cat "$work/archive")" >&2
    exit 1
  }
}
resume 0 'pinned archive' 1 'pinned archive' # all files valid: nothing downloaded
resume 1 'pinned archive' 1 corrupt          # one corrupt file: downloaded again and verified
resume 1 'pinned archive' 1 ''               # truncated to nothing
resume 1 'pinned archive' 1 -                # missing
resume 1 'pinned archive' '' 'pinned archive' # without REGEN_RESUME a valid file is still downloaded again
resume 1 'pinned archive' 0 'pinned archive'  # only the value 1 resumes
echo "fetch resume checks passed"

# regen.sh: a non-empty work directory is refused unless REGEN_RESUME=1. A stub go builds a stub gen, which stops the script
# with 77 on its first language (78 if that language's work directory exists), so "accepted" means exit 77 and nothing is
# downloaded or built.
bindir=$(mktemp -d) wd=$(mktemp -d)
trap 'rm -rf -- "$bindir" "$wd"; rm -f -- "$work/calls" "$work/archive"; rmdir -- "$work"' EXIT
printf '#!/usr/bin/env bash\ncase "$1" in env) echo linux ;; build) cp "%s" "$4" ;; *) exit 99 ;; esac\n' "$bindir/gen-stub" >"$bindir/go"
cat >"$bindir/gen-stub" <<'EOF'
#!/usr/bin/env bash
# gen -plan prints nothing; gen -lang L -work D creates D/L like gen does (os.Mkdir) and stops
for a in "$@"; do [ "$a" != -plan ] || exit 0; done
while [ $# -gt 0 ]; do case "$1" in -lang) lang=$2 ;; -work) work=$2 ;; esac; shift; done
[ ! -e "$work/$lang" ] || exit 78
mkdir "$work/$lang"
exit 77
EOF
chmod +x "$bindir/go" "$bindir/gen-stub"
echo leftover >"$wd/stopped-run"
regen() { PATH="$bindir:$PATH" REGEN_RESUME=$1 bash "$(dirname "$0")/regen.sh" "$wd" "${@:2}" >"$work/regen.out" 2>&1 && return 0 || return $?; }
check_regen() { # <want exit> <REGEN_RESUME value> [lang ...]
  local status
  if regen "$2" "${@:3}"; then status=0; else status=$?; fi
  [ "$status" = "$1" ] || { echo "regen.sh REGEN_RESUME='$2': exit $status, want $1" >&2; cat "$work/regen.out" >&2; exit 1; }
}
check_regen 1 ''
check_regen 1 yes
[ "$(ls -A "$wd")" = stopped-run ] # the refused runs created nothing
check_regen 77 1
mkdir -p "$wd/work/kotlin/patches" # a language stopped half-way: the resumed run starts it from scratch
check_regen 77 1 kotlin
rm -f -- "$work/regen.out"
echo "regen resume checks passed"
