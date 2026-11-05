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
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestQuery_Exec_QueryCostDurationInput(t *testing.T) {
	t.Run("interruption", func(t *testing.T) {
		store := promqltest.LoadedStorage(t, `
load 1s
  metric 0+1x1000
`)
		t.Cleanup(func() { require.NoError(t, store.Close()) })
		for _, expr := range []string{
			"metric[1000s]", "sum_over_time(metric[1000s])", "metric", "timestamp(metric)",
			"metric smoothed", "sum_over_time(metric[1000s:1s])",
		} {
			for _, rangeQuery := range []bool{false, true} {
				if rangeQuery && expr == "metric[1000s]" {
					continue
				}
				for _, quota := range []uint64{0, 1000000} {
					for _, canceled := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/range=%t/quota=%d/canceled=%t", expr, rangeQuery, quota, canceled), func(t *testing.T) {
							synctest.Test(t, func(t *testing.T) {
								ctx, cancel := context.WithCancel(t.Context())
								defer cancel()
								reg := prometheus.NewRegistry()
								engine := durationTestEngine(t, reg, nil, 10*time.Second)
								engine.SetQueryLimits(0, quota, time.Second)
								counted := &sampleReadQueryable{Queryable: store}
								counted.onRead = func() {
									if counted.reads == 10 {
										if canceled {
											cancel()
										} else {
											time.Sleep(2 * time.Second)
										}
									}
								}
								opts := promql.NewPrometheusQueryOpts(true, 0, nil)
								var query promql.Query
								var err error
								if rangeQuery {
									query, err = engine.NewRangeQuery(ctx, counted, opts, expr, time.Unix(1000, 0), time.Unix(1001, 0), time.Second)
								} else {
									query, err = engine.NewInstantQuery(ctx, counted, opts, expr, time.Unix(1000, 0))
								}
								require.NoError(t, err)
								defer query.Close()
								result := query.Exec(ctx)
								if canceled {
									var want promql.ErrQueryCanceled
									require.ErrorAs(t, result.Err, &want)
									require.Zero(t, queryCostRejections(t, reg, "max_duration"))
								} else {
									var want promql.ErrQueryTimeout
									require.ErrorAs(t, result.Err, &want)
									require.Equal(t, float64(1), queryCostRejections(t, reg, "max_duration"))
								}
								require.Nil(t, result.Value)
								require.Equal(t, 10, counted.reads, "do not continue decoding after the interrupted access")
								if expr == "metric[1000s]" || expr == "sum_over_time(metric[1000s])" || expr == "metric smoothed" {
									require.Equal(t, int64(10), query.Stats().Samples.SamplesRead, "preserve the interrupted matrix read cost")
								}
							})
						})
					}
				}
			}
		}
	})

	t.Run("initialization", func(t *testing.T) {
		for _, expr := range []string{"metric", "timestamp(metric)", "metric[1s]", "sum_over_time(metric[1s])"} {
			t.Run(expr, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					reg := prometheus.NewRegistry()
					engine := durationTestEngine(t, reg, nil, 10*time.Second)
					engine.SetQueryLimits(0, 0, time.Second)
					advances := 0
					base := storage.NewListSeries(labels.FromStrings("__name__", "metric"), chunks.GenerateSamples(0, 10))
					series := &storage.SeriesEntry{Lset: base.Labels(), SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator {
						return durationInitialIterator{Iterator: base.Iterator(nil), onNext: func() {
							advances++
							time.Sleep(2 * time.Second)
						}}
					}}
					q := costProbeQueryable{q: costProbeQuerier{Querier: storage.NoopQuerier(), selectSeries: func(context.Context, *storage.SelectHints) storage.SeriesSet {
						return &costProbeSeriesSet{SeriesSet: storage.NoopSeriesSet(), series: series, limit: 1}
					}}}
					query, err := engine.NewInstantQuery(t.Context(), q, nil, expr, time.Unix(0, 0))
					require.NoError(t, err)
					defer query.Close()
					result := query.Exec(t.Context())
					var want promql.ErrQueryTimeout
					require.ErrorAs(t, result.Err, &want)
					require.Nil(t, result.Value)
					require.Equal(t, 1, advances)
					require.Zero(t, query.Stats().Samples.SamplesRead)
					require.Equal(t, float64(1), queryCostRejections(t, reg, "max_duration"))
				})
			})
		}
	})

	t.Run("final_sample", func(t *testing.T) {
		store := promqltest.LoadedStorage(t, `
load 1s
  metric 1
`)
		t.Cleanup(func() { require.NoError(t, store.Close()) })
		for _, expr := range []string{"metric", "scalar(metric)", "metric[1s]"} {
			t.Run(expr, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					engine := newQueryCostEngine(t)
					engine.SetQueryLimits(0, 0, time.Second)
					counted := &sampleReadQueryable{Queryable: store, onRead: func() { time.Sleep(2 * time.Second) }}
					query, err := engine.NewInstantQuery(t.Context(), counted, nil, expr, time.Unix(0, 0))
					require.NoError(t, err)
					defer query.Close()
					result := query.Exec(t.Context())
					var want promql.ErrQueryTimeout
					require.ErrorAs(t, result.Err, &want)
					require.Nil(t, result.Value)
					require.Equal(t, 1, counted.reads)
					require.Equal(t, int64(1), query.Stats().Samples.SamplesRead)
				})
			})
		}
	})
}

