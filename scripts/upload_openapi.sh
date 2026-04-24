#!/usr/bin/env bash
# Creates a GitHub commit status pointing to the Swagger Editor with the raw
# GitHub URL of the OpenAPI 3.1 spec.
#
# Usage: openapi_status.sh <file> [context]
#   file    - path to the OpenAPI YAML file (relative to repo root)
#   context - GitHub status context label (optional; skipped if GH_TOKEN or SHA are unset)

set -euo pipefail

FILE="${1:?usage: openapi_status.sh <file> [context]}"
CONTEXT="${2:-}"

RAW_URL="https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/${SHA}/${FILE}"
VIEWER_URL="https://editor.swagger.io/?url=${RAW_URL}"
echo "${VIEWER_URL}"

if [[ -n "${CONTEXT}" && -n "${GH_TOKEN:-}" && -n "${SHA:-}" ]]; then
    gh api "repos/${GITHUB_REPOSITORY}/statuses/${SHA}" \
        --method POST \
        --field state=success \
        --field context="${CONTEXT}" \
        --field description="View ${CONTEXT} spec" \
        --field target_url="${VIEWER_URL}" \
        || echo "warning: could not create GitHub status (fork PR or insufficient permissions)." >&2
fi
