# Copyright 2018 The Prometheus Authors
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

# Needs to be defined before including Makefile.common to auto-generate targets
DOCKER_ARCHS ?= amd64 armv7 arm64

NPM_LICENSES_TARBALL = "npm_licenses.tar.bz2"
REACT_APP_PATH = web/ui/react-app

TSDB_PROJECT_DIR = "./tsdb"
TSDB_CLI_DIR="$(TSDB_PROJECT_DIR)/cmd/tsdb"
TSDB_BIN = "$(TSDB_CLI_DIR)/tsdb"
TSDB_BENCHMARK_NUM_METRICS ?= 1000
TSDB_BENCHMARK_DATASET ?= "$(TSDB_PROJECT_DIR)/testdata/20kseries.json"
TSDB_BENCHMARK_OUTPUT_DIR ?= "$(TSDB_CLI_DIR)/benchout"

include Makefile.common

DOCKER_IMAGE_NAME       ?= prometheus

$(REACT_APP_PATH)/node_modules: $(REACT_APP_PATH)/package.json $(REACT_APP_PATH)/yarn.lock
	cd $(REACT_APP_PATH) && yarn --frozen-lockfile

.PHONY: build-react-app
build-react-app: $(REACT_APP_PATH)/node_modules
	@echo ">> building React app"
	@./scripts/build_react_app.sh

.PHONY: assets
assets: build-react-app
	@echo ">> writing assets"
	cd web/ui && GO111MODULE=$(GO111MODULE) GOOS= GOARCH= $(GO) generate -x -v $(GOOPTS)
	@$(GOFMT) -w ./web/ui

.PHONY: npm_licenses
npm_licenses:
	@echo ">> bundling npm licenses"
	rm -f $(NPM_LICENSES_TARBALL)
	find web/ui/react-app/node_modules -iname "license*" | tar cfj $(NPM_LICENSES_TARBALL) --transform 's/^/npm_licenses\//' --files-from=-

.PHONY: tarball
tarball: npm_licenses
	$(MAKE) common-tarball

.PHONY: docker
docker: npm_licenses
	$(MAKE) common-docker

.PHONY: build
build: assets
	$(MAKE) common-build

build_tsdb:
	GO111MODULE=$(GO111MODULE) $(GO) build -o $(TSDB_BIN) $(TSDB_CLI_DIR)

bench_tsdb: build_tsdb
	@echo ">> running benchmark, writing result to $(TSDB_BENCHMARK_OUTPUT_DIR)"
	@$(TSDB_BIN) bench write --metrics=$(TSDB_BENCHMARK_NUM_METRICS) --out=$(TSDB_BENCHMARK_OUTPUT_DIR) $(TSDB_BENCHMARK_DATASET)
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/cpu.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/cpuprof.svg
	@$(GO) tool pprof --inuse_space -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.inuse.svg
	@$(GO) tool pprof --alloc_space -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.alloc.svg
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/block.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/blockprof.svg
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mutex.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/mutexprof.svg
