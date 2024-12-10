#!/usr/bin/env bash

## /!\ This file must be used at the root of the prometheus project
## This script provides utils method to help to release and verify the readiness of each libs under the folder ui/

set -e

current=$(pwd)
root_ui_folder=${current}/web/ui

cd "${root_ui_folder}"

files=("../../LICENSE" "../../CHANGELOG.md")
workspaces=$(jq -r '.workspaces[]' < package.json)

function copy() {
  for file in "${files[@]}"; do
    for workspace in ${workspaces}; do
      if [ -f "${file}" ]; then
        cp "${file}" "${workspace}"/"$(basename "${file}")"
      fi
    done
  done
}

function publish() {
  dry_run="${1}"
  cmd="npm publish --access public"
  if [[ "${dry_run}" == "dry-run" ]]; then
    cmd+=" --dry-run"
  fi
  for workspace in ${workspaces}; do
    # package "mantine-ui" is private so we shouldn't try to publish it.
    if [[ "${workspace}" != "mantine-ui" ]]; then
      cd "${workspace}"
      eval "${cmd}"
      cd "${root_ui_folder}"
    fi
  done

}

function checkPackage() {
  version=${1}
  if [[ "${version}" == v* ]]; then
    version="${version:1}"
  fi
  for workspace in ${workspaces}; do
    cd "${workspace}"
    package_version=$(npm run env | grep npm_package_version | cut -d= -f2-)
    if [ "${version}" != "${package_version}" ]; then
      echo "version of ${workspace} is not the correct one"
      echo "expected one: ${version}"
      echo "current one: ${package_version}"
      echo "please use ./ui_release --bump-version ${version}"
      exit 1
    fi
    cd "${root_ui_folder}"
  done
}

function clean() {
  for file in "${files[@]}"; do
    for workspace in ${workspaces}; do
      f="${workspace}"/"$(basename "${file}")"
      if [ -f "${f}" ]; then
        rm "${f}"
      fi
    done
  done
}

function bumpVersion() {
  version="${1}"
  if [[ "${version}" == v* ]]; then
    version="${version:1}"
  fi
  # upgrade the @prometheus-io/* dependencies on all packages
  for workspace in ${workspaces}; do
    # sed -i syntax is different on mac and linux
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -E -i "" "s|(\"@prometheus-io/.+\": )\".+\"|\1\"${version}\"|" "${workspace}"/package.json
    else
      sed -E -i "s|(\"@prometheus-io/.+\": )\".+\"|\1\"${version}\"|" "${workspace}"/package.json
    fi
  done
  # increase the version on all packages
  npm version "${version}" --workspaces  --include-workspace-root
}

if [[ "$1" == "--copy" ]]; then
  copy
fi

if [[ $1 == "--publish" ]]; then
  publish "${@:2}"
fi

if [[ $1 == "--check-package" ]]; then
  checkPackage "${@:2}"
fi

if [[ $1 == "--bump-version" ]]; then
  bumpVersion "${@:2}"
fi

if [[ $1 == "--clean" ]]; then
  clean
fi
