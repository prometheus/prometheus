// Copyright The Prometheus Authors
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

package promql_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/logging"
	"github.com/prometheus/prometheus/util/stats"
	"github.com/prometheus/prometheus/util/testutil"
)

const (
	env                  = "query execution"
	defaultLookbackDelta = 5 * time.Minute
	defaultEpsilon       = 0.000001 // Relative error allowed for sample values.
)

func TestMain(m *testing.M) {
	// Enable experimental functions testing
	parser.EnableExperimentalFunctions = true
	testutil.TolerantVerifyLeak(m)
}

func TestQueryConcurrency(t *testing.T) {
	maxConcurrency := 10

	queryTracker := promql.NewActiveQueryTracker(t.TempDir(), maxConcurrency, nil)
	opts := promql.EngineOpts{
		Logger:             nil,
		Reg:                nil,
		MaxSamples:         10,
		Timeout:            100 * time.Second,
		ActiveQueryTracker: queryTracker,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	ctx, cancelCtx := context.WithCancel(context.Background())
	t.Cleanup(cancelCtx)

	block := make(chan struct{})
	processing := make(chan struct{})
	done := make(chan int)
	t.Cleanup(func() {
		close(done)
	})

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

	var wg sync.WaitGroup
	for range maxConcurrency {
		q := engine.NewTestQuery(f)
		wg.Add(1)
		go func() {
			q.Exec(ctx)
			wg.Done()
		}()
		select {
		case <-processing:
			// Expected.
		case <-time.After(20 * time.Millisecond):
			require.Fail(t, "Query within concurrency threshold not being executed")
		}
	}

	q := engine.NewTestQuery(f)
	wg.Add(1)
	go func() {
		q.Exec(ctx)
		wg.Done()
	}()

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
	for range maxConcurrency {
		block <- struct{}{}
	}

	wg.Wait()
}

// contextDone returns an error if the context was canceled or timed out.
func contextDone(ctx context.Context, env string) error {
	if err := ctx.Err(); err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return promql.ErrQueryCanceled(env)
		case errors.Is(err, context.DeadlineExceeded):
			return promql.ErrQueryTimeout(env)
		default:
			return err
		}
	}
	return nil
}

func TestQueryTimeout(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    5 * time.Millisecond,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)
	ctx := t.Context()

	query := engine.NewTestQuery(func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec(ctx)
	require.Error(t, res.Err, "expected timeout error but got none")

	var e promql.ErrQueryTimeout
	require.ErrorAs(t, res.Err, &e, "expected timeout error but got: %s", res.Err)
}

const errQueryCanceled = promql.ErrQueryCanceled("test statement execution")

func TestQueryCancel(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)
	ctx := t.Context()

	// Cancel a running query before it completes.
	block := make(chan struct{})
	processing := make(chan struct{})

	query1 := engine.NewTestQuery(func(ctx context.Context) error {
		processing <- struct{}{}
		<-block
		return contextDone(ctx, "test statement execution")
	})

	var res *promql.Result

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
	query2 := engine.NewTestQuery(func(ctx context.Context) error {
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

func (q *errQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return errSeriesSet{err: q.err}
}

func (*errQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (*errQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}
func (*errQuerier) Close() error { return nil }

// errSeriesSet implements storage.SeriesSet which always returns error.
type errSeriesSet struct {
	err error
}

func (errSeriesSet) Next() bool                        { return false }
func (errSeriesSet) At() storage.Series                { return nil }
func (e errSeriesSet) Err() error                      { return e.err }
func (errSeriesSet) Warnings() annotations.Annotations { return nil }

func TestQueryError(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)
	errStorage := promql.ErrStorage{errors.New("storage error")}
	queryable := storage.QueryableFunc(func(_, _ int64) (storage.Querier, error) {
		return &errQuerier{err: errStorage}, nil
	})
	ctx := t.Context()

	vectorQuery, err := engine.NewInstantQuery(ctx, queryable, nil, "foo", time.Unix(1, 0))
	require.NoError(t, err)

	res := vectorQuery.Exec(ctx)
	require.Error(t, res.Err, "expected error on failed select but got none")
	require.ErrorIs(t, res.Err, errStorage, "expected error doesn't match")

	matrixQuery, err := engine.NewInstantQuery(ctx, queryable, nil, "foo[1m]", time.Unix(1, 0))
	require.NoError(t, err)

	res = matrixQuery.Exec(ctx)
	require.Error(t, res.Err, "expected error on failed select but got none")
	require.ErrorIs(t, res.Err, errStorage, "expected error doesn't match")
}

type noopHintRecordingQueryable struct {
	hints []*storage.SelectHints
}

func (h *noopHintRecordingQueryable) Querier(int64, int64) (storage.Querier, error) {
	return &hintRecordingQuerier{Querier: &errQuerier{}, h: h}, nil
}

type hintRecordingQuerier struct {
	storage.Querier

	h *noopHintRecordingQueryable
}

func (h *hintRecordingQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	h.h.hints = append(h.h.hints, hints)
	return h.Querier.Select(ctx, sortSeries, hints, matchers...)
}

func TestSelectHintsSetCorrectly(t *testing.T) {
	opts := promql.EngineOpts{
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
				{Start: 5001, End: 10000},
			},
		}, {
			query: "foo @ 15", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 10001, End: 15000},
			},
		}, {
			query: "foo @ 1", start: 10000,
			expected: []*storage.SelectHints{
				{Start: -3999, End: 1000},
			},
		}, {
			query: "foo[2m]", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 80001, End: 200000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 180", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 60001, End: 180000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 300", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 180001, End: 300000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 60", start: 200000,
			expected: []*storage.SelectHints{
				{Start: -59999, End: 60000, Range: 120000},
			},
		}, {
			query: "foo[2m] offset 2m", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 60001, End: 180000, Range: 120000},
			},
		}, {
			query: "foo[2m] @ 200 offset 2m", start: 300000,
			expected: []*storage.SelectHints{
				{Start: -39999, End: 80000, Range: 120000},
			},
		}, {
			query: "foo[2m:1s]", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 300000, Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s])", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 300)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 200)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: 75001, End: 200000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 100)", start: 200000,
			expected: []*storage.SelectHints{
				{Start: -24999, End: 100000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 165001, End: 290000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 155001, End: 280000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 185001, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: 185001, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000,
			expected: []*storage.SelectHints{
				{Start: -44999, End: 80000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "foo", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: 5001, End: 20000, Step: 1000},
			},
		}, {
			query: "foo @ 15", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: 10001, End: 15000, Step: 1000},
			},
		}, {
			query: "foo @ 1", start: 10000, end: 20000,
			expected: []*storage.SelectHints{
				{Start: -3999, End: 1000, Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 180)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 60001, End: 180000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 300)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 180001, End: 300000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] @ 60)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -59999, End: 60000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m])", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 80001, End: 500000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m] offset 2m)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 60001, End: 380000, Range: 120000, Func: "rate", Step: 1000},
			},
		}, {
			query: "rate(foo[2m:1s])", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 500000, Func: "rate", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s])", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 500000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 165001, End: 490000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 300)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 175001, End: 300000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 200)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 75001, End: 200000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time(foo[2m:1s] @ 100)", start: 200000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -24999, End: 100000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 155001, End: 480000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 185001, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			// When the @ is on the vector selector, the enclosing subquery parameters
			// don't affect the hint ranges.
			query: "count_over_time((foo @ 200 offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 185001, End: 190000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "count_over_time((foo offset 10s)[2m:1s] @ 100 offset 10s)", start: 300000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: -44999, End: 80000, Func: "count_over_time", Step: 1000},
			},
		}, {
			query: "sum by (dim1) (foo)", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5001, End: 10000, Func: "sum", By: true, Grouping: []string{"dim1"}},
			},
		}, {
			query: "sum without (dim1) (foo)", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5001, End: 10000, Func: "sum", Grouping: []string{"dim1"}},
			},
		}, {
			query: "sum by (dim1) (avg_over_time(foo[1s]))", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 9001, End: 10000, Func: "avg_over_time", Range: 1000},
			},
		}, {
			query: "sum by (dim1) (max by (dim2) (foo))", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 5001, End: 10000, Func: "max", By: true, Grouping: []string{"dim2"}},
			},
		}, {
			query: "(max by (dim1) (foo))[5s:1s]", start: 10000,
			expected: []*storage.SelectHints{
				{Start: 1, End: 10000, Func: "max", By: true, Grouping: []string{"dim1"}, Step: 1000},
			},
		}, {
			query: "(sum(http_requests{group=~\"p.*\"})+max(http_requests{group=~\"c.*\"}))[20s:5s]", start: 120000,
			expected: []*storage.SelectHints{
				{Start: 95001, End: 120000, Func: "sum", By: true, Step: 5000},
				{Start: 95001, End: 120000, Func: "max", By: true, Step: 5000},
			},
		}, {
			query: "foo @ 50 + bar @ 250 + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 45001, End: 50000, Step: 1000},
				{Start: 245001, End: 250000, Step: 1000},
				{Start: 895001, End: 900000, Step: 1000},
			},
		}, {
			query: "foo @ 50 + bar + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 45001, End: 50000, Step: 1000},
				{Start: 95001, End: 500000, Step: 1000},
				{Start: 895001, End: 900000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s] @ 50) + bar @ 250 + baz @ 900", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 48001, End: 50000, Step: 1000, Func: "rate", Range: 2000},
				{Start: 245001, End: 250000, Step: 1000},
				{Start: 895001, End: 900000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s:1s] @ 50) + bar + baz", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 43001, End: 50000, Step: 1000, Func: "rate"},
				{Start: 95001, End: 500000, Step: 1000},
				{Start: 95001, End: 500000, Step: 1000},
			},
		}, {
			query: "rate(foo[2s:1s] @ 50) + bar + rate(baz[2m:1s] @ 900 offset 2m) ", start: 100000, end: 500000,
			expected: []*storage.SelectHints{
				{Start: 43001, End: 50000, Step: 1000, Func: "rate"},
				{Start: 95001, End: 500000, Step: 1000},
				{Start: 655001, End: 780000, Step: 1000, Func: "rate"},
			},
		}, { // Hints are based on the inner most subquery timestamp.
			query: `sum_over_time(sum_over_time(metric{job="1"}[100s])[100s:25s] @ 50)[3s:1s] @ 3000`, start: 100000,
			expected: []*storage.SelectHints{
				{Start: -149999, End: 50000, Range: 100000, Func: "sum_over_time", Step: 25000},
			},
		}, { // Hints are based on the inner most subquery timestamp.
			query: `sum_over_time(sum_over_time(metric{job="1"}[100s])[100s:25s] @ 3000)[3s:1s] @ 50`,
			expected: []*storage.SelectHints{
				{Start: 2800001, End: 3000000, Range: 100000, Func: "sum_over_time", Step: 25000},
			},
		},
	} {
		t.Run(tc.query, func(t *testing.T) {
			engine := promqltest.NewTestEngineWithOpts(t, opts)
			hintsRecorder := &noopHintRecordingQueryable{}

			var (
				query promql.Query
				err   error
			)
			ctx := context.Background()

			if tc.end == 0 {
				query, err = engine.NewInstantQuery(ctx, hintsRecorder, nil, tc.query, timestamp.Time(tc.start))
			} else {
				query, err = engine.NewRangeQuery(ctx, hintsRecorder, nil, tc.query, timestamp.Time(tc.start), timestamp.Time(tc.end), time.Second)
			}
			require.NoError(t, err)

			res := query.Exec(context.Background())
			require.NoError(t, res.Err)

			require.Equal(t, tc.expected, hintsRecorder.hints)
		})
	}
}

