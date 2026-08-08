#!/usr/bin/env bash

set -u -o pipefail

echo "Checking the release-notes block in the PR description"

content=$(cat | tr -d '\r' | sed -n '/```release-notes/,/```/p' | grep -v '```' | grep -v '^[[:space:]]*$')

if [[ -z "$content" ]]; then
    echo "Error: release-notes block empty or not found, see template at https://github.com/prometheus/prometheus/blob/main/.github/PULL_REQUEST_TEMPLATE.md?plain=1"
    exit 1
fi

if [[ "$content" == "NONE" ]]; then
    echo "Release note check passed, content is NONE"
    exit 0
fi

types=(
    BUGFIX
    CHANGE
    ENHANCEMENT
    FEATURE
    PERF
    SECURITY
)

components=(
    Agent
    Alerting
    API
    Config
    Discovery
    Dockerfile
    Federation
    Mixin
    OTLP
    PromQL
    Promtool
    Relabeling
    "Remote-Read"
    "Remote-Write"
    Rules
    Scraping
    Templates
    Tracing
    TSDB
    UI
)

types_re=$(IFS='|'; printf '%s' "${types[*]}")
components_re=$(IFS='|'; printf '%s' "${components[*]}")

# Expected format: [TYPE] Component: description
pattern="^\[($types_re)\] ($components_re): .+"

while IFS= read -r line; do
  if [[ ! $line =~ $pattern ]]; then
    echo "Error: '$line' does not match the expected format."
    echo "Expected: NONE or [TYPE] Component: Description"
    echo "Valid types: [${types_re//|/] [}]"
    echo "Valid components: ${components_re//|/, }"
    exit 1
  fi
done <<<"$content"

echo "Release note check passed"
