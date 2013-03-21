# Copyright 2013 Prometheus Team
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

TEST_ARTIFACTS = prometheus prometheus.build search_index

all: test

test: build
	go test ./...

build:
	./utility/embed-static.sh web/static web/templates | gofmt > web/blob/files.go
	$(MAKE) -C model
	go build ./...
	go build -o prometheus.build

clean:
	rm -rf web/static/blob/files.go
	rm -rf $(TEST_ARTIFACTS)
	$(MAKE) -C model clean
	-find . -type f -iname '*~' -exec rm '{}' ';'
	-find . -type f -iname '*#' -exec rm '{}' ';'
	-find . -type f -iname '.#*' -exec rm '{}' ';'

format:
	find . -iname '*.go' | egrep -v "generated|\.(l|y)\.go" | xargs -n1 gofmt -w -s=true

advice:
	go tool vet .

search_index:
	godoc -index -write_index -index_files='search_index'

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

.PHONY: advice build clean documentation format search_index test

