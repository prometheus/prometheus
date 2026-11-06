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
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

# Keep git from finding the tags of an enclosing repository when a case runs in
# a directory under it, which would trigger the already-tagged guard.
export GIT_CEILING_DIRECTORIES="${tmpdir}"

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

rc0_changelog='# Changelog

## main / unreleased

## 3.16.0-rc.0 / YYYY-MM-DD
- [CHANGE] Something changed. #1'

# Recorded from the v3.16 release: at 815458acb the newest heading was
# "## 3.16.0-rc.2 / 2026-10-27", and bumping VERSION to 3.16.0-rc.4 produced the
# "## 3.16.0-rc.4 / 2026-11-03" heading of e7d153cb5.
recorded_changelog='# Changelog

## main / unreleased

## 3.16.0-rc.2 / 2026-10-27
- [CHANGE] API: The experimental search endpoints now clamp batch_size to 1000. #19731

## 3.16.0-rc.1 / 2026-10-27
- [FEATURE] PromQL: Add experimental query cost estimation. #20028'

# Recorded from main after the 3.14 merge (#19443): no "main / unreleased"
# heading, the newest release heading comes first.
no_unreleased_changelog='# Changelog

## 3.14.0 / 2026-08-17
- [CHANGE] API: Deprecate the stats query parameter. #19124'

no_date_changelog='# Changelog

## main / unreleased

## 3.16.0
- [CHANGE] Something changed. #1'

placeholder_date_changelog='# Changelog

## main / unreleased

## 3.16.0 / YYYY-MM-DD
- [CHANGE] Something changed. #1'

stale_changelog='# Changelog

## main / unreleased

- [ENHANCEMENT] Not released yet. #3

## 3.15.1 / 2026-10-21
- [BUGFIX] Something fixed. #2'

placeholder_changelog='# Changelog

## main / unreleased

## 3.16.0 / YYYY-MM-DD
- [CHANGE] Something changed. #1'

unreleased_only_changelog='# Changelog

## main / unreleased

- [ENHANCEMENT] Not released yet. #3'

first_release_heading() {
  awk '/^## / && !/^## main \/ unreleased/ { sub(/\r$/, ""); print; exit }' "$1"
}

