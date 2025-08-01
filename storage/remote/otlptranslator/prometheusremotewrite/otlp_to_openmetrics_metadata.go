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
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/prompb"
)

// As part of implementing support for delta temporality, the metric type metadata for delta metrics
// will be "gauge"/"gaugehistogram". 
// See proposal: https://github.com/prometheus/proposals/pull/48/
func otelMetricTypeToPromMetricType(otelMetric pmetric.Metric, allowDeltaTemporality bool) prompb.MetricMetadata_MetricType {
	switch otelMetric.Type() {
	case pmetric.MetricTypeGauge:
		return prompb.MetricMetadata_GAUGE
	case pmetric.MetricTypeSum:
		metricType := prompb.MetricMetadata_GAUGE
		if otelMetric.Sum().IsMonotonic() {
			metricType = prompb.MetricMetadata_COUNTER
		}
		if otelMetric.Sum().AggregationTemporality() == pmetric.AggregationTemporalityDelta && allowDeltaTemporality {
			metricType = prompb.MetricMetadata_GAUGE
		}
		return metricType
	case pmetric.MetricTypeHistogram:
		if otelMetric.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta && allowDeltaTemporality {
			return prompb.MetricMetadata_GAUGEHISTOGRAM
		}
		return prompb.MetricMetadata_HISTOGRAM
	case pmetric.MetricTypeSummary:
		return prompb.MetricMetadata_SUMMARY
	case pmetric.MetricTypeExponentialHistogram:
		if otelMetric.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta && allowDeltaTemporality {
			return prompb.MetricMetadata_GAUGEHISTOGRAM
		}
		return prompb.MetricMetadata_HISTOGRAM
	}
	return prompb.MetricMetadata_UNKNOWN
}
