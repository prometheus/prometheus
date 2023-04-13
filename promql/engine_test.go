// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/prometheus/prometheus/tsdb/tsdbutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/stats"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestQueryConcurrency(t *testing.T) {
	maxConcurrency := 10

	dir, err := os.MkdirTemp("", "test_concurrency")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	queryTracker := NewActiveQueryTracker(dir, maxConcurrency, nil)

	opts := EngineOpts{
		Logger:             nil,
		Reg:                nil,
		MaxSamples:         10,
		Timeout:            100 * time.Second,
		ActiveQueryTracker: queryTracker,
	}

	engine := NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	block := make(chan struct{})
	processing := make(chan struct{})
	done := make(chan int)
	defer close(done)

	f := func(context.Context) error {
		select {
		case processing <- struct{}{}:
		case <-done:
		}

		select {
		case <-block:
		case <-done:
		}
		return nil
	}

	for i := 0; i < maxConcurrency; i++ {
		q := engine.newTestQuery(f)
		go q.Exec(ctx)
		select {
		case <-processing:
			// Expected.
		case <-time.After(20 * time.Millisecond):
			require.Fail(t, "Query within concurrency threshold not being executed")
		}
	}

	q := engine.newTestQuery(f)
	go q.Exec(ctx)

	select {
	case <-processing:
		require.Fail(t, "Query above concurrency threshold being executed")
	case <-time.After(20 * time.Millisecond):
		// Expected.
	}

	// Terminate a running query.
	block <- struct{}{}

	select {
	case <-processing:
		// Expected.
	case <-time.After(20 * time.Millisecond):
		require.Fail(t, "Query within concurrency threshold not being executed")
	}

	// Terminate remaining queries.
	for i := 0; i < maxConcurrency; i++ {
		block <- struct{}{}
	}
}

func TestQueryTimeout(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    5 * time.Millisecond,
	}
	engine := NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	query := engine.newTestQuery(func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec(ctx)
	require.Error(t, res.Err, "expected timeout error but got none")

	var e ErrQueryTimeout
	require.True(t, errors.As(res.Err, &e), "expected timeout error but got: %s", res.Err)
}

const errQueryCanceled = ErrQueryCanceled("test statement execution")

func TestQueryCancel(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// Cancel a running query before it completes.
	block := make(chan struct{})
	processing := make(chan struct{})

	query1 := engine.newTestQuery(func(ctx context.Context) error {
		processing <- struct{}{}
		<-block
		return contextDone(ctx, "test statement execution")
	})

	var res *Result

	go func() {
		res = query1.Exec(ctx)
		processing <- struct{}{}
	}()

	<-processing
	query1.Cancel()
	block <- struct{}{}
	<-processing

	require.Error(t, res.Err, "expected cancellation error for query1 but got none")
	require.Equal(t, errQueryCanceled, res.Err)

	// Canceling a query before starting it must have no effect.
	query2 := engine.newTestQuery(func(ctx context.Context) error {
		return contextDone(ctx, "test statement execution")
	})

	query2.Cancel()
	res = query2.Exec(ctx)
	require.NoError(t, res.Err)
}

// errQuerier implements storage.Querier which always returns error.
type errQuerier struct {
	err error
}

func (q *errQuerier) Select(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return errSeriesSet{err: q.err}
}

func (*errQuerier) LabelValues(string, ...*labels.Matcher) ([]string, storage.Warnings, error) {
	return nil, nil, nil
}

func (*errQuerier) LabelNames(...*labels.Matcher) ([]string, storage.Warnings, error) {
	return nil, nil, nil
}
func (*errQuerier) Close() error { return nil }

// errSeriesSet implements storage.SeriesSet which always returns error.
type errSeriesSet struct {
	err error
}

func (errSeriesSet) Next() bool                   { return false }
func (errSeriesSet) At() storage.Series           { return nil }
func (e errSeriesSet) Err() error                 { return e.err }
func (e errSeriesSet) Warnings() storage.Warnings { return nil }

func TestQueryError(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)
	errStorage := ErrStorage{errors.New("storage error")}
	queryable := storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return &errQuerier{err: errStorage}, nil
	})
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	vectorQuery, err := engine.NewInstantQuery(queryable, nil, "foo", time.Unix(1, 0))
	require.NoError(t, err)

	res := vectorQuery.Exec(ctx)
	require.Error(t, res.Err, "expected error on failed select but got none")
	require.True(t, errors.Is(res.Err, errStorage), "expected error doesn't match")

	matrixQuery, err := engine.NewInstantQuery(queryable, nil, "foo[1m]", time.Unix(1, 0))
	require.NoError(t, err)

	res = matrixQuery.Exec(ctx)
	require.Error(t, res.Err, "expected error on failed select but got none")
	require.True(t, errors.Is(res.Err, errStorage), "expected error doesn't match")
}

type noopHintRecordingQueryable struct {
	hints []*storage.SelectHints
}

func (h *noopHintRecordingQueryable) Querier(context.Context, int64, int64) (storage.Querier, error) {
	return &hintRecordingQuerier{Querier: &errQuerier{}, h: h}, nil
}

type hintRecordingQuerier struct {
	storage.Querier

	h *noopHintRecordingQueryable
}

func (h *hintRecordingQuerier) Select(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	h.h.hints = append(h.h.hints, hints)
	return h.Querier.Select(sortSeries, hints, matchers...)
}

func TestSelectHintsSetCorrectly(t *testing.T) {
	opts := EngineOpts{
		Logger:           nil,
		Reg:              nil,
		MaxSamples:       10,
		Timeout:          10 * time.Second,
		LookbackDelta:    5 * time.Second,
		EnableAtModifier: true,
	}

	for _, tc := range []struct {
		query string

		// All times are in milliseconds.
		start int64
		end   int64

		// TODO(bwplotka): Add support for better hints when subquerying.
		expected []*storage.SelectHints
	}{
		{
			query: "foo", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5000, End: 10000},
			},
		}, {
			query: "foo @ 15", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 10000, End: 15000},
			},
		}, {
			query: "foo @ 1", start: 10000,
			expected: []*storage.SelectHints{
				{Start: -4000, End: 1000},
			},
		}, {
			query: "foo[2m]", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 80000, End: 200000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 180", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 60000, End: 180000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 300", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 180000, End: 300000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 60", start: 200000,
			expected: []*storage.SelectHints{
				{Start: -60000, End: 60000, Range: 120000},
			},
		}, {
			query: "foo[2m] offset 2m", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 60000, End: 180000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 200 offset 2m", start: 300000,
			expected: []*storage.SelectHints{
				{Start: -40000, End: 80000, Range: 120000},
			},
		}, {
			query: "foo[2m:1s]", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 300000, Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s])", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 300)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 200)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 75000, End: 200000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 100)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: -25000, End: 100000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 165000, End: 290000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 155000, End: 280000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 185000, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 185000, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: -45000, End: 80000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "foo", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: 5000, End: 20000, Step: 1000},
			},
		}, {
			query: "foo @ 15", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: 10000, End: 15000, Step: 1000},
			},
		}, {
			query: "foo @ 1", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: -4000, End: 1000, Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 180)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 60000, End: 180000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 300)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 180000, End: 300000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 60)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -60000, End: 60000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m])", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 80000, End: 500000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] offset 2m)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 60000, End: 380000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m:1s])", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 500000, Func: "rate", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s])", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 500000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 165000, End: 490000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 300)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175000, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 200)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 75000, End: 200000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 100)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -25000, End: 100000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 155000, End: 480000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 185000, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 185000, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -45000, End: 80000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "sum by (dim1) (foo)", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5000, End: 10000, Func: "sum", By: true, Grouping: []string{"dim1"}},
			},
		}, {
			query: "sum without (dim1) (foo)", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5000, End: 10000, Func: "sum", Grouping: []string{"dim1"}},
			},
		}, {
			query: "sum by (dim1) (avg_over_time(foo[1s]))", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 9000, End: 10000, Func: "avg_over_time", Range: 1000},
			},
		}, {
			query: "sum by (dim1) (max by (dim2) (foo))", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5000, End: 10000, Func: "max", By: true, Grouping: []string{"dim2"}},
			},
		}, {
			query: "(max by (dim1) (foo))[5s:1s]", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 0, End: 10000, Func: "max", By: true, Grouping: []string{"dim1"}, Step: 1000},
			},
		}, {
			query: "(sum(http_requests{group=~\"p.*\"})+max(http_requests{group=~\"c.*\"}))[20s:5s]", start: 120000,
			expected: []*storage.SelectHints{
				{Start: 95000, End: 120000, Func: "sum", By: true, Step: 5000},
				{Start: 95000, End: 120000, Func: "max", By: true, Step: 5000},
			},
		}, {
			query: "foo @ 50 + bar @ 250 + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 45000, End: 50000, Step: 1000},
				{Start: 245000, End: 250000, Step: 1000},
				{Start: 895000, End: 900000, Step: 1000},
			},
		}, {
			query: "foo @ 50 + bar + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 45000, End: 50000, Step: 1000},
				{Start: 95000, End: 500000, Step: 1000},
				{Start: 895000, End: 900000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s] @ 50) + bar @ 250 + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 48000, End: 50000, Step: 1000, Func: "rate", Range: 2000},
				{Start: 245000, End: 250000, Step: 1000},
				{Start: 895000, End: 900000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s:1s] @ 50) + bar + baz", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 43000, End: 50000, Step: 1000, Func: "rate"},
				{Start: 95000, End: 500000, Step: 1000},
				{Start: 95000, End: 500000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s:1s] @ 50) + bar + rate(baz[2m:1s] @ 900 offset 2m) ", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 43000, End: 50000, Step: 1000, Func: "rate"},
				{Start: 95000, End: 500000, Step: 1000},
				{Start: 655000, End: 780000, Step: 1000, Func: "rate"},
			},
		}, { // Hints are based on the inner most subquery timestamp.
			query: `sum_over_time(sum_over_time(metric{job="1"}[100s])[100s:25s] @ 50)[3s:1s] @ 3000`, start: 100000,
			expected: []*storage.SelectHints{
				{Start: -150000, End: 50000, Range: 100000, Func: "sum_over_time", Step: 25000},
			},
		}, { // Hints are based on the inner most subquery timestamp.
			query: `sum_over_time(sum_over_time(metric{job="1"}[100s])[100s:25s] @ 3000)[3s:1s] @ 50`,
			expected: []*storage.SelectHints{
				{Start: 2800000, End: 3000000, Range: 100000, Func: "sum_over_time", Step: 25000},
			},
		},
	} {
		t.Run(tc.query, func(t *testing.T) {
			engine := NewEngine(opts)
			hintsRecorder := &noopHintRecordingQueryable{}

			var (
				query Query
				err   error
			)
			if tc.end == 0 {
				query, err = engine.NewInstantQuery(hintsRecorder, nil, tc.query, timestamp.Time(tc.start))
			} else {
				query, err = engine.NewRangeQuery(hintsRecorder, nil, tc.query, timestamp.Time(tc.start), timestamp.Time(tc.end), time.Second)
			}
			require.NoError(t, err)

			res := query.Exec(context.Background())
			require.NoError(t, res.Err)

			require.Equal(t, tc.expected, hintsRecorder.hints)
		})
	}
}

