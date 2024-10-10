#!/usr/bin/env bash

# Required minimum versions.
REQUIRED_GO_VERSION=$(grep 'version:' .promu.yml | awk '{print $2}')
REQUIRED_NODE_VERSION=$(cat web/ui/.nvmrc | tr -d '\r' | sed 's/v//')

# Function to compare versions (checks if version A >= version B).
compare_versions() {
    if [ "$1" = "$2" ]; then
        return 0
    fi
    local IFS=.
    local i ver1=($1) ver2=($2)
    for ((i=${#ver1[@]}; i<${#ver2[@]}; i++)); do
        ver1[i]=0
    done
    for ((i=0; i<${#ver1[@]}; i++)); do
        if [[ -z ${ver2[i]} ]]; then
            ver2[i]=0
        fi
        if ((10#${ver1[i]} > 10#${ver2[i]})); then
            return 0
        fi
        if ((10#${ver1[i]} < 10#${ver2[i]})); then
            return 1
        fi
    done
    return 0
}

# Check Go version.
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
if compare_versions "$GO_VERSION" "$REQUIRED_GO_VERSION"; then
    echo "Go version $GO_VERSION is OK"
else
    echo "Go version $GO_VERSION is too old, required >= $REQUIRED_GO_VERSION"
    exit 1
fi

# Check Node.js version.
NODE_VERSION=$(node -v | sed 's/v//')
if compare_versions "$NODE_VERSION" "$REQUIRED_NODE_VERSION"; then
    echo "Node.js version $NODE_VERSION is OK"
else
    echo "Node.js version $NODE_VERSION is too old, required >= $REQUIRED_NODE_VERSION"
    exit 1
fi

echo "All versions are correct."
