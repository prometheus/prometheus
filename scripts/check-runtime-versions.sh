#!/usr/bin/env bash

# Required versions for Nodejs and Go.
MIN_NODE_VERSION=$(cat web/ui/.nvmrc | tr -d '\r' | sed 's/v//')
MIN_GO_VERSION=$(awk '/^go / {print $2}' go.mod)

# Check Nodejs version.
[ "$(echo -e "$(node --version)\n$MIN_NODE_VERSION" | sort -V | head -n 1)" = "$MIN_NODE_VERSION" ] && echo "Nodejs version OK" || echo "Warning: Installed Node.js version is less than the required version $MIN_NODE_VERSION"

# Check Go version.
[ "$(echo -e "$(go version | awk '{print $3}')\n$MIN_GO_VERSION" | sort -V | head -n 1)" = "$MIN_GO_VERSION" ] && echo "Go version OK" || echo "Warning: Installed Go version is less than the required version $MIN_GO_VERSION"
