#!/usr/bin/env bash
#
# Description: Set the newest release heading in CHANGELOG.md to the version in
# the VERSION file and today's date. Must be run from the repository root.

set -euo pipefail

version="$(< VERSION)"
version="${version#v}"

if ! awk -v h="## ${version} / $(date +%Y-%m-%d)" \
  '!done && /^## / && !/^## main \/ unreleased/ { print h; done=1; next } { print } END { exit !done }' \
  CHANGELOG.md > CHANGELOG.md.tmp; then
  rm -f CHANGELOG.md.tmp
  echo "ERROR: no release heading found in CHANGELOG.md" >&2
  exit 1
fi
mv CHANGELOG.md.tmp CHANGELOG.md
