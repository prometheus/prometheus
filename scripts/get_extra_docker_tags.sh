#!/usr/bin/env bash
#
# Generate extra tags for Docker images, such as "latest" and "v3", "v3.1", etc.
# Run from repository root.
set -euo pipefail

# Check if the version argument is provided.
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v3.1.0"
    exit 1
fi

VERSION="$1"

function parse_version() {
    local version="$1"
    local prefix="$2"

    # Remove the leading 'v' if present.
    if [[ "${version}" =~ ^v ]]; then
        version="${version:1}"
    fi

    # The regex comes from https://semver.org/ and is used to validate the
    # version format. I had to remove the PCRE bit about keeping groups
    # unsplit with ?: because bash doesn't support it.
    # Also search and replace \d with [0-9] because bash doesn't support \d.
    # That also means that we cannot directly get the pre-release and build
    # metadata from the regex, so we have to do it manually.
    if ! [[ "${version}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$ ]] ; then
        echo "Does not match semver regex: ${version}"
        return 1
    fi

    local major="${BASH_REMATCH[1]}"
    local minor="${BASH_REMATCH[2]}"
    local patch="${BASH_REMATCH[3]}"
    local mmp="${major}.${minor}.${patch}"
    # Remove the leading major/minor/patch from the version string.
    version="${version#${mmp}}"

    # Check if we have a pre-release version.
    local pre_release=""
    if [[ "${version}" =~ ^- ]]; then
        # Remove the build metadata from the end.
        pre_release="${version%%+*}"
    fi

    local build_metadata="${version#${pre_release}}"

    echo "${prefix}_major=${major} ${prefix}_minor=${minor} ${prefix}_patch=${patch} ${prefix}_pre_release=${pre_release} ${prefix}_build_metadata=${build_metadata}"
}

result=$(parse_version "${VERSION}" "new")
if [ $? -ne 0 ]; then
    echo "Invalid version format: ${VERSION}"
    exit 1
fi
eval "${result}"

if [[ -n "${new_pre_release}" ]]; then
    # If there is pre-release version, we don't add any extra tags.
    exit 0
fi

isLatest="true"
isLatestInMajor="true"
isLatestInMinor="true"

# This loop uses process substitution to read the output of git tag -l "v*",
# see at the end of the loop.
while read -r tag; do
    result=$(parse_version "${tag}" "old")
    if [ $? -ne 0 ]; then
        echo "Invalid version format: ${tag}"
        exit 1
    fi
    eval "${result}"
    if [[ -n "${old_pre_release}" ]]; then
        # Ignore pre-release versions.
        continue
    fi
    if [[ "${old_major}" -eq "${new_major}" && "${old_minor}" -eq "${new_minor}" && "${old_patch}" -eq "${new_patch}" ]]; then
        # Same version as the current one, no extra tags.
        echo "Same version as the current one, no extra tags."
        isLatest="false"
        isLatestInMajor="false"
        isLatestInMinor="false"
        exit 0
    fi
    if [[ "${old_major}" -gt "${new_major}" ]]; then
        # old_major > new_major
        isLatest="false"
    fi
    if [[ "${old_major}" -eq "${new_major}" && "${old_minor}" -gt "${new_minor}" ]]; then
        # old_major == new_major && old_minor > new_minor
        isLatest="false"
        isLatestInMajor="false"
    fi
    if [[ "${old_major}" -eq "${new_major}" && "${old_minor}" -eq "${new_minor}" && "${old_patch}" -gt "${new_patch}" ]]; then
        # old_major == new_major && old_minor == new_minor && old_patch > new_patch
        isLatest="false"
        isLatestInMinor="false"
    fi
done < <(git tag -l "v*")

extra_tags=()
if [[ "${isLatest}" == "true" ]]; then
    extra_tags+=("latest")
fi
if [[ "${isLatestInMajor}" == "true" ]]; then
    extra_tags+=("v${new_major}")
fi
if [[ "${isLatestInMinor}" == "true" ]]; then
    extra_tags+=("v${new_major}.${new_minor}")
fi

echo "${extra_tags[@]}"