func TestEngineShutdown(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)
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
	query1 := engine.NewTestQuery(f)

	// Stopping the engine must cancel the base context. While executing queries is
	// still possible, their context is canceled from the beginning and execution should
	// terminate immediately.

	var res *promql.Result
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

	query2 := engine.NewTestQuery(func(context.Context) error {
		require.FailNow(t, "reached query execution unexpectedly")
		return nil
	})

	// The second query is started after the engine shut down. It must
	// be canceled immediately.
	res2 := query2.Exec(ctx)
	require.Error(t, res2.Err, "expected error on querying with canceled context but got none")

	var e promql.ErrQueryCanceled
	require.ErrorAs(t, res2.Err, &e, "expected cancellation error but got: %s", res2.Err)
}

func TestEngineEvalStmtTimestamps(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
load 10s
  metric 1 2
`)

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
			Result: promql.Scalar{V: 1, T: 1000},
			Start:  time.Unix(1, 0),
		},
		{
			Query: "metric",
			Result: promql.Vector{
				promql.Sample{
					F:      1,
					T:      1000,
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start: time.Unix(1, 0),
		},
		{
			Query: "metric[20s]",
			Result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start: time.Unix(10, 0),
		},
		// Range queries.
		{
			Query: "1",
			Result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 1000}, {F: 1, T: 2000}},
					Metric: labels.EmptyLabels(),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 1000}, {F: 1, T: 2000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 5000}, {F: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(10, 0),
			Interval: 5 * time.Second,
		},
		{
			Query:       `count_values("wrong label!\xff", metric)`,
			ShouldError: true,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d query=%s", i, c.Query), func(t *testing.T) {
			var err error
			var qry promql.Query
			engine := newTestEngine(t)
			if c.Interval == 0 {
				qry, err = engine.NewInstantQuery(context.Background(), storage, nil, c.Query, c.Start)
			} else {
				qry, err = engine.NewRangeQuery(context.Background(), storage, nil, c.Query, c.Start, c.End, c.Interval)
			}
			require.NoError(t, err)

			res := qry.Exec(context.Background())
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
	storage := promqltest.LoadedStorage(t, `
load 10s
  metricWith1SampleEvery10Seconds 1+1x100
  metricWith3SampleEvery10Seconds{a="1",b="1"} 1+1x100
  metricWith3SampleEvery10Seconds{a="2",b="2"} 1+1x100
  metricWith3SampleEvery10Seconds{a="3",b="2"} 1+1x100
  metricWith1HistogramEvery10Seconds {{schema:1 count:5 sum:20 buckets:[1 2 1 1]}}+{{schema:1 count:10 sum:5 buckets:[1 2 3 4]}}x100
`)

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
			Query:        "metricWith1HistogramEvery10Seconds",
			Start:        time.Unix(21, 0),
			PeakSamples:  13,
			TotalSamples: 13, // 1 histogram HPoint of size 13 / 10 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				21000: 13,
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
			Query:        "timestamp(metricWith1HistogramEvery10Seconds)",
			Start:        time.Unix(21, 0),
			PeakSamples:  2,
			TotalSamples: 1, // 1 float sample (because of timestamp) / 10 seconds
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
			Query:        "metricWith1HistogramEvery10Seconds[60s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  78,
			TotalSamples: 78, // 1 histogram (size 13 HPoint) / 10 seconds * 60 seconds
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 78,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[60s])[20s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  10,
			TotalSamples: 24, // (1 sample / 10 seconds * 60 seconds) * 4
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 24,
			},
		},
		{
			Query:        "max_over_time(metricWith1SampleEvery10Seconds[61s])[20s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  11,
			TotalSamples: 26, // (1 sample / 10 seconds * 60 seconds) * 4 + 2 as
			// max_over_time(metricWith1SampleEvery10Seconds[61s]) @ 190 and 200 will return 7 samples.
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 26,
			},
		},
		{
			Query:        "max_over_time(metricWith1HistogramEvery10Seconds[60s])[20s:5s]",
			Start:        time.Unix(201, 0),
			PeakSamples:  78,
			TotalSamples: 312, // (1 histogram (size 13) / 10 seconds * 60 seconds) * 4
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 312,
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
			Query:        "metricWith1HistogramEvery10Seconds[60s] @ 30",
			Start:        time.Unix(201, 0),
			PeakSamples:  52,
			TotalSamples: 52, // @ modifier force the evaluation to at 30 seconds - So it brings 4 datapoints (0, 10, 20, 30 seconds) * 1 series
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 52,
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
			PeakSamples:  7,
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
			Query:        `metricWith1HistogramEvery10Seconds`,
			Start:        time.Unix(204, 0),
			End:          time.Unix(223, 0),
			Interval:     5 * time.Second,
			PeakSamples:  52,
			TotalSamples: 52, // 1 histogram (size 13 HPoint) per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				204000: 13, // aligned to the step time, not the sample time
				209000: 13,
				214000: 13,
				219000: 13,
			},
		},
		{
			// timestamp function has a special handling
			Query:        "timestamp(metricWith1SampleEvery10Seconds)",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  5,
			TotalSamples: 4, // 1 sample per query * 4 steps
			TotalSamplesPerStep: stats.TotalSamplesPerStep{
				201000: 1,
				206000: 1,
				211000: 1,
				216000: 1,
			},
		},
		{
			// timestamp function has a special handling
			Query:        "timestamp(metricWith1HistogramEvery10Seconds)",
			Start:        time.Unix(201, 0),
			End:          time.Unix(220, 0),
			Interval:     5 * time.Second,
			PeakSamples:  5,
			TotalSamples: 4, // 1 sample per query * 4 steps
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

	for _, c := range cases {
		t.Run(c.Query, func(t *testing.T) {
			opts := promql.NewPrometheusQueryOpts(true, 0)
			engine := promqltest.NewTestEngine(t, true, 0, promqltest.DefaultMaxSamplesPerQuery)

			runQuery := func(expErr error) *stats.Statistics {
				var err error
				var qry promql.Query
				if c.Interval == 0 {
					qry, err = engine.NewInstantQuery(context.Background(), storage, opts, c.Query, c.Start)
				} else {
					qry, err = engine.NewRangeQuery(context.Background(), storage, opts, c.Query, c.Start, c.End, c.Interval)
				}
				require.NoError(t, err)

				res := qry.Exec(context.Background())
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
			engine = promqltest.NewTestEngine(t, true, 0, stats.Samples.PeakSamples-1)
			runQuery(promql.ErrTooManySamples(env))
		})
	}
}

func TestMaxQuerySamples(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
load 10s
  metric 1+1x100
  bigmetric{a="1"} 1+1x100
  bigmetric{a="2"} 1+1x100
`)

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
			//   - Subquery takes 20 samples, 10 for each bigmetric.
			//   - Result is calculated per series where the series samples is buffered, hence 10 more here.
			//   - The result of two series is added before the last series buffer is discarded, so 2 more here.
			//   Hence at peak it is 20 (subquery) + 10 (buffer of a series) + 2 (result from 2 series).
			// The subquery samples and the buffer is discarded before duplicating.
			Query:      `rate(bigmetric[10s:1s] @ 10)`,
			MaxSamples: 32,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// Here the reasoning is same as above. But LHS and RHS are done one after another.
			// So while one of them takes 32 samples at peak, we need to hold the 2 sample
			// result of the other till then.
			Query:      `rate(bigmetric[10s:1s] @ 10) + rate(bigmetric[10s:1s] @ 30)`,
			MaxSamples: 34,
			Start:      time.Unix(0, 0),
			End:        time.Unix(10, 0),
			Interval:   5 * time.Second,
		},
		{
			// promql.Sample as above but with only 1 part as step invariant.
			// Here the peak is caused by the non-step invariant part as it touches more time range.
			// Hence at peak it is 2*20 (subquery from 0s to 20s)
			//                     + 10 (buffer of a series per evaluation)
			//                     + 6 (result from 2 series at 3 eval times).
			Query:      `rate(bigmetric[10s:1s]) + rate(bigmetric[10s:1s] @ 30)`,
			MaxSamples: 56,
			Start:      time.Unix(10, 0),
			End:        time.Unix(20, 0),
			Interval:   5 * time.Second,
		},
		{
			// Nested subquery.
			// We saw that innermost rate takes 32 samples which is still the peak
			// since the other two subqueries just duplicate the result.
			Query:      `rate(rate(bigmetric[10:1s] @ 10)[100s:25s] @ 1000)[100s:20s] @ 2000`,
			MaxSamples: 32,
			Start:      time.Unix(10, 0),
		},
		{
			// Nested subquery.
			// Now the outermost subquery produces more samples than innermost rate.
			Query:      `rate(rate(bigmetric[10s:1s] @ 10)[100s:25s] @ 1000)[17s:1s] @ 2000`,
			MaxSamples: 34,
			Start:      time.Unix(10, 0),
		},
	}

	for _, c := range cases {
		t.Run(c.Query, func(t *testing.T) {
			engine := newTestEngine(t)
			testFunc := func(expError error) {
				var err error
				var qry promql.Query
				if c.Interval == 0 {
					qry, err = engine.NewInstantQuery(context.Background(), storage, nil, c.Query, c.Start)
				} else {
					qry, err = engine.NewRangeQuery(context.Background(), storage, nil, c.Query, c.Start, c.End, c.Interval)
				}
				require.NoError(t, err)

				res := qry.Exec(context.Background())
				stats := qry.Stats()
				require.Equal(t, expError, res.Err)
				require.NotNil(t, stats)
				if expError == nil {
					require.Equal(t, c.MaxSamples, stats.Samples.PeakSamples, "peak samples mismatch for query %q", c.Query)
				}
			}

			// Within limit.
			engine = promqltest.NewTestEngine(t, false, 0, c.MaxSamples)
			testFunc(nil)

			// Exceeding limit.
			engine = promqltest.NewTestEngine(t, false, 0, c.MaxSamples-1)
			testFunc(promql.ErrTooManySamples(env))
		})
	}
}

