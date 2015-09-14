package deadline

import (
	"errors"
	"testing"
	"time"
)

func takesFiveMillis(stopper <-chan struct{}) error {
	time.Sleep(5 * time.Millisecond)
	return nil
}

func takesTwentyMillis(stopper <-chan struct{}) error {
	time.Sleep(20 * time.Millisecond)
	return nil
}

func returnsError(stopper <-chan struct{}) error {
	return errors.New("foo")
}

func TestDeadline(t *testing.T) {
	dl := New(10 * time.Millisecond)

	if err := dl.Run(takesFiveMillis); err != nil {
		t.Error(err)
	}

	if err := dl.Run(takesTwentyMillis); err != ErrTimedOut {
		t.Error(err)
	}

	if err := dl.Run(returnsError); err.Error() != "foo" {
		t.Error(err)
	}

	done := make(chan struct{})
	err := dl.Run(func(stopper <-chan struct{}) error {
		<-stopper
		close(done)
		return nil
	})
	if err != ErrTimedOut {
		t.Error(err)
	}
	<-done
}

func ExampleDeadline() {
	dl := New(1 * time.Second)

	err := dl.Run(func(stopper <-chan struct{}) error {
		// do something possibly slow
		// check stopper function and give up if timed out
		return nil
	})

	switch err {
	case ErrTimedOut:
		// execution took too long, oops
	default:
		// some other error
	}
}
