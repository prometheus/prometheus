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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestEstimateCost_RemoteSamplingFallback(t *testing.T) {
	for _, kind := range []string{"float", "histogram"} {
		t.Run(kind, func(t *testing.T) {
			const points = 200000
			var series storage.Series
			metric := labels.FromStrings("__name__", "metric")
			if kind == "float" {
				timestamps, values := make([]int64, points), make([]float64, points)
				for i := range timestamps {
					timestamps[i], values[i] = int64(i), float64(i)
				}
				series = &storage.SeriesEntry{Lset: metric, SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
					return chunkenc.MockSeriesIterator(nil, timestamps, values)
				}}
			} else {
				hs := tsdbutil.GenerateTestHistograms(1000)
				histograms := make([]prompb.Histogram, len(hs))
				for i, h := range hs {
					histograms[i] = prompb.FromIntHistogram(int64(i), h)
				}
				set := remote.FromQueryResult(false, &prompb.QueryResult{Timeseries: []*prompb.TimeSeries{{Labels: []prompb.Label{{Name: "__name__", Value: "metric"}}, Histograms: histograms}}})
				require.True(t, set.Next())
				series = set.At()
			}
			counted := &sampleReadQueryable{}
			client := &costProbeReadClient{series: sampleReadSeries{Series: series, owner: counted}}
			q := remote.NewSampleAndChunkQueryableClient(client, labels.EmptyLabels(), nil, true, nil)
			ts := time.Unix(200, 0)
			estimate, annos, err := promql.EstimateCost(t.Context(), q, estimateTestParser, "sum_over_time(metric[200s]) + sum_over_time(metric[200s])", ts, ts, 0, time.Minute, time.Minute, time.Millisecond)
			require.NoError(t, err)
			require.Equal(t, int64(2), estimate.SeriesTouched)
			require.Equal(t, int64(2*(points+1)), estimate.SamplesRead)
			require.Len(t, client.requests, 2, "one enumeration request per selector; no sampling requests")
			for _, req := range client.requests {
				require.Equal(t, "series", req.Hints.Func)
			}
			require.Zero(t, counted.reads, "remote adapter must never decode and re-encode a whole series")
			require.Len(t, annos.AsErrors(), 1, "fallback warning is deduplicated")
			require.Contains(t, annos.AsErrors()[0].Error(), "bounded storage sampling is unavailable")
		})
	}
}

type costProbeReadClient struct {
	series   storage.Series
	requests []*prompb.Query
}

func (c *costProbeReadClient) Read(_ context.Context, query *prompb.Query, _ bool) (storage.SeriesSet, error) {
	c.requests = append(c.requests, query)
	return &costProbeSeriesSet{SeriesSet: storage.NoopSeriesSet(), series: c.series, limit: 1}, nil
}

func (c *costProbeReadClient) ReadMultiple(ctx context.Context, queries []*prompb.Query, sorted bool) (storage.SeriesSet, error) {
	return c.Read(ctx, queries[0], sorted)
}

type costProbeSeriesSet struct {
	storage.SeriesSet
	series          storage.Series
	limit, position int
	onDone          func()
	onNext          func()
}

func (s *costProbeSeriesSet) Next() bool {
	if s.onNext != nil {
		s.onNext()
	}
	if s.position == s.limit {
		if s.onDone != nil {
			s.onDone()
		}
		return false
	}
	s.position++
	return true
}
func (s *costProbeSeriesSet) At() storage.Series { return s.series }

type costProbeQueryable struct {
	q  storage.Querier
	cq storage.ChunkQuerier
}

func (q costProbeQueryable) Querier(int64, int64) (storage.Querier, error) { return q.q, nil }
func (q costProbeQueryable) ChunkQuerier(int64, int64) (storage.ChunkQuerier, error) {
	return q.cq, nil
}

func (q costProbeQueryable) CostSamplingQueryable() (storage.SampleAndChunkQueryable, bool) {
	return q, true
}

type costProbeQuerier struct {
	storage.Querier
	selectSeries func(context.Context, *storage.SelectHints) storage.SeriesSet
	onClose      func() error
}