func TestExtendedRangeSelectors(t *testing.T) {
	parser.EnableExtendedRangeSelectors = true
	t.Cleanup(func() {
		parser.EnableExtendedRangeSelectors = false
	})

	engine := newTestEngine(t)
	storage := promqltest.LoadedStorage(t, `
	load 10s
		metric 1+1x10
		withreset 1+1x4 1+1x5
		notregular 0 5 100 2 8
	`)

	tc := []struct {
		query    string
		t        time.Time
		expected promql.Matrix
	}{
		{
			query: "metric[10s] smoothed",
			t:     time.Unix(10, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] smoothed",
			t:     time.Unix(15, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1.5, T: 5000}, {F: 2, T: 10000}, {F: 2.5, T: 15000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] smoothed",
			t:     time.Unix(5, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: -5000}, {F: 1, T: 0}, {F: 1.5, T: 5000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] smoothed",
			t:     time.Unix(105, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 10.5, T: 95000}, {F: 11, T: 100000}, {F: 11, T: 105000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "withreset[10s] smoothed",
			t:     time.Unix(45, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 4.5, T: 35000}, {F: 5, T: 40000}, {F: 3, T: 45000}},
					Metric: labels.FromStrings("__name__", "withreset"),
				},
			},
		},
		{
			query: "metric[10s] anchored",
			t:     time.Unix(10, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 0}, {F: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] anchored",
			t:     time.Unix(15, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: 5000}, {F: 2, T: 10000}, {F: 2, T: 15000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] anchored",
			t:     time.Unix(5, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 1, T: -5000}, {F: 1, T: 0}, {F: 1, T: 5000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "metric[10s] anchored",
			t:     time.Unix(105, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 10, T: 95000}, {F: 11, T: 100000}, {F: 11, T: 105000}},
					Metric: labels.FromStrings("__name__", "metric"),
				},
			},
		},
		{
			query: "withreset[10s] anchored",
			t:     time.Unix(45, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 4, T: 35000}, {F: 5, T: 40000}, {F: 5, T: 45000}},
					Metric: labels.FromStrings("__name__", "withreset"),
				},
			},
		},
		{
			query: "notregular[20s] smoothed",
			t:     time.Unix(30, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 5, T: 10000}, {F: 100, T: 20000}, {F: 2, T: 30000}},
					Metric: labels.FromStrings("__name__", "notregular"),
				},
			},
		},
		{
			query: "notregular[20s] anchored",
			t:     time.Unix(30, 0),
			expected: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 5, T: 10000}, {F: 100, T: 20000}, {F: 2, T: 30000}},
					Metric: labels.FromStrings("__name__", "notregular"),
				},
			},
		},
	}

	for _, tc := range tc {
		t.Run(tc.query, func(t *testing.T) {
			engine = promqltest.NewTestEngine(t, false, 0, 100)
			qry, err := engine.NewInstantQuery(context.Background(), storage, nil, tc.query, tc.t)
			require.NoError(t, err)
			res := qry.Exec(context.Background())
			require.NoError(t, res.Err)
			require.Equal(t, tc.expected, res.Value)
		})
	}
}

