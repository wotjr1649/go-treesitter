#!/usr/bin/env bash
# TEST ONLY. Soundness check of the shim's xfields_may_differ pre-check (tools/gen/shim/tsg_abi.c) for the runtime
# in tools/gen/pins.json: build each language twice (product module, and a variant made with gen -field-full-compare,
# which compares EVERY node with children against ts_node_child_by_field_id) and require identical field results.
#
#   field-precheck-soundness.sh <new-empty-out-dir> <ciconf-data-dir> <lang>...
#   env: GEN (gen executable), SRC (-src dir), DL (-dl dir), SDK, WASM2GO, WASM_OPT, TS_CLI (the pinned CLI); the same
#        verified inputs tools/gen/regen.sh prepares. REPO defaults to the repository root.
# Exit 0: no difference. Exit 1: a difference (the digest files stay in <out-dir>). Never writes inside grammars/.
set -euo pipefail
OUT=${1:?out dir}; DATA=${2:?data dir}; shift 2
[ $# -gt 0 ] || { echo "no languages" >&2; exit 2; }
REPO=${REPO:-$(cd "$(dirname "$0")/../.." && pwd)}
command -v cygpath >/dev/null 2>&1 && REPO=$(cygpath -m "$REPO") # Git Bash: Go needs native paths
: "${GEN:?}" "${SRC:?}" "${DL:?}" "${SDK:?}" "${WASM2GO:?}" "${WASM_OPT:?}" "${TS_CLI:?}"
mkdir -p "$OUT"; [ -z "$(ls -A "$OUT")" ] || { echo "$OUT is not empty" >&2; exit 2; }
OUT=$(cd "$OUT" && pwd)
command -v cygpath >/dev/null 2>&1 && OUT=$(cygpath -m "$OUT")
cp "$REPO/tools/conformance/go.mod" "$OUT/full.mod"; [ ! -f "$REPO/tools/conformance/go.sum" ] || cp "$REPO/tools/conformance/go.sum" "$OUT/full.sum"
langs=$(IFS=,; echo "$*" | sed 's/\bgolang\b/go/') # the conformance lane names the Go grammar "go"
for lang in "$@"; do
  "$GEN" -lang "$lang" -root "$REPO/tools/gen" -src "$SRC" -dl "$DL" -sdk "$SDK" -wasm2go "$WASM2GO" -wasm-opt "$WASM_OPT" \
    -tree-sitter "$TS_CLI" -fetch-npm -repo "$REPO" -out "$OUT/full/$lang" -work "$OUT/work/$lang" -field-full-compare
  grep -q '"testOnly": "field-full-compare"' "$OUT/full/$lang/provenance.json" || { echo "variant $lang lacks the testOnly mark" >&2; exit 2; }
  sed -i "s#\(grammars/$lang => \).*#\1$OUT/full/$lang#" "$OUT/full.mod" # point this grammar at the variant
done
run() { # <modfile flag or empty> <digest file>
  (cd "$REPO/tools/conformance" && CGO_ENABLED=0 TSG_DATA_DIR="$DATA" TSG_S15_LANGS="$langs" TSG_FIELDS_DIGEST_OUT="$2" \
    go test -count=1 -run '^TestFieldsDigest$' $1 .)
}
run "" "$OUT/digest-product.tsv"
run "-modfile=$OUT/full.mod" "$OUT/digest-full.tsv"
if diff -q "$OUT/digest-product.tsv" "$OUT/digest-full.tsv" >/dev/null; then
  echo "field-precheck-soundness: identical field results ($(wc -l <"$OUT/digest-product.tsv") inputs)"
else
  echo "field-precheck-soundness: DIFFERENT field results; the pre-check skipped a place where C differs" >&2
  diff "$OUT/digest-product.tsv" "$OUT/digest-full.tsv" | head -20 >&2
  exit 1
fi
