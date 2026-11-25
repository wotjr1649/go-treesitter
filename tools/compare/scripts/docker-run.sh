#!/usr/bin/env bash
# Runs one sandbox script in a digest-pinned container: no network, read-only root, no capabilities, bounded
# processes/memory/CPUs, named environment only, inputs mounted read-only, a tmpfs scratch and one result volume.
#   docker-run.sh WORKDIR IMAGE SCRIPT [NAME=value ...]
set -euo pipefail
T=$(realpath -m "$1"); img=$2; script=$3; shift 3
envs=(); for kv in "$@"; do envs+=(-e "$kv"); done
exec docker run --rm --network none --read-only --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 1024 --memory 12g --memory-swap 12g --cpus 3 --cpuset-cpus 0-2 --stop-timeout 30 \
  --tmpfs /scratch:rw,exec,nosuid,size=8g,mode=1777 --tmpfs /tmp:rw,exec,nosuid,size=1g,mode=1777 \
  -v "$T/go:/goroot:ro" -v "$T/src:/src:ro" -v "$T/bin:/trusted:ro" -v "$T/dl:/dl:ro" -v "$T/corpus:/corpus:ro" \
  -v "$T/large:/large:ro" -v "$T/modcache:/modcache:ro" -v "$T/out:/out:rw" \
  "${envs[@]}" "$img" bash "/src/tools/compare/scripts/$script"
