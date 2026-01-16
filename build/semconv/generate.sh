#!/usr/bin/env bash
# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script generates Go code from semantic convention registries.
# It finds all registry.yaml files in semconv/ directories and generates
# metrics.go files in the same directories.
#
# Usage:
#   WEAVER=/path/to/weaver ./build/semconv/generate.sh
#
# The WEAVER environment variable must be set to the path of the weaver binary.
# Use 'make generate-semconv' which handles this automatically.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEMPLATES="${SCRIPT_DIR}/templates"
POLICIES="${SCRIPT_DIR}/policies"

# Check if WEAVER is set
if [ -z "${WEAVER:-}" ]; then
    echo "Error: WEAVER environment variable is not set."
    echo "Please use 'make generate-semconv' or set WEAVER to the path of the weaver binary."
    exit 1
fi

# Check if weaver binary exists and is executable
if ! command -v "${WEAVER}" &> /dev/null && [ ! -x "${WEAVER}" ]; then
    echo "Error: Weaver binary not found at '${WEAVER}'."
    echo "Please run 'make install-weaver' first."
    exit 1
fi

echo ">> Using weaver: ${WEAVER}"

# Check if gofmt is installed
if ! command -v gofmt &> /dev/null; then
    echo "Error: gofmt is not installed."
    echo "Install Go from: https://go.dev/dl/"
    exit 1
fi

# Find all registries (directories containing registry.yaml under semconv/)
REGISTRIES=$(find "${REPO_ROOT}" -path '*/semconv/registry.yaml' -type f 2>/dev/null || true)

if [ -z "${REGISTRIES}" ]; then
    echo "No semconv registries found."
    echo "Registries should be placed in */semconv/registry.yaml"
    exit 0
fi

echo "Found registries:"
echo "${REGISTRIES}" | while read -r registry; do
    echo "  - ${registry}"
done
echo ""

# Generate code for each registry
for registry in ${REGISTRIES}; do
    dir=$(dirname "${registry}")
    echo ">> Generating ${dir}/metrics.go and ${dir}/README.md"
    
    "${WEAVER}" registry generate \
        --registry "${dir}" \
        --templates "${TEMPLATES}" \
        --policy "${POLICIES}" \
        go "${dir}"
done

# Format all generated files
echo ""
echo ">> Formatting generated files"
find "${REPO_ROOT}" -path '*/semconv/metrics.go' -type f -exec gofmt -w {} \;

echo ""
echo "Done! Generated files:"
find "${REPO_ROOT}" -path '*/semconv/metrics.go' -type f | while read -r file; do
    echo "  - ${file}"
done
find "${REPO_ROOT}" -path '*/semconv/README.md' -type f | while read -r file; do
    echo "  - ${file}"
done
