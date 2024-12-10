// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql_test

import (
	"testing"

	"github.com/prometheus/prometheus/promql/promqltest"
)

// The "info" function is experimental. This is why we write those tests here for now instead of promqltest/testdata/info.test.
func TestInfo(t *testing.T) {
	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
	promqltest.RunTest(t, `
load 5m
  metric{instance="a", job="1", label="value"} 0 1 2
  metric_not_matching_target_info{instance="a", job="2", label="value"} 0 1 2
  metric_with_overlapping_label{instance="a", job="1", label="value", data="base"} 0 1 2
  target_info{instance="a", job="1", data="info", another_data="another info"} 1 1 1
  build_info{instance="a", job="1", build_data="build"} 1 1 1

# Include one info metric data label.
eval range from 0m to 10m step 5m info(metric, {data=~".+"})
  metric{data="info", instance="a", job="1", label="value"} 0 1 2

# Include all info metric data labels.
eval range from 0m to 10m step 5m info(metric)
  metric{data="info", instance="a", job="1", label="value", another_data="another info"} 0 1 2

# Try including all info metric data labels, but non-matching identifying labels.
eval range from 0m to 10m step 5m info(metric_not_matching_target_info)
  metric_not_matching_target_info{instance="a", job="2", label="value"} 0 1 2

# Try including a certain info metric data label with a non-matching matcher not accepting empty labels.
# Metric is ignored, due there being a data label matcher not matching empty labels,
# and there being no info series matches.
eval range from 0m to 10m step 5m info(metric, {non_existent=~".+"})

# Include a certain info metric data label together with a non-matching matcher accepting empty labels.
# Since the non_existent matcher matches empty labels, it's simply ignored when there's no match.
# XXX: This case has to include a matcher not matching empty labels, due the PromQL limitation
# that vector selectors have to contain at least one matcher not accepting empty labels.
# We might need another construct than vector selector to get around this limitation.
eval range from 0m to 10m step 5m info(metric, {data=~".+", non_existent=~".*"})
  metric{data="info", instance="a", job="1", label="value"} 0 1 2

# Info series data labels overlapping with those of base series are ignored.
eval range from 0m to 10m step 5m info(metric_with_overlapping_label)
  metric_with_overlapping_label{data="base", instance="a", job="1", label="value", another_data="another info"} 0 1 2

# Include data labels from target_info specifically.
eval range from 0m to 10m step 5m info(metric, {__name__="target_info"})
  metric{data="info", instance="a", job="1", label="value", another_data="another info"} 0 1 2

# Try to include all data labels from a non-existent info metric.
eval range from 0m to 10m step 5m info(metric, {__name__="non_existent"})
  metric{instance="a", job="1", label="value"} 0 1 2

# Try to include a certain data label from a non-existent info metric.
eval range from 0m to 10m step 5m info(metric, {__name__="non_existent", data=~".+"})

# Include data labels from build_info.
eval range from 0m to 10m step 5m info(metric, {__name__="build_info"})
  metric{instance="a", job="1", label="value", build_data="build"} 0 1 2

# Include data labels from build_info and target_info.
eval range from 0m to 10m step 5m info(metric, {__name__=~".+_info"})
  metric{instance="a", job="1", label="value", build_data="build", data="info", another_data="another info"} 0 1 2

# Info metrics themselves are ignored when it comes to enriching with info metric data labels.
eval range from 0m to 10m step 5m info(build_info, {__name__=~".+_info", build_data=~".+"})
  build_info{instance="a", job="1", build_data="build"} 1 1 1

clear

# Overlapping target_info series.
load 5m
  metric{instance="a", job="1", label="value"} 0 1 2
  target_info{instance="a", job="1", data="info", another_data="another info"} 1 1 _
  target_info{instance="a", job="1", data="updated info", another_data="another info"} _ _ 1

# Conflicting info series are resolved through picking the latest sample.
eval range from 0m to 10m step 5m info(metric)
  metric{data="info", instance="a", job="1", label="value", another_data="another info"} 0 1 _
  metric{data="updated info", instance="a", job="1", label="value", another_data="another info"} _ _ 2

clear

# Non-overlapping target_info series.
load 5m
  metric{instance="a", job="1", label="value"} 0 1 2
  target_info{instance="a", job="1", data="info"} 1 1 stale
  target_info{instance="a", job="1", data="updated info"} _ _ 1

# Include info metric data labels from a metric which data labels change over time.
eval range from 0m to 10m step 5m info(metric)
  metric{data="info", instance="a", job="1", label="value"} 0 1 _
  metric{data="updated info", instance="a", job="1", label="value"} _ _ 2

clear

# Info series selector matches histogram series, info metrics should be float type.
load 5m
  metric{instance="a", job="1", label="value"} 0 1 2
  histogram{instance="a", job="1"} {{schema:1 sum:3 count:22 buckets:[5 10 7]}}

eval_fail range from 0m to 10m step 5m info(metric, {__name__="histogram"})

clear

# Series with skipped scrape.
load 1m
  metric{instance="a", job="1", label="value"} 0 _ 2 3 4
  target_info{instance="a", job="1", data="info"} 1 _ 1 1 1

# Lookback works also for the info series.
eval range from 1m to 4m step 1m info(metric)
  metric{data="info", instance="a", job="1", label="value"} 0 2 3 4

# @ operator works also with info.
# Note that we pick the timestamp missing a sample, lookback should pick previous sample.
eval range from 1m to 4m step 1m info(metric @ 60)
  metric{data="info", instance="a", job="1", label="value"} 0 0 0 0

# offset operator works also with info.
eval range from 1m to 4m step 1m info(metric offset 1m)
  metric{data="info", instance="a", job="1", label="value"} 0 0 2 3
`, engine)
}