func TestAtModifier(t *testing.T) {
	engine := newTestEngine(t)
	storage := promqltest.LoadedStorage(t, `
load 10s
  metric{job="1"} 0+1x1000
  metric{job="2"} 0+2x1000
  metric_topk{instance="1"} 0+1x1000
  metric_topk{instance="2"} 0+2x1000
  metric_topk{instance="3"} 1000-1x1000

load 1ms
  metric_ms 0+1x10000
`)

	lbls1 := labels.FromStrings("__name__", "metric", "job", "1")
	lbls2 := labels.FromStrings("__name__", "metric", "job", "2")
	lblstopk2 := labels.FromStrings("__name__", "metric_topk", "instance", "2")
	lblstopk3 := labels.FromStrings("__name__", "metric_topk", "instance", "3")
	lblsms := labels.FromStrings("__name__", "metric_ms")
	lblsneg := labels.FromStrings("__name__", "metric_neg")

	// Add some samples with negative timestamp.
	db := storage.DB
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
			result: promql.Vector{
				promql.Sample{F: 1, T: 100000, Metric: lblsneg},
			},
		}, {
			query: `metric_neg @ -200`,
			start: 100,
			result: promql.Vector{
				promql.Sample{F: 201, T: 100000, Metric: lblsneg},
			},
		}, {
			query: `metric{job="2"} @ 50`,
			start: -2, end: 2, interval: 1,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 10, T: -2000}, {F: 10, T: -1000}, {F: 10, T: 0}, {F: 10, T: 1000}, {F: 10, T: 2000}},
					Metric: lbls2,
				},
			},
		}, { // Timestamps for matrix selector does not depend on the evaluation time.
			query: "metric[20s] @ 300",
			start: 10,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 29, T: 290000}, {F: 30, T: 300000}},
					Metric: lbls1,
				},
				promql.Series{
					Floats: []promql.FPoint{{F: 58, T: 290000}, {F: 60, T: 300000}},
					Metric: lbls2,
				},
			},
		}, {
			query: `metric_neg[2s] @ 0`,
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 2, T: -1000}, {F: 1, T: 0}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_neg[3s] @ -500`,
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 503, T: -502000}, {F: 502, T: -501000}, {F: 501, T: -500000}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_ms[3ms] @ 2.345`,
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 2343, T: 2343}, {F: 2344, T: 2344}, {F: 2345, T: 2345}},
					Metric: lblsms,
				},
			},
		}, {
			query: "metric[100s:25s] @ 300",
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 22, T: 225000}, {F: 25, T: 250000}, {F: 27, T: 275000}, {F: 30, T: 300000}},
					Metric: lbls1,
				},
				promql.Series{
					Floats: []promql.FPoint{{F: 44, T: 225000}, {F: 50, T: 250000}, {F: 54, T: 275000}, {F: 60, T: 300000}},
					Metric: lbls2,
				},
			},
		}, {
			query: "metric[100s1ms:25s] @ 300", // Add 1ms to the range to see the legacy behavior of the previous test.
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 20, T: 200000}, {F: 22, T: 225000}, {F: 25, T: 250000}, {F: 27, T: 275000}, {F: 30, T: 300000}},
					Metric: lbls1,
				},
				promql.Series{
					Floats: []promql.FPoint{{F: 40, T: 200000}, {F: 44, T: 225000}, {F: 50, T: 250000}, {F: 54, T: 275000}, {F: 60, T: 300000}},
					Metric: lbls2,
				},
			},
		}, {
			query: "metric_neg[50s:25s] @ 0",
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 26, T: -25000}, {F: 1, T: 0}},
					Metric: lblsneg,
				},
			},
		}, {
			query: "metric_neg[50s1ms:25s] @ 0", // Add 1ms to the range to see the legacy behavior of the previous test.
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 51, T: -50000}, {F: 26, T: -25000}, {F: 1, T: 0}},
					Metric: lblsneg,
				},
			},
		}, {
			query: "metric_neg[50s:25s] @ -100",
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 126, T: -125000}, {F: 101, T: -100000}},
					Metric: lblsneg,
				},
			},
		}, {
			query: "metric_neg[50s1ms:25s] @ -100", // Add 1ms to the range to see the legacy behavior of the previous test.
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 151, T: -150000}, {F: 126, T: -125000}, {F: 101, T: -100000}},
					Metric: lblsneg,
				},
			},
		}, {
			query: `metric_ms[101ms:25ms] @ 2.345`,
			start: 100,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 2250, T: 2250}, {F: 2275, T: 2275}, {F: 2300, T: 2300}, {F: 2325, T: 2325}},
					Metric: lblsms,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ 100))`,
			start: 50, end: 80, interval: 10,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 995, T: 50000}, {F: 994, T: 60000}, {F: 993, T: 70000}, {F: 992, T: 80000}},
					Metric: lblstopk3,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ 5000))`,
			start: 50, end: 80, interval: 10,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 10, T: 50000}, {F: 12, T: 60000}, {F: 14, T: 70000}, {F: 16, T: 80000}},
					Metric: lblstopk2,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ end()))`,
			start: 70, end: 100, interval: 10,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 993, T: 70000}, {F: 992, T: 80000}, {F: 991, T: 90000}, {F: 990, T: 100000}},
					Metric: lblstopk3,
				},
			},
		}, {
			query: `metric_topk and topk(1, sum_over_time(metric_topk[50s] @ start()))`,
			start: 100, end: 130, interval: 10,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{{F: 990, T: 100000}, {F: 989, T: 110000}, {F: 988, T: 120000}, {F: 987, T: 130000}},
					Metric: lblstopk3,
				},
			},
		}, {
			// Tests for https://github.com/prometheus/prometheus/issues/8433.
			// The trick here is that the query range should be > lookback delta.
			query: `timestamp(metric_timestamp @ 3600)`,
			start: 0, end: 7 * 60, interval: 60,
			result: promql.Matrix{
				promql.Series{
					Floats: []promql.FPoint{
						{F: 3600, T: 0},
						{F: 3600, T: 60 * 1000},
						{F: 3600, T: 2 * 60 * 1000},
						{F: 3600, T: 3 * 60 * 1000},
						{F: 3600, T: 4 * 60 * 1000},
						{F: 3600, T: 5 * 60 * 1000},
						{F: 3600, T: 6 * 60 * 1000},
						{F: 3600, T: 7 * 60 * 1000},
					},
					Metric:   labels.EmptyLabels(),
					DropName: true,
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
			var qry promql.Query
			if c.end == 0 {
				qry, err = engine.NewInstantQuery(context.Background(), storage, nil, c.query, start)
			} else {
				qry, err = engine.NewRangeQuery(context.Background(), storage, nil, c.query, start, end, interval)
			}
			require.NoError(t, err)

			res := qry.Exec(context.Background())
			require.NoError(t, res.Err)
			if expMat, ok := c.result.(promql.Matrix); ok {
				sort.Sort(expMat)
				sort.Sort(res.Value.(promql.Matrix))
			}
			testutil.RequireEqual(t, c.result, res.Value, "query %q failed", c.query)
		})
	}
}

func TestSubquerySelector(t *testing.T) {
	type caseType struct {
		Query  string
		Result promql.Result
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
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1, T: 0}, {F: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s]",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 5000}, {F: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s] offset 2s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 5000}, {F: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(12, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1, T: 0}, {F: 1, T: 5000}, {F: 2, T: 10000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(20, 0),
				},
				{
					Query: "metric[20s:5s] offset 4s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 2, T: 15000}, {F: 2, T: 20000}, {F: 2, T: 25000}, {F: 2, T: 30000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 5s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 2, T: 15000}, {F: 2, T: 20000}, {F: 2, T: 25000}, {F: 2, T: 30000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 2, T: 10000}, {F: 2, T: 15000}, {F: 2, T: 20000}, {F: 2, T: 25000}},
								Metric: labels.FromStrings("__name__", "metric"),
							},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 7s",
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 2, T: 10000}, {F: 2, T: 15000}, {F: 2, T: 20000}, {F: 2, T: 25000}},
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
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 10000, T: 10000000}, {F: 100, T: 10010000}, {F: 130, T: 10020000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10020, 0),
				},
				{ // Normal selector. Add 1ms to the range to see the legacy behavior of the previous test.
					Query: `http_requests{group=~"pro.*",instance="0"}[30s1ms:10s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 9990, T: 9990000}, {F: 10000, T: 10000000}, {F: 100, T: 10010000}, {F: 130, T: 10020000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10020, 0),
				},
				{ // Default step.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 9840, T: 9840000}, {F: 9900, T: 9900000}, {F: 9960, T: 9960000}, {F: 130, T: 10020000}, {F: 310, T: 10080000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10100, 0),
				},
				{ // Checking if high offset (>LookbackDelta) is being taken care of.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:] offset 20m`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 8640, T: 8640000}, {F: 8700, T: 8700000}, {F: 8760, T: 8760000}, {F: 8820, T: 8820000}, {F: 8880, T: 8880000}},
								Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production"),
							},
						},
						nil,
					},
					Start: time.Unix(10100, 0),
				},
				{
					Query: `rate(http_requests[1m])[15s:5s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats:   []promql.FPoint{{F: 3, T: 7990000}, {F: 3, T: 7995000}, {F: 3, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "0", "group", "canary"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 4, T: 7990000}, {F: 4, T: 7995000}, {F: 4, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "1", "group", "canary"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 1, T: 7990000}, {F: 1, T: 7995000}, {F: 1, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "0", "group", "production"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 2, T: 7990000}, {F: 2, T: 7995000}, {F: 2, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "1", "group", "production"),
								DropName: true,
							},
						},
						nil,
					},
					Start: time.Unix(8000, 0),
				},
				{
					Query: `rate(http_requests[1m])[15s1ms:5s]`, // Add 1ms to the range to see the legacy behavior of the previous test.
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats:   []promql.FPoint{{F: 3, T: 7985000}, {F: 3, T: 7990000}, {F: 3, T: 7995000}, {F: 3, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "0", "group", "canary"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 4, T: 7985000}, {F: 4, T: 7990000}, {F: 4, T: 7995000}, {F: 4, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "1", "group", "canary"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 1, T: 7985000}, {F: 1, T: 7990000}, {F: 1, T: 7995000}, {F: 1, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "0", "group", "production"),
								DropName: true,
							},
							promql.Series{
								Floats:   []promql.FPoint{{F: 2, T: 7985000}, {F: 2, T: 7990000}, {F: 2, T: 7995000}, {F: 2, T: 8000000}},
								Metric:   labels.FromStrings("job", "api-server", "instance", "1", "group", "production"),
								DropName: true,
							},
						},
						nil,
					},
					Start: time.Unix(8000, 0),
				},
				{
					Query: `sum(http_requests{group=~"pro.*"})[30s:10s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 300, T: 100000}, {F: 330, T: 110000}, {F: 360, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `sum(http_requests{group=~"pro.*"})[30s:10s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 300, T: 100000}, {F: 330, T: 110000}, {F: 360, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(121, 0), // 1s later doesn't change the result.
				},
				{
					// Add 1ms to the range to see the legacy behavior of the previous test.
					Query: `sum(http_requests{group=~"pro.*"})[30s1ms:10s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 270, T: 90000}, {F: 300, T: 100000}, {F: 330, T: 110000}, {F: 360, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `sum(http_requests)[40s:10s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 900, T: 90000}, {F: 1000, T: 100000}, {F: 1100, T: 110000}, {F: 1200, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `sum(http_requests)[40s1ms:10s]`, // Add 1ms to the range to see the legacy behavior of the previous test.
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 800, T: 80000}, {F: 900, T: 90000}, {F: 1000, T: 100000}, {F: 1100, T: 110000}, {F: 1200, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `(sum(http_requests{group=~"p.*"})+sum(http_requests{group=~"c.*"}))[20s:5s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1000, T: 105000}, {F: 1100, T: 110000}, {F: 1100, T: 115000}, {F: 1200, T: 120000}},
								Metric: labels.EmptyLabels(),
							},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					// Add 1ms to the range to see the legacy behavior of the previous test.
					Query: `(sum(http_requests{group=~"p.*"})+sum(http_requests{group=~"c.*"}))[20s1ms:5s]`,
					Result: promql.Result{
						nil,
						promql.Matrix{
							promql.Series{
								Floats: []promql.FPoint{{F: 1000, T: 100000}, {F: 1000, T: 105000}, {F: 1100, T: 110000}, {F: 1100, T: 115000}, {F: 1200, T: 120000}},
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
			engine := newTestEngine(t)
			storage := promqltest.LoadedStorage(t, tst.loadString)

			for _, c := range tst.cases {
				t.Run(c.Query, func(t *testing.T) {
					qry, err := engine.NewInstantQuery(context.Background(), storage, nil, c.Query, c.Start)
					require.NoError(t, err)

					res := qry.Exec(context.Background())
					require.Equal(t, c.Result.Err, res.Err)
					mat := res.Value.(promql.Matrix)
					sort.Sort(mat)
					testutil.RequireEqual(t, c.Result.Value, mat)
				})
			}
		})
	}
}

func getLogLines(t *testing.T, name string) []string {
	content, err := os.ReadFile(name)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			lines = append(lines[:i], lines[i+1:]...)
		}
	}
	return lines
}

