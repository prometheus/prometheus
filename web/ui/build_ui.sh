#!/bin/bash

# Copyright 2021 The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

if ! [[ -w  $HOME ]]
then
  export npm_config_cache=$(mktemp -d)
fi

buildOrder=(lezer-promql codemirror-promql)
assetsDir="./static"

function buildModule() {
  for module in "${buildOrder[@]}"; do
    echo "build ${module}"
    npm run build -w "@prometheus-io/${module}"
  done
}

function buildReactApp() {
  echo "build react-app"
  (cd react-app && npm run build)
  mkdir -p ${assetsDir}
  rm -rf ${assetsDir}/react-app
  mv ./react-app/build ${assetsDir}/react-app
}

function buildMantineUI() {
  echo "build mantine-ui"
  npm run build -w @prometheus-io/mantine-ui
  mkdir -p ${assetsDir}
  rm -rf ${assetsDir}/mantine-ui
  mv ./mantine-ui/dist ${assetsDir}/mantine-ui
}

for i in "$@"; do
  case ${i} in
  --all)
    buildModule
    buildReactApp
    buildMantineUI
    shift
    ;;
  --build-module)
    buildModule
    shift
    ;;
  esac
done
