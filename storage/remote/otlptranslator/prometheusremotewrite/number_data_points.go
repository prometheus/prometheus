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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/number_data_points.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"math"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
)

func (c *PrometheusConverter) addGaugeNumberDataPoints(dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, settings Settings, name string) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		labels := createAttributes(
			resource,
			pt.Attributes(),
			settings,
			nil,
			true,
			model.MetricNameLabel,
			name,
		)
		sample := &prompb.Sample{
			// convert ns to ms
			Timestamp: convertTimeStamp(pt.Timestamp()),
		}
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			sample.Value = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			sample.Value = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			sample.Value = math.Float64frombits(value.StaleNaN)
		}
		c.addSample(sample, labels)
	}
}

func (c *PrometheusConverter) addSumNumberDataPoints(dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, metric pmetric.Metric, settings Settings, name string) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		lbls := createAttributes(
			resource,
			pt.Attributes(),
			settings,
			nil,
			true,
			model.MetricNameLabel,
			name,
		)
		sample := &prompb.Sample{
			// convert ns to ms
			Timestamp: convertTimeStamp(pt.Timestamp()),
		}
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			sample.Value = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			sample.Value = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			sample.Value = math.Float64frombits(value.StaleNaN)
		}
		ts := c.addSample(sample, lbls)
		if ts != nil {
			exemplars := getPromExemplars[pmetric.NumberDataPoint](pt)
			ts.Exemplars = append(ts.Exemplars, exemplars...)
		}

		// add created time series if needed
		if settings.ExportCreatedMetric && metric.Sum().IsMonotonic() {
			startTimestamp := pt.StartTimestamp()
			if startTimestamp == 0 {
				return
			}

			createdLabels := make([]prompb.Label, len(lbls))
			copy(createdLabels, lbls)
			for i, l := range createdLabels {
				if l.Name == model.MetricNameLabel {
					createdLabels[i].Value = name + createdSuffix
					break
				}
			}
			c.addTimeSeriesIfNeeded(createdLabels, startTimestamp, pt.Timestamp())
		}
	}
}