func TestQueryLogger_basic(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	queryExec := func() {
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		query := engine.NewTestQuery(func(ctx context.Context) error {
			return contextDone(ctx, "test statement execution")
		})
		res := query.Exec(ctx)
		require.NoError(t, res.Err)
	}

	// promql.Query works without query log initialized.
	queryExec()

	tmpDir := t.TempDir()
	ql1File := filepath.Join(tmpDir, "query1.log")
	f1, err := logging.NewJSONFileLogger(ql1File)
	require.NoError(t, err)

	engine.SetQueryLogger(f1)
	queryExec()
	logLines := getLogLines(t, ql1File)
	require.Contains(t, logLines[0], "params", map[string]any{"query": "test statement"})
	require.Len(t, logLines, 1)

	l := len(logLines)
	queryExec()
	logLines = getLogLines(t, ql1File)
	l2 := len(logLines)
	require.Equal(t, l2, 2*l)

	// Test that we close the query logger when unsetting it. The following
	// attempt to close the file should error.
	engine.SetQueryLogger(nil)
	err = f1.Close()
	require.ErrorContains(t, err, "file already closed", "expected f1 to be closed, got open")
	queryExec()

	// Test that we close the query logger when swapping.
	ql2File := filepath.Join(tmpDir, "query2.log")
	f2, err := logging.NewJSONFileLogger(ql2File)
	require.NoError(t, err)
	ql3File := filepath.Join(tmpDir, "query3.log")
	f3, err := logging.NewJSONFileLogger(ql3File)
	require.NoError(t, err)
	engine.SetQueryLogger(f2)
	queryExec()
	engine.SetQueryLogger(f3)
	err = f2.Close()
	require.ErrorContains(t, err, "file already closed", "expected f2 to be closed, got open")
	queryExec()
	err = f3.Close()
	require.NoError(t, err)
}

func TestQueryLogger_fields(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	tmpDir := t.TempDir()
	ql1File := filepath.Join(tmpDir, "query1.log")
	f1, err := logging.NewJSONFileLogger(ql1File)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f1.Close())
	})

	engine.SetQueryLogger(f1)

	ctx, cancelCtx := context.WithCancel(context.Background())
	ctx = promql.NewOriginContext(ctx, map[string]any{"foo": "bar"})
	defer cancelCtx()
	query := engine.NewTestQuery(func(ctx context.Context) error {
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec(ctx)
	require.NoError(t, res.Err)

	logLines := getLogLines(t, ql1File)
	require.Contains(t, logLines[0], "foo", "bar")
}

func TestQueryLogger_error(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	tmpDir := t.TempDir()
	ql1File := filepath.Join(tmpDir, "query1.log")
	f1, err := logging.NewJSONFileLogger(ql1File)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f1.Close())
	})

	engine.SetQueryLogger(f1)

	ctx, cancelCtx := context.WithCancel(context.Background())
	ctx = promql.NewOriginContext(ctx, map[string]any{"foo": "bar"})
	defer cancelCtx()
	testErr := errors.New("failure")
	query := engine.NewTestQuery(func(context.Context) error {
		return testErr
	})

	res := query.Exec(ctx)
	require.Error(t, res.Err, "query should have failed")

	logLines := getLogLines(t, ql1File)
	require.Contains(t, logLines[0], "error", testErr)
	require.Contains(t, logLines[0], "params", map[string]any{"query": "test statement"})
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
			expected: &parser.NumberLiteral{
				Val:      123.4567,
				PosRange: posrange.PositionRange{Start: 0, End: 8},
			},
		},
		{
			input: `"foo"`,
			expected: &parser.StringLiteral{
				Val:      "foo",
				PosRange: posrange.PositionRange{Start: 0, End: 5},
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
					PosRange: posrange.PositionRange{
						Start: 0,
						End:   3,
					},
				},
				RHS: &parser.VectorSelector{
					Name: "bar",
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "bar"),
					},
					PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
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
			expected: &parser.MatrixSelector{
				VectorSelector: &parser.VectorSelector{
					Name:      "test",
					Timestamp: makeInt64Pointer(1603774699000),
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "a", "b"),
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
					},
					PosRange: posrange.PositionRange{
						Start: 0,
						End:   11,
					},
				},
				Range:  5 * 365 * 24 * time.Hour,
				EndPos: 28,
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
					PosRange: posrange.PositionRange{
						Start: 13,
						End:   24,
					},
				},
				Grouping: []string{"foo"},
				PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
							Start: 13,
							End:   29,
						},
						Timestamp: makeInt64Pointer(10000),
					},
					Grouping: []string{"foo"},
					PosRange: posrange.PositionRange{
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
							PosRange: posrange.PositionRange{
								Start: 4,
								End:   21,
							},
							Timestamp: makeInt64Pointer(10000),
						},
						PosRange: posrange.PositionRange{
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
							PosRange: posrange.PositionRange{
								Start: 29,
								End:   46,
							},
							Timestamp: makeInt64Pointer(20000),
						},
						PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
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
										PosRange: posrange.PositionRange{
											Start: 29,
											End:   40,
										},
										Timestamp: makeInt64Pointer(20000),
									},
									Range:  1 * time.Minute,
									EndPos: 49,
								},
							},
							PosRange: posrange.PositionRange{
								Start: 24,
								End:   50,
							},
						},
						Param: &parser.NumberLiteral{
							Val: 5,
							PosRange: posrange.PositionRange{
								Start: 21,
								End:   22,
							},
						},
						PosRange: posrange.PositionRange{
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
				PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
								PosRange: posrange.PositionRange{
									Start: 4,
									End:   23,
								},
								Timestamp: makeInt64Pointer(20000),
							},
							PosRange: posrange.PositionRange{
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
												PosRange: posrange.PositionRange{
													Start: 19,
													End:   33,
												},
											},
											Range:  2 * time.Second,
											EndPos: 37,
										},
									},
									PosRange: posrange.PositionRange{
										Start: 14,
										End:   38,
									},
								},
								Range:     5 * time.Minute,
								Timestamp: makeInt64Pointer(1603775091000),
								EndPos:    56,
							},
						},
						PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
								PosRange: posrange.PositionRange{
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
									PosRange: posrange.PositionRange{
										Start: 7,
										End:   27,
									},
								},
							},
						},
						PosRange: posrange.PositionRange{
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
							PosRange: posrange.PositionRange{
								Start: 8,
								End:   19,
							},
							Timestamp: makeInt64Pointer(10000),
						}},
						PosRange: posrange.PositionRange{
							Start: 4,
							End:   20,
						},
					}},
					PosRange: posrange.PositionRange{
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
								PosRange: posrange.PositionRange{
									Start: 8,
									End:   25,
								},
								Timestamp: makeInt64Pointer(10000),
							},
							PosRange: posrange.PositionRange{
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
								PosRange: posrange.PositionRange{
									Start: 33,
									End:   50,
								},
								Timestamp: makeInt64Pointer(20000),
							},
							PosRange: posrange.PositionRange{
								Start: 29,
								End:   51,
							},
						},
					},
					PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
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
					PosRange: posrange.PositionRange{
						Start: 0,
						End:   11,
					},
					Timestamp:  makeInt64Pointer(timestamp.FromTime(endTime)),
					StartOrEnd: parser.END,
				},
			},
		},
		{
			input: `sum_over_time(test[5y] @ start())`,
			expected: &parser.StepInvariantExpr{
				Expr: &parser.Call{
					Func: &parser.Function{
						Name:       "sum_over_time",
						ArgTypes:   []parser.ValueType{parser.ValueTypeMatrix},
						ReturnType: parser.ValueTypeVector,
					},
					Args: parser.Expressions{
						&parser.MatrixSelector{
							VectorSelector: &parser.VectorSelector{
								Name:       "test",
								Timestamp:  makeInt64Pointer(timestamp.FromTime(startTime)),
								StartOrEnd: parser.START,
								LabelMatchers: []*labels.Matcher{
									parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
								},
								PosRange: posrange.PositionRange{
									Start: 14,
									End:   18,
								},
							},
							Range:  5 * 365 * 24 * time.Hour,
							EndPos: 32,
						},
					},
					PosRange: posrange.PositionRange{Start: 0, End: 33},
				},
			},
		},
		{
			input: `test[5y] @ end()`,
			expected: &parser.MatrixSelector{
				VectorSelector: &parser.VectorSelector{
					Name:       "test",
					Timestamp:  makeInt64Pointer(timestamp.FromTime(endTime)),
					StartOrEnd: parser.END,
					LabelMatchers: []*labels.Matcher{
						parser.MustLabelMatcher(labels.MatchEqual, "__name__", "test"),
					},
					PosRange: posrange.PositionRange{
						Start: 0,
						End:   4,
					},
				},
				Range:  5 * 365 * 24 * time.Hour,
				EndPos: 16,
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
						PosRange: posrange.PositionRange{
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
						PosRange: posrange.PositionRange{
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
							PosRange: posrange.PositionRange{
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
										PosRange: posrange.PositionRange{
											Start: 21,
											End:   22,
										},
									},
									RHS: &parser.NumberLiteral{
										Val: 1024,
										PosRange: posrange.PositionRange{
											Start: 25,
											End:   29,
										},
									},
								},
								PosRange: posrange.PositionRange{
									Start: 20,
									End:   30,
								},
							},
						},
					},
				},
				PosRange: posrange.PositionRange{
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
			expr, err = promql.PreprocessExpr(expr, startTime, endTime, 0)
			require.NoError(t, err)
			if test.outputTest {
				require.Equal(t, test.input, expr.String(), "error on input '%s'", test.input)
			}
			require.Equal(t, test.expected, expr, "error on input '%s'", test.input)
		})
	}
}

