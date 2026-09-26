// Copyright The Prometheus Authors
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

package histogramconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestExtractHistogramSuffix(t *testing.T) {
	tests := []struct {
		name           string
		matchers       []*labels.Matcher
		expectedName   string
		expectedSuffix string
	}{
		{
			name:           "bucket suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			expectedName:   "http_requests_bucket",
			expectedSuffix: "_bucket",
		},
		{
			name:           "count suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")},
			expectedName:   "http_requests_count",
			expectedSuffix: "_count",
		},
		{
			name:           "sum suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_sum")},
			expectedName:   "http_requests_sum",
			expectedSuffix: "_sum",
		},
		{
			name:           "no suffix - regular metric",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "my_gauge")},
			expectedName:   "",
			expectedSuffix: "",
		},
		{
			name:           "no metric name matcher",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus")},
			expectedName:   "",
			expectedSuffix: "",
		},
		{
			name:           "bucket regex suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, ".+_bucket")},
			expectedName:   ".+_bucket",
			expectedSuffix: "_bucket",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher, suffix, _ := extractHistogramSuffix(tc.matchers)
			if tc.expectedName == "" {
				require.Nil(t, matcher)
			} else {
				require.NotNil(t, matcher)
				require.Equal(t, tc.expectedName, matcher.Value)
			}
			require.Equal(t, tc.expectedSuffix, suffix)
		})
	}
}

func TestNHCBAsClassicQuerier_Select(t *testing.T) {
	nhcb := &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           16,
		Sum:             100.0,
		CustomValues:    []float64{1.0, 5.0, 10.0},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 4}},
		PositiveBuckets: []int64{2, 1, 2, 1},
	}

	tests := []struct {
		name              string
		queryMatchers     []*labels.Matcher
		classicSeries     []storage.Series
		nhcbSeries        []storage.Series
		passthroughSeries []storage.Series
		expectedCount     int
		expectedSuffix    string
	}{
		{
			name:          "non-histogram query passes through",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "my_gauge")},
			passthroughSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "my_gauge"), []chunks.Sample{fSample{t: 1, f: 42}}),
			},
			expectedCount: 1,
		},
		{
			name:          "classic histogram exists - return classic",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			expectedCount: 1,
		},
		{
			name:          "histogram with regex exists - return classic",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_requests_bucket")},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount: 4,
		},
		{
			name:          "no classic - convert NHCB to bucket series",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  4,
			expectedSuffix: "_bucket",
		},
		{
			name:          "no classic - convert NHCB to count series",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  1,
			expectedSuffix: "_count",
		},
		{
			name:          "no classic - convert NHCB to sum series",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_sum")},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  1,
			expectedSuffix: "_sum",
		},
		{
			name:          "both classic and NHCB - return both",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  5,
			expectedSuffix: "_bucket",
		},
		{
			name:          "no classic and no NHCB - return empty",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []storage.Series{},
			nhcbSeries:    []storage.Series{},
			expectedCount: 0,
		},
		{
			name: "le exact match filters to single bucket",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "5.0"),
			},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  1,
			expectedSuffix: "_bucket",
		},
		{
			name: "le exact match +Inf",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "+Inf"),
			},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  1,
			expectedSuffix: "_bucket",
		},
		{
			name: "le exact match no match",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "99.0"),
			},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  0,
			expectedSuffix: "_bucket",
		},
		{
			name: "le regex match filters to matching buckets",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchRegexp, labels.BucketLabel, "1.0|10.0"),
			},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  2,
			expectedSuffix: "_bucket",
		},
		{
			name: "le not equal excludes one bucket",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchNotEqual, labels.BucketLabel, "+Inf"),
			},
			classicSeries: []storage.Series{},
			nhcbSeries: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  3,
			expectedSuffix: "_bucket",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &nhcbMockQuerier{
				classicSeries:     tc.classicSeries,
				nhcbSeries:        tc.nhcbSeries,
				passthroughSeries: tc.passthroughSeries,
			}
			q := NewNHCBAsClassicQuerier(mock)

			ss := q.Select(context.Background(), false, nil, tc.queryMatchers...)
			var count int
			for ss.Next() {
				count++
				s := ss.At()
				if tc.expectedSuffix != "" {
					require.Contains(t, s.Labels().Get(model.MetricNameLabel), tc.expectedSuffix)
				}
			}
			require.NoError(t, ss.Err())
			require.Equal(t, tc.expectedCount, count)
		})
	}
}

