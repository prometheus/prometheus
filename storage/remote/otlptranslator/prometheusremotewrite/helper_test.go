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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/debbf30360b8d3a0ded8db09c4419d2a9c99b94a/pkg/translator/prometheusremotewrite/helper_test.go
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

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestCreateAttributes(t *testing.T) {
	resourceAttrs := map[string]string{
		"service.name":        "service name",
		"service.instance.id": "service ID",
		"existent-attr":       "resource value",
		// This one is for testing conflict with metric attribute.
		"metric-attr": "resource value",
		// This one is for testing conflict with auto-generated job attribute.
		"job": "resource value",
		// This one is for testing conflict with auto-generated instance attribute.
		"instance": "resource value",
	}
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

	resource := pcommon.NewResource()
	for k, v := range resourceAttrs {
		resource.Attributes().PutStr(k, v)
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("metric-attr", "metric value")
	attrs.PutStr("metric-attr-other", "metric value other")

	testCases := []struct {
		name                         string
		scope                        scope
		promoteAllResourceAttributes bool
		promoteResourceAttributes    []string
		promoteScope                 bool
		ignoreResourceAttributes     []string
		ignoreAttrs                  []string
		expectedLabels               labels.Labels
	}{
		{
			name:                      "Successful conversion without resource attribute promotion and without scope promotion",
			scope:                     defaultScope,
			promoteScope:              false,
			promoteResourceAttributes: nil,
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
			),
		},
		{
			name:                      "Successful conversion without resource attribute promotion and with scope promotion",
			scope:                     defaultScope,
			promoteScope:              true,
			promoteResourceAttributes: nil,
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                      "Successful conversion without resource attribute promotion and with scope promotion, but without scope",
			scope:                     scope{},
			promoteResourceAttributes: nil,
			promoteScope:              true,
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
			),
		},
		{
			name:                      "Successful conversion with some attributes ignored",
			promoteResourceAttributes: nil,
			ignoreAttrs:               []string{"metric-attr-other"},
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
			),
		},
		{
			name:                      "Successful conversion with some attributes ignored and with scope promotion",
			scope:                     defaultScope,
			promoteScope:              true,
			promoteResourceAttributes: nil,
			ignoreAttrs:               []string{"metric-attr-other"},
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                      "Successful conversion with resource attribute promotion and with scope promotion",
			scope:                     defaultScope,
			promoteResourceAttributes: []string{"non-existent-attr", "existent-attr"},
			promoteScope:              true,
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"existent_attr", "resource value",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                      "Successful conversion with resource attribute promotion and with scope promotion, conflicting resource attributes are ignored",
			scope:                     defaultScope,
			promoteScope:              true,
			promoteResourceAttributes: []string{"non-existent-attr", "existent-attr", "metric-attr", "job", "instance"},
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"existent_attr", "resource value",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                      "Successful conversion with resource attribute promotion and with scope promotion, attributes are only promoted once",
			scope:                     defaultScope,
			promoteScope:              true,
			promoteResourceAttributes: []string{"existent-attr", "existent-attr"},
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"existent_attr", "resource value",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                         "Successful conversion promoting all resource attributes and with scope promotion",
			scope:                        defaultScope,
			promoteAllResourceAttributes: true,
			promoteScope:                 true,
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"existent_attr", "resource value",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"service_name", "service name",
				"service_instance_id", "service ID",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
		{
			name:                         "Successful conversion promoting all resource attributes and with scope promotion, ignoring 'service.instance.id'",
			scope:                        defaultScope,
			promoteScope:                 true,
			promoteAllResourceAttributes: true,
			ignoreResourceAttributes: []string{
				"service.instance.id",
			},
			expectedLabels: labels.FromStrings(
				"__name__", "test_metric",
				"instance", "service ID",
				"job", "service name",
				"existent_attr", "resource value",
				"metric_attr", "metric value",
				"metric_attr_other", "metric value other",
				"service_name", "service name",
				"otel_scope_name", defaultScope.name,
				"otel_scope_schema_url", defaultScope.schemaURL,
				"otel_scope_version", defaultScope.version,
				"otel_scope_attr1", "value1",
				"otel_scope_attr2", "value2",
			),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewPrometheusConverter(&mockCombinedAppender{})
			settings := Settings{
				PromoteResourceAttributes: NewPromoteResourceAttributes(config.OTLPConfig{
					PromoteAllResourceAttributes: tc.promoteAllResourceAttributes,
					PromoteResourceAttributes:    tc.promoteResourceAttributes,
					IgnoreResourceAttributes:     tc.ignoreResourceAttributes,
				}),
				PromoteScopeMetadata: tc.promoteScope,
			}
			lbls := c.createAttributes(resource, attrs, tc.scope, settings, tc.ignoreAttrs, false, metadata.Metadata{}, model.MetricNameLabel, "test_metric")

			testutil.RequireEqual(t, lbls, tc.expectedLabels)
		})
	}
}

