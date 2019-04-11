#!/bin/bash

set -uo pipefail

# GITHUB_TOKEN required scope: repo.repo_public

git_mail="prometheus-team@googlegroups.com"
git_user="prombot"
branch="makefile_common"
commit_msg="makefile: update Makefile.common with newer version"
pr_title="Synchronize Makefile.common from prometheus/prometheus"
pr_msg="Propagating changes from master Makefile.common located in prometheus/prometheus."
org="prometheus"

GITHUB_TOKEN="${GITHUB_TOKEN:-}"
if [ -z "${GITHUB_TOKEN}" ]; then
	echo -e "\e[31mGitHub token (GITHUB_TOKEN) not set. Terminating.\e[0m"
	exit 1
fi

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)"

source_makefile="$(pwd)/Makefile.common"
source_checksum="$(sha256sum Makefile.common | cut -d' ' -f1)"

tmp_dir=$(mktemp -d)
trap "rm -rf ${tmp_dir}" EXIT

# iterate over all repositories in ${org}
curl --retry 5 --silent -u "${git_user}:${GITHUB_TOKEN}" https://api.github.com/users/${org}/repos 2>/dev/null | jq -r '.[] | select( .name != "prometheus" ) | .name' | while read -r; do
	repo="${REPLY}"
	echo -e "\e[32mAnalyzing '${repo}'\e[0m"

	target_makefile="$(curl -s --fail "https://raw.githubusercontent.com/${org}/${repo}/master/Makefile.common")"
	if [ -z "${target_makefile}" ]; then
		echo "Makefile.common doesn't exist in ${repo}"
		continue
	fi
	target_checksum="$(echo ${target_makefile} | sha256sum | cut -d' ' -f1)"
	if [ "${source_checksum}" == "${target_checksum}" ]; then
		echo "Makefile.common is already in sync."
		continue
	fi

	# Clone target repo to temporary directory and checkout to new branch
	git clone "https://github.com/${org}/${repo}.git" "${tmp_dir}/${repo}"
	cd "${tmp_dir}/${repo}"
	git checkout -b "${branch}"

	# Replace Makefile.common in target repo by one from prometheus/prometheus
	cp -f "${source_makefile}" ./
	if [ -n "$(git status --porcelain)" ]; then
		git config user.email "${git_mail}"
		git config user.name "${git_user}"
		git add .
		git commit -s -m "${commit_msg}"
		if git push "https://${GITHUB_TOKEN}:@github.com/${org}/${repo}" --set-upstream "${branch}"; then
			curl -u "${git_user}:${GITHUB_TOKEN}" \
				-X POST \
				-d "{\"title\":\"${pr_title}\",\"base\":\"master\",\"head\":\"${branch}\",\"body\":\"${pr_msg}\"}" \
				"https://api.github.com/repos/${org}/${repo}/pulls"
		fi
	fi
done
