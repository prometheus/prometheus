#!/usr/bin/env bash

set -e

current=$(pwd)
root_ui_folder=${current}/web/ui

function ncu() {
    target=$1
    pnpm dlx npm-check-updates -u --target "${target}"
}

cd "${root_ui_folder}"

for workspace in $(jq -r '.workspaces[]' < package.json); do
  cd "${workspace}"
  ncu "$1"
  cd "${root_ui_folder}"
done

ncu "$1"
pnpm install

cd "${root_ui_folder}/react-app"
ncu "$1"
pnpm install
