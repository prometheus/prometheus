package util

import (
	"context"
	"errors"
	"sync/atomic"
)

// GracefulShutdowner - provides graceful shutdown.
type GracefulShutdowner struct {
	closed    atomic.Bool
	err       error
	signal    chan struct{}
	exhausted chan struct{}
	stopped   chan struct{}
}

// NewGracefulShutdowner - constructor.
func NewGracefulShutdowner() *GracefulShutdowner {
	return &GracefulShutdowner{
		signal:    make(chan struct{}),
		exhausted: make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// Shutdown - Shutdownable interface implementation.
func (s *GracefulShutdowner) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return errors.New("already closed")
	}

	close(s.signal)

	select {
	case <-ctx.Done():
		close(s.exhausted)
		<-s.stopped
	case <-s.stopped:
		close(s.exhausted)
	}

	return s.err
}

// Signal - signals that Shutdown is called.
func (s *GracefulShutdowner) Signal() <-chan struct{} {
	return s.signal
}

// NotifyContext - creates derived context.
func (s *GracefulShutdowner) NotifyContext(parentCtx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parentCtx)
	go func() {
		defer cancel()

		select {
		case <-s.signal:
			select {
			case <-s.exhausted:
			case <-parentCtx.Done():
			}
		case <-parentCtx.Done():
		}
	}()
	return ctx, cancel
}

// Done - resolves shutdowner.
func (s *GracefulShutdowner) Done(err error) {
	s.err = err
	close(s.stopped)
}
