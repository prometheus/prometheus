// Copyright 2019 Tobias Guggenmos
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

package cache

import (
	"sync"
	"sync/atomic"
)

// waitGroup is a simplified version of sync.WaitGroup
//
// The main difference is that it doesn't mind having multiple
// waiting goroutines, which would cause a sync.WaitGroup to panic.
// Also it does not have the restriction that Wait must not be called before Add
//
// This data structure must be handled with more care than a sync.WaitGroup().
//
// Basically it behaves similar to a condition variable that broadcasts on a counter hitting 0.
// By the time Wait() returns this might already be no longer the case, so the monitored data structure
// must be locked and additional checks whether the Wait Condtion is fulfilled must be performed.
type waitGroup struct {
	workers int32
	cond    *sync.Cond
	mu      sync.Mutex
}

func (wg *waitGroup) initialize() {
	if wg.cond != nil {
		panic("waitGroup initialized twice")
	}

	wg.cond = sync.NewCond(&wg.mu)
}

// Add changes the waitGroup counter by the specified amount.
// It panics if the counter becomes < 0.
func (wg *waitGroup) Add(delta int32) {
	new := atomic.AddInt32(&wg.workers, delta)

	if new < 0 {
		panic("waitGroup waiting for negative number of jobs")
	}

	if new == 0 {
		wg.mu.Lock()
		defer wg.mu.Unlock()

		wg.cond.Broadcast()
	}
}

// Done decrements the waitGroup counter.
func (wg *waitGroup) Done() {
	wg.Add(-1)
}

// Wait blocks until the WaitGroup is zero
// Beware that this might already have changed when the Wait returns.
func (wg *waitGroup) Wait() {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	for atomic.LoadInt32(&wg.workers) != 0 {
		wg.cond.Wait()
	}
}