func TestEngineShutdown(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())

	block := make(chan struct{})
	processing := make(chan struct{})

	// Shutdown engine on first handler execution. Should handler execution ever become
	// concurrent this test has to be adjusted accordingly.
	f := func(ctx context.Context) error {
		processing <- struct{}{}
		<-block
		return contextDone(ctx, "test statement execution")
	}
	query1 := engine.newTestQuery(f)

	// Stopping the engine must cancel the base context. While executing queries is
	// still possible, their context is canceled from the beginning and execution should
	// terminate immediately.

	var res *Result
	go func() {
		res = query1.Exec(ctx)
		processing <- struct{}{}
	}()

	<-processing
	cancelCtx()
	block <- struct{}{}
	<-processing

	require.Error(t, res.Err, "expected error on shutdown during query but got none")
	require.Equal(t, errQueryCanceled, res.Err)

	query2 := engine.newTestQuery(func(context.Context) error {
		require.FailNow(t, "reached query execution unexpectedly")
		return nil
	})

	// The second query is started after the engine shut down. It must
	// be canceled immediately.
	res2 := query2.Exec(ctx)
	require.Error(t, res2.Err, "expected error on querying with canceled context but got none")

	var e ErrQueryCanceled
	require.True(t, errors.As(res2.Err, &e), "expected cancellation error but got: %s", res2.Err)
}

func TestEngineEvalStmtTimestamps(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric 1 2
`)
	require.NoError(t, err)
	defer test.Close()

	err = test.Run()
	require.NoError(t, err)

	cases := []struct {
		Query       string
		Result      parser.Value
		Start       time.Time
		End         time.Time
		Interval    time.Duration
		ShouldError bool
	}{
		// Instant queries.
		{
			Query:  "1",
			Result: Scalar{V: 1, T: 1000},
			Start:  time.Unix(1, 0),
		},
		{
			Query: "metric",
			Result: Vector{
				Sample{
					Point:  Point{V: 1, T: 1000},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start: time.Unix(1, 0),
		},
		{
			Query: "metric[20s]",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start: time.Unix(10, 0),
		},
		// Range queries.
		{
			Query: "1",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(10, 0),
			Interval: 5 * time.Second,
		},
		{
			Query:       `count_values("wrong label!", metric)`,
			ShouldError: true,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d query=%s", i, c.Query), func(t *testing.T) {
			var err error
			var qry Query
			if c.Interval == 0 {
				qry, err = test.QueryEngine().NewInstantQuery(test.Queryable(), nil, c.Query, c.Start)
			} else {
				qry, err = test.QueryEngine().NewRangeQuery(test.Queryable(), nil, c.Query, c.Start, c.End, c.Interval)
			}
			require.NoError(t, err)

			res := qry.Exec(test.Context())
			if c.ShouldError {
				require.Error(t, res.Err, "expected error for the query %q", c.Query)
				return
			}

			require.NoError(t, res.Err)
			require.Equal(t, c.Result, res.Value, "query %q failed", c.Query)
		})
	}
}

func TestQueryStatistics(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metricWith1SampleEvery10Seconds 1+1x100
  metricWith3SampleEvery10Seconds{a="1",b="1"} 1+1x100
  metricWith3SampleEvery10Seconds{a="2",b="2"} 1+1x100
  metricWith3SampleEvery10Seconds{a="3",b="2"} 1+1x100
`)
	require.NoError(t, err)
	defer test.Close()

	err = test.Run()
	require.NoError(t, err)

	cases := []struct {
		Query               string
		SkipMaxCheck        bool
		TotalSamples        int64
		TotalSamplesPerStep stats.TotalSamplesPerStep
		PeakSamples         int
		Start               time.Time
		End                 time.Time
		Interval            time.Duration
	}{
		{
			Query:        `"literal string"`,
			SkipMaxCheck: true, // This can't fail from a max samples limit.
			Start:        time.Unix(21, 0),
			TotalSamples: 0,
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 0,
			},
		},
		{
			Query:        "1",
			Start:        time.Unix(21, 0),
			TotalSamples: 0,
			PeakSamples:  1,
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 0,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds",
			Start:        time.Unix(21, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			// timestamp function has a special handling.
			Query:        "timestamp(metricWith1SampleEvery10Seconds)",
			Start:        time.Unix(21, 0),
			PeakSamples:  2,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds",
			Start:        time.Unix(22, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				22000: 1, // Aligned to the step time, not the sample time.
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds offset 10s",
			Start:        time.Unix(21, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds @ 15",
			Start:        time.Unix(21, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"}`,
			Start:        time.Unix(21, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"} @ 19`,
			Start:        time.Unix(21, 0),
			PeakSamples:  1,
			TotalSamples: 1, // 1 sample / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 1,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"}[20s] @ 19`,
			Start:        time.Unix(21, 0),
			PeakSamples:  2,
			TotalSamples: 2, // (1 sample / 10 seconds) * 20s
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 2,
			},
		},
		{
			Query:        "metricWith3SampleEvery10Seconds",
			Start:        time.Unix(21, 0),
			PeakSamples:  3,
			TotalSamples: 3, // 3 samples / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 3,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds[60s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  6,
			TotalSamples: 6, // 1 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 6,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[59s])[20s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  10,
			TotalSamples: 24, // (1 sample / 10 seconds * 60 seconds) * 60/5 (using 59s so we always return 6 samples
			// as if we run a query on 00 looking back 60 seconds we will return 7 samples;
			// see next test).
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 24,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[60s])[20s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  11,
			TotalSamples: 26, // (1 sample / 10 seconds * 60 seconds) + 2 as
			// max_over_time(metricWith1SampleEvery10Seconds[60s]) @ 190 and 200 will return 7 samples.
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 26,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds[60s] @ 30",
			Start:        time.Unix(201, 0),
			PeakSamples:  4,
			TotalSamples: 4, // @ modifier force the evaluation to at 30 seconds - So it brings 4 datapoints (0, 10, 20, 30 seconds) * 1 series
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 4,
			},
		},
		{
			Query:        "sum(max_over_time(metricWith3SampleEvery10Seconds[60s] @ 30))",
			Start:        time.Unix(201, 0),
			PeakSamples:  7,
			TotalSamples: 12, // @ modifier force the evaluation to at 30 seconds - So it brings 4 datapoints (0, 10, 20, 30 seconds) * 3 series
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
			},
		},
		{
			Query:        "sum by (b) (max_over_time(metricWith3SampleEvery10Seconds[60s] @ 30))",
			Start:        time.Unix(201, 0),
			PeakSamples:  8,
			TotalSamples: 12, // @ modifier force the evaluation to at 30 seconds - So it brings 4 datapoints (0, 10, 20, 30 seconds) * 3 series
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds[60s] offset 10s",
			Start:        time.Unix(201, 0),
			PeakSamples:  6,
			TotalSamples: 6, // 1 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 6,
			},
		},
		{
			Query:        "metricWith3SampleEvery10Seconds[60s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  18,
			TotalSamples: 18, // 3 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 18,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[60s])",
			Start:        time.Unix(201, 0),
			PeakSamples:  7,
			TotalSamples: 6, // 1 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 6,
			},
		},
		{
			Query:        "absent_over_time(metricWith1SampleEvery10Seconds[60s])",
			Start:        time.Unix(201, 0),
			PeakSamples:  7,
			TotalSamples: 6, // 1 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 6,
			},
		},
		{
			Query:        "max_over_time(metricWith3SampleEvery10Seconds[60s])",
			Start:        time.Unix(201, 0),
			PeakSamples:  9,
			TotalSamples: 18, // 3 sample / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 18,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds[60s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  12,
			TotalSamples: 12, // 1 sample per query * 12 queries (60/5)
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
			},
		},
		{
			Query:        "metricWith1SampleEvery10Seconds[60s:5s] offset 10s",
			Start:        time.Unix(201, 0),
			PeakSamples:  12,
			TotalSamples: 12, // 1 sample per query * 12 queries (60/5)
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
			},
		},
		{
			Query:        "max_over_time(metricWith3SampleEvery10Seconds[60s:5s])",
			Start:        time.Unix(201, 0),
			PeakSamples:  51,
			TotalSamples: 36, // 3 sample per query * 12 queries (60/5)
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 36,
			},
		},
		{
			Query:        "sum(max_over_time(metricWith3SampleEvery10Seconds[60s:5s])) + sum(max_over_time(metricWith3SampleEvery10Seconds[60s:5s]))",
			Start:        time.Unix(201, 0),
			PeakSamples:  52,
			TotalSamples: 72, // 2 * (3 sample per query * 12 queries (60/5))
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 72,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"}`,
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  4,
			TotalSamples: 4, // 1 sample per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 1,
				206000: 1,
				211000: 1,
				216000: 1,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"}`,
			Start:        time.Unix(204, 0),
			End:          time.Unix(223, 0),
			Interval:     5 * time.Second,
			PeakSamples:  4,
			TotalSamples: 4, // 1 sample per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				204000: 1, // aligned to the step time, not the sample time
				209000: 1,
				214000: 1,
				219000: 1,
			},
		},
		{
			// timestamp function as a special handling
			Query:        "timestamp(metricWith1SampleEvery10Seconds)",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  5,
			TotalSamples: 4, // (1 sample / 10 seconds) * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 1,
				206000: 1,
				211000: 1,
				216000: 1,
			},
		},
		{
			Query:        `max_over_time(metricWith3SampleEvery10Seconds{a="1"}[10s])`,
			Start:        time.Unix(991, 0),
			End:          time.Unix(1021, 0),
			Interval:     10 * time.Second,
			PeakSamples:  2,
			TotalSamples: 2, // 1 sample per query * 2 steps with data
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				991000:  1,
				1001000: 1,
				1011000: 0,
				1021000: 0,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds{a="1"} offset 10s`,
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  4,
			TotalSamples: 4, // 1 sample per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 1,
				206000: 1,
				211000: 1,
				216000: 1,
			},
		},
		{
			Query:        "max_over_time(metricWith3SampleEvery10Seconds[60s] @ 30)",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  12,
			TotalSamples: 48, // @ modifier force the evaluation timestamp at 30 seconds - So it brings 4 datapoints (0, 10, 20, 30 seconds) * 3 series * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
				206000: 12,
				211000: 12,
				216000: 12,
			},
		},
		{
			Query:        `metricWith3SampleEvery10Seconds`,
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			PeakSamples:  12,
			Interval:     5 * time.Second,
			TotalSamples: 12, // 3 sample per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 3,
				206000: 3,
				211000: 3,
				216000: 3,
			},
		},
		{
			Query:        `max_over_time(metricWith3SampleEvery10Seconds[60s])`,
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  18,
			TotalSamples: 72, // (3 sample / 10 seconds * 60 seconds) * 4 steps = 72
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 18,
				206000: 18,
				211000: 18,
				216000: 18,
			},
		},
		{
			Query:        "max_over_time(metricWith3SampleEvery10Seconds[60s:5s])",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  72,
			TotalSamples: 144, // 3 sample per query * 12 queries (60/5) * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 36,
				206000: 36,
				211000: 36,
				216000: 36,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[60s:5s])",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  32,
			TotalSamples: 48, // 1 sample per query * 12 queries (60/5) * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
				206000: 12,
				211000: 12,
				216000: 12,
			},
		},
		{
			Query:        "sum by (b) (max_over_time(metricWith1SampleEvery10Seconds[60s:5s]))",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  32,
			TotalSamples: 48, // 1 sample per query * 12 queries (60/5) * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 12,
				206000: 12,
				211000: 12,
				216000: 12,
			},
		},
		{
			Query:        "sum(max_over_time(metricWith3SampleEvery10Seconds[60s:5s])) + sum(max_over_time(metricWith3SampleEvery10Seconds[60s:5s]))",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  76,
			TotalSamples: 288, // 2 * (3 sample per query * 12 queries (60/5) * 4 steps)
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 72,
				206000: 72,
				211000: 72,
				216000: 72,
			},
		},
		{
			Query:        "sum(max_over_time(metricWith3SampleEvery10Seconds[60s:5s])) + sum(max_over_time(metricWith1SampleEvery10Seconds[60s:5s]))",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  72,
			TotalSamples: 192, // (1 sample per query * 12 queries (60/5) + 3 sample per query * 12 queries (60/5)) * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 48,
				206000: 48,
				211000: 48,
				216000: 48,
			},
		},
	}

	engine := test.QueryEngine()
	engine.enablePerStepStats = true
	origMaxSamples := engine.maxSamplesPerQuery
	for _, c := range cases {
		t.Run(c.Query, func(t *testing.T) {
			opts := &QueryOpts{EnablePerStepStats: true}
			engine.maxSamplesPerQuery = origMaxSamples

			runQuery := func(expErr error) *stats.Statistics {
				var err error
				var qry Query
				if c.Interval == 0 {
					qry, err = engine.NewInstantQuery(test.Queryable(), opts, c.Query, c.Start)
				} else {
					qry, err = engine.NewRangeQuery(test.Queryable(), opts, c.Query, c.Start, c.End, c.Interval)
				}
				require.NoError(t, err)

				res := qry.Exec(test.Context())
				require.Equal(t, expErr, res.Err)

				return qry.Stats()
			}

			stats := runQuery(nil)
			require.Equal(t, c.TotalSamples, stats.Samples.TotalSamples, "Total samples mismatch")
			require.Equal(t, &c.TotalSamplesPerStep, stats.Samples.TotalSamplesPerStepMap(), "Total samples per time mismatch")
			require.Equal(t, c.PeakSamples, stats.Samples.PeakSamples, "Peak samples mismatch")

			// Check that the peak is correct by setting the max to one less.
			if c.SkipMaxCheck {
				return
			}
			engine.maxSamplesPerQuery = stats.Samples.PeakSamples - 1
			runQuery(ErrTooManySamples(env))
		})
	}
}

func TestMaxQuerySamples(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric 1+1x100
  bigmetric{a="1"} 1+1x100
  bigmetric{a="2"} 1+1x100
`)
	require.NoError(t, err)
	defer test.Close()

	err = test.Run()
	require.NoError(t, err)

	// These test cases should be touching the limit exactly (hence no exceeding).
	// Exceeding the limit will be tested by doing -1 to the MaxSamples.
	cases := []struct {
		Query      string
		MaxSamples int
		Start      time.Time
		End        time.Time
		Interval   time.Duration
	}{
		// Instant queries.
		{
			Query:      "1",
			MaxSamples: 1,
			Start:      time.Unix(1, 0),
		},
		{
			Query:      "metric",
			MaxSamples: 1,
			Start:      time.Unix(1, 0),
		},
		{
			Query:      "metric[20s]",
			MaxSamples: 2,
			Start:      time.Unix(10, 0),
		},
		{
			Query:      "rate(metric[20s])",
			MaxSamples: 3,
			Start:      time.Unix(10, 0),
		},
		{
			Query:      "metric[20s:5s]",
			MaxSamples: 3,
			Start:      time.Unix(10, 0),
		},
		{
			Query:      "metric[20s] @ 10",
			MaxSamples: 2,
			Start:      time.Unix(0, 0),
		},
		// Range queries.
		{
			Query:      "1",
			MaxSamples: 3,
			Start:      time.Unix(0, 0),
			End:        time.Unix(2, 0),
			Interval:   time.Second,
		},
		{
			Query:      "1",
			MaxSamples: 3,
			Start:      time.Unix(0, 0),
			End:        time.Unix(2, 0),
			Interval:   time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 3,
			Start:      time.Unix(0, 0),
			End:        time.Unix(2, 0),
			Interval:   time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 3,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			Query:      "rate(bigmetric[1s])",
			MaxSamples: 1,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// Result is duplicated, so @ also produces 3 samples.
			Query:      "metric @ 10",
			MaxSamples: 3,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// The peak samples in memory is during the first evaluation:
			//   - Subquery takes 22 samples, 11 for each bigmetric,
			//   - Result is calculated per series where the series samples is buffered, hence 11 more here.
			//   - The result of two series is added before the last series buffer is discarded, so 2 more here.
			//   Hence at peak it is 22 (subquery) + 11 (buffer of a series) + 2 (result from 2 series).
			// The subquery samples and the buffer is discarded before duplicating.
			Query:      `rate(bigmetric[10s:1s] @ 10)`,
			MaxSamples: 35,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// Here the reasoning is same as above. But LHS and RHS are done one after another.
			// So while one of them takes 35 samples at peak, we need to hold the 2 sample
			// result of the other till then.
			Query:      `rate(bigmetric[10s:1s] @ 10) + rate(bigmetric[10s:1s] @ 30)`,
			MaxSamples: 37,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// Sample as above but with only 1 part as step invariant.
			// Here the peak is caused by the non-step invariant part as it touches more time range.
			// Hence at peak it is 2*21 (subquery from 0s to 20s)
			//                     + 11 (buffer of a series per evaluation)
			//                     + 6 (result from 2 series at 3 eval times).
			Query:      `rate(bigmetric[10s:1s]) + rate(bigmetric[10s:1s] @ 30)`,
			MaxSamples: 59,
			Start:      time.Unix(10, 0),
			End:        time.Unix(20, 0),
			Interval:   5 * time.Second,
		},
		{
			// Nested subquery.
			// We saw that innermost rate takes 35 samples which is still the peak
			// since the other two subqueries just duplicate the result.
			Query:      `rate(rate(bigmetric[10s:1s] @ 10)[100s:25s] @ 1000)[100s:20s] @ 2000`,
			MaxSamples: 35,
			Start:      time.Unix(10, 0),
		},
		{
			// Nested subquery.
			// Now the outmost subquery produces more samples than inner most rate.
			Query:      `rate(rate(bigmetric[10s:1s] @ 10)[100s:25s] @ 1000)[17s:1s] @ 2000`,
			MaxSamples: 36,
			Start:      time.Unix(10, 0),
		},
	}

	engine := test.QueryEngine()
	for _, c := range cases {
		t.Run(c.Query, func(t *testing.T) {
			testFunc := func(expError error) {
				var err error
				var qry Query
				if c.Interval == 0 {
					qry, err = engine.NewInstantQuery(test.Queryable(), nil, c.Query, c.Start)
				} else {
					qry, err = engine.NewRangeQuery(test.Queryable(), nil, c.Query, c.Start, c.End, c.Interval)
				}
				require.NoError(t, err)

				res := qry.Exec(test.Context())
				stats := qry.Stats()
				require.Equal(t, expError, res.Err)
				require.NotNil(t, stats)
				if expError == nil {
					require.Equal(t, c.MaxSamples, stats.Samples.PeakSamples, "peak samples mismatch for query %q", c.Query)
				}
			}

			// Within limit.
			engine.maxSamplesPerQuery = c.MaxSamples
			testFunc(nil)

			// Exceeding limit.
			engine.maxSamplesPerQuery = c.MaxSamples - 1
			testFunc(ErrTooManySamples(env))
		})
	}
}

