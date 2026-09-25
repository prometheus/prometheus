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

package histogram

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

type sample struct {
	lset labels.Labels
	val  float64
}

func TestConvertNHCBToClassicHistogram(t *testing.T) {
	tests := []struct {
		name      string
		nhcb      any
		labels    labels.Labels
		expectErr bool
		expected  []sample
	}{
		{
			name: "valid histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  100,
				Sum:    100.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 40},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "valid floatHistogram",
			nhcb: &FloatHistogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{20.0, 40.0, 60.0}, // 20 -> 60 ->120
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  120.0,
				Sum:    100.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "empty histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{},
				PositiveBuckets: []int64{},
				PositiveSpans:   []Span{},
				Count:           0,
				Sum:             0.0,
				Schema:          CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 0},
			},
		},
		{
			name: "missing __name__ label",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels:    labels.FromStrings("job", "test_job"),
			expectErr: true,
		},
		{
			name:      "unsupported histogram type",
			nhcb:      nil,
			labels:    labels.FromStrings("__name__", "test_metric"),
			expectErr: true,
		},
		{
			name: "histogram with zero bucket counts",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{0, 10, 0},
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  20,
				Sum:    50.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 50},
			},
		},
		{
			name: "extra bucket counts than custom values",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10, 20, 30},
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 40},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "mismatched bucket lengths with less filled bucket count",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10},
				PositiveSpans:   []Span{{Offset: 0, Length: 2}},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels:    labels.FromStrings("__name__", "test_metric_bucket"),
			expectErr: true,
		},
		{
			name: "single series Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				PositiveSpans: []Span{
					{Offset: 0, Length: 1},
				},
				Count:  10,
				Sum:    20.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 20},
			},
		},
		{
			name: "multiset label histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				PositiveSpans: []Span{
					{Offset: 0, Length: 1},
				},
				Count:  10,
				Sum:    20.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric", "job", "test_job", "instance", "localhost:9090"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "job", "test_job", "instance", "localhost:9090", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "job", "test_job", "instance", "localhost:9090", "le", "+Inf"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "test_job", "instance", "localhost:9090"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_sum", "job", "test_job", "instance", "localhost:9090"), val: 20},
			},
		},
		{
			name: "exponential histogram",
			nhcb: &FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     5.5,
				Count:         3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			labels:    labels.FromStrings("__name__", "test_metric_bucket"),
			expectErr: true,
		},
		{
			name: "sparse histogram",
			nhcb: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				PositiveSpans: []Span{
					{0, 2},
					{4, 1},
					{1, 2},
				},
				PositiveBuckets: []int64{1, 2, 3, 4, 5}, // 1 -> 3 -> 0 -> 0 -> 0 -> 0 -> 6 -> 0 -> 10 -> 15
				Count:           35,                     // 1 -> 4 -> 4 -> 4 -> 4 -> 4 -> 10 -> 10 -> 20 -> 35
				Sum:             123,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 1},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "4.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "5.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "6.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "7.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "8.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "9.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "10.0"), val: 35},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 35},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 35},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 123},
			},
		},
		{
			name: "sparse float histogram",
			nhcb: &FloatHistogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				PositiveSpans: []Span{
					{0, 2},
					{4, 1},
					{1, 2},
				},
				PositiveBuckets: []float64{1, 2, 3, 4, 5}, // 1 -> 2 -> 0 -> 0 -> 0 -> 0 -> 3 -> 0 -> 4 -> 5
				Count:           15,                       // 1 -> 3 -> 3 -> 3 -> 3 -> 3 -> 6 -> 6 -> 10 -> 15
				Sum:             123,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 1},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "4.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "5.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "6.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "7.0"), val: 6},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "8.0"), val: 6},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "9.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "10.0"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 123},
			},
		},
	}

	labelBuilder := labels.NewBuilder(labels.EmptyLabels())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var emittedSamples []sample
			err := ConvertNHCBToClassic(tt.nhcb, tt.labels, labelBuilder, "", nil, func(lbls labels.Labels, val float64) error {
				emittedSamples = append(emittedSamples, sample{lset: lbls, val: val})
				return nil
			})
			require.Equal(t, tt.expectErr, err != nil, "unexpected error: %v", err)
			if !tt.expectErr {
				require.Len(t, emittedSamples, len(tt.expected))
				for i, expSample := range tt.expected {
					require.True(t, labels.Equal(expSample.lset, emittedSamples[i].lset), "labels mismatch at index %d: expected %v, got %v", i, expSample.lset, emittedSamples[i].lset)
					require.Equal(t, expSample.val, emittedSamples[i].val, "value mismatch at index %d", i)
				}
			}
		})
	}
}

