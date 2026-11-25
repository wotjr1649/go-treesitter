#!/usr/bin/env bash
# Runs one sandbox script in a digest-pinned container: no network, read-only root, bounded processes/memory/CPUs,
# named environment only, inputs mounted read-only, tmpfs scratch areas and one result volume.
#
# The container's main process is the MEASURER (this repository's code): root, but with only the capabilities it needs to
# start candidate processes as separate unprivileged users (SETUID, SETGID), to hand each its private directory (CHOWN) and
# to read what they leave behind (DAC_OVERRIDE, FOWNER). Candidate processes run as users 20000+ with no capabilities and
# no_new_privs; they can write only their own directory under /work. The measurer alone writes /out.
#   docker-run.sh WORKDIR IMAGE SCRIPT [NAME=value ...]
set -euo pipefail
T=$(realpath -m "$1"); img=$2; script=$3; shift 3
envs=(); for kv in "$@"; do envs+=(-e "$kv"); done
exec docker run --rm --network none --read-only --user 0:0 \
  --cap-drop ALL --cap-add SETUID --cap-add SETGID --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER \
  --security-opt no-new-privileges \
  --pids-limit 1024 --memory 12g --memory-swap 12g --cpus 3 --cpuset-cpus 0-2 --stop-timeout 30 \
  --tmpfs /scratch:rw,exec,nosuid,size=5g,mode=755 --tmpfs /work:rw,exec,nosuid,size=6g,mode=755 --tmpfs /tmp:rw,exec,nosuid,size=1g,mode=1777 \
  -v "$T/go:/goroot:ro" -v "$T/src:/src:ro" -v "$T/bin:/trusted:ro" -v "$T/dl:/dl:ro" -v "$T/corpus:/corpus:ro" \
  -v "$T/large:/large:ro" -v "$T/modcache:/modcache:ro" -v "$T/out:/out:rw" \
  "${envs[@]}" "$img" bash "/src/tools/compare/scripts/$script"
