#!/usr/bin/env bash
# Runs a command that executes third-party or generated code (a native build, a pinned CLI, a grammar's own scanner) inside
# bubblewrap on a Linux runner. Generalizes nightly-sandbox.sh (which stays the wrapper of the unpinned nightly CLI).
#
#   sandbox-run.sh [--ro PATH]... [--rw PATH]... [--chdir DIR] [--env NAME=VALUE]... -- COMMAND [ARG...]
#
# Inside: new user, PID, IPC, UTS, cgroup and network namespaces (loopback only: no network), an empty environment (only
# PATH, HOME, LANG and the --env pairs), no capabilities and no new privileges (bwrap's defaults when unprivileged; no sudo),
# a new session, killed with its parent. The filesystem is a fresh root with read-only /usr, an empty /tmp, and ONLY the paths
# named here: --ro read-only, --rw writable, each mounted at its own absolute path (so the paths the tools print and record stay
# the same as unsandboxed). Not mounted: $HOME, the runner's files ($RUNNER_TEMP, GITHUB_ENV, ...), the Go cache, the Actions
# credentials. COMMAND is found in /usr/bin or given as a path under a mounted directory. Needs bubblewrap and unprivileged
# user namespaces (sandbox-setup.sh prepares and checks them on a hosted ubuntu runner).
set -euo pipefail
usage() { echo "usage: sandbox-run.sh [--ro PATH]... [--rw PATH]... [--chdir DIR] [--env NAME=VALUE]... -- COMMAND [ARG...]" >&2; exit 2; }
abs() { case $1 in /*) ;; *) echo "sandbox-run: $1 is not an absolute path" >&2; exit 2 ;; esac; [ -e "$1" ] || { echo "sandbox-run: $1 does not exist" >&2; exit 2; }; }
args=(--unshare-all --die-with-parent --new-session --clearenv
  --setenv PATH /usr/bin --setenv HOME /tmp --setenv LANG C.UTF-8
  --ro-bind /usr /usr --symlink usr/bin /bin --symlink usr/lib /lib --symlink usr/lib64 /lib64
  --ro-bind-try /etc/ld.so.cache /etc/ld.so.cache --ro-bind-try /etc/alternatives /etc/alternatives
  --proc /proc --dev /dev --tmpfs /tmp)
chdir=/
while [ $# -gt 0 ]; do
  case $1 in
    --ro) [ $# -ge 2 ] || usage; abs "$2"; args+=(--ro-bind "$2" "$2"); shift 2 ;;
    --rw) [ $# -ge 2 ] || usage; abs "$2"; args+=(--bind "$2" "$2"); shift 2 ;;
    --chdir) [ $# -ge 2 ] || usage; abs "$2"; chdir=$2; shift 2 ;;
    --env)
      [ $# -ge 2 ] || usage
      name=${2%%=*}
      [[ "$2" == *=* && "$name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || usage
      # A credential must never be passed on by name; the allow-list is what the caller spells out here.
      case ${name^^} in *TOKEN*|*SECRET*|*PASSWORD*|ACTIONS_*|GITHUB_*|RUNNER_*) echo "sandbox-run: refusing to pass $name" >&2; exit 2 ;; esac
      args+=(--setenv "$name" "${2#*=}"); shift 2 ;;
    --) shift; break ;;
    *) usage ;;
  esac
done
[ $# -gt 0 ] || usage
exec bwrap "${args[@]}" --chdir "$chdir" "$@"
