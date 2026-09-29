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

package promql_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// queryableOnly wraps a storage.Queryable so it does NOT satisfy
// storage.ChunkQueryable, exercising EstimateCost's plain-Queryable fallback.
type queryableOnly struct{ q storage.Queryable }

func (o queryableOnly) Querier(mint, maxt int64) (storage.Querier, error) {
	return o.q.Querier(mint, maxt)
}

// estimateTestParser is the parser the cost-estimation tests pass to
// promql.EstimateCost. It mirrors the default parser the API normally supplies.
var estimateTestParser = parser.NewParser(parser.Options{})

func TestEstimateCostInstantVectorSelector(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  http_requests_total{job="a",instance="i1"} 0+1x5
  http_requests_total{job="a",instance="i2"} 0+1x5
  http_requests_total{job="b",instance="i1"} 0+1x5
  node_cpu{cpu="0"}                          0+2x5
  node_cpu{cpu="1"}                          0+2x5
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(50, 0)

	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `http_requests_total`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	// Three http_requests_total series match.
	require.Equal(t, int64(3), est.SeriesTouched)
	// Samples are positive (at least one per series).
	require.GreaterOrEqual(t, est.SamplesRead, est.SeriesTouched)
}

func TestEstimateCostRangeSelectorScalesWithWindow(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x100
  metric{a="2"} 0+1x100
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)
	scrape := 10 * time.Second

	// A 5m range window at a 10s scrape interval covers ~30 intervals per
	// series. With 2 matching series we expect roughly 2*30 = 60 samples. We
	// allow a one-interval tolerance per series for inclusive/exclusive window
	// boundaries.
	est5m, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[5m])`, ts, ts, 0, 5*time.Minute, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(2), est5m.SeriesTouched)
	require.InDelta(t, int64(2*(5*60/10)), est5m.SamplesRead, 2)

	// A wider window scans strictly more samples for the same series, and scales
	// with the window: roughly twice as many samples for a 10m window.
	est10m, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[10m])`, ts, ts, 0, 5*time.Minute, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(2), est10m.SeriesTouched)
	require.Greater(t, est10m.SamplesRead, est5m.SamplesRead)
	require.InDelta(t, int64(2*(10*60/10)), est10m.SamplesRead, 2)
}

// TestEstimateCostRangeQueryMatchesActual verifies that the incremental sample
// model (M1) produces an estimate close to what the engine actually scans. It
// executes the same queries through a real engine and compares the estimate's
// SamplesRead against the executed query's actual SamplesRead.
//
// The engine reads a range selector's full window only at the first step and
// then only the new points past the previous step's cutoff, so the estimate
// models samplesPerWindow(range) + (numSteps-1)*samplesPerWindow(step) rather
// than re-reading the whole window at every step.
func TestEstimateCostRangeQueryMatchesActual(t *testing.T) {
	// Load enough samples that every step's window is fully covered, so the
	// comparison is not skewed by the engine reading fewer samples near the end
	// of the data (a documented limitation of the estimator).
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x2000
  metric{a="2"} 0+1x2000
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	scrape := 10 * time.Second
	lookback := 5 * time.Minute

	// Use the default lookback so the estimator and the engine agree on instant
	// selector windows.
	engine := promqltest.NewTestEngine(t, true, lookback, promqltest.DefaultMaxSamplesPerQuery)

	// actualSamplesRead executes the query and returns the engine's real
	// SamplesRead, the I/O figure the estimator targets.
	actualSamplesRead := func(t *testing.T, qs string, start, end time.Time, step time.Duration) int64 {
		t.Helper()
		opts := promql.NewPrometheusQueryOpts(true, lookback, nil)
		var (
			qry promql.Query
			err error
		)
		if step == 0 {
			qry, err = engine.NewInstantQuery(ctx, store, opts, qs, start)
		} else {
			qry, err = engine.NewRangeQuery(ctx, store, opts, qs, start, end, step)
		}
		require.NoError(t, err)
		res := qry.Exec(ctx)
		require.NoError(t, res.Err)
		return qry.Stats().Samples.SamplesRead
	}

	cases := []struct {
		name       string
		query      string
		start, end time.Time
		step       time.Duration
		delta      float64
	}{
		{
			name:  "range-selector instant",
			query: `rate(metric[5m])`,
			start: time.Unix(5000, 0), end: time.Unix(5000, 0), step: 0,
			// A single inclusive-boundary sample per series of slack.
			delta: 4,
		},
		{
			name:  "range-selector range query",
			query: `rate(metric[5m])`,
			start: time.Unix(4000, 0), end: time.Unix(4000+3600, 0), step: time.Minute,
			// The full first window over-counts by one inclusive-boundary sample
			// per series; otherwise the incremental model matches the engine.
			delta: 4,
		},
		{
			// A step wider than the range: consecutive windows do not overlap, so
			// the engine re-reads a whole window at every step instead of only the
			// advanced samples. Two steps over a 5m range at a 10s scrape interval
			// is 2*(300s/10s) = 60 samples per series, 120 for the two series; the
			// estimate adds one inclusive-boundary sample per window per series.
			name:  "step wider than range",
			query: `rate(metric[5m])`,
			start: time.Unix(4000, 0), end: time.Unix(4000+3600, 0), step: time.Hour,
			delta: 4,
		},
		{
			name:  "step shorter than scrape interval",
			query: `rate(metric[5m])`,
			start: time.Unix(4000, 0), end: time.Unix(7600, 0), step: time.Second,
			delta: 4,
		},
		{
			name:  "fractional scrapes per step",
			query: `rate(metric[5m])`,
			start: time.Unix(4000, 0), end: time.Unix(7600, 0), step: 15 * time.Second,
			delta: 4,
		},
		{
			name:  "instant selector range query",
			query: `metric`,
			start: time.Unix(4000, 0), end: time.Unix(4000+3600, 0), step: time.Minute,
			delta: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, c.query, c.start, c.end, c.step, lookback, time.Minute, scrape)
			require.NoError(t, err)
			actual := actualSamplesRead(t, c.query, c.start, c.end, c.step)
			require.InDelta(t, actual, est.SamplesRead, c.delta,
				"estimate %d vs actual %d", est.SamplesRead, actual)
		})
	}
}

