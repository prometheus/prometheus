#!/usr/bin/env bash
# vim: ts=2 et
# Setting -x is absolutely forbidden as it could leak the GitHub token.
set -uo pipefail

# GITHUB_TOKEN required scope: repo.repo_public

git_mail="prometheus-team@googlegroups.com"
git_user="prombot"
branch="repo_sync"
commit_msg="Update common Prometheus files"
pr_title="Synchronize common files from prometheus/prometheus"
pr_msg="Propagating changes from prometheus/prometheus default branch."
orgs="prometheus prometheus-community"

color_red='\e[31m'
color_green='\e[32m'
color_yellow='\e[33m'
color_none='\e[0m'

echo_red() {
  echo -e "${color_red}$@${color_none}" 1>&2
}

echo_green() {
  echo -e "${color_green}$@${color_none}" 1>&2
}

echo_yellow() {
  echo -e "${color_yellow}$@${color_none}" 1>&2
}

GITHUB_TOKEN="${GITHUB_TOKEN:-}"
if [ -z "${GITHUB_TOKEN}" ]; then
  echo_red 'GitHub token (GITHUB_TOKEN) not set. Terminating.'
  exit 1
fi

# List of files that should be synced.
SYNC_FILES="CODE_OF_CONDUCT.md LICENSE Makefile.common SECURITY.md .yamllint .github/workflows/golangci-lint.yml"

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)" || exit 1

source_dir="$(pwd)"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

prometheus_orb_version="$(yq eval '.orbs.prometheus' .circleci/config.yml)"
if [[ -z "${prometheus_orb_version}" ]]; then
  echo_red '"ERROR: Unable to get CircleCI orb version'
  exit 1
fi

## Internal functions
github_api() {
  local url
  url="https://api.github.com/${1}"
  shift 1
  curl --retry 5 --silent --fail -u "${git_user}:${GITHUB_TOKEN}" "${url}" "$@"
}

get_default_branch() {
  github_api "repos/${1}" 2> /dev/null |
    jq -r .default_branch
}

fetch_repos() {
  github_api "orgs/${1}/repos?type=public&per_page=100" 2> /dev/null |
    jq -r '.[] | select( .archived == false and .fork == false and .name != "prometheus" ) | .name'
}

push_branch() {
  local git_url
  git_url="https://${git_user}:${GITHUB_TOKEN}@github.com/${1}"
  # stdout and stderr are redirected to /dev/null otherwise git-push could leak
  # the token in the logs.
  # Delete the remote branch in case it was merged but not deleted.
  git push --quiet "${git_url}" ":${branch}" 1>/dev/null 2>&1
  git push --quiet "${git_url}" --set-upstream "${branch}" 1>/dev/null 2>&1
}

post_pull_request() {
  local repo="$1"
  local default_branch="$2"
  local post_json
  post_json="$(printf '{"title":"%s","base":"%s","head":"%s","body":"%s"}' "${pr_title}" "${default_branch}" "${branch}" "${pr_msg}")"
  echo "Posting PR to ${default_branch} on ${repo}"
  github_api "repos/${repo}/pulls" --data "${post_json}" --show-error |
    jq -r '"PR URL " + .html_url'
}

check_license() {
  # Check to see if the input is an Apache license of some kind
  echo "$1" | grep --quiet --no-messages --ignore-case 'Apache License'
}

check_go() {
  local org_repo
  local default_branch
  org_repo="$1"
  default_branch="$2"

  curl -sLf -o /dev/null "https://raw.githubusercontent.com/${org_repo}/${default_branch}/go.mod"
}

check_circleci_orb() {
  local org_repo
  local default_branch
  org_repo="$1"
  default_branch="$2"
  local ci_config_url="https://raw.githubusercontent.com/${org_repo}/${default_branch}/.circleci/config.yml"

  orb_version="$(curl -sL --fail "${ci_config_url}" | yq eval '.orbs.prometheus' -)"
  if [[ -z "${orb_version}" ]]; then
    echo_yellow "WARNING: Failed to fetch CirleCI orb version from '${org_repo}'"
    return 0
  fi

  if [[ "${orb_version}" != "null" && "${orb_version}" != "${prometheus_orb_version}" ]]; then
    echo "CircleCI-orb"
  fi
}