func (q costProbeQuerier) Select(ctx context.Context, _ bool, hints *storage.SelectHints, _ ...*labels.Matcher) storage.SeriesSet {
	return q.selectSeries(ctx, hints)
}

func (q costProbeQuerier) Close() error {
	if q.onClose != nil {
		return q.onClose()
	}
	return q.Querier.Close()
}

type costProbeChunkQuerier struct {
	storage.ChunkQuerier
	selectChunks func() storage.ChunkSeriesSet
}

func (q costProbeChunkQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	return q.selectChunks()
}

type costProbeChunkSet struct {
	storage.ChunkSeriesSet
	series          storage.ChunkSeries
	limit, position int
	onDone          func()
	onNext          func()
}

func (s *costProbeChunkSet) Next() bool {
	if s.onNext != nil {
		s.onNext()
	}
	if s.position == s.limit {
		if s.onDone != nil {
			s.onDone()
		}
		return false
	}
	s.position++
	return true
}
func (s *costProbeChunkSet) At() storage.ChunkSeries { return s.series }

func TestEstimateCost_SamplingSeriesBudgets(t *testing.T) {
	densityNext, pointNext, pointIterators := 0, 0, 0
	series := &storage.SeriesEntry{Lset: labels.FromStrings("__name__", "metric"), SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
		pointIterators++
		return chunkenc.NewNopIterator()
	}}
	emptyChunks := &storage.ChunkSeriesEntry{Lset: series.Labels(), ChunkIteratorFn: func(chunks.Iterator) chunks.Iterator {
		return storage.NewListChunkSeriesIterator()
	}}
	q := costProbeQueryable{
		q: costProbeQuerier{Querier: storage.NoopQuerier(), selectSeries: func(_ context.Context, hints *storage.SelectHints) storage.SeriesSet {
			set := &costProbeSeriesSet{SeriesSet: storage.NoopSeriesSet(), series: series, limit: 100}
			if hints.Func != "series" {
				set.onNext = func() { pointNext++ }
			}
			return set
		}},
		cq: costProbeChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), selectChunks: func() storage.ChunkSeriesSet {
			return &costProbeChunkSet{ChunkSeriesSet: storage.EmptyChunkSeriesSet(), series: emptyChunks, limit: 100, onNext: func() { densityNext++ }}
		}},
	}
	ts := time.Unix(200, 0)
	estimate, _, err := promql.EstimateCost(t.Context(), q, estimateTestParser, "metric[200s]", ts, ts, 0, time.Minute, time.Minute, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(100), estimate.SeriesTouched)
	require.Equal(t, 50, densityNext, "empty/tombstoned series must not escape the density series budget")
	require.Equal(t, 50, pointNext, "must not materialize a 51st series")
	require.Equal(t, 50, pointIterators)
}

type costResourceParser struct {
	parser.Parser
	beforeParse func()
}

func (p costResourceParser) ParseExpr(expr string) (parser.Expr, error) {
	p.beforeParse()
	return p.Parser.ParseExpr(expr)
}

