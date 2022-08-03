#!/usr/bin/env bash
set -euxo pipefail

# shellcheck disable=SC1091
source venv/bin/activate
./node_modules/.bin/cdk "$@" --require-approval never --no-color 2>&1 |tee /logs
