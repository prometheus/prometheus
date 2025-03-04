package ready

import (
	"sync"
	"sync/atomic"
)

type Notifier interface {
	NotifyReady()
}

type Notifiable interface {
	ReadyChan() <-chan struct{}
}

type Builder struct {
	input  []Notifiable
	output chan struct{}
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) With(notifiable Notifiable) *Builder {
	b.input = append(b.input, notifiable)
	return b
}

type MultiNotifiable struct {
	readyOnce  sync.Once
	ready      chan struct{}
	closedOnce sync.Once
	closed     chan struct{}
	counter    atomic.Int64
}

func (mn *MultiNotifiable) ReadyChan() <-chan struct{} {
	return mn.ready
}

func (mn *MultiNotifiable) setReady() {
	mn.readyOnce.Do(func() {
		close(mn.ready)
	})
}

func (mn *MultiNotifiable) setClosed() {
	mn.closedOnce.Do(func() {
		close(mn.closed)
	})
}

func (mn *MultiNotifiable) Close() error {
	mn.setClosed()
	return nil
}

func (b *Builder) Build() *MultiNotifiable {
	mn := &MultiNotifiable{
		ready:  make(chan struct{}),
		closed: make(chan struct{}),
	}

	mn.counter.Add(int64(len(b.input)))
	for _, notifiable := range b.input {
		go func(notifiable Notifiable) {
			select {
			case <-notifiable.ReadyChan():
				if mn.counter.Add(-1) == 0 {
					mn.setReady()
				}
			case <-mn.closed:
			}
		}(notifiable)
	}

	return mn
}

type NotifiableNotifier struct {
	once sync.Once
	c    chan struct{}
}

func (nn *NotifiableNotifier) NotifyReady() {
	nn.once.Do(func() {
		close(nn.c)
	})
}

func (nn *NotifiableNotifier) ReadyChan() <-chan struct{} {
	return nn.c
}

func NewNotifiableNotifier() *NotifiableNotifier {
	return &NotifiableNotifier{
		c: make(chan struct{}),
	}
}

type NoOpNotifier struct {
}

func (n NoOpNotifier) NotifyReady() {}
