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

# Both upstream and mirrors must satisfy the same archive pin before extraction.
# TSG_DL_CACHE (optional): a directory of archives named like their destinations. CI restores it with actions/cache and only
# pushes to main save it. A cached file is used only when its sha256 equals the pin after it was copied to the destination;
# a missing or mismatching file is downloaded as usual, and only a verified download is stored, so the cache never changes bytes.
fetch_verified_archive() {
  local dst=$1 sum=$2 upstream=$3 mirrors=${4:--} url cache=""
  local -a urls=("$upstream") fallback=()
  if [ -n "${TSG_DL_CACHE:-}" ]; then
    cache=$TSG_DL_CACHE/$(basename "$dst")
    if [ -f "$cache" ] && cp -- "$cache" "$dst" && echo "$sum  $dst" | sha256sum -c --status -; then return 0; fi
    rm -f -- "$dst"
  fi
  if [ "$mirrors" != - ]; then
    IFS=',' read -r -a fallback <<<"$mirrors"
    urls+=("${fallback[@]}")
  fi
  for url in "${urls[@]}"; do
    if fetch_archive "$dst" "$url"; then
      if echo "$sum  $dst" | sha256sum -c -; then
        if [ -n "$cache" ]; then # best effort: a read-only or full cache directory must not fail the fetch
          { mkdir -p -- "$TSG_DL_CACHE" && cp -- "$dst" "$cache.$$" && mv -f -- "$cache.$$" "$cache"; } || rm -f -- "$cache.$$"
        fi
        return 0
      fi
      echo "fetch: sha256 mismatch from $url" >&2
    fi
    rm -f -- "$dst"
    echo "fetch: $url failed; trying next pinned mirror if available" >&2
  done
  return 1
}
