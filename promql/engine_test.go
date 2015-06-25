package promql

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"
)

var noop = testStmt(func(context.Context) error {
	return nil
})

func TestQueryConcurreny(t *testing.T) {
	engine := NewEngine(nil, nil)
	defer engine.Stop()

	block := make(chan struct{})
	processing := make(chan struct{})
	f1 := testStmt(func(context.Context) error {
		processing <- struct{}{}
		<-block
		return nil
	})

	for i := 0; i < DefaultEngineOptions.MaxConcurrentQueries; i++ {
		q := engine.newTestQuery(f1)
		go q.Exec()
		select {
		case <-processing:
			// Expected.
		case <-time.After(5 * time.Millisecond):
			t.Fatalf("Query within concurrency threshold not being executed")
		}
	}

	q := engine.newTestQuery(f1)
	go q.Exec()

	select {
	case <-processing:
		t.Fatalf("Query above concurrency threhosld being executed")
	case <-time.After(5 * time.Millisecond):
		// Expected.
	}

	// Terminate a running query.
	block <- struct{}{}

	select {
	case <-processing:
		// Expected.
	case <-time.After(5 * time.Millisecond):
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

	f1 := testStmt(func(context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	// Timeouts are not exact but checked in designated places. For example between
	// invoking test statements.
	query := engine.newTestQuery(f1, f1)

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

	// As for timeouts, cancellation is only checked at designated points. We ensure
	// that we reach one of those points using the same method.
	f1 := testStmt(func(context.Context) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})

	query1 := engine.newTestQuery(f1, f1)
	query2 := engine.newTestQuery(f1, f1)

	// Cancel query after starting it.
	var wg sync.WaitGroup
	var res *Result

	wg.Add(1)
	go func() {
		res = query1.Exec()
		wg.Done()
	}()
	time.Sleep(1 * time.Millisecond)
	query1.Cancel()
	wg.Wait()

	if res.Err == nil {
		t.Fatalf("expected cancellation error for query1 but got none")
	}
	if _, ok := res.Err.(ErrQueryCanceled); res.Err != nil && !ok {
		t.Fatalf("expected cancellation error for query1 but got: %s", res.Err)
	}

	// Canceling query before starting it must have no effect.
	query2.Cancel()
	res = query2.Exec()
	if res.Err != nil {
		t.Fatalf("unexpeceted error on executing query2: %s", res.Err)
	}
}

func TestEngineShutdown(t *testing.T) {
	engine := NewEngine(nil, nil)

	handlerExecutions := 0
	// Shutdown engine on first handler execution. Should handler execution ever become
	// concurrent this test has to be adjusted accordingly.
	f1 := testStmt(func(context.Context) error {
		handlerExecutions++
		engine.Stop()
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	query1 := engine.newTestQuery(f1, f1)
	query2 := engine.newTestQuery(f1, f1)

	// Stopping the engine must cancel the base context. While executing queries is
	// still possible, their context is canceled from the beginning and execution should
	// terminate immediately.

	res := query1.Exec()
	if res.Err == nil {
		t.Fatalf("expected error on shutdown during query but got none")
	}
	if handlerExecutions != 1 {
		t.Fatalf("expected only one handler to be executed before query cancellation but got %d executions", handlerExecutions)
	}

	res2 := query2.Exec()
	if res2.Err == nil {
		t.Fatalf("expected error on querying shutdown engine but got none")
	}
	if handlerExecutions != 1 {
		t.Fatalf("expected no handler execution for query after engine shutdown")
	}

}
