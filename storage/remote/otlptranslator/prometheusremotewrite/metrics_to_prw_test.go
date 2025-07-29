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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/metrics_to_prw_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

func TestFromMetrics(t *testing.T) {
	t.Run("Successful", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			settings    Settings
			temporality pmetric.AggregationTemporality
		}{
			{
				name:        "Default with cumulative temporality",
				settings:    Settings{},
				temporality: pmetric.AggregationTemporalityCumulative,
			},
			{
				name: "Default with delta temporality",
				settings: Settings{
					AllowDeltaTemporality: true,
				},
				temporality: pmetric.AggregationTemporalityDelta,
			},
			{
				name: "Keep identifying attributes",
				settings: Settings{
					KeepIdentifyingResourceAttributes: true,
				},
				temporality: pmetric.AggregationTemporalityCumulative,
			},
			{
				name: "Add metric suffixes with cumulative temporality",
				settings: Settings{
					AddMetricSuffixes: true,
				},
				temporality: pmetric.AggregationTemporalityCumulative,
			},
			{
				name: "Add metric suffixes with delta temporality",
				settings: Settings{
					AddMetricSuffixes:     true,
					AllowDeltaTemporality: true,
				},
				temporality: pmetric.AggregationTemporalityDelta,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				mockAppender := &mockCombinedAppender{}
				converter := NewPrometheusConverter(mockAppender)
				payload, wantPromMetrics := createExportRequest(5, 128, 128, 2, 0, tc.settings, tc.temporality)
				seenFamilyNames := map[string]struct{}{}
				for _, wantMetric := range wantPromMetrics {
					if _, exists := seenFamilyNames[wantMetric.familyName]; exists {
						continue
					}
					if wantMetric.familyName == "target_info" {
						continue
					}

					seenFamilyNames[wantMetric.familyName] = struct{}{}
				}

				// TODO check returned counters
				annots, err := converter.FromMetrics(
					context.Background(),
					payload.Metrics(),
					tc.settings,
				)
				require.NoError(t, err)
				require.Empty(t, annots)

				ts := mockAppender.samples
				require.Len(t, ts, 1536+1) // +1 for the target_info.

				tgtInfoCount := 0
				for _, s := range ts {
					lbls := s.ls
					if lbls.Get(labels.MetricName) == "target_info" {
						tgtInfoCount++
						require.Equal(t, "test-namespace/test-service", lbls.Get("job"))
						require.Equal(t, "id1234", lbls.Get("instance"))
						if tc.settings.KeepIdentifyingResourceAttributes {
							require.Equal(t, "test-service", lbls.Get("service_name"))
							require.Equal(t, "test-namespace", lbls.Get("service_namespace"))
							require.Equal(t, "id1234", lbls.Get("service_instance_id"))
						} else {
							require.False(t, lbls.Has("service_name"))
							require.False(t, lbls.Has("service_namespace"))
							require.False(t, lbls.Has("service_instance_id"))
						}
					}
				}
				require.Equal(t, 1, tgtInfoCount)
			})
		}
	})

	for _, convertHistogramsToNHCB := range []bool{false, true} {
		t.Run(fmt.Sprintf("successful/convertHistogramsToNHCB=%v", convertHistogramsToNHCB), func(t *testing.T) {
			request := pmetricotlp.NewExportRequest()
			rm := request.Metrics().ResourceMetrics().AppendEmpty()
			generateAttributes(rm.Resource().Attributes(), "resource", 10)

			metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
			ts := pcommon.NewTimestampFromTime(time.Now())

			m := metrics.AppendEmpty()
			m.SetEmptyHistogram()
			m.SetName("histogram-1")
			m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			h := m.Histogram().DataPoints().AppendEmpty()
			h.SetTimestamp(ts)

			h.SetCount(15)
			h.SetSum(155)

			generateAttributes(h.Attributes(), "series", 1)

			mockAppender := &mockCombinedAppender{}
			converter := NewPrometheusConverter(mockAppender)
			annots, err := converter.FromMetrics(
				context.Background(),
				request.Metrics(),
				Settings{ConvertHistogramsToNHCB: convertHistogramsToNHCB},
			)
			require.NoError(t, err)
			require.Empty(t, annots)

			if convertHistogramsToNHCB {
				require.Len(t, mockAppender.histograms, 1)
				require.Empty(t, mockAppender.samples)
			} else {
				require.Empty(t, mockAppender.histograms)
				require.Len(t, mockAppender.samples, 3)
			}
		})
	}

	t.Run("context cancellation", func(t *testing.T) {
		settings := Settings{}
		converter := NewPrometheusConverter(&mockCombinedAppender{})
		ctx, cancel := context.WithCancel(context.Background())
		// Verify that converter.FromMetrics respects cancellation.
		cancel()
		payload, _ := createExportRequest(5, 128, 128, 2, 0, settings, pmetric.AggregationTemporalityCumulative)

		annots, err := converter.FromMetrics(ctx, payload.Metrics(), settings)
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, annots)
	})

	t.Run("context timeout", func(t *testing.T) {
		settings := Settings{}
		converter := NewPrometheusConverter(&mockCombinedAppender{})
		// Verify that converter.FromMetrics respects timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		t.Cleanup(cancel)
		payload, _ := createExportRequest(5, 128, 128, 2, 0, settings, pmetric.AggregationTemporalityCumulative)

		annots, err := converter.FromMetrics(ctx, payload.Metrics(), settings)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Empty(t, annots)
	})

	t.Run("exponential histogram warnings for zero count and non-zero sum", func(t *testing.T) {
		request := pmetricotlp.NewExportRequest()
		rm := request.Metrics().ResourceMetrics().AppendEmpty()
		generateAttributes(rm.Resource().Attributes(), "resource", 10)

		metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
		ts := pcommon.NewTimestampFromTime(time.Now())

		for i := 1; i <= 10; i++ {
			m := metrics.AppendEmpty()
			m.SetEmptyExponentialHistogram()
			m.SetName(fmt.Sprintf("histogram-%d", i))
			m.ExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			h := m.ExponentialHistogram().DataPoints().AppendEmpty()
			h.SetTimestamp(ts)

			h.SetCount(0)
			h.SetSum(155)

			generateAttributes(h.Attributes(), "series", 10)
		}

		converter := NewPrometheusConverter(&mockCombinedAppender{})
		annots, err := converter.FromMetrics(context.Background(), request.Metrics(), Settings{})
		require.NoError(t, err)
		require.NotEmpty(t, annots)
		ws, infos := annots.AsStrings("", 0, 0)
		require.Empty(t, infos)
		require.Equal(t, []string{
			"exponential histogram data point has zero count, but non-zero sum: 155.000000",
		}, ws)
	})

	t.Run("explicit histogram to NHCB warnings for zero count and non-zero sum", func(t *testing.T) {
		request := pmetricotlp.NewExportRequest()
		rm := request.Metrics().ResourceMetrics().AppendEmpty()
		generateAttributes(rm.Resource().Attributes(), "resource", 10)

		metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
		ts := pcommon.NewTimestampFromTime(time.Now())

		for i := 1; i <= 10; i++ {
			m := metrics.AppendEmpty()
			m.SetEmptyHistogram()
			m.SetName(fmt.Sprintf("histogram-%d", i))
			m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			h := m.Histogram().DataPoints().AppendEmpty()
			h.SetTimestamp(ts)

			h.SetCount(0)
			h.SetSum(155)

			generateAttributes(h.Attributes(), "series", 10)
		}

		converter := NewPrometheusConverter(&mockCombinedAppender{})
		annots, err := converter.FromMetrics(
			context.Background(),
			request.Metrics(),
			Settings{ConvertHistogramsToNHCB: true},
		)
		require.NoError(t, err)
		require.NotEmpty(t, annots)
		ws, infos := annots.AsStrings("", 0, 0)
		require.Empty(t, infos)
		require.Equal(t, []string{
			"histogram data point has zero count, but non-zero sum: 155.000000",
		}, ws)
	})

	t.Run("target_info's samples starts at the earliest metric sample timestamp and ends at the latest sample timestamp of the corresponding resource, with one sample every lookback delta/2 timestamps between", func(t *testing.T) {
		request := pmetricotlp.NewExportRequest()
		rm := request.Metrics().ResourceMetrics().AppendEmpty()
		generateAttributes(rm.Resource().Attributes(), "resource", 5)

		// Fake some resource attributes.
		for k, v := range map[string]string{
			"service.name":        "test-service",
			"service.namespace":   "test-namespace",
			"service.instance.id": "id1234",
		} {
			rm.Resource().Attributes().PutStr(k, v)
		}
		metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
		ts := pcommon.NewTimestampFromTime(time.Now())

		for i := range 3 {
			m := metrics.AppendEmpty()
			m.SetEmptyGauge()
			m.SetName(fmt.Sprintf("gauge-%v", i+1))
			m.SetDescription("gauge")
			m.SetUnit("unit")
			// Add samples every lookback delta / 4 timestamps.
			curTs := ts.AsTime()
			for range 6 {
				point := m.Gauge().DataPoints().AppendEmpty()
				point.SetTimestamp(pcommon.NewTimestampFromTime(curTs))
				point.SetDoubleValue(1.23)
				generateAttributes(point.Attributes(), "series", 2)
				curTs = curTs.Add(defaultLookbackDelta / 4)
			}
		}

		mockAppender := &mockCombinedAppender{}
		converter := NewPrometheusConverter(mockAppender)
		settings := Settings{
			LookbackDelta: defaultLookbackDelta,
		}

		annots, err := converter.FromMetrics(context.Background(), request.Metrics(), settings)
		require.NoError(t, err)
		require.Empty(t, annots)

		require.Len(t, mockAppender.samples, 22)
		// There should be a target_info sample at the earliest metric timestamp, then two spaced lookback delta/2 apart,
		// then one at the latest metric timestamp.
		targetInfoLabels := labels.FromStrings(
			"__name__", "target_info",
			"instance", "id1234",
			"job", "test-namespace/test-service",
			"resource_name_1", "value-1",
			"resource_name_2", "value-2",
			"resource_name_3", "value-3",
			"resource_name_4", "value-4",
			"resource_name_5", "value-5",
		)
		targetInfoMeta := metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Target metadata",
		}
		requireEqual(t, []combinedSample{
			{
				metricFamilyName: "target_info",
				v:                1,
				t:                ts.AsTime().UnixMilli(),
				ls:               targetInfoLabels,
				meta:             targetInfoMeta,
			},
			{
				metricFamilyName: "target_info",
				v:                1,
				t:                ts.AsTime().Add(defaultLookbackDelta / 2).UnixMilli(),
				ls:               targetInfoLabels,
				meta:             targetInfoMeta,
			},
			{
				metricFamilyName: "target_info",
				v:                1,
				t:                ts.AsTime().Add(defaultLookbackDelta).UnixMilli(),
				ls:               targetInfoLabels,
				meta:             targetInfoMeta,
			},
			{
				metricFamilyName: "target_info",
				v:                1,
				t:                ts.AsTime().Add(defaultLookbackDelta + defaultLookbackDelta/4).UnixMilli(),
				ls:               targetInfoLabels,
				meta:             targetInfoMeta,
			},
		}, mockAppender.samples[len(mockAppender.samples)-4:])
	})
}

