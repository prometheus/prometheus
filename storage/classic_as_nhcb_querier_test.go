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
	"strconv"
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

func TestNewClassicSuffixMatcher(t *testing.T) {
	tests := []struct {
		name          string
		matcher       *labels.Matcher
		expectedValue string
		expectedNil   bool
	}{
		{
			name:          "equal matcher",
			matcher:       labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"),
			expectedValue: "http_requests(_bucket|_count|_sum)",
		},
		{
			name:          "equal matcher with regexp meta characters",
			matcher:       labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.requests"),
			expectedValue: `http\.requests(_bucket|_count|_sum)`,
		},
		{
			name:          "regexp matcher is grouped to keep the alternation intact",
			matcher:       labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "http_requests|rpc_latency"),
			expectedValue: "(?:http_requests|rpc_latency)(_bucket|_count|_sum)",
		},
		{
			name:        "no matcher",
			matcher:     nil,
			expectedNil: true,
		},
		{
			name:        "name already has a classic suffix",
			matcher:     labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket"),
			expectedNil: true,
		},
		{
			name:        "negative matcher matches an unbounded set of names",
			matcher:     labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "http_requests"),
			expectedNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newClassicSuffixMatcher(tc.matcher)
			if tc.expectedNil {
				require.Nil(t, m)
				return
			}
			require.NotNil(t, m)
			require.Equal(t, labels.MatchRegexp, m.Type)
			require.Equal(t, model.MetricNameLabel, m.Name)
			require.Equal(t, tc.expectedValue, m.Value)
			require.True(t, m.Matches("http_requests_count") || m.Matches("http.requests_count") || m.Matches("rpc_latency_count"))
			require.False(t, m.Matches("http_requests"))
		})
	}
}