// TestEstimateCostSubqueryMatchesActual verifies the subquery handling (M3): a
// selector inside a subquery is evaluated on the subquery's own, finer step grid
// spanning the query range plus the subquery range, so its sample cost is much
// larger than the outer step count alone would imply. The estimate is compared
// against an executed subquery's actual SamplesRead.
//
// The estimator assumes a 1m default subquery resolution, matching the engine's
// NoStepSubqueryIntervalFn here; the query uses an explicit 1m step to make the
// resolution unambiguous.
func TestEstimateCostSubqueryMatchesActual(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x2000
  metric{a="2"} 0+1x2000
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	scrape := 10 * time.Second
	lookback := 5 * time.Minute

	engine := promqltest.NewTestEngine(t, true, lookback, promqltest.DefaultMaxSamplesPerQuery)

	const query = `sum_over_time(rate(metric[5m])[1h:1m])`
	start := time.Unix(4000, 0)
	end := time.Unix(4000+1800, 0)
	step := time.Minute

	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, query, start, end, step, lookback, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)

	opts := promql.NewPrometheusQueryOpts(true, lookback, nil)
	qry, err := engine.NewRangeQuery(ctx, store, opts, query, start, end, step)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	actual := qry.Stats().Samples.SamplesRead

	// The inner rate[5m] selector is read on the subquery grid: span
	// (1800s + 3600s) at a 1m step is 91 inner steps. Per series the estimate is
	// samplesPerWindow(5m) + 90*floor(1m/10s) = 31 + 540 = 571, vs the engine's
	// 564 (one inclusive-boundary sample plus a boundary step of slack). Allow a
	// few samples per series.
	require.InDelta(t, actual, est.SamplesRead, 16,
		"subquery estimate %d vs actual %d", est.SamplesRead, actual)

	// The subquery estimate must dwarf the same selector evaluated as a plain
	// range query over the outer steps only, proving the subquery grid is folded
	// into numSteps rather than ignored.
	plain, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[5m])`, start, end, step, lookback, time.Minute, scrape)
	require.NoError(t, err)
	require.Greater(t, est.SamplesRead, plain.SamplesRead)
}

// TestEstimateCostSaturatesSamplesRead verifies that an extreme window and
// step count never wrap SamplesRead to a negative value (M4). The incremental
// model is bounded by the wall-clock span divided by the scrape interval, so it
// stays well below math.MaxInt64 for realistic Go durations; the saturating
// arithmetic remains a defensive guard against negative overflow.
func TestEstimateCostSaturatesSamplesRead(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x10
  metric{a="2"} 0+1x10
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	// A very wide range selector at a 1ms scrape interval over a very long
	// [start,end] with a tiny step gives a huge sample estimate. The result must
	// stay positive (never wrap negative) and never exceed the int64 ceiling.
	start := time.Unix(0, 0)
	end := time.Unix(100000*86400, 0) // 100000 days.
	step := time.Millisecond
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[100000d])`, start, end, step, 5*time.Minute, time.Minute, time.Millisecond)
	require.NoError(t, err)
	require.Positive(t, est.SamplesRead)
	require.LessOrEqual(t, est.SamplesRead, int64(math.MaxInt64))
}

