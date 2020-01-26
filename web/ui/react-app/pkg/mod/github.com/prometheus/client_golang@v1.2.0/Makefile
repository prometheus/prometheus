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

# http.CloseNotifier is deprecated but we don't want to remove support
# from client_golang to not break anybody still using it.
STATICCHECK_IGNORE = \
  github.com/prometheus/client_golang/prometheus/promhttp/delegator*.go:SA1019 \
  github.com/prometheus/client_golang/prometheus/promhttp/instrument_server_test.go:SA1019 \
  github.com/prometheus/client_golang/prometheus/http.go:SA1019

.PHONY: test
test: deps common-test

.PHONY: test-short
test-short: deps common-test-short