func TestEngineOptsValidation(t *testing.T) {
	cases := []struct {
		opts     promql.EngineOpts
		query    string
		fail     bool
		expError error
	}{
		{
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "metric @ 100", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ 100)", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ 100)", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "metric @ start()", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ start())", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ start())", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "metric @ end()", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1m] @ end())", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: false},
			query: "rate(metric[1h:1m] @ end())", fail: true, expError: promql.ErrValidationAtModifierDisabled,
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: true},
			query: "metric @ 100",
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: true},
			query: "rate(metric[1m] @ start())",
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: true},
			query: "rate(metric[1h:1m] @ end())",
		}, {
			opts:  promql.EngineOpts{EnableNegativeOffset: false},
			query: "metric offset -1s", fail: true, expError: promql.ErrValidationNegativeOffsetDisabled,
		}, {
			opts:  promql.EngineOpts{EnableNegativeOffset: true},
			query: "metric offset -1s",
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: true, EnableNegativeOffset: true},
			query: "metric @ 100 offset -2m",
		}, {
			opts:  promql.EngineOpts{EnableAtModifier: true, EnableNegativeOffset: true},
			query: "metric offset -2m @ 100",
		},
	}

	for _, c := range cases {
		eng := promqltest.NewTestEngineWithOpts(t, c.opts)
		_, err1 := eng.NewInstantQuery(context.Background(), nil, nil, c.query, time.Unix(10, 0))
		_, err2 := eng.NewRangeQuery(context.Background(), nil, nil, c.query, time.Unix(0, 0), time.Unix(10, 0), time.Second)
		if c.fail {
			require.Equal(t, c.expError, err1)
			require.Equal(t, c.expError, err2)
		} else {
			require.NoError(t, err1)
			require.NoError(t, err2)
		}
	}
}

func TestEngine_Close(t *testing.T) {
	t.Run("nil engine", func(t *testing.T) {
		var ng *promql.Engine
		require.NoError(t, ng.Close())
	})

	t.Run("non-nil engine", func(t *testing.T) {
		ng := promql.NewEngine(promql.EngineOpts{
			Logger:                   nil,
			Reg:                      nil,
			MaxSamples:               0,
			Timeout:                  100 * time.Second,
			NoStepSubqueryIntervalFn: nil,
			EnableAtModifier:         true,
			EnableNegativeOffset:     true,
			EnablePerStepStats:       false,
			LookbackDelta:            0,
			EnableDelayedNameRemoval: true,
		})
		require.NoError(t, ng.Close())
	})
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
			ts:            lastDatapointTs.Add(defaultLookbackDelta - time.Millisecond),
			expectSamples: true,
		},
		{
			name:          "outside default lookback delta",
			ts:            lastDatapointTs.Add(defaultLookbackDelta),
			expectSamples: false,
		},
		{
			name:           "custom engine lookback delta",
			ts:             lastDatapointTs.Add(10*time.Minute - time.Millisecond),
			engineLookback: 10 * time.Minute,
			expectSamples:  true,
		},
		{
			name:           "outside custom engine lookback delta",
			ts:             lastDatapointTs.Add(10 * time.Minute),
			engineLookback: 10 * time.Minute,
			expectSamples:  false,
		},
		{
			name:           "custom query lookback delta",
			ts:             lastDatapointTs.Add(20*time.Minute - time.Millisecond),
			engineLookback: 10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  true,
		},
		{
			name:           "outside custom query lookback delta",
			ts:             lastDatapointTs.Add(20 * time.Minute),
			engineLookback: 10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  false,
		},
		{
			name:           "negative custom query lookback delta",
			ts:             lastDatapointTs.Add(20*time.Minute - time.Millisecond),
			engineLookback: -10 * time.Minute,
			queryLookback:  20 * time.Minute,
			expectSamples:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			engine := promqltest.NewTestEngine(t, false, c.engineLookback, promqltest.DefaultMaxSamplesPerQuery)
			storage := promqltest.LoadedStorage(t, load)

			opts := promql.NewPrometheusQueryOpts(false, c.queryLookback)
			qry, err := engine.NewInstantQuery(context.Background(), storage, opts, query, c.ts)
			require.NoError(t, err)

			res := qry.Exec(context.Background())
			require.NoError(t, res.Err)
			vec, ok := res.Value.(promql.Vector)
			require.True(t, ok)
			if c.expectSamples {
				require.NotEmpty(t, vec)
			} else {
				require.Empty(t, vec)
			}
		})
	}
}

func makeInt64Pointer(val int64) *int64 {
	valp := new(int64)
	*valp = val
	return valp
}

func TestHistogramCopyFromIteratorRegression(t *testing.T) {
	// Loading the following histograms creates two chunks because there's a
	// counter reset. Not only the counter is lower in the last histogram
	// but also there's missing buckets.
	// This in turns means that chunk iterators will have different spans.
	load := `load 1m
histogram {{sum:4 count:4 buckets:[2 2]}} {{sum:6 count:6 buckets:[3 3]}} {{sum:1 count:1 buckets:[1]}}
`
	storage := promqltest.LoadedStorage(t, load)

	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)

	verify := func(t *testing.T, qry promql.Query, expected []histogram.FloatHistogram) {
		res := qry.Exec(context.Background())
		require.NoError(t, res.Err)

		m, ok := res.Value.(promql.Matrix)
		require.True(t, ok)

		require.Len(t, m, 1)
		series := m[0]

		require.Empty(t, series.Floats)
		require.Len(t, series.Histograms, len(expected))
		for i, e := range expected {
			series.Histograms[i].H.CounterResetHint = histogram.UnknownCounterReset // Don't care.
			require.Equal(t, &e, series.Histograms[i].H)
		}
	}

	qry, err := engine.NewRangeQuery(context.Background(), storage, nil, "increase(histogram[90s])", time.Unix(0, 0), time.Unix(0, 0).Add(60*time.Second), time.Minute)
	require.NoError(t, err)
	verify(t, qry, []histogram.FloatHistogram{
		{
			Count:           3,
			Sum:             3,                                        // Increase from 4 to 6 is 2. Interpolation adds 1.
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}}, // Two buckets changed between the first and second histogram.
			PositiveBuckets: []float64{1.5, 1.5},                      // Increase from 2 to 3 is 1 in both buckets. Interpolation adds 0.5.
		},
	})

	qry, err = engine.NewInstantQuery(context.Background(), storage, nil, "histogram[61s]", time.Unix(0, 0).Add(2*time.Minute))
	require.NoError(t, err)
	verify(t, qry, []histogram.FloatHistogram{
		{
			Count:           6,
			Sum:             6,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
			PositiveBuckets: []float64{3, 3},
		},
		{
			Count:           1,
			Sum:             1,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []float64{1},
		},
	})
}