func TestEstimateCostOffsetAndAtModifier(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x100
  metric{a="2"} 0+1x100
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)

	for _, q := range []string{
		`metric offset 5m`,
		`metric @ 300`,
		`rate(metric[5m] offset 2m @ 400)`,
	} {
		est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, q, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
		require.NoErrorf(t, err, "query %q should not error", q)
		require.Equalf(t, int64(2), est.SeriesTouched, "query %q should count both series", q)
	}
}

func TestEstimateCostMultiSelectorSumsSeries(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  a{x="1"} 0+1x10
  a{x="2"} 0+1x10
  a{x="3"} 0+1x10
  b{y="1"} 0+1x10
  b{y="2"} 0+1x10
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(50, 0)

	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `a + b`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	// SeriesTouched is the per-selector sum: 3 (a) + 2 (b) = 5.
	require.Equal(t, int64(5), est.SeriesTouched)

	// a + a double-counts the same selector's series, by documented design.
	estDup, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `a + a`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(6), estDup.SeriesTouched)
}

func TestEstimateCostSamplesPerSeriesFallback(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x10
  metric{a="2"} 0+1x10
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(50, 0)

	// A non-positive scrape interval degrades SamplesRead to one per series,
	// i.e. it equals SeriesTouched. The store is wrapped so it exposes only a
	// plain Queryable: with no chunk metadata the estimator cannot measure the
	// effective interval from chunk sample counts and must fall back to the
	// supplied (here non-positive) scrape interval.
	est, _, err := promql.EstimateCost(ctx, queryableOnly{store}, estimateTestParser, `rate(metric[5m])`, ts, ts, 0, 5*time.Minute, time.Minute, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)
	require.Equal(t, est.SeriesTouched, est.SamplesRead)
}

// TestEstimateCostSparseSeries compares sparse estimates with actual engine reads.
func TestEstimateCostSparseSeries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		series int
		gaps   bool
		blocks int
	}{
		{"single series", 1, false, 0},
		{"more series than the sampling budget", 100, false, 0},
		{"gaps across half the population", 100, true, 1},
		{"gaps across multiple blocks", 100, true, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input strings.Builder
			input.WriteString("load 15s\n")
			for i := range tc.series {
				fmt.Fprintf(&input, "metric{instance=\"%03d\"}", i)
				for seconds := 0; seconds < 14400; seconds += 15 {
					if seconds%300 != 0 || (tc.gaps && i%2 == 0 && seconds >= 7200 && seconds < 10800) {
						input.WriteString(" _")
					} else {
						fmt.Fprintf(&input, " %d", seconds)
					}
				}
				input.WriteByte('\n')
			}
			s := promqltest.LoadedStorage(t, input.String())
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			// Persist heterogeneous series in label order so the sampled mix is
			// reproducible rather than dependent on head series iteration order.
			for block := range tc.blocks {
				width := int64(14400000 / tc.blocks)
				mint := int64(block) * width
				require.NoError(t, s.CompactHead(tsdb.NewRangeHead(s.Head(), mint, mint+width-1)))
			}
			engine := newQueryCostEngine(t)
			start, end := time.Unix(7200, 0), time.Unix(14385, 0)
			const expr = "sum_over_time(metric[10m])"
			query, err := engine.NewRangeQuery(context.Background(), s, nil, expr, start, end, time.Minute)
			require.NoError(t, err)
			defer query.Close()
			require.NoError(t, query.Exec(context.Background()).Err)
			est, _, err := promql.EstimateCost(context.Background(), s, estimateTestParser, expr, start, end, time.Minute, 5*time.Minute, time.Minute, 15*time.Second)
			require.NoError(t, err)
			actual := query.Stats().Samples.SamplesRead
			require.Positive(t, actual)
			require.InDelta(t, actual, est.SamplesRead, float64(actual)*0.05)
		})
	}
}

// TestEstimateCostPartialDensitySample exercises a budget ending within a series.
func TestEstimateCostPartialDensitySample(t *testing.T) {
	for _, tc := range []struct {
		name   string
		series int
		points int
	}{
		{"no complete series", 1, 12000},
		{"one complete and one partial series", 2, 4000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input strings.Builder
			input.WriteString("load 10s\n")
			for i := range tc.series {
				fmt.Fprintf(&input, "metric{instance=\"%d\"} 0+1x%d\n", i, tc.points)
			}
			s := promqltest.LoadedStorage(t, input.String())
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			counter := &chunkMetaCountingQueryable{q: s, cq: s}
			ts := time.Unix(int64(tc.points*10), 0)
			expr := fmt.Sprintf("metric[%ds]", tc.points*10)
			est, _, err := promql.EstimateCost(context.Background(), counter, estimateTestParser, expr, ts, ts, 0, 5*time.Minute, time.Minute, time.Millisecond)
			require.NoError(t, err)
			require.Len(t, counter.metas, 1)
			require.Equal(t, 50, *counter.metas[0])
			query, err := newQueryCostEngine(t).NewInstantQuery(context.Background(), s, nil, expr, ts)
			require.NoError(t, err)
			defer query.Close()
			require.NoError(t, query.Exec(context.Background()).Err)
			// A partial series must not depress the average count of completed
			// series. With none completed, measured gaps supply the interval.
			require.InDelta(t, query.Stats().Samples.SamplesRead, est.SamplesRead, float64(tc.series))
		})
	}
}

