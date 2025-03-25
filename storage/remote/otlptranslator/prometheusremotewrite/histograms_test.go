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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/247a9f996e09a83cdc25addf70c05e42b8b30186/pkg/translator/prometheusremotewrite/histograms_test.go
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

	"github.com/prometheus/prometheus/prompb"
)

type expectedBucketLayout struct {
	wantSpans  []prompb.BucketSpan
	wantDeltas []int64
}

func TestConvertBucketsLayout(t *testing.T) {
	tests := []struct {
		name       string
		buckets    func() pmetric.ExponentialHistogramDataPointBuckets
		wantLayout map[int32]expectedBucketLayout
	}{
		{
			name: "zero offset",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(0)
				b.BucketCounts().FromRaw([]uint64{4, 3, 2, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 1,
							Length: 4,
						},
					},
					wantDeltas: []int64{4, -1, -1, -1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 1,
							Length: 2,
						},
					},
					// 4+3, 2+1 = 7, 3 =delta= 7, -4
					wantDeltas: []int64{7, -4},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 1,
							Length: 1,
						},
					},
					// 4+3+2+1 = 10 =delta= 10
					wantDeltas: []int64{10},
				},
			},
		},
		{
			name: "offset 1",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(1)
				b.BucketCounts().FromRaw([]uint64{4, 3, 2, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 2,
							Length: 4,
						},
					},
					wantDeltas: []int64{4, -1, -1, -1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 1,
							Length: 3,
						},
					},
					wantDeltas: []int64{4, 1, -4}, // 0+4, 3+2, 1+0 = 4, 5, 1
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 1,
							Length: 2,
						},
					},
					wantDeltas: []int64{9, -8}, // 0+4+3+2, 1+0+0+0 = 9, 1
				},
			},
		},
		{
			name: "positive offset",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(4)
				b.BucketCounts().FromRaw([]uint64{4, 2, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 5,
							Length: 4,
						},
						{
							Offset: 12,
							Length: 1,
						},
					},
					wantDeltas: []int64{4, -2, -2, 2, -1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 3,
							Length: 2,
						},
						{
							Offset: 6,
							Length: 1,
						},
					},
					// Downscale:
					// 4+2, 0+2, 0+0, 0+0, 0+0, 0+0, 0+0, 0+0, 1+0 = 6, 2, 0, 0, 0, 0, 0, 0, 1
					wantDeltas: []int64{6, -4, -1},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 2,
							Length: 1,
						},
						{
							Offset: 3,
							Length: 1,
						},
					},
					// Downscale:
					// 4+2+0+2, 0+0+0+0, 0+0+0+0, 0+0+0+0, 1+0+0+0 = 8, 0, 0, 0, 1
					// Check from scaling from previous: 6+2, 0+0, 0+0, 0+0, 1+0 = 8, 0, 0, 0, 1
					wantDeltas: []int64{8, -7},
				},
			},
		},
		{
			name: "scaledown merges spans",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(4)
				b.BucketCounts().FromRaw([]uint64{4, 2, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 5,
							Length: 4,
						},
						{
							Offset: 8,
							Length: 1,
						},
					},
					wantDeltas: []int64{4, -2, -2, 2, -1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 3,
							Length: 2,
						},
						{
							Offset: 4,
							Length: 1,
						},
					},
					// Downscale:
					// 4+2, 0+2, 0+0, 0+0, 0+0, 0+0, 1+0 = 6, 2, 0, 0, 0, 0, 1
					wantDeltas: []int64{6, -4, -1},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 2,
							Length: 4,
						},
					},
					// Downscale:
					// 4+2+0+2, 0+0+0+0, 0+0+0+0, 1+0+0+0 = 8, 0, 0, 1
					// Check from scaling from previous: 6+2, 0+0, 0+0, 1+0 = 8, 0, 0, 1
					wantDeltas: []int64{8, -8, 0, 1},
				},
			},
		},
		{
			name: "negative offset",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(-2)
				b.BucketCounts().FromRaw([]uint64{3, 1, 0, 0, 0, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: -1,
							Length: 2,
						},
						{
							Offset: 3,
							Length: 1,
						},
					},
					wantDeltas: []int64{3, -2, 0},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 3,
						},
					},
					// Downscale:
					// 3+1, 0+0, 0+1 = 4, 0, 1
					wantDeltas: []int64{4, -4, 1},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 2,
						},
					},
					// Downscale:
					// 0+0+3+1, 0+0+0+0 = 4, 1
					wantDeltas: []int64{4, -3},
				},
			},
		},
		{
			name: "buckets with gaps of size 1",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(-2)
				b.BucketCounts().FromRaw([]uint64{3, 1, 0, 1, 0, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: -1,
							Length: 6,
						},
					},
					wantDeltas: []int64{3, -2, -1, 1, -1, 1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 3,
						},
					},
					// Downscale:
					// 3+1, 0+1, 0+1 = 4, 1, 1
					wantDeltas: []int64{4, -3, 0},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 2,
						},
					},
					// Downscale:
					// 0+0+3+1, 0+1+0+1 = 4, 2
					wantDeltas: []int64{4, -2},
				},
			},
		},
		{
			name: "buckets with gaps of size 2",
			buckets: func() pmetric.ExponentialHistogramDataPointBuckets {
				b := pmetric.NewExponentialHistogramDataPointBuckets()
				b.SetOffset(-2)
				b.BucketCounts().FromRaw([]uint64{3, 0, 0, 1, 0, 0, 1})
				return b
			},
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: -1,
							Length: 7,
						},
					},
					wantDeltas: []int64{3, -3, 0, 1, -1, 0, 1},
				},
				1: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 4,
						},
					},
					// Downscale:
					// 3+0, 0+1, 0+0, 0+1 = 3, 1, 0, 1
					wantDeltas: []int64{3, -2, -1, 1},
				},
				2: {
					wantSpans: []prompb.BucketSpan{
						{
							Offset: 0,
							Length: 3,
						},
					},
					// Downscale:
					// 0+0+3+0, 0+1+0+0, 1+0+0+0 = 3, 1, 1
					wantDeltas: []int64{3, -2, 0},
				},
			},
		},
		{
			name:    "zero buckets",
			buckets: pmetric.NewExponentialHistogramDataPointBuckets,
			wantLayout: map[int32]expectedBucketLayout{
				0: {
					wantSpans:  nil,
					wantDeltas: nil,
				},
				1: {
					wantSpans:  nil,
					wantDeltas: nil,
				},
				2: {
					wantSpans:  nil,
					wantDeltas: nil,
				},
			},
		},
	}
	for _, tt := range tests {
		for scaleDown, wantLayout := range tt.wantLayout {
			t.Run(fmt.Sprintf("%s-scaleby-%d", tt.name, scaleDown), func(t *testing.T) {
				gotSpans, gotDeltas := convertBucketsLayout(tt.buckets().BucketCounts().AsRaw(), tt.buckets().Offset(), scaleDown, true)
				require.Equal(t, wantLayout.wantSpans, gotSpans)
				require.Equal(t, wantLayout.wantDeltas, gotDeltas)
			})
		}
	}
}

