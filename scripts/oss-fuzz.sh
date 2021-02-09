#!/bin/bash -eu
# Copyright 2020 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
################################################################################

# Fetch dependencies.
go mod vendor

compile_go_fuzzer github.com/prometheus/prometheus/promql FuzzParseMetric fuzzParseMetric
compile_go_fuzzer github.com/prometheus/prometheus/promql FuzzParseOpenMetric fuzzParseOpenMetric
compile_go_fuzzer github.com/prometheus/prometheus/promql FuzzParseMetricSelector fuzzParseMetricSelector
compile_go_fuzzer github.com/prometheus/prometheus/promql FuzzParseExpr fuzzParseExpr
