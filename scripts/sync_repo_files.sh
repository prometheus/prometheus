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
pr_msg="Propagating changes from prometheus/prometheus default branch.\n\n*Source can be found [here](https://github.com/prometheus/prometheus/blob/main/scripts/sync_repo_files.sh).*"
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

repo_log_red() {
  echo_red "${org_repo}: $@"
}

repo_log_green() {
  echo_green "${org_repo}: $@"
}

repo_log_yellow() {
  echo_yellow "${org_repo}: $@"
}

repo_log() {
  echo "${org_repo}: $@" 1>&2
}

GITHUB_TOKEN="${GITHUB_TOKEN:-}"
if [ -z "${GITHUB_TOKEN}" ]; then
  echo_red 'GitHub token (GITHUB_TOKEN) not set. Terminating.'
  exit 1
fi

# Each org requires a dedicated PAT for posting PRs: GITHUB_TOKEN_<ORG>
# where the org name is uppercased and dashes replaced with underscores.
for org in ${orgs}; do
  org_var="GITHUB_TOKEN_${org^^}"
  org_var="${org_var//-/_}"
  if [ -z "${!org_var:-}" ]; then
    echo_red "GitHub token ${org_var} not set. Terminating."
    exit 1
  fi
done

# List of files that should be synced.
SYNC_FILES="CODE_OF_CONDUCT.md LICENSE Makefile.common SECURITY.md .dockerignore .yamllint scripts/dependabot.yml scripts/golangci-lint.yml .github/workflows/govulncheck.yml .github/workflows/scorecards.yml .github/workflows/container_description.yml .github/workflows/stale.yml"

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)" || exit 1

source_dir="$(pwd)"

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

fetch_repos() {
  github_api "orgs/${1}/repos?type=public&per_page=100" 2> /dev/null |
    jq -r '.[] | select( .archived == false and .fork == false and .name != "prometheus" ) | .name'
}

# Fork ${1} into ${git_user}. The forks API is idempotent: it returns the
# existing fork if one already exists. The fork name is prefixed with the
# upstream org to avoid collisions between orgs (e.g. prometheus_node_exporter).
# Returns the full_name of the fork (which may differ from the requested name
# when a fork already exists under a different name).
fork_repo() {
  local org_repo="$1"
  local fork_name="${org_repo//\//_}"
  github_api "repos/${org_repo}/forks" \
    -H "X-GitHub-Api-Version: 2026-03-10" \
    --data "{\"name\":\"${fork_name}\"}" |
    jq -r '.full_name' || return 1
}

push_branch() {
  local git_url
  local safe_url
  git_url="https://${git_user}:${GITHUB_TOKEN}@github.com/${1}"
  safe_url="https://${git_user}:***@github.com/${1}"
  # stdout and stderr are redirected to /dev/null otherwise git-push could leak
  # the token in the logs.
  # Delete the remote branch in case it was merged but not deleted.
  git push --quiet "${git_url}" ":${branch}" 1>/dev/null 2>&1
  # Forking is asynchronous; retry for up to 5 minutes until the git objects
  # are available before giving up.
  local deadline=$(( $(date +%s) + 300 ))
  local push_output
  until push_output="$(git push "${git_url}" --set-upstream "${branch}" 2>&1)"; do
    if [[ $(date +%s) -ge ${deadline} ]]; then
      repo_log_red "push to ${safe_url} failed: $(echo "${push_output}" | sed "s|${GITHUB_TOKEN}|***|g")"
      return 1
    fi
    repo_log_yellow "push to ${safe_url} failed, retrying in 10 seconds."
    sleep 10
  done
}

post_pull_request() {
  local repo="$1"
  local default_branch="$2"
  local fork_owner="$3"
  local fork_org_repo="$4"
  local pr_token="$5"
  local checkout_hint="To check out this branch locally and push changes back:\\n\`\`\`\\ngit remote add ${fork_owner} https://github.com/${fork_org_repo}.git\\ngit fetch ${fork_owner} ${branch}\\ngit checkout -b ${branch} ${fork_owner}/${branch}\\n\`\`\`"
  local post_json
  post_json="$(printf '{"title":"%s","base":"%s","head":"%s:%s","body":"%s","maintainer_can_modify":true}' "${pr_title}" "${default_branch}" "${fork_owner}" "${branch}" "${pr_msg}\\n\\n${checkout_hint}")"
  echo "Posting PR to ${default_branch} on ${repo}"
  curl --retry 5 --silent --fail --show-error \
    -u "${git_user}:${!pr_token}" \
    "https://api.github.com/repos/${repo}/pulls" \
    --data "${post_json}" |
    jq -r '"PR URL " + .html_url'
}

check_license() {
  # Check to see if the input is an Apache license of some kind.
  grep --quiet --no-messages --ignore-case 'Apache License' <<<"$1"
}

check_no_sync() {
  grep --quiet --no-messages 'no_prometheus_repo_sync' <<<"$1"
}

check_go() {
  local org_repo
  local default_branch
  org_repo="$1"
  default_branch="$2"

  curl -sLf -o /dev/null "https://raw.githubusercontent.com/${org_repo}/${default_branch}/go.mod"
}

check_docker() {
  local org_repo
  local default_branch
  org_repo="$1"
  default_branch="$2"

  curl -sLf -o /dev/null "https://raw.githubusercontent.com/${org_repo}/${default_branch}/Dockerfile"
}

