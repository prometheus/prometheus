// Copyright The Prometheus Authors
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

// buffer is a circular buffer for Alerts.
type buffer struct {
	mtx          sync.RWMutex
	data         []*Alert
	size         int
	count        int
	readPointer  int
	writePointer int
	hasWork      chan struct{}
	done         chan struct{}
	closed       atomic.Bool
}

func newBuffer(size int) *buffer {
	return &buffer{
		data:    make([]*Alert, size),
		size:    size,
		hasWork: make(chan struct{}, 1),
		done:    make(chan struct{}, 1),
	}
}

func (b *buffer) push(alerts ...*Alert) (dropped int) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, a := range alerts {
		if b.count == b.size {
			b.readPointer = (b.readPointer + 1) % b.size
			dropped++
		} else {
			b.count++
		}
		b.data[b.writePointer] = a
		b.writePointer = (b.writePointer + 1) % b.size
	}

	// If the buffer still has items left, kick off the next iteration.
	if b.count > 0 {
		b.notifyWork()
	}

	return
}

// pop will move alerts from the buffer into the passed slice.
// Number of moved alerts = min (alerts in buffer and passed slice length).
// The silce length will be dynamically adjusted.
func (b *buffer) pop(alerts *[]*Alert) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.count == 0 {
		// Empty alerts from any cached data.
		*alerts = (*alerts)[:0]
		return
	}

	count := min(b.count, cap(*alerts))
	*alerts = (*alerts)[0:count]

	for i := range count {
		(*alerts)[i] = b.data[b.readPointer]
		b.data[b.readPointer] = nil
		b.readPointer = (b.readPointer + 1) % b.size
		b.count--
	}

	// If the buffer still has items left, kick off the next iteration.
	if b.count > 0 {
		b.notifyWork()
	}
}

func (b *buffer) len() int {
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	return b.count
}

func (b *buffer) notifyWork() {
	if b.isClosed() {
		return
	}

	// Attempt to send a signal on the 'hasWork' channel if no signal is pending.
	select {
	case b.hasWork <- struct{}{}:
	case <-b.done:
		close(b.hasWork)
	default:
		// No action needed if the channel already has a pending signal.
	}
}

func (b *buffer) close() {
	b.done <- struct{}{}
	b.closed.Store(true)
}

func (b *buffer) isClosed() bool {
	return b.closed.Load()
}