// TestEstimateCostMeasuresIntervalFromChunks verifies that when the storage
// exposes chunk metadata the estimator measures the effective sample interval
// from a bounded sample of chunk sample counts, so SamplesRead reflects the
// real data density even when the caller passes no scrape interval. This is the
// counterpart to TestEstimateCostSamplesPerSeriesFallback: there the plain
// Queryable forces the degrade-to-one-per-series fallback, here the chunk
// sampling recovers the true density.
func TestEstimateCostMeasuresIntervalFromChunks(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x100
  metric{a="2"} 0+1x100
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)

	// Pass scrapeInterval=0: without chunk sampling this would collapse to one
	// sample per series (SamplesRead == SeriesTouched). Because the store
	// exposes chunk metadata the estimator measures the ~10s interval from the
	// chunks and sizes the 5m window at roughly 30 samples per series.
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[5m])`, ts, ts, 0, 5*time.Minute, time.Minute, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)
	// A 5m window at the measured 10s interval is ~31 samples per series; allow a
	// one-interval boundary tolerance per series.
	require.InDelta(t, int64(2*(5*60/10)), est.SamplesRead, 2,
		"measured-interval estimate %d", est.SamplesRead)
}

// TestEstimateCostSamplesFromRealWindowExact verifies that observed sample counts
// constrain the estimate when data covers only a small part of the query window.
func TestEstimateCostSamplesFromRealWindowExact(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x5
  metric{a="2"} 0+1x5
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)

	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `metric[10m]`, ts, ts, 0, 5*time.Minute, time.Minute, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)

	// The left boundary is excluded, so each series contributes five samples
	// at 10,...,50s. Do not extrapolate the 10s interval across the empty tail.
	require.Equal(t, int64(10), est.SamplesRead)
	query, err := newQueryCostEngine(t).NewInstantQuery(ctx, store, nil, `metric[10m]`, ts)
	require.NoError(t, err)
	defer query.Close()
	require.NoError(t, query.Exec(ctx).Err)
	require.Equal(t, query.Stats().Samples.SamplesRead, est.SamplesRead)
}

// TestEstimateCostDefaultsLookbackDelta verifies that a zero lookback delta is
// defaulted internally to the package default (5m), so an instant selector
// builds a sane selection window and counts its series rather than collapsing to
// a degenerate (~1ms) window that would miss them (H3a). The lookback delta
// governs which sample is selected at each step, not how many samples are read,
// so the instant per-series estimate is one sample per step regardless.
func TestEstimateCostDefaultsLookbackDelta(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x100
  metric{a="2"} 0+1x100
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)

	// With lookbackDelta=0 the estimator applies the 5m default. The window stays
	// wide enough that both series are still counted; the instant estimate is one
	// sample per series (a pure instant query has a single step).
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `metric`, ts, ts, 0, 0, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)
	require.Equal(t, est.SeriesTouched, est.SamplesRead)
}

// TestEstimateCostHistogramMatchesActual verifies that the native-histogram
// aware estimator (approach B sampling) sizes histogram points by their
// per-bucket cost, so SamplesRead is close to the engine's real SamplesRead
// and much larger than the same estimate would be treating each histogram point
// as a single float unit.
func TestEstimateCostHistogramMatchesActual(t *testing.T) {
	// A schema-0 histogram with three buckets sizes to several sample-units per
	// point, so the estimate must scale well above one unit per point.
	store := promqltest.LoadedStorage(t, `
