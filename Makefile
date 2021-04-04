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

REACT_APP_PATH = web/ui/react-app
REACT_APP_SOURCE_FILES = $(shell find $(REACT_APP_PATH)/public/ $(REACT_APP_PATH)/src/ $(REACT_APP_PATH)/tsconfig.json)
REACT_APP_OUTPUT_DIR = web/ui/static/react
REACT_APP_NODE_MODULES_PATH = $(REACT_APP_PATH)/node_modules
REACT_APP_NPM_LICENSES_TARBALL = "npm_licenses.tar.bz2"
REACT_APP_BUILD_SCRIPT = ./scripts/build_react_app.sh

PROMTOOL = ./promtool
TSDB_BENCHMARK_NUM_METRICS ?= 1000
TSDB_BENCHMARK_DATASET ?= ./tsdb/testdata/20kseries.json
TSDB_BENCHMARK_OUTPUT_DIR ?= ./benchout

GOLANGCI_LINT_OPTS ?= --timeout 5m

include Makefile.common

DOCKER_IMAGE_NAME       ?= prometheus

$(REACT_APP_NODE_MODULES_PATH): $(REACT_APP_PATH)/package.json $(REACT_APP_PATH)/yarn.lock
	cd $(REACT_APP_PATH) && yarn --frozen-lockfile

$(REACT_APP_OUTPUT_DIR): $(REACT_APP_NODE_MODULES_PATH) $(REACT_APP_SOURCE_FILES) $(REACT_APP_BUILD_SCRIPT)
	@echo ">> building React app"
	@$(REACT_APP_BUILD_SCRIPT)

.PHONY: assets
assets: $(REACT_APP_OUTPUT_DIR)
	@echo ">> writing assets"
	# Un-setting GOOS and GOARCH here because the generated Go code is always the same,
	# but the cached object code is incompatible between architectures and OSes (which
	# breaks cross-building for different combinations on CI in the same container).
	cd web/ui && GO111MODULE=$(GO111MODULE) GOOS= GOARCH= $(GO) generate -x -v $(GOOPTS)
	@$(GOFMT) -w ./web/ui

.PHONY: react-app-lint
react-app-lint:
	@echo ">> running React app linting"
	cd $(REACT_APP_PATH) && yarn lint:ci

.PHONY: react-app-lint-fix
react-app-lint-fix:
	@echo ">> running React app linting and fixing errors where possible"
	cd $(REACT_APP_PATH) && yarn lint

.PHONY: react-app-test
react-app-test: | $(REACT_APP_NODE_MODULES_PATH) react-app-lint
	@echo ">> running React app tests"
	cd $(REACT_APP_PATH) && yarn test --no-watch --coverage

.PHONY: test
test: common-test react-app-test

.PHONY: npm_licenses
npm_licenses: $(REACT_APP_NODE_MODULES_PATH)
	@echo ">> bundling npm licenses"
	rm -f $(REACT_APP_NPM_LICENSES_TARBALL)
	find $(REACT_APP_NODE_MODULES_PATH) -iname "license*" | tar cfj $(REACT_APP_NPM_LICENSES_TARBALL) --transform 's/^/npm_licenses\//' --files-from=-

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
