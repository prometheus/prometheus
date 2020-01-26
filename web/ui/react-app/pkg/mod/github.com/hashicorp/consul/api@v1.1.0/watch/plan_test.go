package watch

import (
	"testing"
	"time"
)

func init() {
	watchFuncFactory["noop"] = noopWatch
}

func noopWatch(params map[string]interface{}) (WatcherFunc, error) {
	fn := func(p *Plan) (BlockingParamVal, interface{}, error) {
		idx := WaitIndexVal(0)
		if i, ok := p.lastParamVal.(WaitIndexVal); ok {
			idx = i
		}
		return idx + 1, uint64(idx + 1), nil
	}
	return fn, nil
}

func mustParse(t *testing.T, q string) *Plan {
	params := makeParams(t, q)
	plan, err := Parse(params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	return plan
}

func TestRun_Stop(t *testing.T) {
	t.Parallel()
	plan := mustParse(t, `{"type":"noop"}`)

	var expect uint64 = 1
	doneCh := make(chan struct{})
	plan.Handler = func(idx uint64, val interface{}) {
		if idx != expect {
			t.Fatalf("Bad: %d %d", expect, idx)
		}
		if val != expect {
			t.Fatalf("Bad: %d %d", expect, val)
		}
		if expect == 1 {
			close(doneCh)
		}
		expect++
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- plan.Run("127.0.0.1:8500")
	}()

	select {
	case <-doneCh:
		plan.Stop()

	case <-time.After(1 * time.Second):
		t.Fatalf("handler never ran")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("err: %v", err)
		}

	case <-time.After(1 * time.Second):
		t.Fatalf("watcher didn't exit")
	}

	if expect == 1 {
		t.Fatalf("Bad: %d", expect)
	}
}

func TestRun_Stop_Hybrid(t *testing.T) {
	t.Parallel()
	plan := mustParse(t, `{"type":"noop"}`)

	var expect uint64 = 1
	doneCh := make(chan struct{})
	plan.HybridHandler = func(blockParamVal BlockingParamVal, val interface{}) {
		idxVal, ok := blockParamVal.(WaitIndexVal)
		if !ok {
			t.Fatalf("expected index-based watch")
		}
		idx := uint64(idxVal)
		if idx != expect {
			t.Fatalf("Bad: %d %d", expect, idx)
		}
		if val != expect {
			t.Fatalf("Bad: %d %d", expect, val)
		}
		if expect == 1 {
			close(doneCh)
		}
		expect++
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- plan.Run("127.0.0.1:8500")
	}()

	select {
	case <-doneCh:
		plan.Stop()

	case <-time.After(1 * time.Second):
		t.Fatalf("handler never ran")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("err: %v", err)
		}

	case <-time.After(1 * time.Second):
		t.Fatalf("watcher didn't exit")
	}

	if expect == 1 {
		t.Fatalf("Bad: %d", expect)
	}
}