func BenchmarkConvertBucketLayout(b *testing.B) {
	scenarios := []struct {
		gap int
	}{
		{gap: 0},
		{gap: 1},
		{gap: 2},
		{gap: 3},
	}

	for _, scenario := range scenarios {
		buckets := pmetric.NewExponentialHistogramDataPointBuckets()
		buckets.SetOffset(0)
		for i := 0; i < 1000; i++ {
			if i%(scenario.gap+1) == 0 {
				buckets.BucketCounts().Append(10)
			} else {
				buckets.BucketCounts().Append(0)
			}
		}
		b.Run(fmt.Sprintf("gap %d", scenario.gap), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				convertBucketsLayout(buckets.BucketCounts().AsRaw(), buckets.Offset(), 0, true)
			}
		})
	}
}

func TestExponentialToNativeHistogram(t *testing.T) {
	tests := []struct {
		name            string
		exponentialHist func() pmetric.ExponentialHistogramDataPoint
		wantNativeHist  func() prompb.Histogram
		wantErrMessage  string
	}{
		{
			name: "convert exp. to native histogram",
			exponentialHist: func() pmetric.ExponentialHistogramDataPoint {
				pt := pmetric.NewExponentialHistogramDataPoint()
				pt.SetStartTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(100)))
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))
				pt.SetCount(4)
				pt.SetSum(10.1)
				pt.SetScale(1)
				pt.SetZeroCount(1)

				pt.Positive().BucketCounts().FromRaw([]uint64{1, 1})
				pt.Positive().SetOffset(1)

				pt.Negative().BucketCounts().FromRaw([]uint64{1, 1})
				pt.Negative().SetOffset(1)

				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 4},
					Sum:            10.1,
					Schema:         1,
					ZeroThreshold:  defaultZeroThreshold,
					ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 1},
					NegativeSpans:  []prompb.BucketSpan{{Offset: 2, Length: 2}},
					NegativeDeltas: []int64{1, 0},
					PositiveSpans:  []prompb.BucketSpan{{Offset: 2, Length: 2}},
					PositiveDeltas: []int64{1, 0},
					Timestamp:      500,
				}
			},
		},
		{
			name: "convert exp. to native histogram with no sum",
			exponentialHist: func() pmetric.ExponentialHistogramDataPoint {
				pt := pmetric.NewExponentialHistogramDataPoint()
				pt.SetStartTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(100)))
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))

				pt.SetCount(4)
				pt.SetScale(1)
				pt.SetZeroCount(1)

				pt.Positive().BucketCounts().FromRaw([]uint64{1, 1})
				pt.Positive().SetOffset(1)

				pt.Negative().BucketCounts().FromRaw([]uint64{1, 1})
				pt.Negative().SetOffset(1)

				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 4},
					Schema:         1,
					ZeroThreshold:  defaultZeroThreshold,
					ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 1},
					NegativeSpans:  []prompb.BucketSpan{{Offset: 2, Length: 2}},
					NegativeDeltas: []int64{1, 0},
					PositiveSpans:  []prompb.BucketSpan{{Offset: 2, Length: 2}},
					PositiveDeltas: []int64{1, 0},
					Timestamp:      500,
				}
			},
		},
		{
			name: "invalid negative scale",
			exponentialHist: func() pmetric.ExponentialHistogramDataPoint {
				pt := pmetric.NewExponentialHistogramDataPoint()
				pt.SetScale(-10)
				return pt
			},
			wantErrMessage: "cannot convert exponential to native histogram." +
				" Scale must be >= -4, was -10",
		},
		{
			name: "no downscaling at scale 8",
			exponentialHist: func() pmetric.ExponentialHistogramDataPoint {
				pt := pmetric.NewExponentialHistogramDataPoint()
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))
				pt.SetCount(6)
				pt.SetSum(10.1)
				pt.SetScale(8)
				pt.SetZeroCount(1)

				pt.Positive().BucketCounts().FromRaw([]uint64{1, 1, 1})
				pt.Positive().SetOffset(1)

				pt.Negative().BucketCounts().FromRaw([]uint64{1, 1, 1})
				pt.Negative().SetOffset(2)
				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 6},
					Sum:            10.1,
					Schema:         8,
					ZeroThreshold:  defaultZeroThreshold,
					ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 1},
					PositiveSpans:  []prompb.BucketSpan{{Offset: 2, Length: 3}},
					PositiveDeltas: []int64{1, 0, 0}, // 1, 1, 1
					NegativeSpans:  []prompb.BucketSpan{{Offset: 3, Length: 3}},
					NegativeDeltas: []int64{1, 0, 0}, // 1, 1, 1
					Timestamp:      500,
				}
			},
		},
		{
			name: "downsample if scale is more than 8",
			exponentialHist: func() pmetric.ExponentialHistogramDataPoint {
				pt := pmetric.NewExponentialHistogramDataPoint()
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))
				pt.SetCount(6)
				pt.SetSum(10.1)
				pt.SetScale(9)
				pt.SetZeroCount(1)

				pt.Positive().BucketCounts().FromRaw([]uint64{1, 1, 1})
				pt.Positive().SetOffset(1)

				pt.Negative().BucketCounts().FromRaw([]uint64{1, 1, 1})
				pt.Negative().SetOffset(2)
				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 6},
					Sum:            10.1,
					Schema:         8,
					ZeroThreshold:  defaultZeroThreshold,
					ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 1},
					PositiveSpans:  []prompb.BucketSpan{{Offset: 1, Length: 2}},
					PositiveDeltas: []int64{1, 1}, // 0+1, 1+1 = 1, 2
					NegativeSpans:  []prompb.BucketSpan{{Offset: 2, Length: 2}},
					NegativeDeltas: []int64{2, -1}, // 1+1, 1+0 = 2, 1
					Timestamp:      500,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validateExponentialHistogramCount(t, tt.exponentialHist()) // Sanity check.
			got, annots, err := exponentialToNativeHistogram(tt.exponentialHist())
			if tt.wantErrMessage != "" {
				require.ErrorContains(t, err, tt.wantErrMessage)
				return
			}

			require.NoError(t, err)
			require.Empty(t, annots)
			require.Equal(t, tt.wantNativeHist(), got)
			validateNativeHistogramCount(t, got)
		})
	}
}