process_repo() {
  local org_repo
  local default_branch
  local pr_token
  org_repo="$1"
  pr_token="$2"
  repo_log "Analyzing '${org_repo}'"

  default_branch="$(get_default_branch "${org_repo}")"
  if [[ -z "${default_branch}" ]]; then
    repo_log_red "Can't get the default branch."
    return
  fi
  repo_log "Default branch: ${default_branch}"

  local needs_update=()
  for source_file in ${SYNC_FILES}; do
    source_checksum="$(sha256sum "${source_dir}/${source_file}" | cut -d' ' -f1)"
    if ! check_go "${org_repo}" "${default_branch}" ; then
      if [[ "${source_file}" == 'scripts/golangci-lint.yml' ]] ; then
        repo_log "${org_repo} is not Go, skipping golangci-lint.yml."
        continue
      fi
      if [[ "${source_file}" == '.github/workflows/govulncheck.yml' ]] ; then
        repo_log "${org_repo} is not Go, skipping govulncheck.yml."
        continue
      fi
    fi
    if ! check_docker "${org_repo}" "${default_branch}" ; then
      if [[ "${source_file}" == '.dockerignore' ]] ; then
        repo_log "${org_repo} has no Dockerfile, skipping .dockerignore."
        continue
      fi
      if [[ "${source_file}" == '.github/workflows/container_description.yml' ]] ; then
        repo_log "${org_repo} has no Dockerfile, skipping container_description.yml."
        continue
      fi
    fi
    target_filename="${source_file}"
    if [[ "${source_file}" == 'scripts/dependabot.yml' ]] ; then
      target_filename=".github/dependabot.yml"
    fi
    if [[ "${source_file}" == 'scripts/golangci-lint.yml' ]] ; then
      target_filename=".github/workflows/golangci-lint.yml"
    fi
    target_file="$(curl -sL --fail "https://raw.githubusercontent.com/${org_repo}/${default_branch}/${target_filename}")"
    if [[ -z "${target_file}" ]]; then
      repo_log "${target_filename} doesn't exist in ${org_repo}"
      case "${source_file}" in
        CODE_OF_CONDUCT.md | SECURITY.md | .dockerignore | .github/workflows/container_description.yml | .github/workflows/govulncheck.yml)
          repo_log_yellow "${source_file} missing in ${org_repo}, force updating."
          needs_update+=("${source_file}")
          ;;
      esac
      continue
    fi
    if check_no_sync "${target_file}" ; then
      repo_log "${target_filename} is marked as do not sync, skipping."
      continue
    fi
    if [[ "${source_file}" == 'LICENSE' ]] && ! check_license "${target_file}" ; then
      repo_log "LICENSE in ${org_repo} is not apache, skipping."
      continue
    fi
    target_checksum="$(echo "${target_file}" | sha256sum | cut -d' ' -f1)"
    if [[ "${source_checksum}" == "${target_checksum}" ]] ; then
      repo_log "${source_file} is already in sync."
      continue
    fi
    repo_log_yellow "${source_file} needs updating."
    needs_update+=("${source_file}")
  done

  if [[ "${#needs_update[@]}" -eq 0 ]] ; then
    repo_log_green "No files need sync."
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
    target_filename="${source_file}"
    if [[ "${source_file}" == 'scripts/dependabot.yml' ]] ; then
      target_filename=".github/dependabot.yml"
    fi
    if [[ "${source_file}" == 'scripts/golangci-lint.yml' ]] ; then
      target_filename=".github/workflows/golangci-lint.yml"
    fi
    case "${source_file}" in
      *) cp -f "${source_dir}/${source_file}" "./${target_filename}" ;;
    esac
  done

  repo_log "File sync complete"

  if [[ -n "$(git status --porcelain)" ]]; then
    git config user.email "${git_mail}"
    git config user.name "${git_user}"
    git add .
    git commit -s -m "${commit_msg}"
    repo_log "Commit created"
    local fork_org_repo
    fork_org_repo="$(fork_repo "${org_repo}")" || { repo_log_red "Forking ${org_repo} failed"; return 1; }
    if push_branch "${fork_org_repo}"; then
      if ! post_pull_request "${org_repo}" "${default_branch}" "${git_user}" "${fork_org_repo}" "${pr_token}"; then
        repo_log_red "Posting PR failed"
        return 1
      fi
    else
      repo_log_red "Pushing ${branch} to ${fork_org_repo} failed"
      return 1
    fi
  fi
}

## main
for org in ${orgs}; do
  mkdir -p "${tmp_dir}/${org}"
  org_token_var="GITHUB_TOKEN_${org^^}"
  org_token_var="${org_token_var//-/_}"
  # Iterate over all repositories in ${org}. The GitHub API can return 100 items
  # at most but it should be enough for us as there are less than 40 repositories
  # currently.
  fetch_repos "${org}" | while read -r repo; do
    # Check if a PR is already opened for the branch from the prombot fork.
    fetch_uri="repos/${org}/${repo}/pulls?state=open&head=${git_user}:${branch}"
    prLink="$(github_api "${fetch_uri}" --show-error | jq -r '.[0].html_url')"
    if [[ "${prLink}" != "null" ]]; then
      echo_green "Pull request already opened for branch '${branch}': ${prLink}"
      echo "Either close it or merge it before running this script again!"
      continue
    fi

    if ! process_repo "${org}/${repo}" "${org_token_var}"; then
      echo_red "Failed to process '${org}/${repo}'"
      exit 1
    fi
  done
done
