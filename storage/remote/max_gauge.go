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