func validateHistogramCount(t *testing.T, h pmetric.HistogramDataPoint) {
	actualCount := uint64(0)
	for _, bucket := range h.BucketCounts().AsRaw() {
		actualCount += bucket
	}
	require.Equal(t, h.Count(), actualCount, "histogram count mismatch")
}

func validateExponentialHistogramCount(t *testing.T, h pmetric.ExponentialHistogramDataPoint) {
	actualCount := uint64(0)
	for _, bucket := range h.Positive().BucketCounts().AsRaw() {
		actualCount += bucket
	}
	for _, bucket := range h.Negative().BucketCounts().AsRaw() {
		actualCount += bucket
	}
	require.Equal(t, h.Count(), actualCount, "exponential histogram count mismatch")
}

func validateNativeHistogramCount(t *testing.T, h prompb.Histogram) {
	require.NotNil(t, h.Count)
	require.IsType(t, &prompb.Histogram_CountInt{}, h.Count)
	want := h.Count.(*prompb.Histogram_CountInt).CountInt
	var (
		actualCount uint64
		prevBucket  int64
	)
	for _, delta := range h.PositiveDeltas {
		prevBucket += delta
		actualCount += uint64(prevBucket)
	}
	prevBucket = 0
	for _, delta := range h.NegativeDeltas {
		prevBucket += delta
		actualCount += uint64(prevBucket)
	}
	require.Equal(t, want, actualCount, "native histogram count mismatch")
}

