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
	require.GreaterOrEqual(t, est.SamplesScanned, est.SeriesTouched)
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
	require.InDelta(t, int64(2*(5*60/10)), est5m.SamplesScanned, 2)

	// A wider window scans strictly more samples for the same series, and scales
	// with the window: roughly twice as many samples for a 10m window.
	est10m, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[10m])`, ts, ts, 0, 5*time.Minute, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(2), est10m.SeriesTouched)
	require.Greater(t, est10m.SamplesScanned, est5m.SamplesScanned)
	require.InDelta(t, int64(2*(10*60/10)), est10m.SamplesScanned, 2)
}

// TestEstimateCostRangeQueryMatchesActual verifies that the incremental sample
// model (M1) produces an estimate close to what the engine actually scans. It
// executes the same queries through a real engine and compares the estimate's
// SamplesScanned against the executed query's actual SamplesRead.
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
		opts := promql.NewPrometheusQueryOpts(true, lookback)
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
			require.InDelta(t, actual, est.SamplesScanned, c.delta,
				"estimate %d vs actual %d", est.SamplesScanned, actual)
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

	opts := promql.NewPrometheusQueryOpts(true, lookback)
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
	require.InDelta(t, actual, est.SamplesScanned, 16,
		"subquery estimate %d vs actual %d", est.SamplesScanned, actual)

	// The subquery estimate must dwarf the same selector evaluated as a plain
	// range query over the outer steps only, proving the subquery grid is folded
	// into numSteps rather than ignored.
	plain, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[5m])`, start, end, step, lookback, time.Minute, scrape)
	require.NoError(t, err)
	require.Greater(t, est.SamplesScanned, plain.SamplesScanned)
}

// TestEstimateCostSaturatesSamplesScanned verifies that an extreme window and
// step count never wrap SamplesScanned to a negative value (M4). The incremental
// model is bounded by the wall-clock span divided by the scrape interval, so it
// stays well below math.MaxInt64 for realistic Go durations; the saturating
// arithmetic remains a defensive guard against negative overflow.
func TestEstimateCostSaturatesSamplesScanned(t *testing.T) {
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
	require.Positive(t, est.SamplesScanned)
	require.LessOrEqual(t, est.SamplesScanned, int64(math.MaxInt64))
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

	// A non-positive scrape interval degrades SamplesScanned to one per series,
	// i.e. it equals SeriesTouched. The store is wrapped so it exposes only a
	// plain Queryable: with no chunk metadata the estimator cannot measure the
	// effective interval from chunk sample counts and must fall back to the
	// supplied (here non-positive) scrape interval.
	est, _, err := promql.EstimateCost(ctx, queryableOnly{store}, estimateTestParser, `rate(metric[5m])`, ts, ts, 0, 5*time.Minute, time.Minute, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)
	require.Equal(t, est.SeriesTouched, est.SamplesScanned)
}

// TestEstimateCostMeasuresIntervalFromChunks verifies that when the storage
// exposes chunk metadata the estimator measures the effective sample interval
// from a bounded sample of chunk sample counts, so SamplesScanned reflects the
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
	// sample per series (SamplesScanned == SeriesTouched). Because the store
	// exposes chunk metadata the estimator measures the ~10s interval from the
	// chunks and sizes the 5m window at roughly 30 samples per series.
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `rate(metric[5m])`, ts, ts, 0, 5*time.Minute, time.Minute, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), est.SeriesTouched)
	// A 5m window at the measured 10s interval is ~31 samples per series; allow a
	// one-interval boundary tolerance per series.
	require.InDelta(t, int64(2*(5*60/10)), est.SamplesScanned, 2,
		"measured-interval estimate %d", est.SamplesScanned)
}

