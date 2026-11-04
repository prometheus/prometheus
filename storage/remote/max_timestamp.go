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

package remote

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

type maxTimestamp struct {
	mtx   sync.Mutex
	value atomic.Float64
	prometheus.Gauge
}

func (m *maxTimestamp) Set(value float64) {
	// Fast path: timestamps are truncated to whole seconds, so the vast majority
	// of calls do not advance the maximum and can return without locking.
	if value <= m.value.Load() {
		return
	}
	m.mtx.Lock()
	if value > m.value.Load() {
		m.value.Store(value)
		m.Gauge.Set(value)
	}
	m.mtx.Unlock()
}

func (m *maxTimestamp) Get() float64 {
	return m.value.Load()
}
