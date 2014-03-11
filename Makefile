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

$(GOCC): $(BUILD_PATH)/cache/$(GOPKG) $(FULL_GOPATH)
	tar -C $(BUILD_PATH)/root -xzf $<
	touch $@

advice:
	$(GO) tool vet .

binary: build

build: config dependencies model preparation tools web
	$(GO) build -o prometheus $(BUILDFLAGS) .
	cp prometheus $(BUILD_PATH)/package/prometheus
	rsync -av --delete $(BUILD_PATH)/root/lib/ $(BUILD_PATH)/package/lib/

docker: build
	docker build -t prometheus:$(REV) .

tarball: $(ARCHIVE)

$(ARCHIVE): build
	tar -C $(BUILD_PATH)/package -czf $(ARCHIVE) .

release: REMOTE     ?= $(error "can't upload, REMOTE not set")
release: REMOTE_DIR ?= $(error "can't upload, REMOTE_DIR not set")
release: $(ARCHIVE)
	scp $< $(REMOTE):$(REMOTE_DIR)/

tag:
	git tag $(VERSION)
	git push --tags

$(BUILD_PATH)/cache/$(GOPKG):
	curl -o $@ $(GOURL)/$(GOPKG)

clean:
	$(MAKE) -C $(BUILD_PATH) clean
	$(MAKE) -C tools clean
	$(MAKE) -C web clean
	rm -rf $(TEST_ARTIFACTS)
	-rm prometheus.tar.gz
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
	find . -iname '*.go' | egrep -v "^\./\.build|./generated|\.(l|y)\.go" | xargs -n1 $(GOFMT) -w -s=true

model: dependencies preparation
	$(MAKE) -C model

preparation: $(GOCC) $(FULL_GOPATH)
	$(MAKE) -C $(BUILD_PATH)

race_condition_binary: build
	CGO_CFLAGS="-I$(BUILD_PATH)/root/include" CGO_LDFLAGS="-L$(BUILD_PATH)/root/lib" $(GO) build -race -o prometheus.race $(BUILDFLAGS) .

race_condition_run: race_condition_binary
	./prometheus.race $(ARGUMENTS)

run: binary
	./prometheus -alsologtostderr -stderrthreshold=0 $(ARGUMENTS)

search_index:
	godoc -index -write_index -index_files='search_index'

server: config dependencies model preparation
	$(MAKE) -C server

# $(FULL_GOPATH) is responsible for ensuring that the builder has not done anything
# stupid like working on Prometheus outside of ${GOPATH}.
$(FULL_GOPATH):
	-[ -d "$(FULL_GOPATH)" ] || { mkdir -vp $(FULL_GOPATH_BASE) ; ln -s "$(PWD)" "$(FULL_GOPATH)" ; }
	[ -d "$(FULL_GOPATH)" ]

test: config dependencies model preparation tools web
	$(GO) test $(GO_TEST_FLAGS) ./...

tools: dependencies preparation
	$(MAKE) -C tools

update:
	$(GO) get -d

web: config dependencies model preparation
	$(MAKE) -C web

.PHONY: advice binary build clean config dependencies documentation format model preparation race_condition_binary race_condition_run release run search_index tag tarball test tools update
