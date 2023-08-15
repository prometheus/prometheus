// DO NOT EDIT. COPIED AS-IS. SEE README.md

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

	prometheustranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus"
)

// addSingleSumNumberDataPoint converts the Gauge metric data point to a
// Prometheus time series with samples and labels. The result is stored in the
// series map.
func addSingleGaugeNumberDataPoint(
	pt pmetric.NumberDataPoint,
	resource pcommon.Resource,
	metric pmetric.Metric,
	settings Settings,
	series map[string]*prompb.TimeSeries,
) {
	name := prometheustranslator.BuildPromCompliantName(metric, settings.Namespace)
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
) {
	name := prometheustranslator.BuildPromCompliantName(metric, settings.Namespace)
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

	if ts, ok := series[sig]; sig != "" && ok {
		exemplars := getPromExemplars[pmetric.NumberDataPoint](pt)
		ts.Exemplars = append(ts.Exemplars, exemplars...)
	}

	// add _created time series if needed
	if settings.ExportCreatedMetric && metric.Sum().IsMonotonic() {
		startTimestamp := pt.StartTimestamp()
		if startTimestamp != 0 {
			createdLabels := createAttributes(
				resource,
				pt.Attributes(),
				settings.ExternalLabels,
				nameStr,
				name+createdSuffix,
			)
			addCreatedTimeSeriesIfNeeded(series, createdLabels, startTimestamp, metric.Type().String())
		}
	}
}
