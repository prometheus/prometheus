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

TEST_ARTIFACTS = prometheus prometheus.race search_index

include Makefile.INCLUDE

all: binary test

$(GOCC): build/cache/$(GOPKG)
	tar -C build/root -xzf $<
	touch $@

advice:
	$(GO) tool vet .

binary: build

build: config dependencies model preparation tools web
	$(GO) build -o prometheus $(BUILDFLAGS) .
	cp prometheus build/package/prometheus
	rsync -av build/root/lib/ build/package/lib/

build/cache/$(GOPKG):
	curl -o $@ http://go.googlecode.com/files/$(GOPKG)

clean:
	$(MAKE) -C build clean
	$(MAKE) -C tools clean
	$(MAKE) -C web clean
	rm -rf $(TEST_ARTIFACTS)
	-find . -type f -iname '*~' -exec rm '{}' ';'
	-find . -type f -iname '*#' -exec rm '{}' ';'
	-find . -type f -iname '.#*' -exec rm '{}' ';'

config: dependencies preparation
	$(MAKE) -C config

dependencies: preparation
	$(GO) get -d

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

format:
	find . -iname '*.go' | egrep -v "generated|\.(l|y)\.go" | xargs -n1 $(GOFMT) -w -s=true

model: dependencies preparation
	$(MAKE) -C model

preparation: $(GOCC) source_path
	$(MAKE) -C build

race_condition_binary: build
	CGO_CFLAGS="-I$(PWD)/build/root/include" CGO_LDFLAGS="-L$(PWD)/build/root/lib" $(GO) build -race -o prometheus.race $(BUILDFLAGS) .

race_condition_run: race_condition_binary
	./prometheus.race $(ARGUMENTS)

run: binary
	./prometheus $(ARGUMENTS)

search_index:
	godoc -index -write_index -index_files='search_index'

server: config dependencies model preparation
	$(MAKE) -C server

# source_path is responsible for ensuring that the builder has not done anything
# stupid like working on Prometheus outside of ${GOPATH}.
source_path:
	-[ -d "$(FULL_GOPATH)" ] || { mkdir -vp $(FULL_GOPATH_BASE) ; ln -s "$(PWD)" "$(FULL_GOPATH)" ; }
	[ -d "$(FULL_GOPATH)" ]

test: build
	$(GOENV) find . -maxdepth 1 -mindepth 1 -type d -and -not -path ./build -exec $(GOCC) test {}/... $(GO_TEST_FLAGS) \;
	$(GO) test $(GO_TEST_FLAGS)

tools: dependencies preparation
	$(MAKE) -C tools

web: config dependencies model preparation
	$(MAKE) -C web

.PHONY: advice binary build clean config dependencies documentation format model package preparation race_condition_binary race_condition_run run search_index source_path test tools
