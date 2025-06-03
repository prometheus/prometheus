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
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestFromMetrics(t *testing.T) {
	for _, keepIdentifyingResourceAttributes := range []bool{false, true} {
		t.Run(fmt.Sprintf("successful/keepIdentifyingAttributes=%v", keepIdentifyingResourceAttributes), func(t *testing.T) {
			converter := NewPrometheusConverter()
			payload := createExportRequest(5, 128, 128, 2, 0)
			var expMetadata []prompb.MetricMetadata
			resourceMetricsSlice := payload.Metrics().ResourceMetrics()
			for i := 0; i < resourceMetricsSlice.Len(); i++ {
				scopeMetricsSlice := resourceMetricsSlice.At(i).ScopeMetrics()
				for j := 0; j < scopeMetricsSlice.Len(); j++ {
					metricSlice := scopeMetricsSlice.At(j).Metrics()
					for k := 0; k < metricSlice.Len(); k++ {
						metric := metricSlice.At(k)
						namer := otlptranslator.MetricNamer{}
						promName := namer.Build(TranslatorMetricFromOtelMetric(metric))
						expMetadata = append(expMetadata, prompb.MetricMetadata{
							Type:             otelMetricTypeToPromMetricType(metric),
							MetricFamilyName: promName,
							Help:             metric.Description(),
							Unit:             metric.Unit(),
						})
					}
				}
			}

			annots, err := converter.FromMetrics(
				context.Background(),
				payload.Metrics(),
				Settings{KeepIdentifyingResourceAttributes: keepIdentifyingResourceAttributes},
			)
			require.NoError(t, err)
			require.Empty(t, annots)

			if diff := cmp.Diff(expMetadata, converter.Metadata()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}

			ts := converter.TimeSeries()
			require.Len(t, ts, 1408+1) // +1 for the target_info.

			tgtInfoCount := 0
			for _, s := range ts {
				b := labels.NewScratchBuilder(2)
				lbls := s.ToLabels(&b, nil)
				if lbls.Get(labels.MetricName) == "target_info" {
					tgtInfoCount++
					require.Equal(t, "test-namespace/test-service", lbls.Get("job"))
					require.Equal(t, "id1234", lbls.Get("instance"))
					if keepIdentifyingResourceAttributes {
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

			converter := NewPrometheusConverter()
			annots, err := converter.FromMetrics(
				context.Background(),
				request.Metrics(),
				Settings{ConvertHistogramsToNHCB: convertHistogramsToNHCB},
			)
			require.NoError(t, err)
			require.Empty(t, annots)

			series := converter.TimeSeries()

			if convertHistogramsToNHCB {
				require.Len(t, series[0].Histograms, 1)
				require.Empty(t, series[0].Samples)
			} else {
				require.Len(t, series, 3)
				for i := range series {
					require.Len(t, series[i].Samples, 1)
					require.Nil(t, series[i].Histograms)
				}
			}
		})
	}

	t.Run("context cancellation", func(t *testing.T) {
		converter := NewPrometheusConverter()
		ctx, cancel := context.WithCancel(context.Background())
		// Verify that converter.FromMetrics respects cancellation.
		cancel()
		payload := createExportRequest(5, 128, 128, 2, 0)

		annots, err := converter.FromMetrics(ctx, payload.Metrics(), Settings{})
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, annots)
	})

	t.Run("context timeout", func(t *testing.T) {
		converter := NewPrometheusConverter()
		// Verify that converter.FromMetrics respects timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		t.Cleanup(cancel)
		payload := createExportRequest(5, 128, 128, 2, 0)

		annots, err := converter.FromMetrics(ctx, payload.Metrics(), Settings{})
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

		converter := NewPrometheusConverter()
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

		converter := NewPrometheusConverter()
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
}

func TestTemporality(t *testing.T) {
	ts := time.Unix(100, 0)

	tests := []struct {
		name           string
		allowDelta     bool
		convertToNHCB  bool
		inputSeries    []pmetric.Metric
		expectedSeries []prompb.TimeSeries
		expectedError  string
	}{
		{
			name:       "all cumulative when delta not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityCumulative, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_metric_1", ts),
				createPromFloatSeries("test_metric_2", ts),
			},
		},
		{
			name:       "all delta when allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityDelta, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_metric_1", ts),
				createPromFloatSeries("test_metric_2", ts),
			},
		},
		{
			name:       "mixed temporality when delta allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_metric_1", ts),
				createPromFloatSeries("test_metric_2", ts),
			},
		},
		{
			name:       "delta rejected when not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelSum("test_metric_1", pmetric.AggregationTemporalityCumulative, ts),
				createOtelSum("test_metric_2", pmetric.AggregationTemporalityDelta, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_metric_1", ts),
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
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_metric_1", ts),
			},
			expectedError: `invalid temporality and type combination for metric "test_metric_2"`,
		},
		{
			name:       "cumulative histogram",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromNativeHistogramSeries("test_histogram", prompb.Histogram_UNKNOWN, ts),
			},
		},
		{
			name:       "delta histogram when allowed",
			allowDelta: true,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExponentialHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromNativeHistogramSeries("test_histogram_1", prompb.Histogram_GAUGE, ts),
				createPromNativeHistogramSeries("test_histogram_2", prompb.Histogram_UNKNOWN, ts),
			},
		},
		{
			name:       "delta histogram when not allowed",
			allowDelta: false,
			inputSeries: []pmetric.Metric{
				createOtelExponentialHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExponentialHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromNativeHistogramSeries("test_histogram_2", prompb.Histogram_UNKNOWN, ts),
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
			expectedSeries: []prompb.TimeSeries{
				createPromNHCBSeries("test_histogram", prompb.Histogram_UNKNOWN, ts),
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
			expectedSeries: []prompb.TimeSeries{
				createPromNHCBSeries("test_histogram_1", prompb.Histogram_GAUGE, ts),
				createPromNHCBSeries("test_histogram_2", prompb.Histogram_UNKNOWN, ts),
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
			expectedSeries: []prompb.TimeSeries{
				createPromNHCBSeries("test_histogram_2", prompb.Histogram_UNKNOWN, ts),
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
			expectedSeries: createPromClassicHistogramSeries("test_histogram_2", ts),
			expectedError:  `invalid temporality and type combination for metric "test_histogram_1"`,
		},
		{
			name:          "delta histogram with buckets and convertToNHCB=false when allowed",
			allowDelta:    true,
			convertToNHCB: false,
			inputSeries: []pmetric.Metric{
				createOtelExplicitHistogram("test_histogram_1", pmetric.AggregationTemporalityDelta, ts),
				createOtelExplicitHistogram("test_histogram_2", pmetric.AggregationTemporalityCumulative, ts),
			},
			expectedSeries: append(
				createPromClassicHistogramSeries("test_histogram_1", ts),
				createPromClassicHistogramSeries("test_histogram_2", ts)...,
			),
		},
		{
			name: "summary does not have temporality",
			inputSeries: []pmetric.Metric{
				createOtelSummary("test_summary_1", ts),
			},
			expectedSeries: createPromSummarySeries("test_summary_1", ts),
		},
		{
			name: "gauge does not have temporality",
			inputSeries: []pmetric.Metric{
				createOtelGauge("test_gauge_1", ts),
			},
			expectedSeries: []prompb.TimeSeries{
				createPromFloatSeries("test_gauge_1", ts),
			},
		},
		{
			name: "empty metric type errors",
			inputSeries: []pmetric.Metric{
				createOtelEmptyType("test_empty"),
			},
			expectedSeries: []prompb.TimeSeries{},
			expectedError:  `could not get aggregation temporality for test_empty as it has unsupported metric type Empty`,
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

			c := NewPrometheusConverter()
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

			series := c.TimeSeries()

			// Sort series to make the test deterministic.
			testutil.RequireEqual(t, sortTimeSeries(tc.expectedSeries), sortTimeSeries(series))
		})
	}
}

func createOtelSum(name string, temporality pmetric.AggregationTemporality, ts time.Time) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	sum := m.SetEmptySum()
	sum.SetAggregationTemporality(temporality)
	dp := sum.DataPoints().AppendEmpty()
	dp.SetDoubleValue(5)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))
	dp.Attributes().PutStr("test_label", "test_value")
	return m
}

