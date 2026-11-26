#!/usr/bin/env bash
# Installs bubblewrap on a hosted ubuntu runner, allows unprivileged user namespaces (Ubuntu 24.04's AppArmor restricts them;
# the VM is discarded after the job) and proves that sandbox-run.sh isolates what it claims to: no token or runner variable, no
# runner files, a read-only input, a writable output, no network. Each check fails the step on its own.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
sudo apt-get update -qq && sudo apt-get install -y -qq --no-install-recommends bubblewrap
if [ -e /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]; then
  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
fi
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
mkdir "$t/in" "$t/out"
echo data > "$t/in/f"
# bash stands in for the third-party program. $1 is a runner path that must not be visible, $2 the writable output.
GH_TOKEN=x GITHUB_TOKEN=x bash "$here/sandbox-run.sh" --ro "$t/in" --rw "$t/out" --chdir "$t/in" -- /usr/bin/bash -c '
  [ -z "${GH_TOKEN:-}${GITHUB_TOKEN:-}${GITHUB_ENV:-}${RUNNER_TEMP:-}" ] &&
  [ -r f ] && ! (echo x >> f) 2>/dev/null && ! touch new 2>/dev/null &&
  [ ! -e "$1" ] && [ ! -e "$3" ] &&
  ! (exec 3<>/dev/tcp/140.82.112.3/443) 2>/dev/null &&
  echo ok > "$2/written"' check "${RUNNER_TEMP:-/home/runner/work/_temp}" "$t/out" "${GITHUB_WORKSPACE:-/home/runner/work}"
[ "$(cat "$t/out/written" 2>/dev/null)" = ok ] || { echo "::error::sandbox check: an isolation check failed or the output was not written"; exit 1; }
[ ! -e "$t/in/new" ] && [ "$(cat "$t/in/f")" = data ] || { echo "::error::sandbox check: the read-only input changed"; exit 1; }
# The wrapper refuses to pass a credential by name.
if bash "$here/sandbox-run.sh" --env GITHUB_TOKEN=x -- /usr/bin/true 2>/dev/null; then echo "::error::sandbox check: --env passed a token"; exit 1; fi
echo "sandbox ok"
