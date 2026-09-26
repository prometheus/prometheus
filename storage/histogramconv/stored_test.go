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
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestQuerier_Stored(t *testing.T) {
	var (
		stale = math.Float64frombits(value.StaleNaN)
		// The String() format of nhcb is {count:1, sum:1, [-Inf,1]:1}, the one
		// of nhe {count:1, sum:1, (0.5,1]:1}.
		nhcb = &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			Count:           1,
			Sum:             1,
			CustomValues:    []float64{1},
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []int64{1},
		}
		nhe = &histogram.Histogram{
			Count:           1,
			Sum:             1,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []int64{1},
		}
		// This is how the TSDB returns a staleness marker of a histogram
		// series, note the exponential schema 0.
		staleHistogram = &histogram.Histogram{Sum: stale}

		foo  = labels.FromStrings(model.MetricNameLabel, "foo")
		name = func(n string) *labels.Matcher {
			return labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, n)
		}
		convert = func(v string) *labels.Matcher {
			return labels.MustNewMatcher(labels.MatchRegexp, ConvertStoredAsLabel, v)
		}
		debug = labels.MustNewMatcher(labels.MatchEqual, DebugStoredAsLabel, "true")
	)

	for _, tc := range []struct {
		name        string
		convertFrom []Representation
		series      []storage.Series
		matchers    []*labels.Matcher
		expected    []string
		err         error
	}{
		{
			name: "without control matchers, stored series are returned unchanged",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhe},
			})},
			matchers: []*labels.Matcher{name("foo")},
			expected: []string{`{__name__="foo"} {count:1, sum:1, [-Inf,1]:1}@1 {count:1, sum:1, (0.5,1]:1}@2`},
		},
		{
			name: "debug splits a series by representation",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhcb}, hSample{t: 3, h: nhe}, hSample{t: 4, h: nhe},
			})},
			matchers: []*labels.Matcher{name("foo"), debug},
			expected: []string{
				`{__name__="foo", __stored_as__="nhcb"} {count:1, sum:1, [-Inf,1]:1}@1 {count:1, sum:1, [-Inf,1]:1}@2 stale@3`,
				`{__name__="foo", __stored_as__="nhe"} {count:1, sum:1, (0.5,1]:1}@3 {count:1, sum:1, (0.5,1]:1}@4`,
			},
		},
		{
			name: "samples of other representations are dropped",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhcb}, hSample{t: 3, h: nhe}, hSample{t: 4, h: nhe},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhcb")},
			expected: []string{`{__name__="foo"} {count:1, sum:1, [-Inf,1]:1}@1 {count:1, sum:1, [-Inf,1]:1}@2 stale@3`},
		},
		{
			name: "several representations are returned as one series",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhe},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhcb|nhe")},
			expected: []string{`{__name__="foo"} {count:1, sum:1, [-Inf,1]:1}@1 {count:1, sum:1, (0.5,1]:1}@2`},
		},
		{
			name: "alternating representations",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhe}, hSample{t: 3, h: nhcb}, hSample{t: 4, h: nhe},
			})},
			matchers: []*labels.Matcher{name("foo"), debug},
			expected: []string{
				`{__name__="foo", __stored_as__="nhcb"} {count:1, sum:1, [-Inf,1]:1}@1 stale@2 {count:1, sum:1, [-Inf,1]:1}@3 stale@4`,
				`{__name__="foo", __stored_as__="nhe"} {count:1, sum:1, (0.5,1]:1}@2 stale@3 {count:1, sum:1, (0.5,1]:1}@4`,
			},
		},
		{
			// Leading and consecutive staleness markers, and the ones of
			// dropped samples, are dropped.
			name: "staleness markers",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: staleHistogram},
				hSample{t: 2, h: nhcb},
				hSample{t: 3, h: staleHistogram},
				hSample{t: 4, h: staleHistogram},
				hSample{t: 5, h: nhe},
				hSample{t: 6, h: staleHistogram},
				hSample{t: 7, h: nhcb},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhcb")},
			expected: []string{`{__name__="foo"} {count:1, sum:1, [-Inf,1]:1}@2 stale@3 {count:1, sum:1, [-Inf,1]:1}@7`},
		},
		{
			name: "staleness markers in debug mode",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: staleHistogram},
				hSample{t: 2, h: nhcb},
				hSample{t: 3, h: staleHistogram},
				hSample{t: 4, h: staleHistogram},
				hSample{t: 5, h: nhe},
				hSample{t: 6, h: staleHistogram},
				hSample{t: 7, h: nhcb},
			})},
			matchers: []*labels.Matcher{name("foo"), debug},
			expected: []string{
				`{__name__="foo", __stored_as__="nhcb"} {count:1, sum:1, [-Inf,1]:1}@2 stale@3 {count:1, sum:1, [-Inf,1]:1}@7`,
				`{__name__="foo", __stored_as__="nhe"} {count:1, sum:1, (0.5,1]:1}@5 stale@6`,
			},
		},
		{
			name: "float samples are classic",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				fSample{t: 1, f: 1}, hSample{t: 2, h: nhcb}, fSample{t: 3, f: 1},
			})},
			matchers: []*labels.Matcher{name("foo"), debug},
			expected: []string{
				`{__name__="foo", __stored_as__="classic"} 1@1 stale@2 1@3`,
				`{__name__="foo", __stored_as__="nhcb"} {count:1, sum:1, [-Inf,1]:1}@2 stale@3`,
			},
		},
		{
			name: "float histograms",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				fhSample{t: 1, fh: nhcb.ToFloat(nil)}, fhSample{t: 2, fh: nhe.ToFloat(nil)}, fhSample{t: 3, fh: staleHistogram.ToFloat(nil)},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhe")},
			expected: []string{`{__name__="foo"} {count:1, sum:1, (0.5,1]:1}@2 stale@3`},
		},
		{
			name: "series with dropped samples only are not returned",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhe}, hSample{t: 2, h: staleHistogram},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhcb")},
		},
		{
			name: "start timestamps are kept",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				fSample{st: 1, t: 2, f: 1}, hSample{st: 1, t: 3, h: nhcb}, fhSample{st: 1, t: 4, fh: nhe.ToFloat(nil)},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nhcb|nhe|classic")},
			expected: []string{`{__name__="foo"} 1@2(st=1) {count:1, sum:1, [-Inf,1]:1}@3(st=1) {count:1, sum:1, (0.5,1]:1}@4(st=1)`},
		},
		{
			name: "series converted to classic histograms in debug mode",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb}, hSample{t: 2, h: nhe},
			})},
			convertFrom: []Representation{NHCB, NHE},
			matchers:    []*labels.Matcher{name("foo_count"), debug},
			expected: []string{
				`{__name__="foo_count", __stored_as__="nhcb"} 1@1 stale@2`,
				`{__name__="foo_count", __stored_as__="nhe"} 1@2`,
			},
		},
		{
			name: "series converted to NHCB in debug mode",
			series: []storage.Series{
				storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "foo_bucket", "le", "1"), []chunks.Sample{fSample{t: 1, f: 1}}),
				storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "foo_bucket", "le", "+Inf"), []chunks.Sample{fSample{t: 1, f: 1}}),
				storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "foo_sum"), []chunks.Sample{fSample{t: 1, f: 1}}),
			},
			matchers: []*labels.Matcher{name("foo"), convert("classic"), debug},
			expected: []string{`{__name__="foo", __stored_as__="classic"} {count:1, sum:1, [-Inf,1]:1}@1`},
		},
		{
			name: "matchers matching no representation select nothing",
			series: []storage.Series{storage.NewListSeries(foo, []chunks.Sample{
				hSample{t: 1, h: nhcb},
			})},
			matchers: []*labels.Matcher{name("foo"), convert("nh")},
		},
		{
			name:     "only control matchers",
			matchers: []*labels.Matcher{convert("nhcb"), labels.MustNewMatcher(labels.MatchRegexp, "job", ".*")},
			err:      errOnlyControlMatchers,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := NewQuerier(storedQuerier(tc.series...), tc.convertFrom)
			ss := q.Select(context.Background(), false, nil, tc.matchers...)
			if tc.err != nil {
				require.False(t, ss.Next())
				require.ErrorIs(t, ss.Err(), tc.err)
				return
			}
			require.ElementsMatch(t, tc.expected, samplesSummary(t, ss))
		})
	}
}

// storedQuerier returns a querier that selects from the given series, like a
// storage.
func storedQuerier(series ...storage.Series) storage.Querier {
	return &storage.MockQuerier{SelectMockFunction: func(_ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
		var selected []storage.Series
	Series:
		for _, s := range series {
			for _, m := range matchers {
				if !m.Matches(s.Labels().Get(m.Name)) {
					continue Series
				}
			}
			selected = append(selected, s)
		}
		return newMockSeriesSet(selected...)
	}}
}
