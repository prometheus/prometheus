#!/usr/bin/env bash
#
# Description: Validate `go` directive in various Go mod files.

set -u -o pipefail

echo "Checking version support"

version_url='https://go.dev/dl/?mode=json'
get_supported_version() {
  curl -s -f "${version_url}" \
    | jq -r '.[].version' \
    | sed 's/^go//' \
    | cut -f2 -d'.' \
    | sort -V \
    | head -n1
}

get_current_version() {
  awk '$1 == "go" {print $2}' go.mod \
    | cut -f2 -d'.'
}

supported_version="$(get_supported_version)"
if [[ "${supported_version}" -le 0 ]]; then
  echo "Error getting supported version from '${version_url}'"
  exit 1
fi
current_version="$(get_current_version)"
if [[ "${current_version}" -le 0 ]]; then
  echo "Error getting current version from go.mod"
  exit 1
fi

if [[ "${current_version}" -gt "${supported_version}" ]] ; then
   echo "Go mod version (1.${current_version}) is newer than upstream supported version (1.${supported_version})"
   exit 1
fi

readarray -t mod_files < <(git ls-files go.mod go.work '*/go.mod' || find . -type f -name go.mod -or -name go.work)

echo "Checking files ${mod_files[@]}"

matches=$(awk '$1 == "go" {print $2}' "${mod_files[@]}" | sort -u | wc -l)

if [[ "${matches}" -ne 1 ]]; then
  echo 'Not all go.mod/go.work files have matching go versions'
  exit 1
fi
