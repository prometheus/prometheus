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

package histogramconv_test

import (
	"context"
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
	"github.com/prometheus/prometheus/storage/histogramconv"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestNHClassicCompatQuerier(t *testing.T) {
	// The same observations (1 in (0.5,1], 2 in (1,2] and 1 in (2,4]) are
	// stored as a classic histogram, an NHCB and an exponential native
	// histogram, each for a different job, e.g. a partially migrated metric.
	var (
		classic = []struct {
			lset labels.Labels
			v    float64
		}{
			{lset: labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds_bucket", "job", "classic", "le", "1"), v: 1},
			{lset: labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds_bucket", "job", "classic", "le", "2"), v: 3},
			{lset: labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds_bucket", "job", "classic", "le", "+Inf"), v: 4},
			{lset: labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds_count", "job", "classic"), v: 4},
			{lset: labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds_sum", "job", "classic"), v: 6},
		}
		nhcb = &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			Count:           4,
			Sum:             6,
			CustomValues:    []float64{1, 2},
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
			PositiveBuckets: []int64{1, 1, -1},
		}
		exponential = &histogram.Histogram{
			Schema:          0,
			Count:           4,
			Sum:             6,
			ZeroThreshold:   0.001,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
			PositiveBuckets: []int64{1, 1, -1},
		}
		nhcbLabels        = labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds", "job", "nhcb")
		exponentialLabels = labels.FromStrings(model.MetricNameLabel, "rpc_latency_seconds", "job", "exponential")
		stale             = math.Float64frombits(value.StaleNaN)
	)
	st := teststorage.New(t)
	app := st.Appender(context.Background())
	for _, ts := range []int64{0, 60_000} {
		for _, s := range classic {
			_, err := app.Append(0, s.lset, ts, s.v)
			require.NoError(t, err)
		}
		_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "up", "job", "classic"), ts, 1)
		require.NoError(t, err)
		_, err = app.AppendHistogram(0, nhcbLabels, ts, nhcb, nil)
		require.NoError(t, err)
		_, err = app.AppendHistogram(0, exponentialLabels, ts, exponential, nil)
		require.NoError(t, err)
	}
	// All histograms go stale, e.g. because their targets went away.
	for _, s := range classic {
		_, err := app.Append(0, s.lset, 120_000, stale)
		require.NoError(t, err)
	}
	_, err := app.AppendHistogram(0, nhcbLabels, 120_000, &histogram.Histogram{Sum: stale}, nil)
	require.NoError(t, err)
	_, err = app.AppendHistogram(0, exponentialLabels, 120_000, &histogram.Histogram{Sum: stale}, nil)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	for _, tc := range []struct {
		name       string
		newQuerier func(storage.Querier) storage.Querier
		matchers   []*labels.Matcher
		// expected holds every returned series, see selectSummary. Duplicates
		// are not allowed, which would e.g. show up if the classic series were
		// converted to NHCB and back.
		expected []string
	}{
		{
			name:       "classic buckets, stored and converted from both NHCB and exponential histograms",
			newQuerier: histogramconv.NewNHClassicCompatQuerier,
			matchers:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_latency_seconds_bucket")},
			expected: []string{
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="1"} 1@0 1@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="2"} 3@0 3@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="+Inf"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="1.0"} 1@0 1@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="2.0"} 3@0 3@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="+Inf"} 4@0 4@60000 stale@120000`,
				// The lower boundary of the lowest bucket is emitted too.
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="0.5"} 0@0 0@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="1.0"} 1@0 1@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="2.0"} 3@0 3@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="4.0"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="+Inf"} 4@0 4@60000 stale@120000`,
			},
		},
		{
			name:       "classic buckets filtered by le",
			newQuerier: histogramconv.NewNHClassicCompatQuerier,
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_latency_seconds_bucket"),
				labels.MustNewMatcher(labels.MatchEqual, labels.BucketLabel, "+Inf"),
			},
			expected: []string{
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="+Inf"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="+Inf"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="exponential", le="+Inf"} 4@0 4@60000 stale@120000`,
			},
		},
		{
			name:       "classic count",
			newQuerier: histogramconv.NewNHClassicCompatQuerier,
			matchers:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_latency_seconds_count")},
			expected: []string{
				`{__name__="rpc_latency_seconds_count", job="classic"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_count", job="nhcb"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_count", job="exponential"} 4@0 4@60000 stale@120000`,
			},
		},
		{
			name:       "native histograms, stored and converted from the classic histogram",
			newQuerier: histogramconv.NewNHClassicCompatQuerier,
			matchers:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_latency_seconds")},
			expected: []string{
				`{__name__="rpc_latency_seconds", job="classic"} {count:4, sum:6, [-Inf,1]:1, (1,2]:2, (2,+Inf]:1}@0 {count:4, sum:6, [-Inf,1]:1, (1,2]:2, (2,+Inf]:1}@60000 stale@120000`,
				`{__name__="rpc_latency_seconds", job="nhcb"} {count:4, sum:6, [-Inf,1]:1, (1,2]:2, (2,+Inf]:1}@0 {count:4, sum:6, [-Inf,1]:1, (1,2]:2, (2,+Inf]:1}@60000 stale@120000`,
				`{__name__="rpc_latency_seconds", job="exponential"} {count:4, sum:6, (0.5,1]:1, (1,2]:2, (2,4]:1}@0 {count:4, sum:6, (0.5,1]:1, (1,2]:2, (2,4]:1}@60000 stale@120000`,
			},
		},
		{
			name:       "other metrics are passed through",
			newQuerier: histogramconv.NewNHClassicCompatQuerier,
			matchers:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up")},
			expected:   []string{`{__name__="up", job="classic"} 1@0 1@60000`},
		},
		{
			name:       "promql-nhcb-as-classic does not convert exponential histograms",
			newQuerier: histogramconv.NewNHCBAsClassicQuerier,
			matchers:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc_latency_seconds_bucket")},
			expected: []string{
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="1"} 1@0 1@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="2"} 3@0 3@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="classic", le="+Inf"} 4@0 4@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="1.0"} 1@0 1@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="2.0"} 3@0 3@60000 stale@120000`,
				`{__name__="rpc_latency_seconds_bucket", job="nhcb", le="+Inf"} 4@0 4@60000 stale@120000`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := st.Querier(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, q.Close()) })

			ss := tc.newQuerier(q).Select(context.Background(), false, nil, tc.matchers...)
			require.ElementsMatch(t, tc.expected, selectSummary(t, ss))
			require.Empty(t, ss.Warnings())
		})
	}
}

// selectSummary renders every series of ss as its labels followed by its
// samples as value@timestamp, with histograms in their String() format and
// staleness markers as stale@timestamp.
func selectSummary(t *testing.T, ss storage.SeriesSet) []string {
	t.Helper()

	var got []string
	for ss.Next() {
		var sb strings.Builder
		sb.WriteString(ss.At().Labels().String())
		it := ss.At().Iterator(nil)
		for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
			var (
				ts int64
				v  string
			)
			if valType == chunkenc.ValFloat {
				var f float64
				ts, f = it.At()
				v = strconv.FormatFloat(f, 'g', -1, 64)
				if value.IsStaleNaN(f) {
					v = "stale"
				}
			} else {
				var fh *histogram.FloatHistogram
				ts, fh = it.AtFloatHistogram(nil)
				v = fh.String()
				if value.IsStaleNaN(fh.Sum) {
					v = "stale"
				}
			}
			fmt.Fprintf(&sb, " %s@%d", v, ts)
		}
		require.NoError(t, it.Err())
		got = append(got, sb.String())
	}
	require.NoError(t, ss.Err())
	return got
}