// TestEstimateCostSamplesFromRealWindowExact verifies that when a selector's
// real window is cheap enough (few enough chunks/series to fit within
// chunkSampleLimit/histogramSampleLimit), sampleEffectiveInterval and
// sampleAvgPointCost measure directly from that real window instead of a
// synthetic proxy window near maxt, so the estimate is exact rather than an
// approximation.
//
// The query passes a deliberately wrong scrape interval (1ms). Before this fix,
// the estimator always sampled a narrow proxy window ending at maxt (clamped
// between 5m and 30m); here that proxy window ([300s,600s]) falls entirely after
// the loaded data ([0s,50s]) and would find nothing, so the old code would fall
// back to the caller-supplied (wrong) 1ms scrape interval, giving a wildly
// inflated estimate. With the fix, because the selector's real window has only
// one chunk per series (well under chunkSampleLimit) and only two series (well
// under histogramSampleLimit), the estimator measures the real 10s interval and
// float per-point cost directly from the real window, so SamplesScanned is exact
// and can be asserted with require.Equal rather than require.InDelta.
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

	// Each series has six samples at 0,10,...,50s: a real measured interval of
	// exactly 10s (a 50000ms span over 5 gaps). samplesPerWindow(600000ms, 10s) =
	// 600000/10000 + 1 = 61 samples per series; two series at a float per-point
	// cost of 1 gives exactly 122.
	require.Equal(t, int64(122), est.SamplesScanned)
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
	require.Equal(t, est.SeriesTouched, est.SamplesScanned)
}

// TestEstimateCostHistogramMatchesActual verifies that the native-histogram
// aware estimator (approach B sampling) sizes histogram points by their
// per-bucket cost, so SamplesScanned is close to the engine's real SamplesRead
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
		opts := promql.NewPrometheusQueryOpts(true, lookback)
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, c.query, c.start, c.end, c.step, lookback, time.Minute, scrape)
			require.NoError(t, err)
			actual := actualSamplesRead(t, c.query, c.start, c.end, c.step)
			require.InDelta(t, actual, est.SamplesScanned, c.delta,
				"estimate %d vs actual %d", est.SamplesScanned, actual)

			// Prove the multiplier works: the same query over a float series of
			// the identical layout (one unit per point) scans far fewer samples,
			// so the histogram estimate must be strictly and substantially larger.
			require.Greater(t, est.SamplesScanned, actual/2,
				"histogram estimate %d should reflect per-bucket cost", est.SamplesScanned)
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

	opts := promql.NewPrometheusQueryOpts(true, lookback)
	qry, err := engine.NewRangeQuery(ctx, store, opts, query, start, end, step)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	actual := qry.Stats().Samples.SamplesRead

	require.InDelta(t, actual, est.SamplesScanned, 4,
		"float estimate %d vs actual %d", est.SamplesScanned, actual)
}

// TestEstimateCostHistogramFallback verifies approach A: when sampling finds no
// in-window points (the series exist in the index but hold no samples in the
// narrow proxy window) the estimator falls back to the documented default
// per-point cost without erroring, so the estimate degrades to one unit per
// point rather than zero or a failure.
//
// The selector's real series/chunk count is pushed above
// histogramSampleLimit/chunkSampleLimit (51 series, one chunk each) so both
// sampleAvgPointCost and sampleEffectiveInterval take the proxy-window fallback
// path rather than sampling the real window directly; the proxy window is then
// placed where it holds no data, exercising the genuine "sampled nothing"
// fallback rather than a real measurement that happens to match the default.
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

	// Use a wide range selector evaluated at a time whose narrow proxy sampling
	// window (the last few minutes ending at maxt) holds no points, while the
	// selector's full range window still overlaps the loaded block so the index
	// matches the series. With more than histogramSampleLimit/chunkSampleLimit
	// series (each contributing one in-window chunk), the real window is too
	// expensive to sample directly, forcing the proxy-window fallback path; that
	// proxy window then finds nothing. This exercises the fallback path:
	// sampling finds no in-window points and the estimator assumes one unit per
	// point.
	ts := time.Unix(1000, 0)
	est, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `last_over_time(metric[30m])`, ts, ts, 0, 5*time.Minute, time.Minute, scrape)
	require.NoError(t, err)
	require.Equal(t, int64(numSeries), est.SeriesTouched)
	// With the data ending well before the proxy sampling window, the fallback
	// per-point cost (one unit) keeps SamplesScanned strictly positive and finite
	// rather than collapsing to zero or erroring.
	require.Positive(t, est.SamplesScanned)
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
	require.Equal(t, int64(62), est.SamplesScanned)
}