func createPromFloatSeries(name string, ts time.Time) prompb.TimeSeries {
	return prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "__name__", Value: name},
			{Name: "test_label", Value: "test_value"},
		},
		Samples: []prompb.Sample{{
			Value:     5,
			Timestamp: ts.UnixMilli(),
		}},
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

func createPromNativeHistogramSeries(name string, hint prompb.Histogram_ResetHint, ts time.Time) prompb.TimeSeries {
	return prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "__name__", Value: name},
			{Name: "test_label", Value: "test_value"},
		},
		Histograms: []prompb.Histogram{
			{
				Count:         &prompb.Histogram_CountInt{CountInt: 1},
				Sum:           5,
				Schema:        0,
				ZeroThreshold: 1e-128,
				ZeroCount:     &prompb.Histogram_ZeroCountInt{ZeroCountInt: 0},
				Timestamp:     ts.UnixMilli(),
				ResetHint:     hint,
			},
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

func createPromNHCBSeries(name string, hint prompb.Histogram_ResetHint, ts time.Time) prompb.TimeSeries {
	return prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "__name__", Value: name},
			{Name: "test_label", Value: "test_value"},
		},
		Histograms: []prompb.Histogram{
			{
				Count:         &prompb.Histogram_CountInt{CountInt: 20},
				Sum:           30,
				Schema:        -53,
				ZeroThreshold: 0,
				ZeroCount:     nil,
				PositiveSpans: []prompb.BucketSpan{
					{
						Length: 3,
					},
				},
				PositiveDeltas: []int64{10, 0, -10},
				CustomValues:   []float64{1, 2},
				Timestamp:      ts.UnixMilli(),
				ResetHint:      hint,
			},
		},
	}
}