# check runs one case.
#
#   check <name> <tag> <want> <want heading> <changelog> <version> <message> [args...]
#
# <tag> is a tag to create in the throwaway repository before running, or "".
# <want> is "ok" or "fail". <want heading> is the expected newest release
# heading, or "@unchanged" to require that CHANGELOG.md was not touched.
# <version> is written to VERSION, or "" to leave the file out. <message> is a
# substring the output must contain, or "" to skip that assertion.
check() {
  local name="$1" tag="$2" want="$3" want_heading="$4" changelog="$5" version="$6" message="$7"
  shift 7
  total=$((total + 1))

  local dir output status got_heading changed leftover problems=""
  local date_before date_after
  dir="$(mktemp -d "${tmpdir}/case.XXXXXX")"

  printf '%s\n' "${changelog}" >"${dir}/CHANGELOG.md"
  if [[ -n "${version}" ]]; then
    # %b so cases can inject whitespace such as a trailing CR.
    printf '%b' "${version}" >"${dir}/VERSION"
  fi
  cp "${dir}/CHANGELOG.md" "${dir}/CHANGELOG.md.before"

  if [[ -n "${tag}" ]] && ! command -v git >/dev/null 2>&1; then
    total=$((total - 1))
    echo "SKIP: ${name} (git is not available)"
    return
  fi

  if [[ -n "${tag}" ]]; then
    (
      cd "${dir}" &&
        git init -q . &&
        git add -A &&
        git -c user.name=test -c user.email=test@example.com commit -qm "init" &&
        git tag "${tag}"
    ) >/dev/null 2>&1 || problems="${problems} could not create tag ${tag};"
  fi

  date_before="$(date +%Y-%m-%d)"
  output="$(cd "${dir}" && "${script}" "$@" 2>&1)"
  status=$?
  date_after="$(date +%Y-%m-%d)"

  if [[ "${want}" == "ok" ]]; then
    [[ ${status} -eq 0 ]] || problems="${problems} exited ${status}, want 0;"
  else
    [[ ${status} -ne 0 ]] || problems="${problems} exited 0, want non-zero;"
  fi

  if [[ "${want_heading}" == "@unchanged" ]]; then
    cmp -s "${dir}/CHANGELOG.md.before" "${dir}/CHANGELOG.md" ||
      problems="${problems} CHANGELOG.md was modified;"
  elif [[ "${want_heading}" == @today:* ]]; then
    # Accept the date either side of the run, so a rollover at midnight does not
    # make this fail while still pinning the value rather than its shape.
    local today_version="${want_heading#@today:}"
    got_heading="$(first_release_heading "${dir}/CHANGELOG.md")"
    if [[ "${got_heading}" != "## ${today_version} / ${date_before}" &&
      "${got_heading}" != "## ${today_version} / ${date_after}" ]]; then
      problems="${problems} heading '${got_heading}' is not today's date (${date_before});"
    fi
  else
    got_heading="$(first_release_heading "${dir}/CHANGELOG.md")"
    [[ "${got_heading}" == "${want_heading}" ]] ||
      problems="${problems} heading is '${got_heading}', want '${want_heading}';"
  fi

  # A successful run must only rewrite the heading itself.
  if [[ "${want}" == "ok" ]]; then
    changed="$(diff "${dir}/CHANGELOG.md.before" "${dir}/CHANGELOG.md" | grep -c '^[<>]')"
    [[ "${changed}" -le 2 ]] ||
      problems="${problems} ${changed} changelog lines changed, want at most 2;"
  fi

  if head -1 "${dir}/CHANGELOG.md.before" | grep -q $'\r'; then
    local crlf_lines all_lines
    all_lines="$(wc -l <"${dir}/CHANGELOG.md")"
    crlf_lines="$(grep -c $'\r$' "${dir}/CHANGELOG.md")"
    [[ "${all_lines}" -eq "${crlf_lines}" ]] ||
      problems="${problems} only ${crlf_lines} of ${all_lines} lines kept CRLF;"
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

# Promoting an RC in place is what a final release does, and used to work
# before the heading match became strict.
check "rc heading is promoted to the final version" "" ok \
  "## 3.16.0 / 2026-11-04" "${rc_changelog}" '3.16.0\n' "" --date 2026-11-04

check "rc heading with a matching rc version takes the new date" "" ok \
  "## 3.16.0-rc.4 / 2026-11-04" "${rc_changelog}" '3.16.0-rc.4\n' "" --date 2026-11-04

# Bumping one rc to the next renames the heading in place, as does promoting
# the last rc to the final version.
check "an rc heading is renamed for the next rc" "" ok \
  "## 3.16.0-rc.1 / 2026-10-27" "${rc0_changelog}" '3.16.0-rc.1\n' "" --date 2026-10-27

check "replays the recorded rc.2 to rc.4 bump" "" ok \
  "## 3.16.0-rc.4 / 2026-11-03" "${recorded_changelog}" '3.16.0-rc.4\n' "" --date 2026-11-03

check "placeholder heading is filled in" "" ok \
  "## 3.16.0 / 2026-11-09" "${placeholder_changelog}" '3.16.0\n' "" --date 2026-11-09

check "date defaults to today" "" ok \
  "@today:3.16.0" "${final_changelog}" '3.16.0\n' ""

# The original bug: with the release notes not pasted in yet, the previous
# release's heading was silently relabelled.
check "refuses to relabel a previous release heading" "" fail \
  "@unchanged" "${stale_changelog}" '3.16.0\n' "expected" --date 2026-11-04

check "refuses a heading for a different version while preparing an rc" "" fail \
  "@unchanged" "${stale_changelog}" '3.16.0-rc.3\n' "expected"

check "refuses to downgrade a released heading to an rc" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0-rc.4\n' "expected"

check "refuses an already tagged version" "v3.16.0" fail \
  "@unchanged" "${final_changelog}" '3.16.0\n' "already tagged" --date 2026-11-05

check "a tag for another version does not block" "v3.16.1" ok \
  "## 3.16.0 / 2026-11-05" "${final_changelog}" '3.16.0\n' "" --date 2026-11-05

check "keeps CRLF endings on a CRLF checkout" "" ok \
  "## 3.16.0 / 2026-11-05" "${final_changelog//$'\n'/$'\r\n'}"$'\r' '3.16.0\n' "" --date 2026-11-05

check "bumps a changelog without an unreleased heading" "" ok \
  "## 3.14.0 / 2026-08-18" "${no_unreleased_changelog}" '3.14.0\n' "" --date 2026-08-18

check "refuses a changelog without a release heading" "" fail \
  "@unchanged" "${unreleased_only_changelog}" '3.16.0\n' "no heading"

check "version is read despite whitespace and a v prefix" "" ok \
  "## 3.16.0 / 2026-11-04" "${rc_changelog}" ' v3.16.0 \r\n' "" --date 2026-11-04

check "refuses an empty version" "" fail \
  "@unchanged" "${final_changelog}" '\n' "VERSION is empty"

check "refuses a missing version file" "" fail \
  "@unchanged" "${final_changelog}" "" "cannot read VERSION"

check "refuses a date that does not exist" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0\n' "invalid date" --date 2026-02-29

check "accepts a leap day" "" ok \
  "## 3.16.0 / 2028-02-29" "${final_changelog}" '3.16.0\n' "" --date 2028-02-29

check "refuses an empty date" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0\n' "requires a value" --date ""

check "refuses an unknown argument" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0\n' "unknown argument" --bogus

# --check validates the invariant a release branch drifts from, and never writes.
check "--check accepts a matching final version" "" ok \
  "@unchanged" "${final_changelog}" '3.16.0\n' "" --check

check "--check accepts a matching rc version" "" ok \
  "@unchanged" "${rc_changelog}" '3.16.0-rc.4\n' "" --check

check "--check normalises VERSION like the bump does" "" ok \
  "@unchanged" "${final_changelog}" ' v3.16.0 \r\n' "" --check

# The recorded drift: 92e4be026 bumped VERSION to 3.16.0-rc.3 while the newest
# heading still said 3.16.0-rc.2.
check "--check flags a VERSION ahead of the heading" "" fail \
  "@unchanged" "${recorded_changelog}" '3.16.0-rc.3\n' "newest release heading" --check

# The bump and the check have to agree on what a valid heading is, except for the
# promotion of an rc heading, which only the bump accepts.
check "--check rejects the release notes placeholder date" "" fail \
  "@unchanged" "${placeholder_date_changelog}" '3.16.0\n' "not a YYYY-MM-DD date" --check

check "--check rejects a heading without a date" "" fail \
  "@unchanged" "${no_date_changelog}" '3.16.0\n' "has no date" --check

check "the bump also rejects a heading without a date" "" fail \
  "@unchanged" "${no_date_changelog}" '3.16.0\n' "expected"

check "--check rejects the rc heading the bump would promote" "" fail \
  "@unchanged" "${rc_changelog}" '3.16.0\n' "run make bump-versions" --check

check "--check flags a changelog without a release heading" "" fail \
  "@unchanged" "${unreleased_only_changelog}" '3.16.0\n' "no release heading" --check

check "--check rejects a malformed VERSION instead of comparing it" "" fail \
  "@unchanged" "${final_changelog}" 'main\n' "does not name a release version" --check

check "--check rejects a second malformation case" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0 / 2026-11-01\n' "does not name a release version" --check

# The malformed-VERSION guard refuses "main" in both modes; the message must
# explain VERSION rather than naming a heading that is obviously not the issue.
check "the bump reports a malformed VERSION with a focused message" "" fail \
  "@unchanged" "${final_changelog}" 'main\n' "does not name a release version"

check "--check rejects --date" "" fail \
  "@unchanged" "${final_changelog}" '3.16.0\n' "cannot be combined with --check" --check --date 2026-11-05

check "prints usage for --help" "" ok \
  "@unchanged" "${final_changelog}" '3.16.0\n' "usage:" --help

# The documented flow starts with generate_release_notes.sh. Its release-notes
# stub renders the template's version variable as --end-rev, exactly as the
# release-notes tool renders CurrentRevision. A literal version in the template
# is left untouched. Feed the generated notes through the bump to cover the
# whole flow without network access or a real release-notes installation.
check_generated_release_notes_flow() {
  local dir output generated_notes generated_changelog status
  dir="$(mktemp -d "${tmpdir}/generate.XXXXXX")"
  mkdir "${dir}/bin"
  cat >"${dir}/bin/release-notes" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail

end_rev=""
template=""
for arg in "$@"; do
  case "${arg}" in
    --end-rev=*)
      end_rev="${arg#--end-rev=}"
      ;;
    --go-template=go-template:inline:*)
      template="${arg#--go-template=go-template:inline:}"
      ;;
  esac
