#!/usr/bin/env bash
#
# Description: Regression tests for scripts/bump_changelog_version.sh.
#
# Each case builds a throwaway tree with a VERSION file and a CHANGELOG.md, runs
# the script in it, and asserts the exit status, the resulting newest release
# heading, that a refusal leaves CHANGELOG.md untouched, and that no temporary
# file is left behind. Run it from anywhere; it exits non-zero if any case
# fails.

set -uo pipefail

script="$(cd "$(dirname "$0")" && pwd)/bump_changelog_version.sh"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

failures=0
total=0

final_changelog='# Changelog

## main / unreleased

## 3.16.0 / 2026-11-04
- [CHANGE] Something changed. #1

## 3.15.1 / 2026-10-21
- [BUGFIX] Something fixed. #2'

rc_changelog='# Changelog

## main / unreleased

## 3.16.0-rc.4 / 2026-11-03
- [CHANGE] Something changed. #1

## 3.15.1 / 2026-10-21
- [BUGFIX] Something fixed. #2'

# Release notes as generate_release_notes.sh emits them, with a placeholder date.
# A patch release has no rc heading to rename, only this one to fill in.
patch_changelog='# Changelog

## main / unreleased

## 3.16.1 / YYYY-MM-DD
- [BUGFIX] Something fixed. #3

## 3.16.0 / 2026-11-04
- [CHANGE] Something changed. #1'

rc0_changelog='# Changelog

## main / unreleased

## 3.16.0-rc.0 / YYYY-MM-DD
- [CHANGE] Something changed. #1'

# Recorded from the v3.16 release: at 815458acb the newest heading was
# "## 3.16.0-rc.2 / 2026-10-27", which e7d153cb5 renamed for 3.16.0-rc.4.
recorded_changelog='# Changelog

## main / unreleased

## 3.16.0-rc.2 / 2026-10-27
- [CHANGE] API: The experimental search endpoints now clamp batch_size to 1000. #19731

## 3.16.0-rc.1 / 2026-10-27
- [FEATURE] PromQL: Add experimental query cost estimation. #20028'

# The release notes of the next release have not been pasted in yet.
stale_changelog='# Changelog

## main / unreleased

- [ENHANCEMENT] Not released yet. #3

## 3.15.1 / 2026-10-21
- [BUGFIX] Something fixed. #2'

first_release_heading() {
  awk '/^## / && !/^## main \/ unreleased/ { print; exit }' "$1"
}

# check runs one case.
#
#   check <name> <want> <want version> <changelog> <version> <message> [args...]
#
# <want> is "ok" or "fail". <want version> is the version the newest release
# heading must carry, with today's date, or "@unchanged" to require that
# CHANGELOG.md was not touched. <version> is written to VERSION. <message> is a
# substring the output must contain, or "" to skip that assertion.
check() {
  local name="$1" want="$2" want_version="$3" changelog="$4" version="$5" message="$6"
  shift 6
  total=$((total + 1))

  local dir output status got_heading changed leftover problems=""
  local date_before date_after
  dir="$(mktemp -d "${tmpdir}/case.XXXXXX")"

  printf '%s\n' "${changelog}" >"${dir}/CHANGELOG.md"
  printf '%s\n' "${version}" >"${dir}/VERSION"
  cp "${dir}/CHANGELOG.md" "${dir}/CHANGELOG.md.before"

  date_before="$(date +%Y-%m-%d)"
  output="$(cd "${dir}" && "${script}" "$@" 2>&1)"
  status=$?
  date_after="$(date +%Y-%m-%d)"

  if [[ "${want}" == "ok" ]]; then
    [[ ${status} -eq 0 ]] || problems="${problems} exited ${status}, want 0;"
  else
    [[ ${status} -ne 0 ]] || problems="${problems} exited 0, want non-zero;"
  fi

  if [[ "${want_version}" == "@unchanged" ]]; then
    cmp -s "${dir}/CHANGELOG.md.before" "${dir}/CHANGELOG.md" ||
      problems="${problems} CHANGELOG.md was modified;"
  else
    # Accept the date either side of the run, so a rollover at midnight does not
    # make this fail while still pinning the value rather than its shape.
    got_heading="$(first_release_heading "${dir}/CHANGELOG.md")"
    if [[ "${got_heading}" != "## ${want_version} / ${date_before}" &&
      "${got_heading}" != "## ${want_version} / ${date_after}" ]]; then
      problems="${problems} heading is '${got_heading}', want '## ${want_version} / ${date_before}';"
    fi
  fi

  # A successful run must only rewrite the heading itself.
  if [[ "${want}" == "ok" ]]; then
    changed="$(diff "${dir}/CHANGELOG.md.before" "${dir}/CHANGELOG.md" | grep -c '^[<>]')"
    [[ "${changed}" -le 2 ]] ||
      problems="${problems} ${changed} changelog lines changed, want at most 2;"
  fi

  if [[ -n "${message}" && "${output}" != *"${message}"* ]]; then
    problems="${problems} output does not contain '${message}';"
  fi

  leftover="$(ls "${dir}"/*.tmp 2>/dev/null)"
  [[ -z "${leftover}" ]] || problems="${problems} left ${leftover} behind;"

  if [[ -n "${problems}" ]]; then
    failures=$((failures + 1))
    echo "FAIL: ${name}"
    echo "  ${problems# }"
    [[ -z "${output}" ]] || echo "  output: $(printf '%s' "${output}" | tr '\n' '|')"
  fi
}

# The heading transitions a release performs: a patch release fills in the date
# of the pasted release notes, each rc renames the previous rc's heading in
# place, and the final release promotes the last rc's heading.
check "fills in the date of pasted release notes" ok \
  "3.16.1" "${patch_changelog}" "3.16.1" ""

check "renames an rc heading for a later rc" ok \
  "3.16.0-rc.4" "${recorded_changelog}" "3.16.0-rc.4" ""

check "promotes an rc heading to the final version" ok \
  "3.16.0" "${rc_changelog}" "3.16.0" ""

# The original bug: with the release notes not pasted in yet, the previous
# release's heading was silently relabelled.
check "refuses to relabel a previous release heading" fail \
  "@unchanged" "${stale_changelog}" "3.16.0" "found release heading"

# Renaming rc headings in place must not extend to a released heading.
check "refuses to downgrade a released heading to an rc" fail \
  "@unchanged" "${final_changelog}" "3.16.0-rc.4" "found release heading"

# Without this refusal, a trailing space or CR in VERSION ends up in the heading,
# and --check accepts the result because both sides carry it.
check "refuses a VERSION with trailing whitespace" fail \
  "@unchanged" "${rc_changelog}" "3.16.0-rc.5 " "does not name a release version"

# --check validates the invariant a release branch drifts from, and never writes.
check "--check accepts a heading that matches VERSION" ok \
  "@unchanged" "${final_changelog}" "3.16.0" "" --check

# The recorded drift: 92e4be026 bumped VERSION to 3.16.0-rc.3 while the newest
# heading still said 3.16.0-rc.2, and v3.16.0-rc.3 was tagged that way.
check "--check flags a VERSION ahead of the heading" fail \
  "@unchanged" "${recorded_changelog}" "3.16.0-rc.3" "newest release heading" --check

# The rc.0 heading of the v3.16 release kept this placeholder for five days.
check "--check rejects the release notes placeholder date" fail \
  "@unchanged" "${rc0_changelog}" "3.16.0-rc.0" "no release date" --check

if [[ ${failures} -ne 0 ]]; then
  echo "${failures} of ${total} cases failed"
  exit 1
fi
echo "${total} cases passed"
