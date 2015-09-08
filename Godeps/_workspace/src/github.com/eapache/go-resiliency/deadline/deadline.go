// Package deadline implements the deadline (also known as "timeout") resiliency pattern for Go.
package deadline

import (
	"errors"
	"time"
)

// ErrTimedOut is the error returned from Run when the deadline expires.
var ErrTimedOut = errors.New("timed out waiting for function to finish")

// Deadline implements the deadline/timeout resiliency pattern.
type Deadline struct {
	timeout time.Duration
}

// New constructs a new Deadline with the given timeout.
func New(timeout time.Duration) *Deadline {
	return &Deadline{
		timeout: timeout,
	}
}

// Run runs the given function, passing it a stopper channel. If the deadline passes before
// the function finishes executing, Run returns ErrTimeOut to the caller and closes the stopper
// channel so that the work function can attempt to exit gracefully. It does not (and cannot)
// simply kill the running function, so if it doesn't respect the stopper channel then it may
// keep running after the deadline passes. If the function finishes before the deadline, then
// the return value of the function is returned from Run.
func (d *Deadline) Run(work func(<-chan struct{}) error) error {
	result := make(chan error)
	stopper := make(chan struct{})

	go func() {
		result <- work(stopper)
	}()

	select {
	case ret := <-result:
		return ret
	case <-time.After(d.timeout):
		close(stopper)
		return ErrTimedOut
	}
}
