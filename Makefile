# Copyright The Prometheus Authors
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

.PHONY: update-npm-deps
update-npm-deps:
	@echo ">> updating npm dependencies"
	./scripts/npm-deps.sh "minor"

.PHONY: upgrade-npm-deps
upgrade-npm-deps:
	@echo ">> upgrading npm dependencies"
	./scripts/npm-deps.sh "latest"

.PHONY: ui-bump-version
ui-bump-version:
	version=$$(./scripts/get_module_version.sh) && ./scripts/ui_release.sh --bump-version "$${version}"
	cd web/ui && npm install
	git add "./web/ui/package-lock.json" "./**/package.json"

.PHONY: ui-install
ui-install:
	cd $(UI_PATH) && npm install
	# The old React app has been separated from the npm workspaces setup to avoid
	# issues with conflicting dependencies. This is a temporary solution until the
	# new Mantine-based UI is fully integrated and the old app can be removed.
	cd $(UI_PATH)/react-app && npm install

.PHONY: ui-build
ui-build:
	cd $(UI_PATH) && CI="" npm run build

.PHONY: ui-build-module
ui-build-module:
	cd $(UI_PATH) && npm run build:module

.PHONY: ui-test
ui-test:
	cd $(UI_PATH) && CI=true npm run test

.PHONY: ui-lint
ui-lint:
	cd $(UI_PATH) && npm run lint
	# The old React app has been separated from the npm workspaces setup to avoid
	# issues with conflicting dependencies. This is a temporary solution until the
	# new Mantine-based UI is fully integrated and the old app can be removed.
	cd $(UI_PATH)/react-app && npm run lint

.PHONY: generate-promql-functions
generate-promql-functions: ui-install
	@echo ">> generating PromQL function signatures"
	@cd $(UI_PATH)/mantine-ui/src/promql/tools && $(GO) run ./gen_functions_list > ../functionSignatures.ts
	@echo ">> generating PromQL function documentation"
	@cd $(UI_PATH)/mantine-ui/src/promql/tools && $(GO) run ./gen_functions_docs $(CURDIR)/docs/querying/functions.md > ../functionDocs.tsx
	@echo ">> formatting generated files"
	@cd $(UI_PATH)/mantine-ui && npx prettier --write --print-width 120 src/promql/functionSignatures.ts src/promql/functionDocs.tsx

.PHONY: check-generated-promql-functions
check-generated-promql-functions: generate-promql-functions
	@echo ">> checking generated PromQL functions"
	@git diff --exit-code -- $(UI_PATH)/mantine-ui/src/promql/functionSignatures.ts $(UI_PATH)/mantine-ui/src/promql/functionDocs.tsx || (echo "Generated PromQL function files are out of date. Please run 'make generate-promql-functions' and commit the changes." && false)

.PHONY: assets
ifndef SKIP_UI_BUILD
assets: check-node-version ui-install ui-build

.PHONY: npm_licenses
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
assets-compress: assets
	@echo '>> compressing assets'
	scripts/compress_assets.sh

.PHONY: assets-tarball
assets-tarball: assets
	@echo '>> packaging assets'
	scripts/package_assets.sh

.PHONY: parser
parser:
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
clean-parser:
	@echo ">> cleaning generated parser"
	@rm -f promql/parser/generated_parser.y.go

.PHONY: check-generated-parser
check-generated-parser: clean-parser promql/parser/generated_parser.y.go
	@echo ">> checking generated parser"
	@git diff --exit-code -- promql/parser/generated_parser.y.go || (echo "Generated parser is out of date. Please run 'make parser' and commit the changes." && false)

.PHONY: install-goyacc
install-goyacc:
	@echo ">> installing goyacc $(GOYACC_VERSION)"
	@go install golang.org/x/tools/cmd/goyacc@$(GOYACC_VERSION)

.PHONY: test
# If we only want to only test go code we have to change the test target
# which is called by all.
ifeq ($(GO_ONLY),1)
test: common-test check-go-mod-version
else
test: check-generated-parser common-test check-node-version ui-build-module ui-test ui-lint check-go-mod-version
endif

.PHONY: tarball
tarball: npm_licenses common-tarball

.PHONY: docker
docker: npm_licenses common-docker

.PHONY: build
build: assets npm_licenses assets-compress common-build

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

.PHONY: cli-documentation
cli-documentation:
	$(GO) run ./cmd/prometheus/ --write-documentation > docs/command-line/prometheus.md
	$(GO) run ./cmd/promtool/ write-documentation > docs/command-line/promtool.md

.PHONY: check-go-mod-version
check-go-mod-version:
	@echo ">> checking go.mod version matching"
	@./scripts/check-go-mod-version.sh

.PHONY: update-features-testdata
update-features-testdata:
	@echo ">> updating features testdata"
	@$(GO) test ./cmd/prometheus -run TestFeaturesAPI -update-features

GO_SUBMODULE_DIRS := documentation/examples/remote_storage internal/tools web/ui/mantine-ui/src/promql/tools

.PHONY: update-all-go-deps
update-all-go-deps: update-go-deps
	$(foreach dir,$(GO_SUBMODULE_DIRS),$(MAKE) update-go-deps-in-dir DIR=$(dir);)
	@echo ">> syncing Go workspace"
	@$(GO) work sync

.PHONY: update-go-deps-in-dir
update-go-deps-in-dir:
	@echo ">> updating Go dependencies in ./$(DIR)/"
	@cd ./$(DIR) && for m in $$($(GO) list -mod=readonly -m -f '{{ if and (not .Indirect) (not .Main)}}{{.Path}}{{end}}' all); do \
		$(GO) get $$m; \
	done
	@cd ./$(DIR) && $(GO) mod tidy

.PHONY: check-node-version
check-node-version:
	@./scripts/check-node-version.sh

.PHONY: bump-go-version
bump-go-version:
	@echo ">> bumping Go minor version"
	@./scripts/bump_go_version.sh
