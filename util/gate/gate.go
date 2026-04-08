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

// Observer is the interface used to record observations (e.g., histogram metrics).
// We define it locally to avoid a circular dependency on the prometheus package.
type Observer interface {
	Observe(float64)
}

// A Gate controls the maximum number of concurrently running and waiting queries.
type Gate struct {
	ch       chan struct{}
	observer Observer
}

// New returns a query gate that limits the number of queries
// being concurrently executed.
// Note: This signature is preserved for backward compatibility.
func New(length int) *Gate {
	return &Gate{
		ch: make(chan struct{}, length),
	}
}

// NewWithObserver returns a query gate that limits the number of queries
// and records waiting duration metrics via the provided Observer.
func NewWithObserver(length int, observer Observer) *Gate {
	return &Gate{
		ch:       make(chan struct{}, length),
		observer: observer,
	}
}

// Start blocks until the gate has a free spot or the context is done.
// It records the time spent waiting if an observer is configured.
func (g *Gate) Start(ctx context.Context) error {
	begin := time.Now()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.ch <- struct{}{}:
		if g.observer != nil {
			// Record synchronously (Blocking Write)
			g.observer.Observe(time.Since(begin).Seconds())
		}
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
