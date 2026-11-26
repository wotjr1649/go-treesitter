#!/usr/bin/env bash
# REPORT-ONLY rehearsal: regenerate grammars with the tree-sitter `nightly` CLI and compare with the pinned CLI's output
# (CI: .github/workflows/nightly-cli-rehearsal.yml; design: ADR-0027, "Nightly rehearsal"). Not a gate, never edits grammars/.
#
#   tools/gen/nightly-rehearsal.sh <phase> <work-dir> [lang ...]     (needs: go, curl, gzip; gh for `nightly`; bwrap on Linux)
#     fetch    <work-dir> must not exist or be empty. Builds gen and fetches the pinned CLI and grammar archives,
#              SHA-256-verified exactly as regen.sh does. Needs no token.
#     nightly  Reads the `nightly` release metadata through gh (GH_TOKEN or a login) and downloads the asset; executes nothing.
#              Writes <work-dir>/nightly-meta.json: asset URL, API size and digest, and the tag commit before and after the download.
#     run      gen nightly-rehearsal: checks the common inputs (npm inputs fetched and verified), cross-checks the nightly
#              identity (API size/digest against the file, --version build commit against the tag; a mismatch holds the
#              comparison), then generates with both CLIs. Runs without any GitHub token in its environment. On Linux every
#              nightly execution goes through tools/gen/nightly-sandbox.sh (bubblewrap: no network, empty environment, the
#              call's inputs read-only, only its --output directory writable). Windows has no sandbox: `run` stops before
#              the nightly CLI runs unless NIGHTLY_REHEARSAL_UNSANDBOXED=1 (local opt-in; the report says "sandbox": "none").
#     all      fetch, nightly and run in order (local use).
# Output: <work-dir>/out/{report.json,summary.md,digest.txt,files/} (no digest.txt when the identity is held).
# CANDIDATE_CLI_VERSION=X.Y.Z (runtime + CLI pair rehearsal, tools/gen/cli-candidate.sh wrote the asset and metadata instead of the
# `nightly` phase): `run` compares the pinned CLI with that release's CLI and names it "candidate" in the summary.
set -euo pipefail

PHASE=${1:?usage: nightly-rehearsal.sh fetch|nightly|run|all <work-dir> [lang ...]}
W=${2:?usage: nightly-rehearsal.sh fetch|nightly|run|all <work-dir> [lang ...]}
shift 2
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
if [ "$PHASE" = fetch ] || [ "$PHASE" = all ]; then
  mkdir -p "$W"
  [ -z "$(ls -A "$W")" ] || { echo "nightly-rehearsal: $W is not empty" >&2; exit 1; }
fi
W=$(cd "$W" && pwd)
if command -v cygpath >/dev/null 2>&1; then # Git Bash: native paths for the Go tools
  W=$(cygpath -m "$W")
  REPO=$(cygpath -m "$REPO")
fi
EXE=""
case "$(go env GOOS)-$(go env GOARCH)" in
  linux-amd64) ASSET=tree-sitter-linux-x64.gz ;;
  windows-amd64) ASSET=tree-sitter-windows-x64.gz; EXE=.exe ;;
  *) echo "nightly-rehearsal: only linux-amd64 and windows-amd64 are supported" >&2; exit 1 ;;
esac
NIGHTLY_REPO=tree-sitter/tree-sitter
GEN="$W/bin/gen$EXE"

fetch_phase() {
  mkdir -p "$W/bin" "$W/dl" "$W/nightly"
  (cd "$REPO/tools/gen" && CGO_ENABLED=0 go build -trimpath -o "$GEN" .)
  # The same verified fetch as regen.sh, but only the CLI and the source archives (no wasi-sdk, no wasm2go).
  "$GEN" -root "$REPO/tools/gen" -plan host >"$W/plan.tsv"
  while IFS=$'\t' read -r kind file url sum dest mirrors; do
    case "$kind" in
      cli)
        fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors"
        gzip -dc "$W/dl/$file" >"$W/$dest"
        chmod +x "$W/$dest"
        ;;
      source) fetch_verified_archive "$W/dl/$file" "$sum" "$url" "$mirrors" ;;
    esac
  done <"$W/plan.tsv"
}

