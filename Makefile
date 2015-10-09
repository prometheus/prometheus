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

GO   := GO15VENDOREXPERIMENT=1 go
pkgs  = $(shell $(GO) list ./... | grep -v /vendor/)

ifdef DEBUG
	bindata_flags = -debug
endif


all: format build test

style:
	@echo ">> checking code style"
	@! gofmt -d **/*.go | grep '^'

test:
	@echo ">> running tests"
	@$(GO) test -short $(pkgs)

format:
	@echo ">> formatting code"
	@$(GO) fmt $(pkgs)

vet:
	@echo ">> vetting code"
	@$(GO) vet $(pkgs)

build:
	@echo ">> building binaries"
	@./scripts/build.sh

tarballs:
	@echo ">> building release tarballs"
	@./scripts/release_tarballs.sh

docker:
	@docker build -t prometheus:$(shell git rev-parse --short HEAD) .

assets:
	@echo ">> writing assets"
	@$(GO) get -u github.com/jteeuwen/go-bindata/...
	@go-bindata $(bindata_flags) -pkg ui -o web/ui/bindata.go -ignore '(.*\.map|bootstrap\.js|bootstrap-theme\.css|bootstrap\.css)'  web/ui/templates/... web/ui/static/...


.PHONY: all style format build test vet docker assets tarballs

