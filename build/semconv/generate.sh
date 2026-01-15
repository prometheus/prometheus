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
#   ./build/semconv/generate.sh
#
# Requirements:
#   - gofmt must be available
#   - curl or wget for downloading weaver (if not installed)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEMPLATES="${SCRIPT_DIR}/templates"

# Weaver version to use - update this when upgrading
WEAVER_VERSION="v0.20.0"

# Local bin directory for downloaded tools
LOCAL_BIN="${REPO_ROOT}/.bin"
WEAVER_BIN="${LOCAL_BIN}/weaver-${WEAVER_VERSION}"

# Detect OS and architecture for downloading weaver
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)  os="unknown-linux-gnu" ;;
        Darwin*) os="apple-darwin" ;;
        *)       echo "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64)  arch="x86_64" ;;
        aarch64) arch="aarch64" ;;
        arm64)   arch="aarch64" ;;
        *)       echo "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    echo "${arch}-${os}"
}

# Download weaver if not present
install_weaver() {
    local platform="$1"
    local tarball="weaver-${platform}.tar.xz"
    local url="https://github.com/open-telemetry/weaver/releases/download/${WEAVER_VERSION}/${tarball}"

    echo ">> Installing weaver ${WEAVER_VERSION} for ${platform}"
    mkdir -p "${LOCAL_BIN}"

    # Download using curl or wget
    if command -v curl &> /dev/null; then
        if ! curl -sSfL "${url}" -o "${LOCAL_BIN}/${tarball}"; then
            echo "Error: Failed to download weaver from ${url}"
            exit 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -q "${url}" -O "${LOCAL_BIN}/${tarball}"; then
            echo "Error: Failed to download weaver from ${url}"
            exit 1
        fi
    else
        echo "Error: Neither curl nor wget found. Please install one of them."
        exit 1
    fi

    # Extract the binary (tar.xz format)
    # The archive contains a directory like weaver-aarch64-apple-darwin/weaver
    if ! tar -xJf "${LOCAL_BIN}/${tarball}" -C "${LOCAL_BIN}"; then
        echo "Error: Failed to extract weaver archive"
        rm -f "${LOCAL_BIN}/${tarball}"
        exit 1
    fi
    mv "${LOCAL_BIN}/weaver-${platform}/weaver" "${WEAVER_BIN}"
    chmod +x "${WEAVER_BIN}"

    # Cleanup tarball and extracted directory
    rm -f "${LOCAL_BIN}/${tarball}"
    rm -rf "${LOCAL_BIN}/weaver-${platform}"

    echo ">> Installed weaver to ${WEAVER_BIN}"
}

# Get the weaver binary path, installing if necessary
get_weaver() {
    # First check if the pinned version is already downloaded
    if [ -x "${WEAVER_BIN}" ]; then
        echo "${WEAVER_BIN}"
        return
    fi

    # Check if weaver is in PATH and matches our version
    if command -v weaver &> /dev/null; then
        local installed_version
        installed_version=$(weaver --version 2>/dev/null | head -1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "")
        if [ "${installed_version}" = "${WEAVER_VERSION}" ]; then
            echo "weaver"
            return
        fi
        echo ">> System weaver version (${installed_version}) differs from required (${WEAVER_VERSION})" >&2
    fi

    # Download and install
    local platform
    platform=$(detect_platform)
    install_weaver "${platform}" >&2
    echo "${WEAVER_BIN}"
}

# Get weaver binary
WEAVER=$(get_weaver)
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
        go "${dir}" \
        --skip-policies
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
