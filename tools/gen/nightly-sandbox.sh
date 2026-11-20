#!/usr/bin/env bash
# Runs the UNPINNED nightly tree-sitter CLI for `gen nightly-rehearsal -nightly-sandbox` (Linux, bubblewrap). gen calls this
# file as the CLI, with the CLI's arguments, from the working directory grammarpatch chose. gen sets NIGHTLY_BIN (the
# extracted binary); for each `generate` call grammarpatch sets TS_GENERATE_INPUT (that call's prepared tree: grammar,
# patched inputs, npm inputs, Swift's grammar.json) and TS_GENERATE_OUTPUT (an empty directory passed as --output).
#
# Inside: new user, PID, IPC, UTS, cgroup and network namespaces (loopback only: no network), an empty environment, no
# capabilities and no new privileges (bwrap's defaults when unprivileged; no sudo), a new session, killed with its parent.
# The filesystem is a fresh root with read-only /usr, the binary read-only, an empty /tmp, the call's input tree
# READ-ONLY and its output directory as the only writable path. The other generation tree, the checkout, pins, patches,
# archives, the Go cache, $HOME and the runner's files ($RUNNER_TEMP, GITHUB_ENV, ...) are not mounted. grammarpatch
# then imports only the expected generated files from the output directory. Needs unprivileged user namespaces (the
# workflow enables them on its runner).
set -euo pipefail
bin=${NIGHTLY_BIN:?NIGHTLY_BIN is not set}
args=(--unshare-all --die-with-parent --new-session --clearenv
  --setenv PATH /usr/bin --setenv HOME /tmp --setenv LANG C.UTF-8
  --ro-bind /usr /usr --symlink usr/bin /bin --symlink usr/lib /lib --symlink usr/lib64 /lib64
  --ro-bind-try /etc/ld.so.cache /etc/ld.so.cache
  --proc /proc --dev /dev --tmpfs /tmp
  --ro-bind "$bin" /opt/nightly/tree-sitter)
in=${TS_GENERATE_INPUT:-} out=${TS_GENERATE_OUTPUT:-}
if [ -n "$in$out" ]; then
  [ -n "$in" ] && [ -n "$out" ] || { echo "nightly-sandbox: TS_GENERATE_INPUT and TS_GENERATE_OUTPUT go together" >&2; exit 1; }
  case "$PWD/" in "$in"/*) ;; *) echo "nightly-sandbox: $PWD is outside $in" >&2; exit 1 ;; esac
  case "$out/" in "$in"/*) echo "nightly-sandbox: output $out is inside the input tree" >&2; exit 1 ;; esac
  args+=(--ro-bind "$in" "$in" --bind "$out" "$out" --chdir "$PWD")
else
  args+=(--chdir /)
fi
exec bwrap "${args[@]}" /opt/nightly/tree-sitter "$@"