load 10s
  nh{a="1"} {{schema:0 sum:5 count:4 buckets:[1 2 1]}}+{{schema:0 sum:5 count:4 buckets:[1 2 1]}}x2000
  nh{a="2"} {{schema:0 sum:5 count:4 buckets:[1 2 1]}}+{{schema:0 sum:5 count:4 buckets:[1 2 1]}}x2000
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	scrape := 10 * time.Second
	lookback := 5 * time.Minute

	engine := promqltest.NewTestEngine(t, true, lookback, promqltest.DefaultMaxSamplesPerQuery)

	actualSamplesRead := func(t *testing.T, qs string, start, end time.Time, step time.Duration) int64 {
		t.Helper()
		opts := promql.NewPrometheusQueryOpts(true, lookback, nil)
		var (
			qry promql.Query
			err error
		)
		if step == 0 {
			qry, err = engine.NewInstantQuery(ctx, store, opts, qs, start)
		} else {
			qry, err = engine.NewRangeQuery(ctx, store, opts, qs, start, end, step)
		}
		require.NoError(t, err)
		res := qry.Exec(ctx)
		require.NoError(t, res.Err)
		return qry.Stats().Samples.SamplesRead
	}

	cases := []struct {
		name       string
		query      string
		start, end time.Time
		step       time.Duration
		delta      float64
	}{
		{
			name:  "histogram range-selector instant",
			query: `rate(nh[5m])`,
			start: time.Unix(5000, 0), end: time.Unix(5000, 0), step: 0,
			// The full first window over-counts by one inclusive-boundary point per
			// series, but each point now costs its per-bucket sample units (13 for
			// this schema-0, three-bucket histogram), so the slack is 2 series * 13.
			delta: 28,
		},
		{
			name:  "histogram range query",
			query: `rate(nh[5m])`,
			start: time.Unix(4000, 0), end: time.Unix(4000+1800, 0), step: time.Minute,
			delta: 28,
		},
		{
			name:  "histogram count without buckets",
			query: `histogram_count(nh)`,
			start: time.Unix(5000, 0), end: time.Unix(5000, 0),
			delta: 0,
		},
		{
			name:  "histogram sum of a range function without buckets",
			query: `histogram_sum(rate(nh[5m]))`,
			start: time.Unix(4000, 0), end: time.Unix(5800, 0), step: time.Minute,
			delta: 24,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, c.query, c.start, c.end, c.step, lookback, time.Minute, scrape)
			require.NoError(t, err)
			actual := actualSamplesRead(t, c.query, c.start, c.end, c.step)
			require.InDelta(t, actual, est.SamplesRead, c.delta,
				"estimate %d vs actual %d", est.SamplesRead, actual)

			// Prove the multiplier works: the same query over a float series of
			// the identical layout (one unit per point) scans far fewer samples,
			// so the histogram estimate must be strictly and substantially larger.
			require.Greater(t, est.SamplesRead, actual/2,
				"histogram estimate %d should reflect per-bucket cost", est.SamplesRead)
		})
	}
}

// TestEstimateCostFloatStillAccurate verifies that adding the per-point cost
// sampling does not regress float-only queries: the estimate still matches the
// engine's real SamplesRead.
func TestEstimateCostFloatStillAccurate(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x2000
  metric{a="2"} 0+1x2000
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	scrape := 10 * time.Second
	lookback := 5 * time.Minute

	engine := promqltest.NewTestEngine(t, true, lookback, promqltest.DefaultMaxSamplesPerQuery)

	const query = `rate(metric[5m])`
	start := time.Unix(4000, 0)
	end := time.Unix(4000+3600, 0)
	step := time.Minute

	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, query, start, end, step, lookback, time.Minute, scrape)
	require.NoError(t, err)

	opts := promql.NewPrometheusQueryOpts(true, lookback, nil)
	qry, err := engine.NewRangeQuery(ctx, store, opts, query, start, end, step)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	actual := qry.Stats().Samples.SamplesRead

	require.InDelta(t, actual, est.SamplesRead, 4,
		"float estimate %d vs actual %d", est.SamplesRead, actual)
}

// TestEstimateCostHistogramFallback verifies that histogram sizing falls back
// to float-sized points when its narrow sampling window holds no data.
func TestEstimateCostHistogramFallback(t *testing.T) {
	// 51 exceeds both histogramSampleLimit and chunkSampleLimit (50).
	const numSeries = 51

	var sb strings.Builder
	sb.WriteString("load 10s\n")
	for i := range numSeries {
		fmt.Fprintf(&sb, "  metric{a=\"%d\"} 0+1x5\n", i)
	}
	store := promqltest.LoadedStorage(t, sb.String())
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	scrape := 10 * time.Second

	// Density sampling sees the old data in the full selector window. Histogram
	// sizing samples near the end because the series count exceeds its budget,
	// and that narrow window holds no points.
	ts := time.Unix(1000, 0)
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `last_over_time(metric[30m])`, ts, ts, 0, 5*time.Minute, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(numSeries), est.SeriesTouched)
	// With the data ending well before the proxy sampling window, the fallback
	// per-point cost (one unit) keeps SamplesRead strictly positive and finite
	// rather than collapsing to zero or erroring.
	require.Positive(t, est.SamplesRead)
}

// selectRecordingQueryable wraps a storage.Queryable and records the SelectHints
// of every Select call made through the queriers it hands out. It deliberately
// does not implement storage.ChunkQueryable, so EstimateCost takes its
// plain-Queryable path.
type selectRecordingQueryable struct {
	q     storage.Queryable
	hints []*storage.SelectHints
}

func (r *selectRecordingQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	qr, err := r.q.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &selectRecordingQuerier{Querier: qr, parent: r}, nil
}

