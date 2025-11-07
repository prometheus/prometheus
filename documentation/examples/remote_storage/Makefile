# Copyright 2022 The Prometheus Authors
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

.PHONY: help
help: ## Displays commands and their descriptions.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[38;2;230;82;44m<target>\033[0m\n\nMain Targets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[38;2;230;82;44m%-20s\033[0m %s\n", $$1, $$2 }' Makefile
build: ## Build Prometheus remote storage adapter examples.
	@echo ">> building example_write_adapter"
	@$(GO) build -o example_write_adapter/example_write_adapter ./example_write_adapter
	@echo ">> building remote_storage_adapter"
	@$(GO) build -o remote_storage_adapter/remote_storage_adapter ./remote_storage_adapter