func Test_convertTimeStamp(t *testing.T) {
	tests := []struct {
		name string
		arg  pcommon.Timestamp
		want int64
	}{
		{"zero", 0, 0},
		{"1ms", 1_000_000, 1},
		{"1s", pcommon.Timestamp(time.Unix(1, 0).UnixNano()), 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTimeStamp(tt.arg)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPrometheusConverter_AddSummaryDataPoints(t *testing.T) {
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
			name: "summary with start time and without scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_summary")
				metric.SetEmptySummary()

				dp := metric.Summary().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)
				dp.SetStartTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				return []combinedSample{
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_summary"+sumStr,
						),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_summary"+countStr,
						),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_summary"+createdSuffix,
						),
						t: convertTimeStamp(ts),
						v: float64(convertTimeStamp(ts)),
					},
				}
			},
		},
		{
			name: "summary with start time and with scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_summary")
				metric.SetEmptySummary()

				dp := metric.Summary().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)
				dp.SetStartTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: true,
			want: func() []combinedSample {
				scopeLabels := []string{
					"otel_scope_attr1", "value1",
					"otel_scope_attr2", "value2",
					"otel_scope_name", defaultScope.name,
					"otel_scope_schema_url", defaultScope.schemaURL,
					"otel_scope_version", defaultScope.version,
				}
				return []combinedSample{
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_summary"+sumStr)...),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_summary"+countStr)...),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_summary"+createdSuffix,
						)...),
						t: convertTimeStamp(ts),
						v: float64(convertTimeStamp(ts)),
					},
				}
			},
		},
		{
			name: "summary without start time and without scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_summary")
				metric.SetEmptySummary()

				dp := metric.Summary().DataPoints().AppendEmpty()
				dp.SetTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				return []combinedSample{
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_summary"+sumStr,
						),
						t: convertTimeStamp(ts),
						v: 0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_summary"+countStr,
						),
						t: convertTimeStamp(ts),
						v: 0,
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

			converter.addSummaryDataPoints(
				context.Background(),
				metric.Summary().DataPoints(),
				pcommon.NewResource(),
				Settings{
					ExportCreatedMetric:  true,
					PromoteScopeMetadata: tt.promoteScope,
				},
				metric.Name(),
				tt.scope,
				metadata.Metadata{},
			)

			requireEqual(t, tt.want(), mockAppender.samples)
			require.Empty(t, converter.conflicts)
		})
	}
}

func TestPrometheusConverter_AddHistogramDataPoints(t *testing.T) {
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
			name: "histogram with start time and without scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist")
				metric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.Histogram().DataPoints().AppendEmpty()
				pt.SetTimestamp(ts)
				pt.SetStartTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: false,
			want: func() []combinedSample {
				return []combinedSample{
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_hist"+countStr,
						),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_hist_bucket",
							model.BucketLabel, "+Inf",
						),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_hist"+createdSuffix,
						),
						t: convertTimeStamp(ts),
						v: float64(convertTimeStamp(ts)),
					},
				}
			},
		},
		{
			name: "histogram with start time and with scope promotion",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist")
				metric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.Histogram().DataPoints().AppendEmpty()
				pt.SetTimestamp(ts)
				pt.SetStartTimestamp(ts)

				return metric
			},
			scope:        defaultScope,
			promoteScope: true,
			want: func() []combinedSample {
				scopeLabels := []string{
					"otel_scope_attr1", "value1",
					"otel_scope_attr2", "value2",
					"otel_scope_name", defaultScope.name,
					"otel_scope_schema_url", defaultScope.schemaURL,
					"otel_scope_version", defaultScope.version,
				}
				return []combinedSample{
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_hist"+countStr)...),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_hist_bucket",
							model.BucketLabel, "+Inf")...),
						t:  convertTimeStamp(ts),
						ct: convertTimeStamp(ts),
						v:  0,
					},
					{
						ls: labels.FromStrings(append(scopeLabels,
							model.MetricNameLabel, "test_hist"+createdSuffix)...),
						t: convertTimeStamp(ts),
						v: float64(convertTimeStamp(ts)),
					},
				}
			},
		},
		{
			name: "histogram without start time",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist")
				metric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.Histogram().DataPoints().AppendEmpty()
				pt.SetTimestamp(ts)

				return metric
			},
			want: func() []combinedSample {
				return []combinedSample{
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_hist"+countStr,
						),
						t: convertTimeStamp(ts),
						v: 0,
					},
					{
						ls: labels.FromStrings(
							model.MetricNameLabel, "test_hist_bucket",
							model.BucketLabel, "+Inf",
						),
						t: convertTimeStamp(ts),
						v: 0,
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

			converter.addHistogramDataPoints(
				context.Background(),
				metric.Histogram().DataPoints(),
				pcommon.NewResource(),
				Settings{
					ExportCreatedMetric:  true,
					PromoteScopeMetadata: tt.promoteScope,
				},
				metric.Name(),
				tt.scope,
				metadata.Metadata{},
			)

			requireEqual(t, tt.want(), mockAppender.samples)
			require.Empty(t, converter.conflicts)
		})
	}
}

