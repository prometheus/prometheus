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
	"math"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
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
			name:           "bucket regex matcher is not rewritten",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_bucket")},
			expectedName:   "",
			expectedSuffix: "",
		},
		{
			name:           "bucket not-equal matcher is not rewritten",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket")},
			expectedName:   "",
			expectedSuffix: "",
		},
		{
			name: "contradictory metric name matchers are not rewritten",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests_bucket"),
			},
			expectedName:   "",
			expectedSuffix: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher, suffix, _, _ := extractHistogramSuffix(tc.matchers)
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
	makeNHCB := func(customValues []float64, counts []int64) *histogram.Histogram {
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

	tests := []struct {
		name              string
		queryMatchers     []*labels.Matcher
		classicSeries     []Series
		nhcbSeries        []Series
		passthroughSeries []Series
		expectedCount     int
		expectedSuffix    string
		expectedSamples   map[string][]fSample
	}{
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
			name:          "histogram with regex name matcher passes through without NHCB conversion",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+_requests_bucket")},
			classicSeries: []Series{},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
			name:          "both classic and NHCB for same series and timestamp - stored classic wins",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  1,
			expectedSuffix: "_bucket",
		},
		{
			name:          "both classic and NHCB for different series - return both",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "job", "classic", "le", "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "nhcb"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
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
			name: "stored classic count series with le label does not shadow NHCB count series without le label",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count"),
			},
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "le", "1.0"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedCount:  2,
			expectedSuffix: "_count",
		},
		{
			name:          "NHCB series that already has le label is ignored",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries: []Series{},
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
			expectedSamples: map[string][]fSample{
				`{__name__="http_requests_bucket", job="api", le="1.0"}`:  {{t: 1, f: 1}, {t: 2, f: 2}, {t: 3, f: 3}, {t: 4, f: 4}},
				`{__name__="http_requests_bucket", job="api", le="2.0"}`:  {{t: 1, f: 3}, {t: 2, f: 6}, {t: 3, f: math.Float64frombits(value.StaleNaN)}, {t: 4, f: 9}},
				`{__name__="http_requests_bucket", job="api", le="3.0"}`:  {{t: 1, f: 6}, {t: 2, f: 12}, {t: 3, f: math.Float64frombits(value.StaleNaN)}, {t: 4, f: 15}},
				`{__name__="http_requests_bucket", job="api", le="5.0"}`:  {{t: 3, f: 10}, {t: 4, f: math.Float64frombits(value.StaleNaN)}},
				`{__name__="http_requests_bucket", job="api", le="+Inf"}`: {{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 15}, {t: 4, f: 22}},
				`{__name__="http_requests_bucket", job="web", le="1.0"}`:  {{t: 1, f: 5}, {t: 2, f: 6}},
				`{__name__="http_requests_bucket", job="web", le="5.0"}`:  {{t: 1, f: 15}, {t: 2, f: 18}},
				`{__name__="http_requests_bucket", job="web", le="+Inf"}`: {{t: 1, f: 30}, {t: 2, f: 36}},
			},
		},
		{
			name: "summary _count query ignores float quantile series on base name and passes through classic _count",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_duration_seconds_count"),
			},
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "rpc_duration_seconds_count", "job", "api"), []chunks.Sample{
					fSample{t: 1, f: 42},
					fSample{t: 2, f: 84},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "rpc_duration_seconds", "job", "api", "quantile", "0.5"), []chunks.Sample{
					fSample{t: 1, f: 0.12},
					fSample{t: 2, f: 0.15},
				}),
				NewListSeries(labels.FromStrings("__name__", "rpc_duration_seconds", "job", "api", "quantile", "0.99"), []chunks.Sample{
					fSample{t: 1, f: 0.45},
					fSample{t: 2, f: 0.50},
				}),
			},
			expectedCount:  1,
			expectedSuffix: "_count",
			expectedSamples: map[string][]fSample{
				`{__name__="rpc_duration_seconds_count", job="api"}`: {{t: 1, f: 42}, {t: 2, f: 84}},
			},
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
					exp, ok := tc.expectedSamples[s.Labels().String()]
					require.True(t, ok, "unexpected series %s", s.Labels())
					require.Len(t, samples, len(exp))
					for i := range exp {
						require.Equal(t, exp[i].t, samples[i].t)
						require.Equal(t, math.Float64bits(exp[i].f), math.Float64bits(samples[i].f))
					}

					// Also verify Seek on the same series using iterator reuse.
					if len(samples) > 0 {
						it = s.Iterator(it)
						mid := samples[len(samples)/2]
						require.Equal(t, chunkenc.ValFloat, it.Seek(mid.t))
						ts, v := it.At()
						require.Equal(t, mid.t, ts)
						require.Equal(t, math.Float64bits(mid.f), math.Float64bits(v))
						require.Equal(t, chunkenc.ValNone, it.Seek(samples[len(samples)-1].t+100))
						if _, isFSampleSeries := s.(*fSampleSeries); isFSampleSeries {
							require.Panics(t, func() { it.AtHistogram(nil) })
							require.Panics(t, func() { it.AtFloatHistogram(nil) })
						}
					}
				}
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

	t.Run("sortSeries=false sorts each group by labels.Compare", func(t *testing.T) {
		for range 5 {
			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
			var seriesLabels []string
			for ss.Next() {
				seriesLabels = append(seriesLabels, ss.At().Labels().String())
			}
			require.NoError(t, ss.Err())

			expectedOrder := []string{
				`{__name__="http_requests_bucket", job="api", le="+Inf"}`,
				`{__name__="http_requests_bucket", job="api", le="1.0"}`,
				`{__name__="http_requests_bucket", job="api", le="10.0"}`,
				`{__name__="http_requests_bucket", job="api", le="5.0"}`,
				`{__name__="http_requests_bucket", job="web", le="+Inf"}`,
				`{__name__="http_requests_bucket", job="web", le="1.0"}`,
				`{__name__="http_requests_bucket", job="web", le="10.0"}`,
				`{__name__="http_requests_bucket", job="web", le="5.0"}`,
			}
			require.Equal(t, expectedOrder, seriesLabels)
		}
	})

	t.Run("sortSeries=true sorts globally across groups by labels.Compare", func(t *testing.T) {
		// Use a label name ("z_job") that sorts after "le" so global order differs from per-group order.
		mockGlobal := &nhcbMockQuerier{
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "z_job", "api"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
				NewListSeries(labels.FromStrings("__name__", "http_requests", "z_job", "web"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
		}
		qGlobal := NewNHCBAsClassicQuerier(mockGlobal)

		ss := qGlobal.Select(context.Background(), true, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
		var seriesLabels []string
		for ss.Next() {
			seriesLabels = append(seriesLabels, ss.At().Labels().String())
		}
		require.NoError(t, ss.Err())

		expectedGlobalOrder := []string{
			`{__name__="http_requests_bucket", le="+Inf", z_job="api"}`,
			`{__name__="http_requests_bucket", le="+Inf", z_job="web"}`,
			`{__name__="http_requests_bucket", le="1.0", z_job="api"}`,
			`{__name__="http_requests_bucket", le="1.0", z_job="web"}`,
			`{__name__="http_requests_bucket", le="10.0", z_job="api"}`,
			`{__name__="http_requests_bucket", le="10.0", z_job="web"}`,
			`{__name__="http_requests_bucket", le="5.0", z_job="api"}`,
			`{__name__="http_requests_bucket", le="5.0", z_job="web"}`,
		}
		require.Equal(t, expectedGlobalOrder, seriesLabels)
	})
}

func TestNHCBAsClassicQuerier_Collisions(t *testing.T) {
	staleF := math.Float64frombits(value.StaleNaN)
	// nhcb builds a valid CustomBucketsSchema Histogram from per-bucket
	// observation counts (last entry is the +Inf bucket).
	nhcb := func(sum float64, customValues []float64, bucketCounts []int64) *histogram.Histogram {
		deltas := make([]int64, len(bucketCounts))
		var total uint64
		var prev int64
		for i, c := range bucketCounts {
			deltas[i] = c - prev
			prev = c
			total += uint64(c)
		}
		return &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			Count:           total,
			Sum:             sum,
			CustomValues:    customValues,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: uint32(len(bucketCounts))}},
			PositiveBuckets: deltas,
		}
	}
	staleNHCB := &histogram.Histogram{
		Sum: staleF,
	}

	type seriesSamples struct {
		labels  string
		samples []fSample
	}

	readAll := func(t *testing.T, ss SeriesSet) []seriesSamples {
		t.Helper()
		var out []seriesSamples
		var it chunkenc.Iterator
		for ss.Next() {
			s := ss.At()
			it = s.Iterator(it)
			var samples []fSample
			for it.Next() == chunkenc.ValFloat {
				ts, v := it.At()
				samples = append(samples, fSample{t: ts, f: v})
			}
			require.NoError(t, it.Err())
			out = append(out, seriesSamples{
				labels:  s.Labels().String(),
				samples: samples,
			})
		}
		require.NoError(t, ss.Err())
		return out
	}

	assertSeriesSamplesEqual := func(t *testing.T, expected, actual []seriesSamples) {
		t.Helper()
		require.Len(t, actual, len(expected))
		for i := range expected {
			require.Equal(t, expected[i].labels, actual[i].labels, "series %d labels mismatch", i)
			require.Len(t, actual[i].samples, len(expected[i].samples), "series %s sample count mismatch", expected[i].labels)
			for j := range expected[i].samples {
				exp := expected[i].samples[j]
				got := actual[i].samples[j]
				require.Equal(t, exp.t, got.t, "series %s sample %d timestamp mismatch", expected[i].labels, j)
				if value.IsStaleNaN(exp.f) {
					require.True(t, value.IsStaleNaN(got.f), "series %s sample %d at t=%d expected StaleNaN, got %v", expected[i].labels, j, exp.t, got.f)
				} else {
					require.Equal(t, exp.f, got.f, "series %s sample %d at t=%d value mismatch", expected[i].labels, j, exp.t)
				}
			}
		}
	}

	t.Run("identical labelset and timestamp (dual classic + NHCB scrape): stored classic wins", func(t *testing.T) {
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
					fSample{t: 1, f: 10},
					fSample{t: 2, f: 20},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
					hSample{t: 1, h: nhcb(100, []float64{1.0}, []int64{50, 49})},
					hSample{t: 2, h: nhcb(200, []float64{1.0}, []int64{100, 99})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_count", job="api"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
			},
		}, got)
	})

	t.Run("disjoint timestamps across migration cutover: merged into single continuous series", func(t *testing.T) {
		// 1. Classic (t=1,2) -> NHCB (t=3,4) forward migration.
		qForward := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
					fSample{t: 1, f: 10},
					fSample{t: 2, f: 20},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
					hSample{t: 3, h: nhcb(300, []float64{1.0}, []int64{15, 15})},
					hSample{t: 4, h: nhcb(400, []float64{1.0}, []int64{20, 20})},
				}),
			},
		})
		gotForward := readAll(t, qForward.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_count", job="api"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 30}, {t: 4, f: 40}},
			},
		}, gotForward)

		// 2. NHCB (t=1,2) -> Classic (t=3,4) rollback where stored classic continues after converted samples end.
		qRollback := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
					fSample{t: 3, f: 30},
					fSample{t: 4, f: 40},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
					hSample{t: 1, h: nhcb(100, []float64{1.0}, []int64{5, 5})},
					hSample{t: 2, h: nhcb(200, []float64{1.0}, []int64{10, 10})},
				}),
			},
		})
		gotRollback := readAll(t, qRollback.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_count", job="api"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 30}, {t: 4, f: 40}},
			},
		}, gotRollback)
	})

	t.Run("same-timestamp staleness marker vs live sample at cutover: live sample wins", func(t *testing.T) {
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
					fSample{t: 1, f: 10},
					fSample{t: 2, f: staleF}, // classic marked stale at cutover t=2
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
					hSample{t: 2, h: nhcb(200, []float64{1.0}, []int64{10, 10})}, // NHCB starts at t=2
					hSample{t: 3, h: nhcb(300, []float64{1.0}, []int64{15, 15})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_count", job="api"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 30}},
			},
		}, got)
	})

	t.Run("staggered staleness marker from old classic scrape after NHCB cutover is dropped", func(t *testing.T) {
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
					fSample{t: 1, f: 10},
					fSample{t: 3, f: staleF}, // delayed classic StaleNaN after NHCB already started at t=2
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
					hSample{t: 2, h: nhcb(200, []float64{1.0}, []int64{10, 10})},
					hSample{t: 4, h: nhcb(400, []float64{1.0}, []int64{20, 20})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_count", job="api"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 4, f: 40}},
			},
		}, got)
	})

	t.Run("different bucket layouts or unnormalized le at same timestamp: stored classic shadows NHCB group", func(t *testing.T) {
		// At t=1, stored classic has unnormalized le="1" and "+Inf".
		// NHCB at t=1 has le="1.0", "5.0", "+Inf". Stored classic must shadow
		// all NHCB buckets at t=1 so le="1.0" and le="5.0" do not leak at t=1.
		// At t=2, only NHCB has a sample, so converted buckets are emitted at t=2.
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1"), []chunks.Sample{
					fSample{t: 1, f: 5},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf"), []chunks.Sample{
					fSample{t: 1, f: 10},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0, 5.0}, []int64{5, 3, 2})},
					hSample{t: 2, h: nhcb(100, []float64{1.0, 5.0}, []int64{10, 6, 4})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1"}`,
				samples: []fSample{{t: 1, f: 5}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 2, f: 10}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="5.0"}`,
				samples: []fSample{{t: 2, f: 16}},
			},
		}, got)
	})

	t.Run("partial le matcher still shadows NHCB when stored classic exists at timestamp", func(t *testing.T) {
		// Stored classic at t=1 only has le="1.0" and "+Inf" (no le="5.0").
		// Query selects le="5.0". At t=1 stored classic is active, so NHCB's
		// le="5.0" must still be shadowed at t=1 and only appear at t=2.
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1.0"), []chunks.Sample{
					fSample{t: 1, f: 5},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf"), []chunks.Sample{
					fSample{t: 1, f: 10},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0, 5.0}, []int64{5, 3, 2})},
					hSample{t: 2, h: nhcb(100, []float64{1.0, 5.0}, []int64{10, 6, 4})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
			labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "5.0")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="5.0"}`,
				samples: []fSample{{t: 2, f: 16}},
			},
		}, got)
	})

	t.Run("NHCB to classic cutover marks dropped NHCB buckets stale when classic takes over", func(t *testing.T) {
		// At t=1, NHCB has le="1.0", "5.0", "+Inf".
		// At t=2, stored classic takes over with only le="1.0", "+Inf" (and NHCB has no sample at t=2).
		// Converted le="5.0" must receive a StaleNaN at t=2 so it does not linger in PromQL's lookback window.
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1.0"), []chunks.Sample{
					fSample{t: 2, f: 12},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf"), []chunks.Sample{
					fSample{t: 2, f: 20},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0, 5.0}, []int64{5, 3, 2})},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 1, f: 5}, {t: 2, f: 12}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="5.0"}`,
				samples: []fSample{{t: 1, f: 8}, {t: 2, f: staleF}},
			},
		}, got)
	})

	t.Run("NHCB staleness marker and bucket layout change emit StaleNaN on converted series", func(t *testing.T) {
		// t=1: CustomValues [1.0, 5.0]
		// t=2: CustomValues [1.0] (bucket 5.0 removed -> gets StaleNaN at t=2)
		// t=3: histogram NHCB stale marker -> active series (le="1.0", "+Inf") get StaleNaN at t=3
		// t=4: CustomValues [1.0] (resumes)
		// t=5: float StaleNaN (common scrape staleness marker) -> active series get StaleNaN at t=5
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0, 5.0}, []int64{5, 3, 2})},
					hSample{t: 2, h: nhcb(75, []float64{1.0}, []int64{8, 7})},
					hSample{t: 3, h: staleNHCB},
					hSample{t: 4, h: nhcb(90, []float64{1.0}, []int64{10, 8})},
					fSample{t: 5, f: staleF},
				}),
			},
		})
		got := readAll(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 15}, {t: 3, f: staleF}, {t: 4, f: 18}, {t: 5, f: staleF}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 1, f: 5}, {t: 2, f: 8}, {t: 3, f: staleF}, {t: 4, f: 10}, {t: 5, f: staleF}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="5.0"}`,
				samples: []fSample{{t: 1, f: 8}, {t: 2, f: staleF}},
			},
		}, got)
	})

	t.Run("exponential native histogram is never converted and NHCB to NHE transition emits StaleNaN", func(t *testing.T) {
		expNH := &histogram.Histogram{
			Schema:          3,
			Count:           30,
			Sum:             120,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
			PositiveBuckets: []int64{15, 0},
		}

		// 1. Pure NHE series: ignored, 0 classic series returned.
		qPureNHE := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: expNH},
				}),
			},
		})
		gotPure := readAll(t, qPureNHE.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		require.Empty(t, gotPure)

		// 2. NHE series coexisting with stored classic series: stored classic is returned untouched.
		qNHEWithClassic := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf"), []chunks.Sample{
					fSample{t: 1, f: 10},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1.0"), []chunks.Sample{
					fSample{t: 1, f: 5},
				}),
			},
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: expNH},
					hSample{t: 2, h: expNH},
				}),
			},
		})
		gotWithClassic := readAll(t, qNHEWithClassic.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 1, f: 5}},
			},
		}, gotWithClassic)

		// 3. Same base series transitioning NHCB (t=1) -> NHE (t=2) -> NHCB (t=3):
		// t=1 is converted to classic, t=2 emits StaleNaN on the converted series, and t=3 resumes conversion.
		qTransition := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0}, []int64{5, 5})},
					hSample{t: 2, h: expNH},
					hSample{t: 3, h: nhcb(75, []float64{1.0}, []int64{8, 7})},
				}),
			},
		})
		gotTransition := readAll(t, qTransition.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: staleF}, {t: 3, f: 15}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 1, f: 5}, {t: 2, f: staleF}, {t: 3, f: 8}},
			},
		}, gotTransition)
	})

	t.Run("NewMergeQuerier with sortSeries=true deduplicates converted and remote series", func(t *testing.T) {
		// Primary querier has NHCB series; secondary querier (e.g. remote_read)
		// has classic series with overlapping le="+Inf". Because NewMergeQuerier
		// requires sorted SeriesSets, NHCBAsClassicQuerier must return series in
		// labels.Compare order so le="+Inf" is merged rather than emitted twice.
		primary := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			nhcbSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests"), []chunks.Sample{
					hSample{t: 1, h: nhcb(50, []float64{1.0, 5.0}, []int64{5, 3, 2})},
				}),
			},
		})
		secondary := &nhcbMockQuerier{
			classicSeries: []Series{
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf"), []chunks.Sample{
					fSample{t: 2, f: 20},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1.0"), []chunks.Sample{
					fSample{t: 2, f: 10},
				}),
				NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "5.0"), []chunks.Sample{
					fSample{t: 2, f: 16},
				}),
			},
		}
		merged := NewMergeQuerier([]Querier{primary}, []Querier{secondary}, ChainedSeriesMerge)
		got := readAll(t, merged.Select(context.Background(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0"}`,
				samples: []fSample{{t: 1, f: 5}, {t: 2, f: 10}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="5.0"}`,
				samples: []fSample{{t: 1, f: 8}, {t: 2, f: 16}},
			},
		}, got)
	})

	t.Run("multi-group hybrid collision with classic and multiple NHCB series respects sortSeries", func(t *testing.T) {
		// Use label "z" (which sorts after "le") so that per-group sorting
		// (sortSeries=false) and global sorting across groups (sortSeries=true)
		// produce distinct series orders.
		newQuerier := func() Querier {
			return NewNHCBAsClassicQuerier(&nhcbMockQuerier{
				classicSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "1.0", "z", "a"), []chunks.Sample{
						fSample{t: 1, f: 3},
					}),
					NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "le", "+Inf", "z", "a"), []chunks.Sample{
						fSample{t: 1, f: 10},
					}),
				},
				nhcbSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests", "z", "a"), []chunks.Sample{
						hSample{t: 1, h: nhcb(50, []float64{1.0}, []int64{5, 5})},
						hSample{t: 2, h: nhcb(100, []float64{1.0}, []int64{8, 7})},
					}),
					NewListSeries(labels.FromStrings("__name__", "http_requests", "z", "b"), []chunks.Sample{
						hSample{t: 1, h: nhcb(30, []float64{1.0}, []int64{2, 4})},
					}),
				},
			})
		}

		gotUnsorted := readAll(t, newQuerier().Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf", z="a"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 15}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0", z="a"}`,
				samples: []fSample{{t: 1, f: 3}, {t: 2, f: 8}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf", z="b"}`,
				samples: []fSample{{t: 1, f: 6}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0", z="b"}`,
				samples: []fSample{{t: 1, f: 2}},
			},
		}, gotUnsorted)

		gotSorted := readAll(t, newQuerier().Select(context.Background(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")))
		assertSeriesSamplesEqual(t, []seriesSamples{
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf", z="a"}`,
				samples: []fSample{{t: 1, f: 10}, {t: 2, f: 15}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="+Inf", z="b"}`,
				samples: []fSample{{t: 1, f: 6}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0", z="a"}`,
				samples: []fSample{{t: 1, f: 3}, {t: 2, f: 8}},
			},
			{
				labels:  `{__name__="http_requests_bucket", le="1.0", z="b"}`,
				samples: []fSample{{t: 1, f: 2}},
			},
		}, gotSorted)
	})

	t.Run("control label __nhcb_as_classic__ toggles conversion and debug __from_nhcb__ output label", func(t *testing.T) {
		newDualQuerier := func() Querier {
			return NewNHCBAsClassicQuerier(&nhcbMockQuerier{
				classicSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
						fSample{t: 1, f: 10},
						fSample{t: 2, f: 20},
					}),
				},
				nhcbSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
						hSample{t: 2, h: nhcb(200, []float64{1.0}, []int64{10, 15})}, // Shadowed at t=2 by stored classic, except in debug mode.
						hSample{t: 3, h: nhcb(300, []float64{1.0}, []int64{15, 15})},
						hSample{t: 4, h: nhcb(400, []float64{1.0}, []int64{20, 20})},
					}),
				},
			})
		}

		newMixedBucketQuerier := func() Querier {
			// Group job="web" has stored classic and NHCB series, group job="api" has
			// NHCB only. Classic series are indexed first, so job="web" is the first group.
			return NewNHCBAsClassicQuerier(&nhcbMockQuerier{
				classicSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "job", "web", "le", "+Inf"), []chunks.Sample{
						fSample{t: 1, f: 6},
					}),
					NewListSeries(labels.FromStrings("__name__", "http_requests_bucket", "job", "web", "le", "1.0"), []chunks.Sample{
						fSample{t: 1, f: 4},
					}),
				},
				nhcbSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "web"), []chunks.Sample{
						hSample{t: 3, h: nhcb(30, []float64{1.0}, []int64{7, 3})},
					}),
					NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
						hSample{t: 3, h: nhcb(50, []float64{1.0}, []int64{10, 5})},
					}),
				},
			})
		}
		debugBucketLe := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
			labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1.0"),
			labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug"),
		}

		countName := labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")
		for _, tc := range []struct {
			name        string
			querier     Querier // Defaults to newDualQuerier().
			sortSeries  bool
			matchers    []*labels.Matcher
			expected    []seriesSamples
			expectedErr error
		}{
			{
				name:     "equal true keeps conversion on without debug label",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "true")},
				expected: []seriesSamples{
					{
						labels:  `{__name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}, {t: 3, f: 30}, {t: 4, f: 40}},
					},
				},
			},
			{
				name:     "equal false turns conversion off and returns stored classic only",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "false")},
				expected: []seriesSamples{
					{
						labels:  `{__name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
					},
				},
			},
			{
				name:     "not-equal true turns conversion off without triggering debug mode",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchNotEqual, NHCBAsClassicLabel, "true")},
				expected: []seriesSamples{
					{
						labels:  `{__name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
					},
				},
			},
			{
				name:     "equal empty turns conversion off",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "")},
				expected: []seriesSamples{
					{
						labels:  `{__name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
					},
				},
			},
			{
				name:     "equal debug emits __from_nhcb__=false/true on separate series",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug")},
				expected: []seriesSamples{
					{
						labels:  `{__from_nhcb__="false", __name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
					},
					{
						labels:  `{__from_nhcb__="true", __name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 2, f: 25}, {t: 3, f: 30}, {t: 4, f: 40}},
					},
				},
			},
			{
				name:     "regexp true|debug emits __from_nhcb__=false/true on separate series",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchRegexp, NHCBAsClassicLabel, "true|debug")},
				expected: []seriesSamples{
					{
						labels:  `{__from_nhcb__="false", __name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}},
					},
					{
						labels:  `{__from_nhcb__="true", __name__="http_requests_count", job="api"}`,
						samples: []fSample{{t: 2, f: 25}, {t: 3, f: 30}, {t: 4, f: 40}},
					},
				},
			},
			{
				name:     "unknown control value returns empty set",
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "invalid")},
				expected: nil,
			},
			{
				name: "debug with classic-only and NHCB-only groups labels each group by source",
				querier: NewNHCBAsClassicQuerier(&nhcbMockQuerier{
					classicSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "a"), []chunks.Sample{
							fSample{t: 1, f: 10},
						}),
					},
					nhcbSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "b"), []chunks.Sample{
							hSample{t: 1, h: nhcb(100, []float64{1.0}, []int64{10, 5})},
						}),
					},
				}),
				matchers: []*labels.Matcher{countName, labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug")},
				expected: []seriesSamples{
					{
						labels:  `{__from_nhcb__="false", __name__="http_requests_count", job="a"}`,
						samples: []fSample{{t: 1, f: 10}},
					},
					{
						labels:  `{__from_nhcb__="true", __name__="http_requests_count", job="b"}`,
						samples: []fSample{{t: 1, f: 15}},
					},
				},
			},
			{
				name:     "debug with _bucket and le matcher on mixed groups, sortSeries=false sorts per group",
				querier:  newMixedBucketQuerier(),
				matchers: debugBucketLe,
				expected: []seriesSamples{
					{labels: `{__from_nhcb__="false", __name__="http_requests_bucket", job="web", le="1.0"}`, samples: []fSample{{t: 1, f: 4}}},
					{labels: `{__from_nhcb__="true", __name__="http_requests_bucket", job="web", le="1.0"}`, samples: []fSample{{t: 3, f: 7}}},
					{labels: `{__from_nhcb__="true", __name__="http_requests_bucket", job="api", le="1.0"}`, samples: []fSample{{t: 3, f: 10}}},
				},
			},
			{
				name:       "debug with _bucket and le matcher on mixed groups, sortSeries=true sorts globally",
				querier:    newMixedBucketQuerier(),
				sortSeries: true,
				matchers:   debugBucketLe,
				expected: []seriesSamples{
					{labels: `{__from_nhcb__="false", __name__="http_requests_bucket", job="web", le="1.0"}`, samples: []fSample{{t: 1, f: 4}}},
					{labels: `{__from_nhcb__="true", __name__="http_requests_bucket", job="api", le="1.0"}`, samples: []fSample{{t: 3, f: 10}}},
					{labels: `{__from_nhcb__="true", __name__="http_requests_bucket", job="web", le="1.0"}`, samples: []fSample{{t: 3, f: 7}}},
				},
			},
			{
				name: "debug on non-histogram selector labels passthrough series as not converted",
				querier: NewNHCBAsClassicQuerier(&nhcbMockQuerier{
					passthroughSeries: []Series{
						NewListSeries(labels.FromStrings("__name__", "up", "job", "api"), []chunks.Sample{fSample{t: 1, f: 1}}),
					},
				}),
				matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up"),
					labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug"),
				},
				expected: []seriesSamples{
					{labels: `{__from_nhcb__="false", __name__="up", job="api"}`, samples: []fSample{{t: 1, f: 1}}},
				},
			},
			{
				name: "debug on regexp __name__ selector labels unconverted stored classic series",
				matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "http_requests_count"),
					labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug"),
				},
				expected: []seriesSamples{
					{labels: `{__from_nhcb__="false", __name__="http_requests_count", job="api"}`, samples: []fSample{{t: 1, f: 10}, {t: 2, f: 20}}},
				},
			},
			{
				name:        "selector with only control matchers returns error",
				matchers:    []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "true")},
				expectedErr: errOnlyControlMatchers,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				q := tc.querier
				if q == nil {
					q = newDualQuerier()
				}
				ss := q.Select(context.Background(), tc.sortSeries, nil, tc.matchers...)
				if tc.expectedErr != nil {
					require.False(t, ss.Next())
					require.ErrorIs(t, ss.Err(), tc.expectedErr)
					return
				}
				got := readAll(t, ss)
				assertSeriesSamplesEqual(t, tc.expected, got)
			})
		}

		t.Run("debug mode on pure NHCB fast path emits __from_nhcb__=true", func(t *testing.T) {
			q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
				nhcbSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests", "job", "api"), []chunks.Sample{
						hSample{t: 1, h: nhcb(100, []float64{1.0}, []int64{10, 5})},
					}),
				},
			})
			got := readAll(t, q.Select(context.Background(), false, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
				labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug"),
			))
			assertSeriesSamplesEqual(t, []seriesSamples{
				{
					labels:  `{__from_nhcb__="true", __name__="http_requests_bucket", job="api", le="+Inf"}`,
					samples: []fSample{{t: 1, f: 15}},
				},
				{
					labels:  `{__from_nhcb__="true", __name__="http_requests_bucket", job="api", le="1.0"}`,
					samples: []fSample{{t: 1, f: 10}},
				},
			}, got)
		})

		t.Run("debug mode on pure classic fast path emits __from_nhcb__=false", func(t *testing.T) {
			q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
				classicSeries: []Series{
					NewListSeries(labels.FromStrings("__name__", "http_requests_count", "job", "api"), []chunks.Sample{
						fSample{t: 1, f: 10},
					}),
				},
			})
			got := readAll(t, q.Select(context.Background(), false, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count"),
				labels.MustNewMatcher(labels.MatchEqual, NHCBAsClassicLabel, "debug"),
			))
			assertSeriesSamplesEqual(t, []seriesSamples{
				{
					labels:  `{__from_nhcb__="false", __name__="http_requests_count", job="api"}`,
					samples: []fSample{{t: 1, f: 10}},
				},
			}, got)
		})

	})
}

func TestExtractControlMatchers(t *testing.T) {
	name := labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")
	le := labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1.0")
	ctrl := func(mt labels.MatchType, val string) *labels.Matcher {
		return labels.MustNewMatcher(mt, NHCBAsClassicLabel, val)
	}

	for _, tc := range []struct {
		name         string
		matchers     []*labels.Matcher
		wantStripped []*labels.Matcher
		wantConvert  bool
		wantDebug    bool
		wantMatched  bool
		wantErr      error
	}{
		{
			name:         "no control matchers enables conversion",
			matchers:     []*labels.Matcher{name},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "equal true enables conversion",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchEqual, "true")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "non-control matchers including le are preserved in order",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchEqual, "true"), le},
			wantStripped: []*labels.Matcher{name, le},
			wantConvert:  true,
			wantMatched:  true,
		},
		{
			name:         "equal false disables conversion",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchEqual, "false")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  false,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "not-equal true disables conversion without debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchNotEqual, "true")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  false,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "not-equal false enables conversion without debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchNotEqual, "false")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "not-equal debug enables conversion without debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchNotEqual, "debug")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "equal debug enables conversion and debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchEqual, "debug")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    true,
			wantMatched:  true,
		},
		{
			name:         "regexp true|debug enables conversion and debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchRegexp, "true|debug")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  true,
			wantDebug:    true,
			wantMatched:  true,
		},
		{
			name:         "regexp matching false disables conversion without debug",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchRegexp, "false|debug")},
			wantStripped: []*labels.Matcher{name},
			wantConvert:  false,
			wantDebug:    false,
			wantMatched:  true,
		},
		{
			name:         "contradictory matchers match nothing",
			matchers:     []*labels.Matcher{name, ctrl(labels.MatchEqual, "true"), ctrl(labels.MatchEqual, "false")},
			wantStripped: []*labels.Matcher{name},
			wantMatched:  false,
		},
		{
			name:     "only control matchers returns error",
			matchers: []*labels.Matcher{ctrl(labels.MatchEqual, "true")},
			wantErr:  errOnlyControlMatchers,
		},
		{
			name: "only empty-matching matchers besides control matcher returns error",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", ".*"),
				ctrl(labels.MatchEqual, "debug"),
			},
			wantErr: errOnlyControlMatchers,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stripped, convert, debug, matched, err := extractControlMatchers(tc.matchers)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantConvert, convert)
			require.Equal(t, tc.wantDebug, debug)
			require.Equal(t, tc.wantMatched, matched)
			require.Equal(t, tc.wantStripped, stripped)
		})
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

type nhcbMockQuerier struct {
	classicSeries     []Series
	nhcbSeries        []Series
	passthroughSeries []Series // For non-histogram queries

	// For error/warning injection in tests.
	classicErr      error
	nhcbErr         error
	classicWarnings annotations.Annotations
	nhcbWarnings    annotations.Annotations
}

func (m *nhcbMockQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name == NHCBAsClassicLabel {
			return ErrSeriesSet(errors.New("control matcher was not stripped before calling underlying Querier.Select"))
		}
	}
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
			return &mockSeriesSet{idx: -1, series: matched, warnings: m.classicWarnings}
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
			name: "nhcb set error during iteration",
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

	t.Run("classic and nhcb set warnings both propagate", func(t *testing.T) {
		classicWarn := annotations.New().Add(errors.New("classic warning"))
		nhcbWarn := annotations.New().Add(errors.New("nhcb warning"))
		q := NewNHCBAsClassicQuerier(&nhcbMockQuerier{
			classicSeries:   []Series{},
			nhcbSeries:      []Series{nhcbSeries},
			classicWarnings: classicWarn,
			nhcbWarnings:    nhcbWarn,
		})

		ss := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"))
		for ss.Next() {
		}
		require.NoError(t, ss.Err())
		var expected annotations.Annotations
		expected.Merge(classicWarn)
		expected.Merge(nhcbWarn)
		require.Equal(t, expected, ss.Warnings())
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