func TestNHCBAsClassicQuerier_ConsistentOrder(t *testing.T) {
	nhcb := &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           16,
		Sum:             100.0,
		CustomValues:    []float64{1.0, 5.0, 10.0},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 4}},
		PositiveBuckets: []int64{2, 1, 2, 1},
	}

	mock := &nhcbMockQuerier{
		classicSeries: []storage.Series{},
		nhcbSeries: []storage.Series{
			storage.NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			storage.NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "web"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
		},
	}
	q := NewNHCBAsClassicQuerier(mock)

	// Run the same query multiple times and verify order is consistent.
	for range 5 {
		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
		var seriesLabels []string
		for ss.Next() {
			seriesLabels = append(seriesLabels, ss.At().Labels().String())
		}
		require.NoError(t, ss.Err())

		// 2 NHCB series × 4 buckets each (le=1.0, 5.0, 10.0, +Inf) = 8 series.
		require.Len(t, seriesLabels, 8)

		// Expect buckets for "api" job first (in le order), then "web" job (in le order).
		expectedOrder := []string{
			`{__name__="http_requests_bucket", job="api", le="1.0"}`,
			`{__name__="http_requests_bucket", job="api", le="5.0"}`,
			`{__name__="http_requests_bucket", job="api", le="10.0"}`,
			`{__name__="http_requests_bucket", job="api", le="+Inf"}`,
			`{__name__="http_requests_bucket", job="web", le="1.0"}`,
			`{__name__="http_requests_bucket", job="web", le="5.0"}`,
			`{__name__="http_requests_bucket", job="web", le="10.0"}`,
			`{__name__="http_requests_bucket", job="web", le="+Inf"}`,
		}
		require.Equal(t, expectedOrder, seriesLabels)
	}
}

func TestNHCBAsClassicQuerier_FloatHistogram(t *testing.T) {
	fhNHCB := &histogram.FloatHistogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           15,
		Sum:             150.0,
		CustomValues:    []float64{1.0, 5.0},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
		PositiveBuckets: []float64{3, 5, 7},
	}

	mock := &nhcbMockQuerier{
		classicSeries: []storage.Series{},
		nhcbSeries: []storage.Series{
			storage.NewListSeries(labels.FromStrings("__name__", "latency"), []chunks.Sample{fhSample{t: 1, fh: fhNHCB}}),
		},
	}
	q := NewNHCBAsClassicQuerier(mock)

	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "latency_bucket"))
	var count int
	for ss.Next() {
		count++
	}
	require.NoError(t, ss.Err())
	require.Equal(t, 3, count)
}

