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

set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT
case_number=0
manifests=(go.mod go.work compliance/go.mod documentation/examples/remote_storage/go.mod internal/tools/go.mod web/ui/mantine-ui/src/promql/tools/go.mod)

new_fixture() {
  case_number=$((case_number + 1))
  mkdir "$temp_dir/$case_number"
  cd "$temp_dir/$case_number"
  git init -q
  for i in "${!manifests[@]}"; do
    file=${manifests[i]}
    mkdir -p "$(dirname "$file")"
    if [[ "$file" == go.work ]]; then
      printf 'go 1.26.0\n\nuse (\n\t.\n\t./compliance\n\t./documentation/examples/remote_storage\n\t./internal/tools\n\t./web/ui/mantine-ui/src/promql/tools\n)\n' > "$file"
    else
      printf 'module example.test/module%s\n\ngo 1.26.0\n' "$i" > "$file"
    fi
  done
  git add .
}

set_version() {
  sed "s/^go .*/go $2/" "$1" > "$temp_dir/version"
  cat "$temp_dir/version" > "$1"
}

assert_version() {
  local expected=$1 actual file
  for file in "${manifests[@]}"; do
    actual=$(awk '$1 == "go" {print $2}' "$file")
    if [[ "$actual" != "$expected" ]]; then
      echo "$file: expected $expected, got $actual" >&2
      exit 1
    fi
  done
}

assert_rejected() {
  local expected=$1
  git diff --no-ext-diff --binary > "$temp_dir/before"
  if bash "$script_dir/sync-go-versions.sh" > "$temp_dir/output" 2>&1; then
    echo "Expected synchronization to fail" >&2
    exit 1
  fi
  if ! grep -Eq "$expected" "$temp_dir/output"; then
    cat "$temp_dir/output" >&2
    exit 1
  fi
  git diff --no-ext-diff --binary > "$temp_dir/after"
  cmp "$temp_dir/before" "$temp_dir/after"
}

# Reproduce #19765 and verify that the highest version can originate anywhere.
for source in go.mod internal/tools/go.mod go.work; do
  new_fixture
  set_version "$source" 1.26.7
  bash "$script_dir/sync-go-versions.sh"
  assert_version 1.26.7
  git add .
  bash "$script_dir/sync-go-versions.sh"
  git diff --exit-code
done

new_fixture
set_version go.mod 1.26.9
set_version internal/tools/go.mod 1.26.10
bash "$script_dir/sync-go-versions.sh"
assert_version 1.26.10

new_fixture
set_version go.mod 1.26
bash "$script_dir/sync-go-versions.sh"
assert_version 1.26.0

new_fixture
bash "$script_dir/sync-go-versions.sh"
git diff --exit-code

# Preserve comments, formatting, file modes, and a missing final newline.
new_fixture
printf 'module example.test/preserved\n\tgo\t1.26.0  // Keep 1.26.0 in this comment.\n\n// Last line' > go.mod
chmod 640 go.mod
set_version go.work 1.26.7
printf 'module example.test/preserved\n\tgo\t1.26.7  // Keep 1.26.0 in this comment.\n\n// Last line' > "$temp_dir/expected"
mkdir -p untracked 'nested directory'
printf 'go 1.99.0\n' > untracked/go.mod
printf 'go 1.26.0\n' > 'nested directory/go.mod'
printf 'unrelated data\n' > go.sum
git add 'nested directory/go.mod' go.sum
bash "$script_dir/sync-go-versions.sh"
cmp "$temp_dir/expected" go.mod
[[ $(find go.mod -perm 0640) == go.mod ]]
[[ $(cat 'nested directory/go.mod') == 'go 1.26.7' ]]
[[ $(cat untracked/go.mod) == 'go 1.99.0' ]]
git diff --exit-code -- go.sum

# An invalid file sorted last must not leave earlier manifests partly updated.
for invalid in '' 'go 1.27.0' 'go 2.26.0' 'go 1.26rc1' 'go 1.26.01' 'go 1.26.7 extra' $'go 1.26.0\ngo 1.26.7'; do
  new_fixture
  printf 'go 1.26.7\n' > go.mod
  printf '%s\n' "$invalid" > web/ui/mantine-ui/src/promql/tools/go.mod
  assert_rejected 'Expected|versions differ'
done

new_fixture
rm web/ui/mantine-ui/src/promql/tools/go.mod
assert_rejected 'Expected a regular manifest'

new_fixture
rm web/ui/mantine-ui/src/promql/tools/go.mod
ln -s ../../../../../go.mod web/ui/mantine-ui/src/promql/tools/go.mod
assert_rejected 'Expected a regular manifest'

new_fixture
set_version go.mod 1.26.7
mkdir "$temp_dir/bin"
cat > "$temp_dir/bin/cp" <<'EOF'
#!/bin/sh
if [ "$2" = web/ui/mantine-ui/src/promql/tools/go.mod ]; then
  echo 'Injected preparation failure' >&2
  exit 1
fi
exec /bin/cp "$@"
EOF
chmod +x "$temp_dir/bin/cp"
PATH="$temp_dir/bin:$PATH" assert_rejected 'Injected preparation failure'

new_fixture
git rm -q -r -f .
assert_rejected 'No tracked Go manifests'

echo "Go version synchronization tests passed ($case_number fixtures)."
