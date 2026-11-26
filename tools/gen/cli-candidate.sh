#!/usr/bin/env bash
# Download a candidate tree-sitter CLI release asset and record the release API facts, for the runtime + CLI pair rehearsal.
# Executes nothing and needs no token (anonymous GitHub API and git, a public repository); jq is needed.
#
#   tools/gen/cli-candidate.sh <work-dir> <X.Y.Z>
#
# Writes <work-dir>/nightly/<asset> and <work-dir>/nightly-meta.json, the files tools/gen/nightly-rehearsal.sh `run` reads for
# its unpinned CLI (so the CLI-only report, `gen nightly-rehearsal`, checks them exactly as it does the nightly asset: API size
# and digest against the bytes, the tag read before and after the download). <work-dir> is the one nightly-rehearsal.sh fetch
# prepared. Run the report with CANDIDATE_CLI_VERSION=<X.Y.Z> so it expects this release (no build commit in a release's --version).
set -euo pipefail

W=${1:?usage: cli-candidate.sh <work-dir> <X.Y.Z>}
VERSION=${2:?usage: cli-candidate.sh <work-dir> <X.Y.Z>}
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "cli-candidate: $VERSION is not X.Y.Z" >&2; exit 1; }
REPO=$(cd "$(dirname "$0")/../.." && pwd)
source "$REPO/tools/gen/fetch.sh"
UPSTREAM=tree-sitter/tree-sitter
TAG=v$VERSION
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) ASSET=tree-sitter-linux-x64.gz ;;
  *) echo "cli-candidate: only linux-x86_64 is supported" >&2; exit 1 ;;
esac
[ -d "$W" ] || { echo "cli-candidate: $W is missing (run nightly-rehearsal.sh fetch first)" >&2; exit 1; }
mkdir -p "$W/nightly"
[ ! -e "$W/nightly/$ASSET" ] && [ ! -e "$W/nightly-meta.json" ] || { echo "cli-candidate: $W already holds a candidate" >&2; exit 1; }

tag_commit() { # annotated tags list the commit on the peeled line
  git ls-remote "https://github.com/$UPSTREAM" "refs/tags/$TAG^{}" "refs/tags/$TAG" |
    awk '/\^\{\}$/ { peeled = $1 } !/\^\{\}$/ { tag = $1 } END { print peeled ? peeled : tag }'
}
before=$(tag_commit)
asset=$(curl -fsSL --connect-timeout 30 --max-time 120 "https://api.github.com/repos/$UPSTREAM/releases/tags/$TAG" |
  jq -c --arg n "$ASSET" '.published_at as $p | [.assets[] | select(.name == $n) | {url: .browser_download_url, size: .size, apiDigest: .digest, publishedAt: $p}] | if length == 1 then .[0] else empty end')
[ -n "$asset" ] || { echo "cli-candidate: release $TAG has no single asset $ASSET" >&2; exit 1; }
url=$(jq -r .url <<<"$asset")
[ "$url" = "https://github.com/$UPSTREAM/releases/download/$TAG/$ASSET" ] || { echo "cli-candidate: unexpected asset URL $url" >&2; exit 1; }
fetch_archive "$W/nightly/$ASSET" "$url"
after=$(tag_commit)
jq -c --arg b "$before" --arg a "$after" '. + {tagCommitBefore: $b, tagCommitAfter: $a, tagMoved: ($b != $a)}' <<<"$asset" >"$W/nightly-meta.json"
echo "cli-candidate: $TAG $ASSET sha256 $(sha256sum "$W/nightly/$ASSET" | cut -d' ' -f1) ($(wc -c <"$W/nightly/$ASSET") bytes)"
