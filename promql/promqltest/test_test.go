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

package promqltest

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestLazyLoader_WithSamplesTill(t *testing.T) {
	type testCase struct {
		ts             time.Time
		series         []promql.Series // Each series is checked separately. Need not mention all series here.
		checkOnlyError bool            // If this is true, series is not checked.
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
					series: []promql.Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []promql.FPoint{
								{T: 0, F: 1}, {T: 10000, F: 2}, {T: 20000, F: 3}, {T: 30000, F: 4}, {T: 40000, F: 5},
							},
						},
					},
				},
				{
					ts: time.Unix(10, 0),
					series: []promql.Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []promql.FPoint{
								{T: 0, F: 1}, {T: 10000, F: 2}, {T: 20000, F: 3}, {T: 30000, F: 4}, {T: 40000, F: 5},
							},
						},
					},
				},
				{
					ts: time.Unix(60, 0),
					series: []promql.Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []promql.FPoint{
								{T: 0, F: 1}, {T: 10000, F: 2}, {T: 20000, F: 3}, {T: 30000, F: 4}, {T: 40000, F: 5}, {T: 50000, F: 6}, {T: 60000, F: 7},
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
					series: []promql.Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Floats: []promql.FPoint{
								{T: 0, F: 1}, {T: 10000, F: 1}, {T: 20000, F: 1}, {T: 30000, F: 1}, {T: 40000, F: 1}, {T: 50000, F: 1},
							},
						},
						{
							Metric: labels.FromStrings("__name__", "metric2"),
							Floats: []promql.FPoint{
								{T: 0, F: 1}, {T: 10000, F: 2}, {T: 20000, F: 3}, {T: 30000, F: 4}, {T: 40000, F: 5}, {T: 50000, F: 6}, {T: 60000, F: 7}, {T: 70000, F: 8},
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
		suite, err := NewLazyLoader(c.loadString, LazyLoaderOpts{})
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
					got := promql.Series{
						Metric: storageSeries.Labels(),
					}
					it := storageSeries.Iterator(nil)
					for it.Next() == chunkenc.ValFloat {
						t, v := it.At()
						got.Floats = append(got.Floats, promql.FPoint{T: t, F: v})
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
		"instant query with expected float result": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
	{group="canary"} 70
`,
		},
		"instant query with unexpected float result": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
	{group="canary"} 80
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected 80 for {group="canary"} but got 70`,
		},
		"instant query with expected histogram result": {
			input: `
load 5m
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}}

eval instant at 0 testmetric
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}}
`,
		},
		"instant query with unexpected histogram result": {
			input: `
load 5m
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}}

eval instant at 0 testmetric
	testmetric {{schema:-1 sum:6 count:1 buckets:[1] offset:1}}
`,
			expectedError: `error in eval testmetric (line 5): expected {{schema:-1 count:1 sum:6 offset:1 buckets:[1]}} for {__name__="testmetric"} but got {{schema:-1 count:1 sum:4 offset:1 buckets:[1]}}`,
		},
		"instant query with float value returned when histogram expected": {
			input: `
load 5m
	testmetric 2

eval instant at 0 testmetric
	testmetric {{}}
`,
			expectedError: `error in eval testmetric (line 5): expected histogram {{}} for {__name__="testmetric"} but got float value 2`,
		},
		"instant query with histogram returned when float expected": {
			input: `
load 5m
	testmetric {{}}

eval instant at 0 testmetric
	testmetric 2
`,
			expectedError: `error in eval testmetric (line 5): expected float value 2.000000 for {__name__="testmetric"} but got histogram {{}}`,
		},
		"instant query, but result has an unexpected series with a float value": {
			input: testData + `
eval instant at 5m sum by (group) (http_requests)
	{group="production"} 30
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): unexpected metric {group="canary"} in result, has value 70`,
		},
		"instant query, but result has an unexpected series with a histogram value": {
			input: `
load 5m
	testmetric {{}}

eval instant at 5m testmetric
`,
			expectedError: `error in eval testmetric (line 5): unexpected metric {__name__="testmetric"} in result, has value {count:0, sum:0}`,
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
		"instant query expected to fail with specific error message, and query fails with that error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_message vector cannot contain metrics with the same labelset
`,
		},
		"instant query expected to fail with specific error message, and query fails with a different error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_message something else went wrong
`,
			expectedError: `expected error "something else went wrong" evaluating query "ceil({__name__=~'testmetric1|testmetric2'})" (line 6), but got: vector cannot contain metrics with the same labelset`,
		},

		"instant query expected to fail with error matching pattern, and query fails with that error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp vector .* contain metrics
`,
		},
		"instant query expected to fail with error matching pattern, and query fails with a different error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp something else went wrong
`,
			expectedError: `expected error matching pattern "something else went wrong" evaluating query "ceil({__name__=~'testmetric1|testmetric2'})" (line 6), but got: vector cannot contain metrics with the same labelset`,
		},
		"instant query expected to fail with error matching pattern, and pattern is not a valid regexp": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail instant at 0m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp [
`,
			expectedError: `error in eval ceil({__name__=~'testmetric1|testmetric2'}) (line 7): invalid regexp '[' for expected_fail_regexp: error parsing regexp: missing closing ]: ` + "`[`",
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
			expectedError: `error in eval sort(http_requests) (line 8): unexpected metric {__name__="http_requests", group="canary", instance="1", job="api-server"} in result, has value 400`,
		},
		"instant query with invalid timestamp": {
			input:         `eval instant at abc123 vector(0)`,
			expectedError: `error in eval vector(0) (line 1): invalid timestamp definition "abc123": not a valid duration string: "abc123"`,
		},
		"range query with expected result": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30 60
	{group="canary"} 0 70 140
`,
		},
		"range query with unexpected float value": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30 60
	{group="canary"} 0 80 140
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected float value at index 1 (t=300000) for {group="canary"} to be 80, but got 70 (result has 3 float points [0 @[0] 70 @[300000] 140 @[600000]] and 0 histogram points [])`,
		},
		"range query with expected histogram values": {
			input: `
load 5m
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}} {{schema:-1 sum:5 count:1 buckets:[1] offset:1}} {{schema:-1 sum:6 count:1 buckets:[1] offset:1}}