func TestAtModifier(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric{job="1"} 0+1x1000
  metric{job="2"} 0+2x1000
  metric_topk{instance="1"} 0+1x1000
  metric_topk{instance="2"} 0+2x1000
  metric_topk{instance="3"} 1000-1x1000

load 1ms
  metric_ms 0+1x10000
`)
	require.NoError(t, err)
	defer test.Close()

	err = test.Run()
	require.NoError(t, err)

	lbls1 := labels.FromStrings("__name__", "metric", "job", "1")
	lbls2 := labels.FromStrings("__name__", "metric", "job", "2")
	lblstopk2 := labels.FromStrings("__name__", "metric_topk", "instance", "2")
	lblstopk3 := labels.FromStrings("__name__", "metric_topk", "instance", "3")
	lblsms := labels.FromStrings("__name__", "metric_ms")
	lblsneg := labels.FromStrings("__name__", "metric_neg")

	// Add some samples with negative timestamp.
	db := test.TSDB()
	app := db.Appender(context.Background())
	ref, err := app.Append(0, lblsneg, -1000000, 1000)
	require.NoError(t, err)
	for ts := int64(-1000000 + 1000); ts <= 0; ts += 1000 {
		_, err := app.Append(ref, labels.EmptyLabels(), ts, -float64(ts/1000)+1)
		require.NoError(t, err)
	}

	// To test the fix for https://github.com/prometheus/prometheus/issues/8433.
	_, err = app.Append(0, labels.FromStrings("__name__", "metric_timestamp"), 3600*1000, 1000)
	require.NoError(t, err)

	require.NoError(t, app.Commit())

	cases := []struct {
		query                string
		start, end, interval int64 // Time in seconds.
		result               parser.Value
	}{
		{ // Time of the result is the evaluation time.
			query: `metric_neg @ 0`,
			start: 100,
			result: Vector{
				Sample{Point: Point{V: 1, T: 100000}, Metric: lblsneg},
			},
		}, {
			query: `metric_neg @ -200`,
			start: 100,
			result: Vector{
				Sample{Point: Point{V: 201, T: 100000}, Metric: lblsneg},
			},
		}, {
			query: `metric{job="2"} @ 50`,
			start: -2, end: 2, interval: 1,
			result: Matrix{
				Series{
					Points: []Point{{V: 10, T: -2000}, {V: 10, T: -1000}, {V: 10, T: 0}, {V: 10, T: 1000}, {V: 10, T: 2000}},
					Metric: lbls2,
				},
			},
		}, { // Timestamps for matrix selector does not depend on the evaluation time.
			query: "metric[20s] @ 300",
			start: 10,
			result: Matrix{
				Series{
					Points: []Point{{V: 28, T: 280000}, {V: 29, T: 290000}, {V: 30, T: 300000}},
					Metric: lbls1,
				},
				Series{
					Points: []Point{{V: 56, T: 280000}, {V: 58, T: 290000}, {V: 60, T: 300000}},
					Metric: lbls2,
				},
			},
		}, {
			query: `metric_neg[2s] @ 0`,
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 3, T: -2000}, {V: 2, T: -1000}, {V: 1, T: 0}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_neg[3s] @ -500`,
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 504, T: -503000}, {V: 503, T: -502000}, {V: 502, T: -501000}, {V: 501, T: -500000}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_ms[3ms] @ 2.345`,
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 2342, T: 2342}, {V: 2343, T: 2343}, {V: 2344, T: 2344}, {V: 2345, T: 2345}},
					Metric: lblsms,
				},
			},
		}, {
			query: "metric[100s:25s] @ 300",
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 20, T: 200000}, {V: 22, T: 225000}, {V: 25, T: 250000}, {V: 27, T: 275000}, {V: 30, T: 300000}},
					Metric: lbls1,
				},
				Series{
					Points: []Point{{V: 40, T: 200000}, {V: 44, T: 225000}, {V: 50, T: 250000}, {V: 54, T: 275000}, {V: 60, T: 300000}},
					Metric: lbls2,
				},
			},
		}, {
			query: "metric_neg[50s:25s] @ 0",
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 51, T: -50000}, {V: 26, T: -25000}, {V: 1, T: 0}},
					Metric: lblsneg,
				},
			},
		}, {
			query: "metric_neg[50s:25s] @ -100",
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 151, T: -150000}, {V: 126, T: -125000}, {V: 101, T: -100000}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_ms[100ms:25ms] @ 2.345`,
			start: 100,
			result: Matrix{
				Series{
					Points: []Point{{V: 2250, T: 2250}, {V: 2275, T: 2275}, {V: 2300, T: 2300}, {V: 2325, T: 2325}},
					Metric: lblsms,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ 100))`,
			start: 50, end: 80, interval: 10,
			result: Matrix{
				Series{
					Points: []Point{{V: 995, T: 50000}, {V: 994, T: 60000}, {V: 993, T: 70000}, {V: 992, T: 80000}},
					Metric: lblstopk3,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ 5000))`,
			start: 50, end: 80, interval: 10,
			result: Matrix{
				Series{
					Points: []Point{{V: 10, T: 50000}, {V: 12, T: 60000}, {V: 14, T: 70000}, {V: 16, T: 80000}},
					Metric: lblstopk2,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ end()))`,
			start: 70, end: 100, interval: 10,
			result: Matrix{
				Series{
					Points: []Point{{V: 993, T: 70000}, {V: 992, T: 80000}, {V: 991, T: 90000}, {V: 990, T: 100000}},
					Metric: lblstopk3,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ start()))`,
			start: 100, end: 130, interval: 10,
			result: Matrix{
				Series{
					Points: []Point{{V: 990, T: 100000}, {V: 989, T: 110000}, {V: 988, T: 120000}, {V: 987, T: 130000}},
					Metric: lblstopk3,
				},
			},
		}, {
			// Tests for https://github.com/prometheus/prometheus/issues/8433.
			// The trick here is that the query range should be > lookback delta.
			query: `timestamp(metric_timestamp @ 3600)`,
			start: 0, end: 7 * 60, interval: 60,
			result: Matrix{
				Series{
					Points: []Point{
						{V: 3600, T: 0},
						{V: 3600, T: 60 * 1000},
						{V: 3600, T: 2 * 60 * 1000},
						{V: 3600, T: 3 * 60 * 1000},
						{V: 3600, T: 4 * 60 * 1000},
						{V: 3600, T: 5 * 60 * 1000},
						{V: 3600, T: 6 * 60 * 1000},
						{V: 3600, T: 7 * 60 * 1000},
					},
					Metric: labels.EmptyLabels(),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if c.interval == 0 {
				c.interval = 1
			}
			start, end, interval := time.Unix(c.start, 0), time.Unix(c.end, 0), time.Duration(c.interval)*time.Second
			var err error
			var qry Query
			if c.end == 0 {
				qry, err = test.QueryEngine().NewInstantQuery(test.Queryable(), nil, c.query, start)
			} else {
				qry, err = test.QueryEngine().NewRangeQuery(test.Queryable(), nil, c.query, start, end, interval)
			}
			require.NoError(t, err)

			res := qry.Exec(test.Context())
			require.NoError(t, res.Err)
			if expMat, ok := c.result.(Matrix); ok {
				sort.Sort(expMat)
				sort.Sort(res.Value.(Matrix))
			}
			require.Equal(t, c.result, res.Value, "query %q failed", c.query)
		})
	}
}

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var output []interface{}
	logger := log.Logger(log.LoggerFunc(func(keyvals ...interface{}) error {
		output = append(output, keyvals...)
		return nil
	}))
	ev := &evaluator{logger: logger}

	expr, _ := parser.ParseExpr("sum(up)")

	var err error

	defer func() {
		require.EqualError(t, err, "unexpected error: runtime error: index out of range [123] with length 0")
		require.Contains(t, output, "sum(up)")
	}()
	defer ev.recover(expr, nil, &err)

	// Cause a runtime panic.
	var a []int
	//nolint:govet
	a[123] = 1
}

