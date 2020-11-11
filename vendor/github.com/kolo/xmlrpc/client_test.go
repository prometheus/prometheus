// +build integration

package xmlrpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

func Test_BadStatus(t *testing.T) {

	// this is a mock xmlrpc server which sends an invalid status code on the first request
	// and an empty methodResponse for all subsequence requests
	first := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if first {
			first = false
			http.Error(w, "bad status", http.StatusInternalServerError)
		} else {
			io.WriteString(w, `
				<?xml version="1.0" encoding="UTF-8"?>
				<methodResponse>
					<params>
						<param>
							<value>
								<struct></struct>
							</value>
						</param>
					</params>
				</methodResponse>
			`)
		}
	}))

	client, err := NewClient(ts.URL, nil)
	if err != nil {
		t.Fatalf("Can't create client: %v", err)
	}
	defer client.Close()

	var result interface{}

	// expect an error due to the bad status code
	if err := client.Call("method", nil, &result); err == nil {
		t.Fatalf("Bad status didn't result in error")
	}

	// expect subsequent calls to succeed
	if err := client.Call("method", nil, &result); err != nil {
		t.Fatalf("Failed to recover after bad status: %v", err)
	}
}

func newClient(t *testing.T) *Client {
	client, err := NewClient("http://localhost:5001", nil)
	if err != nil {
		t.Fatalf("Can't create client: %v", err)
	}
	return client
}
