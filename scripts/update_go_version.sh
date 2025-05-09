#!/usr/bin/env bash
#
# Update Go version across the Prometheus project.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/update_go_version.sh" ]]; then
  echo "must be run from repository root"
  exit 255
fi

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <current_version> <new_version>"
  echo "Example: $0 1.22 1.23"
  exit 1
fi

CURRENT_VERSION=$1
NEW_VERSION=$2

echo "Updating Go version from ${CURRENT_VERSION} to ${NEW_VERSION}"

# Update .promu.yml
sed -i.bak -E "s/^([[:space:]]*version:[[:space:]]*)[0-9]+\.[0-9]+/\1${NEW_VERSION}/" .promu.yml

# Update go.mod
sed -i.bak -E "s/^go [0-9]+\.[0-9]+\.[0-9]+$/go ${CURRENT_VERSION}.0/" go.mod
sed -i.bak -E "s/^toolchain go[0-9]+\.[0-9]+\.[0-9]+$/go ${NEW_VERSION}/" go.mod

# Update GitHub workflow files
sed -i.bak -E "s|(quay\.io/prometheus/golang-builder:)[0-9]+\.[0-9]+(-base)|\1${NEW_VERSION}\2|g" .github/workflows/ci.yml
sed -i.bak -E "s/go-version: [0-9]+\.[0-9]+\.x/go-version: ${NEW_VERSION}.x/" .github/workflows/ci.yml
awk '
/name: Go tests with previous Go version/,/steps:/ {
  if ($0 ~ /image: quay.io\/prometheus\/golang-builder:/) {
    gsub(/image: quay.io\/prometheus\/golang-builder:[0-9]+\.[0-9]+(-base)/, "image: quay.io/prometheus/golang-builder:'"${CURRENT_VERSION}"'-base");
  }
}
{print}' .github/workflows/ci.yml > .github/workflows/ci.yml.new && mv .github/workflows/ci.yml.new .github/workflows/ci.yml


# Update golangci-lint.yml - replace any version with the new version
sed -i.bak "s/go-version: [0-9][0-9]*\.[0-9][0-9]*\.x/go-version: ${NEW_VERSION}.x/g" scripts/golangci-lint.yml

# Update remote_storage example go.mod
sed -i.bak -E "s/^go [0-9]+\.[0-9]+\.[0-9]+$/go ${CURRENT_VERSION}.0/" documentation/examples/remote_storage/go.mod

# Clean up backup files
find . -name "*.bak" -type f -delete

echo "Go version updated successfully from ${CURRENT_VERSION} to ${NEW_VERSION}"
echo "Please review the changes and commit them."