// classicHistogramSeries returns the classic series of a histogram with the
// buckets le=1 and le=+Inf, observed at t=1 and t=2.
func classicHistogramSeries(extraLabels ...string) []Series {
	lbls := func(name string, kv ...string) labels.Labels {
		all := append([]string{model.MetricNameLabel, name}, kv...)
		return labels.FromStrings(append(all, extraLabels...)...)
	}
	return []Series{
		NewListSeries(lbls("http_requests_bucket", labels.BucketLabel, "1"), []chunks.Sample{fSample{t: 1, f: 2}, fSample{t: 2, f: 3}}),
		NewListSeries(lbls("http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 5}, fSample{t: 2, f: 7}}),
		NewListSeries(lbls("http_requests_count"), []chunks.Sample{fSample{t: 1, f: 5}, fSample{t: 2, f: 7}}),
		NewListSeries(lbls("http_requests_sum"), []chunks.Sample{fSample{t: 1, f: 10}, fSample{t: 2, f: 14}}),
	}
}

func TestClassicAsNHCBQuerier_Select(t *testing.T) {
	nhcb := &histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           5,
		Sum:             10,
		CustomValues:    []float64{1},
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
		PositiveBuckets: []int64{2, 1},
	}

	tests := []struct {
		name           string
		queryMatchers  []*labels.Matcher
		classicSeries  []Series
		nativeSeries   []Series
		expectedSeries []string
	}{
		{
			name:          "no classic histogram - native series passes through",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
			nativeSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedSeries: []string{`{__name__="http_requests"} @[1]`},
		},
		{
			name:          "classic histogram only - converted to NHCB",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
			classicSeries: classicHistogramSeries(),
			expectedSeries: []string{
				`{__name__="http_requests"} @[1] @[2]`,
			},
		},
		{
			name:          "classic histogram only - matched by regexp",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "http_.*")},
			classicSeries: classicHistogramSeries(),
			expectedSeries: []string{
				`{__name__="http_requests"} @[1] @[2]`,
			},
		},
		{
			name:          "one classic histogram per label set",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
			classicSeries: append(classicHistogramSeries("job", "api"), classicHistogramSeries("job", "web")...),
			expectedSeries: []string{
				`{__name__="http_requests", job="api"} @[1] @[2]`,
				`{__name__="http_requests", job="web"} @[1] @[2]`,
			},
		},
		{
			// This is the migration overlap: the same series is returned twice,
			// which PromQL rejects as a labelset collision if the samples
			// overlap in time. See TestNHCBCompatLayers in the promql package.
			name:          "native and classic histogram - both are returned",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
			classicSeries: classicHistogramSeries(),
			nativeSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests"), []chunks.Sample{hSample{t: 1, h: nhcb}}),
			},
			expectedSeries: []string{
				`{__name__="http_requests"} @[1]`,
				`{__name__="http_requests"} @[1] @[2]`,
			},
		},
		{
			name: "le matcher disables the conversion",
			queryMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"),
				labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "1"),
			},
			classicSeries:  classicHistogramSeries(),
			expectedSeries: nil,
		},
		{
			name:           "classic histogram query passes through",
			queryMatchers:  []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_bucket")},
			classicSeries:  classicHistogramSeries(),
			expectedSeries: nil, // The mock querier only answers base name queries with classic series.
		},
		{
			name:          "buckets returned in lexicographic le order are sorted",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 5}}),
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "1"), []chunks.Sample{fSample{t: 1, f: 2}}),
			},
			expectedSeries: []string{`{__name__="http_requests"} @[1]`},
		},
		{
			name:          "no data at all",
			queryMatchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := NewClassicAsNHCBQuerier(&classicMockQuerier{
				classicSeries: tc.classicSeries,
				nativeSeries:  tc.nativeSeries,
			})

			ss := q.Select(context.Background(), false, nil, tc.queryMatchers...)
			var got []string
			for ss.Next() {
				got = append(got, seriesSummary(t, ss.At()))
			}
			require.NoError(t, ss.Err())
			require.Equal(t, tc.expectedSeries, got)
		})
	}

	t.Run("converted samples", func(t *testing.T) {
		q := NewClassicAsNHCBQuerier(&classicMockQuerier{classicSeries: classicHistogramSeries()})
		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
		require.True(t, ss.Next())

		it := ss.At().Iterator(nil)
		require.Equal(t, chunkenc.ValHistogram, it.Next())
		ts, h := it.AtHistogram(nil)
		require.Equal(t, int64(1), ts)
		require.Equal(t, nhcb, h)

		require.Equal(t, chunkenc.ValHistogram, it.Next())
		ts, h = it.AtHistogram(nil)
		require.Equal(t, int64(2), ts)
		require.Equal(t, &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			Count:           7,
			Sum:             14,
			CustomValues:    []float64{1},
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
			PositiveBuckets: []int64{3, 1},
		}, h)

		require.Equal(t, chunkenc.ValNone, it.Next())
		require.False(t, ss.Next())
		require.NoError(t, ss.Err())
		require.Empty(t, ss.Warnings())
	})

	t.Run("float bucket counts produce a float histogram", func(t *testing.T) {
		q := NewClassicAsNHCBQuerier(&classicMockQuerier{classicSeries: []Series{
			NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "1"), []chunks.Sample{fSample{t: 1, f: 2.5}}),
			NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 5}}),
		}})
		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
		require.True(t, ss.Next())

		it := ss.At().Iterator(nil)
		require.Equal(t, chunkenc.ValFloatHistogram, it.Next())
		_, fh := it.AtFloatHistogram(nil)
		require.Equal(t, float64(5), fh.Count)
		require.Equal(t, []float64{2.5, 2.5}, fh.PositiveBuckets)
		require.NoError(t, ss.Err())
	})
}

