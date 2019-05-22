// +build integration

package xmlrpc

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func Test_CallWithoutArgs(t *testing.T) {
	client := newClient(t)
	defer client.Close()

	var result time.Time
	if err := client.Call("service.time", nil, &result); err != nil {
		t.Fatalf("service.time call error: %v", err)
	}
}

func Test_CallWithOneArg(t *testing.T) {
	client := newClient(t)
	defer client.Close()

	var result string
	if err := client.Call("service.upcase", "xmlrpc", &result); err != nil {
		t.Fatalf("service.upcase call error: %v", err)
	}

	if result != "XMLRPC" {
		t.Fatalf("Unexpected result of service.upcase: %s != %s", "XMLRPC", result)
	}
}

func Test_CallWithTwoArgs(t *testing.T) {
	client := newClient(t)
	defer client.Close()

	var sum int
	if err := client.Call("service.sum", []interface{}{2, 3}, &sum); err != nil {
		t.Fatalf("service.sum call error: %v", err)
	}

	if sum != 5 {
		t.Fatalf("Unexpected result of service.sum: %d != %d", 5, sum)
	}
}

func Test_TwoCalls(t *testing.T) {
	client := newClient(t)
	defer client.Close()

	var upcase string
	if err := client.Call("service.upcase", "xmlrpc", &upcase); err != nil {
		t.Fatalf("service.upcase call error: %v", err)
	}

	var sum int
	if err := client.Call("service.sum", []interface{}{2, 3}, &sum); err != nil {
		t.Fatalf("service.sum call error: %v", err)
	}

}

func Test_FailedCall(t *testing.T) {
	client := newClient(t)
	defer client.Close()

	var result int
	if err := client.Call("service.error", nil, &result); err == nil {
		t.Fatal("expected service.error returns error, but it didn't")
	}
}

func Test_ConcurrentCalls(t *testing.T) {
	client := newClient(t)

	call := func() {
		var result time.Time
		client.Call("service.time", nil, &result)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			call()
			wg.Done()
		}()
	}

	wg.Wait()
	client.Close()
}

func Test_CloseMemoryLeak(t *testing.T) {
	expected := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		client := newClient(t)
		client.Call("service.time", nil, nil)
		client.Close()
	}

	var actual int

	// It takes some time to stop running goroutinges. This function checks number of
	// running goroutines. It finishes execution if number is same as expected or timeout
	// has been reached.
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				actual = runtime.NumGoroutine()
				if actual == expected {
					return
				}
			}
		}
	}()

	if actual != expected {
		t.Errorf("expected number of running goroutines to be %d, but got %d", expected, actual)
	}
}

func newClient(t *testing.T) *Client {
	client, err := NewClient("http://localhost:5001", nil)
	if err != nil {
		t.Fatalf("Can't create client: %v", err)
	}

	return client
}