# Nothing to verify against, so record what the API says; gen cross-checks it against the bytes and --version.
# The tag moves with every nightly build; read it before and after.
nightly_phase() {
  local before after asset url
  before=$(gh api "repos/$NIGHTLY_REPO/commits/nightly" --jq .sha)
  asset=$(gh api "repos/$NIGHTLY_REPO/releases/tags/nightly" --jq \
    ".published_at as \$p | .assets[] | select(.name == \"$ASSET\") | {url: .browser_download_url, size: .size, apiDigest: .digest, publishedAt: \$p}")
  [ -n "$asset" ] || { echo "nightly-rehearsal: nightly release has no asset $ASSET" >&2; exit 1; }
  url=$(printf '%s' "$asset" | sed -n 's/.*"url":"\([^"]*\)".*/\1/p')
  case "$url" in https://github.com/$NIGHTLY_REPO/releases/download/nightly/*) ;; *) echo "nightly-rehearsal: unexpected asset URL $url" >&2; exit 1 ;; esac
  fetch_archive "$W/nightly/$ASSET" "$url"
  after=$(gh api "repos/$NIGHTLY_REPO/commits/nightly" --jq .sha)
  printf '%s\n' "${asset%\}},\"tagCommitBefore\":\"$before\",\"tagCommitAfter\":\"$after\",\"tagMoved\":$([ "$before" = "$after" ] && echo false || echo true)}" >"$W/nightly-meta.json"
}

run_phase() {
  local pinned sandbox=()
  pinned="$W/$(awk -F'\t' '$1 == "cli" { print $5 }' "$W/plan.tsv")"
  if [ "$(go env GOOS)" = linux ]; then
    command -v bwrap >/dev/null || { echo "nightly-rehearsal: bwrap (bubblewrap) is required on Linux" >&2; exit 1; }
    install -m 755 "$REPO/tools/gen/nightly-sandbox.sh" "$W/bin/nightly-sandbox" # gen executes it as the nightly CLI
    sandbox=(-nightly-sandbox "$W/bin/nightly-sandbox")
  elif [ "${NIGHTLY_REHEARSAL_UNSANDBOXED:-}" = 1 ]; then
    echo "nightly-rehearsal: no sandbox on $(go env GOOS): NIGHTLY_REHEARSAL_UNSANDBOXED=1 runs the nightly CLI directly, with your files" >&2
    sandbox=(-nightly-unsandboxed)
  else
    echo "nightly-rehearsal: no sandbox on $(go env GOOS); set NIGHTLY_REHEARSAL_UNSANDBOXED=1 to run the nightly CLI unisolated" >&2
    exit 1
  fi
  local candidate=()
  if [ -n "${CANDIDATE_CLI_VERSION:-}" ]; then
    candidate=(-label candidate -expect-version "$CANDIDATE_CLI_VERSION" -asset-prefix "https://github.com/$NIGHTLY_REPO/releases/download/v$CANDIDATE_CLI_VERSION/")
  fi
  cd "$REPO"
  # gen and the CLIs it starts never need a GitHub token (the `nightly` phase does, in `all` as well).
  env -u GH_TOKEN -u GITHUB_TOKEN -u GH_ENTERPRISE_TOKEN -u GITHUB_ENTERPRISE_TOKEN \
    "$GEN" nightly-rehearsal -root tools/gen -dl "$W/dl" -tree-sitter "$pinned" -nightly-gz "$W/nightly/$ASSET" \
    -nightly-meta "$W/nightly-meta.json" "${sandbox[@]}" "${candidate[@]}" -fetch-npm -out "$W/out" "$@"
}

case "$PHASE" in
  fetch) fetch_phase ;;
  nightly) nightly_phase ;;
  run) run_phase "$@" ;;
  all) fetch_phase; nightly_phase; run_phase "$@" ;;
  *) echo "nightly-rehearsal: unknown phase $PHASE" >&2; exit 1 ;;
esac
