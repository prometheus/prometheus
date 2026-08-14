#!/usr/bin/env bash

YELLOW='\033[0;33m'
NC='\033[0m'

MIN_NODE_VERSION=$(cat web/ui/.nvmrc | sed 's/v//')
CURRENT_NODE_VERSION=$(node --version | sed 's/v//')

if [ "$(echo -e "$CURRENT_NODE_VERSION\n$MIN_NODE_VERSION" | sort -V | head -n 1)" != "$MIN_NODE_VERSION" ]; then
    printf "${YELLOW}Warning:${NC}: Node.js version mismatch! Required minimum: $MIN_NODE_VERSION Installed version: $CURRENT_NODE_VERSION\n"
fi