type selectRecordingQuerier struct {
	storage.Querier
	parent *selectRecordingQueryable
}

func (r *selectRecordingQuerier) Select(ctx context.Context, sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	r.parent.hints = append(r.parent.hints, hints)
	return r.Querier.Select(ctx, sorted, hints, matchers...)
}

// TestEstimateCostPlainQueryableStaysIndexOnly verifies that per-point cost
// sampling is gated on storage.ChunkQueryable: against a plain storage.Queryable
// the estimator never decodes a sample and keeps fallbackAvgPointCost (one unit
// per point), even for a selector made entirely of native histograms whose real
// per-point cost is far above one unit.
func TestEstimateCostPlainQueryableStaysIndexOnly(t *testing.T) {
	// A schema-0 histogram with three buckets costs 13 sample-units per point in
	// the engine's accounting, so a decoded measurement would be unmistakable.
	store := promqltest.LoadedStorage(t, `
load 10s
  nh{a="1"} {{schema:0 sum:5 count:4 buckets:[1 2 1]}}+{{schema:0 sum:5 count:4 buckets:[1 2 1]}}x100
  nh{a="2"} {{schema:0 sum:5 count:4 buckets:[1 2 1]}}+{{schema:0 sum:5 count:4 buckets:[1 2 1]}}x100
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ts := time.Unix(600, 0)

	recorder := &selectRecordingQueryable{q: store}
	est, _, err := promql.EstimateCost(ctx, recorder, estimateTestParser, `nh[5m]`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)

	// Index-only: the single selector triggers exactly one Select, and it carries
	// the "series" hint that tells the storage samples are not needed. A
	// sample-decoding Select (sampleAvgPointCost selects without that hint) would
	// show up as a second, hint-less entry.
	require.Len(t, recorder.hints, 1)
	require.Equal(t, "series", recorder.hints[0].Func)

	// With fallbackAvgPointCost the histogram points are sized as floats:
	// samplesPerWindow(300000ms, 10s) = 300000/10000 + 1 = 31 samples per series,
	// two series at one unit per point = 62. Had the estimator decoded a point it
	// would have scaled by 13 instead.
	require.Equal(t, int64(62), est.SamplesRead)
}

// chunkMetaCountingQueryable wraps a storage exposing chunk metadata and counts,
// per ChunkQuerier it hands out, how many chunk metas that querier's callers
// examine. EstimateCost opens one ChunkQuerier per selector for density sampling.
type chunkMetaCountingQueryable struct {
	q     storage.Queryable
	cq    storage.ChunkQueryable
	metas []*int
}

func (c *chunkMetaCountingQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	return c.q.Querier(mint, maxt)
}

func (c *chunkMetaCountingQueryable) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	qr, err := c.cq.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	n := new(int)
	c.metas = append(c.metas, n)
	return &chunkMetaCountingQuerier{ChunkQuerier: qr, metas: n}, nil
}

type chunkMetaCountingQuerier struct {
	storage.ChunkQuerier
	metas *int
}

func (c *chunkMetaCountingQuerier) Select(ctx context.Context, sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	return &chunkMetaCountingSeriesSet{
		ChunkSeriesSet: c.ChunkQuerier.Select(ctx, sorted, hints, matchers...),
		metas:          c.metas,
	}
}

type chunkMetaCountingSeriesSet struct {
	storage.ChunkSeriesSet
	metas *int
}

func (s *chunkMetaCountingSeriesSet) At() storage.ChunkSeries {
	return &chunkMetaCountingSeries{ChunkSeries: s.ChunkSeriesSet.At(), metas: s.metas}
}

type chunkMetaCountingSeries struct {
	storage.ChunkSeries
	metas *int
}

// Iterator ignores the iterator offered for reuse: it belongs to this wrapper,
// not to the wrapped series.
func (s *chunkMetaCountingSeries) Iterator(chunks.Iterator) chunks.Iterator {
	return &chunkMetaCountingIterator{Iterator: s.ChunkSeries.Iterator(nil), metas: s.metas}
}

type chunkMetaCountingIterator struct {
	chunks.Iterator
	metas *int
}

// Next counts the chunks walked. Advancing a chunk iterator is what faults the
// chunk in and CRC-checks it, so counting Next rather than At measures the work
// the sample budgets exist to bound, whether or not the chunk is then used.
func (i *chunkMetaCountingIterator) Next() bool {
	if !i.Iterator.Next() {
		return false
	}
	*i.metas++
	return true
}

// TestEstimateCostChunkSampleBudgetBoundsChunksExamined verifies that the density
// sampler stops at its chunk budget even when every chunk holds only one point.
// Reaching a chunk meta faults it in; series counting uses the index instead.
func TestEstimateCostChunkSampleBudgetBoundsChunksExamined(t *testing.T) {
	// 200 single-sample series give 200 single-chunk series, well above
	// chunkSampleLimit (50), so the budget must bite.
	const numSeries = 200

	var sb strings.Builder
	sb.WriteString("load 10s\n")
	for i := range numSeries {
		fmt.Fprintf(&sb, "  metric{a=\"%d\"} 42\n", i)
	}
	store := promqltest.LoadedStorage(t, sb.String())
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	// The selector includes the only sample at t=0 for each series.
	ts := time.Unix(0, 0)

	counter := &chunkMetaCountingQueryable{q: store, cq: store}
	est, _, err := promql.EstimateCost(ctx, counter, estimateTestParser, `metric[10m]`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(numSeries), est.SeriesTouched)

	// Only the density sampler reads chunk metadata, stopping at its budget.
	require.Len(t, counter.metas, 1)
	require.Equal(t, 50, *counter.metas[0])
	require.Equal(t, int64(numSeries), est.SamplesRead)
}

// TestEstimateCostSamplingUsesSelectorWindow verifies that density sampling uses
// the selector's offset or @ window, while histogram sizing's fallback window
// ends at the same selector timestamp.
func TestEstimateCostSamplingUsesSelectorWindow(t *testing.T) {
	// 51 exceeds both histogramSampleLimit and chunkSampleLimit (50).
	const numSeries = 51

	var sb strings.Builder
	sb.WriteString("load 10s\n")
	for i := range numSeries {
		// Sparse: a sample every 60s for t in [0s,540s]. Then dense: a sample
		// every 10s for t in [600s,1200s].
		fmt.Fprintf(&sb, "  metric{a=\"%d\"} %s 1+0x60\n", i, strings.TrimSpace(strings.Repeat("1 _x5 ", 10)))
	}
	store := promqltest.LoadedStorage(t, sb.String())
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	// The query ends inside the dense region.
	ts := time.Unix(1200, 0)

	for _, q := range []string{
		// Both read [300s,600s], entirely inside the sparse region.
		`metric[5m] offset 10m`,
		`metric[5m] @ 600`,
	} {
		t.Run(q, func(t *testing.T) {
			// A deliberately wrong 1ms scrape interval keeps the fallback window at
			// its 5m minimum, [300s,600s], and makes any failure to measure
			// obvious: falling back to the supplied interval would size the 5m
			// range window at 300001 samples per series.
			est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, q, ts, ts, 0, 5*time.Minute, time.Minute, time.Millisecond)
			require.NoError(t, err)
			require.Equal(t, int64(numSeries), est.SeriesTouched)

			// The selector excludes 300s, leaving five points per series. Using
			// the query timestamp would instead sample the dense region.
			require.Equal(t, int64(numSeries*5), est.SamplesRead)
		})
	}
}

// TestEstimateCostStepInvariantSelectorReadOnce verifies that a selector inside
// a step-invariant subtree is charged once rather than once per step. The engine
// evaluates such a subtree at a single timestamp and copies the result to every
// step, so charging it per step over-estimates in proportion to the step count.
func TestEstimateCostStepInvariantSelectorReadOnce(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x2000
  metric{a="2"} 0+1x2000
`)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	start, end := time.Unix(4000, 0), time.Unix(4000+3600, 0)
	step := time.Minute

	// `metric @ 5000` is step invariant: two series read once each.
	invariant, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `metric @ 5000`, start, end, step, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(2), invariant.SamplesRead)

	// The same selector without the @ modifier is read at every one of the 61
	// steps, which is what the step-invariant form must not be charged.
	perStep, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `metric`, start, end, step, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(122), perStep.SamplesRead)
}

