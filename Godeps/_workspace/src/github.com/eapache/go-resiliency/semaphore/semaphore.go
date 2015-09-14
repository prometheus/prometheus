// Package semaphore implements the semaphore resiliency pattern for Go.
package semaphore

import (
	"errors"
	"time"
)

// ErrNoTickets is the error returned by Acquire when it could not acquire
// a ticket from the semaphore within the configured timeout.
var ErrNoTickets = errors.New("could not aquire semaphore ticket")

// Semaphore implements the semaphore resiliency pattern
type Semaphore struct {
	sem     chan struct{}
	timeout time.Duration
}

// New constructs a new Semaphore with the given ticket-count
// and timeout.
func New(tickets int, timeout time.Duration) *Semaphore {
	return &Semaphore{
		sem:     make(chan struct{}, tickets),
		timeout: timeout,
	}
}

// Acquire tries to acquire a ticket from the semaphore. If it can, it returns nil.
// If it cannot after "timeout" amount of time, it returns ErrNoTickets. It is
// safe to call Acquire concurrently on a single Semaphore.
func (s *Semaphore) Acquire() error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-time.After(s.timeout):
		return ErrNoTickets
	}
}

// Release releases an acquired ticket back to the semaphore. It is safe to call
// Release concurrently on a single Semaphore. It is an error to call Release on
// a Semaphore from which you have not first acquired a ticket.
func (s *Semaphore) Release() {
	<-s.sem
}
