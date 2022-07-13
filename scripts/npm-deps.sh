#!/bin/bash

function ncu() {
    target=$1
    npx npm-check-updates -u --target "${target}"
}

cd web/ui
for workspace in $(npm ls --production --depth 1 -json | jq -r '.dependencies[].resolved[8:]'); do
  cd "${workspace}"
  ncu "$1"
  cd ../
done

ncu "$1"
npm install
