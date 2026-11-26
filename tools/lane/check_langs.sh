#!/usr/bin/env bash
# Fails unless it gets at least one grammar name and every name is a grammars/<name> directory. Works from any directory.
#
#   bash tools/lane/check_langs.sh $GROUP_LANGS
#
# A matrix entry that lost its grammar list would otherwise run nothing (a loop over an empty list passes) or everything (regen.sh and
# native-ref take all grammars when given none), and the job would still be green.
set -euo pipefail
cd "$(dirname "$0")/../.."

[ $# -gt 0 ] || { echo "::error::empty grammar list in the matrix entry"; exit 1; }
for lang in "$@"; do
  [[ $lang =~ ^[a-z0-9]+$ ]] && [ -d "grammars/$lang" ] || { echo "::error::unknown grammar in the matrix entry: '$lang'"; exit 1; }
done
echo "grammars: $*"
