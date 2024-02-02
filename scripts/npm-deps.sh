#!/bin/bash

set -e

current=$(pwd)
root_ui_folder=${current}/web/ui

function ncu() {
    target=$1
    npx npm-check-updates -u --target "${target}"
}

cd "${root_ui_folder}"

for workspace in $(jq -r '.workspaces[]' < package.json); do
  cd "${workspace}"
  ncu "$1"
  cd "${root_ui_folder}"
done

ncu "$1"
npm install
