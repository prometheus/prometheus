package breaker

import (
	"errors"
	"testing"
	"time"
)

var errSomeError = errors.New("errSomeError")

func alwaysPanics() error {
	panic("foo")
}

func returnsError() error {
	return errSomeError
}

func returnsSuccess() error {
	return nil
}

func TestBreakerErrorExpiry(t *testing.T) {
	breaker := New(2, 1, 1*time.Second)

	for i := 0; i < 3; i++ {
		if err := breaker.Run(returnsError); err != errSomeError {
			t.Error(err)
		}
		time.Sleep(1 * time.Second)
	}

	for i := 0; i < 3; i++ {
		if err := breaker.Go(returnsError); err != nil {
			t.Error(err)
		}
		time.Sleep(1 * time.Second)
	}
}

func TestBreakerPanicsCountAsErrors(t *testing.T) {
	breaker := New(3, 2, 1*time.Second)

	// three errors opens the breaker
	for i := 0; i < 3; i++ {
		func() {
			defer func() {
				val := recover()
				if val.(string) != "foo" {
					t.Error("incorrect panic")
				}
			}()
			if err := breaker.Run(alwaysPanics); err != nil {
				t.Error(err)
			}
			t.Error("shouldn't get here")
		}()
	}

	// breaker is open
	for i := 0; i < 5; i++ {
		if err := breaker.Run(returnsError); err != ErrBreakerOpen {
			t.Error(err)
		}
	}
}

func TestBreakerStateTransitions(t *testing.T) {
	breaker := New(3, 2, 1*time.Second)

	// three errors opens the breaker
	for i := 0; i < 3; i++ {
		if err := breaker.Run(returnsError); err != errSomeError {
			t.Error(err)
		}
	}

	// breaker is open
	for i := 0; i < 5; i++ {
		if err := breaker.Run(returnsError); err != ErrBreakerOpen {
			t.Error(err)
		}
	}

	// wait for it to half-close
	time.Sleep(2 * time.Second)
	// one success works, but is not enough to fully close
	if err := breaker.Run(returnsSuccess); err != nil {
		t.Error(err)
	}
	// error works, but re-opens immediately
	if err := breaker.Run(returnsError); err != errSomeError {
		t.Error(err)
	}
	// breaker is open
	if err := breaker.Run(returnsError); err != ErrBreakerOpen {
		t.Error(err)
	}

	// wait for it to half-close
	time.Sleep(2 * time.Second)
	// two successes is enough to close it for good
	for i := 0; i < 2; i++ {
		if err := breaker.Run(returnsSuccess); err != nil {
			t.Error(err)
		}
	}
	// error works
	if err := breaker.Run(returnsError); err != errSomeError {
		t.Error(err)
	}
	// breaker is still closed
	if err := breaker.Run(returnsSuccess); err != nil {
		t.Error(err)
	}
}

func TestBreakerAsyncStateTransitions(t *testing.T) {
	breaker := New(3, 2, 1*time.Second)

	// three errors opens the breaker
	for i := 0; i < 3; i++ {
		if err := breaker.Go(returnsError); err != nil {
			t.Error(err)
		}
	}

	// just enough to yield the scheduler and let the goroutines work off
	time.Sleep(1 * time.Millisecond)

	// breaker is open
	for i := 0; i < 5; i++ {
		if err := breaker.Go(returnsError); err != ErrBreakerOpen {
			t.Error(err)
		}
	}

	// wait for it to half-close
	time.Sleep(2 * time.Second)
	// one success works, but is not enough to fully close
	if err := breaker.Go(returnsSuccess); err != nil {
		t.Error(err)
	}
	// error works, but re-opens immediately
	if err := breaker.Go(returnsError); err != nil {
		t.Error(err)
	}
	// just enough to yield the scheduler and let the goroutines work off
	time.Sleep(1 * time.Millisecond)
	// breaker is open
	if err := breaker.Go(returnsError); err != ErrBreakerOpen {
		t.Error(err)
	}

	// wait for it to half-close
	time.Sleep(2 * time.Second)
	// two successes is enough to close it for good
	for i := 0; i < 2; i++ {
		if err := breaker.Go(returnsSuccess); err != nil {
			t.Error(err)
		}
	}
	// just enough to yield the scheduler and let the goroutines work off
	time.Sleep(1 * time.Millisecond)
	// error works
	if err := breaker.Go(returnsError); err != nil {
		t.Error(err)
	}
	// just enough to yield the scheduler and let the goroutines work off
	time.Sleep(1 * time.Millisecond)
	// breaker is still closed
	if err := breaker.Go(returnsSuccess); err != nil {
		t.Error(err)
	}
}

func ExampleBreaker() {
	breaker := New(3, 1, 5*time.Second)

	for {
		result := breaker.Run(func() error {
			// communicate with some external service and
			// return an error if the communication failed
			return nil
		})

		switch result {
		case nil:
			// success!
		case ErrBreakerOpen:
			// our function wasn't run because the breaker was open
		default:
			// some other error
		}
	}
}
