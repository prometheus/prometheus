// Copyright 2019 The Prometheus Authors
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
)

type maxGauge struct {
	mtx   sync.Mutex
	value float64
	prometheus.Gauge
}

func (m *maxGauge) Set(value float64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if value > m.value {
		m.value = value
		m.Gauge.Set(value)
	}
}

func (m *maxGauge) Get() float64 {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.value
}
