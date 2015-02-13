# Copyright 2013 The Prometheus Authors
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

$(GOCC): $(BUILD_PATH)/cache/$(GOPKG) $(FULL_GOPATH)
	tar -C $(BUILD_PATH)/root -xzf $<
	touch $@

advice:
	$(GO) vet ./...

binary: build

build: config dependencies tools web
	$(GO) build -o prometheus $(BUILDFLAGS) .

docker: build
	docker build -t prometheus:$(REV) .

tarball: $(ARCHIVE)

$(ARCHIVE): build
	tar -czf $(ARCHIVE) prometheus

release: REMOTE     ?= $(error "can't upload, REMOTE not set")
release: REMOTE_DIR ?= $(error "can't upload, REMOTE_DIR not set")
release: $(ARCHIVE)
	scp $< $(REMOTE):$(REMOTE_DIR)/

tag:
	git tag $(VERSION)
	git push --tags

$(BUILD_PATH)/cache/$(GOPKG):
	$(CURL) -o $@ -L $(GOURL)/$(GOPKG)

benchmark: config dependencies tools
	$(GO) test $(GO_TEST_FLAGS) -test.run='NONE' -test.bench='.*' -test.benchmem ./... | tee benchmark.txt

clean:
	$(MAKE) -C $(BUILD_PATH) clean
	$(MAKE) -C tools clean
	$(MAKE) -C web clean
	rm -rf $(TEST_ARTIFACTS)
	-rm $(ARCHIVE)
	-find . -type f -name '*~' -exec rm '{}' ';'
	-find . -type f -name '*#' -exec rm '{}' ';'
	-find . -type f -name '.#*' -exec rm '{}' ';'

config: dependencies
	$(MAKE) -C config

dependencies: $(GOCC) $(FULL_GOPATH)
	cp -a $(CURDIR)/Godeps/_workspace/src/* $(GOPATH)/src
	$(GO) get -d

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

format:
	find . -iname '*.go' | egrep -v "^\./\.build|./generated|\./Godeps|\.(l|y)\.go" | xargs -n1 $(GOFMT) -w -s=true

race_condition_binary: build
	$(GO) build -race -o prometheus.race $(BUILDFLAGS) .

race_condition_run: race_condition_binary
	./prometheus.race $(ARGUMENTS)

run: binary
	./prometheus -alsologtostderr -stderrthreshold=0 $(ARGUMENTS)

search_index:
	godoc -index -write_index -index_files='search_index'

server: config dependencies
	$(MAKE) -C server

# $(FULL_GOPATH) is responsible for ensuring that the builder has not done anything
# stupid like working on Prometheus outside of ${GOPATH}.
$(FULL_GOPATH):
	-[ -d "$(FULL_GOPATH)" ] || { mkdir -vp $(FULL_GOPATH_BASE) ; ln -s "$(PWD)" "$(FULL_GOPATH)" ; }
	[ -d "$(FULL_GOPATH)" ]

test: config dependencies tools web
	$(GO) test $(GO_TEST_FLAGS) ./...

tools: dependencies
	$(MAKE) -C tools

web: config dependencies
	$(MAKE) -C web

.PHONY: advice binary build clean config dependencies documentation format race_condition_binary race_condition_run release run search_index tag tarball test tools