eval range from 0 to 10m step 5m testmetric
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}} {{schema:-1 sum:5 count:1 buckets:[1] offset:1}} {{schema:-1 sum:6 count:1 buckets:[1] offset:1}}
`,
		},
		"range query with unexpected histogram value": {
			input: `
load 5m
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}} {{schema:-1 sum:5 count:1 buckets:[1] offset:1}} {{schema:-1 sum:6 count:1 buckets:[1] offset:1}}

eval range from 0 to 10m step 5m testmetric
	testmetric {{schema:-1 sum:4 count:1 buckets:[1] offset:1}} {{schema:-1 sum:7 count:1 buckets:[1] offset:1}} {{schema:-1 sum:8 count:1 buckets:[1] offset:1}}
`,
			expectedError: `error in eval testmetric (line 5): expected histogram value at index 1 (t=300000) for {__name__="testmetric"} to be {count:1, sum:7, (1,4]:1}, but got {count:1, sum:5, (1,4]:1} (result has 0 float points [] and 3 histogram points [{count:1, sum:4, (1,4]:1} @[0] {count:1, sum:5, (1,4]:1} @[300000] {count:1, sum:6, (1,4]:1} @[600000]])`,
		},
		"range query with too many points for query time range": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30 60 90
	{group="canary"} 0 70 140
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected 4 points for {group="production"}, but query time range cannot return this many points`,
		},
		"range query with missing point in result": {
			input: `
load 5m
	testmetric 5

eval range from 0 to 6m step 6m testmetric
	testmetric 5 10
`,
			expectedError: `error in eval testmetric (line 5): expected 2 float points and 0 histogram points for {__name__="testmetric"}, but got 1 float point [5 @[0]] and 0 histogram points []`,
		},
		"range query with extra point in result": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30
	{group="canary"} 0 70 140
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected 2 float points and 0 histogram points for {group="production"}, but got 3 float points [0 @[0] 30 @[300000] 60 @[600000]] and 0 histogram points []`,
		},
		"range query, but result has an unexpected series": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30 60
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): unexpected metric {group="canary"} in result, has 3 float points [0 @[0] 70 @[300000] 140 @[600000]] and 0 histogram points []`,
		},
		"range query, but result is missing a series": {
			input: testData + `
eval range from 0 to 10m step 5m sum by (group) (http_requests)
	{group="production"} 0 30 60
	{group="canary"} 0 70 140
	{group="test"} 0 100 200
`,
			expectedError: `error in eval sum by (group) (http_requests) (line 8): expected metric {group="test"} not found`,
		},
		"range query expected to fail, and query fails": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
`,
		},
		"range query expected to fail, but query succeeds": {
			input:         `eval_fail range from 0 to 10m step 5m vector(0)`,
			expectedError: `expected error evaluating query "vector(0)" (line 1) but got none`,
		},
		"range query expected to fail with specific error message, and query fails with that error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_message vector cannot contain metrics with the same labelset
`,
		},
		"range query expected to fail with specific error message, and query fails with a different error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_message something else went wrong
`,
			expectedError: `expected error "something else went wrong" evaluating query "ceil({__name__=~'testmetric1|testmetric2'})" (line 6), but got: vector cannot contain metrics with the same labelset`,
		},
		"range query expected to fail with error matching pattern, and query fails with that error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp vector .* contain metrics
`,
		},
		"range query expected to fail with error matching pattern, and query fails with a different error": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp something else went wrong
`,
			expectedError: `expected error matching pattern "something else went wrong" evaluating query "ceil({__name__=~'testmetric1|testmetric2'})" (line 6), but got: vector cannot contain metrics with the same labelset`,
		},
		"range query expected to fail with error matching pattern, and pattern is not a valid regexp": {
			input: `