func TestPrometheusConverter_addExponentialHistogramDataPoints(t *testing.T) {
	tests := []struct {
		name       string
		metric     func() pmetric.Metric
		wantSeries func() map[uint64]*prompb.TimeSeries
	}{
		{
			name: "histogram data points with same labels",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist")
				metric.SetEmptyExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.ExponentialHistogram().DataPoints().AppendEmpty()
				pt.SetCount(7)
				pt.SetScale(1)
				pt.Positive().SetOffset(-1)
				pt.Positive().BucketCounts().FromRaw([]uint64{4, 2})
				pt.Exemplars().AppendEmpty().SetDoubleValue(1)
				pt.Attributes().PutStr("attr", "test_attr")

				pt = metric.ExponentialHistogram().DataPoints().AppendEmpty()
				pt.SetCount(4)
				pt.SetScale(1)
				pt.Positive().SetOffset(-1)
				pt.Positive().BucketCounts().FromRaw([]uint64{4, 2, 1})
				pt.Exemplars().AppendEmpty().SetDoubleValue(2)
				pt.Attributes().PutStr("attr", "test_attr")

				return metric
			},
			wantSeries: func() map[uint64]*prompb.TimeSeries {
				labels := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist"},
					{Name: "attr", Value: "test_attr"},
				}
				return map[uint64]*prompb.TimeSeries{
					timeSeriesSignature(labels): {
						Labels: labels,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 7},
								Schema:         1,
								ZeroThreshold:  defaultZeroThreshold,
								ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 0},
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
								PositiveDeltas: []int64{4, -2},
							},
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 4},
								Schema:         1,
								ZeroThreshold:  defaultZeroThreshold,
								ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 0},
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 3}},
								PositiveDeltas: []int64{4, -2, -1},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 1},
							{Value: 2},
						},
					},
				}
			},
		},
		{
			name: "histogram data points with different labels",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist")
				metric.SetEmptyExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.ExponentialHistogram().DataPoints().AppendEmpty()
				pt.SetCount(7)
				pt.SetScale(1)
				pt.Positive().SetOffset(-1)
				pt.Positive().BucketCounts().FromRaw([]uint64{4, 2})
				pt.Exemplars().AppendEmpty().SetDoubleValue(1)
				pt.Attributes().PutStr("attr", "test_attr")

				pt = metric.ExponentialHistogram().DataPoints().AppendEmpty()
				pt.SetCount(4)
				pt.SetScale(1)
				pt.Negative().SetOffset(-1)
				pt.Negative().BucketCounts().FromRaw([]uint64{4, 2, 1})
				pt.Exemplars().AppendEmpty().SetDoubleValue(2)
				pt.Attributes().PutStr("attr", "test_attr_two")

				return metric
			},
			wantSeries: func() map[uint64]*prompb.TimeSeries {
				labels := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist"},
					{Name: "attr", Value: "test_attr"},
				}
				labelsAnother := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist"},
					{Name: "attr", Value: "test_attr_two"},
				}

				return map[uint64]*prompb.TimeSeries{
					timeSeriesSignature(labels): {
						Labels: labels,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 7},
								Schema:         1,
								ZeroThreshold:  defaultZeroThreshold,
								ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 0},
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
								PositiveDeltas: []int64{4, -2},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 1},
						},
					},
					timeSeriesSignature(labelsAnother): {
						Labels: labelsAnother,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 4},
								Schema:         1,
								ZeroThreshold:  defaultZeroThreshold,
								ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 0},
								NegativeSpans:  []prompb.BucketSpan{{Offset: 0, Length: 3}},
								NegativeDeltas: []int64{4, -2, -1},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 2},
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()

			converter := NewPrometheusConverter()
			annots, err := converter.addExponentialHistogramDataPoints(
				context.Background(),
				metric.ExponentialHistogram().DataPoints(),
				pcommon.NewResource(),
				Settings{
					ExportCreatedMetric: true,
				},
				otlptranslator.BuildCompliantMetricName(metric, "", true),
			)
			require.NoError(t, err)
			require.Empty(t, annots)

			require.Equal(t, tt.wantSeries(), converter.unique)
			require.Empty(t, converter.conflicts)
		})
	}
}

