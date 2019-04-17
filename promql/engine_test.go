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
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestQueryConcurrency(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       100 * time.Second,
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

	for i := 0; i < opts.MaxConcurrent; i++ {
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
	for i := 0; i < opts.MaxConcurrent; i++ {
		block <- struct{}{}
	}
}

func TestQueryTimeout(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 20,
		MaxSamples:    10,
		Timeout:       5 * time.Millisecond,
	}
	engine := NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	query := engine.newTestQuery(func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec(ctx)
	if res.Err == nil {
		t.Fatalf("expected timeout error but got none")
	}
	if _, ok := res.Err.(ErrQueryTimeout); res.Err != nil && !ok {
		t.Fatalf("expected timeout error but got: %s", res.Err)
	}
}

func TestQueryCancel(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
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

	if res.Err == nil {
		t.Fatalf("expected cancellation error for query1 but got none")
	}
	if ee := ErrQueryCanceled("test statement execution"); res.Err != ee {
		t.Fatalf("expected error %q, got %q", ee, res.Err)
	}

	// Canceling a query before starting it must have no effect.
	query2 := engine.newTestQuery(func(ctx context.Context) error {
		return contextDone(ctx, "test statement execution")
	})

	query2.Cancel()
	res = query2.Exec(ctx)
	if res.Err != nil {
		t.Fatalf("unexpected error on executing query2: %s", res.Err)
	}
}

// errQuerier implements storage.Querier which always returns error.
type errQuerier struct {
	err error
}

func (q *errQuerier) Select(*storage.SelectParams, ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	return errSeriesSet{err: q.err}, nil, q.err
}
func (*errQuerier) LabelValues(name string) ([]string, error) { return nil, nil }
func (*errQuerier) LabelNames() ([]string, error)             { return nil, nil }
func (*errQuerier) Close() error                              { return nil }

// errSeriesSet implements storage.SeriesSet which always returns error.
type errSeriesSet struct {
	err error
}

func (errSeriesSet) Next() bool         { return false }
func (errSeriesSet) At() storage.Series { return nil }
func (e errSeriesSet) Err() error       { return e.err }

func TestQueryError(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
	}
	engine := NewEngine(opts)
	errStorage := ErrStorage{errors.New("storage error")}
	queryable := storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return &errQuerier{err: errStorage}, nil
	})
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	vectorQuery, err := engine.NewInstantQuery(queryable, "foo", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("unexpected error creating query: %q", err)
	}
	res := vectorQuery.Exec(ctx)
	if res.Err == nil {
		t.Fatalf("expected error on failed select but got none")
	}
	if res.Err != errStorage {
		t.Fatalf("expected error %q, got %q", errStorage, res.Err)
	}

	matrixQuery, err := engine.NewInstantQuery(queryable, "foo[1m]", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("unexpected error creating query: %q", err)
	}
	res = matrixQuery.Exec(ctx)
	if res.Err == nil {
		t.Fatalf("expected error on failed select but got none")
	}
	if res.Err != errStorage {
		t.Fatalf("expected error %q, got %q", errStorage, res.Err)
	}
}

// paramCheckerQuerier implements storage.Querier which checks the start and end times
// in params.
type paramCheckerQuerier struct {
	start int64
	end   int64

	t *testing.T
}

func (q *paramCheckerQuerier) Select(sp *storage.SelectParams, _ ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	testutil.Equals(q.t, q.start, sp.Start)
	testutil.Equals(q.t, q.end, sp.End)

	return errSeriesSet{err: nil}, nil, nil
}
func (*paramCheckerQuerier) LabelValues(name string) ([]string, error) { return nil, nil }
func (*paramCheckerQuerier) LabelNames() ([]string, error)             { return nil, nil }
func (*paramCheckerQuerier) Close() error                              { return nil }

func TestParamsSetCorrectly(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
	}

	// Set the lookback to be smaller and reset at the end.
	currLookback := LookbackDelta
	LookbackDelta = 5 * time.Second
	defer func() {
		LookbackDelta = currLookback
	}()

	cases := []struct {
		query string

		// All times are in seconds.
		start int64
		end   int64

		paramStart int64
		paramEnd   int64
	}{{
		query: "foo",
		start: 10,

		paramStart: 5,
		paramEnd:   10,
	}, {
		query: "foo[2m]",
		start: 200,

		paramStart: 80, // 200 - 120
		paramEnd:   200,
	}, {
		query: "foo[2m] offset 2m",
		start: 300,

		paramStart: 60,
		paramEnd:   180,
	}, {
		query: "foo[2m:1s]",
		start: 300,

		paramStart: 175, // 300 - 120 - 5
		paramEnd:   300,
	}, {
		query: "count_over_time(foo[2m:1s])",
		start: 300,

		paramStart: 175, // 300 - 120 - 5
		paramEnd:   300,
	}, {
		query: "count_over_time(foo[2m:1s] offset 10s)",
		start: 300,

		paramStart: 165, // 300 - 120 - 5 - 10
		paramEnd:   300,
	}, {
		query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)",
		start: 300,

		paramStart: 155, // 300 - 120 - 5 - 10 - 10
		paramEnd:   290,
	}, {
		// Range queries now.
		query: "foo",
		start: 10,
		end:   20,

		paramStart: 5,
		paramEnd:   20,
	}, {
		query: "rate(foo[2m])",
		start: 200,
		end:   500,

		paramStart: 80, // 200 - 120
		paramEnd:   500,
	}, {
		query: "rate(foo[2m] offset 2m)",
		start: 300,
		end:   500,

		paramStart: 60,
		paramEnd:   380,
	}, {
		query: "rate(foo[2m:1s])",
		start: 300,
		end:   500,

		paramStart: 175, // 300 - 120 - 5
		paramEnd:   500,
	}, {
		query: "count_over_time(foo[2m:1s])",
		start: 300,
		end:   500,

		paramStart: 175, // 300 - 120 - 5
		paramEnd:   500,
	}, {
		query: "count_over_time(foo[2m:1s] offset 10s)",
		start: 300,
		end:   500,

		paramStart: 165, // 300 - 120 - 5 - 10
		paramEnd:   500,
	}, {
		query: "count_over_time((foo offset 10s)[2m:1s] offset 10s)",
		start: 300,
		end:   500,

		paramStart: 155, // 300 - 120 - 5 - 10 - 10
		paramEnd:   490,
	}}

	for _, tc := range cases {
		engine := NewEngine(opts)
		queryable := storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
			return &paramCheckerQuerier{start: tc.paramStart * 1000, end: tc.paramEnd * 1000, t: t}, nil
		})

		var (
			query Query
			err   error
		)
		if tc.end == 0 {
			query, err = engine.NewInstantQuery(queryable, tc.query, time.Unix(tc.start, 0))
		} else {
			query, err = engine.NewRangeQuery(queryable, tc.query, time.Unix(tc.start, 0), time.Unix(tc.end, 0), time.Second)
		}
		testutil.Ok(t, err)

		res := query.Exec(context.Background())
		testutil.Ok(t, res.Err)
	}
}

