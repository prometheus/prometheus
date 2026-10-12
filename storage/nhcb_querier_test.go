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

package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
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
		{
			name:     "bucket regex matcher is not rewritten",
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_bucket")},
		},
		{
			name:     "bucket not-equal matcher is not rewritten",
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket")},
		},
		{
			name: "contradictory metric name matchers are not rewritten",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket"),
			},
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

	type testCase struct {
		name              string
		queryMatchers     []*labels.Matcher
		classicSeries     []Series
		nhcbSeries        []Series
		passthroughSeries []Series
		expectedCount     int
		expectedSuffix    string
		expectedSamples   []seriesSamples
	}

	groups := []struct {
		name  string
		cases []testCase
	}{
		{
			name: "conversion",
			cases: []testCase{
				{
					name:          "non-histogram query passes through",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "my_gauge")},
					passthroughSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "my_gauge"), []chunks.Sample{fSample{t: 1, f: 42}}),
					},
					expectedCount: 1,
				},
				{
					name:          "classic histogram exists - return classic",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
					},
					expectedCount: 1,
				},
				{
					name:          "regex name matcher passes through without conversion",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_requests_bucket")},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount: 0,
				},
				{
					name:          "not-equal name matcher passes through without conversion",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket")},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "rpc_latency"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount: 0,
				},
				{
					name: "contradictory name matchers pass through without conversion",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
						labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket"),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "rpc_latency"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount: 0,
				},
				{
					name:          "no classic - convert NHCB to bucket series",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  4,
					expectedSuffix: "_bucket",
				},
				{
					name:          "no classic - convert NHCB to count series",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  1,
					expectedSuffix: "_count",
				},
				{
					name:          "no classic - convert NHCB to sum series",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_sum")},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  1,
					expectedSuffix: "_sum",
				},
				{
					name:          "both classic and NHCB - return both",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  5,
					expectedSuffix: "_bucket",
				},
				{
					name:          "no classic and no NHCB - return empty",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
					classicSeries: []Series{},
					nhcbSeries:    []Series{},
					expectedCount: 0,
				},
				{
					name: "le exact match filters to single bucket",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
						labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "5.0"),
					},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  3,
					expectedSuffix: "_bucket",
				},
				{
					name: "multiple le matchers all apply",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
						labels.MustNewMatcher(labels.MatchNotEqual, labels.BucketLabel, "+Inf"),
						labels.MustNewMatcher(labels.MatchNotEqual, labels.BucketLabel, "1.0"),
					},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  2,
					expectedSuffix: "_bucket",
				},
				{
					name: "le matcher on count query excludes series without le label",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count"),
						labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1.0"),
					},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  0,
					expectedSuffix: "_count",
				},
				{
					name: "le matcher on sum query excludes series without le label",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_sum"),
						labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1.0"),
					},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  0,
					expectedSuffix: "_sum",
				},
				{
					name: "le matcher on count query returns stored classic count series with le label and excludes NHCB",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count"),
						labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1.0"),
					},
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests_count", "le", "1.0"), []chunks.Sample{fSample{t: 1, f: 5}}),
						NewListSeries(labels.FromStrings("__name__", "http_requests_count"), []chunks.Sample{fSample{t: 1, f: 10}}),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount:  1,
					expectedSuffix: "_count",
				},
				{
					name:          "non-histogram _bucket series without le label is returned alongside converted NHCB",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "notahist_bucket")},
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "notahist_bucket", "job", "api"), []chunks.Sample{fSample{t: 1, f: 42}}),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "notahist", "job", "api"), []chunks.Sample{hSample{t: 1, h: makeNHCB([]float64{1.0}, []int64{5, 5})}}),
					},
					expectedCount: 3,
					expectedSamples: []seriesSamples{
						{labels: `{__name__="notahist_bucket", job="api"}`, samples: []fSample{{t: 1, f: 42}}},
						{labels: `{__name__="notahist_bucket", job="api", le="1.0"}`, samples: []fSample{{t: 1, f: 5}}},
						{labels: `{__name__="notahist_bucket", job="api", le="+Inf"}`, samples: []fSample{{t: 1, f: 10}}},
					},
				},
				{
					name:          "NHCB series that already has le label is ignored",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests", "le", "custom"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
					},
					expectedCount: 0,
				},
				{
					name: "multiple samples per series with mid-series bucket layout change and builder reuse across series",
					queryMatchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
					},
					classicSeries: []Series{},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
							hSample{t: 1, h: makeNHCB([]float64{1.0, 2.0, 3.0}, []int64{1, 2, 3, 4})},
							hSample{t: 2, h: makeNHCB([]float64{1.0, 2.0, 3.0}, []int64{2, 4, 6, 8})},
							hSample{t: 3, h: makeNHCB([]float64{1.0, 5.0}, []int64{3, 7, 5})},
							hSample{t: 4, h: makeNHCB([]float64{1.0, 2.0, 3.0}, []int64{4, 5, 6, 7})},
						}),
						NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "web"), []chunks.Sample{
							hSample{t: 1, h: makeNHCB([]float64{1.0, 5.0}, []int64{5, 10, 15})},
							hSample{t: 2, h: makeNHCB([]float64{1.0, 5.0}, []int64{6, 12, 18})},
						}),
					},
					expectedCount:  8,
					expectedSuffix: "_bucket",
					expectedSamples: []seriesSamples{
						{labels: `{__name__="http_requests_bucket", job="api", le="1.0"}`, samples: []fSample{{t: 1, f: 1}, {t: 2, f: 2}, {t: 3, f: 3}, {t: 4, f: 4}}},
						{labels: `{__name__="http_requests_bucket", job="api", le="2.0"}`, samples: []fSample{{t: 1, f: 3}, {t: 2, f: 6}, {t: 4, f: 9}}},
						{labels: `{__name__="http_requests_bucket", job="api", le="3.0"}`, samples: []fSample{{t: 1, f: 6}, {t: 2, f: 12}, {t: 4, f: 15}}},
						{labels: `{__name__="http_requests_bucket", job="api", le="+Inf"}`, samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 15}, {t: 4, f: 22}}},
						{labels: `{__name__="http_requests_bucket", job="api", le="5.0"}`, samples: []fSample{{t: 3, f: 10}}},
						{labels: `{__name__="http_requests_bucket", job="web", le="1.0"}`, samples: []fSample{{t: 1, f: 5}, {t: 2, f: 6}}},
						{labels: `{__name__="http_requests_bucket", job="web", le="5.0"}`, samples: []fSample{{t: 1, f: 15}, {t: 2, f: 18}}},
						{labels: `{__name__="http_requests_bucket", job="web", le="+Inf"}`, samples: []fSample{{t: 1, f: 30}, {t: 2, f: 36}}},
					},
				},
			},
		},
		{
			// TODO: Stored classic and NHCB series with identical labels are not
			// merged yet, each case documents the current output.
			name: "collisions not yet handled",
			cases: []testCase{
				{
					// Stored classic should win, returning only the classic series.
					name:          "identical labelset and timestamp returns duplicate series",
					queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")},
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{fSample{t: 1, f: 10}}),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{hSample{t: 1, h: makeNHCB([]float64{1.0}, []int64{50, 49})}}),
					},
					expectedCount: 2,
					expectedSamples: []seriesSamples{
						{labels: `{__name__="http_requests_count", job="api"}`, samples: []fSample{{t: 1, f: 10}}},
						{labels: `{__name__="http_requests_count", job="api"}`, samples: []fSample{{t: 1, f: 99}}},
					},
				},
			},
		},
	}

	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			for _, tc := range g.cases {
				t.Run(tc.name, func(t *testing.T) {
					mock := &nhcbMockQuerier{
						classicSeries:     tc.classicSeries,
						nhcbSeries:        tc.nhcbSeries,
						passthroughSeries: tc.passthroughSeries,
					}
					q := NewNHCBAsClassicQuerier(mock)

					ss := q.Select(context.Background(), false, nil, tc.queryMatchers...)
					var collected []Series
					for ss.Next() {
						s := ss.At()
						if tc.expectedSuffix != "" {
							require.Contains(t, s.Labels().Get(model.MetricNameLabel), tc.expectedSuffix)
						}
						collected = append(collected, s)
					}
					require.NoError(t, ss.Err())
					require.Len(t, collected, tc.expectedCount)

					if tc.expectedSamples != nil {
						// Iterate collected series after draining the SeriesSet to verify
						// that each series's sample slab remains valid across Next() calls
						// and that fSampleSeries.Iterator reuses an existing fSampleIterator.
						gotSamples := make([]seriesSamples, 0, len(collected))
						var it chunkenc.Iterator
						for _, s := range collected {
							it = s.Iterator(it)
							var samples []fSample
							for it.Next() == chunkenc.ValFloat {
								ts, v := it.At()
								require.Equal(t, ts, it.AtT())
								require.Equal(t, int64(0), it.AtST())
								samples = append(samples, fSample{t: ts, f: v})
							}
							require.NoError(t, it.Err())
							gotSamples = append(gotSamples, seriesSamples{labels: s.Labels().String(), samples: samples})

							// Also verify Seek on the same series using iterator reuse.
							if len(samples) > 0 {
								it = s.Iterator(it)
								mid := samples[len(samples)/2]
								require.Equal(t, chunkenc.ValFloat, it.Seek(mid.t))
								ts, v := it.At()
								require.Equal(t, mid.t, ts)
								require.Equal(t, mid.f, v)
								require.Equal(t, chunkenc.ValNone, it.Seek(samples[len(samples)-1].t+100))
								require.Panics(t, func() { it.AtHistogram(nil) })
								require.Panics(t, func() { it.AtFloatHistogram(nil) })
							}
						}
						require.Equal(t, tc.expectedSamples, gotSamples)
					}
				})
			}
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
		classicSeries: []Series{},
		nhcbSeries: []Series{
			NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "web"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
		classicSeries: []Series{},
		nhcbSeries: []Series{
			NewListSeries(labels.FromStrings("__name__", "latency"), []chunks.Sample{fhSample{t: 1, fh: fhNHCB}}),
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

// makeNHCB builds a custom buckets histogram from per-bucket counts, the last
// count being the +Inf bucket.
func makeNHCB(customValues []float64, counts []int64) *histogram.Histogram {
	deltas := make([]int64, len(counts))
	var total uint64
	var prev int64
	for i, c := range counts {
		deltas[i] = c - prev
		prev = c
		total += uint64(c)
	}
	return &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           total,
		Sum:             float64(total),
		CustomValues:    customValues,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: uint32(len(counts))}},
		PositiveBuckets: deltas,
	}
}