process_repo() {
  local org_repo
  local default_branch
  org_repo="$1"
  echo_green "Analyzing '${org_repo}'"

  default_branch="$(get_default_branch "${org_repo}")"
  if [[ -z "${default_branch}" ]]; then
    echo "Can't get the default branch."
    return
  fi
  echo "Default branch: ${default_branch}"

  local needs_update=()
  for source_file in ${SYNC_FILES}; do
    source_checksum="$(sha256sum "${source_dir}/${source_file}" | cut -d' ' -f1)"

    target_file="$(curl -sL --fail "https://raw.githubusercontent.com/${org_repo}/${default_branch}/${source_file}")"
    if [[ "${source_file}" == 'LICENSE' ]] && ! check_license "${target_file}" ; then
      echo "LICENSE in ${org_repo} is not apache, skipping."
      continue
    fi
    if [[ "${source_file}" == '.github/workflows/golangci-lint.yml' ]] && ! check_go "${org_repo}" "${default_branch}" ; then
      echo "${org_repo} is not Go, skipping .github/workflows/golangci-lint.yml."
      continue
    fi
    if [[ -z "${target_file}" ]]; then
      echo "${source_file} doesn't exist in ${org_repo}"
      case "${source_file}" in
        CODE_OF_CONDUCT.md | SECURITY.md | .github/workflows/golangci-lint.yml)
          echo "${source_file} missing in ${org_repo}, force updating."
          needs_update+=("${source_file}")
          ;;
      esac
      continue
    fi
    target_checksum="$(echo "${target_file}" | sha256sum | cut -d' ' -f1)"
    if [ "${source_checksum}" == "${target_checksum}" ]; then
      echo "${source_file} is already in sync."
      continue
    fi
    echo "${source_file} needs updating."
    needs_update+=("${source_file}")
  done

  local circleci
  circleci="$(check_circleci_orb "${org_repo}" "${default_branch}")"
  if [[ -n "${circleci}" ]]; then
    echo "${circleci} needs updating."
    needs_update+=("${circleci}")
  fi

  if [[ "${#needs_update[@]}" -eq 0 ]] ; then
    echo "No files need sync."
    return
  fi

  # Clone target repo to temporary directory and checkout to new branch
  git clone --quiet "https://github.com/${org_repo}.git" "${tmp_dir}/${org_repo}"
  cd "${tmp_dir}/${org_repo}" || return 1
  git checkout -b "${branch}" || return 1

  # If we need to add an Actions file this directory needs to be present.
  mkdir -p "./.github/workflows"

  # Update the files in target repo by one from prometheus/prometheus.
  for source_file in "${needs_update[@]}"; do
    case "${source_file}" in
      CircleCI-orb) yq eval -i ".orbs.prometheus = \"${prometheus_orb_version}\"" .circleci/config.yml ;;
      *) cp -f "${source_dir}/${source_file}" "./${source_file}" ;;
    esac
  done

  if [[ -n "$(git status --porcelain)" ]]; then
    git config user.email "${git_mail}"
    git config user.name "${git_user}"
    git add .
    git commit -s -m "${commit_msg}"
    if push_branch "${org_repo}"; then
      if ! post_pull_request "${org_repo}" "${default_branch}"; then
        return 1
      fi
    else
      echo "Pushing ${branch} to ${org_repo} failed"
      return 1
    fi
  fi
}

## main
for org in ${orgs}; do
  mkdir -p "${tmp_dir}/${org}"
  # Iterate over all repositories in ${org}. The GitHub API can return 100 items
  # at most but it should be enough for us as there are less than 40 repositories
  # currently.
  fetch_repos "${org}" | while read -r repo; do
    # Check if a PR is already opened for the branch.
    fetch_uri="repos/${org}/${repo}/pulls?state=open&head=${org}:${branch}"
    prLink="$(github_api "${fetch_uri}" --show-error | jq -r '.[0].html_url')"
    if [[ "${prLink}" != "null" ]]; then
      echo_green "Pull request already opened for branch '${branch}': ${prLink}"
      echo "Either close it or merge it before running this script again!"
      continue
    fi

    if ! process_repo "${org}/${repo}"; then
      echo_red "Failed to process '${org}/${repo}'"
      exit 1
    fi
  done
done
