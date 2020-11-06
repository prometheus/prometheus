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
	"io/ioutil"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestQueryConcurrency(t *testing.T) {
	maxConcurrency := 10

	dir, err := ioutil.TempDir("", "test_concurrency")
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

	f := func(context.Context) error {
		processing <- struct{}{}
		<-block
		return nil
	}

	for i := 0; i < maxConcurrency; i++ {
		q := engine.newTestQuery(f)
		go q.Exec(ctx)
		select {
		case <-processing:
			// Expected.
		case <-time.After(20 * time.Millisecond):
			t.Fatalf("Query within concurrency threshold not being executed")
		}
	}

	q := engine.newTestQuery(f)
	go q.Exec(ctx)

	select {
	case <-processing:
		t.Fatalf("Query above concurrency threshold being executed")
	case <-time.After(20 * time.Millisecond):
		// Expected.
	}

	// Terminate a running query.
	block <- struct{}{}

	select {
	case <-processing:
		// Expected.
	case <-time.After(20 * time.Millisecond):
		t.Fatalf("Query within concurrency threshold not being executed")
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
func (*errQuerier) LabelValues(string) ([]string, storage.Warnings, error) { return nil, nil, nil }
func (*errQuerier) LabelNames() ([]string, storage.Warnings, error)        { return nil, nil, nil }
func (*errQuerier) Close() error                                           { return nil }

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

	vectorQuery, err := engine.NewInstantQuery(queryable, "foo", time.Unix(1, 0))
	require.NoError(t, err)

	res := vectorQuery.Exec(ctx)
	require.Error(t, res.Err, "expected error on failed select but got none")
	require.True(t, errors.Is(res.Err, errStorage), "expected error doesn't match")

	matrixQuery, err := engine.NewInstantQuery(queryable, "foo[1m]", time.Unix(1, 0))
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
		Logger:        nil,
		Reg:           nil,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
		LookbackDelta: 5 * time.Second,
	}

	for _, tc := range []struct {
		query string

		// All times are in milliseconds.
		start int64
		end   int64

		// TODO(bwplotka): Add support for better hints when subquerying.
		expected []*storage.SelectHints
	}{{
		query: "foo", start: 10000,
		expected: []*storage.SelectHints{
			{Start: 5000, End: 10000},
		},
	}, {
		query: "foo[2m]", start: 200000,
		expected: []*storage.SelectHints{
			{Start: 80000, End: 200000, Range: 120000},
		},
	}, {
		query: "foo[2m] offset 2m", start: 300000,
		expected: []*storage.SelectHints{
			{Start: 60000, End: 180000, Range: 120000},
		},
	}, {
		query: "foo[2m:1s]", start: 300000,
		expected: []*storage.SelectHints{
			{Start: 175000, End: 300000},
		},
	}, {
		query: "count_over_time(foo[2m:1s])", start: 300000,
		expected: []*storage.SelectHints{
			{Start: 175000, End: 300000, Func: "count_over_time"},
		},
	}, {
		query: "count_over_time(foo[2m:1s] offset 10s)", start: 300000,
		expected: []*storage.SelectHints{
			{Start: 165000, End: 290000, Func: "count_over_time"},
		},
	}, {
		query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000,
		expected: []*storage.SelectHints{
			{Start: 155000, End: 280000, Func: "count_over_time"},
		},
	}, {

		query: "foo", start: 10000, end: 20000,
		expected: []*storage.SelectHints{
			{Start: 5000, End: 20000, Step: 1000},
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
		query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)", start: 300000, end: 500000,
		expected: []*storage.SelectHints{
			{Start: 155000, End: 480000, Func: "count_over_time", Step: 1000},
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
			{Start: 0, End: 10000, Func: "max", By: true, Grouping: []string{"dim1"}},
		},
	}, {
		query: "(sum(http_requests{group=~\"p.*\"})+max(http_requests{group=~\"c.*\"}))[20s:5s]", start: 120000,
		expected: []*storage.SelectHints{
			{Start: 95000, End: 120000, Func: "sum", By: true},
			{Start: 95000, End: 120000, Func: "max", By: true},
		},
	}} {
		t.Run(tc.query, func(t *testing.T) {
			engine := NewEngine(opts)
			hintsRecorder := &noopHintRecordingQueryable{}

			var (
				query Query
				err   error
			)
			if tc.end == 0 {
				query, err = engine.NewInstantQuery(hintsRecorder, tc.query, timestamp.Time(tc.start))
			} else {
				query, err = engine.NewRangeQuery(hintsRecorder, tc.query, timestamp.Time(tc.start), timestamp.Time(tc.end), time.Second)
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
		t.Fatalf("reached query execution unexpectedly")
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
				Sample{Point: Point{V: 1, T: 1000},
					Metric: labels.FromStrings("__name__", "metric")},
			},
			Start: time.Unix(1, 0),
		},
		{
			Query: "metric[20s]",
			Result: Matrix{Series{
				Points: []Point{{V: 1, T: 0}, {V: 2, T: 10000}},
				Metric: labels.FromStrings("__name__", "metric")},
			},
			Start: time.Unix(10, 0),
		},
		// Range queries.
		{
			Query: "1",
			Result: Matrix{Series{
				Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
				Metric: labels.FromStrings()},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: Matrix{Series{
				Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
				Metric: labels.FromStrings("__name__", "metric")},
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query: "metric",
			Result: Matrix{Series{
				Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
				Metric: labels.FromStrings("__name__", "metric")},
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

	for _, c := range cases {
		var err error
		var qry Query
		if c.Interval == 0 {
			qry, err = test.QueryEngine().NewInstantQuery(test.Queryable(), c.Query, c.Start)
		} else {
			qry, err = test.QueryEngine().NewRangeQuery(test.Queryable(), c.Query, c.Start, c.End, c.Interval)
		}
		require.NoError(t, err)

		res := qry.Exec(test.Context())
		if c.ShouldError {
			require.Error(t, res.Err, "expected error for the query %q", c.Query)
			continue
		}

		require.NoError(t, res.Err)
		require.Equal(t, c.Result, res.Value, "query %q failed", c.Query)
	}
}

func TestMaxQuerySamples(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric 1 2
  bigmetric{a="1"} 1 2
  bigmetric{a="2"} 1 2
`)
	require.NoError(t, err)
	defer test.Close()

	err = test.Run()
	require.NoError(t, err)

	cases := []struct {
		Query      string
		MaxSamples int
		Result     Result
		Start      time.Time
		End        time.Time
		Interval   time.Duration
	}{
		// Instant queries.
		{
			Query:      "1",
			MaxSamples: 1,
			Result: Result{
				nil,
				Scalar{V: 1, T: 1000},
				nil},
			Start: time.Unix(1, 0),
		},
		{
			Query:      "1",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start: time.Unix(1, 0),
		},
		{
			Query:      "metric",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start: time.Unix(1, 0),
		},
		{
			Query:      "metric",
			MaxSamples: 1,
			Result: Result{
				nil,
				Vector{
					Sample{Point: Point{V: 1, T: 1000},
						Metric: labels.FromStrings("__name__", "metric")},
				},
				nil,
			},
			Start: time.Unix(1, 0),
		},
		{
			Query:      "metric[20s]",
			MaxSamples: 2,
			Result: Result{
				nil,
				Matrix{Series{
					Points: []Point{{V: 1, T: 0}, {V: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric")},
				},
				nil,
			},
			Start: time.Unix(10, 0),
		},
		{
			Query:      "rate(metric[20s])",
			MaxSamples: 3,
			Result: Result{
				nil,
				Vector{
					Sample{
						Point:  Point{V: 0.1, T: 10000},
						Metric: labels.Labels{},
					},
				},
				nil,
			},
			Start: time.Unix(10, 0),
		},
		{
			Query:      "metric[20s:5s]",
			MaxSamples: 3,
			Result: Result{
				nil,
				Matrix{Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric")},
				},
				nil,
			},
			Start: time.Unix(10, 0),
		},
		{
			Query:      "metric[20s]",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start: time.Unix(10, 0),
		},
		// Range queries.
		{
			Query:      "1",
			MaxSamples: 3,
			Result: Result{
				nil,
				Matrix{Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
					Metric: labels.FromStrings()},
				},
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query:      "1",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 3,
			Result: Result{
				nil,
				Matrix{Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 1000}, {V: 1, T: 2000}},
					Metric: labels.FromStrings("__name__", "metric")},
				},
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 2,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(2, 0),
			Interval: time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 3,
			Result: Result{
				nil,
				Matrix{Series{
					Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
					Metric: labels.FromStrings("__name__", "metric")},
				},
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(10, 0),
			Interval: 5 * time.Second,
		},
		{
			Query:      "metric",
			MaxSamples: 2,
			Result: Result{
				ErrTooManySamples(env),
				nil,
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(10, 0),
			Interval: 5 * time.Second,
		},
		{
			Query:      "rate(bigmetric[1s])",
			MaxSamples: 1,
			Result: Result{
				nil,
				Matrix{},
				nil,
			},
			Start:    time.Unix(0, 0),
			End:      time.Unix(10, 0),
			Interval: 5 * time.Second,
		},
	}

	engine := test.QueryEngine()
	for _, c := range cases {
		var err error
		var qry Query

		engine.maxSamplesPerQuery = c.MaxSamples

		if c.Interval == 0 {
			qry, err = engine.NewInstantQuery(test.Queryable(), c.Query, c.Start)
		} else {
			qry, err = engine.NewRangeQuery(test.Queryable(), c.Query, c.Start, c.End, c.Interval)
		}
		require.NoError(t, err)

		res := qry.Exec(test.Context())
		require.Equal(t, c.Result.Err, res.Err)
		require.Equal(t, c.Result.Value, res.Value, "query %q failed", c.Query)
	}
}

func TestRecoverEvaluatorRuntime(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}

	var err error
	defer ev.recover(nil, &err)

	// Cause a runtime panic.
	var a []int
	//nolint:govet
	a[123] = 1

	if err.Error() != "unexpected error" {
		t.Fatalf("wrong error message: %q, expected %q", err, "unexpected error")
	}
}

func TestRecoverEvaluatorError(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}
	var err error

	e := errors.New("custom error")

	defer func() {
		if err.Error() != e.Error() {
			t.Fatalf("wrong error message: %q, expected %q", err, e)
		}
	}()
	defer ev.recover(nil, &err)

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
		if err.Error() != e.Error() {
			t.Fatalf("wrong error message: %q, expected %q", err, e)
		}
		if len(ws) != len(warnings) && ws[0] != warnings[0] {
			t.Fatalf("wrong warning message: %q, expected %q", ws[0], warnings[0])
		}
	}()
	defer ev.recover(&ws, &err)

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
						Matrix{Series{
							Points: []Point{{V: 1, T: 0}, {V: 2, T: 10000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s]",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(10, 0),
				},
				{
					Query: "metric[20s:5s] offset 2s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(12, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 1, T: 0}, {V: 1, T: 5000}, {V: 2, T: 10000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(20, 0),
				},
				{
					Query: "metric[20s:5s] offset 4s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}, {V: 2, T: 30000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 5s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}, {V: 2, T: 30000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 6s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}},
							Metric: labels.FromStrings("__name__", "metric")},
						},
						nil,
					},
					Start: time.Unix(35, 0),
				},
				{
					Query: "metric[20s:5s] offset 7s",
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 2, T: 10000}, {V: 2, T: 15000}, {V: 2, T: 20000}, {V: 2, T: 25000}},
							Metric: labels.FromStrings("__name__", "metric")},
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
						Matrix{Series{
							Points: []Point{{V: 9990, T: 9990000}, {V: 10000, T: 10000000}, {V: 100, T: 10010000}, {V: 130, T: 10020000}},
							Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production")},
						},
						nil,
					},
					Start: time.Unix(10020, 0),
				},
				{ // Default step.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:]`,
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 9840, T: 9840000}, {V: 9900, T: 9900000}, {V: 9960, T: 9960000}, {V: 130, T: 10020000}, {V: 310, T: 10080000}},
							Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production")},
						},
						nil,
					},
					Start: time.Unix(10100, 0),
				},
				{ // Checking if high offset (>LookbackDelta) is being taken care of.
					Query: `http_requests{group=~"pro.*",instance="0"}[5m:] offset 20m`,
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 8640, T: 8640000}, {V: 8700, T: 8700000}, {V: 8760, T: 8760000}, {V: 8820, T: 8820000}, {V: 8880, T: 8880000}},
							Metric: labels.FromStrings("__name__", "http_requests", "job", "api-server", "instance", "0", "group", "production")},
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
						Matrix{Series{
							Points: []Point{{V: 270, T: 90000}, {V: 300, T: 100000}, {V: 330, T: 110000}, {V: 360, T: 120000}},
							Metric: labels.Labels{}},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `sum(http_requests)[40s:10s]`,
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 800, T: 80000}, {V: 900, T: 90000}, {V: 1000, T: 100000}, {V: 1100, T: 110000}, {V: 1200, T: 120000}},
							Metric: labels.Labels{}},
						},
						nil,
					},
					Start: time.Unix(120, 0),
				},
				{
					Query: `(sum(http_requests{group=~"p.*"})+sum(http_requests{group=~"c.*"}))[20s:5s]`,
					Result: Result{
						nil,
						Matrix{Series{
							Points: []Point{{V: 1000, T: 100000}, {V: 1000, T: 105000}, {V: 1100, T: 110000}, {V: 1100, T: 115000}, {V: 1200, T: 120000}},
							Metric: labels.Labels{}},
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
					qry, err := engine.NewInstantQuery(test.Queryable(), c.Query, c.Start)
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
