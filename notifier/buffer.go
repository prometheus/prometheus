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

// Buffer is a circular buffer for Alerts.
type Buffer struct {
	mtx          sync.RWMutex
	data         []*Alert
	size         int
	count        int
	readPointer  int
	writePointer int
	more         chan struct{}
	done         chan struct{}
	closed       atomic.Bool
}

func newBuffer(size int) *Buffer {
	return &Buffer{
		data: make([]*Alert, size),
		size: size,
		more: make(chan struct{}, 1),
		done: make(chan struct{}, 1),
	}
}

func (b *Buffer) push(alerts ...*Alert) (dropped int) {
	if b.isClosed() {
		return len(alerts)
	}

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

	b.setMore()

	return
}

// pop will moce alerts from the buffer into the passed slice.
// Number of moved alerts = min (alerts in buffer and passed slice length).
// The silce length will be dynamically adjusted.
func (b *Buffer) pop(alerts *[]*Alert) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.count == 0 {
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
}

func (b *Buffer) len() int {
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	return b.count
}

func (b *Buffer) setMore() {
	if b.isClosed() {
		return
	}

	// Attempt to send a signal on the 'more' channel if no signal is pending.
	select {
	case b.more <- struct{}{}:
	case <-b.done:
		close(b.more)
	default:
		// No action needed if the channel already has a pending signal.
	}
}

func (b *Buffer) close() {
	b.done <- struct{}{}
	b.closed.Store(true)
}

func (b *Buffer) isClosed() bool {
	return b.closed.Load()
}