func TestNHCBAsClassicQuerier_Staleness(t *testing.T) {
	nhcb := func(customValues ...float64) *histogram.Histogram {
		return &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			Count:           4,
			Sum:             6,
			CustomValues:    customValues,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
			PositiveBuckets: []int64{1, 1, -1},
		}
	}
	exponential := &histogram.Histogram{
		Count:           4,
		Sum:             6,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
		PositiveBuckets: []int64{1, 1, -1},
	}
	// This is how the TSDB returns a stale marker of a histogram series.
	staleMarker := &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}
	name := func(n string) *labels.Matcher {
		return labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, n)
	}

	for _, tc := range []struct {
		name     string
		series   []storage.Series
		matchers []*labels.Matcher
		expected []string
	}{
		{
			name: "stale marker",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: staleMarker}, hSample{t: 3, h: nhcb(1, 2)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", le="1.0"} 1@1 stale@2 1@3`,
				`{__name__="foo_bucket", le="2.0"} 3@1 stale@2 3@3`,
				`{__name__="foo_bucket", le="+Inf"} 4@1 stale@2 4@3`,
			},
		},
		{
			name: "float stale marker",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, fSample{t: 2, f: math.Float64frombits(value.StaleNaN)},
			})},
			matchers: []*labels.Matcher{name("foo_count")},
			expected: []string{`{__name__="foo_count"} 4@1 stale@2`},
		},
		{
			name: "consecutive stale markers result in a single one",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: staleMarker}, hSample{t: 3, h: staleMarker}, hSample{t: 4, h: nhcb(1, 2)},
			})},
			matchers: []*labels.Matcher{name("foo_sum")},
			expected: []string{`{__name__="foo_sum"} 6@1 stale@2 6@4`},
		},
		{
			name: "bucket layout change",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: nhcb(1, 4)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", le="1.0"} 1@1 1@2`,
				`{__name__="foo_bucket", le="2.0"} 3@1 stale@2`,
				`{__name__="foo_bucket", le="+Inf"} 4@1 4@2`,
				`{__name__="foo_bucket", le="4.0"} 3@2`,
			},
		},
		{
			name: "sample that is not converted",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: exponential}, hSample{t: 3, h: nhcb(1, 2)},
			})},
			matchers: []*labels.Matcher{name("foo_count")},
			expected: []string{`{__name__="foo_count"} 4@1 stale@2 4@3`},
		},
		{
			name: "le matcher",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: staleMarker},
			})},
			matchers: []*labels.Matcher{name("foo_bucket"), labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "+Inf")},
			expected: []string{`{__name__="foo_bucket", le="+Inf"} 4@1 stale@2`},
		},
		{
			name: "native histogram series go stale independently",
			series: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "a"), []chunks.Sample{
					hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: staleMarker},
				}),
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "b"), []chunks.Sample{
					hSample{t: 1, h: nhcb(1, 2)}, hSample{t: 2, h: nhcb(1, 2)},
				}),
			},
			matchers: []*labels.Matcher{name("foo_count")},
			expected: []string{
				`{__name__="foo_count", job="a"} 4@1 stale@2`,
				`{__name__="foo_count", job="b"} 4@1 4@2`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{nhcbSeries: tc.series})
			ss := q.Select(context.Background(), false, nil, tc.matchers...)
			require.ElementsMatch(t, tc.expected, samplesSummary(t, ss))
		})
	}
}