func createPromClassicHistogramSeries(name string, ts time.Time) []prompb.TimeSeries {
	return []prompb.TimeSeries{
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_bucket"},
				{Name: "le", Value: "1"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{Value: 10, Timestamp: ts.UnixMilli()}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_bucket"},
				{Name: "le", Value: "2"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{Value: 20, Timestamp: ts.UnixMilli()}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_bucket"},
				{Name: "le", Value: "+Inf"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{Value: 20, Timestamp: ts.UnixMilli()}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_count"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{Value: 20, Timestamp: ts.UnixMilli()}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_sum"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{Value: 30, Timestamp: ts.UnixMilli()}},
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

func createPromSummarySeries(name string, ts time.Time) []prompb.TimeSeries {
	return []prompb.TimeSeries{
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_sum"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{
				Value:     18,
				Timestamp: ts.UnixMilli(),
			}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name + "_count"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{
				Value:     9,
				Timestamp: ts.UnixMilli(),
			}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "quantile", Value: "0.5"},
				{Name: "test_label", Value: "test_value"},
			},
			Samples: []prompb.Sample{{
				Value:     2,
				Timestamp: ts.UnixMilli(),
			}},
		},
	}
}

func createOtelEmptyType(name string) pmetric.Metric {
	metrics := pmetric.NewMetricSlice()
	m := metrics.AppendEmpty()
	m.SetName(name)
	return m
}

func sortTimeSeries(series []prompb.TimeSeries) []prompb.TimeSeries {
	for i := range series {
		sort.Slice(series[i].Labels, func(j, k int) bool {
			return series[i].Labels[j].Name < series[i].Labels[k].Name
		})
	}

	sort.Slice(series, func(i, j int) bool {
		return fmt.Sprint(series[i].Labels) < fmt.Sprint(series[j].Labels)
	})

	return series
}

func BenchmarkPrometheusConverter_FromMetrics(b *testing.B) {
	for _, resourceAttributeCount := range []int{0, 5, 50} {
		b.Run(fmt.Sprintf("resource attribute count: %v", resourceAttributeCount), func(b *testing.B) {
			for _, histogramCount := range []int{0, 1000} {
				b.Run(fmt.Sprintf("histogram count: %v", histogramCount), func(b *testing.B) {
					nonHistogramCounts := []int{0, 1000}

					if resourceAttributeCount == 0 && histogramCount == 0 {
						// Don't bother running a scenario where we'll generate no series.
						nonHistogramCounts = []int{1000}
					}

					for _, nonHistogramCount := range nonHistogramCounts {
						b.Run(fmt.Sprintf("non-histogram count: %v", nonHistogramCount), func(b *testing.B) {
							for _, labelsPerMetric := range []int{2, 20} {
								b.Run(fmt.Sprintf("labels per metric: %v", labelsPerMetric), func(b *testing.B) {
									for _, exemplarsPerSeries := range []int{0, 5, 10} {
										b.Run(fmt.Sprintf("exemplars per series: %v", exemplarsPerSeries), func(b *testing.B) {
											payload := createExportRequest(resourceAttributeCount, histogramCount, nonHistogramCount, labelsPerMetric, exemplarsPerSeries)
											b.ResetTimer()

											for range b.N {
												converter := NewPrometheusConverter()
												annots, err := converter.FromMetrics(context.Background(), payload.Metrics(), Settings{})
												require.NoError(b, err)
												require.Empty(b, annots)
												require.NotNil(b, converter.TimeSeries())
												require.NotNil(b, converter.Metadata())
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

func createExportRequest(resourceAttributeCount, histogramCount, nonHistogramCount, labelsPerMetric, exemplarsPerSeries int) pmetricotlp.ExportRequest {
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

	for i := 1; i <= histogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptyHistogram()
		m.SetName(fmt.Sprintf("histogram-%v", i))
		m.SetDescription("histogram")
		m.SetUnit("unit")
		m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		h := m.Histogram().DataPoints().AppendEmpty()
		h.SetTimestamp(ts)

		// Set 50 samples, 10 each with values 0.5, 1, 2, 4, and 8
		h.SetCount(50)
		h.SetSum(155)
		h.BucketCounts().FromRaw([]uint64{10, 10, 10, 10, 10, 0})
		h.ExplicitBounds().FromRaw([]float64{.5, 1, 2, 4, 8, 16}) // Bucket boundaries include the upper limit (ie. each sample is on the upper limit of its bucket)

		generateAttributes(h.Attributes(), "series", labelsPerMetric)
		generateExemplars(h.Exemplars(), exemplarsPerSeries, ts)
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptySum()
		m.SetName(fmt.Sprintf("sum-%v", i))
		m.SetDescription("sum")
		m.SetUnit("unit")
		m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		point := m.Sum().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)
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
	}

	return request
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
