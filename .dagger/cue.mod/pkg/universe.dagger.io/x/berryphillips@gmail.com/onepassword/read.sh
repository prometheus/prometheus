#!/usr/bin/env bash

set -eo pipefail

set -u
: "$ADDRESS"
: "$EMAIL"
: "$SECRET_KEY"
: "$PASSWORD"
: "$REFERENCE"

OP_DEVICE=$(head -c 16 /dev/urandom | base32 | tr -d = | tr '[:upper:]' '[:lower:]')
export OP_DEVICE

eval "$(echo "$PASSWORD" | op account add --signin --address "$ADDRESS" --email "$EMAIL" --secret-key "$SECRET_KEY")"

op read --force --no-newline "$REFERENCE" > /tmp/secret.txt