func TestNHCBAsClassicQuerier_Exponential(t *testing.T) {
	// exponential returns an exponential histogram with the given schema and
	// positive bucket counts, starting at the bucket with index offset.
	exponential := func(schema, offset int32, buckets ...float64) *histogram.FloatHistogram {
		fh := &histogram.FloatHistogram{
			Schema:          schema,
			Sum:             6,
			PositiveSpans:   []histogram.Span{{Offset: offset, Length: uint32(len(buckets))}},
			PositiveBuckets: buckets,
		}
		for _, b := range buckets {
			fh.Count += b
		}
		return fh
	}
	nhcb := &histogram.FloatHistogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           4,
		Sum:             6,
		CustomValues:    []float64{1, 2},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
		PositiveBuckets: []float64{1, 2, 1},
	}
	// This is how the TSDB returns a stale marker of a histogram series,
	// note the exponential schema 0.
	staleMarker := &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}
	name := func(n string) *labels.Matcher {
		return labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, n)
	}

	for _, tc := range []struct {
		name               string
		includeExponential bool
		series             []storage.Series
		matchers           []*labels.Matcher
		expected           []string
	}{
		{
			// The buckets are (0.5,1], (1,2] and (2,4].
			name:               "exponential histogram",
			includeExponential: true,
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", le="0.5"} 0@1`,
				`{__name__="foo_bucket", le="1.0"} 1@1`,
				`{__name__="foo_bucket", le="2.0"} 3@1`,
				`{__name__="foo_bucket", le="4.0"} 4@1`,
				`{__name__="foo_bucket", le="+Inf"} 4@1`,
			},
		},
		{
			name:               "exponential histogram count",
			includeExponential: true,
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)},
			})},
			matchers: []*labels.Matcher{name("foo_count")},
			expected: []string{`{__name__="foo_count"} 4@1`},
		},
		{
			name:               "exponential histogram sum",
			includeExponential: true,
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)},
			})},
			matchers: []*labels.Matcher{name("foo_sum")},
			expected: []string{`{__name__="foo_sum"} 6@1`},
		},
		{
			name: "exponential histograms are not converted by default",
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket")},
		},
		{
			// The buckets of job a are (0.5,1] and (1,2]. The ones of job b
			// are (√2,2] and (2,2√2], which are (1,2] and (2,4] in schema 0.
			name:               "all series are converted with the same boundaries",
			includeExponential: true,
			series: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "a"), []chunks.Sample{
					fhSample{t: 1, fh: exponential(0, 0, 1, 2)},
				}),
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "b"), []chunks.Sample{
					fhSample{t: 2, fh: exponential(1, 2, 1, 2)},
				}),
			},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", job="a", le="0.5"} 0@1`,
				`{__name__="foo_bucket", job="a", le="1.0"} 1@1`,
				`{__name__="foo_bucket", job="a", le="2.0"} 3@1`,
				`{__name__="foo_bucket", job="a", le="4.0"} 3@1`,
				`{__name__="foo_bucket", job="a", le="+Inf"} 3@1`,
				`{__name__="foo_bucket", job="b", le="0.5"} 0@2`,
				`{__name__="foo_bucket", job="b", le="1.0"} 0@2`,
				`{__name__="foo_bucket", job="b", le="2.0"} 1@2`,
				`{__name__="foo_bucket", job="b", le="4.0"} 3@2`,
				`{__name__="foo_bucket", job="b", le="+Inf"} 3@2`,
			},
		},
		{
			// The buckets are (1,√2] and (√2,2] first, then (1,2]. The
			// boundary √2 of the first sample is not used, which would mark
			// its series stale at the second sample.
			name:               "schema change",
			includeExponential: true,
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(1, 1, 1, 2)}, fhSample{t: 2, fh: exponential(0, 1, 3)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", le="1.0"} 0@1 0@2`,
				`{__name__="foo_bucket", le="2.0"} 3@1 3@2`,
				`{__name__="foo_bucket", le="+Inf"} 3@1 3@2`,
			},
		},
		{
			name:               "NHCB keep their own boundaries",
			includeExponential: true,
			series: []storage.Series{
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "nhcb"), []chunks.Sample{
					fhSample{t: 1, fh: nhcb},
				}),
				storage.NewListSeries(labels.FromStrings("__name__", "foo", "job", "exponential"), []chunks.Sample{
					fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)},
				}),
			},
			matchers: []*labels.Matcher{name("foo_bucket")},
			expected: []string{
				`{__name__="foo_bucket", job="nhcb", le="1.0"} 1@1`,
				`{__name__="foo_bucket", job="nhcb", le="2.0"} 3@1`,
				`{__name__="foo_bucket", job="nhcb", le="+Inf"} 4@1`,
				`{__name__="foo_bucket", job="exponential", le="0.5"} 0@1`,
				`{__name__="foo_bucket", job="exponential", le="1.0"} 1@1`,
				`{__name__="foo_bucket", job="exponential", le="2.0"} 3@1`,
				`{__name__="foo_bucket", job="exponential", le="4.0"} 4@1`,
				`{__name__="foo_bucket", job="exponential", le="+Inf"} 4@1`,
			},
		},
		{
			name:               "stale marker",
			includeExponential: true,
			series: []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "foo"), []chunks.Sample{
				fhSample{t: 1, fh: exponential(0, 0, 1, 2, 1)}, fhSample{t: 2, fh: staleMarker}, fhSample{t: 3, fh: exponential(0, 0, 1, 2, 1)},
			})},
			matchers: []*labels.Matcher{name("foo_bucket"), labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "2.0")},
			expected: []string{`{__name__="foo_bucket", le="2.0"} 3@1 stale@2 3@3`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &NHCBAsClassicQuerier{Querier: &nhcbMockQuerier{nhcbSeries: tc.series}, includeExponential: tc.includeExponential}
			ss := q.Select(context.Background(), false, nil, tc.matchers...)
			require.ElementsMatch(t, tc.expected, samplesSummary(t, ss))
		})
	}
}

