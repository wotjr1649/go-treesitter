#!/usr/bin/env bash
# Install bubblewrap on a GitHub-hosted ubuntu runner and prove that tools/gen/nightly-sandbox.sh isolates as documented, before
# any unpinned CLI runs. The same step is inline in .github/workflows/nightly-cli-rehearsal.yml; this copy is for the workflows
# added after it. The VM is discarded after the job: the apparmor switch is not a host change.
#
#   GITHUB_WORKSPACE=<checkout> RUNNER_TEMP=<temp> bash tools/gen/sandbox-prepare.sh
set -euo pipefail
: "${GITHUB_WORKSPACE:?}" "${RUNNER_TEMP:?}" "${GITHUB_ACTIONS:?this script prepares a CI runner}"
sudo apt-get update -qq && sudo apt-get install -y -qq --no-install-recommends bubblewrap
if [ -e /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]; then
  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
fi
# Self-check with bash in the nightly binary's place and the mount layout of a real generate call (grammarpatch:
# tree Work/first read-only, Work/first.out writable, Work/second beside them).
t="$RUNNER_TEMP/sandbox-check/work"
mkdir -p "$t/first/src" "$t/second/src" "$t/first.out"
echo 'module.exports = 0;' >"$t/first/src/grammar.js"
echo other >"$t/second/src/grammar.js"
(cd "$t/first" && GH_TOKEN=not-a-token TS_GENERATE_INPUT="$t/first" TS_GENERATE_OUTPUT="$t/first.out" NIGHTLY_BIN=/usr/bin/bash \
  bash "$GITHUB_WORKSPACE/tools/gen/nightly-sandbox.sh" -c '
    [ -z "${GH_TOKEN:-}${GITHUB_ENV:-}${RUNNER_TEMP:-}" ] &&
    [ -r src/grammar.js ] && ! (echo x >> src/grammar.js) 2>/dev/null && ! touch src/new 2>/dev/null &&
    [ ! -e "$2" ] && [ ! -e "$3" ] && [ ! -e "$4" ] && [ "$(ls -A "$5")" = work ] &&
    ! (exec 3<>/dev/tcp/140.82.112.3/443) 2>/dev/null &&
    echo ok > "$1/written"' check "$t/first.out" "$t/second" "${GITHUB_ENV:-/nonexistent}" "$GITHUB_WORKSPACE" "$HOME")
# Outside again: the output was written, the inputs were not. Each check fails the step on its own.
[ "$(cat "$t/first.out/written" 2>/dev/null)" = ok ] || { echo "::error::sandbox check: the output was not written"; exit 1; }
[ ! -e "$t/first/src/new" ] || { echo "::error::sandbox check: a file was created in the read-only input"; exit 1; }
[ "$(cat "$t/first/src/grammar.js")" = 'module.exports = 0;' ] || { echo "::error::sandbox check: the read-only input changed"; exit 1; }
rm -rf "$RUNNER_TEMP/sandbox-check"
