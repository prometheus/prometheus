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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/number_data_points.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"math"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/value"
)

func (c *PrometheusConverter) addGaugeNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	settings Settings, meta Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		labels, err := c.createAttributes(
			pt.Attributes(),
			settings,
			nil,
			true,
			meta,
			model.MetricNameLabel,
			meta.MetricFamilyName,
		)
		if err != nil {
			return err
		}
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
		st := convertTimeStamp(pt.StartTimestamp())
		if err := c.appender.AppendSample(labels, meta, st, ts, val, nil); err != nil {
			return err
		}
	}

	return nil
}

func (c *PrometheusConverter) addSumNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	settings Settings, meta Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		lbls, err := c.createAttributes(
			pt.Attributes(),
			settings,
			nil,
			true,
			meta,
			model.MetricNameLabel,
			meta.MetricFamilyName,
		)
		if err != nil {
			return nil
		}
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
		st := convertTimeStamp(pt.StartTimestamp())
		exemplars, err := c.getPromExemplars(ctx, pt.Exemplars())
		if err != nil {
			return err
		}
		if err := c.appender.AppendSample(lbls, meta, st, ts, val, exemplars); err != nil {
			return err
		}
	}

	return nil
}
