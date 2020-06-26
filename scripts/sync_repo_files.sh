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

GITHUB_TOKEN="${GITHUB_TOKEN:-}"
if [ -z "${GITHUB_TOKEN}" ]; then
  echo -e "\e[31mGitHub token (GITHUB_TOKEN) not set. Terminating.\e[0m"
  exit 1
fi

# List of files that should be synced.
SYNC_FILES="CODE_OF_CONDUCT.md LICENSE Makefile.common"

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)" || exit 1

source_dir="$(pwd)"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

fetch_repos() {
  local url="https://api.github.com/users/${1}/repos?per_page=100"
  curl --retry 5 --silent -u "${git_user}:${GITHUB_TOKEN}" "${url}" 2>/dev/null |
    jq -r '.[] | select( .name != "prometheus" ) | .name'
}

push_branch() {
  # stdout and stderr are redirected to /dev/null otherwise git-push could leak
  # the token in the logs.
  git push --quiet \
    "https://${GITHUB_TOKEN}:@github.com/${1}" \
    --set-upstream "${branch}" 1>/dev/null 2>&1
}

post_template='{"title":"%s","base":"master","head":"%s","body":"%s"}'
post_json="$(printf "${post_template}" "${pr_title}" "${branch}" "${pr_msg}")"
post_pull_request() {
  curl --show-error --silent --fail \
    -u "${git_user}:${GITHUB_TOKEN}" \
    -d "${post_json}" \
    "https://api.github.com/repos/${1}/pulls"
}

check_license() {
  # Check to see if the input is an Apache license of some kind
  echo "$1" | grep --quiet --no-messages --ignore-case 'Apache License'
}

process_repo() {
  local org_repo="$1"
  echo -e "\e[32mAnalyzing '${org_repo}'\e[0m"

  local needs_update=()
  for source_file in ${SYNC_FILES}; do
    source_checksum="$(sha256sum "${source_dir}/${source_file}" | cut -d' ' -f1)"

    target_file="$(curl -s --fail "https://raw.githubusercontent.com/${org_repo}/master/${source_file}")"
    if [[ "${source_file}" == 'LICENSE' ]] && ! check_license "${target_file}" ; then
      echo "LICENSE in ${org_repo} is not apache, skipping."
      continue
    fi
    if [[ -z "${target_file}" ]]; then
      echo "${source_file} doesn't exist in ${org_repo}"
      if [[ "${source_file}" == 'CODE_OF_CONDUCT.md' ]] ; then
        echo "CODE_OF_CONDUCT.md missing in ${org_repo}, force updating."
        needs_update+=('CODE_OF_CONDUCT.md')
      fi
      continue
    fi
    target_checksum="$(echo "${target_file}" | sha256sum | cut -d' ' -f1)"
    if [ "${source_checksum}" == "${target_checksum}" ]; then
      echo "${source_file} is already in sync."
      continue
    fi
    needs_update+=("${source_file}")
  done

  if [[ "${#needs_update[@]}" -eq 0 ]] ; then
    echo "No files need sync."
    return
  fi

  # Clone target repo to temporary directory and checkout to new branch
  git clone --quiet "https://github.com/${org_repo}.git" "${tmp_dir}/${org_repo}"
  cd "${tmp_dir}/${org_repo}" || return 1
  git checkout -b "${branch}" || return 1

  # Update the files in target repo by one from prometheus/prometheus.
  for source_file in ${needs_update[@]}; do
    cp -f "${source_dir}/${source_file}" "./${source_file}"
  done

  if [ -n "$(git status --porcelain)" ]; then
    git config user.email "${git_mail}"
    git config user.name "${git_user}"
    git add .
    git commit -s -m "${commit_msg}"
    if push_branch "${org_repo}"; then
      post_pull_request "${org_repo}"
    fi
  fi
}

for org in ${orgs}; do
  mkdir -p "${tmp_dir}/${org}"
  # Iterate over all repositories in ${org}. The GitHub API can return 100 items
  # at most but it should be enough for us as there are less than 40 repositories
  # currently.
  fetch_repos "${org}" | while read -r repo; do
    if ! process_repo "${org}/${repo}"; then
      echo "Failed to process '${org}/${repo}'"
      exit 1
    fi
  done
done
