// Copyright 2025 The Prometheus Authors
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

package notifier

import (
	"sync"

	"go.uber.org/atomic"
)

type Queue struct {
	mtx          sync.RWMutex
	alerts       []*Alert
	capacity     int
	maxBatchSize int
	more         chan struct{}
	done         chan struct{}
	closed       atomic.Bool
}

func newQueue(capacity, maxBatchSize int) *Queue {
	return &Queue{
		capacity:     capacity,
		alerts:       make([]*Alert, 0, capacity),
		more:         make(chan struct{}, 1),
		done:         make(chan struct{}, 1),
		maxBatchSize: maxBatchSize,
	}
}

func (q *Queue) push(alerts []*Alert) (dropped int) {
	newAlerts := alerts

	if q.isClosed() {
		return len(newAlerts)
	}

	q.mtx.Lock()
	defer q.mtx.Unlock()

	// Queue capacity should be significantly larger than a single alert batch could be.
	if d := len(newAlerts) - q.capacity; d > 0 {
		newAlerts = newAlerts[d:]
		dropped = d
	}

	// If the Queue is full, remove the oldest alerts in favor of newer ones.
	if d := (len(q.alerts) + len(newAlerts)) - q.capacity; d > 0 {
		q.alerts = q.alerts[d:]

		dropped += d
	}

	q.alerts = append(q.alerts, newAlerts...)

	q.setMore()

	return
}

func (q *Queue) pop() []*Alert {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	var alerts []*Alert

	if len(q.alerts) > q.maxBatchSize {
		alerts = make([]*Alert, q.maxBatchSize)
		copy(alerts, q.alerts[:q.maxBatchSize])
		q.alerts = q.alerts[q.maxBatchSize:]
	} else {
		alerts = make([]*Alert, len(q.alerts))
		copy(alerts, q.alerts)
		q.alerts = q.alerts[:0]
	}

	return alerts
}

func (q *Queue) len() int {
	q.mtx.RLock()
	defer q.mtx.RUnlock()
	return len(q.alerts)
}

func (q *Queue) setMore() {
	if q.isClosed() {
		return
	}

	// Attempt to send a signal on the 'more' channel if no signal is pending.
	select {
	case q.more <- struct{}{}:
	case <-q.done:
		close(q.more)
	default:
		// No action needed if the channel already has a pending signal.
	}
}

func (q *Queue) close() {
	q.done <- struct{}{}
	q.closed.Store(true)
}

func (q *Queue) isClosed() bool {
	return q.closed.Load()
}
