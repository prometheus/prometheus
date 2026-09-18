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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/247a9f996e09a83cdc25addf70c05e42b8b30186/pkg/translator/prometheusremotewrite/number_data_points_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestPrometheusConverter_addGaugeNumberDataPoints(t *testing.T) {
	ts := uint64(time.Now().UnixNano())
	tests := []struct {
		name   string
		metric func() pmetric.Metric
		want   func() []sample
	}{
		{
			name: "gauge",
			metric: func() pmetric.Metric {
				return getIntGaugeMetric(
					"test",
					pcommon.NewMap(),
					1, ts,
				)
			},
			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []sample{
					{
						MF: "test",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(pcommon.Timestamp(ts)),
						V:  1,
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()
			appTest := teststorage.NewAppendable()
			app := appTest.AppenderV2(t.Context())
			converter := NewPrometheusConverter(app)
			settings := Settings{}
			resource := pcommon.NewResource()

			require.NoError(t, converter.setResourceContext(resource, settings))

			converter.addGaugeNumberDataPoints(
				context.Background(),
				metric.Gauge().DataPoints(),
				settings,
				storage.AOptions{
					MetricFamilyName: metric.Name(),
				},
			)
			require.NoError(t, app.Commit())
			teststorage.RequireEqual(t, tt.want(), appTest.ResultSamples())
		})
	}
}

func TestPrometheusConverter_addSumNumberDataPoints(t *testing.T) {
	ts := pcommon.Timestamp(time.Now().UnixNano())
	tests := []struct {
		name   string
		metric func() pmetric.Metric
		want   func() []sample
	}{
		{
			name: "sum",
			metric: func() pmetric.Metric {
				return getIntSumMetric(
					"test",
					pcommon.NewMap(),
					1,
					uint64(ts.AsTime().UnixNano()),
				)
			},
			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []sample{
					{
						MF: "test",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(ts),
						V:  1,
					},
				}
			},
		},
		{
			name: "sum with exemplars",
			metric: func() pmetric.Metric {
				m := getIntSumMetric(
					"test",
					pcommon.NewMap(),
					1,
					uint64(ts.AsTime().UnixNano()),
				)
				m.Sum().DataPoints().At(0).Exemplars().AppendEmpty().SetDoubleValue(2)
				return m
			},

			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []sample{
					{
						MF: "test",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(ts),
						V:  1,
						ES: []exemplar.Exemplar{
							{Value: 2},
						},
					},
				}
			},
		},
		{
			name: "monotonic cumulative sum with start timestamp",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_sum")
				metric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				metric.SetEmptySum().SetIsMonotonic(true)

				dp := metric.Sum().DataPoints().AppendEmpty()
				dp.SetDoubleValue(1)
				dp.SetTimestamp(ts)
				dp.SetStartTimestamp(ts)

				return metric
			},

			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []sample{
					{
						MF: "test_sum",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(ts),
						ST: convertTimeStamp(ts),
						V:  1,
					},
				}
			},
		},
		{
			name: "monotonic cumulative sum with no start time",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_sum")
				metric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				metric.SetEmptySum().SetIsMonotonic(true)

				dp := metric.Sum().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)

				return metric
			},

			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []sample{
					{
						MF: "test_sum",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(ts),
						V:  0,
					},
				}
			},
		},
		{
			name: "non-monotonic cumulative sum with start time",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_sum")
				metric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				metric.SetEmptySum().SetIsMonotonic(false)

				dp := metric.Sum().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)

				return metric
			},

			want: func() []sample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []sample{
					{
						MF: "test_sum",
						L:  lbls,
						M:  metadata.Metadata{},
						T:  convertTimeStamp(ts),
						V:  0,
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()
			appTest := teststorage.NewAppendable()
			app := appTest.AppenderV2(t.Context())
			converter := NewPrometheusConverter(app)
			settings := Settings{}
			resource := pcommon.NewResource()

			require.NoError(t, converter.setResourceContext(resource, settings))

			converter.addSumNumberDataPoints(
				context.Background(),
				metric.Sum().DataPoints(),
				settings,
				storage.AOptions{
					MetricFamilyName: metric.Name(),
				},
			)
			require.NoError(t, app.Commit())
			teststorage.RequireEqual(t, tt.want(), appTest.ResultSamples())
		})
	}
}
