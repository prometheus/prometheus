PREFIX            ?= $(shell pwd)
FILES_TO_FMT      ?= $(shell find . -path ./vendor -prune -o -name '*.go' -print)

# Ensure everything works even if GOPATH is not set, which is often the case.
# The `go env GOPATH` will work for all cases for Go 1.8+.
GOPATH            ?= $(shell go env GOPATH)

TMP_GOPATH        ?= /tmp/flagarize-go
GOBIN             ?= $(firstword $(subst :, ,${GOPATH}))/bin
GO111MODULE       ?= on
export GO111MODULE
GOPROXY           ?= https://proxy.golang.org
export GOPROXY

# Tools.
EMBEDMD           ?= $(GOBIN)/embedmd-$(EMBEDMD_VERSION)
# v2.0.0
EMBEDMD_VERSION   ?= 97c13d6e41602fc6e397eb51c45f38069371a969
LICHE             ?= $(GOBIN)/liche-$(LICHE_VERSION)
LICHE_VERSION     ?= 2a2e6e56f6c615c17b2e116669c4cdb31b5453f3
GOIMPORTS         ?= $(GOBIN)/goimports-$(GOIMPORTS_VERSION)
GOIMPORTS_VERSION ?= 9d4d845e86f14303813298ede731a971dd65b593
GIT               ?= $(shell which git)
GOLANGCILINT_VERSION ?= d2b1eea2c6171a1a1141a448a745335ce2e928a1
GOLANGCILINT         ?= $(GOBIN)/golangci-lint-$(GOLANGCILINT_VERSION)
MISSPELL_VERSION     ?= c0b55c8239520f6b5aa15a0207ca8b28027ba49e
MISSPELL             ?= $(GOBIN)/misspell-$(MISSPELL_VERSION)
FUNCBENCH_VERSION       ?= 36bc2803457da1bb58ae220523c78ed229834546
FUNCBENCH               ?= $(GOBIN)/funcbench-$(FUNCBENCH_VERSION)
BENCH_FUNC		?="*"

# Support gsed on OSX (installed via brew), falling back to sed. On Linux
# systems gsed won't be installed, so will use sed as expected.
SED ?= $(shell which gsed 2>/dev/null || which sed)

ME                ?= $(shell whoami)

FAILLINT_VERSION        ?= v1.2.0
FAILLINT                ?=$(GOBIN)/faillint-$(FAILLINT_VERSION)

# fetch_go_bin_version downloads (go gets) the binary from specific version and installs it in $(GOBIN)/<bin>-<version>
# arguments:
# $(1): Install path. (e.g github.com/campoy/embedmd)
# $(2): Tag or revision for checkout.
define fetch_go_bin_version
	@mkdir -p $(GOBIN)
	@mkdir -p $(TMP_GOPATH)

	@echo ">> fetching $(1)@$(2) revision/version"
	@if [ ! -d '$(TMP_GOPATH)/src/$(1)' ]; then \
    GOPATH='$(TMP_GOPATH)' GO111MODULE='off' go get -d -u '$(1)/...'; \
  else \
    CDPATH='' cd -- '$(TMP_GOPATH)/src/$(1)' && git fetch; \
  fi
	@CDPATH='' cd -- '$(TMP_GOPATH)/src/$(1)' && git checkout -f -q '$(2)'
	@echo ">> installing $(1)@$(2)"
	@GOBIN='$(TMP_GOPATH)/bin' GOPATH='$(TMP_GOPATH)' GO111MODULE='off' go install '$(1)'
	@mv -- '$(TMP_GOPATH)/bin/$(shell basename $(1))' '$(GOBIN)/$(shell basename $(1))-$(2)'
	@echo ">> produced $(GOBIN)/$(shell basename $(1))-$(2)"

endef

# fetch_go_bin_version_mod downloads (go gets) the binary from specific version and installs it in $(GOBIN)/<bin>-<version>
# arguments:
# $(1): Install path. (e.g github.com/campoy/embedmd)
# $(2): Tag or revision for checkout.
define fetch_go_bin_version_mod
	@mkdir -p $(GOBIN)
	@mkdir -p $(TMP_GOPATH)

	@echo ">> fetching $(1)@$(2) revision/version"
	@if [ ! -d '$(TMP_GOPATH)/src/$(1)' ]; then \
    GOPATH='$(TMP_GOPATH)' GO111MODULE='off' go get -d -u '$(1)/...'; \
  else \
    CDPATH='' cd -- '$(TMP_GOPATH)/src/$(1)' && git fetch; \
  fi
	@CDPATH='' cd -- '$(TMP_GOPATH)/src/$(1)' && git checkout -f -q '$(2)'
	@echo ">> installing $(1)@$(2)"
	@ # Extra step for those who does not vendor deps.
	@CDPATH='' cd -- '$(TMP_GOPATH)/src/$(1)' && go mod vendor
	@GOBIN='$(TMP_GOPATH)/bin' GOPATH='$(TMP_GOPATH)' GO111MODULE='off' go install '$(1)'
	@mv -- '$(TMP_GOPATH)/bin/$(shell basename $(1))' '$(GOBIN)/$(shell basename $(1))-$(2)'
	@echo ">> produced $(GOBIN)/$(shell basename $(1))-$(2)"

endef


define require_clean_work_tree
	@git update-index -q --ignore-submodules --refresh

    @if ! git diff-files --quiet --ignore-submodules --; then \
        echo >&2 "cannot $1: you have unstaged changes."; \
        git diff-files --name-status -r --ignore-submodules -- >&2; \
        echo >&2 "Please commit or stash them."; \
        exit 1; \
    fi

    @if ! git diff-index --cached --quiet HEAD --ignore-submodules --; then \
        echo >&2 "cannot $1: your index contains uncommitted changes."; \
        git diff-index --cached --name-status -r --ignore-submodules HEAD -- >&2; \
        echo >&2 "Please commit or stash them."; \
        exit 1; \
    fi

