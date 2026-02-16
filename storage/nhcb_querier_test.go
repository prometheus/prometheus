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
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestExtractHistogramSuffix(t *testing.T) {
	tests := []struct {
		name           string
		matchers       []*labels.Matcher
		expectedBase   string
		expectedSuffix string
	}{
		{
			name:           "bucket suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			expectedBase:   "http_requests",
			expectedSuffix: "_bucket",
		},
		{
			name:           "count suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_count")},
			expectedBase:   "http_requests",
			expectedSuffix: "_count",
		},
		{
			name:           "sum suffix",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_sum")},
			expectedBase:   "http_requests",
			expectedSuffix: "_sum",
		},
		{
			name:           "no suffix - regular metric",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "my_gauge")},
			expectedBase:   "",
			expectedSuffix: "",
		},
		{
			name:           "no metric name matcher",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus")},
			expectedBase:   "",
			expectedSuffix: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseName, suffix, _ := extractHistogramSuffix(tc.matchers)
			require.Equal(t, tc.expectedBase, baseName)
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
		classicSeries     []Series
		nhcbSeries        []Series
		passthroughSeries []Series
		expectedCount     int
		expectedSuffix    string
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
}

func (m *nhcbMockQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name == model.MetricNameLabel {
			// Check if this is a histogram suffix query (classic histogram query)
			if strings.HasSuffix(matcher.Value, "_bucket") ||
				strings.HasSuffix(matcher.Value, "_count") ||
				strings.HasSuffix(matcher.Value, "_sum") {
				return NewMockSeriesSet(m.classicSeries...)
			}
			// If passthroughSeries is set, use it for non-histogram metric queries
			if len(m.passthroughSeries) > 0 {
				return NewMockSeriesSet(m.passthroughSeries...)
			}
			// Base metric name query - return NHCB series
			return NewMockSeriesSet(m.nhcbSeries...)
		}
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
