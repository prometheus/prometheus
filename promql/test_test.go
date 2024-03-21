// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestLazyLoader_WithSamplesTill(t *testing.T) {
	type testCase struct {
		ts             time.Time
		series         []Series // Each series is checked separately. Need not mention all series here.
		checkOnlyError bool     // If this is true, series is not checked.
	}

	cases := []struct {
		loadString string
		// These testCases are run in sequence. So the testCase being run is dependent on the previous testCase.
		testCases []testCase
	}{
		{
			loadString: `
				load 10s
					metric1 1+1x10
			`,
			testCases: []testCase{
				{
					ts: time.Unix(40, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []FPoint{
								{0, 1}, {10000, 2}, {20000, 3}, {30000, 4}, {40000, 5},
							},
						},
					},
				},
				{
					ts: time.Unix(10, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []FPoint{
								{0, 1}, {10000, 2}, {20000, 3}, {30000, 4}, {40000, 5},
							},
						},
					},
				},
				{
					ts: time.Unix(60, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []FPoint{
								{0, 1}, {10000, 2}, {20000, 3}, {30000, 4}, {40000, 5}, {50000, 6}, {60000, 7},
							},
						},
					},
				},
			},
		},
		{
			loadString: `
				load 10s
					metric1 1+0x5
					metric2 1+1x100
			`,
			testCases: []testCase{
				{ // Adds all samples of metric1.
					ts: time.Unix(70, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []FPoint{
								{0, 1}, {10000, 1}, {20000, 1}, {30000, 1}, {40000, 1}, {50000, 1},
							},
						},
						{
							Metric: labels.FromStrings("__name__", "metric2"),
							Floats: []FPoint{
								{0, 1}, {10000, 2}, {20000, 3}, {30000, 4}, {40000, 5}, {50000, 6}, {60000, 7}, {70000, 8},
							},
						},
					},
				},
				{ // This tests fix for https://github.com/prometheus/prometheus/issues/5064.
					ts:             time.Unix(300, 0),
					checkOnlyError: true,
				},
			},
		},
	}

	for _, c := range cases {
		suite, err := NewLazyLoader(t, c.loadString, LazyLoaderOpts{})
		require.NoError(t, err)
		defer suite.Close()

		for _, tc := range c.testCases {
			suite.WithSamplesTill(tc.ts, func(err error) {
				require.NoError(t, err)
				if tc.checkOnlyError {
					return
				}

				// Check the series.
				queryable := suite.Queryable()
				querier, err := queryable.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				for _, s := range tc.series {
					var matchers []*labels.Matcher
					s.Metric.Range(func(label labels.Label) {
						m, err := labels.NewMatcher(labels.MatchEqual, label.Name, label.Value)
						require.NoError(t, err)
						matchers = append(matchers, m)
					})

					// Get the series for the matcher.
					ss := querier.Select(suite.Context(), false, nil, matchers...)
					require.True(t, ss.Next())
					storageSeries := ss.At()
					require.False(t, ss.Next(), "Expecting only 1 series")

					// Convert `storage.Series` to `promql.Series`.
					got := Series{
						Metric: storageSeries.Labels(),
					}
					it := storageSeries.Iterator(nil)
					for it.Next() == chunkenc.ValFloat {
						t, v := it.At()
						got.Floats = append(got.Floats, FPoint{T: t, F: v})
					}
					require.NoError(t, it.Err())

					require.Equal(t, s, got)
				}
			})
		}
	}
}

func TestRunTest(t *testing.T) {
	testData := `
load 5m
	http_requests{job="api-server", instance="0", group="production"}	0+10x10
	http_requests{job="api-server", instance="1", group="production"}	0+20x10
	http_requests{job="api-server", instance="0", group="canary"}		0+30x10
	http_requests{job="api-server", instance="1", group="canary"}		0+40x10
`

	testCases := map[string]struct {
		input         string
		expectedError string
	}{
		"instant query with expected result": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
	{group="canary"} 70
`,
		},
		"instant query with incorrect result": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
	{group="canary"} 80
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected 80 for {group="canary"} but got 70`,
		},
		"instant query, but result has an unexpected series": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): unexpected metric {group="canary"} in result`,
		},
		"instant query, but result is missing a series": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
	{group="canary"} 70
	{group="test"} 100
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected metric {group="test"} with 3: [100.000000] not found`,
		},
		"instant query expected to fail, and query fails": {
			input: `
load 5m
  testmetric1{src="a",dst="b"} 0
  testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
`,
		},
		"instant query expected to fail, but query succeeds": {
			input:         `eval_fail instant at 0s vector(0)`,
			expectedError: `expected error evaluating query "vector(0)" (line 1) but got none`,
		},
		"instant query with results expected to match provided order, and result is in expected order": {
			input: testData + `
eval_ordered instant at 50m sort(http_requests)
	http_requests{group="production", instance="0", job="api-server"} 100
	http_requests{group="production", instance="1", job="api-server"} 200
	http_requests{group="canary", instance="0", job="api-server"} 300
	http_requests{group="canary", instance="1", job="api-server"} 400
`,
		},
		"instant query with results expected to match provided order, but result is out of order": {
			input: testData + `
eval_ordered instant at 50m sort(http_requests)
	http_requests{group="production", instance="0", job="api-server"} 100
	http_requests{group="production", instance="1", job="api-server"} 200
	http_requests{group="canary", instance="1", job="api-server"} 400
	http_requests{group="canary", instance="0", job="api-server"} 300
`,
			expectedError: `error in eval sort(http_requests) (line 8): expected metric {__name__="http_requests", group="canary", instance="0", job="api-server"} with [300.000000] at position 4 but was at 3`,
		},
		"instant query with results expected to match provided order, but result has an unexpected series": {
			input: testData + `
eval_ordered instant at 50m sort(http_requests)
	http_requests{group="production", instance="0", job="api-server"} 100
	http_requests{group="production", instance="1", job="api-server"} 200
	http_requests{group="canary", instance="0", job="api-server"} 300
`,
			expectedError: `error in eval sort(http_requests) (line 8): unexpected metric {__name__="http_requests", group="canary", instance="1", job="api-server"} in result`,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			err := runTest(t, testCase.input, newTestEngine())

			if testCase.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, testCase.expectedError)
			}
		})
	}
}
