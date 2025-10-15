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
GOYACC_VERSION ?= v0.6.0

include Makefile.common

DOCKER_IMAGE_NAME       ?= prometheus

# Only build UI if PREBUILT_ASSETS_STATIC_DIR is not set
ifdef PREBUILT_ASSETS_STATIC_DIR
  SKIP_UI_BUILD = true
endif

.PHONY: help
help: ## Displays commands and their descriptions.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[38;2;230;82;44m<target>\033[0m\n\nMain Targets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[38;2;230;82;44m%-20s\033[0m %s\n", $$1, $$2 }' Makefile
	@echo ""
	@echo "Common Targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[38;2;230;82;44m%-20s\033[0m %s\n", $$1, $$2 }' Makefile.common

.PHONY: update-npm-deps
update-npm-deps: ## Update Prometheus web UI npm dependencies to minor versions.
	@echo ">> updating npm dependencies"
	./scripts/npm-deps.sh "minor"

.PHONY: upgrade-npm-deps
upgrade-npm-deps: ## Upgrade Prometheus web UI npm dependencies to latest versions.
	@echo ">> upgrading npm dependencies"
	./scripts/npm-deps.sh "latest"

.PHONY: ui-bump-version
ui-bump-version: ## Bump Prometheus web UI version and update package files.
	version=$$(./scripts/get_module_version.sh) && ./scripts/ui_release.sh --bump-version "$${version}"
	cd web/ui && npm install
	git add "./web/ui/package-lock.json" "./**/package.json"

.PHONY: ui-install
ui-install: ## Install npm dependencies for Prometheus web UI.
	cd $(UI_PATH) && npm install
	# The old React app has been separated from the npm workspaces setup to avoid
	# issues with conflicting dependencies. This is a temporary solution until the
	# new Mantine-based UI is fully integrated and the old app can be removed.
	cd $(UI_PATH)/react-app && npm install

.PHONY: ui-build
ui-build: ## Build Prometheus web UI assets.
	cd $(UI_PATH) && CI="" npm run build

.PHONY: ui-build-module
ui-build-module: ## Build Prometheus web UI as a module.
	cd $(UI_PATH) && npm run build:module

.PHONY: ui-test
ui-test: ## Run Prometheus web UI tests.
	cd $(UI_PATH) && CI=true npm run test

.PHONY: ui-lint
ui-lint: ## Lint Prometheus web UI code.
	cd $(UI_PATH) && npm run lint
	# The old React app has been separated from the npm workspaces setup to avoid
	# issues with conflicting dependencies. This is a temporary solution until the
	# new Mantine-based UI is fully integrated and the old app can be removed.
	cd $(UI_PATH)/react-app && npm run lint

.PHONY: assets
ifndef SKIP_UI_BUILD
assets: ## Build and embed Prometheus web UI assets (Skipped if using pre-built assets).
assets: check-node-version ui-install ui-build

.PHONY: npm_licenses
npm_licenses: ## Bundle npm license files for Prometheus web UI (Skipped if using pre-built assets).
npm_licenses: ui-install
	@echo ">> bundling npm licenses"
	rm -f $(REACT_APP_NPM_LICENSES_TARBALL) npm_licenses
	ln -s . npm_licenses
	find npm_licenses/$(UI_NODE_MODULES_PATH) -iname "license*" | tar cfj $(REACT_APP_NPM_LICENSES_TARBALL) --files-from=-
	rm -f npm_licenses
else
assets:
	@echo '>> skipping assets build, pre-built assets provided'

npm_licenses:
	@echo '>> skipping assets npm licenses, pre-built assets provided'
endif

.PHONY: assets-compress
assets-compress: ## Compress Prometheus web UI assets for distribution.
assets-compress: assets
	@echo '>> compressing assets'
	scripts/compress_assets.sh

.PHONY: assets-tarball
assets-tarball: ## Package Prometheus web UI assets into a tarball for distribution.
assets-tarball: assets
	@echo '>> packaging assets'
	scripts/package_assets.sh

