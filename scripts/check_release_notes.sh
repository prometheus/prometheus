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

prefixes='FEATURE|ENHANCEMENT|PERF|BUGFIX|SECURITY|CHANGE'

while IFS= read -r line; do
  if [[ ! $line =~ ^\[($prefixes)\] ]]; then
    echo "Error: Invalid prefix in '$line'"
    # Convert pipes to brackets
    echo "Content should be NONE or entries should start with one of: [${prefixes//|/] [}]"
    exit 1
  fi
done <<<"$content"

echo "Release note check passed"