func TestConvertExplicitHistogramBucketsToNHCBLayout(t *testing.T) {
	tests := []struct {
		name       string
		buckets    []uint64
		wantLayout expectedBucketLayout
	}{
		{
			name:    "zero offset",
			buckets: []uint64{4, 3, 2, 1},
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 0,
						Length: 4,
					},
				},
				wantDeltas: []int64{4, -1, -1, -1},
			},
		},
		{
			name:    "leading empty buckets",
			buckets: []uint64{0, 0, 1, 1, 2, 3},
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 2,
						Length: 4,
					},
				},
				wantDeltas: []int64{1, 0, 1, 1},
			},
		},
		{
			name:    "trailing empty buckets",
			buckets: []uint64{0, 0, 1, 1, 2, 3, 0, 0}, // TODO: add tests for 3 trailing buckets
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 2,
						Length: 6,
					},
				},
				wantDeltas: []int64{1, 0, 1, 1, -3, 0},
			},
		},
		{
			name:    "bucket gap of 2",
			buckets: []uint64{1, 2, 0, 0, 2},
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 0,
						Length: 5,
					},
				},
				wantDeltas: []int64{1, 1, -2, 0, 2},
			},
		},
		{
			name:    "bucket gap > 2",
			buckets: []uint64{1, 2, 0, 0, 0, 2, 4, 4},
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 0,
						Length: 2,
					},
					{
						Offset: 3,
						Length: 3,
					},
				},
				wantDeltas: []int64{1, 1, 0, 2, 0},
			},
		},
		{
			name:    "multiple bucket gaps",
			buckets: []uint64{0, 0, 1, 2, 0, 0, 0, 2, 4, 4, 0, 0},
			wantLayout: expectedBucketLayout{
				wantSpans: []prompb.BucketSpan{
					{
						Offset: 2,
						Length: 2,
					},
					{
						Offset: 3,
						Length: 5,
					},
				},
				wantDeltas: []int64{1, 1, 0, 2, 0, -4, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buckets := tt.buckets
			offset := getBucketOffset(buckets)
			bucketCounts := buckets[offset:]

			gotSpans, gotDeltas := convertBucketsLayout(bucketCounts, int32(offset), 0, false)
			require.Equal(t, tt.wantLayout.wantSpans, gotSpans)
			require.Equal(t, tt.wantLayout.wantDeltas, gotDeltas)
		})
	}
}