func TestRateAnnotations(t *testing.T) {
	testCases := map[string]struct {
		data                       string
		expr                       string
		typeAndUnitLabelsEnabled   bool
		expectedWarningAnnotations []string
		expectedInfoAnnotations    []string
	}{
		"info annotation when two samples are selected": {
			data: `
				series 1 2
			`,
			expr:                       "rate(series[1m1s])",
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations: []string{
				`PromQL info: metric might not be a counter, name does not end in _total/_sum/_count/_bucket: "series" (1:6)`,
			},
		},
		"no info annotations when no samples": {
			data: `
				series
			`,
			expr:                       "rate(series[1m1s])",
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotations when selecting one sample": {
			data: `
				series 1 2
			`,
			expr:                       "rate(series[10s])",
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotations when no samples due to mixed data types": {
			data: `
				series{label="a"} 1 {{schema:1 sum:15 count:10 buckets:[1 2 3]}}
			`,
			expr: "rate(series[1m1s])",
			expectedWarningAnnotations: []string{
				`PromQL warning: encountered a mix of histograms and floats for metric name "series" (1:6)`,
			},
			expectedInfoAnnotations: []string{},
		},
		"no info annotations when selecting two native histograms": {
			data: `
				series{label="a"} {{schema:1 sum:10 count:5 buckets:[1 2 3]}} {{schema:1 sum:15 count:10 buckets:[1 2 3]}}
			`,
			expr:                       "rate(series[1m1s])",
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"info annotation when rate() over series without _total suffix": {
			data: `
				series{label="a"} 1 2 3
			`,
			expr: "rate(series[1m1s])",
			expectedInfoAnnotations: []string{
				`PromQL info: metric might not be a counter, name does not end in _total/_sum/_count/_bucket: "series" (1:6)`,
			},
			expectedWarningAnnotations: []string{},
		},
		"info annotation when increase() over series without _total suffix": {
			data: `
				series{label="a"} 1 2 3
			`,
			expr: "increase(series[1m1s])",
			expectedInfoAnnotations: []string{
				`PromQL info: metric might not be a counter, name does not end in _total/_sum/_count/_bucket: "series" (1:10)`,
			},
			expectedWarningAnnotations: []string{},
		},
		"info annotation when rate() over series without __type__=counter label": {
			data: `
				series{label="a"} 1 2 3
			`,
			expr:                     "rate(series[1m1s])",
			typeAndUnitLabelsEnabled: true,
			expectedInfoAnnotations: []string{
				`PromQL info: metric might not be a counter, __type__ label is not set to "counter" or "histogram", got "": "series" (1:6)`,
			},
			expectedWarningAnnotations: []string{},
		},
		"info annotation when increase() over series without __type__=counter label": {
			data: `
				series{label="a"} 1 2 3
			`,
			expr:                     "increase(series[1m1s])",
			typeAndUnitLabelsEnabled: true,
			expectedInfoAnnotations: []string{
				`PromQL info: metric might not be a counter, __type__ label is not set to "counter" or "histogram", got "": "series" (1:10)`,
			},
			expectedWarningAnnotations: []string{},
		},
		"no info annotation when rate() over series with __type__=counter label": {
			data: `
				series{label="a", __type__="counter"} 1 2 3
			`,
			expr:                       "rate(series[1m1s])",
			typeAndUnitLabelsEnabled:   true,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when rate() over series with __type__=histogram label": {
			data: `
				series{label="a", __type__="histogram"} 1 2 3
			`,
			expr:                       "rate(series[1m1s])",
			typeAndUnitLabelsEnabled:   true,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with __type__=counter label": {
			data: `
				series{label="a", __type__="counter"} 1 2 3
			`,
			expr:                       "increase(series[1m1s])",
			typeAndUnitLabelsEnabled:   true,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with __type__=histogram label": {
			data: `
				series{label="a", __type__="histogram"} 1 2 3
			`,
			expr:                       "increase(series[1m1s])",
			typeAndUnitLabelsEnabled:   true,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when rate() over series with _total suffix": {
			data: `
				series_total{label="a"} 1 2 3
			`,
			expr:                       "rate(series_total[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when rate() over series with _sum suffix": {
			data: `
				series_sum{label="a"} 1 2 3
			`,
			expr:                       "rate(series_sum[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when rate() over series with _count suffix": {
			data: `
				series_count{label="a"} 1 2 3
			`,
			expr:                       "rate(series_count[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when rate() over series with _bucket suffix": {
			data: `
				series_bucket{label="a"} 1 2 3
			`,
			expr:                       "rate(series_bucket[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with _total suffix": {
			data: `
				series_total{label="a"} 1 2 3
			`,
			expr:                       "increase(series_total[1m1s])",
			expectedWarningAnnotations: []string{},
			typeAndUnitLabelsEnabled:   false,
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with _sum suffix": {
			data: `
				series_sum{label="a"} 1 2 3
			`,
			expr:                       "increase(series_sum[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with _count suffix": {
			data: `
				series_count{label="a"} 1 2 3
			`,
			expr:                       "increase(series_count[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
		"no info annotation when increase() over series with _bucket suffix": {
			data: `
				series_bucket{label="a"} 1 2 3
			`,
			expr:                       "increase(series_bucket[1m1s])",
			typeAndUnitLabelsEnabled:   false,
			expectedWarningAnnotations: []string{},
			expectedInfoAnnotations:    []string{},
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			store := promqltest.LoadedStorage(t, "load 1m\n"+strings.TrimSpace(testCase.data))
			t.Cleanup(func() { _ = store.Close() })

			engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
				Logger:                   nil,
				Reg:                      nil,
				MaxSamples:               promqltest.DefaultMaxSamplesPerQuery,
				Timeout:                  100 * time.Minute,
				NoStepSubqueryIntervalFn: func(int64) int64 { return int64((1 * time.Minute / time.Millisecond / time.Nanosecond)) },
				EnableAtModifier:         true,
				EnableNegativeOffset:     true,
				EnablePerStepStats:       false,
				LookbackDelta:            0,
				EnableDelayedNameRemoval: true,
				EnableTypeAndUnitLabels:  testCase.typeAndUnitLabelsEnabled,
			})
			query, err := engine.NewInstantQuery(context.Background(), store, nil, testCase.expr, timestamp.Time(0).Add(1*time.Minute))
			require.NoError(t, err)
			t.Cleanup(query.Close)

			res := query.Exec(context.Background())
			require.NoError(t, res.Err)

			warnings, infos := res.Warnings.AsStrings(testCase.expr, 0, 0)
			testutil.RequireEqual(t, testCase.expectedWarningAnnotations, warnings)
			testutil.RequireEqual(t, testCase.expectedInfoAnnotations, infos)
		})
	}
}

func TestHistogramRateWithFloatStaleness(t *testing.T) {
	// Make a chunk with two normal histograms of the same value.
	h1 := histogram.Histogram{
		Schema:          2,
		Count:           10,
		Sum:             100,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{100},
	}

	c1 := chunkenc.NewHistogramChunk()
	app, err := c1.Appender()
	require.NoError(t, err)
	var (
		newc    chunkenc.Chunk
		recoded bool
	)

	newc, recoded, app, err = app.AppendHistogram(nil, 0, 0, h1.Copy(), false)
	require.NoError(t, err)
	require.False(t, recoded)
	require.Nil(t, newc)

	newc, recoded, _, err = app.AppendHistogram(nil, 0, 10, h1.Copy(), false)
	require.NoError(t, err)
	require.False(t, recoded)
	require.Nil(t, newc)

	// Make a chunk with a single float stale marker.
	c2 := chunkenc.NewXORChunk()
	app, err = c2.Appender()
	require.NoError(t, err)

	app.Append(0, 20, math.Float64frombits(value.StaleNaN))

	// Make a chunk with two normal histograms that have zero value.
	h2 := histogram.Histogram{
		Schema: 2,
	}

	c3 := chunkenc.NewHistogramChunk()
	app, err = c3.Appender()
	require.NoError(t, err)

	newc, recoded, app, err = app.AppendHistogram(nil, 0, 30, h2.Copy(), false)
	require.NoError(t, err)
	require.False(t, recoded)
	require.Nil(t, newc)

	newc, recoded, _, err = app.AppendHistogram(nil, 0, 40, h2.Copy(), false)
	require.NoError(t, err)
	require.False(t, recoded)
	require.Nil(t, newc)

	querier := storage.MockQuerier{
		SelectMockFunction: func(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
			return &singleSeriesSet{
				series: mockSeries{chunks: []chunkenc.Chunk{c1, c2, c3}, labelSet: []string{"__name__", "foo"}},
			}
		},
	}

	queriable := storage.MockQueryable{MockQuerier: &querier}

	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)

	q, err := engine.NewInstantQuery(context.Background(), &queriable, nil, "rate(foo[40s])", timestamp.Time(45))
	require.NoError(t, err)
	defer q.Close()

	res := q.Exec(context.Background())
	require.NoError(t, res.Err)

	vec, err := res.Vector()
	require.NoError(t, err)

	// Single sample result.
	require.Len(t, vec, 1)
	// The result is a histogram.
	require.NotNil(t, vec[0].H)
	// The result should be zero as the histogram has not increased, so the rate is zero.
	require.Equal(t, 0.0, vec[0].H.Count)
	require.Equal(t, 0.0, vec[0].H.Sum)
}

type singleSeriesSet struct {
	series   storage.Series
	consumed bool
}

func (s *singleSeriesSet) Next() bool                     { c := s.consumed; s.consumed = true; return !c }
func (s singleSeriesSet) At() storage.Series              { return s.series }
func (singleSeriesSet) Err() error                        { return nil }
func (singleSeriesSet) Warnings() annotations.Annotations { return nil }

type mockSeries struct {
	chunks   []chunkenc.Chunk
	labelSet []string
}

func (s mockSeries) Labels() labels.Labels {
	return labels.FromStrings(s.labelSet...)
}

func (s mockSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	iterables := []chunkenc.Iterator{}
	for _, c := range s.chunks {
		iterables = append(iterables, c.Iterator(nil))
	}
	return storage.ChainSampleIteratorFromIterators(it, iterables)
}

func TestEvaluationWithDelayedNameRemovalDisabled(t *testing.T) {
	opts := promql.EngineOpts{
		Logger:                   nil,
		Reg:                      nil,
		EnableAtModifier:         true,
		MaxSamples:               10000,
		Timeout:                  10 * time.Second,
		EnableDelayedNameRemoval: false,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	promqltest.RunTest(t, `
load 5m
	metric_total{env="1"}	0 60 120
	another_metric{env="1"}	60 120 180

# Does not drop __name__ for vector selector
eval instant at 10m metric_total{env="1"}
	metric_total{env="1"} 120

# Drops __name__ for unary operators
eval instant at 10m -metric_total
	{env="1"} -120

# Drops __name__ for binary operators
eval instant at 10m metric_total + another_metric
	{env="1"} 300

# Does not drop __name__ for binary comparison operators
eval instant at 10m metric_total <= another_metric
	metric_total{env="1"} 120

# Drops __name__ for binary comparison operators with "bool" modifier
eval instant at 10m metric_total <= bool another_metric
	{env="1"} 1

# Drops __name__ for vector-scalar operations
eval instant at 10m metric_total * 2
	{env="1"} 240

# Drops __name__ for instant-vector functions
eval instant at 10m clamp(metric_total, 0, 100)
	{env="1"} 100

# Drops __name__ for round function
eval instant at 10m round(metric_total)
	{env="1"} 120

# Drops __name__ for range-vector functions
eval instant at 10m rate(metric_total{env="1"}[10m])
	{env="1"} 0.2

# Does not drop __name__ for last_over_time function
eval instant at 10m last_over_time(metric_total{env="1"}[10m])
	metric_total{env="1"} 120

# Does not drop __name__ for first_over_time function
eval instant at 10m first_over_time(metric_total{env="1"}[10m])
	metric_total{env="1"} 60

# Drops name for other _over_time functions
eval instant at 10m max_over_time(metric_total{env="1"}[10m])
	{env="1"} 120

clear

load 1m
	float{a="b"}									2x10
	classic_histogram_invalid{abe="0.1"}			2x10
	native_histogram{a="b", c="d"}					{{sum:0 count:0}}x10
    histogram_nan{case="100% NaNs"} 				{{schema:0 count:0 sum:0}} {{schema:0 count:3 sum:NaN}}
    histogram_nan{case="20% NaNs"} 					{{schema:0 count:0 sum:0}} {{schema:0 count:15 sum:NaN buckets:[12]}}
	conflicting_histogram{le="0.1"}					2x10
	conflicting_histogram							{{sum:0}}x10
	nonmonotonic_bucket{le="0.1"}   				0+2x10
	nonmonotonic_bucket{le="1"}     				0+1x10
	nonmonotonic_bucket{le="10"}    				0+5x10
	nonmonotonic_bucket{le="100"}   				0+4x10
	nonmonotonic_bucket{le="1000"}  				0+9x10
	nonmonotonic_bucket{le="+Inf"}  				0+8x10

eval instant at 1m histogram_quantile(0.5, classic_histogram_invalid)
	expect warn msg: PromQL warning: bucket label "le" is missing or has a malformed value of ""

eval instant at 1m histogram_fraction(0.5, 1, conflicting_histogram)
	expect warn msg: PromQL warning: vector contains a mix of classic and native histograms

eval instant at 1m histogram_quantile(0.5, nonmonotonic_bucket)
	expect info msg: PromQL info: input to histogram_quantile needed to be fixed for monotonicity (see https://prometheus.io/docs/prometheus/latest/querying/functions/#histogram_quantile)
	{} 8.5

eval instant at 1m histogram_quantile(1, histogram_nan)
    expect info msg: PromQL info: input to histogram_quantile has NaN observations, result is NaN
    {case="100% NaNs"} NaN
    {case="20% NaNs"} NaN

eval instant at 1m histogram_quantile(0.8, histogram_nan{case="20% NaNs"})
    expect info msg: PromQL info: input to histogram_quantile has NaN observations, result is skewed higher
    {case="20% NaNs"} 1

eval instant at 1m histogram_fraction(-Inf, 0.7071067811865475, histogram_nan)
    expect info msg: PromQL info: input to histogram_fraction has NaN observations, which are excluded from all fractions
    {case="100% NaNs"} 0.0
    {case="20% NaNs"} 0.4

# Test unary negation with non-overlapping series that have different metric names.
# After negation, the __name__ label is dropped, so series with different names
# but same other labels should merge if they don't overlap in time.
clear
load 20m
  http_requests{job="api"} 2 _
  http_errors{job="api"} _ 4

eval instant at 0 -{job="api"}
  {job="api"} -2

eval instant at 20m -{job="api"}
  {job="api"} -4

eval range from 0 to 20m step 20m -{job="api"}
  {job="api"} -2 -4

# Test unary negation failure with overlapping timestamps (same labelset at same time).
clear
load 1m
  http_requests{job="api"} 1
  http_errors{job="api"} 2

eval_fail instant at 0 -{job="api"}

# Test unary negation with "or" operator combining metrics with removed names.
clear
load 10m
    metric_a   1  _
    metric_b   3  4

# Use "-" unary operator as a simple way to remove the metric name.
eval range from 0 to 20m step 10m -metric_a or -metric_b
    {} -1  -4

`, engine)
}

// TestInconsistentHistogramCount is testing for
// https://github.com/prometheus/prometheus/issues/16681 which needs mixed
// integer and float histograms. The promql test framework only uses float
// histograms, so we cannot move this to promql tests.
func TestInconsistentHistogramCount(t *testing.T) {
	dir := t.TempDir()

	opts := tsdb.DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	// We use TSDB head only. By using full TSDB DB, and appending samples to it, closing it would cause unnecessary HEAD compaction, which slows down the test.
	head, err := tsdb.NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = head.Close()
	})

	app := head.Appender(context.Background())

	l := labels.FromStrings("__name__", "series_1")

	mint := time.Now().Add(-time.Hour).UnixMilli()
	maxt := mint
	dT := int64(15 * 1000) // 15 seconds in milliseconds.
	inHs := []*histogram.Histogram{
		{
			Count: 2,
			Sum:   100,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 1},
			},
			PositiveBuckets: []int64{2},
		},
		{
			Count: 4,
			Sum:   200,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 1},
			},
			PositiveBuckets: []int64{4},
		},
		{}, // Empty histogram to trigger counter reset.
	}

	for i, h := range inHs {
		maxt = mint + dT*int64(i)
		_, err = app.AppendHistogram(0, l, maxt, h, nil)
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	queryable := storage.QueryableFunc(func(mint, maxt int64) (storage.Querier, error) {
		return tsdb.NewBlockQuerier(head, mint, maxt)
	})

	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples:    1000000,
		Timeout:       10 * time.Second,
		LookbackDelta: 5 * time.Minute,
	})

	var (
		query       promql.Query
		queryResult *promql.Result
		v           promql.Vector
	)

	query, err = engine.NewInstantQuery(context.Background(), queryable, nil, "(rate(series_1[1m]))", time.UnixMilli(maxt))
	require.NoError(t, err)
	queryResult = query.Exec(context.Background())
	require.NoError(t, queryResult.Err)
	require.NotNil(t, queryResult)
	v, err = queryResult.Vector()
	require.NoError(t, err)
	require.Len(t, v, 1)
	require.NotNil(t, v[0].H)
	require.Greater(t, v[0].H.Count, float64(0))
	query.Close()
	countFromHistogram := v[0].H.Count

	query, err = engine.NewInstantQuery(context.Background(), queryable, nil, "histogram_count((rate(series_1[1m])))", time.UnixMilli(maxt))
	require.NoError(t, err)
	queryResult = query.Exec(context.Background())
	require.NoError(t, queryResult.Err)
	require.NotNil(t, queryResult)
	v, err = queryResult.Vector()
	require.NoError(t, err)
	require.Len(t, v, 1)
	require.Nil(t, v[0].H)
	query.Close()
	countFromFunction := float64(v[0].F)

	require.Equal(t, countFromHistogram, countFromFunction, "histogram_count function should return the same count as the histogram itself")
}

