# Copyright 2015 The Prometheus Authors
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

# Ensure GOBIN is not set during build so that promu is installed to the correct path
unexport GOBIN

GO           ?= go
GOFMT        ?= $(GO)fmt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
PROMU        := $(FIRST_GOPATH)/bin/promu
STATICCHECK  := $(FIRST_GOPATH)/bin/staticcheck
GOVENDOR     := $(FIRST_GOPATH)/bin/govendor
pkgs          = $(shell $(GO) list ./... | grep -v /vendor/)

PREFIX                  ?= $(shell pwd)
BIN_DIR                 ?= $(shell pwd)
DOCKER_IMAGE_NAME       ?= prometheus
DOCKER_IMAGE_TAG        ?= $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))

ifdef DEBUG
	bindata_flags = -debug
endif

STATICCHECK_IGNORE = \
  github.com/prometheus/prometheus/discovery/kubernetes/kubernetes.go:SA1019 \
  github.com/prometheus/prometheus/discovery/kubernetes/node.go:SA1019 \
  github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_adapter/main.go:SA1019 \
  github.com/prometheus/prometheus/pkg/textparse/lex.l.go:SA4006 \
  github.com/prometheus/prometheus/pkg/pool/pool.go:SA6002 \
  github.com/prometheus/prometheus/promql/engine.go:SA6002

all: format staticcheck unused build test

style:
	@echo ">> checking code style"
	@! $(GOFMT) -d $(shell find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

check_license:
	@echo ">> checking license header"
	@./scripts/check_license.sh

# TODO(fabxc): example tests temporarily removed.
test-short:
	@echo ">> running short tests"
	@$(GO) test -short $(shell $(GO) list ./... | grep -v /vendor/ | grep -v examples)

test:
	@echo ">> running all tests"
	@$(GO) test -race $(shell $(GO) list ./... | grep -v /vendor/ | grep -v examples)

format:
	@echo ">> formatting code"
	@$(GO) fmt $(pkgs)

vet:
	@echo ">> vetting code"
	@$(GO) vet $(pkgs)

staticcheck: $(STATICCHECK)
	@echo ">> running staticcheck"
	@$(STATICCHECK) -ignore "$(STATICCHECK_IGNORE)" $(pkgs)

unused: $(GOVENDOR)
	@echo ">> running check for unused packages"
	@$(GOVENDOR) list +unused

build: promu
	@echo ">> building binaries"
	@$(PROMU) build --prefix $(PREFIX)

tarball: promu
	@echo ">> building release tarball"
	@$(PROMU) tarball --prefix $(PREFIX) $(BIN_DIR)

docker:
	@echo ">> building docker image"
	@docker build -t "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" .

assets:
	@echo ">> writing assets"
	@$(GO) get -u github.com/jteeuwen/go-bindata/...
	@go-bindata $(bindata_flags) -pkg ui -o web/ui/bindata.go -ignore '(.*\.map|bootstrap\.js|bootstrap-theme\.css|bootstrap\.css)'  web/ui/templates/... web/ui/static/...
	@$(GO) fmt ./web/ui

promu:
	@echo ">> fetching promu"
	@GOOS= GOARCH= $(GO) get -u github.com/prometheus/promu

$(FIRST_GOPATH)/bin/staticcheck:
	@GOOS= GOARCH= $(GO) get -u honnef.co/go/tools/cmd/staticcheck

$(FIRST_GOPATH)/bin/govendor:
	@GOOS= GOARCH= $(GO) get -u github.com/kardianos/govendor

.PHONY: all style check_license format build test vet assets tarball docker promu staticcheck $(FIRST_GOPATH)/bin/staticcheck govendor $(FIRST_GOPATH)/bin/govendor
