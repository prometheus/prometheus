#!/usr/bin/env bash
# vim: ts=2 et
# Setting -x is absolutely forbidden as it could leak the GitHub token.
set -uo pipefail

# GITHUB_TOKEN required scope: repo.repo_public

git_mail="prometheus-team@googlegroups.com"
git_user="prombot"
branch="repo_sync_codemirror"
commit_msg="Update codemirror"
pr_title="Synchronize codemirror from prometheus/prometheus"
pr_msg="Propagating changes from prometheus/prometheus default branch."
target_repo="prometheus-community/codemirror-promql"
source_path="web/ui/module/codemirror-promql"

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

# List of files that should not be synced.
excluded_files="CODE_OF_CONDUCT.md LICENSE Makefile.common SECURITY.md .yamllint MAINTAINERS.md"
excluded_dirs=".github .circleci"

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)" || exit 1

source_dir="$(pwd)/${source_path}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

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

process_repo() {
  local org_repo
  local default_branch
  org_repo="$1"
  mkdir -p "${tmp_dir}/${org_repo}"
  echo_green "Processing '${org_repo}'"

  default_branch="$(get_default_branch "${org_repo}")"
  if [[ -z "${default_branch}" ]]; then
    echo "Can't get the default branch."
    return
  fi
  echo "Default branch: ${default_branch}"

  # Clone target repo to temporary directory and checkout to new branch
  git clone --quiet "https://github.com/${org_repo}.git" "${tmp_dir}/${org_repo}"
  cd "${tmp_dir}/${org_repo}" || return 1
  git checkout -b "${branch}" || return 1

  git rm -r .

  cp -ra ${source_dir}/. .
  git add .

  for excluded_dir in ${excluded_dirs}; do
      git reset -- "${excluded_dir}/*"
      git checkout -- "${excluded_dir}/*"
  done

  for excluded_file in ${excluded_files}; do
      git reset -- "${excluded_file}"
      git checkout -- "${excluded_file}"
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

process_repo ${target_repo}
