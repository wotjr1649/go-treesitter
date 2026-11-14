#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/fetch.sh"
work=$(mktemp -d)
trap 'rm -f -- "$work/calls" "$work/archive"; rmdir -- "$work"' EXIT

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
