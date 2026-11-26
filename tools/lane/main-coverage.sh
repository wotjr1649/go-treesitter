#!/usr/bin/env bash
# Result of every main push run per first-parent commit; no run, or a run cancelled before it started, is NOT_RUN.
# usage: main-coverage.sh [COUNT [REF]]  (default: the last 30 first-parent commits of origin/main; needs gh and jq)
set -euo pipefail
runs=$(mktemp)
trap 'rm -f "$runs"' EXIT
gh run list --branch main --event push --limit 400 --json headSha,workflowName,status,conclusion >"$runs"
git rev-list --first-parent -n "${1:-30}" "${2:-origin/main}" | jq -Rrn --slurpfile r "$runs" '
  ($r[0] | map(.workflowName) | unique) as $w
  | ($w | join("\t")) as $head
  | "commit\t" + $head,
    (inputs as $s | $s[0:10] + "\t" + ([$w[] as $n
      | ($r[0] | map(select(.headSha == $s and .workflowName == $n)) | first) as $x
      | if $x == null or $x.conclusion == "cancelled" then "NOT_RUN" elif $x.status == "completed" then $x.conclusion else $x.status end] | join("\t")))'
