#!/usr/bin/env bash

set -u -o pipefail

# Bump Go version (minor version bump only) across the repository.
# Run from repo root.

IFS='.' read -r current_major current_minor _ <<< "$(go mod edit -json go.mod | jq -r .Go)"
new_minor=$((current_minor + 1))
# We need to support latest-1.
latest_minor=$((current_minor + 2))

CURRENT_VERSION="${current_major}.${current_minor}"
NEW_VERSION="${current_major}.${new_minor}"
LATEST_VERSION="${current_major}.${latest_minor}"

printf "Current minimum supported version: ${CURRENT_VERSION}\nNew minimum supported version: ${NEW_VERSION}\nLatest version: ${LATEST_VERSION}\n"

# Update go.mod files
go mod edit -go=${NEW_VERSION}.0
go mod edit -go=${NEW_VERSION}.0 documentation/examples/remote_storage/go.mod
go mod edit -go=${NEW_VERSION}.0 web/ui/mantine-ui/src/promql/tools/go.mod
go mod edit -go=${NEW_VERSION}.0 internal/tools/go.mod

# Update .promu.yml
sed -i "s/version: ${NEW_VERSION}/version: ${LATEST_VERSION}/g" .promu.yml

# Update GitHub Actions workflows
# Keep ordered so some versions aren't bumped twice
sed -i "s/go-version: ${NEW_VERSION}\.x/go-version: ${LATEST_VERSION}.x/g" scripts/golangci-lint.yml .github/workflows/*.yml
sed -i "s/golang-builder:${NEW_VERSION}-base/golang-builder:${LATEST_VERSION}-base/g" .github/workflows/*.yml
sed -i "s/go-version: ${CURRENT_VERSION}\.x/go-version: ${NEW_VERSION}.x/g" scripts/golangci-lint.yml .github/workflows/*.yml
sed -i "s/golang-builder:${CURRENT_VERSION}-base/golang-builder:${NEW_VERSION}-base/g" .github/workflows/*.yml

echo "Please review the changes and commit them."
