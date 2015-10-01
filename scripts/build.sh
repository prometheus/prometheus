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

repo_path="github.com/prometheus/prometheus"

version=$( cat version/VERSION )
revision=$( git rev-parse --short HEAD 2> /dev/null || echo 'unknown' )
branch=$( git rev-parse --abbrev-ref HEAD 2> /dev/null || echo 'unknown' )
host=$( hostname -f )
build_date=$( date +%Y%m%d-%H:%M:%S )
go_version=$( go version | sed -e 's/^[^0-9.]*\([0-9.]*\).*/\1/' )

if [ "$(go env GOOS)" = "windows" ]; then
	ext=".exe"
fi

ldflags="
  -X ${repo_path}/version.Version=${version}
  -X ${repo_path}/version.Revision=${revision}
  -X ${repo_path}/version.Branch=${branch}
  -X ${repo_path}/version.BuildUser=${USER}@${host}
  -X ${repo_path}/version.BuildDate=${build_date}
  -X ${repo_path}/version.GoVersion=${go_version}"

export GO15VENDOREXPERIMENT="1"

echo " >   prometheus"
go build -ldflags "${ldflags}" -o prometheus${ext} ${repo_path}/cmd/prometheus

echo " >   promtool"
go build -ldflags "${ldflags}" -o promtool${ext} ${repo_path}/cmd/promtool

exit 0