.PHONY: parser
parser: ## Generate Prometheus PromQL parser Go code using goyacc.
	@echo ">> running goyacc to generate the .go file."
ifeq (, $(shell command -v goyacc 2> /dev/null))
	@echo "goyacc not installed so skipping"
	@echo "To install: \"go install golang.org/x/tools/cmd/goyacc@$(GOYACC_VERSION)\" or run \"make install-goyacc\""
else
	$(MAKE) promql/parser/generated_parser.y.go
endif

promql/parser/generated_parser.y.go: promql/parser/generated_parser.y
	@echo ">> running goyacc to generate the .go file."
	@$(FIRST_GOPATH)/bin/goyacc -l -o promql/parser/generated_parser.y.go promql/parser/generated_parser.y

.PHONY: clean-parser
clean-parser: ## Remove generated Prometheus PromQL parser files.
	@echo ">> cleaning generated parser"
	@rm -f promql/parser/generated_parser.y.go

.PHONY: check-generated-parser
check-generated-parser: clean-parser promql/parser/generated_parser.y.go ## Verify Prometheus PromQL parser is up to date.
	@echo ">> checking generated parser"
	@git diff --exit-code -- promql/parser/generated_parser.y.go || (echo "Generated parser is out of date. Please run 'make parser' and commit the changes." && false)

.PHONY: install-goyacc
install-goyacc: ## Install goyacc tool for Prometheus PromQL parser generation.
	@echo ">> installing goyacc $(GOYACC_VERSION)"
	@go install golang.org/x/tools/cmd/goyacc@$(GOYACC_VERSION)

.PHONY: test
# If we only want to only test go code we have to change the test target
# which is called by all.
ifeq ($(GO_ONLY),1)
test: common-test check-go-mod-version
else
test: ## Run all Prometheus tests including Go, UI, and parser checks (To test Go only set GO_ONLY=1).
test: check-generated-parser common-test check-node-version ui-build-module ui-test ui-lint check-go-mod-version
endif

.PHONY: tarball
tarball: ## Create Prometheus release tarball with licenses.
tarball: npm_licenses common-tarball

.PHONY: docker
docker: ## Build Prometheus Docker image with licenses.
docker: npm_licenses common-docker

plugins/plugins.go: plugins.yml plugins/generate.go
	@echo ">> creating plugins list"
	$(GO) generate -tags plugins ./plugins

.PHONY: plugins
plugins:  ## Generate Prometheus plugins list from plugins.yml.
plugins: plugins/plugins.go

.PHONY: build
build: ## Build complete Prometheus binary with UI, licenses, and plugins.
build: assets npm_licenses assets-compress plugins common-build

.PHONY: bench_tsdb
bench_tsdb: ## Run Prometheus TSDB benchmarks and generate profiling reports.
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

.PHONY: cli-documentation
cli-documentation: ## Generate Prometheus and promtool command-line documentation.
	$(GO) run ./cmd/prometheus/ --write-documentation > docs/command-line/prometheus.md
	$(GO) run ./cmd/promtool/ write-documentation > docs/command-line/promtool.md

.PHONY: check-go-mod-version
check-go-mod-version: ## Verify Prometheus go.mod version consistency.
	@echo ">> checking go.mod version matching"
	@./scripts/check-go-mod-version.sh

.PHONY: update-all-go-deps
update-all-go-deps: ## Update all Prometheus Go dependencies including examples.
	@$(MAKE) update-go-deps
	@echo ">> updating Go dependencies in ./documentation/examples/remote_storage/"
	@cd ./documentation/examples/remote_storage/ && for m in $$($(GO) list -mod=readonly -m -f '{{ if and (not .Indirect) (not .Main)}}{{.Path}}{{end}}' all); do \
		$(GO) get $$m; \
	done
	@cd ./documentation/examples/remote_storage/ && $(GO) mod tidy
.PHONY: check-node-version
check-node-version: ## Check Node Version
	@./scripts/check-node-version.sh

.PHONY: bump-go-version ## Update Go version to minor version
bump-go-version:
	@echo ">> bumping Go minor version"
	@./scripts/bump_go_version.sh

