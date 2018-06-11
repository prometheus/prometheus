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

include Makefile.common

STATICCHECK_IGNORE = \
  github.com/prometheus/prometheus/discovery/kubernetes/kubernetes.go:SA1019 \
  github.com/prometheus/prometheus/discovery/kubernetes/node.go:SA1019 \
  github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_adapter/main.go:SA1019 \
  github.com/prometheus/prometheus/pkg/textparse/lex.l.go:SA4006 \
  github.com/prometheus/prometheus/pkg/pool/pool.go:SA6002 \
  github.com/prometheus/prometheus/promql/engine.go:SA6002

DOCKER_IMAGE_NAME       ?= prometheus

ifdef DEBUG
	bindata_flags = -debug
endif

.PHONY: assets
assets:
	@echo ">> writing assets"
	@$(GO) get -u github.com/jteeuwen/go-bindata/...
	@go-bindata $(bindata_flags) -pkg ui -o web/ui/bindata.go -ignore '(.*\.map|bootstrap\.js|bootstrap-theme\.css|bootstrap\.css)'  web/ui/templates/... web/ui/static/...
	@$(GO) fmt ./web/ui
