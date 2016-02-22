#!/usr/bin/env bash

# Copyright 2015 The Prometheus Authors
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

set -e

version=$(cat version/VERSION)

for GOOS in "darwin" "freebsd" "linux" "windows"; do
  for GOARCH in "amd64" "386"; do
    export GOARCH
    export GOOS
    make build

    tarball_dir="prometheus-${version}.${GOOS}-${GOARCH}"
    tarball="${tarball_dir}.tar.gz"

    if [ "$(go env GOOS)" = "windows" ]; then
      ext=".exe"
    fi

    echo " >   $tarball"
    mkdir -p "${tarball_dir}"
    cp -a "prometheus${ext}" "promtool${ext}" consoles console_libraries "${tarball_dir}"
    tar -czf "${tarball}" "${tarball_dir}"
    rm -rf "${tarball_dir}"
    rm "prometheus${ext}" "promtool${ext}"
  done
done

exit 0
