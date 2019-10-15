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

TSDB_PROJECT_DIR = "./tsdb"
TSDB_CLI_DIR="$(TSDB_PROJECT_DIR)/cmd/tsdb"
TSDB_BIN = "$(TSDB_CLI_DIR)/tsdb"
TSDB_BENCHMARK_NUM_METRICS ?= 1000
TSDB_BENCHMARK_DATASET ?= "$(TSDB_PROJECT_DIR)/testdata/20kseries.json"
TSDB_BENCHMARK_OUTPUT_DIR ?= "$(TSDB_CLI_DIR)/benchout"

.PHONY: all
all: common-all check_assets

include Makefile.common

DOCKER_IMAGE_NAME       ?= prometheus

.PHONY: assets
assets:
	@echo ">> writing assets"
	cd $(PREFIX)/web/ui && GO111MODULE=$(GO111MODULE) $(GO) generate -x -v $(GOOPTS)
	@$(GOFMT) -w ./web/ui

.PHONY: check_assets
check_assets: assets
	@echo ">> checking that assets are up-to-date"
	@if ! (cd $(PREFIX)/web/ui && git diff --exit-code); then \
		echo "Run 'make assets' and commit the changes to fix the error."; \
		exit 1; \
	fi

.PHONY: build_tsdb
build_tsdb:
	GO111MODULE=$(GO111MODULE) $(GO) build -o $(TSDB_BIN) $(TSDB_CLI_DIR)

.PHONY: bench_tsdb
bench_tsdb: build_tsdb
	@echo ">> running benchmark, writing result to $(TSDB_BENCHMARK_OUTPUT_DIR)"
	@$(TSDB_BIN) bench write --metrics=$(TSDB_BENCHMARK_NUM_METRICS) --out=$(TSDB_BENCHMARK_OUTPUT_DIR) $(TSDB_BENCHMARK_DATASET)
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/cpu.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/cpuprof.svg
	@$(GO) tool pprof --inuse_space -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.inuse.svg
	@$(GO) tool pprof --alloc_space -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mem.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/memprof.alloc.svg
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/block.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/blockprof.svg
	@$(GO) tool pprof -svg $(TSDB_BIN) $(TSDB_BENCHMARK_OUTPUT_DIR)/mutex.prof > $(TSDB_BENCHMARK_OUTPUT_DIR)/mutexprof.svg