// samplesSummary returns a string per series of ss with its labels and
// samples, e.g. `{__name__="foo_count"} 4@1 stale@2`.
func samplesSummary(t *testing.T, ss storage.SeriesSet) []string {
	t.Helper()

	var (
		summary []string
		it      chunkenc.Iterator
	)
	for ss.Next() {
		s := ss.At()
		var sb strings.Builder
		sb.WriteString(s.Labels().String())
		it = s.Iterator(it)
		for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
			var v string
			switch vt {
			case chunkenc.ValFloat:
				_, f := it.At()
				v = strconv.FormatFloat(f, 'g', -1, 64)
				if value.IsStaleNaN(f) {
					v = "stale"
				}
			case chunkenc.ValHistogram, chunkenc.ValFloatHistogram:
				_, fh := it.AtFloatHistogram(nil)
				v = fh.String()
				if value.IsStaleNaN(fh.Sum) {
					v = "stale"
				}
			}
			fmt.Fprintf(&sb, " %s@%d", v, it.AtT())
		}
		require.NoError(t, it.Err())
		summary = append(summary, sb.String())
	}
	require.NoError(t, ss.Err())
	return summary
}

// mockSeriesSet returns the given series and warnings.
type mockSeriesSet struct {
	idx      int
	series   []storage.Series
	warnings annotations.Annotations
}

func newMockSeriesSet(series ...storage.Series) storage.SeriesSet {
	return &mockSeriesSet{idx: -1, series: series}
}

func (m *mockSeriesSet) Next() bool {
	m.idx++
	return m.idx < len(m.series)
}

func (m *mockSeriesSet) At() storage.Series { return m.series[m.idx] }

func (*mockSeriesSet) Err() error { return nil }

func (m *mockSeriesSet) Warnings() annotations.Annotations { return m.warnings }

type nhcbMockQuerier struct {
	classicSeries     []storage.Series
	nhcbSeries        []storage.Series
	passthroughSeries []storage.Series // For non-histogram queries

	// For error/warning injection in tests.
	classicErr   error
	nhcbErr      error
	nhcbWarnings annotations.Annotations
}

func (m *nhcbMockQuerier) Select(_ context.Context, _ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name == model.MetricNameLabel {
			// Check if this is a histogram suffix query (classic histogram query)
			if strings.HasSuffix(matcher.Value, "_bucket") ||
				strings.HasSuffix(matcher.Value, "_count") ||
				strings.HasSuffix(matcher.Value, "_sum") {
				if m.classicErr != nil {
					return storage.ErrSeriesSet(m.classicErr)
				}
				return newMockSeriesSet(m.classicSeries...)
			}
			// If passthroughSeries is set, use it for non-histogram metric queries
			if len(m.passthroughSeries) > 0 {
				return newMockSeriesSet(m.passthroughSeries...)
			}
			// Base metric name query - return NHCB series
			if m.nhcbErr != nil {
				return storage.ErrSeriesSet(m.nhcbErr)
			}
			return &mockSeriesSet{idx: -1, series: m.nhcbSeries, warnings: m.nhcbWarnings}
		}
	}
	return newMockSeriesSet()
}

func (*nhcbMockQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbMockQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbMockQuerier) Close() error {
	return nil
}

// deferredErrSeriesSet returns series normally but reports a non-nil error only
// after Next() has been exhausted. This lets the pre-iteration Err() check pass
// while still exercising error paths that are evaluated after draining the set.
type deferredErrSeriesSet struct {
	series    []storage.Series
	idx       int
	err       error
	exhausted bool
}

func newDeferredErrSeriesSet(err error, series ...storage.Series) storage.SeriesSet {
	return &deferredErrSeriesSet{idx: -1, series: series, err: err}
}

func (s *deferredErrSeriesSet) Next() bool {
	s.idx++
	if s.idx >= len(s.series) {
		s.exhausted = true
		return false
	}
	return true
}

func (s *deferredErrSeriesSet) At() storage.Series { return s.series[s.idx] }

func (s *deferredErrSeriesSet) Err() error {
	if s.exhausted {
		return s.err
	}
	return nil
}

func (*deferredErrSeriesSet) Warnings() annotations.Annotations { return nil }

// nhcbSetQuerier routes suffix queries (_bucket/_count/_sum) to classicSet and
// all other queries to nhcbSet. This lets tests inject arbitrary storage.SeriesSet
// implementations for either path without duplicating routing logic.
type nhcbSetQuerier struct {
	classicSet storage.SeriesSet
	nhcbSet    storage.SeriesSet
}