func TestRecoverEvaluatorError(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}
	var err error

	e := errors.New("custom error")

	defer func() {
		require.EqualError(t, err, e.Error())
	}()
	defer ev.recover(nil, nil, &err)

	panic(e)
}

func TestRecoverEvaluatorErrorWithWarnings(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}
	var err error
	var ws storage.Warnings

	warnings := storage.Warnings{errors.New("custom warning")}
	e := errWithWarnings{
		err:      errors.New("custom error"),
		warnings: warnings,
	}

	defer func() {
		require.EqualError(t, err, e.Error())
		require.Equal(t, warnings, ws, "wrong warning message")
	}()
	defer ev.recover(nil, &ws, &err)

	panic(e)
}

func TestSubquerySelector(t *testing.T) {
	type caseType struct {
		Query  string
		Result Result
		Start  time.Time
	}

	for _, tst := range []struct {
		loadString string
		cases      []caseType
	}{
		{
			loadString: `load 10s
							metric 1 2`,
			cases: []caseType{
				{
					Query: "metric[20s:10s]",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 1, T: 0}, {V: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s]",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s] offset 2s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(12, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(20, 0),
				},
				{
					Query: "metric[20s:5s] offset 4s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}, {V: 2, T: 30000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 5s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}, {V: 2, T: 30000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 7s",
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
			},
		},
		{
			loadString: `load 10s
							http_requests{job="api-server", instance="0", group="production"}	0+10x1000 100+30x1000
							http_requests{job="api-server", instance="1", group="production"}	0+20x1000 200+30x1000
							http_requests{job="api-server", instance="0", group="canary"}		0+30x1000 300+80x1000
							http_requests{job="api-server", instance="1", group="canary"}		0+40x2000`,
			cases: []caseType{
				{ // Normal selector.
					Query: `http_requests{group=~"pro.*",instance="0"}[30s:10s]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 9990, T: 9990000}, {V: 10000, T: 10000000}, {V: 100, T: 10010000}, {V: 130, T: 10020000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10020, 0),
				},
				{ // Default step.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 9840, T: 9840000}, {V: 9900, T: 9900000}, {V: 9960, T: 9960000}, {V: 130, T: 10020000}, {V: 310, T: 10080000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10100, 0),
				},
				{ // Checking if high offset (>LookbackDelta) is being taken care of.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:] offset 20m`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 8640, T: 8640000}, {V: 8700, T: 8700000}, {V: 8760, T: 8760000}, {V: 8820, T: 8820000}, {V: 8880, T: 8880000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10100, 0),
				},
				{
					Query: `rate(http_requests[1m])[15s:5s]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 3, T: 7985000}, {V: 3, T: 7990000}, {V: 3, T: 7995000}, {V: 3, T: 8000000}},
								Metric: labels.FromStrings("job", "api-server", "instance", "0", "group", "canary"),
							},
							Series{
								Points: []Point{{V: 4, T: 7985000}, {V: 4, T: 7990000}, {V: 4, T: 7995000}, {V: 4, T: 8000000}},
								Metric: labels.FromStrings("job", "api-server", "instance", "1", "group", "canary"),
							},
							Series{
								Points: []Point{{V: 1, T: 7985000}, {V: 1, T: 7990000}, {V: 1, T: 7995000}, {V: 1, T: 8000000}},
								Metric: labels.FromStrings("job", "api-server", "instance", "0", "group", "production"),
							},
							Series{
								Points: []Point{{V: 2, T: 7985000}, {V: 2, T: 7990000}, {V: 2, T: 7995000}, {V: 2, T: 8000000}},
								Metric: labels.FromStrings("job", "api-server", "instance", "1", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(8000, 0),
				},
				{
					Query: `sum(http_requests{group=~"pro.*"})[30s:10s]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 270, T: 90000}, {V: 300, T: 100000}, {V: 330, T: 110000}, {V: 360, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `sum(http_requests)[40s:10s]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 800, T: 80000}, {V: 900, T: 90000}, {V: 1000, T: 100000}, {V: 1100, T: 110000}, {V: 1200, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `(sum(http_requests{group=~"p.*"})+sum(http_requests{group=~"c.*"}))[20s:5s]`,
					Result: Result{
						nil,
						Matrix{
							Series{
								Points: []Point{{V: 1000, T: 100000}, {V: 1000, T: 105000}, {V: 1100, T: 110000}, {V: 1100, T: 115000}, {V: 1200, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			test, err := NewTest(t, tst.loadString)
			require.NoError(t, err)
			defer test.Close()

			require.NoError(t, test.Run())
			engine := test.QueryEngine()
			for _, c := range tst.cases {
				t.Run(c.Query, func(t *testing.T) {
					qry, err := engine.NewInstantQuery(test.Queryable(), nil, c.Query, c.Start)
					require.NoError(t, err)

					res := qry.Exec(test.Context())
					require.Equal(t, c.Result.Err, res.Err)
					mat := res.Value.(Matrix)
					sort.Sort(mat)
					require.Equal(t, c.Result.Value, mat)
				})
			}
		})
	}
}

type FakeQueryLogger struct {
	closed bool
	logs   []interface{}
}

func NewFakeQueryLogger() *FakeQueryLogger {
	return &FakeQueryLogger{
		closed: false,
		logs:   make([]interface{}, 0),
	}
}

func (f *FakeQueryLogger) Close() error {
	f.closed = true
	return nil
}

func (f *FakeQueryLogger) Log(l ...interface{}) error {
	f.logs = append(f.logs, l...)
	return nil
}

func TestQueryLogger_basic(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)

	queryExec := func() {
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		query := engine.newTestQuery(func(ctx context.Context) error {
			return contextDone(ctx, "test statement execution")
		})
		res := query.Exec(ctx)
		require.NoError(t, res.Err)
	}

	// Query works without query log initialized.
	queryExec()

	f1 := NewFakeQueryLogger()
	engine.SetQueryLogger(f1)
	queryExec()
	for i, field := range []interface{}{"params", map[string]interface{}{"query": "test statement"}} {
		require.Equal(t, field, f1.logs[i])
	}

	l := len(f1.logs)
	queryExec()
	require.Equal(t, 2*l, len(f1.logs))

	// Test that we close the query logger when unsetting it.
	require.False(t, f1.closed, "expected f1 to be open, got closed")
	engine.SetQueryLogger(nil)
	require.True(t, f1.closed, "expected f1 to be closed, got open")
	queryExec()

	// Test that we close the query logger when swapping.
	f2 := NewFakeQueryLogger()
	f3 := NewFakeQueryLogger()
	engine.SetQueryLogger(f2)
	require.False(t, f2.closed, "expected f2 to be open, got closed")
	queryExec()
	engine.SetQueryLogger(f3)
	require.True(t, f2.closed, "expected f2 to be closed, got open")
	require.False(t, f3.closed, "expected f3 to be open, got closed")
	queryExec()
}

func TestQueryLogger_fields(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)

	f1 := NewFakeQueryLogger()
	engine.SetQueryLogger(f1)

	ctx, cancelCtx := context.WithCancel(context.Background())
	ctx = NewOriginContext(ctx, map[string]interface{}{"foo": "bar"})
	defer cancelCtx()
	query := engine.newTestQuery(func(ctx context.Context) error {
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec(ctx)
	require.NoError(t, res.Err)

	expected := []string{"foo", "bar"}
	for i, field := range expected {
		v := f1.logs[len(f1.logs)-len(expected)+i].(string)
		require.Equal(t, field, v)
	}
}

func TestQueryLogger_error(t *testing.T) {
	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)

	f1 := NewFakeQueryLogger()
	engine.SetQueryLogger(f1)

	ctx, cancelCtx := context.WithCancel(context.Background())
	ctx = NewOriginContext(ctx, map[string]interface{}{"foo": "bar"})
	defer cancelCtx()
	testErr := errors.New("failure")
	query := engine.newTestQuery(func(ctx context.Context) error {
		return testErr
	})

	res := query.Exec(ctx)
	require.Error(t, res.Err, "query should have failed")

	for i, field := range []interface{}{"params", map[string]interface{}{"query": "test statement"}, "error", testErr} {
		require.Equal(t, f1.logs[i], field)
	}
}

func TestPreprocessAndWrapWithStepInvariantExpr(t *testing.T) {
	startTime := time.Unix(1000, 0)
	endTime := time.Unix(9999, 0)
	testCases := []struct {
		input      string      // The input to be parsed.
		expected   parser.Expr // The expected expression AST.
		outputTest bool
	}{
		{
			input: "123.4567",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.NumberLiteral{
					Val:      123.4567,
					PosRange: parser.PositionRange{Start: 0, End: 8},
				},
			},
		},
		{
			input: `"foo"`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.StringLiteral{
					Val:      "foo",
					PosRange: parser.PositionRange{Start: 0, End: 5},
				},
			},
		},
		{
			input: "foo * bar",
			expected: &parser.BinaryExpr{
				Op: parser.MUL,
				LHS: &parser.VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   3,
					},
				},
				RHS: &parser.VectorSelector{
					Name: "bar",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "bar"),
					},
					PosRange: parser.PositionRange{
						Start: 6,
						End:   9,
					},
				},
				VectorMatching: &parser.VectorMatching{Card: parser.CardOneToOne},
			},
		},
		{
			input: "foo * bar @ 10",
			expected: &parser.BinaryExpr{
				Op: parser.MUL,
				LHS: &parser.VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   3,
					},
				},
				RHS: &parser.StepInvariantExpr{
					Expr: &parser.VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "bar"),
						},
						PosRange: parser.PositionRange{
							Start: 6,
							End:   14,
						},
						Timestamp: makeInt64Pointer(10000),
					},
				},
				VectorMatching: &parser.VectorMatching{Card: parser.CardOneToOne},
			},
		},
		{
			input: "foo @ 20 * bar @ 10",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.BinaryExpr{
					Op: parser.MUL,
					LHS: &parser.VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   8,
						},
						Timestamp: makeInt64Pointer(20000),
					},
					RHS: &parser.VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "bar"),
						},
						PosRange: parser.PositionRange{
							Start: 11,
							End:   19,
						},
						Timestamp: makeInt64Pointer(10000),
					},
					VectorMatching: &parser.VectorMatching{Card: parser.CardOneToOne},
				},
			},
		},
		{
			input: "test[5s]",
			expected: &parser.MatrixSelector{
				VectorSelector: &parser.VectorSelector{
					Name: "test",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   4,
					},
				},
				Range:  5 * time.Second,
				EndPos: 8,
			},
		},
		{
			input: `test{a="b"}[5y] @ 1603774699`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.MatrixSelector{
					VectorSelector: &parser.VectorSelector{
						Name:      "test",
						Timestamp: makeInt64Pointer(1603774699000),
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "a", "b"),
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   11,
						},
					},
					Range:  5 * 365 * 24 * time.Hour,
					EndPos: 28,
				},
			},
		},
		{
			input: "sum by (foo)(some_metric)",
			expected: &parser.AggregateExpr{
				Op: parser.SUM,
				Expr: &parser.VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
					},
					PosRange: parser.PositionRange{
						Start: 13,
						End:   24,
					},
				},
				Grouping: []string{"foo"},
				PosRange: parser.PositionRange{
					Start: 0,
					End:   25,
				},
			},
		},
		{
			input: "sum by (foo)(some_metric @ 10)",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.AggregateExpr{
					Op: parser.SUM,
					Expr: &parser.VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
						},
						PosRange: parser.PositionRange{
							Start: 13,
							End:   29,
						},
						Timestamp: makeInt64Pointer(10000),
					},
					Grouping: []string{"foo"},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   30,
					},
				},
			},
		},
		{
			input: "sum(some_metric1 @ 10) + sum(some_metric2 @ 20)",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.BinaryExpr{
					Op:             parser.ADD,
					VectorMatching: &parser.VectorMatching{},
					LHS: &parser.AggregateExpr{
						Op: parser.SUM,
						Expr: &parser.VectorSelector{
							Name: "some_metric1",
							LabelMatchers: []*labels.Matcher{
								parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric1"),
							},
							PosRange: parser.PositionRange{
								Start: 4,
								End:   21,
							},
							Timestamp: makeInt64Pointer(10000),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   22,
						},
					},
					RHS: &parser.AggregateExpr{
						Op: parser.SUM,
						Expr: &parser.VectorSelector{
							Name: "some_metric2",
							LabelMatchers: []*labels.Matcher{
								parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric2"),
							},
							PosRange: parser.PositionRange{
								Start: 29,
								End:   46,
							},
							Timestamp: makeInt64Pointer(20000),
						},
						PosRange: parser.PositionRange{
							Start: 25,
							End:   47,
						},
					},
				},
			},
		},
		{
			input: "some_metric and topk(5, rate(some_metric[1m] @ 20))",
			expected: &parser.BinaryExpr{
				Op: parser.LAND,
				VectorMatching: &parser.VectorMatching{
					Card: parser.CardManyToMany,
				},
				LHS: &parser.VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   11,
					},
				},
				RHS: &parser.StepInvariantExpr{
					Expr: &parser.AggregateExpr{
						Op: parser.TOPK,
						Expr: &parser.Call{
							Func: parser.MustGetFunction("rate"),
							Args: parser.Expressions{
								&parser.MatrixSelector{
									VectorSelector: &parser.VectorSelector{
										Name: "some_metric",
										LabelMatchers: []*labels.Matcher{
											parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
										},
										PosRange: parser.PositionRange{
											Start: 29,
											End:   40,
										},
										Timestamp: makeInt64Pointer(20000),
									},
									Range:  1 * time.Minute,
									EndPos: 49,
								},
							},
							PosRange: parser.PositionRange{
								Start: 24,
								End:   50,
							},
						},
						Param: &parser.NumberLiteral{
							Val: 5,
							PosRange: parser.PositionRange{
								Start: 21,
								End:   22,
							},
						},
						PosRange: parser.PositionRange{
							Start: 16,
							End:   51,
						},
					},
				},
			},
		},
		{
			input: "time()",
			expected: &parser.Call{
				Func: parser.MustGetFunction("time"),
				Args: parser.Expressions{},
				PosRange: parser.PositionRange{
					Start: 0,
					End:   6,
				},
			},
		},
		{
			input: `foo{bar="baz"}[10m:6s]`,
			expected: &parser.SubqueryExpr{
				Expr: &parser.VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   14,
					},
				},
				Range:  10 * time.Minute,
				Step:   6 * time.Second,
				EndPos: 22,
			},
		},
		{
			input: `foo{bar="baz"}[10m:6s] @ 10`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   14,
						},
					},
					Range:     10 * time.Minute,
					Step:      6 * time.Second,
					Timestamp: makeInt64Pointer(10000),
					EndPos:    27,
				},
			},
		},
		{ // Even though the subquery is step invariant, the inside is also wrapped separately.
			input: `sum(foo{bar="baz"} @ 20)[10m:6s] @ 10`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.StepInvariantExpr{
						Expr: &parser.AggregateExpr{
							Op: parser.SUM,
							Expr: &parser.VectorSelector{
								Name: "foo",
								LabelMatchers: []*labels.Matcher{
									parser.MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
									parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
								},
								PosRange: parser.PositionRange{
									Start: 4,
									End:   23,
								},
								Timestamp: makeInt64Pointer(20000),
							},
							PosRange: parser.PositionRange{
								Start: 0,
								End:   24,
							},
						},
					},
					Range:     10 * time.Minute,
					Step:      6 * time.Second,
					Timestamp: makeInt64Pointer(10000),
					EndPos:    37,
				},
			},
		},
		{
			input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] @ 1603775091)[4m:3s]`,
			expected: &parser.SubqueryExpr{
				Expr: &parser.StepInvariantExpr{
					Expr: &parser.Call{
						Func: parser.MustGetFunction("min_over_time"),
						Args: parser.Expressions{
							&parser.SubqueryExpr{
								Expr: &parser.Call{
									Func: parser.MustGetFunction("rate"),
									Args: parser.Expressions{
										&parser.MatrixSelector{
											VectorSelector: &parser.VectorSelector{
												Name: "foo",
												LabelMatchers: []*labels.Matcher{
													parser.MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
													parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
												},
												PosRange: parser.PositionRange{
													Start: 19,
													End:   33,
												},
											},
											Range:  2 * time.Second,
											EndPos: 37,
										},
									},
									PosRange: parser.PositionRange{
										Start: 14,
										End:   38,
									},
								},
								Range:     5 * time.Minute,
								Timestamp: makeInt64Pointer(1603775091000),
								EndPos:    56,
							},
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   57,
						},
					},
				},
				Range:  4 * time.Minute,
				Step:   3 * time.Second,
				EndPos: 64,
			},
		},
		{
			input: `some_metric @ 123 offset 1m [10m:5s]`,
			expected: &parser.SubqueryExpr{
				Expr: &parser.StepInvariantExpr{
					Expr: &parser.VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   27,
						},
						Timestamp:      makeInt64Pointer(123000),
						OriginalOffset: 1 * time.Minute,
					},
				},
				Range:  10 * time.Minute,
				Step:   5 * time.Second,
				EndPos: 36,
			},
		},
		{
			input: `some_metric[10m:5s] offset 1m @ 123`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   11,
						},
					},
					Timestamp:      makeInt64Pointer(123000),
					OriginalOffset: 1 * time.Minute,
					Range:          10 * time.Minute,
					Step:           5 * time.Second,
					EndPos:         35,
				},
			},
		},
		{
			input: `(foo + bar{nm="val"} @ 1234)[5m:] @ 1603775019`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.ParenExpr{
						Expr: &parser.BinaryExpr{
							Op: parser.ADD,
							VectorMatching: &parser.VectorMatching{
								Card: parser.CardOneToOne,
							},
							LHS: &parser.VectorSelector{
								Name: "foo",
								LabelMatchers: []*labels.Matcher{
									parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
								},
								PosRange: parser.PositionRange{
									Start: 1,
									End:   4,
								},
							},
							RHS: &parser.StepInvariantExpr{
								Expr: &parser.VectorSelector{
									Name: "bar",
									LabelMatchers: []*labels.Matcher{
										parser.MustLabelMatcher(labels.MatchEqual, "nm", "val"),
										parser.MustLabelMatcher(labels.MatchEqual, "__name__", "bar"),
									},
									Timestamp: makeInt64Pointer(1234000),
									PosRange: parser.PositionRange{
										Start: 7,
										End:   27,
									},
								},
							},
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   28,
						},
					},
					Range:     5 * time.Minute,
					Timestamp: makeInt64Pointer(1603775019000),
					EndPos:    46,
				},
			},
		},
		{
			input: "abs(abs(metric @ 10))",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.Call{
					Func: &parser.Function{
						Name:       "abs",
						ArgTypes:   []parser.ValueType{parser.ValueTypeVector},
						ReturnType: parser.ValueTypeVector,
					},
					Args: parser.Expressions{&parser.Call{
						Func: &parser.Function{
							Name:       "abs",
							ArgTypes:   []parser.ValueType{parser.ValueTypeVector},
							ReturnType: parser.ValueTypeVector,
						},
						Args: parser.Expressions{&parser.VectorSelector{
							Name: "metric",
							LabelMatchers: []*labels.Matcher{
								parser.MustLabelMatcher(labels.MatchEqual, "__name__", "metric"),
							},
							PosRange: parser.PositionRange{
								Start: 8,
								End:   19,
							},
							Timestamp: makeInt64Pointer(10000),
						}},
						PosRange: parser.PositionRange{
							Start: 4,
							End:   20,
						},
					}},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   21,
					},
				},
			},
		},
		{
			input: "sum(sum(some_metric1 @ 10) + sum(some_metric2 @ 20))",
			expected: &parser.StepInvariantExpr{
				Expr: &parser.AggregateExpr{
					Op: parser.SUM,
					Expr: &parser.BinaryExpr{
						Op:             parser.ADD,
						VectorMatching: &parser.VectorMatching{},
						LHS: &parser.AggregateExpr{
							Op: parser.SUM,
							Expr: &parser.VectorSelector{
								Name: "some_metric1",
								LabelMatchers: []*labels.Matcher{
									parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric1"),
								},
								PosRange: parser.PositionRange{
									Start: 8,
									End:   25,
								},
								Timestamp: makeInt64Pointer(10000),
							},
							PosRange: parser.PositionRange{
								Start: 4,
								End:   26,
							},
						},
						RHS: &parser.AggregateExpr{
							Op: parser.SUM,
							Expr: &parser.VectorSelector{
								Name: "some_metric2",
								LabelMatchers: []*labels.Matcher{
									parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric2"),
								},
								PosRange: parser.PositionRange{
									Start: 33,
									End:   50,
								},
								Timestamp: makeInt64Pointer(20000),
							},
							PosRange: parser.PositionRange{
								Start: 29,
								End:   52,
							},
						},
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   52,
					},
				},
			},
		},
		{
			input: `foo @ start()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   13,
					},
					Timestamp:  makeInt64Pointer(timestamp.FromTime(startTime)),
					StartOrEnd: parser.START,
				},
			},
		},
		{
			input: `foo @ end()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "foo"),
					},
					PosRange: parser.PositionRange{
						Start: 0,
						End:   11,
					},
					Timestamp:  makeInt64Pointer(timestamp.FromTime(endTime)),
					StartOrEnd: parser.END,
				},
			},
		},
		{
			input: `test[5y] @ start()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.MatrixSelector{
					VectorSelector: &parser.VectorSelector{
						Name:       "test",
						Timestamp:  makeInt64Pointer(timestamp.FromTime(startTime)),
						StartOrEnd: parser.START,
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   4,
						},
					},
					Range:  5 * 365 * 24 * time.Hour,
					EndPos: 18,
				},
			},
		},
		{
			input: `test[5y] @ end()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.MatrixSelector{
					VectorSelector: &parser.VectorSelector{
						Name:       "test",
						Timestamp:  makeInt64Pointer(timestamp.FromTime(endTime)),
						StartOrEnd: parser.END,
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   4,
						},
					},
					Range:  5 * 365 * 24 * time.Hour,
					EndPos: 16,
				},
			},
		},
		{
			input: `some_metric[10m:5s] @ start()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   11,
						},
					},
					Timestamp:  makeInt64Pointer(timestamp.FromTime(startTime)),
					StartOrEnd: parser.START,
					Range:      10 * time.Minute,
					Step:       5 * time.Second,
					EndPos:     29,
				},
			},
		},
		{
			input: `some_metric[10m:5s] @ end()`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.SubqueryExpr{
					Expr: &parser.VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
						},
						PosRange: parser.PositionRange{
							Start: 0,
							End:   11,
						},
					},
					Timestamp:  makeInt64Pointer(timestamp.FromTime(endTime)),
					StartOrEnd: parser.END,
					Range:      10 * time.Minute,
					Step:       5 * time.Second,
					EndPos:     27,
				},
			},
		},
		{
			input:      `floor(some_metric / (3 * 1024))`,
			outputTest: true,
			expected: &parser.Call{
				Func: &parser.Function{
					Name:       "floor",
					ArgTypes:   []parser.ValueType{parser.ValueTypeVector},
					ReturnType: parser.ValueTypeVector,
				},
				Args: parser.Expressions{
					&parser.BinaryExpr{
						Op: parser.DIV,
						LHS: &parser.VectorSelector{
							Name: "some_metric",
							LabelMatchers: []*labels.Matcher{
								parser.MustLabelMatcher(labels.MatchEqual, "__name__", "some_metric"),
							},
							PosRange: parser.PositionRange{
								Start: 6,
								End:   17,
							},
						},
						RHS: &parser.StepInvariantExpr{
							Expr: &parser.ParenExpr{
								Expr: &parser.BinaryExpr{
									Op: parser.MUL,
									LHS: &parser.NumberLiteral{
										Val: 3,
										PosRange: parser.PositionRange{
											Start: 21,
											End:   22,
										},
									},
									RHS: &parser.NumberLiteral{
										Val: 1024,
										PosRange: parser.PositionRange{
											Start: 25,
											End:   29,
										},
									},
								},
								PosRange: parser.PositionRange{
									Start: 20,
									End:   30,
								},
							},
						},
					},
				},
				PosRange: parser.PositionRange{
					Start: 0,
					End:   31,
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.input, func(t *testing.T) {
			expr, err := parser.ParseExpr(test.input)
			require.NoError(t, err)
			expr = PreprocessExpr(expr, startTime, endTime)
			if test.outputTest {
				require.Equal(t, test.input, expr.String(), "error on input '%s'", test.input)
			}
			require.Equal(t, test.expected, expr, "error on input '%s'", test.input)
		})
	}
}

func TestEngineOptsValidation(t *testing.T) {
	cases := []struct {
		opts     EngineOpts
		query    string
		fail     bool
		expError error
	}{
		{
			opts:  EngineOpts{EnableAtModifier: false},
			query: "metric @ 100", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ 100)", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ 100)", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "metric @ start()", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ start())", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ start())", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "metric @ end()", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ end())", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ end())", fail: true, expError: ErrValidationAtModifierDisabled,
		}, {
			opts:  EngineOpts{EnableAtModifier: true},
			query: "metric @ 100",
		}, {
			opts:  EngineOpts{EnableAtModifier: true},
			query: "rate(metric[1m] @ start())",
		}, {
			opts:  EngineOpts{EnableAtModifier: true},
			query: "rate(metric[1h:1m] @ end())",
		}, {
			opts:  EngineOpts{EnableNegativeOffset: false},
			query: "metric offset -1s", fail: true, expError: ErrValidationNegativeOffsetDisabled,
		}, {
			opts:  EngineOpts{EnableNegativeOffset: true},
			query: "metric offset -1s",
		}, {
			opts:  EngineOpts{EnableAtModifier: true, EnableNegativeOffset: true},
			query: "metric @ 100 offset -2m",
		}, {
			opts:  EngineOpts{EnableAtModifier: true, EnableNegativeOffset: true},
			query: "metric offset -2m @ 100",
		},
	}

	for _, c := range cases {
		eng := NewEngine(c.opts)
		_, err1 := eng.NewInstantQuery(nil, nil, c.query, time.Unix(10, 0))
		_, err2 := eng.NewRangeQuery(nil, nil, c.query, time.Unix(0, 0), time.Unix(10, 0), time.Second)
		if c.fail {
			require.Equal(t, c.expError, err1)
			require.Equal(t, c.expError, err2)
		} else {
			require.Nil(t, err1)
			require.Nil(t, err2)
		}
	}
}

func TestRangeQuery(t *testing.T) {
	cases := []struct {
		Name     string
		Load     string
		Query    string
		Result   parser.Value
		Start    time.Time
		End      time.Time
		Interval time.Duration
	}{
		{
			Name: "sum_over_time with all values",
			Load: `load 30s
              bar 0 1 10 100 1000`,
			Query: "sum_over_time(bar[30s])",
			Result: Matrix{
				Series{
					Points: []Point{{V: 0, T: 0}, {V: 11, T: 60000}, {V: 1100, T: 120000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 60 * time.Second,
		},
		{
			Name: "sum_over_time with trailing values",
			Load: `load 30s
              bar 0 1 10 100 1000 0 0 0 0`,
			Query: "sum_over_time(bar[30s])",
			Result: Matrix{
				Series{
					Points: []Point{{V: 0, T: 0}, {V: 11, T: 60000}, {V: 1100, T: 120000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 60 * time.Second,
		},
		{
			Name: "sum_over_time with all values long",
			Load: `load 30s
              bar 0 1 10 100 1000 10000 100000 1000000 10000000`,
			Query: "sum_over_time(bar[30s])",
			Result: Matrix{
				Series{
					Points: []Point{{V: 0, T: 0}, {V: 11, T: 60000}, {V: 1100, T: 120000}, {V: 110000, T: 180000}, {V: 11000000, T: 240000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(240, 0),
			Interval: 60 * time.Second,
		},
		{
			Name: "sum_over_time with all values random",
			Load: `load 30s
              bar 5 17 42 2 7 905 51`,
			Query: "sum_over_time(bar[30s])",
			Result: Matrix{
				Series{
					Points: []Point{{V: 5, T: 0}, {V: 59, T: 60000}, {V: 9, T: 120000}, {V: 956, T: 180000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(180, 0),
			Interval: 60 * time.Second,
		},
		{
			Name: "metric query",
			Load: `load 30s
              metric 1+1x4`,
			Query: "metric",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 1 * time.Minute,
		},
		{
			Name: "metric query with trailing values",
			Load: `load 30s
              metric 1+1x8`,
			Query: "metric",
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 1 * time.Minute,
		},
		{
			Name: "short-circuit",
			Load: `load 30s
							foo{job="1"} 1+1x4
							bar{job="2"} 1+1x4`,
			Query: `foo > 2 or bar`,
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings(
						"__name__", "bar",
						"job", "2",
					),
				},
				Series{
					Points: []Point{{V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings(
						"__name__", "foo",
						"job", "1",
					),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 1 * time.Minute,
		},
		{
			Name: "short-circuit",
			Load: `load 30s
							foo{job="1"} 1+1x4
							bar{job="2"} 1+1x4`,
			Query: `foo > 2 or bar`,
			Result: Matrix{
				Series{
					Points: []Point{{V: 1, T: 0}, {V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings("__name__", "bar", "job", "2"),
				},
				Series{
					Points: []Point{{V: 3, T: 60000}, {V: 5, T: 120000}},
					Metric: labels.FromStrings("__name__", "foo", "job", "1"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(120, 0),
			Interval: 1 * time.Minute,
		},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			test, err := NewTest(t, c.Load)
			require.NoError(t, err)
			defer test.Close()

			err = test.Run()
			require.NoError(t, err)

			qry, err := test.QueryEngine().NewRangeQuery(test.Queryable(), nil, c.Query, c.Start, c.End, c.Interval)
			require.NoError(t, err)

			res := qry.Exec(test.Context())
			require.NoError(t, res.Err)
			require.Equal(t, c.Result, res.Value)
		})
	}
}

func TestNativeHistogramRate(t *testing.T) {
	// TODO(beorn7): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	test, err := NewTest(t, "")
	require.NoError(t, err)
	defer test.Close()

	seriesName := "sparse_histogram_series"
	lbls := labels.FromStrings("__name__", seriesName)

	app := test.Storage().Appender(context.TODO())
	for i, h := range tsdbutil.GenerateTestHistograms(100) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(15*time.Second/time.Millisecond), h, nil)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	require.NoError(t, test.Run())
	engine := test.QueryEngine()

	queryString := fmt.Sprintf("rate(%s[1m])", seriesName)
	qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(int64(5*time.Minute/time.Millisecond)))
	require.NoError(t, err)
	res := qry.Exec(test.Context())
	require.NoError(t, res.Err)
	vector, err := res.Vector()
	require.NoError(t, err)
	require.Len(t, vector, 1)
	actualHistogram := vector[0].H
	expectedHistogram := &histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           1,
		ZeroThreshold:    0.001,
		ZeroCount:        1. / 15.,
		Count:            8. / 15.,
		Sum:              1.226666666666667,
		PositiveSpans:    []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		PositiveBuckets:  []float64{1. / 15., 1. / 15., 1. / 15., 1. / 15.},
		NegativeSpans:    []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		NegativeBuckets:  []float64{1. / 15., 1. / 15., 1. / 15., 1. / 15.},
	}
	require.Equal(t, expectedHistogram, actualHistogram)
}

func TestNativeFloatHistogramRate(t *testing.T) {
	// TODO(beorn7): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	test, err := NewTest(t, "")
	require.NoError(t, err)
	defer test.Close()

	seriesName := "sparse_histogram_series"
	lbls := labels.FromStrings("__name__", seriesName)

	app := test.Storage().Appender(context.TODO())
	for i, fh := range tsdbutil.GenerateTestFloatHistograms(100) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(15*time.Second/time.Millisecond), nil, fh)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	require.NoError(t, test.Run())
	engine := test.QueryEngine()

	queryString := fmt.Sprintf("rate(%s[1m])", seriesName)
	qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(int64(5*time.Minute/time.Millisecond)))
	require.NoError(t, err)
	res := qry.Exec(test.Context())
	require.NoError(t, res.Err)
	vector, err := res.Vector()
	require.NoError(t, err)
	require.Len(t, vector, 1)
	actualHistogram := vector[0].H
	expectedHistogram := &histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           1,
		ZeroThreshold:    0.001,
		ZeroCount:        1. / 15.,
		Count:            8. / 15.,
		Sum:              1.226666666666667,
		PositiveSpans:    []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		PositiveBuckets:  []float64{1. / 15., 1. / 15., 1. / 15., 1. / 15.},
		NegativeSpans:    []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		NegativeBuckets:  []float64{1. / 15., 1. / 15., 1. / 15., 1. / 15.},
	}
	require.Equal(t, expectedHistogram, actualHistogram)
}

func TestNativeHistogram_HistogramCountAndSum(t *testing.T) {
	// TODO(codesome): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	h := &histogram.Histogram{
		Count:         24,
		ZeroCount:     4,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -2, 3},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{2, 1, -2, 3},
	}
	for _, floatHisto := range []bool{true, false} {
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
			test, err := NewTest(t, "")
			require.NoError(t, err)
			t.Cleanup(test.Close)

			seriesName := "sparse_histogram_series"
			lbls := labels.FromStrings("__name__", seriesName)
			engine := test.QueryEngine()

			ts := int64(10 * time.Minute / time.Millisecond)
			app := test.Storage().Appender(context.TODO())
			if floatHisto {
				_, err = app.AppendHistogram(0, lbls, ts, nil, h.ToFloat())
			} else {
				_, err = app.AppendHistogram(0, lbls, ts, h, nil)
			}
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			queryString := fmt.Sprintf("histogram_count(%s)", seriesName)
			qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(ts))
			require.NoError(t, err)

			res := qry.Exec(test.Context())
			require.NoError(t, res.Err)

			vector, err := res.Vector()
			require.NoError(t, err)

			require.Len(t, vector, 1)
			require.Nil(t, vector[0].H)
			if floatHisto {
				require.Equal(t, float64(h.ToFloat().Count), vector[0].V)
			} else {
				require.Equal(t, float64(h.Count), vector[0].V)
			}

			queryString = fmt.Sprintf("histogram_sum(%s)", seriesName)
			qry, err = engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(ts))
			require.NoError(t, err)

			res = qry.Exec(test.Context())
			require.NoError(t, res.Err)

			vector, err = res.Vector()
			require.NoError(t, err)

			require.Len(t, vector, 1)
			require.Nil(t, vector[0].H)
			if floatHisto {
				require.Equal(t, h.ToFloat().Sum, vector[0].V)
			} else {
				require.Equal(t, h.Sum, vector[0].V)
			}
		})
	}
}

func TestNativeHistogram_HistogramQuantile(t *testing.T) {
	// TODO(codesome): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	type subCase struct {
		quantile string
		value    float64
	}

	cases := []struct {
		text string
		// Histogram to test.
		h *histogram.Histogram
		// Different quantiles to test for this histogram.
		subCases []subCase
	}{
		{
			text: "all positive buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{2, 1, -2, 3},
			},
			subCases: []subCase{
				{
					quantile: "1.0001",
					value:    math.Inf(1),
				},
				{
					quantile: "1",
					value:    16,
				},
				{
					quantile: "0.99",
					value:    15.759999999999998,
				},
				{
					quantile: "0.9",
					value:    13.600000000000001,
				},
				{
					quantile: "0.6",
					value:    4.799999999999997,
				},
				{
					quantile: "0.5",
					value:    1.6666666666666665,
				},
				{ // Zero bucket.
					quantile: "0.1",
					value:    0.0006000000000000001,
				},
				{
					quantile: "0",
					value:    0,
				},
				{
					quantile: "-1",
					value:    math.Inf(-1),
				},
			},
		},
		{
			text: "all negative buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{2, 1, -2, 3},
			},
			subCases: []subCase{
				{
					quantile: "1.0001",
					value:    math.Inf(1),
				},
				{ // Zero bucket.
					quantile: "1",
					value:    0,
				},
				{ // Zero bucket.
					quantile: "0.99",
					value:    -6.000000000000048e-05,
				},
				{ // Zero bucket.
					quantile: "0.9",
					value:    -0.0005999999999999996,
				},
				{
					quantile: "0.5",
					value:    -1.6666666666666667,
				},
				{
					quantile: "0.1",
					value:    -13.6,
				},
				{
					quantile: "0",
					value:    -16,
				},
				{
					quantile: "-1",
					value:    math.Inf(-1),
				},
			},
		},
		{
			text: "both positive and negative buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         24,
				ZeroCount:     4,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{2, 1, -2, 3},
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{2, 1, -2, 3},
			},
			subCases: []subCase{
				{
					quantile: "1.0001",
					value:    math.Inf(1),
				},
				{
					quantile: "1",
					value:    16,
				},
				{
					quantile: "0.99",
					value:    15.519999999999996,
				},
				{
					quantile: "0.9",
					value:    11.200000000000003,
				},
				{
					quantile: "0.7",
					value:    1.2666666666666657,
				},
				{ // Zero bucket.
					quantile: "0.55",
					value:    0.0006000000000000005,
				},
				{ // Zero bucket.
					quantile: "0.5",
					value:    0,
				},
				{ // Zero bucket.
					quantile: "0.45",
					value:    -0.0005999999999999996,
				},
				{
					quantile: "0.3",
					value:    -1.266666666666667,
				},
				{
					quantile: "0.1",
					value:    -11.2,
				},
				{
					quantile: "0.01",
					value:    -15.52,
				},
				{
					quantile: "0",
					value:    -16,
				},
				{
					quantile: "-1",
					value:    math.Inf(-1),
				},
			},
		},
	}

	test, err := NewTest(t, "")
	require.NoError(t, err)
	t.Cleanup(test.Close)
	idx := int64(0)
	for _, floatHisto := range []bool{true, false} {
		for _, c := range cases {
			t.Run(fmt.Sprintf("%s floatHistogram=%t", c.text, floatHisto), func(t *testing.T) {
				seriesName := "sparse_histogram_series"
				lbls := labels.FromStrings("__name__", seriesName)
				engine := test.QueryEngine()
				ts := idx * int64(10*time.Minute/time.Millisecond)
				app := test.Storage().Appender(context.TODO())
				if floatHisto {
					_, err = app.AppendHistogram(0, lbls, ts, nil, c.h.ToFloat())
				} else {
					_, err = app.AppendHistogram(0, lbls, ts, c.h, nil)
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				for j, sc := range c.subCases {
					t.Run(fmt.Sprintf("%d %s", j, sc.quantile), func(t *testing.T) {
						queryString := fmt.Sprintf("histogram_quantile(%s, %s)", sc.quantile, seriesName)
						qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(ts))
						require.NoError(t, err)

						res := qry.Exec(test.Context())
						require.NoError(t, res.Err)

						vector, err := res.Vector()
						require.NoError(t, err)

						require.Len(t, vector, 1)
						require.Nil(t, vector[0].H)
						require.True(t, almostEqual(sc.value, vector[0].V))
					})
				}
				idx++
			})
		}
	}
}

func TestNativeHistogram_HistogramFraction(t *testing.T) {
	// TODO(codesome): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	type subCase struct {
		lower, upper string
		value        float64
	}

	invariantCases := []subCase{
		{
			lower: "42",
			upper: "3.1415",
			value: 0,
		},
		{
			lower: "0",
			upper: "0",
			value: 0,
		},
		{
			lower: "0.000001",
			upper: "0.000001",
			value: 0,
		},
		{
			lower: "42",
			upper: "42",
			value: 0,
		},
		{
			lower: "-3.1",
			upper: "-3.1",
			value: 0,
		},
		{
			lower: "3.1415",
			upper: "NaN",
			value: math.NaN(),
		},
		{
			lower: "NaN",
			upper: "42",
			value: math.NaN(),
		},
		{
			lower: "NaN",
			upper: "NaN",
			value: math.NaN(),
		},
		{
			lower: "-Inf",
			upper: "+Inf",
			value: 1,
		},
	}

	cases := []struct {
		text string
		// Histogram to test.
		h *histogram.Histogram
		// Different ranges to test for this histogram.
		subCases []subCase
	}{
		{
			text: "empty histogram",
			h:    &histogram.Histogram{},
			subCases: []subCase{
				{
					lower: "3.1415",
					upper: "42",
					value: math.NaN(),
				},
			},
		},
		{
			text: "all positive buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{2, 1, -2, 3}, // Abs: 2, 3, 1, 4
			},
			subCases: append([]subCase{
				{
					lower: "0",
					upper: "+Inf",
					value: 1,
				},
				{
					lower: "-Inf",
					upper: "0",
					value: 0,
				},
				{
					lower: "-0.001",
					upper: "0",
					value: 0,
				},
				{
					lower: "0",
					upper: "0.001",
					value: 2. / 12.,
				},
				{
					lower: "0",
					upper: "0.0005",
					value: 1. / 12.,
				},
				{
					lower: "0.001",
					upper: "inf",
					value: 10. / 12.,
				},
				{
					lower: "-inf",
					upper: "-0.001",
					value: 0,
				},
				{
					lower: "1",
					upper: "2",
					value: 3. / 12.,
				},
				{
					lower: "1.5",
					upper: "2",
					value: 1.5 / 12.,
				},
				{
					lower: "1",
					upper: "8",
					value: 4. / 12.,
				},
				{
					lower: "1",
					upper: "6",
					value: 3.5 / 12.,
				},
				{
					lower: "1.5",
					upper: "6",
					value: 2. / 12.,
				},
				{
					lower: "-2",
					upper: "-1",
					value: 0,
				},
				{
					lower: "-2",
					upper: "-1.5",
					value: 0,
				},
				{
					lower: "-8",
					upper: "-1",
					value: 0,
				},
				{
					lower: "-6",
					upper: "-1",
					value: 0,
				},
				{
					lower: "-6",
					upper: "-1.5",
					value: 0,
				},
			}, invariantCases...),
		},
		{
			text: "all negative buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{2, 1, -2, 3},
			},
			subCases: append([]subCase{
				{
					lower: "0",
					upper: "+Inf",
					value: 0,
				},
				{
					lower: "-Inf",
					upper: "0",
					value: 1,
				},
				{
					lower: "-0.001",
					upper: "0",
					value: 2. / 12.,
				},
				{
					lower: "0",
					upper: "0.001",
					value: 0,
				},
				{
					lower: "-0.0005",
					upper: "0",
					value: 1. / 12.,
				},
				{
					lower: "0.001",
					upper: "inf",
					value: 0,
				},
				{
					lower: "-inf",
					upper: "-0.001",
					value: 10. / 12.,
				},
				{
					lower: "1",
					upper: "2",
					value: 0,
				},
				{
					lower: "1.5",
					upper: "2",
					value: 0,
				},
				{
					lower: "1",
					upper: "8",
					value: 0,
				},
				{
					lower: "1",
					upper: "6",
					value: 0,
				},
				{
					lower: "1.5",
					upper: "6",
					value: 0,
				},
				{
					lower: "-2",
					upper: "-1",
					value: 3. / 12.,
				},
				{
					lower: "-2",
					upper: "-1.5",
					value: 1.5 / 12.,
				},
				{
					lower: "-8",
					upper: "-1",
					value: 4. / 12.,
				},
				{
					lower: "-6",
					upper: "-1",
					value: 3.5 / 12.,
				},
				{
					lower: "-6",
					upper: "-1.5",
					value: 2. / 12.,
				},
			}, invariantCases...),
		},
		{
			text: "both positive and negative buckets with zero bucket",
			h: &histogram.Histogram{
				Count:         24,
				ZeroCount:     4,
				ZeroThreshold: 0.001,
				Sum:           100, // Does not matter.
				Schema:        0,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{2, 1, -2, 3},
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{2, 1, -2, 3},
			},
			subCases: append([]subCase{
				{
					lower: "0",
					upper: "+Inf",
					value: 0.5,
				},
				{
					lower: "-Inf",
					upper: "0",
					value: 0.5,
				},
				{
					lower: "-0.001",
					upper: "0",
					value: 2. / 24,
				},
				{
					lower: "0",
					upper: "0.001",
					value: 2. / 24.,
				},
				{
					lower: "-0.0005",
					upper: "0.0005",
					value: 2. / 24.,
				},
				{
					lower: "0.001",
					upper: "inf",
					value: 10. / 24.,
				},
				{
					lower: "-inf",
					upper: "-0.001",
					value: 10. / 24.,
				},
				{
					lower: "1",
					upper: "2",
					value: 3. / 24.,
				},
				{
					lower: "1.5",
					upper: "2",
					value: 1.5 / 24.,
				},
				{
					lower: "1",
					upper: "8",
					value: 4. / 24.,
				},
				{
					lower: "1",
					upper: "6",
					value: 3.5 / 24.,
				},
				{
					lower: "1.5",
					upper: "6",
					value: 2. / 24.,
				},
				{
					lower: "-2",
					upper: "-1",
					value: 3. / 24.,
				},
				{
					lower: "-2",
					upper: "-1.5",
					value: 1.5 / 24.,
				},
				{
					lower: "-8",
					upper: "-1",
					value: 4. / 24.,
				},
				{
					lower: "-6",
					upper: "-1",
					value: 3.5 / 24.,
				},
				{
					lower: "-6",
					upper: "-1.5",
					value: 2. / 24.,
				},
			}, invariantCases...),
		},
	}
	idx := int64(0)
	for _, floatHisto := range []bool{true, false} {
		for _, c := range cases {
			t.Run(fmt.Sprintf("%s floatHistogram=%t", c.text, floatHisto), func(t *testing.T) {
				test, err := NewTest(t, "")
				require.NoError(t, err)
				t.Cleanup(test.Close)

				seriesName := "sparse_histogram_series"
				lbls := labels.FromStrings("__name__", seriesName)
				engine := test.QueryEngine()

				ts := idx * int64(10*time.Minute/time.Millisecond)
				app := test.Storage().Appender(context.TODO())
				if floatHisto {
					_, err = app.AppendHistogram(0, lbls, ts, nil, c.h.ToFloat())
				} else {
					_, err = app.AppendHistogram(0, lbls, ts, c.h, nil)
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				for j, sc := range c.subCases {
					t.Run(fmt.Sprintf("%d %s %s", j, sc.lower, sc.upper), func(t *testing.T) {
						queryString := fmt.Sprintf("histogram_fraction(%s, %s, %s)", sc.lower, sc.upper, seriesName)
						qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(ts))
						require.NoError(t, err)

						res := qry.Exec(test.Context())
						require.NoError(t, res.Err)

						vector, err := res.Vector()
						require.NoError(t, err)

						require.Len(t, vector, 1)
						require.Nil(t, vector[0].H)
						if math.IsNaN(sc.value) {
							require.True(t, math.IsNaN(vector[0].V))
							return
						}
						require.Equal(t, sc.value, vector[0].V)
					})
				}
				idx++
			})
		}
	}
}

func TestNativeHistogram_Sum_Count_AddOperator(t *testing.T) {
	// TODO(codesome): Integrate histograms into the PromQL testing framework
	// and write more tests there.
	cases := []struct {
		histograms []histogram.Histogram
		expected   histogram.FloatHistogram
	}{
		{
			histograms: []histogram.Histogram{
				{
					Schema:        0,
					Count:         21,
					Sum:           1234.5,
					ZeroThreshold: 0.001,
					ZeroCount:     4,
					PositiveSpans: []histogram.Span{
						{Offset: 0, Length: 2},
						{Offset: 1, Length: 2},
					},
					PositiveBuckets: []int64{1, 1, -1, 0},
					NegativeSpans: []histogram.Span{
						{Offset: 0, Length: 2},
						{Offset: 2, Length: 2},
					},
					NegativeBuckets: []int64{2, 2, -3, 8},
				},
				{
					Schema:        0,
					Count:         36,
					Sum:           2345.6,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpans: []histogram.Span{
						{Offset: 0, Length: 4},
						{Offset: 0, Length: 0},
						{Offset: 0, Length: 3},
					},
					PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
					NegativeSpans: []histogram.Span{
						{Offset: 1, Length: 4},
						{Offset: 2, Length: 0},
						{Offset: 2, Length: 3},
					},
					NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
				},
				{
					Schema:        0,
					Count:         36,
					Sum:           1111.1,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpans: []histogram.Span{
						{Offset: 0, Length: 4},
						{Offset: 0, Length: 0},
						{Offset: 0, Length: 3},
					},
					PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
					NegativeSpans: []histogram.Span{
						{Offset: 1, Length: 4},
						{Offset: 2, Length: 0},
						{Offset: 2, Length: 3},
					},
					NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
				},
			},
			expected: histogram.FloatHistogram{
				Schema:        0,
				ZeroThreshold: 0.001,
				ZeroCount:     14,
				Count:         93,
				Sum:           4691.2,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 3},
					{Offset: 0, Length: 4},
				},
				PositiveBuckets: []float64{3, 8, 2, 5, 3, 2, 2},
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 2},
					{Offset: 3, Length: 3},
				},
				NegativeBuckets: []float64{2, 6, 8, 4, 15, 9, 10, 10, 4},
			},
		},
	}

	idx0 := int64(0)
	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t %d", floatHisto, idx0), func(t *testing.T) {
				test, err := NewTest(t, "")
				require.NoError(t, err)
				t.Cleanup(test.Close)

				seriesName := "sparse_histogram_series"

				engine := test.QueryEngine()

				ts := idx0 * int64(10*time.Minute/time.Millisecond)
				app := test.Storage().Appender(context.TODO())
				for idx1, h := range c.histograms {
					lbls := labels.FromStrings("__name__", seriesName, "idx", fmt.Sprintf("%d", idx1))
					// Since we mutate h later, we need to create a copy here.
					if floatHisto {
						_, err = app.AppendHistogram(0, lbls, ts, nil, h.Copy().ToFloat())
					} else {
						_, err = app.AppendHistogram(0, lbls, ts, h.Copy(), nil)
					}
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				queryAndCheck := func(queryString string, exp Vector) {
					qry, err := engine.NewInstantQuery(test.Queryable(), nil, queryString, timestamp.Time(ts))
					require.NoError(t, err)

					res := qry.Exec(test.Context())
					require.NoError(t, res.Err)

					vector, err := res.Vector()
					require.NoError(t, err)

					require.Equal(t, exp, vector)
				}

				// sum().
				queryString := fmt.Sprintf("sum(%s)", seriesName)
				queryAndCheck(queryString, []Sample{
					{Point{T: ts, H: &c.expected}, labels.EmptyLabels()},
				})

				// + operator.
				queryString = fmt.Sprintf(`%s{idx="0"}`, seriesName)
				for idx := 1; idx < len(c.histograms); idx++ {
					queryString += fmt.Sprintf(` + ignoring(idx) %s{idx="%d"}`, seriesName, idx)
				}
				queryAndCheck(queryString, []Sample{
					{Point{T: ts, H: &c.expected}, labels.EmptyLabels()},
				})

				// count().
				queryString = fmt.Sprintf("count(%s)", seriesName)
				queryAndCheck(queryString, []Sample{
					{Point{T: ts, V: 3}, labels.EmptyLabels()},
				})
			})
			idx0++
		}
	}
}

func TestQueryLookbackDelta(t *testing.T) {
	var (
		load = `load 5m
metric 0 1 2
`
		query           = "metric"
		lastDatapointTs = time.Unix(600, 0)
	)

	cases := []struct {
		name                          string
		ts                            time.Time
		engineLookback, queryLookback time.Duration
		expectSamples                 bool
	}{
		{
			name:          "default lookback delta",
			ts:            lastDatapointTs.Add(defaultLookbackDelta),
			expectSamples: true,
		},
		{
			name:          "outside default lookback delta",
			ts:            lastDatapointTs.Add(defaultLookbackDelta + time.Millisecond),
			expectSamples: false,
		},
		{
			name:           "custom engine lookback delta",
			ts:             lastDatapointTs.Add(10 * time.Minute),
			engineLookback: 10 * time.Minute,
			expectSamples:  true,
		},
		{
			name:           "outside custom engine lookback delta",
			ts:             lastDatapointTs.Add(10*time.Minute + time.Millisecond),
			engineLookback: 10 * time.Minute,
			expectSamples:  false,
		},
		{
			name:           "custom query lookback delta",
			ts:             lastDatapointTs.Add(20 * time.Minute),
			engineLookback: 10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  true,
		},
		{
			name:           "outside custom query lookback delta",
			ts:             lastDatapointTs.Add(20*time.Minute + time.Millisecond),
			engineLookback: 10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  false,
		},
		{
			name:           "negative custom query lookback delta",
			ts:             lastDatapointTs.Add(20 * time.Minute),
			engineLookback: -10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			test, err := NewTest(t, load)
			require.NoError(t, err)
			defer test.Close()

			err = test.Run()
			require.NoError(t, err)

			eng := test.QueryEngine()
			if c.engineLookback != 0 {
				eng.lookbackDelta = c.engineLookback
			}
			opts := &QueryOpts{
				LookbackDelta: c.queryLookback,
			}
			qry, err := eng.NewInstantQuery(test.Queryable(), opts, query, c.ts)
			require.NoError(t, err)

			res := qry.Exec(test.Context())
			require.NoError(t, res.Err)
			vec, ok := res.Value.(Vector)
			require.True(t, ok)
			if c.expectSamples {
				require.NotEmpty(t, vec)
			} else {
				require.Empty(t, vec)
			}
		})
	}
}