func TestEstimateCost_CancellationAtBoundaries(t *testing.T) {
	for _, stage := range []string{"before parsing", "selector-free parsing", "invalid query parsing", "empty series", "series exhaustion", "empty density", "empty points", "reader close"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			selects, chunkSelects, parses := 0, 0, 0
			series := storage.NewListSeries(labels.FromStrings("__name__", "metric"), chunks.GenerateSamples(0, 1))
			chunkSeries := storage.NewListChunkSeriesFromSamples(series.Labels(), chunks.GenerateSamples(0, 1))
			q := costProbeQueryable{
				q: costProbeQuerier{Querier: storage.NoopQuerier(), selectSeries: func(_ context.Context, hints *storage.SelectHints) storage.SeriesSet {
					selects++
					set := &costProbeSeriesSet{SeriesSet: storage.NoopSeriesSet(), series: series, limit: 1}
					if hints.Func == "series" {
						if stage == "empty series" {
							set.limit = 0
						}
						if stage == "empty series" || stage == "series exhaustion" {
							set.onDone = cancel
						}
					} else if stage == "empty points" {
						set.limit = 0
						set.onDone = cancel
					}
					return set
				}, onClose: func() error {
					if stage == "reader close" {
						cancel()
					}
					return nil
				}},
				cq: costProbeChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), selectChunks: func() storage.ChunkSeriesSet {
					chunkSelects++
					set := &costProbeChunkSet{ChunkSeriesSet: storage.EmptyChunkSeriesSet(), series: chunkSeries, limit: 1}
					if stage == "empty density" {
						set.limit = 0
						set.onDone = cancel
					}
					return set
				}},
			}
			p := costResourceParser{Parser: estimateTestParser, beforeParse: func() {
				parses++
				if stage == "selector-free parsing" || stage == "invalid query parsing" {
					cancel()
				}
			}}
			expr := "metric[200s]"
			if stage == "selector-free parsing" {
				expr = "1"
			}
			if stage == "invalid query parsing" {
				expr = "broken("
			}
			if stage == "before parsing" {
				cancel()
			}
			ts := time.Unix(200, 0)
			estimate, _, err := promql.EstimateCost(ctx, q, p, expr, ts, ts, 0, time.Minute, time.Minute, time.Second)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, promql.CostEstimate{}, estimate, "no successful or partial estimate after cancellation")
			if stage == "invalid query parsing" {
				var parseErrors parser.ParseErrors
				require.ErrorAs(t, err, &parseErrors)
			}
			if stage == "before parsing" {
				require.Zero(t, parses)
			}
			if stage == "before parsing" || stage == "selector-free parsing" || stage == "invalid query parsing" {
				require.Zero(t, selects)
			}
			if stage == "empty series" || stage == "series exhaustion" {
				require.Zero(t, chunkSelects)
			}
			if stage == "empty density" {
				require.Equal(t, 1, selects, "no point sampling after cancellation")
			}
		})
	}
}

func TestEstimateCost_QuerierCloseErrors(t *testing.T) {
	want := errors.New("reader close failed")
	q := costProbeQueryable{q: costProbeQuerier{Querier: storage.NoopQuerier(), selectSeries: func(context.Context, *storage.SelectHints) storage.SeriesSet {
		return storage.NoopSeriesSet()
	}, onClose: func() error { return want }}, cq: storage.NoopChunkedQuerier()}
	ts := time.Unix(200, 0)
	estimate, _, err := promql.EstimateCost(t.Context(), q, estimateTestParser, "metric", ts, ts, 0, time.Minute, time.Minute, time.Second)
	require.ErrorIs(t, err, want)
	require.Equal(t, promql.CostEstimate{}, estimate)
}

type costSamplingOverrideQueryable struct {
	storage.Queryable
	source storage.SampleAndChunkQueryable
}

func (q costSamplingOverrideQueryable) CostSamplingQueryable() (storage.SampleAndChunkQueryable, bool) {
	return q.source, true
}

type unsupportedPointCostQueryable struct {
	storage.SampleAndChunkQueryable
}

func (unsupportedPointCostQueryable) Querier(int64, int64) (storage.Querier, error) {
	return nil, storage.ErrCostSamplingUnsupported
}

func TestEstimateCost_SamplingFallbackDiscardsDensity(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 10s
  metric 0+1x20
`)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	source, supported := store.CostSamplingQueryable()
	require.True(t, supported)
	q := costSamplingOverrideQueryable{Queryable: store, source: unsupportedPointCostQueryable{source}}
	ts := time.Unix(200, 0)
	estimate, annos, err := promql.EstimateCost(t.Context(), q, estimateTestParser, "metric[200s]", ts, ts, 0, time.Minute, time.Minute, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(1), estimate.SeriesTouched)
	require.Equal(t, int64(201), estimate.SamplesRead, "later unsupported point sampling must discard the measured density and window cap")
	require.Len(t, annos.AsErrors(), 1)
	require.Contains(t, annos.AsErrors()[0].Error(), "bounded storage sampling is unavailable")
}
