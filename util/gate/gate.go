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
	ch        chan struct{}
	histogram prometheus.Histogram
}

// New returns a gate that limits the number of concurrent operations.
// If registerer is not nil, a histogram metric tracking wait duration will be registered.
// The metric name will be "{metricPrefix}_gate_wait_duration_seconds".
func New(length int, registerer prometheus.Registerer, metricPrefix string) *Gate {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: metricPrefix + "_gate_wait_duration_seconds",
		Help: "Time spent waiting for the gate in seconds.",
	})

	if registerer != nil {
		registerer.MustRegister(histogram)
	}

	return &Gate{
		ch:        make(chan struct{}, length),
		histogram: histogram,
	}
}

// Start blocks until the gate has a free spot or the context is done.
// It records the duration spent waiting in the histogram metric.
func (g *Gate) Start(ctx context.Context) error {
	startTime := time.Now()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.ch <- struct{}{}:
		// Successfully acquired the gate, record the wait time
		g.histogram.Observe(time.Since(startTime).Seconds())
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
