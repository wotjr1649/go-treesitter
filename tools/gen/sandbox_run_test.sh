#!/usr/bin/env bash
# Argument handling of sandbox-run.sh against a stub bwrap that records its argv (no bubblewrap or namespaces needed; the real
# isolation is checked on the runner by sandbox-setup.sh).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/bin" "$work/in" "$work/out"
printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$@" >"%s/argv"\n' "$work" >"$work/bin/bwrap"
chmod +x "$work/bin/bwrap"
run() { PATH="$work/bin:$PATH" bash "$here/sandbox-run.sh" "$@"; }
fail() { echo "sandbox_run_test: $*" >&2; exit 1; }
refuses() { # refuses WHY args...: exit status 2 and no bwrap call
  local why=$1; shift; rm -f "$work/argv"
  if run "$@" 2>/dev/null; then fail "accepted: $why"; else [ $? = 2 ] || fail "wrong status: $why"; fi
  [ ! -e "$work/argv" ] || fail "bwrap called: $why"
}
has() { grep -qxF -- "$1" "$work/argv" || fail "argv lacks: $1"; }
hasseq() { tr '\n' '\r' <"$work/argv" | grep -qF -- "$(printf '%s\r' "$@")" || fail "argv lacks sequence: $*"; }

run --ro "$work/in" --rw "$work/out" --chdir "$work/in" --env FOO=bar -- /usr/bin/true a "b c"
has --unshare-all; has --clearenv; has --die-with-parent; has --new-session
hasseq --ro-bind "$work/in" "$work/in"
hasseq --bind "$work/out" "$work/out"
hasseq --setenv FOO bar
hasseq --chdir "$work/in" /usr/bin/true a "b c"
tail -3 "$work/argv" | head -1 | grep -qx /usr/bin/true || fail "command not last"
refuses "no command" --ro "$work/in" --
refuses "no separator" --ro "$work/in"
refuses "unknown option" --bogus -- /usr/bin/true
refuses "relative path" --ro relative -- /usr/bin/true
refuses "missing path" --rw "$work/none" -- /usr/bin/true
refuses "option without value" --ro
refuses "env without equals" --env FOO -- /usr/bin/true
for n in GITHUB_TOKEN gh_token MY_SECRET DB_PASSWORD GITHUB_ENV ACTIONS_RUNTIME_URL RUNNER_TEMP; do
  refuses "env $n" --env "$n=x" -- /usr/bin/true
done
echo ok
