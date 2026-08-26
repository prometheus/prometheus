#!/usr/bin/env bash

## /!\ This file must be used at the root of the prometheus project
## This script provides utils method to help to release and verify the readiness of each libs under the folder ui/

set -e

current=$(pwd)
root_ui_folder=${current}/web/ui

# Source of truth for which release is the latest and which are LTS.
# Overridable for testing.
DOWNLOAD_JSON_URL="${DOWNLOAD_JSON_URL:-https://prometheus.io/download.json}"

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

# distTag prints the npm dist-tag to publish under, given the Prometheus
# version (e.g. 3.13.0-rc.1, as found in the VERSION file). It consults
# download.json to tell apart the latest, LTS and older stable releases so
# that pre-releases and old patches never move the "latest" tag.
function distTag() {
  local version="${1#v}"
  local offline="${2:-}"

  # Pre-release versions (semver pre-release, contains "-") must never move
  # the "latest" tag. Derive the channel from the pre-release identifier.
  if [[ "${version}" == *-* ]]; then
    case "${version#*-}" in
      rc*)    echo "rc" ;;
      beta*)  echo "beta" ;;
      alpha*) echo "alpha" ;;
      *)      echo "next" ;;
    esac
    return
  fi

  # Stable release. Dry-runs do not need the network: keep the historical
  # behaviour of publishing to "latest".
  if [[ "${offline}" == "offline" ]]; then
    echo "latest"
    return
  fi

  # download.json keys releases with a leading "v".
  local ref="v${version}"

  local json
  json=$(curl -sSL "${DOWNLOAD_JSON_URL}" || true)
  if [[ -z "${json}" ]]; then
    echo "ERROR: failed to fetch ${DOWNLOAD_JSON_URL}; cannot determine npm dist-tag" >&2
    exit 1
  fi

  local entry
  entry=$(echo "${json}" | jq -r --arg v "${ref}" \
    '[.prometheus[]? | select(.version==$v)] | if length>0 then "\(.[0].latest) \(.[0].lts)" else "missing" end')

  # Version not yet listed in download.json: fall back to "latest".
  if [[ "${entry}" == "missing" ]]; then
    echo "latest"
    return
  fi

  local is_latest is_lts
  read -r is_latest is_lts <<< "${entry}"

  if [[ "${is_latest}" == "true" ]]; then
    echo "latest"
    return
  fi

  if [[ "${is_lts}" == "true" ]]; then
    # The shared "lts" tag tracks the newest LTS line only; older LTS patch
    # releases are published as "oldstable".
    local newest_lts
    newest_lts=$(echo "${json}" | jq -r '.prometheus[]? | select(.lts==true) | .version' | sort -V | tail -1)
    if [[ "${ref}" == "${newest_lts}" ]]; then
      echo "lts"
    else
      echo "oldstable"
    fi
    return
  fi

  # Stable, but neither the latest nor an LTS release (e.g. an old patch).
  echo "oldstable"
}

function publish() {
  dry_run="${1}"
  # The VERSION file holds the Prometheus version and is keyed the same way as
  # download.json; it drives both the pre-release channel and the lookup.
  version="$(< "${current}/VERSION")"
  offline=""
  if [[ "${dry_run}" == "dry-run" ]]; then
    offline="offline"
  fi
  tag=$(distTag "${version}" "${offline}")
  cmd="pnpm publish --access public --no-git-checks --tag ${tag}"
  if [[ "${dry_run}" == "dry-run" ]]; then
    cmd+=" --dry-run"
  fi
  echo "Publishing ${version} under npm dist-tag '${tag}'"
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
    package_version=$(pnpm pkg get version | tr -d '"')
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