type durationInitialIterator struct {
	chunkenc.Iterator
	onNext func()
}

func (it durationInitialIterator) Next() chunkenc.ValueType {
	it.onNext()
	return it.Iterator.Next()
}

func durationTestEngine(t *testing.T, reg *prometheus.Registry, tracker promql.QueryTracker, timeout time.Duration) *promql.Engine {
	return promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		Reg: reg, MaxSamples: 1000000, Timeout: timeout, ActiveQueryTracker: tracker,
		EnableQueryCost: true, EnablePerStepStats: true, EnableAtModifier: true,
		EnableNegativeOffset: true, Parser: parser.NewParser(promqltest.TestParserOpts),
		NoStepSubqueryIntervalFn: func(int64) int64 { return time.Minute.Milliseconds() },
	})
}

type costCompletionQueryable struct {
	storage.Queryable
	onClose func()
	warning error
	closes  int
}

func (q *costCompletionQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	inner, err := q.Queryable.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return costCompletionQuerier{Querier: inner, owner: q}, nil
}

type costCompletionQuerier struct {
	storage.Querier
	owner *costCompletionQueryable
}

func (q costCompletionQuerier) Close() error {
	err := q.Querier.Close()
	q.owner.closes++
	if q.owner.closes == 1 {
		q.owner.onClose()
	}
	return err
}

func (q costCompletionQuerier) Select(ctx context.Context, sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	return costCompletionSet{SeriesSet: q.Querier.Select(ctx, sorted, hints, matchers...), warning: q.owner.warning}
}

type costCompletionSet struct {
	storage.SeriesSet
	warning error
}

func (s costCompletionSet) Warnings() annotations.Annotations {
	annos := s.SeriesSet.Warnings()
	return annos.Add(s.warning)
}

func TestQuery_Exec_QueryCostDurationCompletion(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
load 1s
  metric{instance="a"} 1
  metric{instance="b"} 2
`)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	for _, tc := range []struct {
		name                  string
		ceiling, request      time.Duration
		caller, canceled      bool
		seriesLimit           uint64
		wantDurationRejection float64
	}{
		{name: "configured duration", ceiling: time.Second, wantDurationRejection: 1},
		{name: "request duration", request: time.Second, wantDurationRejection: 1},
		{name: "startup timeout"},
		{name: "earlier caller deadline", ceiling: 10 * time.Second, caller: true},
		{name: "caller cancellation", ceiling: 10 * time.Second, canceled: true},
		{name: "preserve execution error", ceiling: time.Second, seriesLimit: 1},
	} {
		for _, expr := range []string{"metric", "scalar(metric)", "metric[1s]"} {
			t.Run(tc.name+"/"+expr, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if tc.caller {
						var deadlineCancel context.CancelFunc
						ctx, deadlineCancel = context.WithTimeout(ctx, time.Second)
						defer deadlineCancel()
					}
					tracker, err := promql.NewActiveQueryTracker(t.TempDir(), 1, nil)
					require.NoError(t, err)
					reg := prometheus.NewRegistry()
					engine := durationTestEngine(t, reg, tracker, time.Second)
					engine.SetQueryLimits(tc.seriesLimit, 0, tc.ceiling)
					warning := errors.New("storage advisory")
					q := &costCompletionQueryable{Queryable: store, warning: warning, onClose: func() {
						if tc.canceled {
							cancel()
						} else {
							time.Sleep(2 * time.Second)
						}
					}}
					opts := promql.NewPrometheusQueryOptsWithLimits(true, 0, 0, 0, tc.request, nil)
					query, err := engine.NewInstantQuery(ctx, q, opts, expr, time.Unix(0, 0))
					require.NoError(t, err)
					defer query.Close()
					result := query.Exec(ctx)
					switch {
					case tc.seriesLimit > 0:
						var want promql.ErrTooManySeries
						require.ErrorAs(t, result.Err, &want)
						require.Equal(t, float64(1), queryCostRejections(t, reg, "max_series"))
					case tc.canceled:
						var want promql.ErrQueryCanceled
						require.ErrorAs(t, result.Err, &want)
					default:
						var want promql.ErrQueryTimeout
						require.ErrorAs(t, result.Err, &want)
					}
					require.Nil(t, result.Value)
					require.Equal(t, 1, q.closes)
					require.Equal(t, int64(2), query.Stats().Samples.TotalSeries)
					if tc.seriesLimit == 0 {
						require.Equal(t, int64(2), query.Stats().Samples.SamplesRead)
						require.NotEmpty(t, result.Warnings.AsErrors())
						require.ErrorIs(t, result.Warnings.AsErrors()[0], warning)
					}
					require.Equal(t, tc.wantDurationRejection, queryCostRejections(t, reg, "max_duration"))
					// Failure must release the single query slot and leave storage usable.
					engine.SetQueryLimits(0, 0, 10*time.Second)
					recovery, err := engine.NewInstantQuery(t.Context(), q, nil, "metric", time.Unix(0, 0))
					require.NoError(t, err)
					defer recovery.Close()
					require.NoError(t, recovery.Exec(t.Context()).Err)
					require.Equal(t, 2, q.closes)
				})
			})
		}
	}
}
