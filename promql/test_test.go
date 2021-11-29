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
							Points: []Point{
								{0, 1, nil}, {10000, 2, nil}, {20000, 3, nil}, {30000, 4, nil}, {40000, 5, nil},
							},
						},
					},
				},
				{
					ts: time.Unix(10, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Points: []Point{
								{0, 1, nil}, {10000, 2, nil}, {20000, 3, nil}, {30000, 4, nil}, {40000, 5, nil},
							},
						},
					},
				},
				{
					ts: time.Unix(60, 0),
					series: []Series{
						{
							Metric: labels.FromStrings("__name__", "metric1"),
							Points: []Point{
								{0, 1, nil}, {10000, 2, nil}, {20000, 3, nil}, {30000, 4, nil}, {40000, 5, nil}, {50000, 6, nil}, {60000, 7, nil},
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
							Points: []Point{
								{0, 1, nil}, {10000, 1, nil}, {20000, 1, nil}, {30000, 1, nil}, {40000, 1, nil}, {50000, 1, nil},
							},
						},
						{
							Metric: labels.FromStrings("__name__", "metric2"),
							Points: []Point{
								{0, 1, nil}, {10000, 2, nil}, {20000, 3, nil}, {30000, 4, nil}, {40000, 5, nil}, {50000, 6, nil}, {60000, 7, nil}, {70000, 8, nil},
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
				querier, err := queryable.Querier(suite.Context(), math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				for _, s := range tc.series {
					var matchers []*labels.Matcher
					for _, label := range s.Metric {
						m, err := labels.NewMatcher(labels.MatchEqual, label.Name, label.Value)
						require.NoError(t, err)
						matchers = append(matchers, m)
					}

					// Get the series for the matcher.
					ss := querier.Select(false, nil, matchers...)
					require.True(t, ss.Next())
					storageSeries := ss.At()
					require.False(t, ss.Next(), "Expecting only 1 series")

					// Convert `storage.Series` to `promql.Series`.
					got := Series{
						Metric: storageSeries.Labels(),
					}
					it := storageSeries.Iterator()
					for it.Next() == chunkenc.ValFloat {
						t, v := it.At()
						got.Points = append(got.Points, Point{T: t, V: v})
					}
					require.NoError(t, it.Err())

					require.Equal(t, s, got)
				}
			})
		}
	}
}
