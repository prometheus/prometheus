package semaphore

import (
	"testing"
	"time"
)

func TestSemaphoreAcquireRelease(t *testing.T) {
	sem := New(3, 1*time.Second)

	for i := 0; i < 10; i++ {
		if err := sem.Acquire(); err != nil {
			t.Error(err)
		}
		if err := sem.Acquire(); err != nil {
			t.Error(err)
		}
		if err := sem.Acquire(); err != nil {
			t.Error(err)
		}
		sem.Release()
		sem.Release()
		sem.Release()
	}
}

func TestSemaphoreBlockTimeout(t *testing.T) {
	sem := New(1, 200*time.Millisecond)

	if err := sem.Acquire(); err != nil {
		t.Error(err)
	}

	start := time.Now()
	if err := sem.Acquire(); err != ErrNoTickets {
		t.Error(err)
	}
	if start.Add(200 * time.Millisecond).After(time.Now()) {
		t.Error("semaphore did not wait long enough")
	}

	sem.Release()
	if err := sem.Acquire(); err != nil {
		t.Error(err)
	}
}

func ExampleSemaphore() {
	sem := New(3, 1*time.Second)

	for i := 0; i < 10; i++ {
		go func() {
			if err := sem.Acquire(); err != nil {
				return //could not acquire semaphore
			}
			defer sem.Release()

			// do something semaphore-guarded
		}()
	}
}
