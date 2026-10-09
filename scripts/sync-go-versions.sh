#!/usr/bin/env bash

# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Align tracked Go manifests to their highest patch version without changing
# the supported Go minor version. This must work before the workspace can load.
set -euo pipefail
export LC_ALL=C

cd "$(git rev-parse --show-toplevel)"
temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT

git ls-files -z -- go.mod go.work '*/go.mod' > "$temp_dir/files"
files=()
versions=()
major_minor=
max_patch=0
version_pattern='(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?'
directive_pattern="^[[:space:]]*go[[:space:]]+($version_pattern)[[:space:]]*(//.*)?$"

# Validate every input before preparing any replacements.
while IFS= read -r -d '' file; do
  if [[ ! -f "$file" || -L "$file" ]]; then
    echo "Expected a regular manifest: $file" >&2
    exit 1
  fi
  if ! directive=$(awk '$1 == "go" { count++; line = $0 } END { if (count != 1) exit 1; print line }' "$file"); then
    echo "Expected exactly one go directive in $file" >&2
    exit 1
  fi
  if [[ ! $directive =~ $directive_pattern ]]; then
    echo "Expected a stable Go version in $file: $directive" >&2
    exit 1
  fi
  version=${BASH_REMATCH[1]}
  IFS=. read -r major minor patch <<< "$version"
  patch=${patch:-0}
  if [[ -n "$major_minor" && "$major.$minor" != "$major_minor" ]]; then
    echo "Go major/minor versions differ; update them manually ($file: $version, expected $major_minor.x)" >&2
    exit 1
  fi
  major_minor="$major.$minor"
  # Compare decimal strings without interpreting leading zeroes or overflowing.
  # shellcheck disable=SC2071
  if [[ ${#patch} -gt ${#max_patch} || ( ${#patch} -eq ${#max_patch} && "$patch" > "$max_patch" ) ]]; then
    max_patch=$patch
  fi
  files+=("$file")
  versions+=("$version")
done < "$temp_dir/files"

if [[ ${#files[@]} -eq 0 ]]; then
  echo "No tracked Go manifests found" >&2
  exit 1
fi
target="$major_minor.$max_patch"

# Stage all replacements first, preserving file modes, whitespace and final newlines.
for i in "${!files[@]}"; do
  [[ ${versions[i]} != "$target" ]] || continue
  cp -p "${files[i]}" "$temp_dir/$i"
  while true; do
    line=
    if IFS= read -r line; then
      newline=$'\n'
    elif [[ -n "$line" ]]; then
      newline=
    else
      break
    fi
    if [[ $line =~ $directive_pattern ]]; then
      line=${line/"${versions[i]}"/"$target"}
    fi
    printf '%s%s' "$line" "$newline"
  done < "${files[i]}" > "$temp_dir/$i"
done

for i in "${!files[@]}"; do
  [[ ${versions[i]} != "$target" ]] || continue
  mv "$temp_dir/$i" "${files[i]}"
  echo "${files[i]}: ${versions[i]} -> $target"
done
