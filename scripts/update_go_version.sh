#!/usr/bin/env bash
#
# Update Go version across the Prometheus project (minor version bump only).
# Run from repository root.
set -euo pipefail

# Get current Go version from go.mod and calculate next minor version
current_full_version=$(go mod edit -json go.mod | jq -r .Go)
# Extract major & minor
major=${current_full_version%%.*}                                # "1"
minor=${current_full_version#*.}; minor=${minor%%.*}            # "23"
# Bump minor version
new_minor=$((minor + 1))                           # e.g. 24
# Create version strings
CURRENT_VERSION="${major}.${minor}"
NEW_VERSION="${major}.${new_minor}"

echo "Current version: ${CURRENT_VERSION}"
echo "New version: ${NEW_VERSION}"
echo "Updating Go version from ${CURRENT_VERSION} to ${NEW_VERSION}"

# Update .promu.yml
sed -i '' "s/version: ${CURRENT_VERSION}/version: ${NEW_VERSION}/" .promu.yml

# Update go.mod
go mod edit -go=${CURRENT_VERSION}.0

# Update GitHub workflow files
sed -i '' -E "s|(quay\.io/prometheus/golang-builder:)[0-9]+\.[0-9]+(-base)|\1${NEW_VERSION}\2|g" .github/workflows/ci.yml
sed -i '' -E "s/go-version: [0-9]+\.[0-9]+\.x/go-version: ${NEW_VERSION}.x/" .github/workflows/ci.yml
sed -i '' -E "/name: Go tests with previous Go version/,/steps:/ { s|(quay\.io/prometheus/golang-builder:)[0-9]+\.[0-9]+(-base)|\1${CURRENT_VERSION}\2|g; }" .github/workflows/ci.yml

# Update golangci-lint.yml - replace any version with the new version
sed -i '' "s/go-version: ${CURRENT_VERSION}.x/go-version: ${NEW_VERSION}.x/g" scripts/golangci-lint.yml

# Update remote_storage example go.mod
(cd documentation/examples/remote_storage && go mod edit -go=${CURRENT_VERSION}.0)

echo "Please review the changes and commit them."