func TestGetPromExemplars(t *testing.T) {
	ctx := context.Background()
	c := NewPrometheusConverter(&mockCombinedAppender{})

	t.Run("Exemplars with int value", func(t *testing.T) {
		es := pmetric.NewExemplarSlice()
		exemplar := es.AppendEmpty()
		exemplar.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		exemplar.SetIntValue(42)
		exemplars, err := c.getPromExemplars(ctx, es)
		require.NoError(t, err)
		require.Len(t, exemplars, 1)
		require.Equal(t, float64(42), exemplars[0].Value)
	})

	t.Run("Exemplars with double value", func(t *testing.T) {
		es := pmetric.NewExemplarSlice()
		exemplar := es.AppendEmpty()
		exemplar.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		exemplar.SetDoubleValue(69.420)
		exemplars, err := c.getPromExemplars(ctx, es)
		require.NoError(t, err)
		require.Len(t, exemplars, 1)
		require.Equal(t, 69.420, exemplars[0].Value)
	})

	t.Run("Exemplars with unsupported value type", func(t *testing.T) {
		es := pmetric.NewExemplarSlice()
		exemplar := es.AppendEmpty()
		exemplar.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		_, err := c.getPromExemplars(ctx, es)
		require.Error(t, err)
	})
}

func TestAddTypeAndUnitLabels(t *testing.T) {
	testCases := []struct {
		name           string
		inputLabels    []prompb.Label
		metadata       prompb.MetricMetadata
		expectedLabels []prompb.Label
	}{
		{
			name: "overwrites existing type and unit labels and preserves other labels",
			inputLabels: []prompb.Label{
				{Name: "job", Value: "test-job"},
				{Name: "__type__", Value: "old_type"},
				{Name: "instance", Value: "test-instance"},
				{Name: "__unit__", Value: "old_unit"},
				{Name: "custom_label", Value: "custom_value"},
			},
			metadata: prompb.MetricMetadata{
				Type: prompb.MetricMetadata_COUNTER,
				Unit: "seconds",
			},
			expectedLabels: []prompb.Label{
				{Name: "job", Value: "test-job"},
				{Name: "instance", Value: "test-instance"},
				{Name: "custom_label", Value: "custom_value"},
				{Name: "__type__", Value: "counter"},
				{Name: "__unit__", Value: "seconds"},
			},
		},
		{
			name: "adds type and unit labels when missing",
			inputLabels: []prompb.Label{
				{Name: "job", Value: "test-job"},
				{Name: "instance", Value: "test-instance"},
			},
			metadata: prompb.MetricMetadata{
				Type: prompb.MetricMetadata_GAUGE,
				Unit: "bytes",
			},
			expectedLabels: []prompb.Label{
				{Name: "job", Value: "test-job"},
				{Name: "instance", Value: "test-instance"},
				{Name: "__type__", Value: "gauge"},
				{Name: "__unit__", Value: "bytes"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := addTypeAndUnitLabels(tc.inputLabels, tc.metadata, Settings{AllowUTF8: false})
			require.ElementsMatch(t, tc.expectedLabels, result)
		})
	}
}