// TestSubQueryHistogramsCopy reproduces a bug where native histogram values from subqueries are not copied.
func TestSubQueryHistogramsCopy(t *testing.T) {
	start := time.Unix(0, 0)
	end := time.Unix(144, 0)
	step := time.Second * 3

	load := `load 2m
			http_request_duration_seconds{pod="nginx-1"} {{schema:0 count:110 sum:818.00 buckets:[1 14 95]}}+{{schema:0 count:110 buckets:[1 14 95]}}x20
			http_request_duration_seconds{pod="nginx-2"} {{schema:0 count:210 sum:1598.00 buckets:[1 19 190]}}+{{schema:0 count:210 buckets:[1 19 190]}}x30`

	subQuery := `min_over_time({__name__="http_request_duration_seconds"}[1h:1m])`
	testQuery := `rate({__name__="http_request_duration_seconds"}[3m])`
	ctx := context.Background()

	for range 100 {
		queryable := promqltest.LoadedStorage(t, load)
		engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)

		q, err := engine.NewRangeQuery(ctx, queryable, nil, subQuery, start, end, step)
		require.NoError(t, err)
		q.Exec(ctx)
		q.Close()
		queryable.Close()
	}

	for range 100 {
		queryable := promqltest.LoadedStorage(t, load)
		engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)

		q, err := engine.NewRangeQuery(ctx, queryable, nil, testQuery, start, end, step)
		require.NoError(t, err)
		result := q.Exec(ctx)

		mat, err := result.Matrix()
		require.NoError(t, err)

		for _, s := range mat {
			for _, h := range s.Histograms {
				require.NotEmpty(t, h.H.PositiveBuckets)
			}
		}
		q.Close()
		queryable.Close()
	}
}

func TestHistogram_CounterResetHint(t *testing.T) {
	baseT := timestamp.Time(0)
	load := `
		load 2m
		test_histogram {{count:0}}+{{count:1}}x4
	`
	testCases := []struct {
		name  string
		query string
		want  histogram.CounterResetHint
	}{
		{
			name:  "adding histograms",
			query: `test_histogram + test_histogram`,
			want:  histogram.NotCounterReset,
		},
		{
			name:  "subtraction",
			query: `test_histogram - test_histogram`,
			want:  histogram.GaugeType,
		},
		{
			name:  "negated addition",
			query: `test_histogram + (-test_histogram)`,
			want:  histogram.GaugeType,
		},
		{
			name:  "negated subtraction",
			query: `test_histogram - (-test_histogram)`,
			want:  histogram.GaugeType,
		},
		{
			name:  "unary negation",
			query: `-test_histogram`,
			want:  histogram.GaugeType,
		},
		{
			name:  "repeated unary negation",
			query: `-(-test_histogram)`,
			want:  histogram.GaugeType,
		},
		{
			name:  "multiplication",
			query: `2 * test_histogram`,
			want:  histogram.NotCounterReset,
		},
		{
			name:  "negative multiplication",
			query: `-1 * test_histogram`,
			want:  histogram.GaugeType,
		},
		{
			name:  "division",
			query: `test_histogram / 2`,
			want:  histogram.NotCounterReset,
		},
		{
			name:  "negative division",
			query: `test_histogram / -1`,
			want:  histogram.GaugeType,
		},
		// Equality and unequality should always return the LHS:
		{
			name:  "equality (not counter reset)",
			query: `test_histogram == test_histogram`,
			want:  histogram.NotCounterReset,
		},
		{
			name:  "equality (gauge)",
			query: `-test_histogram == -test_histogram`,
			want:  histogram.GaugeType,
		},
		{
			name:  "unequality (not counter reset)",
			query: `test_histogram != -test_histogram`,
			want:  histogram.NotCounterReset,
		},
		{
			name:  "unequality (gauge)",
			query: `-test_histogram != test_histogram`,
			want:  histogram.GaugeType,
		},
	}
	ctx := context.Background()
	queryable := promqltest.LoadedStorage(t, load)
	defer queryable.Close()
	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := engine.NewInstantQuery(ctx, queryable, nil, tc.query, baseT.Add(2*time.Minute))
			require.NoError(t, err)
			defer q.Close()
			res := q.Exec(ctx)
			require.NoError(t, res.Err)
			v, err := res.Vector()
			require.NoError(t, err)
			require.Len(t, v, 1)
			require.NotNil(t, v[0].H)
			require.Equal(t, tc.want, v[0].H.CounterResetHint)
		})
	}
}
