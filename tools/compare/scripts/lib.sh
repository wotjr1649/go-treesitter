# Shared by the sandbox scripts (sourced). Runs as the measurer; candidate work goes through asu.
# Adapter users: 20000 + index in $ADAPTERS (the measurer passes the same order to compare -uid-base 20000).
UID_BASE=20000
uidof() { local i=0 a; for a in $ADAPTERS; do [ "$a" = "$1" ] && { echo $((UID_BASE + i)); return; }; i=$((i + 1)); done; echo $((UID_BASE + i)); }

# mkwork ADAPTER: the adapter's private writable area (nothing else is writable by its user).
mkwork() { local u; u=$(uidof "$1"); mkdir -p "/work/$1/tmp" "/work/$1/run" && chown -R "$u:$u" "/work/$1" && chmod 700 "/work/$1"; }

# asu ADAPTER CMD...: run CMD as the adapter's user (no capabilities, no_new_privs), in its area, with the Go cache there.
asu() { local a=$1 u; shift; u=$(uidof "$a")
  env HOME="/work/$a" TMPDIR="/work/$a/tmp" GOCACHE="/work/$a/gocache" setpriv --reuid="$u" --regid="$u" --clear-groups --no-new-privs "$@"; }

# take SRC DST: copy one regular, non-link file out of a candidate's area, owned by the measurer.
take() { [ -f "$1" ] && [ ! -L "$1" ] && install -m 0755 "$1" "$2"; }

# diag FILE: bounded, printable, comma- and quote-free text from a log a candidate could influence.
diag() { head -c 300 "$1" 2>/dev/null | tr -cd '[:print:]' | tr ',"' '__'; }