func TestEngineShutdown(t *testing.T) {
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
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

	if res.Err == nil {
		t.Fatalf("expected error on shutdown during query but got none")
	}
	if ee := ErrQueryCanceled("test statement execution"); res.Err != ee {
		t.Fatalf("expected error %q, got %q", ee, res.Err)
	}

	query2 := engine.newTestQuery(func(context.Context) error {
		t.Fatalf("reached query execution unexpectedly")
		return nil
	})

	// The second query is started after the engine shut down. It must
	// be canceled immediately.
	res2 := query2.Exec(ctx)
	if res2.Err == nil {
		t.Fatalf("expected error on querying with canceled context but got none")
	}
	if _, ok := res2.Err.(ErrQueryCanceled); !ok {
		t.Fatalf("expected cancellation error, got %q", res2.Err)
	}
}

func TestEngineEvalStmtTimestamps(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric 1 2
`)
	if err != nil {
		t.Fatalf("unexpected error creating test: %q", err)
	}
	defer test.Close()

	err = test.Run()
	if err != nil {
		t.Fatalf("unexpected error initializing test: %q", err)
	}

	cases := []struct {
		Query       string
		Result      Value
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
		if err != nil {
			t.Fatalf("unexpected error creating query: %q", err)
		}
		res := qry.Exec(test.Context())
		if c.ShouldError {
			testutil.NotOk(t, res.Err, "expected error for the query %q", c.Query)
			continue
		}
		if res.Err != nil {
			t.Fatalf("unexpected error running query: %q", res.Err)
		}
		if !reflect.DeepEqual(res.Value, c.Result) {
			t.Fatalf("unexpected result for query %q: got %q wanted %q", c.Query, res.Value.String(), c.Result.String())
		}
	}

}

func TestMaxQuerySamples(t *testing.T) {
	test, err := NewTest(t, `
load 10s
  metric 1 2
`)

	if err != nil {
		t.Fatalf("unexpected error creating test: %q", err)
	}
	defer test.Close()

	err = test.Run()
	if err != nil {
		t.Fatalf("unexpected error initializing test: %q", err)
	}

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
		if err != nil {
			t.Fatalf("unexpected error creating query: %q", err)
		}
		res := qry.Exec(test.Context())
		if res.Err != nil && res.Err != c.Result.Err {
			t.Fatalf("unexpected error running query: %q, expected to get result: %q", res.Err, c.Result.Value)
		}
		if !reflect.DeepEqual(res.Value, c.Result.Value) {
			t.Fatalf("unexpected result for query %q: got %q wanted %q", c.Query, res.Value.String(), c.Result.String())
		}
	}
}

func TestRecoverEvaluatorRuntime(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}

	var err error
	defer ev.recover(&err)

	// Cause a runtime panic.
	var a []int
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
	defer ev.recover(&err)

	panic(e)
}

func TestSubquerySelector(t *testing.T) {
	tests := []struct {
		loadString string
		cases      []struct {
			Query  string
			Result Result
			Start  time.Time
		}
	}{
		{
			loadString: `load 10s
							metric 1 2`,
			cases: []struct {
				Query  string
				Result Result
				Start  time.Time
			}{
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
			cases: []struct {
				Query  string
				Result Result
				Start  time.Time
			}{
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
	}

	SetDefaultEvaluationInterval(1 * time.Minute)
	for _, tst := range tests {
		test, err := NewTest(t, tst.loadString)

		if err != nil {
			t.Fatalf("unexpected error creating test: %q", err)
		}
		defer test.Close()

		err = test.Run()
		if err != nil {
			t.Fatalf("unexpected error initializing test: %q", err)
		}

		engine := test.QueryEngine()
		for _, c := range tst.cases {
			var err error
			var qry Query

			qry, err = engine.NewInstantQuery(test.Queryable(), c.Query, c.Start)
			if err != nil {
				t.Fatalf("unexpected error creating query: %q", err)
			}
			res := qry.Exec(test.Context())
			if res.Err != nil && res.Err != c.Result.Err {
				t.Fatalf("unexpected error running query: %q, expected to get result: %q", res.Err, c.Result.Value)
			}
			if !reflect.DeepEqual(res.Value, c.Result.Value) {
				t.Fatalf("unexpected result for query %q: got %q wanted %q", c.Query, res.Value.String(), c.Result.String())
			}
		}
	}
}
