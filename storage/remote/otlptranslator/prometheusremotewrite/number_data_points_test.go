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
)

func TestPrometheusConverter_addGaugeNumberDataPoints(t *testing.T) {
	scopeAttrs := pcommon.NewMap()
	scopeAttrs.FromRaw(map[string]any{
		"attr1": "value1",
		"attr2": "value2",
	})
	defaultScope := scope{
		name:       "test-scope",
		version:    "1.0.0",
		schemaURL:  "https://schema.com",
		attributes: scopeAttrs,
	}
	ts := uint64(time.Now().UnixNano())
	tests := []struct {
		name         string
		metric       func() pmetric.Metric
		scope        scope
		promoteScope bool
		want         func() []combinedSample
	}{
		{
			name: "gauge without scope promotion",
			metric: func() pmetric.Metric {
				return getIntGaugeMetric(
					"test",
					pcommon.NewMap(),
					1, ts,
				)
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []combinedSample{
					{
						metricFamilyName: "test",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(pcommon.Timestamp(ts)),
						v:                1,
					},
				}
			},
		},
		{
			name: "gauge with scope promotion",
			metric: func() pmetric.Metric {
				return getIntGaugeMetric(
					"test",
					pcommon.NewMap(),
					1, ts,
				)
			},
			scope:        defaultScope,
			promoteScope: true,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
					"otel_scope_name", defaultScope.name,
					"otel_scope_schema_url", defaultScope.schemaURL,
					"otel_scope_version", defaultScope.version,
					"otel_scope_attr1", "value1",
					"otel_scope_attr2", "value2",
				)
				return []combinedSample{
					{
						metricFamilyName: "test",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(pcommon.Timestamp(ts)),
						v:                1,
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()
			mockAppender := &mockCombinedAppender{}
			converter := NewPrometheusConverter(mockAppender)

			converter.addGaugeNumberDataPoints(
				context.Background(),
				metric.Gauge().DataPoints(),
				pcommon.NewResource(),
				Settings{
					PromoteScopeMetadata: tt.promoteScope,
				},
				tt.scope,
				Metadata{
					MetricFamilyName: metric.Name(),
				},
			)
			require.NoError(t, mockAppender.Commit())

			requireEqual(t, tt.want(), mockAppender.samples)
		})
	}
}

func TestPrometheusConverter_addSumNumberDataPoints(t *testing.T) {
	scopeAttrs := pcommon.NewMap()
	scopeAttrs.FromRaw(map[string]any{
		"attr1": "value1",
		"attr2": "value2",
	})
	defaultScope := scope{
		name:       "test-scope",
		version:    "1.0.0",
		schemaURL:  "https://schema.com",
		attributes: scopeAttrs,
	}
	ts := pcommon.Timestamp(time.Now().UnixNano())
	tests := []struct {
		name         string
		metric       func() pmetric.Metric
		scope        scope
		promoteScope bool
		want         func() []combinedSample
	}{
		{
			name: "sum without scope promotion",
			metric: func() pmetric.Metric {
				return getIntSumMetric(
					"test",
					pcommon.NewMap(),
					1,
					uint64(ts.AsTime().UnixNano()),
				)
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []combinedSample{
					{
						metricFamilyName: "test",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						v:                1,
					},
				}
			},
		},
		{
			name: "sum with scope promotion",
			metric: func() pmetric.Metric {
				return getIntSumMetric(
					"test",
					pcommon.NewMap(),
					1,
					uint64(ts.AsTime().UnixNano()),
				)
			},
			scope:        defaultScope,
			promoteScope: true,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
					"otel_scope_name", defaultScope.name,
					"otel_scope_schema_url", defaultScope.schemaURL,
					"otel_scope_version", defaultScope.version,
					"otel_scope_attr1", "value1",
					"otel_scope_attr2", "value2",
				)
				return []combinedSample{
					{
						metricFamilyName: "test",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						v:                1,
					},
				}
			},
		},
		{
			name: "sum with exemplars and without scope promotion",
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
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test",
				)
				return []combinedSample{
					{
						metricFamilyName: "test",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						v:                1,
						es: []exemplar.Exemplar{
							{Value: 2},
						},
					},
				}
			},
		},
		{
			name: "monotonic cumulative sum with start timestamp and without scope promotion",
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
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []combinedSample{
					{
						metricFamilyName: "test_sum",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						st:               convertTimeStamp(ts),
						v:                1,
					},
				}
			},
		},
		{
			name: "monotonic cumulative sum with no start time and without scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_sum")
				metric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				metric.SetEmptySum().SetIsMonotonic(true)

				dp := metric.Sum().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []combinedSample{
					{
						metricFamilyName: "test_sum",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						v:                0,
					},
				}
			},
		},
		{
			name: "non-monotonic cumulative sum with start time and without scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_sum")
				metric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				metric.SetEmptySum().SetIsMonotonic(false)

				dp := metric.Sum().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				lbls := labels.FromStrings(
					model.MetricNameLabel, "test_sum",
				)
				return []combinedSample{
					{
						metricFamilyName: "test_sum",
						ls:               lbls,
						meta:             metadata.Metadata{},
						t:                convertTimeStamp(ts),
						v:                0,
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()
			mockAppender := &mockCombinedAppender{}
			converter := NewPrometheusConverter(mockAppender)

			converter.addSumNumberDataPoints(
				context.Background(),
				metric.Sum().DataPoints(),
				pcommon.NewResource(),
				Settings{
					PromoteScopeMetadata: tt.promoteScope,
				},
				tt.scope,
				Metadata{
					MetricFamilyName: metric.Name(),
				},
			)
			require.NoError(t, mockAppender.Commit())

			requireEqual(t, tt.want(), mockAppender.samples)
		})
	}
}
