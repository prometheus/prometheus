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

	"github.com/prometheus/client_golang/prometheus"
)

// A Gate controls the maximum number of concurrently running and waiting queries.
type Gate struct {
	ch chan struct{}
	// available tracks the net available capacity of the gate:
	//   - Positive values: Number of free slots (e.g., 2 means 2 requests can proceed immediately)
	//   - Zero: Gate at full capacity (next request will wait)
	//   - Negative values: Number of requests waiting in queue (e.g., -3 means 3 requests are waiting)
	available    prometheus.Gauge
	waitDuration prometheus.Counter // waitDuration tracks time spent waiting for gate access
}

// New returns a query gate that limits the number of queries
// being concurrently executed.
func New(length int, available prometheus.Gauge, waitDuration prometheus.Counter) *Gate {
	g := &Gate{
		ch:           make(chan struct{}, length),
		available:    available,
		waitDuration: waitDuration,
	}
	if available != nil {
		available.Set(float64(length))
	}
	return g
}

// Start blocks until the gate has a free spot or the context is done.
func (g *Gate) Start(ctx context.Context) error {
	start := time.Now()
	if g.available != nil {
		g.available.Dec() // Decrement available slots (may go negative)
	}

	defer func() {
		if g.waitDuration != nil {
			g.waitDuration.Add(time.Since(start).Seconds())
		}
	}()

	select {
	case <-ctx.Done():
		if g.available != nil {
			g.available.Inc() // Restore the slot if context cancelled
		}
		return ctx.Err()
	case g.ch <- struct{}{}:
		return nil
	}
}

// Done releases a single spot in the gate.
func (g *Gate) Done() {
	select {
	case <-g.ch:
		if g.available != nil {
			g.available.Inc() // Release a slot
		}
	default:
		panic("gate.Done: more operations done than started")
	}
}