func TestClassicAsNHCBQuerier_ConversionWarnings(t *testing.T) {
	tests := []struct {
		name            string
		classicSeries   []Series
		expectedSeries  int
		expectedWarning string
	}{
		{
			name: "non-cumulative buckets",
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "1"), []chunks.Sample{fSample{t: 1, f: 5}}),
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 2}}),
			},
			expectedWarning: "count is not cumulative",
		},
		{
			name: "count does not match the +Inf bucket",
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 5}}),
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_count"), []chunks.Sample{fSample{t: 1, f: 7}}),
			},
			expectedWarning: "count mismatch",
		},
		{
			name: "sum without buckets and without count",
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_sum"), []chunks.Sample{fSample{t: 1, f: 7}}),
			},
			expectedWarning: "count must be provided when no buckets are present",
		},
		{
			name: "malformed le label",
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "not-a-number"), []chunks.Sample{fSample{t: 1, f: 5}}),
			},
			expectedWarning: `malformed bucket label "not-a-number"`,
		},
		{
			// Only the broken timestamp is dropped, the rest of the series is
			// still returned.
			name: "partially broken series",
			classicSeries: []Series{
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_bucket", labels.BucketLabel, "+Inf"), []chunks.Sample{fSample{t: 1, f: 5}, fSample{t: 2, f: 7}}),
				NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests_count"), []chunks.Sample{fSample{t: 1, f: 5}, fSample{t: 2, f: 9}}),
			},
			expectedSeries:  1,
			expectedWarning: "count mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := NewClassicAsNHCBQuerier(&classicMockQuerier{classicSeries: tc.classicSeries})
			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
			var count int
			for ss.Next() {
				count++
			}
			require.NoError(t, ss.Err())
			require.Equal(t, tc.expectedSeries, count)

			warnings, _ := ss.Warnings().AsStrings("", 0, 0)
			require.Len(t, warnings, 1)
			require.Contains(t, warnings[0], "classic histogram could not be converted")
			require.Contains(t, warnings[0], tc.expectedWarning)
		})
	}
}

func TestClassicAsNHCBQuerier_Staleness(t *testing.T) {
	stale := math.Float64frombits(value.StaleNaN)
	// series returns a classic series with the given values at t=1, t=2, etc.
	series := func(name string, lbls []string, values ...float64) Series {
		samples := make([]chunks.Sample, 0, len(values))
		for i, v := range values {
			samples = append(samples, fSample{t: int64(i + 1), f: v})
		}
		return NewListSeries(labels.FromStrings(append([]string{model.MetricNameLabel, name}, lbls...)...), samples)
	}

	for _, tc := range []struct {
		name          string
		classicSeries []Series
		expected      []string
	}{
		{
			name: "all series stale",
			classicSeries: []Series{
				series("http_requests_bucket", []string{labels.BucketLabel, "1"}, 2, stale, 3),
				series("http_requests_bucket", []string{labels.BucketLabel, "+Inf"}, 5, stale, 7),
				series("http_requests_count", nil, 5, stale, 7),
				series("http_requests_sum", nil, 10, stale, 14),
			},
			expected: []string{
				`{__name__="http_requests"} {count:5, sum:10, [-Inf,1]:2, (1,+Inf]:3}@1 stale@2 {count:7, sum:14, [-Inf,1]:3, (1,+Inf]:4}@3`,
			},
		},
		{
			name: "some series stale",
			classicSeries: []Series{
				series("http_requests_bucket", []string{labels.BucketLabel, "1"}, 2, stale),
				series("http_requests_bucket", []string{labels.BucketLabel, "+Inf"}, 5, 7),
				series("http_requests_count", nil, 5, 7),
				series("http_requests_sum", nil, 10, 14),
			},
			expected: []string{
				`{__name__="http_requests"} {count:5, sum:10, [-Inf,1]:2, (1,+Inf]:3}@1 {count:7, sum:14, [-Inf,+Inf]:7}@2`,
			},
		},
		{
			name: "classic histograms go stale independently",
			classicSeries: []Series{
				series("http_requests_bucket", []string{labels.BucketLabel, "+Inf", "job", "a"}, 5, stale),
				series("http_requests_bucket", []string{labels.BucketLabel, "+Inf", "job", "b"}, 5, 7),
			},
			expected: []string{
				`{__name__="http_requests", job="a"} {count:5, sum:0, [-Inf,+Inf]:5}@1 stale@2`,
				`{__name__="http_requests", job="b"} {count:5, sum:0, [-Inf,+Inf]:5}@1 {count:7, sum:0, [-Inf,+Inf]:7}@2`,
			},
		},
		{
			name: "only stale markers",
			classicSeries: []Series{
				series("http_requests_bucket", []string{labels.BucketLabel, "+Inf"}, stale),
				series("http_requests_count", nil, stale),
			},
			expected: []string{`{__name__="http_requests"} stale@1`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := NewClassicAsNHCBQuerier(&classicMockQuerier{classicSeries: tc.classicSeries})
			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
			require.ElementsMatch(t, tc.expected, samplesSummary(t, ss))
			require.Empty(t, ss.Warnings())
		})
	}
}

