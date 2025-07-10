// Copyright 2024 The Prometheus Authors
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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/otlp_to_openmetrics_metadata.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func otelMetricTypeToPromMetricType(otelMetric pmetric.Metric) model.MetricType {
	switch otelMetric.Type() {
	case pmetric.MetricTypeGauge:
		return model.MetricTypeGauge
	case pmetric.MetricTypeSum:
		metricType := model.MetricTypeGauge
		if otelMetric.Sum().IsMonotonic() {
			metricType = model.MetricTypeCounter
		}
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
		// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
		if otelMetric.Sum().AggregationTemporality() == pmetric.AggregationTemporalityDelta {
			metricType = model.MetricTypeUnknown
		}
		return metricType
	case pmetric.MetricTypeHistogram:
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
		// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
		if otelMetric.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta {
			return model.MetricTypeUnknown
		}
		return model.MetricTypeHistogram
	case pmetric.MetricTypeSummary:
		return model.MetricTypeSummary
	case pmetric.MetricTypeExponentialHistogram:
		if otelMetric.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta {
			// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
			// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
			return model.MetricTypeUnknown
		}
		return model.MetricTypeHistogram
	}
	return model.MetricTypeUnknown
}
