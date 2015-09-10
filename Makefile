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

$(GOCC): $(BUILD_PATH)/cache/$(GOPKG)
	tar -C $(BUILD_PATH)/root -xzf $<
	touch $@

advice: $(GOCC)
	$(GO) vet ./...

binary: build

build: dependencies $(GOPATH)
	$(GO) build -o prometheus $(BUILDFLAGS) github.com/prometheus/prometheus/cmd/prometheus
	$(GO) build -o promtool $(BUILDFLAGS) github.com/prometheus/prometheus/cmd/promtool

docker:
	docker build -t prometheus:$(REV) .

tarball: $(ARCHIVE)

$(ARCHIVE): build
	mkdir -p $(ARCHIVEDIR)
	cp -a prometheus promtool consoles console_libraries $(ARCHIVEDIR)
	tar -czf $(ARCHIVE) $(ARCHIVEDIR)
	rm -rf $(ARCHIVEDIR)

release: REMOTE     ?= $(error "can't upload, REMOTE not set")
release: REMOTE_DIR ?= $(error "can't upload, REMOTE_DIR not set")
release: $(ARCHIVE)
	scp $< $(REMOTE):$(REMOTE_DIR)/

tag:
	git tag $(VERSION)
	git push --tags

$(BUILD_PATH)/cache/$(GOPKG):
	$(CURL) -o $@ -L $(GOURL)/$(GOPKG)

benchmark: dependencies
	$(GO) test $(GO_TEST_FLAGS) -test.run='NONE' -test.bench='.*' -test.benchmem ./... | tee benchmark.txt

clean:
	$(MAKE) -C $(BUILD_PATH) clean
	rm -rf $(TEST_ARTIFACTS)
	-rm $(ARCHIVE)
	-find . -type f -name '*~' -exec rm '{}' ';'
	-find . -type f -name '*#' -exec rm '{}' ';'
	-find . -type f -name '.#*' -exec rm '{}' ';'

$(SELFLINK): $(GOPATH)
	ln -s $(MAKEFILE_DIR) $@

dependencies: $(GOCC) | $(SELFLINK)

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

format: dependencies
	find . -iname '*.go' | egrep -v "^\./\.build/" | xargs -n1 $(GOFMT) -w -s=true

race_condition_binary: build
	$(GO) build -race -o prometheus.race $(BUILDFLAGS) github.com/prometheus/prometheus/cmd/prometheus
	$(GO) build -race -o promtool.race $(BUILDFLAGS) github.com/prometheus/prometheus/cmd/promtool

search_index:
	godoc -index -write_index -index_files='search_index'

test: dependencies
	$(GO) test $(GO_TEST_FLAGS) ./...

web: dependencies
	$(MAKE) -C web

.PHONY: advice binary build clean dependencies documentation format race_condition_binary race_condition_run release run search_index tag tarball test
