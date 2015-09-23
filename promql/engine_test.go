package promql

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"
)

var noop = testStmt(func(context.Context) error {
	return nil
})

func TestQueryConcurrency(t *testing.T) {
	engine := NewEngine(nil, nil)
	defer engine.Stop()

	block := make(chan struct{})
	processing := make(chan struct{})

	f := func(context.Context) error {
		processing <- struct{}{}
		<-block
		return nil
	}

	for i := 0; i < DefaultEngineOptions.MaxConcurrentQueries; i++ {
		q := engine.newTestQuery(f)
		go q.Exec()
		select {
		case <-processing:
			// Expected.
		case <-time.After(20 * time.Millisecond):
			t.Fatalf("Query within concurrency threshold not being executed")
		}
	}

	q := engine.newTestQuery(f)
	go q.Exec()

	select {
	case <-processing:
		t.Fatalf("Query above concurrency threhosld being executed")
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
	for i := 0; i < DefaultEngineOptions.MaxConcurrentQueries; i++ {
		block <- struct{}{}
	}
}

func TestQueryTimeout(t *testing.T) {
	engine := NewEngine(nil, &EngineOptions{
		Timeout:              5 * time.Millisecond,
		MaxConcurrentQueries: 20,
	})
	defer engine.Stop()

	query := engine.newTestQuery(func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return contextDone(ctx, "test statement execution")
	})

	res := query.Exec()
	if res.Err == nil {
		t.Fatalf("expected timeout error but got none")
	}
	if _, ok := res.Err.(ErrQueryTimeout); res.Err != nil && !ok {
		t.Fatalf("expected timeout error but got: %s", res.Err)
	}
}

func TestQueryCancel(t *testing.T) {
	engine := NewEngine(nil, nil)
	defer engine.Stop()

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
		res = query1.Exec()
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
	res = query2.Exec()
	if res.Err != nil {
		t.Fatalf("unexpeceted error on executing query2: %s", res.Err)
	}
}

func TestEngineShutdown(t *testing.T) {
	engine := NewEngine(nil, nil)

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
		res = query1.Exec()
		processing <- struct{}{}
	}()

	<-processing
	engine.Stop()
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
	res2 := query2.Exec()
	if res2.Err == nil {
		t.Fatalf("expected error on querying shutdown engine but got none")
	}
	if _, ok := res2.Err.(ErrQueryCanceled); !ok {
		t.Fatalf("expected cancelation error, got %q", res2.Err)
	}
}

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var ev *evaluator
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
	var ev *evaluator
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