func BenchmarkConvertHistogramBucketsToNHCBLayout(b *testing.B) {
	scenarios := []struct {
		gap int
	}{
		{gap: 0},
		{gap: 1},
		{gap: 2},
		{gap: 3},
	}

	for _, scenario := range scenarios {
		var buckets []uint64
		for i := 0; i < 1000; i++ {
			if i%(scenario.gap+1) == 0 {
				buckets = append(buckets, uint64(10))
			} else {
				buckets = append(buckets, uint64(0))
			}
		}
		b.Run(fmt.Sprintf("gap %d", scenario.gap), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				offset := getBucketOffset(buckets)
				convertBucketsLayout(buckets, int32(offset), 0, false)
			}
		})
	}
}

func TestHistogramToCustomBucketsHistogram(t *testing.T) {
	tests := []struct {
		name           string
		hist           func() pmetric.HistogramDataPoint
		wantNativeHist func() prompb.Histogram
		wantErrMessage string
	}{
		{
			name: "convert hist to custom buckets hist",
			hist: func() pmetric.HistogramDataPoint {
				pt := pmetric.NewHistogramDataPoint()
				pt.SetStartTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(100)))
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))
				pt.SetCount(2)
				pt.SetSum(10.1)

				pt.BucketCounts().FromRaw([]uint64{1, 1})
				pt.ExplicitBounds().FromRaw([]float64{0, 1})
				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 2},
					Sum:            10.1,
					Schema:         -53,
					PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDeltas: []int64{1, 0},
					CustomValues:   []float64{0, 1},
					Timestamp:      500,
				}
			},
		},
		{
			name: "convert hist to custom buckets hist with no sum",
			hist: func() pmetric.HistogramDataPoint {
				pt := pmetric.NewHistogramDataPoint()
				pt.SetStartTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(100)))
				pt.SetTimestamp(pcommon.NewTimestampFromTime(time.UnixMilli(500)))
				pt.SetCount(4)

				pt.BucketCounts().FromRaw([]uint64{2, 2})
				pt.ExplicitBounds().FromRaw([]float64{0, 1})
				return pt
			},
			wantNativeHist: func() prompb.Histogram {
				return prompb.Histogram{
					Count:          &prompb.Histogram_CountInt{CountInt: 4},
					Schema:         -53,
					PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDeltas: []int64{2, 0},
					CustomValues:   []float64{0, 1},
					Timestamp:      500,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validateHistogramCount(t, tt.hist())
			got, annots, err := explicitHistogramToCustomBucketsHistogram(tt.hist())
			if tt.wantErrMessage != "" {
				require.ErrorContains(t, err, tt.wantErrMessage)
				return
			}

			require.NoError(t, err)
			require.Empty(t, annots)
			require.Equal(t, tt.wantNativeHist(), got)
			validateNativeHistogramCount(t, got)
		})
	}
}

