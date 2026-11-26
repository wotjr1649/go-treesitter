#!/usr/bin/env bash
# Integrity record for the built native references that .github/workflows/conformance.yml caches.
#
#   cachesums.sh write <temp-dir>   record every file below <temp-dir>/{exe,corpus,queries,scanner-stages} and its sha256
#                                   in <temp-dir>/native-cache.sums (line 1: the temp dir, which the scanner stage manifests embed
#                                   as absolute paths; line 2: the run and commit that built them)
#   cachesums.sh check <temp-dir>   succeed only when the temp dir is the recorded one and the same files exist with the same sha256
set -euo pipefail

mode=${1:?usage: cachesums.sh write|check <temp-dir>}
t=${2:?usage: cachesums.sh write|check <temp-dir>}
cd "$t"
dirs=(exe corpus queries scanner-stages)
mkdir -p "${dirs[@]}"
list() { find "${dirs[@]}" -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum; }

case "$mode" in
  write)
    { echo "temp=$t"; echo "built by run ${GITHUB_RUN_ID:-local}, commit ${GITHUB_SHA:-local}"; list; } >native-cache.sums
    ;;
  check)
    [ "$(sed -n 1p native-cache.sums)" = "temp=$t" ] || { echo "cachesums: recorded for another temp dir"; exit 1; }
    diff <(tail -n +3 native-cache.sums) <(list) >/dev/null || { echo "cachesums: files differ from the record"; exit 1; }
    ;;
  *) echo "usage: cachesums.sh write|check <temp-dir>" >&2; exit 2 ;;
esac