// chunkMetaCountingQueryable wraps a storage exposing chunk metadata and counts,
// per ChunkQuerier it hands out, how many chunk metas that querier's callers
// examine. EstimateCost opens one ChunkQuerier over the union window for
// countSeriesAndChunks and then one per selector inside sampleEffectiveInterval,
// so the counts are indexed in that order.
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

// TestEstimateCostChunkSampleBudgetBoundsChunksExamined verifies that
// chunkSampleLimit bounds every chunk meta the estimator examines, not just the
// usable ones it finds. Every series here holds a single sample, so every chunk
// spans no gap and can never inform the interval; without the budget counting
// every chunk examined the sampler would walk the whole selector looking for a
// usable one.
//
// Reaching a chunk meta faults the chunk in, so neither the budget probe nor the
// density sampler may walk the selector's chunks in proportion to its size: the
// series count comes from the index-only plain Querier instead.
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
	// The only samples sit at t=0, which is where the selector ends, so the
	// fallback sampling window near the selector's end really does hand the
	// sampler chunks to walk.
	ts := time.Unix(0, 0)

	counter := &chunkMetaCountingQueryable{q: store, cq: store}
	est, _, err := promql.EstimateCost(ctx, counter, estimateTestParser, `metric[10m]`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(numSeries), est.SeriesTouched)

	// One ChunkQuerier for the union-window chunk-budget probe and one for the
	// single selector's density sample.
	require.Len(t, counter.metas, 2)
	// The budget probe only has to learn whether the real window fits, so it must
	// stop one chunk past the budget rather than walking all 200.
	require.Equal(t, 51, *counter.metas[0])
	// The density sampler must stop at the budget.
	require.Equal(t, 50, *counter.metas[1])
}

// TestEstimateCostFallbackWindowEndsAtSelectorEnd verifies that the fallback
// sampling window is anchored at the selector's maxt rather than at the query's
// end. An offset or an @ modifier shifts a selector's window away from the query
// end; the fallback must measure the data the selector actually reads.
//
// The fixture is sparse (one sample per minute) early and dense (one sample per
// 10s) late, with 51 series so both samplers exceed their 50-item budgets and
// take the fallback path. The selector reads the sparse region while the query
// ends in the dense one, so the two anchors give measurably different estimates.
func TestEstimateCostFallbackWindowEndsAtSelectorEnd(t *testing.T) {
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

			// The fallback window [300s,600s] holds 6 sparse samples per series
			// spanning 300000ms over 5 gaps, so the measured interval is exactly
			// 60s. samplesPerWindow(300000ms, 60s) = 6 per series and the sampled
			// points are floats, so SamplesScanned is 51*6 = 306. Anchored at the
			// query's end (1200s) the sampler would instead measure the dense
			// region's 10s interval and report 51*31 = 1581.
			require.Equal(t, int64(numSeries*6), est.SamplesScanned)
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
	require.Equal(t, int64(2), invariant.SamplesScanned)

	// The same selector without the @ modifier is read at every one of the 61
	// steps, which is what the step-invariant form must not be charged.
	perStep, _, err := promql.EstimateCost(ctx, store, estimateTestParser, `metric`, start, end, step, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(122), perStep.SamplesScanned)
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

	durationParser := parser.NewParser(parser.Options{ExperimentalDurationExpr: true})
	ctx := context.Background()
	ts := time.Unix(5000, 0)

	// `[2m*2]` resolves to a 4m range, so both forms must estimate identically.
	expr, _, err := promql.EstimateCost(ctx, store, durationParser, `rate(metric[2m*2])`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	literal, _, err := promql.EstimateCost(ctx, store, durationParser, `rate(metric[4m])`, ts, ts, 0, 5*time.Minute, time.Minute, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, literal.SamplesScanned, expr.SamplesScanned)
	require.Positive(t, expr.SamplesScanned)
}