func TestPrometheusConverter_addCustomBucketsHistogramDataPoints(t *testing.T) {
	tests := []struct {
		name       string
		metric     func() pmetric.Metric
		wantSeries func() map[uint64]*prompb.TimeSeries
	}{
		{
			name: "histogram data points with same labels",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist_to_nhcb")
				metric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.Histogram().DataPoints().AppendEmpty()
				pt.SetCount(3)
				pt.SetSum(3)
				pt.BucketCounts().FromRaw([]uint64{2, 0, 1})
				pt.ExplicitBounds().FromRaw([]float64{5, 10})
				pt.Exemplars().AppendEmpty().SetDoubleValue(1)
				pt.Attributes().PutStr("attr", "test_attr")

				pt = metric.Histogram().DataPoints().AppendEmpty()
				pt.SetCount(11)
				pt.SetSum(5)
				pt.BucketCounts().FromRaw([]uint64{3, 8, 0})
				pt.ExplicitBounds().FromRaw([]float64{0, 1})
				pt.Exemplars().AppendEmpty().SetDoubleValue(2)
				pt.Attributes().PutStr("attr", "test_attr")

				return metric
			},
			wantSeries: func() map[uint64]*prompb.TimeSeries {
				labels := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist_to_nhcb"},
					{Name: "attr", Value: "test_attr"},
				}
				return map[uint64]*prompb.TimeSeries{
					timeSeriesSignature(labels): {
						Labels: labels,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 3},
								Sum:            3,
								Schema:         -53,
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 3}},
								PositiveDeltas: []int64{2, -2, 1},
								CustomValues:   []float64{5, 10},
							},
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 11},
								Sum:            5,
								Schema:         -53,
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 3}},
								PositiveDeltas: []int64{3, 5, -8},
								CustomValues:   []float64{0, 1},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 1},
							{Value: 2},
						},
					},
				}
			},
		},
		{
			name: "histogram data points with different labels",
			metric: func() pmetric.Metric {
				metric := pmetric.NewMetric()
				metric.SetName("test_hist_to_nhcb")
				metric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

				pt := metric.Histogram().DataPoints().AppendEmpty()
				pt.SetCount(6)
				pt.SetSum(3)
				pt.BucketCounts().FromRaw([]uint64{4, 2})
				pt.ExplicitBounds().FromRaw([]float64{0, 1})
				pt.Exemplars().AppendEmpty().SetDoubleValue(1)
				pt.Attributes().PutStr("attr", "test_attr")

				pt = metric.Histogram().DataPoints().AppendEmpty()
				pt.SetCount(11)
				pt.SetSum(5)
				pt.BucketCounts().FromRaw([]uint64{3, 8})
				pt.ExplicitBounds().FromRaw([]float64{0, 1})
				pt.Exemplars().AppendEmpty().SetDoubleValue(2)
				pt.Attributes().PutStr("attr", "test_attr_two")

				return metric
			},
			wantSeries: func() map[uint64]*prompb.TimeSeries {
				labels := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist_to_nhcb"},
					{Name: "attr", Value: "test_attr"},
				}
				labelsAnother := []prompb.Label{
					{Name: model.MetricNameLabel, Value: "test_hist_to_nhcb"},
					{Name: "attr", Value: "test_attr_two"},
				}

				return map[uint64]*prompb.TimeSeries{
					timeSeriesSignature(labels): {
						Labels: labels,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 6},
								Sum:            3,
								Schema:         -53,
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
								PositiveDeltas: []int64{4, -2},
								CustomValues:   []float64{0, 1},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 1},
						},
					},
					timeSeriesSignature(labelsAnother): {
						Labels: labelsAnother,
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 11},
								Sum:            5,
								Schema:         -53,
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
								PositiveDeltas: []int64{3, 5},
								CustomValues:   []float64{0, 1},
							},
						},
						Exemplars: []prompb.Exemplar{
							{Value: 2},
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := tt.metric()

			converter := NewPrometheusConverter()
			annots, err := converter.addCustomBucketsHistogramDataPoints(
				context.Background(),
				metric.Histogram().DataPoints(),
				pcommon.NewResource(),
				Settings{
					ExportCreatedMetric:     true,
					ConvertHistogramsToNHCB: true,
				},
				otlptranslator.BuildCompliantMetricName(metric, "", true),
			)

			require.NoError(t, err)
			require.Empty(t, annots)

			require.Equal(t, tt.wantSeries(), converter.unique)
			require.Empty(t, converter.conflicts)
		})
	}
}