// TestConvertNHCBToClassicHistogram_CacheMatchesNoCache re-runs every case
// above through a reused ClassicSeriesCache to prove the cached path emits
// byte-for-byte the same labels and values as the uncached path.
func TestConvertNHCBToClassicHistogram_CacheMatchesNoCache(t *testing.T) {
	h := &Histogram{
		CustomValues:    []float64{1, 2, 3},
		PositiveBuckets: []int64{10, 20, 30},
		PositiveSpans:   []Span{{Offset: 0, Length: 3}},
		Count:           100,
		Sum:             100.0,
		Schema:          CustomBucketsSchema,
	}
	lset := labels.FromStrings("__name__", "test_metric", "job", "test_job")
	labelBuilder := labels.NewBuilder(labels.EmptyLabels())

	var without []sample
	require.NoError(t, ConvertNHCBToClassic(h, lset, labelBuilder, "", nil, func(lbls labels.Labels, val float64) error {
		without = append(without, sample{lset: lbls, val: val})
		return nil
	}))

	cache := &ClassicSeriesCache{}
	for iteration := range 3 {
		var with []sample
		require.NoError(t, ConvertNHCBToClassic(h, lset, labelBuilder, "", cache, func(lbls labels.Labels, val float64) error {
			with = append(with, sample{lset: lbls, val: val})
			return nil
		}))
		require.Len(t, with, len(without))
		for i := range without {
			require.True(t, labels.Equal(without[i].lset, with[i].lset), "iteration %d: labels mismatch at index %d", iteration, i)
			require.Equal(t, without[i].val, with[i].val, "iteration %d: value mismatch at index %d", iteration, i)
		}
	}
}