func TestClassicAsNHCBQuerier_ErrorPropagation(t *testing.T) {
	testError := errors.New("storage error")

	tests := []struct {
		name    string
		querier Querier
	}{
		{
			name:    "native set immediate error",
			querier: &classicMockQuerier{nativeErr: testError},
		},
		{
			name:    "classic set immediate error",
			querier: &classicMockQuerier{classicErr: testError},
		},
		{
			name: "classic set error during iteration",
			querier: &classicSetQuerier{
				nativeSet:  NewMockSeriesSet(),
				classicSet: newDeferredErrSeriesSet(testError),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := NewClassicAsNHCBQuerier(tc.querier)
			ss := q.Select(context.Background(), false, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
			for ss.Next() {
			}
			require.ErrorIs(t, ss.Err(), testError)
		})
	}
}

func TestClassicAsNHCBQuerier_WarningPropagation(t *testing.T) {
	nativeWarning := annotations.New().Add(errors.New("native warning"))
	classicWarning := annotations.New().Add(errors.New("classic warning"))

	q := NewClassicAsNHCBQuerier(&classicSetQuerier{
		nativeSet: &mockSeriesSet{idx: -1, warnings: nativeWarning, series: []Series{
			NewListSeries(labels.FromStrings(model.MetricNameLabel, "http_requests"), []chunks.Sample{fSample{t: 1, f: 1}}),
		}},
		classicSet: &mockSeriesSet{idx: -1, warnings: classicWarning, series: classicHistogramSeries()},
	})

	ss := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests"))
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	expected := annotations.New()
	expected.Merge(nativeWarning)
	expected.Merge(classicWarning)
	require.Equal(t, *expected, ss.Warnings())
}

// seriesSummary renders a series as its labels followed by its sample
// timestamps, which keeps the expectations of table driven tests readable.
func seriesSummary(t *testing.T, s Series) string {
	t.Helper()

	var sb strings.Builder
	sb.WriteString(s.Labels().String())
	it := s.Iterator(nil)
	for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
		sb.WriteString(" @[")
		sb.WriteString(strconv.FormatInt(it.AtT(), 10))
		sb.WriteString("]")
	}
	require.NoError(t, it.Err())
	return sb.String()
}

// classicMockQuerier answers queries for the base metric name with
// nativeSeries, and queries carrying the classic suffix pattern with
// classicSeries.
type classicMockQuerier struct {
	classicSeries []Series
	nativeSeries  []Series

	classicErr error
	nativeErr  error
}

func (m *classicMockQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			continue
		}
		if strings.Contains(matcher.Value, classicSuffixesPattern) {
			if m.classicErr != nil {
				return ErrSeriesSet(m.classicErr)
			}
			return NewMockSeriesSet(m.classicSeries...)
		}
		if m.nativeErr != nil {
			return ErrSeriesSet(m.nativeErr)
		}
		return NewMockSeriesSet(m.nativeSeries...)
	}
	return NewMockSeriesSet()
}

func (*classicMockQuerier) LabelValues(context.Context, string, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*classicMockQuerier) LabelNames(context.Context, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*classicMockQuerier) Close() error { return nil }

// classicSetQuerier routes queries with the classic suffix pattern to
// classicSet and all other queries to nativeSet, so that tests can inject
// arbitrary SeriesSet implementations.
type classicSetQuerier struct {
	classicSet SeriesSet
	nativeSet  SeriesSet
}

func (m *classicSetQuerier) Select(_ context.Context, _ bool, _ *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			continue
		}
		if strings.Contains(matcher.Value, classicSuffixesPattern) {
			return m.classicSet
		}
		return m.nativeSet
	}
	return NewMockSeriesSet()
}

func (*classicSetQuerier) LabelValues(context.Context, string, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*classicSetQuerier) LabelNames(context.Context, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*classicSetQuerier) Close() error { return nil }
