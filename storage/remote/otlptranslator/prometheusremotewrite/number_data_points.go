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
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

func (c *PrometheusConverter) addGaugeNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, settings Settings, name string, scope scope, meta metadata.Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		labels := c.createAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
			model.MetricNameLabel,
			name,
		)
		var val float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			val = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}
		ts := convertTimeStamp(pt.Timestamp())
		if _, err := c.appender.Append(0, labels, ts, val); err != nil {
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				c.logger.Error("Out of order sample from OTLP", "err", err.Error(), "series", labels.String(), "timestamp", ts)
			}
			return err
		}
		return c.updateMetadataIfNeeded(labels, meta)
	}

	return nil
}

func (c *PrometheusConverter) addSumNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, metric pmetric.Metric, settings Settings, name string, scope scope, meta metadata.Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		lbls := c.createAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
			model.MetricNameLabel,
			name,
		)
		var val float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			val = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}
		ts := convertTimeStamp(pt.Timestamp())
		if _, err := c.appender.Append(0, lbls, ts, val); err != nil {
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				c.logger.Error("Out of order sample from OTLP", "err", err.Error(), "series", lbls.String(), "timestamp", ts)
			}
			return err
		}
		if err := c.updateMetadataIfNeeded(lbls, meta); err != nil {
			return err
		}
		exemplars, err := c.getPromExemplars(ctx, pt.Exemplars())
		if err != nil {
			return err
		}
		for _, ex := range exemplars {
			if _, err := c.appender.AppendExemplar(0, lbls, ex); err != nil {
				switch {
				case errors.Is(err, storage.ErrOutOfOrderExemplar):
					// TODO: metric for out of order exemplars
					c.logger.Debug("Out of order exemplar", "series", lbls.String(), "exemplar", fmt.Sprintf("%+v", ex))
				default:
					// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
					c.logger.Debug("Error while adding exemplar in AppendExemplar", "series", lbls.String(), "exemplar", fmt.Sprintf("%+v", ex), "err", err)
				}
			}
		}

		// add created time series if needed
		if settings.ExportCreatedMetric && metric.Sum().IsMonotonic() {
			startTimestamp := pt.StartTimestamp()
			if startTimestamp == 0 {
				return nil
			}

			c.builder.Reset(lbls)
			// Add created suffix to the metric name for CT series.
			c.builder.Set(model.MetricNameLabel, c.builder.Get(model.MetricNameLabel)+createdSuffix)
			c.addTimeSeriesIfNeeded(c.builder.Labels(), meta, startTimestamp, pt.Timestamp())
		}
	}

	return nil
}