// BenchmarkConvertNHCBToClassic simulates the real hot path: the same NHCB
// series converted once per scrape/timestamp over a query range. with_cache
// reuses one ClassicSeriesCache across iterations, as storage/nhcb_querier.go
// does per raw NHCB series; no_cache rebuilds names, le strings and label
// sets from scratch every call, as the code did before caching.
func BenchmarkConvertNHCBToClassic(b *testing.B) {
	const numBuckets = 30
	customValues := make([]float64, numBuckets)
	positiveBuckets := make([]int64, numBuckets)
	wantCount := int64(0)
	for i := range customValues {
		customValues[i] = float64(i+1) * 0.5
		positiveBuckets[i] = 1 // delta per bucket; absolute count in bucket i is i+1.
		wantCount += int64(i + 1)
	}
	h := &Histogram{
		CustomValues:    customValues,
		PositiveBuckets: positiveBuckets,
		PositiveSpans:   []Span{{Offset: 0, Length: uint32(numBuckets)}},
		Count:           uint64(wantCount),
		Sum:             123.45,
		Schema:          CustomBucketsSchema,
	}
	lset := labels.FromStrings("__name__", "bench_request_duration_seconds", "job", "bench", "instance", "localhost:9090")
	noop := func(labels.Labels, float64) error { return nil }

	b.Run("no_cache", func(b *testing.B) {
		lsetBuilder := labels.NewBuilder(labels.EmptyLabels())
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := ConvertNHCBToClassic(h, lset, lsetBuilder, "", nil, noop); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("with_cache", func(b *testing.B) {
		lsetBuilder := labels.NewBuilder(labels.EmptyLabels())
		cache := &ClassicSeriesCache{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := ConvertNHCBToClassic(h, lset, lsetBuilder, "", cache, noop); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestConvertNHCBToClassicIntFloatAgreement checks that an integer NHCB and its
// exact FloatHistogram representation convert to the same classic series, and
// that the +Inf bucket matches the count.
func TestConvertNHCBToClassicIntFloatAgreement(t *testing.T) {
	h := &Histogram{
		Schema:       CustomBucketsSchema,
		CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		PositiveSpans: []Span{
			{Offset: 0, Length: 2},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4, 5},
		Count:           35,
		Sum:             123,
	}
	require.NoError(t, h.Validate())

	convert := func(nhcb any) []sample {
		var got []sample
		lb := labels.NewBuilder(labels.EmptyLabels())
		require.NoError(t, ConvertNHCBToClassic(nhcb, labels.FromStrings("__name__", "test_metric"), lb, "", nil,
			func(l labels.Labels, v float64) error {
				got = append(got, sample{lset: l, val: v})
				return nil
			}))
		return got
	}

	fromInt := convert(h)
	fromFloat := convert(h.ToFloat(nil))

	require.Len(t, fromInt, len(fromFloat))
	for i := range fromInt {
		require.True(t, labels.Equal(fromInt[i].lset, fromFloat[i].lset), "labels mismatch at index %d", i)
		require.Equal(t, fromFloat[i].val, fromInt[i].val, "value mismatch at index %d for %s", i, fromInt[i].lset)
	}

	// In a classic histogram the +Inf bucket holds every observation.
	var infBucket, count float64
	for _, s := range fromInt {
		switch s.lset.Get(model.MetricNameLabel) {
		case "test_metric_bucket":
			if s.lset.Get(model.BucketLabel) == "+Inf" {
				infBucket = s.val
			}
		case "test_metric_count":
			count = s.val
		}
	}
	require.Equal(t, count, infBucket)
}

func TestConvertExponentialToClassic(t *testing.T) {
	bucket := func(le string, val float64) sample {
		return sample{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_bucket", model.BucketLabel, le), val: val}
	}
	count := func(val float64) sample {
		return sample{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_count"), val: val}
	}
	sum := func(val float64) sample {
		return sample{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_sum"), val: val}
	}

	// Cases converting the same series share a cache, which checks that the
	// cache stays correct across changing bucket layouts.
	caches := map[string]*ClassicSeriesCache{}
	for _, tc := range []struct {
		name string
		h    any
		// boundaries default to the ones of h, see AppendClassicBoundaries.
		boundaries []float64
		lset       labels.Labels // Defaults to {__name__="test_metric"}.
		expectErr  bool
		expected   []sample
	}{
		{
			// The lower boundary of the lowest bucket is emitted, too, as
			// histogram_quantile() would otherwise assume it to be 0.
			name: "positive buckets",
			h: &Histogram{
				Schema:          0,
				Count:           4,
				Sum:             6,
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1}, // 1, 2 and 1 in (0.5,1], (1,2] and (2,4].
			},
			expected: []sample{
				bucket("0.5", 0), bucket("1.0", 1), bucket("2.0", 3), bucket("4.0", 4), bucket("+Inf", 4),
				count(4), sum(6),
			},
		},
		{
			// The lower boundary of the first bucket after a gap is emitted,
			// too, as histogram_quantile() would otherwise assume observations
			// within the gap.
			name: "gap between buckets",
			h: &FloatHistogram{
				Schema:          0,
				Count:           4,
				Sum:             20,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}, {Offset: 2, Length: 1}},
				PositiveBuckets: []float64{1, 3}, // (0.5,1] and (4,8].
			},
			expected: []sample{
				bucket("0.5", 0), bucket("1.0", 1), bucket("4.0", 1), bucket("8.0", 4), bucket("+Inf", 4),
				count(4), sum(20),
			},
		},
		{
			name: "negative, zero and positive buckets",
			h: &FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.25,
				ZeroCount:       1,
				Count:           5,
				Sum:             0,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{2}, // (0.5,1].
				NegativeSpans:   []Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []float64{2}, // [-1,-0.5).
			},
			expected: []sample{
				bucket("-1.0", 0), bucket("-0.5", 2), bucket("-0.25", 2), bucket("0.25", 3), bucket("0.5", 3), bucket("1.0", 5), bucket("+Inf", 5),
				count(5), sum(0),
			},
		},
		{
			// Both classic and native histogram_quantile() assume 0 as the
			// lower boundary of a zero bucket without negative buckets.
			name: "zero bucket as the lowest bucket",
			h: &FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.25,
				ZeroCount:       1,
				Count:           3,
				Sum:             2,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{2},
			},
			expected: []sample{
				bucket("0.25", 1), bucket("0.5", 1), bucket("1.0", 3), bucket("+Inf", 3),
				count(3), sum(2),
			},
		},
		{
			// The part of (0.5,1] covered by the zero bucket counts towards
			// the zero bucket.
			name: "zero bucket overlapping the lowest positive bucket",
			h: &FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.75,
				ZeroCount:       1,
				Count:           3,
				Sum:             2,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{2},
			},
			expected: []sample{
				bucket("0.75", 1), bucket("1.0", 3), bucket("+Inf", 3),
				count(3), sum(2),
			},
		},
		{
			name: "zero bucket with a zero threshold of 0",
			h: &FloatHistogram{
				Schema:    0,
				ZeroCount: 1,
				Count:     1,
			},
			expected: []sample{
				bucket("0.0", 1), bucket("+Inf", 1),
				count(1), sum(0),
			},
		},
		{
			// Only the bucket for observations of +Inf has an infinite upper
			// boundary. It must not result in a second +Inf bucket.
			name: "observations of +Inf",
			h: &FloatHistogram{
				Schema:          0,
				Count:           1,
				Sum:             math.Inf(1),
				PositiveSpans:   []Span{{Offset: 1025, Length: 1}},
				PositiveBuckets: []float64{1}, // (math.MaxFloat64,+Inf].
			},
			expected: []sample{
				bucket("1.7976931348623157e+308", 0), bucket("+Inf", 1),
				count(1), sum(math.Inf(1)),
			},
		},
		{
			name: "no observations",
			h:    &Histogram{Schema: 3},
			expected: []sample{
				bucket("+Inf", 0),
				count(0), sum(0),
			},
		},
		{
			name: "labels are preserved",
			h: &Histogram{
				Schema:          0,
				Count:           1,
				Sum:             1,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
			},
			lset: labels.FromStrings(model.MetricNameLabel, "test_metric", "job", "test_job"),
			expected: []sample{
				{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_bucket", "job", "test_job", model.BucketLabel, "0.5"), val: 0},
				{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_bucket", "job", "test_job", model.BucketLabel, "1.0"), val: 1},
				{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_bucket", "job", "test_job", model.BucketLabel, "+Inf"), val: 1},
				{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_count", "job", "test_job"), val: 1},
				{lset: labels.FromStrings(model.MetricNameLabel, "test_metric_sum", "job", "test_job"), val: 1},
			},
		},
		{
			// Every bucket boundary of a lower schema is a bucket boundary
			// of the higher schema, too.
			name: "boundaries of a lower schema",
			h: &Histogram{
				Schema:          0,
				Count:           4,
				Sum:             6,
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1}, // 1, 2 and 1 in (0.5,1], (1,2] and (2,4].
			},
			boundaries: []float64{0.25, 1, 4}, // Schema -1.
			expected: []sample{
				bucket("0.25", 0), bucket("1.0", 1), bucket("4.0", 4), bucket("+Inf", 4),
				count(4), sum(6),
			},
		},
		{
			name: "boundaries beyond the buckets",
			h: &Histogram{
				Schema:          0,
				Count:           4,
				Sum:             6,
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1},
			},
			boundaries: []float64{0.125, 0.5, 1, 2, 4, 8},
			expected: []sample{
				bucket("0.125", 0), bucket("0.5", 0), bucket("1.0", 1), bucket("2.0", 3), bucket("4.0", 4), bucket("8.0", 4), bucket("+Inf", 4),
				count(4), sum(6),
			},
		},
		{
			// The observations of a bucket only count towards a boundary
			// that is at least the upper boundary of the bucket.
			name: "boundary within a bucket",
			h: &Histogram{
				Schema:          0,
				Count:           4,
				Sum:             6,
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1},
			},
			boundaries: []float64{1.5},
			expected: []sample{
				bucket("1.5", 1), bucket("+Inf", 4),
				count(4), sum(6),
			},
		},
		{
			name: "negative boundaries and the zero bucket",
			h: &FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.25,
				ZeroCount:       1,
				Count:           5,
				Sum:             0,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{2}, // (0.5,1].
				NegativeSpans:   []Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []float64{2}, // [-1,-0.5).
			},
			boundaries: []float64{-2, -0.5, 0, 0.5, 2},
			expected: []sample{
				bucket("-2.0", 0), bucket("-0.5", 2), bucket("0.0", 2), bucket("0.5", 3), bucket("2.0", 5), bucket("+Inf", 5),
				count(5), sum(0),
			},
		},
		{
			name: "no boundaries",
			h: &Histogram{
				Schema:          0,
				Count:           4,
				Sum:             6,
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1},
			},
			boundaries: []float64{},
			expected: []sample{
				bucket("+Inf", 4),
				count(4), sum(6),
			},
		},
		{
			name:       "unsorted boundaries",
			h:          &Histogram{Schema: 0},
			boundaries: []float64{1, 0.5},
			expectErr:  true,
		},
		{
			name:       "duplicate boundaries",
			h:          &Histogram{Schema: 0},
			boundaries: []float64{1, 1},
			expectErr:  true,
		},
		{
			name:       "infinite boundary",
			h:          &Histogram{Schema: 0},
			boundaries: []float64{1, math.Inf(1)},
			expectErr:  true,
		},
		{
			name:       "NaN boundary",
			h:          &Histogram{Schema: 0},
			boundaries: []float64{math.NaN()},
			expectErr:  true,
		},
		{
			name: "custom buckets",
			h: &Histogram{
				Schema:          CustomBucketsSchema,
				Count:           1,
				CustomValues:    []float64{1},
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
			},
			expectErr: true,
		},
		{
			name: "invalid histogram",
			h: &FloatHistogram{
				Schema:          0,
				Count:           1,
				PositiveSpans:   []Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []float64{1},
			},
			expectErr: true,
		},
		{
			name:      "missing __name__ label",
			h:         &Histogram{Schema: 0},
			lset:      labels.FromStrings("job", "test_job"),
			expectErr: true,
		},
		{
			name:      "unsupported histogram type",
			h:         nil,
			expectErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lset := tc.lset
			if lset.IsEmpty() {
				lset = labels.FromStrings(model.MetricNameLabel, "test_metric")
			}
			cache, ok := caches[lset.String()]
			if !ok {
				cache = &ClassicSeriesCache{}
				caches[lset.String()] = cache
			}
			boundaries := tc.boundaries
			convert := func(h any, onlySuffix string, cache *ClassicSeriesCache) ([]string, error) {
				var got []string
				err := ConvertExponentialToClassic(h, boundaries, lset, labels.NewBuilder(labels.EmptyLabels()), onlySuffix, cache, func(l labels.Labels, v float64) error {
					got = append(got, fmt.Sprintf("%s %v", l, v))
					return nil
				})
				return got, err
			}

			if tc.expectErr {
				_, err := convert(tc.h, "", nil)
				require.Error(t, err)
				return
			}

			histograms := []any{tc.h}
			fh, ok := tc.h.(*FloatHistogram)
			if h, isInt := tc.h.(*Histogram); isInt {
				fh, ok = h.ToFloat(nil), true
				histograms = append(histograms, fh)
			}
			require.True(t, ok)
			if boundaries == nil {
				boundaries = AppendClassicBoundaries(nil, fh)
			}
			for _, h := range histograms {
				for _, suffix := range []string{"", ClassicSuffixBucket, ClassicSuffixCount, ClassicSuffixSum} {
					var expected []string
					for _, s := range tc.expected {
						if strings.HasSuffix(s.lset.Get(model.MetricNameLabel), suffix) {
							expected = append(expected, fmt.Sprintf("%s %v", s.lset, s.val))
						}
					}
					for _, c := range []*ClassicSeriesCache{nil, cache} {
						got, err := convert(h, suffix, c)
						require.NoError(t, err)
						require.Equal(t, expected, got, "%T, suffix %q, cached %v", h, suffix, c != nil)
					}
				}
			}
		})
	}
}
