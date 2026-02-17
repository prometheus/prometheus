// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gate

import (
	"context"
	"time"
)

// Observer is the minimal interface for recording metrics.
type Observer interface {
	Observe(float64)
}

// A Gate controls the maximum number of concurrently running and waiting queries.
type Gate struct {
	ch           chan struct{}
	waitDuration Observer // Optional: nil if not tracking
}

// New returns a query gate that limits the number of queries
// being concurrently executed.
func New(length int) *Gate {
	return NewWithMetric(length, nil)
}

// NewWithMetric returns a gate that optionally tracks wait duration.
// If waitDuration is non-nil, it observes the duration spent waiting
// to acquire a slot in the gate.
func NewWithMetric(length int, waitDuration Observer) *Gate {
	return &Gate{
		ch:           make(chan struct{}, length),
		waitDuration: waitDuration,
	}
}

// Start blocks until the gate has a free spot or the context is done.
func (g *Gate) Start(ctx context.Context) error {
	if g.waitDuration != nil {
		// Try non-blocking send first to avoid timing overhead when no wait is needed.
		select {
		case g.ch <- struct{}{}:
			// Acquired immediately, no wait time.
			g.waitDuration.Observe(0)
			return nil
		default:
			// Channel is full, will need to wait.
		}

		// Start timing the actual wait.
		start := time.Now()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case g.ch <- struct{}{}:
			g.waitDuration.Observe(time.Since(start).Seconds())
			return nil
		}
	}

	// No metric tracking, use simple select.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.ch <- struct{}{}:
		return nil
	}
}

// Done releases a single spot in the gate.
func (g *Gate) Done() {
	select {
	case <-g.ch:
	default:
		panic("gate.Done: more operations done than started")
	}
}
