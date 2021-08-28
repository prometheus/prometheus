#!/bin/bash

set -e

MODULE_LIST=(codemirror-promql)

build-module() {
  for module in "${MODULE_LIST[@]}"; do
    cd "${module}"
    echo "building ${module}"
    npm run build
    cd ../
  done
}

lint-module() {
  for module in "${MODULE_LIST[@]}"; do
    cd "${module}"
    echo "running linter for ${module}"
    npm run lint
    cd ../
  done
}

test-module() {
  for module in "${MODULE_LIST[@]}"; do
    cd "${module}"
    echo "running all tests for ${module}"
    npm run test
    cd ../
  done
}

install-module(){
  for module in "${MODULE_LIST[@]}"; do
    cd "${module}"
    echo "install deps for ${module}"
    npm ci
    cd ../
  done
}

for i in "$@"; do
  case ${i} in
    --build)
      build-module
      shift
      ;;
    --lint)
      lint-module
      shift
      ;;
    --test)
      test-module
      ;;
    --install)
      install-module
      ;;
  esac
done
