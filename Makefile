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

TEST_ARTIFACTS = prometheus search_index

include Makefile.INCLUDE

all: test

advice:
	go tool vet .

binary: build
	go build $(BUILDFLAGS) .

build: preparation config model web

clean:
	$(MAKE) -C build clean
	$(MAKE) -C config clean
	$(MAKE) -C model clean
	$(MAKE) -C web clean
	rm -rf $(TEST_ARTIFACTS)
	-find . -type f -iname '*~' -exec rm '{}' ';'
	-find . -type f -iname '*#' -exec rm '{}' ';'
	-find . -type f -iname '.#*' -exec rm '{}' ';'

config: preparation
	$(MAKE) -C config

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

format:
	find . -iname '*.go' | egrep -v "generated|\.(l|y)\.go" | xargs -n1 gofmt -w -s=true

model: preparation
	$(MAKE) -C model

package: binary
	cp prometheus.build build/package/prometheus
	rsync -av build/root/lib/ build/package/lib/

preparation: source_path
	$(MAKE) -C build

run: binary
	./prometheus $(ARGUMENTS)

search_index:
	godoc -index -write_index -index_files='search_index'

server: config model preparation
	$(MAKE) -C server

# source_path is responsible for ensuring that the builder has not done anything
# stupid like working on Prometheus outside of ${GOPATH}.
source_path:
	-[ -d "$(FULL_GOPATH)" ] || { mkdir -vp $(FULL_GOPATH_BASE) ; ln -s "$(PWD)" "$(FULL_GOPATH)" ; }
	[ -d "$(FULL_GOPATH)" ]

test: build
	go test ./... $(GO_TEST_FLAGS)

web: preparation config model
	$(MAKE) -C web

.PHONY: advice binary build clean config documentation format model package preparation run search_index source_path test
