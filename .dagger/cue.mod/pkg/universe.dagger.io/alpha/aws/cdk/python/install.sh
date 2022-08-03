#!/usr/bin/env bash
set -euxo pipefail

npm i "$CDK_CLI_NAME"@"$CDK_CLI_VERSION"

python -m venv venv
# shellcheck disable=SC1091
source venv/bin/activate
pip install -r requirements.txt | tee -a /logs
