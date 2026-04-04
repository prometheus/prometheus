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

package gate

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Gate controls the maximum number of concurrent requests.
type Gate struct {
	ch      chan struct{}
	waiting prometheus.Observer
}

// New returns a new Gate.
func New(max int, waiting prometheus.Observer) *Gate {
	return &Gate{
		ch:      make(chan struct{}, max),
		waiting: waiting,
	}
}

// Start blocks until a slot is available or the context is canceled.
func (g *Gate) Start(ctx context.Context) error {
	start := time.Now()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.ch <- struct{}{}:
		if g.waiting != nil {
			g.waiting.Observe(time.Since(start).Seconds())
		}
		return nil
	}
}

// Done releases a slot.
func (g *Gate) Done() {
	<-g.ch
}
