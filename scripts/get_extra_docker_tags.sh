#!/usr/bin/env bash
#
# Generate extra tags for Docker images, such as "latest" and "v3", "v3.1", etc.
# Run from repository root.
set -euo pipefail

VERSION="$(< VERSION)"

git tag -l "v*" | go run scripts/get_extra_docker_tags/main.go "v${VERSION}"