done
[[ -n "${end_rev}" && -n "${template}" ]] || {
  echo "missing --end-rev or inline template" >&2
  exit 1
}
heading="$(printf '%s\n' "${template}" | grep -m1 '^## ')"
heading="${heading//'{{$version}}'/${end_rev}}"
printf '%s\n' "${heading}" '- [BUGFIX] Generated entry. #1'
STUB
  chmod +x "${dir}/bin/release-notes"

  output="$(cd "${dir}" && PATH="${dir}/bin:${PATH}" GITHUB_TOKEN=test \
    "${repo_root}/scripts/generate_release_notes.sh" 3.16.1 2>&1)"
  status=$?
  if [[ ${status} -ne 0 ]]; then
    total=$((total + 1))
    failures=$((failures + 1))
    echo "FAIL: generated release notes feed the version bump"
    echo "  generator exited ${status}: $(printf '%s' "${output}" | tr '\n' '|')"
    return
  fi

  # The generator prints a diagnostic before the Markdown; pasting the release
  # notes means taking the generated document starting at its heading.
  generated_notes="$(printf '%s\n' "${output}" | awk '/^## / { copy=1 } copy')"
  generated_changelog="# Changelog

## main / unreleased

${generated_notes}

## 3.16.0 / 2026-11-04
- [BUGFIX] Previous release. #2"
  check "generated release notes feed the version bump" "" ok \
    "## 3.16.1 / 2026-11-05" "${generated_changelog}" '3.16.1\n' "" --date 2026-11-05
}