func (m *nhcbSetQuerier) Select(_ context.Context, _ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name == model.MetricNameLabel {
			if strings.HasSuffix(matcher.Value, "_bucket") ||
				strings.HasSuffix(matcher.Value, "_count") ||
				strings.HasSuffix(matcher.Value, "_sum") {
				return m.classicSet
			}
			return m.nhcbSet
		}
	}
	return newMockSeriesSet()
}

func (*nhcbSetQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbSetQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbSetQuerier) Close() error { return nil }

func TestNHCBAsClassicQuerier_ErrorPropagation(t *testing.T) {
	nhcb := &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           3,
		Sum:             10.0,
		CustomValues:    []float64{1.0},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
		PositiveBuckets: []int64{1, 1},
	}
	testError := errors.New("storage error")

	tests := []struct {
		name    string
		querier storage.Querier
	}{
		{
			name: "classic set immediate error",
			querier: &nhcbMockQuerier{
				classicErr: testError,
				nhcbSeries: []storage.Series{
					storage.NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
				},
			},
		},
		{
			name: "nhcb set immediate error",
			querier: &nhcbMockQuerier{
				classicSeries: []storage.Series{},
				nhcbErr:       testError,
			},
		},
		{
			// The nhcb set's Err() is nil initially (passes the pre-check) but
			// becomes non-nil once Next() is exhausted, exercising the error
			// path inside nhcbToClassicSeriesSet.Next().
			name: "nhcb set error during iteration inside nhcbToClassicSeriesSet",
			querier: &nhcbSetQuerier{
				classicSet: newMockSeriesSet(),
				nhcbSet:    newDeferredErrSeriesSet(testError),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := NewNHCBAsClassicQuerier(tc.querier)
			ss := q.Select(context.Background(), false, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
			for ss.Next() {
			}
			require.ErrorIs(t, ss.Err(), testError)
		})
	}
}

func TestNHCBAsClassicQuerier_WarningPropagation(t *testing.T) {
	nhcb := &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           3,
		Sum:             10.0,
		CustomValues:    []float64{1.0},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
		PositiveBuckets: []int64{1, 1},
	}
	nhcbSeries := storage.NewListSeries(
		labels.FromStrings("__name__", "http_requests"),
		[]chunks.Sample{hSample{t: 1, h: nhcb}},
	)

	t.Run("nhcb set warnings propagate", func(t *testing.T) {
		warn := annotations.New().Add(errors.New("nhcb warning"))
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []storage.Series{},
			nhcbSeries:    []storage.Series{nhcbSeries},
			nhcbWarnings:  warn,
		})

		ss := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
		for ss.Next() {
		}
		require.NoError(t, ss.Err())
		require.Equal(t, warn, ss.Warnings())
	})

	t.Run("classic set warnings propagate", func(t *testing.T) {
		warn := annotations.New().Add(errors.New("classic warning"))
		classicSeries := []storage.Series{storage.NewListSeries(
			labels.FromStrings("__name__", "http_requests_bucket", "le", "1"),
			[]chunks.Sample{fSample{t: 1, f: 5}},
		)}
		q := NewNHCBAsClassicQuerier(&nhcbSetQuerier{
			classicSet: &mockSeriesSet{idx: -1, series: classicSeries, warnings: warn},
			nhcbSet:    newMockSeriesSet(),
		})

		ss := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
		for ss.Next() {
		}
		require.NoError(t, ss.Err())
		require.Equal(t, warn, ss.Warnings())
	})

	t.Run("non-histogram passthrough preserves warnings", func(t *testing.T) {
		warn := annotations.New().Add(errors.New("passthrough warning"))
		series := []storage.Series{storage.NewListSeries(labels.FromStrings("__name__", "my_gauge"), []chunks.Sample{fSample{t: 1, f: 1}})}
		q := NewNHCBAsClassicQuerier(&nhcbSetQuerier{
			nhcbSet: &mockSeriesSet{idx: -1, series: series, warnings: warn},
		})

		ss := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "my_gauge"))
		for ss.Next() {
		}
		require.NoError(t, ss.Err())
		require.Equal(t, warn, ss.Warnings())
	})
}
