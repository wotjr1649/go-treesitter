#!/usr/bin/env bash
# Retry transient transport and HTTP errors; other HTTP, certificate and local write errors fail immediately.
fetch_archive() {
  local attempt code http_code
  for attempt in 1 2 3; do
    if http_code=$(curl -fsSL --connect-timeout 30 --max-time 600 --write-out '%{http_code}' -o "$1" "$2"); then
      return 0
    else
      code=$?
    fi
    case "$code" in
      5 | 6 | 7 | 18 | 28 | 52 | 55 | 56 | 92) ;;
      22) case "$http_code" in 408 | 429 | 5[0-9][0-9]) ;; *) break ;; esac ;;
      *) break ;;
    esac
    [ "$attempt" -lt 3 ] || break
    echo "fetch: curl error $code (HTTP $http_code); retry $attempt/2 in ${attempt}s" >&2
    sleep "$attempt"
  done
  rm -f -- "$1"
  return "$code"
}
