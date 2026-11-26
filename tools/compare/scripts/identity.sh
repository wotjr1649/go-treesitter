#!/usr/bin/env bash
# identity.sh WORKDIR MODE: writes provenance/workflow.json and the module-graph records next to the results.
set -euo pipefail
T=$(realpath -m "$1"); mode=$2
mkdir -p "$T/out/provenance"
cp "$T"/identity/*.txt "$T/out/provenance/" 2>/dev/null || true
printf '{"repository":"%s","run_id":"%s","run_attempt":"%s","sha":"%s","ref":"%s","workflow":"%s","runner_arch":"%s","runner_image":"%s","mode":"%s"}\n' \
  "$GITHUB_REPOSITORY" "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "$GITHUB_SHA" "$GITHUB_REF" "$GITHUB_WORKFLOW" "$RUNNER_ARCH" "${ImageVersion:-unknown}" "$mode" \
  > "$T/out/provenance/workflow.json"
