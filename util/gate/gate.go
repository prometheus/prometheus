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
	ch           chan struct{}
	waitDuration prometheus.Histogram
}

// New returns a query gate that limits the number of queries
// being concurrently executed. If reg is non-nil, a histogram tracking
// gate wait duration is registered.
func New(length int, reg prometheus.Registerer) *Gate {
	g := &Gate{
		ch: make(chan struct{}, length),
		waitDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                           "gate_wait_duration_seconds",
			Help:                           "How long a request spent waiting for the gate to open before proceeding.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}),
	}
	if reg != nil {
		reg.MustRegister(g.waitDuration)
	}
	return g
}

// Start blocks until the gate has a free spot or the context is done.
func (g *Gate) Start(ctx context.Context) error {
	start := time.Now()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.ch <- struct{}{}:
		g.waitDuration.Observe(time.Since(start).Seconds())
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
