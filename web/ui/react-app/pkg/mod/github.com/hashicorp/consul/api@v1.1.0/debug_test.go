package api

import (
	"testing"

	"github.com/hashicorp/consul/sdk/testutil"
)

func TestAPI_DebugHeap(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.EnableDebug = true
	})

	defer s.Stop()

	debug := c.Debug()
	raw, err := debug.Heap()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(raw) <= 0 {
		t.Fatalf("no response: %#v", raw)
	}
}

func TestAPI_DebugProfile(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.EnableDebug = true
	})

	defer s.Stop()

	debug := c.Debug()
	raw, err := debug.Profile(1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(raw) <= 0 {
		t.Fatalf("no response: %#v", raw)
	}
}

func TestAPI_DebugGoroutine(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.EnableDebug = true
	})

	defer s.Stop()

	debug := c.Debug()
	raw, err := debug.Goroutine()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(raw) <= 0 {
		t.Fatalf("no response: %#v", raw)
	}
}

func TestAPI_DebugTrace(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.EnableDebug = true
	})

	defer s.Stop()

	debug := c.Debug()
	raw, err := debug.Trace(1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(raw) <= 0 {
		t.Fatalf("no response: %#v", raw)
	}
}