// seriesSamples is a series' labels and float samples, in the order returned.
type seriesSamples struct {
	labels  string
	samples []fSample
}

type nhcbMockQuerier struct {
	classicSeries     []Series
	nhcbSeries        []Series
	passthroughSeries []Series // For non-histogram queries

	// For error/warning injection in tests.
	classicErr   error
	nhcbErr      error
	nhcbWarnings annotations.Annotations
}

func (m *nhcbMockQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			continue
		}
		// Check if this is a histogram suffix query (classic histogram query)
		if strings.HasSuffix(matcher.Value, "_bucket") ||
			strings.HasSuffix(matcher.Value, "_count") ||
			strings.HasSuffix(matcher.Value, "_sum") {
			if m.classicErr != nil {
				return ErrSeriesSet(m.classicErr)
			}
			var matched []Series
			for _, s := range m.classicSeries {
				if matchesAll(s.Labels(), matchers) {
					matched = append(matched, s)
				}
			}
			return NewMockSeriesSet(matched...)
		}
		// If passthroughSeries is set, use it for non-histogram metric queries
		if len(m.passthroughSeries) > 0 {
			return NewMockSeriesSet(m.passthroughSeries...)
		}
		// Base metric name query - return NHCB series
		if m.nhcbErr != nil {
			return ErrSeriesSet(m.nhcbErr)
		}
		var matched []Series
		for _, s := range m.nhcbSeries {
			if matchesAll(s.Labels(), matchers) {
				matched = append(matched, s)
			}
		}
		return &mockSeriesSet{idx: -1, series: matched, warnings: m.nhcbWarnings}
	}
	return NewMockSeriesSet()
}

