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
DOCKER_ARCHS ?= amd64 armv7 arm64 ppc64le s390x

UI_PATH = web/ui
UI_NODE_MODULES_PATH = $(UI_PATH)/node_modules
REACT_APP_NPM_LICENSES_TARBALL = "npm_licenses.tar.bz2"

PROMTOOL = ./promtool
TSDB_BENCHMARK_NUM_METRICS ?= 1000
TSDB_BENCHMARK_DATASET ?= ./tsdb/testdata/20kseries.json
TSDB_BENCHMARK_OUTPUT_DIR ?= ./benchout

GOLANGCI_LINT_OPTS ?= --timeout 4m

include Makefile.common

DOCKER_IMAGE_NAME       ?= prometheus

.PHONY: ui-install
ui-install:
	cd $(UI_PATH) && npm install

.PHONY: ui-build
ui-build:
	cd $(UI_PATH) && npm run build

.PHONY: ui-build-module
ui-build-module:
	cd $(UI_PATH) && npm run build:module

.PHONY: ui-test
ui-test:
	cd $(UI_PATH) && npm run test:coverage

.PHONY: ui-lint
ui-lint:
	cd $(UI_PATH) && npm run lint

.PHONY: assets
assets: ui-install ui-build
	@echo ">> writing assets"
	# Un-setting GOOS and GOARCH here because the generated Go code is always the same,
	# but the cached object code is incompatible between architectures and OSes (which
	# breaks cross-building for different combinations on CI in the same container).
	cd web/ui && GO111MODULE=$(GO111MODULE) GOOS= GOARCH= $(GO) generate -x -v $(GOOPTS)
	@$(GOFMT) -w ./web/ui

.PHONY: test
# If we only want to only test go code we have to change the test target
# which is called by all.
ifeq ($(GO_ONLY),1)
test: common-test
else
test: common-test ui-build-module ui-test ui-lint
endif

.PHONY: npm_licenses
npm_licenses: ui-install
	@echo ">> bundling npm licenses"
	rm -f $(REACT_APP_NPM_LICENSES_TARBALL)
	find $(UI_NODE_MODULES_PATH) -iname "license*" | tar cfj $(REACT_APP_NPM_LICENSES_TARBALL) --transform 's/^/npm_licenses\//' --files-from=-

.PHONY: tarball
tarball: npm_licenses common-tarball

.PHONY: docker
docker: npm_licenses common-docker

.PHONY: build
build: assets common-build

.PHONY: bench_tsdb
bench_tsdb: $(PROMU)
	@echo ">> building promtool"
	@GO111MODULE=$(GO111MODULE) $(PROMU) build --prefix $(PREFIX) promtool
	@echo ">> running benchmark, writing result to $(TSDB_BENCHMARK_OUTPUT_DIR)"
	@$(PROMTOOL) tsdb bench write --metrics=$(TSDB_BENCHMARK_NUM_METRICS) --out=$(TSDB_BENCHMARK_OUTPUT_DIR) $(TSDB_BENCHMARK_DATASET)
	@$(GO) tool pprof -svg $(PROMTOOL) $(TSDB_BENCHMARK_OUTPUT_DIR)/cpu.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/cpuprof.svg
	@$(GO) tool pprof --inuse_space -svg $(PROMTOOL) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.inuse.svg
	@$(GO) tool pprof --alloc_space -svg $(PROMTOOL) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.alloc.svg
	@$(GO) tool pprof -svg $(PROMTOOL) $(TSDB_BENCHMARK_OUTPUT_DIR)/block.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/blockprof.svg
	@$(GO) tool pprof -svg $(PROMTOOL) $(TSDB_BENCHMARK_OUTPUT_DIR)/mutex.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/mutexprof.svg