load 5m
	testmetric1{src="a",dst="b"} 0
	testmetric2{src="a",dst="b"} 1

eval_fail range from 0 to 10m step 5m ceil({__name__=~'testmetric1|testmetric2'})
	expected_fail_regexp [
`,
			expectedError: `error in eval ceil({__name__=~'testmetric1|testmetric2'}) (line 7): invalid regexp '[' for expected_fail_regexp: error parsing regexp: missing closing ]: ` + "`[`",
		},
		"range query with from and to timestamps in wrong order": {
			input:         `eval range from 10m to 9m step 5m vector(0)`,
			expectedError: `error in eval vector(0) (line 1): invalid test definition, end timestamp (9m) is before start timestamp (10m)`,
		},
		"range query with sparse output": {
			input: `
load 6m
	testmetric 1 _ 3

eval range from 0 to 18m step 6m testmetric
	testmetric 1 _ 3
`,
		},
		"range query with float value returned when no value expected": {
			input: `
load 6m
	testmetric 1 2 3

eval range from 0 to 18m step 6m testmetric
	testmetric 1 _ 3
`,
			expectedError: `error in eval testmetric (line 5): expected 2 float points and 0 histogram points for {__name__="testmetric"}, but got 3 float points [1 @[0] 2 @[360000] 3 @[720000]] and 0 histogram points []`,
		},
		"range query with float value returned when histogram expected": {
			input: `
load 5m
	testmetric 2 3

eval range from 0 to 5m step 5m testmetric
	testmetric {{}} {{}}
`,
			expectedError: `error in eval testmetric (line 5): expected 0 float points and 2 histogram points for {__name__="testmetric"}, but got 2 float points [2 @[0] 3 @[300000]] and 0 histogram points []`,
		},
		"range query with histogram returned when float expected": {
			input: `
load 5m
	testmetric {{}} {{}}

eval range from 0 to 5m step 5m testmetric
	testmetric 2 3
`,
			expectedError: `error in eval testmetric (line 5): expected 2 float points and 0 histogram points for {__name__="testmetric"}, but got 0 float points [] and 2 histogram points [{count:0, sum:0} @[0] {count:0, sum:0} @[300000]]`,
		},
		"range query with expected mixed results": {
			input: `
load 6m
	testmetric{group="a"} {{}} _ _
	testmetric{group="b"} _ _ 3

eval range from 0 to 12m step 6m sum(testmetric)
	{} {{}} _ 3
`,
		},
		"range query with mixed results and incorrect values": {
			input: `
load 5m
	testmetric 3 {{}}

eval range from 0 to 5m step 5m testmetric
	testmetric {{}} 3
`,
			expectedError: `error in eval testmetric (line 5): expected float value at index 0 for {__name__="testmetric"} to have timestamp 300000, but it had timestamp 0 (result has 1 float point [3 @[0]] and 1 histogram point [{count:0, sum:0} @[300000]])`,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			err := runTest(t, testCase.input, NewTestEngine(false, 0, DefaultMaxSamplesPerQuery))

			if testCase.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, testCase.expectedError)
			}
		})
	}
}

func TestAssertMatrixSorted(t *testing.T) {
	testCases := map[string]struct {
		matrix        promql.Matrix
		expectedError string
	}{
		"empty matrix": {
			matrix: promql.Matrix{},
		},
		"matrix with one series": {
			matrix: promql.Matrix{
				promql.Series{Metric: labels.FromStrings("the_label", "value_1")},
			},
		},
		"matrix with two series, series in sorted order": {
			matrix: promql.Matrix{
				promql.Series{Metric: labels.FromStrings("the_label", "value_1")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_2")},
			},
		},
		"matrix with two series, series in reverse order": {
			matrix: promql.Matrix{
				promql.Series{Metric: labels.FromStrings("the_label", "value_2")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_1")},
			},
			expectedError: `matrix results should always be sorted by labels, but matrix is not sorted: series at index 1 with labels {the_label="value_1"} sorts before series at index 0 with labels {the_label="value_2"}`,
		},
		"matrix with three series, series in sorted order": {
			matrix: promql.Matrix{
				promql.Series{Metric: labels.FromStrings("the_label", "value_1")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_2")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_3")},
			},
		},
		"matrix with three series, series not in sorted order": {
			matrix: promql.Matrix{
				promql.Series{Metric: labels.FromStrings("the_label", "value_1")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_3")},
				promql.Series{Metric: labels.FromStrings("the_label", "value_2")},
			},
			expectedError: `matrix results should always be sorted by labels, but matrix is not sorted: series at index 2 with labels {the_label="value_2"} sorts before series at index 1 with labels {the_label="value_3"}`,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			err := assertMatrixSorted(testCase.matrix)

			if testCase.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, testCase.expectedError)
			}
		})
	}
}
