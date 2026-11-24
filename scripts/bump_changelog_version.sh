#!/usr/bin/env bash
#
# Description: Set the newest release heading in CHANGELOG.md to the version in
# the VERSION file and today's date. Must be run from the repository root.
#
# Usage: ./scripts/bump_changelog_version.sh [--check]
#
# Only a heading for the version in VERSION, or for a pre-release of the same
# version, is rewritten, so bumping 3.16.0-rc.4 to 3.16.0-rc.5, or 3.16.0-rc.5
# to 3.16.0, renames the existing section. Any other heading means the release
# notes have not been pasted in yet, and the script refuses rather than relabel
# a previous release.
#
# With --check nothing is written; the script exits non-zero unless the newest
# heading names the version in VERSION and has a date, so CI can catch drift.

set -euo pipefail

# newest_release_heading prints the version and the date of the first release
# heading in the given changelog, tab separated, without the "## " prefix. The
# date is empty when the heading has none.
newest_release_heading() {
  awk '/^## / && !/^## main \/ unreleased/ {
    line = $0
    sub(/^## /, "", line)
    date = ""
    if (match(line, / \/ /)) {
      date = substr(line, RSTART + 3)
      line = substr(line, 1, RSTART - 1)
    }
    printf "%s\t%s\n", line, date
    exit
  }' "$1"
}

check_only=false
if [[ "$*" == "--check" ]]; then
  check_only=true
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--check]" >&2
  exit 1
fi

version="$(< VERSION)"
version="${version#v}"

# Refuse anything that is not a release version, such as a branch name, rather
# than writing or comparing a heading for it.
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "ERROR: VERSION does not name a release version ('${version}'); expected something like 3.16.0 or 3.16.0-rc.0" >&2
  exit 1
fi

heading="$(newest_release_heading CHANGELOG.md)"
if [[ -z "${heading}" ]]; then
  echo "ERROR: no release heading found in CHANGELOG.md" >&2
  exit 1
fi
heading_version="${heading%%$'\t'*}"
heading_date="${heading#*$'\t'}"

if [[ "${check_only}" == true ]]; then
  if [[ "${heading_version}" != "${version}" ]]; then
    echo "ERROR: VERSION is ${version} but the newest release heading in CHANGELOG.md is ${heading_version}; run make bump-versions to promote the heading, or revert the VERSION change" >&2
    exit 1
  fi
  # A missing date fails this too, as does the YYYY-MM-DD placeholder that
  # generate_release_notes.sh emits.
  if [[ ! "${heading_date}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    echo "ERROR: the newest CHANGELOG.md heading for ${version} has no release date (found '${heading_date}'); run make bump-versions to set it" >&2
    exit 1
  fi
  exit 0
fi

# A pre-release heading is renamed in place when the release is bumped: rc.N to
# rc.N+1, and rc.N to the final version. A final heading is never rewritten back
# to a pre-release.
if [[ "${heading_version}" != "${version}" ]] &&
  ! [[ "${heading_version}" == *-* && "${heading_version%%-*}" == "${version%%-*}" ]]; then
  echo "ERROR: found release heading for ${heading_version} but VERSION is ${version}; check you are on the right branch and that the release notes for ${version} have been pasted into CHANGELOG.md" >&2
  exit 1
fi

tmp="CHANGELOG.md.tmp"
trap 'rm -f "${tmp}"' EXIT
awk -v h="## ${version} / $(date +%Y-%m-%d)" \
  '!done && /^## / && !/^## main \/ unreleased/ { print h; done=1; next } { print }' \
  CHANGELOG.md >"${tmp}"
mv "${tmp}" CHANGELOG.md
