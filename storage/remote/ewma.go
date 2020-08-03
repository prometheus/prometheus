// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

// ewmaRate tracks an exponentially weighted moving average of a per-second rate.
type ewmaRate struct {
	newEvents atomic.Int64

	alpha    float64
	interval time.Duration
	lastRate float64
	init     bool
	mutex    sync.Mutex
}

// newEWMARate always allocates a new ewmaRate, as this guarantees the atomically
// accessed int64 will be aligned on ARM.  See prometheus#2666.
func newEWMARate(alpha float64, interval time.Duration) *ewmaRate {
	return &ewmaRate{
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
	newEvents := r.newEvents.Swap(0)
	instantRate := float64(newEvents) / r.interval.Seconds()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.init {
		r.lastRate += r.alpha * (instantRate - r.lastRate)
	} else if newEvents > 0 {
		r.init = true
		r.lastRate = instantRate
	}
}

// inc counts one event.
func (r *ewmaRate) incr(incr int64) {
	r.newEvents.Add(incr)
}
