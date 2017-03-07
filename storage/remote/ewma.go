package remote

import (
	"sync"
	"sync/atomic"
	"time"
)

// ewmaRate tracks an exponentially weighted moving average of a per-second rate.
type ewmaRate struct {
	newEvents int64
	alpha     float64
	interval  time.Duration
	lastRate  float64
	init      bool
	mutex     sync.Mutex
}

func newEWMARate(alpha float64, interval time.Duration) ewmaRate {
	return ewmaRate{
		alpha:    alpha,
		interval: interval,
	}
}

// rate returns the per-second rate.
func (r *ewmaRate) rate() float64 {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.lastRate
}

// tick assumes to be called every r.interval.
func (r *ewmaRate) tick() {
	newEvents := atomic.LoadInt64(&r.newEvents)
	atomic.AddInt64(&r.newEvents, -newEvents)
	instantRate := float64(newEvents) / r.interval.Seconds()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.init {
		r.lastRate += r.alpha * (instantRate - r.lastRate)
	} else {
		r.init = true
		r.lastRate = instantRate
	}
}

// inc counts one event.
func (r *ewmaRate) incr(incr int64) {
	atomic.AddInt64(&r.newEvents, incr)
}
