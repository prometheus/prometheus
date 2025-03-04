package head

import (
	"errors"
	"sync/atomic"
)

type Finalizer struct {
	finalizeCalled atomic.Bool
	finalized      atomic.Bool
	done           chan struct{}
}

func NewFinalizer() *Finalizer {
	return &Finalizer{
		finalizeCalled: atomic.Bool{},
		finalized:      atomic.Bool{},
		done:           make(chan struct{}),
	}
}

func (f *Finalizer) Finalized() bool {
	return f.finalized.Load()
}

func (f *Finalizer) FinalizeCalled() bool {
	return f.finalizeCalled.Load()
}

func (f *Finalizer) Finalize(fn func() error) error {
	if !f.finalizeCalled.CompareAndSwap(false, true) {
		// concurrent or subsequent finalize call
		<-f.done
		return errors.New("already finalized")
	}

	err := fn()
	f.finalized.Store(true)
	close(f.done)
	return err
}

func (f *Finalizer) Done() <-chan struct{} {
	return f.done
}
