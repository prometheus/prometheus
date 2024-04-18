// DO NOT EDIT. COPIED AS-IS. SEE ../README.md

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite // import "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"

import (
	"math"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// addSingleGaugeNumberDataPoint converts the Gauge metric data point to a
// Prometheus time series with samples and labels. The result is stored in the
// series map.
func addSingleGaugeNumberDataPoint(
	pt pmetric.NumberDataPoint,
	resource pcommon.Resource,
	metric pmetric.Metric,
	settings Settings,
	series map[string]*prompb.TimeSeries,
	name string,
) {
	labels := createAttributes(
		resource,
		pt.Attributes(),
		settings.ExternalLabels,
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
	addSample(series, sample, labels, metric.Type().String())
}

// addSingleSumNumberDataPoint converts the Sum metric data point to a Prometheus
// time series with samples, labels and exemplars. The result is stored in the
// series map.
func addSingleSumNumberDataPoint(
	pt pmetric.NumberDataPoint,
	resource pcommon.Resource,
	metric pmetric.Metric,
	settings Settings,
	series map[string]*prompb.TimeSeries,
	name string,
) {
	labels := createAttributes(
		resource,
		pt.Attributes(),
		settings.ExternalLabels,
		model.MetricNameLabel, name,
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
	sig := addSample(series, sample, labels, metric.Type().String())

	if ts := series[sig]; sig != "" && ts != nil {
		exemplars := getPromExemplars[pmetric.NumberDataPoint](pt)
		ts.Exemplars = append(ts.Exemplars, exemplars...)
	}

	// add _created time series if needed
	if settings.ExportCreatedMetric && metric.Sum().IsMonotonic() {
		startTimestamp := pt.StartTimestamp()
		if startTimestamp == 0 {
			return
		}

		createdLabels := make([]prompb.Label, len(labels))
		copy(createdLabels, labels)
		for i, l := range createdLabels {
			if l.Name == model.MetricNameLabel {
				createdLabels[i].Value = name + createdSuffix
				break
			}
		}
		addCreatedTimeSeriesIfNeeded(series, createdLabels, startTimestamp, pt.Timestamp(), metric.Type().String())
	}
}
