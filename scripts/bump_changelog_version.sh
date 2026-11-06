#!/usr/bin/env bash
#
# Description: Set the newest release heading in CHANGELOG.md to the version in
# the VERSION file and the release date, renaming a preceding release candidate
# heading in place. Refuses a heading that belongs to a different version, and a
# version that is already tagged. Must be run from the repository root.
#
# Usage: ./scripts/bump_changelog_version.sh [--date YYYY-MM-DD]
#
# Only a heading that belongs to the version in the VERSION file is rewritten:
# either "## <version> / ..." or a pre-release heading of the same version, so
# that bumping 3.16.0-rc.4 to 3.16.0-rc.5, or 3.16.0-rc.5 to 3.16.0, renames the
# section that is already there. Any other first release heading means the
# generated release notes have not been pasted into CHANGELOG.md yet, and the
# script refuses to run instead of relabelling a previous release. Renaming a
# pre-release heading backwards is not treated as an error, since only released
# headings are protected.

set -euo pipefail

# valid_date reports whether its argument is a YYYY-MM-DD date that exists in
# the calendar. Doing this in bash keeps the script independent of whether the
# local date(1) understands -d (GNU) or -f (BSD).
valid_date() {
  local year month day max_days
  local -a days_in_month=(31 28 31 30 31 30 31 31 30 31 30 31)

  [[ "$1" =~ ^([0-9]{4})-([0-9]{2})-([0-9]{2})$ ]] || return 1
  year=$((10#${BASH_REMATCH[1]}))
  month=$((10#${BASH_REMATCH[2]}))
  day=$((10#${BASH_REMATCH[3]}))

  ((month >= 1 && month <= 12)) || return 1
  max_days=${days_in_month[month - 1]}
  if ((month == 2)) && (((year % 4 == 0 && year % 100 != 0) || year % 400 == 0)); then
    max_days=29
  fi
  ((day >= 1 && day <= max_days))
}

release_date=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --date)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        echo "ERROR: --date requires a value" >&2
        exit 1
      fi
      release_date="$2"
      shift 2
      ;;
    -h | --help)
      echo "usage: $0 [--date YYYY-MM-DD]"
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

for file in VERSION CHANGELOG.md; do
  if [[ ! -r "${file}" ]]; then
    echo "ERROR: cannot read ${file}; run this script from the repository root" >&2
    exit 1
  fi
done

version="$(< VERSION)"
# Strip whitespace first, such as the trailing space or CR an editor may leave
# behind, so the heading comparison is not silently defeated by it, and only
# then the optional "v" prefix.
version="${version//[[:space:]]/}"
version="${version#v}"

if [[ -z "${version}" ]]; then
  echo "ERROR: VERSION is empty" >&2
  exit 1
fi

if [[ -z "${release_date}" ]]; then
  release_date="$(date +%Y-%m-%d)"
fi

if ! valid_date "${release_date}"; then
  echo "ERROR: invalid date '${release_date}', expected an existing YYYY-MM-DD date" >&2
  exit 1
fi

# Rewriting the heading of a tagged release re-dates a section that users have
# already been pointed at, so require manual editing for that case. Only local
# tags are visible to this check, and it is skipped outside a git repository.
if git rev-parse --verify --quiet "refs/tags/v${version}" >/dev/null 2>&1; then
  echo "ERROR: v${version} is already tagged, so its CHANGELOG.md heading describes a released version; edit it by hand if the date really needs to change" >&2
  exit 1
fi

tmp="CHANGELOG.md.tmp"
trap 'rm -f "${tmp}"' EXIT

if ! awk -v heading="## ${version} / ${release_date}" -v version="${version}" '
  !done && !failed && /^## / && !/^## main \/ unreleased/ {
    n = split($0, fields, " / ")
    found = substr(fields[1], 4)
    base = version
    sub(/-.*$/, "", base)
    found_base = found
    sub(/-.*$/, "", found_base)
    # A pre-release heading is renamed in place when the release is bumped:
    # rc.N to rc.N+1, and rc.N to the final version. A final heading is never
    # rewritten back to a pre-release.
    matches = n == 2 && (found == version || (found_base == base && index(found, "-") > 0))
    if (matches) {
      # Keep the line ending of the file so a CRLF checkout stays consistent.
      if (substr($0, length($0), 1) == "\r") {
        printf "%s\r\n", heading
      } else {
        print heading
      }
      done = 1
      next
    }
    printf "ERROR: found release heading \"%s\" but expected \"## %s / <date>\"; check you are on the right branch and that the release notes for %s have been pasted into CHANGELOG.md\n", $0, version, version > "/dev/stderr"
    failed = 1
    exit 1
  }
  { print }
  END {
    if (!done && !failed) {
      printf "ERROR: no heading for \"## %s\" found in CHANGELOG.md\n", version > "/dev/stderr"
      exit 1
    }
    exit !done
  }
' CHANGELOG.md >"${tmp}"; then
  # awk has already explained the refusal; CHANGELOG.md is left untouched.
  exit 1
fi

mv "${tmp}" CHANGELOG.md
