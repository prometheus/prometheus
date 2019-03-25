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
  github.com/prometheus/prometheus/pkg/textparse/promlex.l.go:SA4006 \
  github.com/prometheus/prometheus/pkg/textparse/openmetricslex.l.go:SA4006

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
