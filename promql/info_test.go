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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestInfoFunc(t *testing.T) {
	const (
		load1 = `
load 10s
  metric{instance="a", job="1", label="value"} 0 1 2
  metric_not_matching_target_info{instance="a", job="2", label="value"} 0 1 2
  metric_with_overlapping_label{instance="a", job="1", label="value", data="base"} 0 1 2
  target_info{instance="a", job="1", data="info", another_data="another info"} 1 1 1
  build_info{instance="a", job="1", build_data="build"} 1 1 1
`
		// Overlapping target_info series.
		load2 = `
load 10s
  metric{instance="a", job="1", label="value"} 0 1 2
  target_info{instance="a", job="1", data="info", another_data="another info"} 1 1 _
  target_info{instance="a", job="1", data="updated info", another_data="another info"} _ _ 1
`
		// Non-overlapping target_info series.
		load3 = `
load 10s
  metric{instance="a", job="1", label="value"} 0 1 2
  target_info{instance="a", job="1", data="info"} 1 1 stale
  target_info{instance="a", job="1", data="updated info"} _ _ 1
`
	)

	testCases := []struct {
		name   string
		load   string
		query  string
		result promql.Matrix
		expErr error
	}{
		{
			name:  "include one info metric data label",
			load:  load1,
			query: `info(metric, {data=~".+"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "include all info metric data labels",
			load:  load1,
			query: `info(metric)`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"another_data", "another info",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "try including all info metric data labels, but non-matching identifying labels",
			load:  load1,
			query: `info(metric_not_matching_target_info)`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric_not_matching_target_info",
						"instance", "a",
						"job", "2",
						"label", "value",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "try including a certain info metric data label with a non-matching matcher not accepting empty labels",
			load:  load1,
			query: `info(metric, {non_existent=~".+"})`,
			// metric is ignored, due there being a data label matcher not matching empty labels,
			// and there being no info series matches.
			result: promql.Matrix{},
		},
		{
			// XXX: This case has to include a matcher not matching empty labels, due the PromQL limitation
			// that vector selectors have to contain at least one matcher not accepting empty labels.
			// We might need another construct than vector selector to get around this limitation.
			name:  "include a certain info metric data label together with a non-matching matcher accepting empty labels",
			load:  load1,
			query: `info(metric, {data=~".+", non_existent=~".*"})`,
			// Since the non_existent matcher matches empty labels, it's simply ignored when there's no match.
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "info series data labels overlapping with those of base series are ignored",
			load:  load1,
			query: `info(metric_with_overlapping_label)`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric_with_overlapping_label",
						"data", "base",
						"instance", "a",
						"job", "1",
						"label", "value",
						"another_data", "another info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "include data labels from target_info specifically",
			load:  load1,
			query: `info(metric, {__name__="target_info"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"another_data", "another info",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "try to include all data labels from a non-existent info metric",
			load:  load1,
			query: `info(metric, {__name__="non_existent"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:   "try to include a certain data label from a non-existent info metric",
			load:   load1,
			query:  `info(metric, {__name__="non_existent", data=~".+"})`,
			result: promql.Matrix{},
		},
		{
			name:  "include data labels from build_info",
			load:  load1,
			query: `info(metric, {__name__="build_info"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"build_data", "build",
						"instance", "a",
						"job", "1",
						"label", "value",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "include data labels from build_info and target_info",
			load:  load1,
			query: `info(metric, {__name__=~".+_info"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"another_data", "another info",
						"build_data", "build",
						"data", "info",
						"instance", "a",
						"job", "1",
						"label", "value",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "info metrics themselves are ignored when it comes to enriching with info metric data labels",
			load:  load1,
			query: `info(build_info, {__name__=~".+_info", build_data=~".+"})`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "build_info",
						"build_data", "build",
						"instance", "a",
						"job", "1",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 1,
						},
						{
							T: 10000,
							F: 1,
						},
						{
							T: 20000,
							F: 1,
						},
					},
				},
			},
		},
		{
			name:  "conflicting info series are resolved through picking the latest sample",
			load:  load2,
			query: `info(metric)`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"another_data", "another info",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
					},
				},
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"another_data", "another info",
						"data", "updated info",
					),
					Floats: []promql.FPoint{
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
		{
			name:  "include info metric data labels from a metric which data labels change over time",
			load:  load3,
			query: `info(metric)`,
			result: promql.Matrix{
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"data", "info",
					),
					Floats: []promql.FPoint{
						{
							T: 0,
							F: 0,
						},
						{
							T: 10000,
							F: 1,
						},
					},
				},
				promql.Series{
					Metric: labels.FromStrings(
						labels.MetricName, "metric",
						"instance", "a",
						"job", "1",
						"label", "value",
						"data", "updated info",
					),
					Floats: []promql.FPoint{
						{
							T: 20000,
							F: 2,
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
			ctx := context.Background()
			storage := promqltest.LoadedStorage(t, tc.load)
			t.Cleanup(func() { _ = storage.Close() })

			start := time.Unix(0, 0)
			end := time.Unix(20, 0)
			qry, err := engine.NewRangeQuery(ctx, storage, nil, tc.query, start, end, 10*time.Second)
			require.NoError(t, err)

			res := qry.Exec(ctx)
			if tc.expErr != nil {
				require.EqualError(t, res.Err, tc.expErr.Error())
				return
			}

			require.NoError(t, res.Err)
			require.Empty(t, res.Warnings)
			mat, ok := res.Value.(promql.Matrix)
			require.True(t, ok)
			testutil.RequireEqual(t, tc.result, mat)
		})
	}
}
