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
  cmd="pnpm publish --access public --no-git-checks"
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
    package_version=$(pnpm run env | grep npm_package_version | cut -d= -f2-)
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
  # increase the version on all packages in the pnpm workspace
  pnpm -r --include-workspace-root version "${version}" --no-git-tag-version --git-checks=false
  # bump the react-app version. its @prometheus-io/* dependencies use the
  # "link:" protocol to consume the locally built workspace packages, so they
  # carry no version to rewrite and must be left untouched.
  cd react-app
  pnpm version "${version}" --no-git-tag-version --git-checks=false
  cd "${root_ui_folder}"
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