// TestEstimateCostResolvesDurationExpressions verifies that a range written as a
// duration expression is resolved before the windows are computed. Without
// preprocessing the expression is misread and the estimate collapses.
func TestEstimateCostResolvesDurationExpressions(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric{a="1"} 0+1x2000
  metric{a="2"} 0+1x2000
`)
	t.Cleanup(func() { store.Close() })

	durationParser := parser.NewParser(parser.Options{})
	ctx := context.Background()
	ts := time.Unix(5000, 0)

	// `[2m*2]` resolves to a 4m range, so both forms must estimate identically.
	expr, _, err := promql.EstimateCost(ctx, store, durationParser, `rate(metric[2m*2])`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	literal, _, err := promql.EstimateCost(ctx, store, durationParser, `rate(metric[4m])`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, literal.SamplesRead, expr.SamplesRead)
	require.Positive(t, expr.SamplesRead)
}

func TestEstimateCostInfoIncomplete(t *testing.T) {
	s := promqltest.LoadedStorage(t, `
load 10s
  metric{instance="a",job="1"} 1+0x100
  target_info{instance="a",job="1",data="one"} 1+0x100
  unrelated{data="one"} 1+0x100
`)
	t.Cleanup(func() { s.Close() })
	p := parser.NewParser(promqltest.TestParserOpts)
	for _, query := range []string{`info(metric)`, `info(metric, {data="one"})`, `info(info(metric))`} {
		t.Run(query, func(t *testing.T) {
			ts := time.Unix(1000, 0)
			est, warnings, err := promql.EstimateCost(context.Background(), s, p, query, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
			require.NoError(t, err)
			require.Equal(t, int64(1), est.SeriesTouched, "info label selectors must not be counted as standalone reads")
			require.Len(t, warnings, 1)
			require.Contains(t, warnings.AsErrors()[0].Error(), "estimate is incomplete: info()")
		})
	}
}

func TestEstimateCostEvaluationGrids(t *testing.T) {
	s := promqltest.LoadedStorage(t, `
load 10s
  metric{instance="a"} 0+1x2000
  metric{instance="b"} 0+1x2000
`)
	t.Cleanup(func() { s.Close() })
	engine := newQueryCostEngine(t)
	for _, query := range []string{
		"sum_over_time(metric[5m:1m])",
		"sum_over_time(metric[1s:1m])",
		"sum_over_time(sum_over_time(metric[20m:1m] @ 3000)[5m:1m])",
		"sum_over_time(sum_over_time(metric[20m:1m] offset 25s)[5m:1m] @ 3000)",
		"sum_over_time(metric[5m:1m] @ 1000)",
		"sum_over_time((metric @ 1000)[5m:1m])",
		"sum_over_time(sum_over_time(metric[20m:1m])[5m:1m])",
		"sum_over_time(metric[5m:1m] offset 25s)",
		"sum_over_time(metric[5m:1m] offset -25s)",
		"sum_over_time(metric[5m:] @ start())",
		"sum_over_time(metric[5m:1m] @ end())",
		"sum_over_time(rate(metric[2m])[5m:1m])",
	} {
		for _, step := range []time.Duration{0, 30 * time.Second} {
			t.Run(fmt.Sprintf("%s/%s", query, step), func(t *testing.T) {
				start, end := time.Unix(4000, 0), time.Unix(4000, 0)
				if step > 0 {
					end = time.Unix(7607, 0)
				}
				est, _, err := promql.EstimateCost(context.Background(), s, estimateTestParser, query, start, end, step, 5*time.Minute, time.Minute, 10*time.Second)
				require.NoError(t, err)
				var q promql.Query
				if step == 0 {
					q, err = engine.NewInstantQuery(context.Background(), s, nil, query, start)
				} else {
					q, err = engine.NewRangeQuery(context.Background(), s, nil, query, start, end, step)
				}
				require.NoError(t, err)
				defer q.Close()
				require.NoError(t, q.Exec(context.Background()).Err)
				require.InDelta(t, q.Stats().Samples.SamplesRead, est.SamplesRead, 4)
			})
		}
	}
}

// Inject an iterator error only into the point-size sampling path.
type sampleErrorQueryable struct {
	storage.SampleAndChunkQueryable
	err error
}

func (q sampleErrorQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	inner, err := q.SampleAndChunkQueryable.Querier(mint, maxt)
	return sampleErrorQuerier{Querier: inner, err: q.err}, err
}

type sampleErrorQuerier struct {
	storage.Querier
	err error
}

func (q sampleErrorQuerier) Select(ctx context.Context, sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	set := q.Querier.Select(ctx, sorted, hints, matchers...)
	if hints.Func == "series" {
		return set
	}
	return sampleErrorSeriesSet{SeriesSet: set, err: q.err}
}

type sampleErrorSeriesSet struct {
	storage.SeriesSet
	err error
}

func (s sampleErrorSeriesSet) At() storage.Series {
	return &storage.SeriesEntry{
		Lset: s.SeriesSet.At().Labels(),
		SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
			return sampleErrorIterator{Iterator: chunkenc.NewNopIterator(), err: s.err}
		},
	}
}

type sampleErrorIterator struct {
	chunkenc.Iterator
	err error
}

func (it sampleErrorIterator) Err() error { return it.err }

func TestEstimateCostSamplingErrors(t *testing.T) {
	s := promqltest.LoadedStorage(t, "load 10s\n  metric 1+1x10\n")
	t.Cleanup(func() { s.Close() })
	for _, want := range []error{errors.New("corrupt sample data"), context.Canceled} {
		t.Run(want.Error(), func(t *testing.T) {
			q := sampleErrorQueryable{SampleAndChunkQueryable: s, err: want}
			ts := time.Unix(100, 0)
			_, _, err := promql.EstimateCost(context.Background(), q, estimateTestParser, "metric", ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
			require.ErrorIs(t, err, want)
		})
	}
}
