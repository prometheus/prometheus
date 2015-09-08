// Package batcher implements the batching resiliency pattern for Go.
package batcher

import (
	"sync"
	"time"
)

type work struct {
	param  interface{}
	future chan error
}

// Batcher implements the batching resiliency pattern
type Batcher struct {
	timeout   time.Duration
	prefilter func(interface{}) error

	lock   sync.Mutex
	submit chan *work
	doWork func([]interface{}) error
}

// New constructs a new batcher that will batch all calls to Run that occur within
// `timeout` time before calling doWork just once for the entire batch. The doWork
// function must be safe to run concurrently with itself as this may occur, especially
// when the timeout is small.
func New(timeout time.Duration, doWork func([]interface{}) error) *Batcher {
	return &Batcher{
		timeout: timeout,
		doWork:  doWork,
	}
}

// Run runs the work function with the given parameter, possibly
// including it in a batch with other calls to Run that occur within the
// specified timeout. It is safe to call Run concurrently on the same batcher.
func (b *Batcher) Run(param interface{}) error {
	if b.prefilter != nil {
		if err := b.prefilter(param); err != nil {
			return err
		}
	}

	if b.timeout == 0 {
		return b.doWork([]interface{}{param})
	}

	w := &work{
		param:  param,
		future: make(chan error, 1),
	}

	b.submitWork(w)

	return <-w.future
}

// Prefilter specifies an optional function that can be used to run initial checks on parameters
// passed to Run before being added to the batch. If the prefilter returns a non-nil error,
// that error is returned immediately from Run and the batcher is not invoked. A prefilter
// cannot safely be specified for a batcher if Run has already been invoked. The filter function
// specified must be concurrency-safe.
func (b *Batcher) Prefilter(filter func(interface{}) error) {
	b.prefilter = filter
}

func (b *Batcher) submitWork(w *work) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.submit == nil {
		b.submit = make(chan *work, 4)
		go b.batch()
	}

	b.submit <- w
}

func (b *Batcher) batch() {
	var params []interface{}
	var futures []chan error
	input := b.submit

	go b.timer()

	for work := range input {
		params = append(params, work.param)
		futures = append(futures, work.future)
	}

	ret := b.doWork(params)

	for _, future := range futures {
		future <- ret
		close(future)
	}
}

func (b *Batcher) timer() {
	time.Sleep(b.timeout)

	b.lock.Lock()
	defer b.lock.Unlock()

	close(b.submit)
	b.submit = nil
}