func TestTemporality(t *testing.T) {
	ts := time.Unix(100, 0)

	tests := []struct {
		name               string
		allowDelta         bool
		convertToNHCB      bool
		inputSeries        []pmetric.Metric
		expectedSamples    []combinedSample
		expectedHistograms []combinedHistogram
		expectedError      string
	}{
		{
			name:       "all cumulative when delta not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityCumulative, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_metric_1", ts, model.MetricTypeCounter),
				createPromFloatSeries("test_metric_2", ts, model.MetricTypeCounter),
			},
		},
		{
			name:       "all delta when allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityDelta, ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_metric_1", ts, model.MetricTypeUnknown),
				createPromFloatSeries("test_metric_2", ts, model.MetricTypeUnknown),
			},
		},
		{
			name:       "mixed temporality when delta allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_metric_1", ts, model.MetricTypeUnknown),
				createPromFloatSeries("test_metric_2", ts, model.MetricTypeCounter),
			},
		},
		{
			name:       "delta rejected when not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityCumulative, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityDelta, ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_metric_1", ts, model.MetricTypeCounter),
			},
			expectedError: `invalid temporality and type combination for metric "test_metric_2"`,
		},
		{
			name:       "unspecified temporality not allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityCumulative, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityUnspecified, ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_metric_1", ts, model.MetricTypeCounter),
			},
			expectedError: `invalid temporality and type combination for metric "test_metric_2"`,
		},
		{
			name:       "cumulative histogram",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNativeHistogramSeries("test_histogram", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
		},
		{
			name:       "delta histogram when allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExponentialHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNativeHistogramSeries("test_histogram_1", histogram.GaugeType, ts, model.MetricTypeUnknown),
				createPromNativeHistogramSeries("test_histogram_2", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
		},
		{
			name:       "delta histogram when not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExponentialHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNativeHistogramSeries("test_histogram_2", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
			expectedError: `invalid temporality and type combination for metric "test_histogram_1"`,
		},
		{
			name:          "cumulative histogram with buckets",
			allowDelta:    false,
			convertToNHCB: true,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNHCBSeries("test_histogram", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
		},
		{
			name:          "delta histogram with buckets when allowed",
			allowDelta:    true,
			convertToNHCB: true,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExplicitHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNHCBSeries("test_histogram_1", histogram.GaugeType, ts, model.MetricTypeUnknown),
				createPromNHCBSeries("test_histogram_2", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
		},
		{
			name:          "delta histogram with buckets when not allowed",
			allowDelta:    false,
			convertToNHCB: true,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExplicitHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedHistograms: []combinedHistogram{
				createPromNHCBSeries("test_histogram_2", histogram.UnknownCounterReset, ts, model.MetricTypeHistogram),
			},
			expectedError: `invalid temporality and type combination for metric "test_histogram_1"`,
		},
		{
			name:          "delta histogram with buckets and convertToNHCB=false when not allowed",
			allowDelta:    false,
			convertToNHCB: false,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExplicitHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSamples: createPromClassicHistogramSeries("test_histogram_2", ts, model.MetricTypeHistogram),
			expectedError:   `invalid temporality and type combination for metric "test_histogram_1"`,
		},
		{
			name:          "delta histogram with buckets and convertToNHCB=false when allowed",
			allowDelta:    true,
			convertToNHCB: false,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExplicitHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSamples: append(
				createPromClassicHistogramSeries("test_histogram_1", ts, model.MetricTypeUnknown),
				createPromClassicHistogramSeries("test_histogram_2", ts, model.MetricTypeHistogram)...,
			),
		},
		{
			name: "summary does not have temporality",
			inputSeries: []pmetric.Metric{
				createOtelSummary("test_summary_1", ts),
			},
			expectedSamples: createPromSummarySeries("test_summary_1", ts),
		},
		{
			name: "gauge does not have temporality",
			inputSeries: []pmetric.Metric{
				createOtelGauge("test_gauge_1", ts),
			},
			expectedSamples: []combinedSample{
				createPromFloatSeries("test_gauge_1", ts, model.MetricTypeGauge),
			},
		},
		{
			name: "empty metric type errors",
			inputSeries: []pmetric.Metric{
				createOtelEmptyType("test_empty"),
			},
			expectedError: `could not get aggregation temporality for test_empty as it has unsupported metric type Empty`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics := pmetric.NewMetrics()
			rm := metrics.ResourceMetrics().AppendEmpty()
			sm := rm.ScopeMetrics().AppendEmpty()

			for _, s := range tc.inputSeries {
				s.CopyTo(sm.Metrics().AppendEmpty())
			}

			mockAppender := &mockCombinedAppender{}
			c := NewPrometheusConverter(mockAppender)
			settings := Settings{
				AllowDeltaTemporality:   tc.allowDelta,
				ConvertHistogramsToNHCB: tc.convertToNHCB,
			}

			_, err := c.FromMetrics(context.Background(), metrics, settings)

			if tc.expectedError != "" {
				require.EqualError(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			// Sort series to make the test deterministic.
			requireEqual(t, tc.expectedSamples, mockAppender.samples)
			requireEqual(t, tc.expectedHistograms, mockAppender.histograms)
		})
	}
}

func createOtelSum(name string, temporality pmetric.AggregationTemporality, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	sum := m.SetEmptySum()
	sum.SetAggregationTemporality(temporality)
	sum.SetIsMonotonic(true)
	dp := sum.DataPoints().AppendEmpty()
	dp.SetDoubleValue(5)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createPromFloatSeries(name string, ts time.Time, typ model.MetricType) combinedSample {
	return combinedSample{
		metricFamilyName: name,
		ls:               labels.FromStrings("__name__", name, "test_label", "test_value"),
		t:                ts.UnixMilli(),
		v:                5,
		meta: metadata.Metadata{
			Type: typ,
		},
	}
}

func createOtelGauge(name string, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	gauge := m.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetDoubleValue(5)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createOtelExponentialHistogram(name string, temporality pmetric.AggregationTemporality, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	hist := m.SetEmptyExponentialHistogram()
	hist.SetAggregationTemporality(temporality)
	dp := hist.DataPoints().AppendEmpty()
	dp.SetCount(1)
	dp.SetSum(5)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createPromNativeHistogramSeries(name string, hint histogram.CounterResetHint, ts time.Time, typ model.MetricType) combinedHistogram {
	return combinedHistogram{
		metricFamilyName: name,
		ls:               labels.FromStrings("__name__", name, "test_label", "test_value"),
		t:                ts.UnixMilli(),
		meta: metadata.Metadata{
			Type: typ,
		},
		h: &histogram.Histogram{
			Count:            1,
			Sum:              5,
			Schema:           0,
			ZeroThreshold:    1e-128,
			ZeroCount:        0,
			CounterResetHint: hint,
		},
	}
}

func createOtelExplicitHistogram(name string, temporality pmetric.AggregationTemporality, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	hist := m.SetEmptyHistogram()
	hist.SetAggregationTemporality(temporality)
	dp := hist.DataPoints().AppendEmpty()
	dp.SetCount(20)
	dp.SetSum(30)
	dp.BucketCounts().FromRaw([]uint64{10, 10, 0})
	dp.ExplicitBounds().FromRaw([]float64{1, 2})
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createPromNHCBSeries(name string, hint histogram.CounterResetHint, ts time.Time, typ model.MetricType) combinedHistogram {
	return combinedHistogram{
		metricFamilyName: name,
		ls:               labels.FromStrings("__name__", name, "test_label", "test_value"),
		meta: metadata.Metadata{
			Type: typ,
		},
		t: ts.UnixMilli(),
		h: &histogram.Histogram{
			Count:         20,
			Sum:           30,
			Schema:        -53,
			ZeroThreshold: 0,
			PositiveSpans: []histogram.Span{
				{
					Length: 3,
				},
			},
			PositiveBuckets:  []int64{10, 0, -10},
			CustomValues:     []float64{1, 2},
			CounterResetHint: hint,
		},
	}
}

func createPromClassicHistogramSeries(name string, ts time.Time, typ model.MetricType) []combinedSample {
	return []combinedSample{
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_sum", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                30,
			meta: metadata.Metadata{
				Type: typ,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_count", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                20,
			meta: metadata.Metadata{
				Type: typ,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_bucket", "le", "1", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                10,
			meta: metadata.Metadata{
				Type: typ,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_bucket", "le", "2", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                20,
			meta: metadata.Metadata{
				Type: typ,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_bucket", "le", "+Inf", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                20,
			meta: metadata.Metadata{
				Type: typ,
			},
		},
	}
}

func createOtelSummary(name string, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	summary := m.SetEmptySummary()
	dp := summary.DataPoints().AppendEmpty()
	dp.SetCount(9)
	dp.SetSum(18)
	qv := dp.QuantileValues().AppendEmpty()
	qv.SetQuantile(0.5)
	qv.SetValue(2)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createPromSummarySeries(name string, ts time.Time) []combinedSample {
	return []combinedSample{
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_sum", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                18,
			meta: metadata.Metadata{
				Type: model.MetricTypeSummary,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name+"_count", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                9,
			meta: metadata.Metadata{
				Type: model.MetricTypeSummary,
			},
		},
		{
			metricFamilyName: name,
			ls:               labels.FromStrings("__name__", name, "quantile", "0.5", "test_label", "test_value"),
			t:                ts.UnixMilli(),
			v:                2,
			meta: metadata.Metadata{
				Type: model.MetricTypeSummary,
			},
		},
	}
}

func createOtelEmptyType(name string) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	return m
}

func TestTranslatorMetricFromOtelMetric(t *testing.T) {
	tests := []struct {
		name           string
		inputMetric    pmetric.Metric
		expectedMetric otlptranslator.Metric
	}{
		{
			name:        "gauge metric",
			inputMetric: createOTelGaugeForTranslator("test_gauge", "bytes", "Test gauge metric"),
			expectedMetric: otlptranslator.Metric{
				Name: "test_gauge",
				Unit: "bytes",
				Type: otlptranslator.MetricTypeGauge,
			},
		},
		{
			name:        "monotonic sum metric",
			inputMetric: createOTelSumForTranslator("test_sum", "count", "Test sum metric", true),
			expectedMetric: otlptranslator.Metric{
				Name: "test_sum",
				Unit: "count",
				Type: otlptranslator.MetricTypeMonotonicCounter,
			},
		},
		{
			name:        "non-monotonic sum metric",
			inputMetric: createOTelSumForTranslator("test_sum", "count", "Test sum metric", false),
			expectedMetric: otlptranslator.Metric{
				Name: "test_sum",
				Unit: "count",
				Type: otlptranslator.MetricTypeNonMonotonicCounter,
			},
		},
		{
			name:        "histogram metric",
			inputMetric: createOTelHistogramForTranslator("test_histogram", "seconds", "Test histogram metric"),
			expectedMetric: otlptranslator.Metric{
				Name: "test_histogram",
				Unit: "seconds",
				Type: otlptranslator.MetricTypeHistogram,
			},
		},
		{
			name:        "exponential histogram metric",
			inputMetric: createOTelExponentialHistogramForTranslator("test_exp_histogram", "milliseconds", "Test exponential histogram metric"),
			expectedMetric: otlptranslator.Metric{
				Name: "test_exp_histogram",
				Unit: "milliseconds",
				Type: otlptranslator.MetricTypeExponentialHistogram,
			},
		},
		{
			name:        "summary metric",
			inputMetric: createOTelSummaryForTranslator("test_summary", "duration", "Test summary metric"),
			expectedMetric: otlptranslator.Metric{
				Name: "test_summary",
				Unit: "duration",
				Type: otlptranslator.MetricTypeSummary,
			},
		},
		{
			name:        "empty metric name and unit",
			inputMetric: createOTelGaugeForTranslator("", "", ""),
			expectedMetric: otlptranslator.Metric{
				Name: "",
				Unit: "",
				Type: otlptranslator.MetricTypeGauge,
			},
		},
		{
			name:        "empty metric type defaults to unknown",
			inputMetric: createOTelEmptyMetricForTranslator("test_empty"),
			expectedMetric: otlptranslator.Metric{
				Name: "test_empty",
				Unit: "",
				Type: otlptranslator.MetricTypeUnknown,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := TranslatorMetricFromOtelMetric(tc.inputMetric)
			require.Equal(t, tc.expectedMetric, result)
		})
	}
}

func createOTelMetricForTranslator(name, unit, description string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName(name)
	m.SetUnit(unit)
	m.SetDescription(description)
	return m
}

func createOTelGaugeForTranslator(name, unit, description string) pmetric.Metric {
	m := createOTelMetricForTranslator(name, unit, description)
	m.SetEmptyGauge()
	return m
}

func createOTelSumForTranslator(name, unit, description string, isMonotonic bool) pmetric.Metric {
	m := createOTelMetricForTranslator(name, unit, description)
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(isMonotonic)
	return m
}

func createOTelHistogramForTranslator(name, unit, description string) pmetric.Metric {
	m := createOTelMetricForTranslator(name, unit, description)
	m.SetEmptyHistogram()
	return m
}

func createOTelExponentialHistogramForTranslator(name, unit, description string) pmetric.Metric {
	m := createOTelMetricForTranslator(name, unit, description)
	m.SetEmptyExponentialHistogram()
	return m
}

func createOTelSummaryForTranslator(name, unit, description string) pmetric.Metric {
	m := createOTelMetricForTranslator(name, unit, description)
	m.SetEmptySummary()
	return m
}

func createOTelEmptyMetricForTranslator(name string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName(name)
	return m
}

func BenchmarkPrometheusConverter_FromMetrics(b *testing.B) {
	for _, resourceAttributeCount := range []int{0, 5, 50} {
		b.Run(fmt.Sprintf("resource attribute count: %v", resourceAttributeCount), func(b *testing.B) {
			for _, histogramCount := range []int{0, 1000} {
				b.Run(fmt.Sprintf("histogram count: %v", histogramCount), func(b *testing.B) {
					nonHistogramCounts := []int{0, 1000}

					if histogramCount == 0 {
						// Don't bother running a scenario where we'll generate no series.
						nonHistogramCounts = []int{1000}
					}

					for _, nonHistogramCount := range nonHistogramCounts {
						b.Run(fmt.Sprintf("non-histogram count: %v", nonHistogramCount), func(b *testing.B) {
							for _, labelsPerMetric := range []int{2, 20} {
								b.Run(fmt.Sprintf("labels per metric: %v", labelsPerMetric), func(b *testing.B) {
									for _, exemplarsPerSeries := range []int{0, 5, 10} {
										b.Run(fmt.Sprintf("exemplars per series: %v", exemplarsPerSeries), func(b *testing.B) {
											settings := Settings{}
											payload, _ := createExportRequest(
												resourceAttributeCount,
												histogramCount,
												nonHistogramCount,
												labelsPerMetric,
												exemplarsPerSeries,
												settings,
												pmetric.AggregationTemporalityCumulative,
											)
											b.ResetTimer()

											for range b.N {
												mockAppender := &mockCombinedAppender{}
												converter := NewPrometheusConverter(mockAppender)
												annots, err := converter.FromMetrics(context.Background(), payload.Metrics(), settings)
												require.NoError(b, err)
												require.Empty(b, annots)
												require.Positive(b, len(mockAppender.samples)+len(mockAppender.histograms))
											}
										})
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

type wantPrometheusMetric struct {
	name        string
	familyName  string
	metricType  model.MetricType
	description string
	unit        string
}

func createExportRequest(resourceAttributeCount, histogramCount, nonHistogramCount, labelsPerMetric, exemplarsPerSeries int, settings Settings, temporality pmetric.AggregationTemporality) (pmetricotlp.ExportRequest, []wantPrometheusMetric) {
	request := pmetricotlp.NewExportRequest()

	rm := request.Metrics().ResourceMetrics().AppendEmpty()
	generateAttributes(rm.Resource().Attributes(), "resource", resourceAttributeCount)

	// Fake some resource attributes.
	for k, v := range map[string]string{
		"service.name":        "test-service",
		"service.namespace":   "test-namespace",
		"service.instance.id": "id1234",
	} {
		rm.Resource().Attributes().PutStr(k, v)
	}

	metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
	ts := pcommon.NewTimestampFromTime(time.Now())

	var suffix string
	if settings.AddMetricSuffixes {
		suffix = "_unit"
	}
	var wantPromMetrics []wantPrometheusMetric
	for i := 1; i <= histogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptyHistogram()
		m.SetName(fmt.Sprintf("histogram-%v", i))
		m.SetDescription("histogram")
		m.SetUnit("unit")
		m.Histogram().SetAggregationTemporality(temporality)
		h := m.Histogram().DataPoints().AppendEmpty()
		h.SetTimestamp(ts)

		// Set 50 samples, 10 each with values 0.5, 1, 2, 4, and 8
		h.SetCount(50)
		h.SetSum(155)
		h.BucketCounts().FromRaw([]uint64{10, 10, 10, 10, 10, 0})
		h.ExplicitBounds().FromRaw([]float64{.5, 1, 2, 4, 8, 16}) // Bucket boundaries include the upper limit (ie. each sample is on the upper limit of its bucket)

		generateAttributes(h.Attributes(), "series", labelsPerMetric)
		generateExemplars(h.Exemplars(), exemplarsPerSeries, ts)

		metricType := model.MetricTypeHistogram
		if temporality != pmetric.AggregationTemporalityCumulative {
			// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
			// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
			metricType = model.MetricTypeUnknown
		}
		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("histogram_%d%s_bucket", i, suffix),
			familyName:  fmt.Sprintf("histogram_%d%s", i, suffix),
			metricType:  metricType,
			unit:        "unit",
			description: "histogram",
		})
		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("histogram_%d%s_count", i, suffix),
			familyName:  fmt.Sprintf("histogram_%d%s", i, suffix),
			metricType:  metricType,
			unit:        "unit",
			description: "histogram",
		})
		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("histogram_%d%s_sum", i, suffix),
			familyName:  fmt.Sprintf("histogram_%d%s", i, suffix),
			metricType:  metricType,
			unit:        "unit",
			description: "histogram",
		})
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptySum()
		m.SetName(fmt.Sprintf("non.monotonic.sum-%v", i))
		m.SetDescription("sum")
		m.SetUnit("unit")
		m.Sum().SetAggregationTemporality(temporality)
		point := m.Sum().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)

		metricType := model.MetricTypeGauge
		if temporality != pmetric.AggregationTemporalityCumulative {
			// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
			// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
			metricType = model.MetricTypeUnknown
		}
		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("non_monotonic_sum_%d%s", i, suffix),
			familyName:  fmt.Sprintf("non_monotonic_sum_%d%s", i, suffix),
			metricType:  metricType,
			unit:        "unit",
			description: "sum",
		})
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptySum()
		m.SetName(fmt.Sprintf("monotonic.sum-%v", i))
		m.SetDescription("sum")
		m.SetUnit("unit")
		m.Sum().SetAggregationTemporality(temporality)
		m.Sum().SetIsMonotonic(true)
		point := m.Sum().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)

		var counterSuffix string
		if settings.AddMetricSuffixes {
			counterSuffix = suffix + "_total"
		}

		metricType := model.MetricTypeCounter
		if temporality != pmetric.AggregationTemporalityCumulative {
			// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
			// We don't have a proper way to flag delta metrics yet, therefore marking the metric type as unknown for now.
			metricType = model.MetricTypeUnknown
		}
		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("monotonic_sum_%d%s", i, counterSuffix),
			familyName:  fmt.Sprintf("monotonic_sum_%d%s", i, counterSuffix),
			metricType:  metricType,
			unit:        "unit",
			description: "sum",
		})
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptyGauge()
		m.SetName(fmt.Sprintf("gauge-%v", i))
		m.SetDescription("gauge")
		m.SetUnit("unit")
		point := m.Gauge().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)

		wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
			name:        fmt.Sprintf("gauge_%d%s", i, suffix),
			familyName:  fmt.Sprintf("gauge_%d%s", i, suffix),
			metricType:  model.MetricTypeGauge,
			unit:        "unit",
			description: "gauge",
		})
	}

	wantPromMetrics = append(wantPromMetrics, wantPrometheusMetric{
		name:       "target_info",
		familyName: "target_info",
	})
	return request, wantPromMetrics
}

func generateAttributes(m pcommon.Map, prefix string, count int) {
	for i := 1; i <= count; i++ {
		m.PutStr(fmt.Sprintf("%v-name-%v", prefix, i), fmt.Sprintf("value-%v", i))
	}
}

func generateExemplars(exemplars pmetric.ExemplarSlice, count int, ts pcommon.Timestamp) {
	for i := 1; i <= count; i++ {
		e := exemplars.AppendEmpty()
		e.SetTimestamp(ts)
		e.SetDoubleValue(2.22)
		e.SetSpanID(pcommon.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
		e.SetTraceID(pcommon.TraceID{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f})
	}
}
