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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
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

func (q *errQuerier) Select(*storage.SelectParams, ...*labels.Matcher) (storage.SeriesSet, error) {
	return errSeriesSet{err: q.err}, q.err
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
	errStorage := ErrStorage{fmt.Errorf("storage error")}
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
				Scalar{V: 1, T: 1000}},
			Start: time.Unix(1, 0),
		},
		{
			Query:      "1",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
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
			},
			Start: time.Unix(10, 0),
		},
		{
			Query:      "metric[20s]",
			MaxSamples: 0,
			Result: Result{
				ErrTooManySamples(env),
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

	e := fmt.Errorf("custom error")

	defer func() {
		if err.Error() != e.Error() {
			t.Fatalf("wrong error message: %q, expected %q", err, e)
		}
	}()
	defer ev.recover(&err)

	panic(e)
}
