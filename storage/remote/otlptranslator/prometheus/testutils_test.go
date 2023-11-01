// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var ilm pmetric.ScopeMetrics

func init() {
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()
	ilm = resourceMetrics.ScopeMetrics().AppendEmpty()
}

// Returns a new Metric of type "Gauge" with specified name and unit.
func createGauge(name, unit string) pmetric.Metric {
	gauge := ilm.Metrics().AppendEmpty()
	gauge.SetName(name)
	gauge.SetUnit(unit)
	gauge.SetEmptyGauge()
	return gauge
}

// Returns a new Metric of type Monotonic Sum with specified name and unit.
func createCounter(name, unit string) pmetric.Metric {
	counter := ilm.Metrics().AppendEmpty()
	counter.SetEmptySum().SetIsMonotonic(true)
	counter.SetName(name)
	counter.SetUnit(unit)
	return counter
}