check_generated_release_notes_flow

# check_bump_versions_target guards the Makefile target: the changelog step has
# to run before ui-bump-version so that a refusal leaves the tree untouched, and
# RELEASE_DATE has to reach the script, but only when it is given.
check_bump_versions_target() {
  total=$((total + 1))

  if [[ ! -f "${repo_root}/Makefile" ]] || ! command -v make >/dev/null 2>&1; then
    total=$((total - 1))
    echo "SKIP: bump-versions target (needs both a Makefile and make)"
    return
  fi

  local plain dated flattened first_line script_line ui_line problems=""
  # RELEASE_DATE is passed explicitly, so a value exported in the environment
  # cannot stand in for the target passing one.
  plain="$(cd "${repo_root}" && make -n bump-versions RELEASE_DATE= 2>/dev/null)"
  dated="$(cd "${repo_root}" && make -n bump-versions RELEASE_DATE=2026-11-05 2>/dev/null)"
  # Tolerate the date being quoted in the recipe, and judge only the line that
  # runs the script rather than the sub-make output below it.
  flattened="${dated//\'/}"
  first_line="$(printf '%s\n' "${plain}" | head -1)"

  script_line="$(printf '%s\n' "${plain}" | grep -n 'bump_changelog_version\.sh' | head -1 | cut -d: -f1)"
  ui_line="$(printf '%s\n' "${plain}" | grep -n 'ui-bump-version' | head -1 | cut -d: -f1)"

  [[ -n "${script_line}" && -n "${ui_line}" && ${script_line} -lt ${ui_line} ]] ||
    problems="${problems} changelog step at line ${script_line:-none}, ui-bump-version at line ${ui_line:-none};"
  [[ "${first_line}" != *"--date"* ]] ||
    problems="${problems} bump-versions passes a date when RELEASE_DATE is unset;"
  [[ "${flattened}" == *"--date 2026-11-05"* ]] ||
    problems="${problems} RELEASE_DATE is not passed through to the script;"

  if [[ -n "${problems}" ]]; then
    failures=$((failures + 1))
    echo "FAIL: bump-versions target"
    echo "  ${problems# }"
  fi
}

check_bump_versions_target

if [[ ${failures} -ne 0 ]]; then
  echo "${failures} of ${total} cases failed"
  exit 1
fi
echo "${total} cases passed"