func (*nhcbMockQuerier) LabelValues(context.Context, string, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbMockQuerier) LabelNames(context.Context, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbMockQuerier) Close() error {
	return nil
}

func matchesAll(lset labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(lset.Get(m.Name)) {
			return false
		}
	}
	return true
}

// deferredErrSeriesSet returns series normally but reports a non-nil error only
// after Next() has been exhausted. This lets the pre-iteration Err() check pass
// while still exercising error paths that are evaluated after draining the set.
type deferredErrSeriesSet struct {
	series    []Series
	idx       int
	err       error
	exhausted bool
}

func newDeferredErrSeriesSet(err error, series ...Series) SeriesSet {
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

func (s *deferredErrSeriesSet) At() Series { return s.series[s.idx] }

func (s *deferredErrSeriesSet) Err() error {
	if s.exhausted {
		return s.err
	}
	return nil
}

func (*deferredErrSeriesSet) Warnings() annotations.Annotations { return nil }

// nhcbSetQuerier routes suffix queries (_bucket/_count/_sum) to classicSet and
// all other queries to nhcbSet. This lets tests inject arbitrary SeriesSet
// implementations for either path without duplicating routing logic.
type nhcbSetQuerier struct {
	classicSet SeriesSet
	nhcbSet    SeriesSet
}

func (m *nhcbSetQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
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
	return NewMockSeriesSet()
}

func (*nhcbSetQuerier) LabelValues(context.Context, string, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*nhcbSetQuerier) LabelNames(context.Context, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
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
		querier Querier
	}{
		{
			name: "classic set immediate error",
			querier: &nhcbMockQuerier{
				classicErr: testError,
				nhcbSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
				},
			},
		},
		{
			name: "nhcb set immediate error",
			querier: &nhcbMockQuerier{
				classicSeries: []Series{},
				nhcbErr:       testError,
			},
		},
		{
			// The nhcb set's Err() is nil initially (passes the pre-check) but
			// becomes non-nil once Next() is exhausted, exercising the error
			// path inside nhcbToClassicSeriesSet.Next().
			name: "nhcb set error during iteration inside nhcbToClassicSeriesSet",
			querier: &nhcbSetQuerier{
				classicSet: NewMockSeriesSet(),
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
	nhcbSeries := NewListSeries(
		labels.FromStrings("__name__", "http_requests"),
		[]chunks.Sample{hSample{t: 1, h: nhcb}},
	)

	t.Run("nhcb set warnings propagate", func(t *testing.T) {
		warn := annotations.New().Add(errors.New("nhcb warning"))
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{},
			nhcbSeries:    []Series{nhcbSeries},
			nhcbWarnings:  warn,
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
		series := []Series{NewListSeries(labels.FromStrings("__name__", "my_gauge"), []chunks.Sample{fSample{t: 1, f: 1}})}
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