endef

help: ## Displays help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: all
all: format build

.PHONY: bench
bench: ## Run $(BENCH_FUNC) benchmarks and compare with master using https://github.com/prometheus/test-infra/tree/master/funcbench.
bench: $(FUNCBENCH)
	@echo ">> benchmark $(BENCH_FUNC) and compare with master"
	@$(FUNCBENCH) -v --bench-time=30s --timeout=2h --result-cache=/tmp/cache master '"$(BENCH_FUNC)"'

.PHONY: bench-master
bench-master: ## Run $(BENCH_FUNC) benchmarks and compare with newest using https://github.com/prometheus/test-infra/tree/master/funcbench.
bench-master: $(FUNCBENCH)
	@echo ">> benchmark $(BENCH_FUNC) and compare with: $(shell git tag | grep -E ^v[0-9]+\.[0-9]\.[0-9]+$$ | sort -t . -k1,1n -k2,2n -k3,3n | tail -1)"
	@$(FUNCBENCH) -v --bench-time=30s --timeout=2h --result-cache=/tmp/cache  "$(shell git tag | grep -E ^v[0-9]+\.[0-9]\.[0-9]+$$ | sort -t . -k1,1n -k2,2n -k3,3n | tail -1)" '"$(BENCH_FUNC)"'

.PHONY: bench-release
bench-release: ## Run $(BENCH_FUNC) benchmarks and compare with older release using https://github.com/prometheus/test-infra/tree/master/funcbench.
bench-release: $(FUNCBENCH)
	@echo ">> benchmark $(BENCH_FUNC) and compare with"
	@$(FUNCBENCH) -v --bench-time=30s --timeout=2h --result-cache=/tmp/cache "$(shell git tag | grep -E ^v[0-9]+\.[0-9]\.[0-9]+$$ | sort -t . -k1,1n -k2,2n -k3,3n | grep -B1 $(CURRENT_RELEASE) | head -1)" '"$(BENCH_FUNC)"'

.PHONY: deps
deps: ## Ensures fresh go.mod and go.sum.
	@go mod tidy
	@go mod verify

.PHONY: check-comments
check-comments: ## Checks Go code comments if they have trailing period (excludes protobuffers and vendor files). Comments with more than 3 spaces at beginning are omitted from the check, example: '//    - foo'.
	@printf ">> checking Go comments trailing periods\n\n\n"
	@./scripts/build-check-comments.sh

.PHONY: format
format: ## Formats Go code including imports and cleans up white noise.
format: $(GOIMPORTS) check-comments
	@echo ">> formatting code"
	@$(GOIMPORTS) -w $(FILES_TO_FMT)
	@SED_BIN="$(SED)" scripts/cleanup-white-noise.sh $(FILES_TO_FMT)

.PHONY: test
test: ## Runs all Go unit tests.
test:
	@echo ">> running unit tests"
	@go test $(shell go list ./... | grep -v /vendor/);

.PHONY: check-git
check-git:
ifneq ($(GIT),)
	@test -x $(GIT) || (echo >&2 "No git executable binary found at $(GIT)."; exit 1)
else
	@echo >&2 "No git binary found."; exit 1
endif

# PROTIP:
# Add
#      --cpu-profile-path string   Path to CPU profile output file
#      --mem-profile-path string   Path to memory profile output file
# to debug big allocations during linting.
lint: ## Runs various static analysis against our code.
lint: check-git deps $(GOLANGCILINT) $(MISSPELL) $(FAILLINT)
	$(call require_clean_work_tree,"detected not clean master before running lint")
	@echo ">> verifying modules being imported"
	@$(FAILLINT) -paths "errors=github.com/pkg/errors,github.com/thanos-io/thanos/pkg/testutil=github.com/bwplotka/flagarize/testutil" ./...
	@$(FAILLINT) -paths "fmt.{Print,PrintfPrintln,Sprint}" -ignore-tests ./...
	@echo ">> examining all of the Go files"
	@go vet -stdmethods=false ./...
	@echo ">> linting all of the Go files GOGC=${GOGC}"
	@$(GOLANGCILINT) run
	@echo ">> detecting misspells"
	@find . -type f | grep -v vendor/ | grep -vE '\./\..*' | xargs $(MISSPELL) -error
	@echo ">> detecting white noise"
	@find . -type f \( -name "*.md" -o -name "*.go" \) | SED_BIN="$(SED)" xargs scripts/cleanup-white-noise.sh
	$(call require_clean_work_tree,"detected white noise")
	@echo ">> ensuring Copyright headers"
	@go run ./scripts/copyright
	$(call require_clean_work_tree,"detected files without copyright")

$(EMBEDMD):
	$(call fetch_go_bin_version,github.com/campoy/embedmd,$(EMBEDMD_VERSION))

$(GOIMPORTS):
	$(call fetch_go_bin_version,golang.org/x/tools/cmd/goimports,$(GOIMPORTS_VERSION))

$(LICHE):
	$(call fetch_go_bin_version,github.com/raviqqe/liche,$(LICHE_VERSION))

$(GOLANGCILINT):
	$(call fetch_go_bin_version,github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCILINT_VERSION))

$(MISSPELL):
	$(call fetch_go_bin_version,github.com/client9/misspell/cmd/misspell,$(MISSPELL_VERSION))

$(FAILLINT):
	$(call fetch_go_bin_version,github.com/fatih/faillint,$(FAILLINT_VERSION))

$(FUNCBENCH):
	$(call fetch_go_bin_version_mod,github.com/prometheus/test-infra/funcbench,$(FUNCBENCH_VERSION))
