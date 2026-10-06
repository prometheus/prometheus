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

// TestConvertNHCBToClassicHistogram_CacheMatchesNoCache verifies that a reused
// ClassicSeriesCache emits the exact same labels and values as the uncached
// path across repeated calls for the same series, across different suffixes,
// and when reused across series that share __name__ but differ in other labels.
func TestConvertNHCBToClassicHistogram_CacheMatchesNoCache(t *testing.T) {
	h := &Histogram{
		CustomValues:    []float64{1, 2, 3},
		PositiveBuckets: []int64{10, 20, 30},
		PositiveSpans:   []Span{{Offset: 0, Length: 3}},
		Count:           100,
		Sum:             100.0,
		Schema:          CustomBucketsSchema,
	}
	lsets := []labels.Labels{
		labels.FromStrings("__name__", "test_metric", "job", "job_a"),
		labels.FromStrings("__name__", "test_metric", "job", "job_a"), // cache hit
		labels.FromStrings("__name__", "test_metric", "job", "job_b"), // same __name__, different label
		labels.FromStrings("__name__", "other_metric", "job", "job_b"),
	}
	suffixes := []string{"", ClassicSuffixBucket, ClassicSuffixCount, ClassicSuffixSum}
	labelBuilder := labels.NewBuilder(labels.EmptyLabels())

	for _, suffix := range suffixes {
		cache := &ClassicSeriesCache{}
		for i, lset := range lsets {
			var without, with []sample
			require.NoError(t, ConvertNHCBToClassic(h, lset, labelBuilder, suffix, nil, func(lbls labels.Labels, val float64) error {
				without = append(without, sample{lset: lbls, val: val})
				return nil
			}))
			require.NoError(t, ConvertNHCBToClassic(h, lset, labelBuilder, suffix, cache, func(lbls labels.Labels, val float64) error {
				with = append(with, sample{lset: lbls, val: val})
				return nil
			}))
			require.Len(t, with, len(without))
			for j := range without {
				require.True(t, labels.Equal(without[j].lset, with[j].lset), "suffix %q step %d: labels mismatch at index %d: expected %v, got %v", suffix, i, j, without[j].lset, with[j].lset)
				require.Equal(t, without[j].val, with[j].val, "suffix %q step %d: value mismatch at index %d", suffix, i, j)
			}
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
	// Schema 0 buckets: (0.5,1]:10, (1,2]:20, (2,4]:30.
	posOnly := &FloatHistogram{
		Schema:          0,
		Count:           60,
		Sum:             100,
		PositiveSpans:   []Span{{Offset: 0, Length: 3}},
		PositiveBuckets: []float64{10, 20, 30},
	}
	// Zero bucket [-0.1,0.1]:5 plus (0.5,1]:10.
	withZero := &FloatHistogram{
		Schema:          0,
		Count:           15,
		Sum:             10,
		ZeroThreshold:   0.1,
		ZeroCount:       5,
		PositiveSpans:   []Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []float64{10},
	}
	// Negative bucket [-1,-0.5):7, zero bucket [-0.1,0.1]:5, (0.5,1]:10.
	withNegative := &FloatHistogram{
		Schema:          0,
		Count:           22,
		Sum:             1,
		ZeroThreshold:   0.1,
		ZeroCount:       5,
		NegativeSpans:   []Span{{Offset: 0, Length: 1}},
		NegativeBuckets: []float64{7},
		PositiveSpans:   []Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []float64{10},
	}
	// NaN observations are only reflected in Count.
	withNaN := &FloatHistogram{
		Schema:          0,
		Count:           12,
		Sum:             math.NaN(),
		PositiveSpans:   []Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []float64{10},
	}
	// Only the zero bucket [-0.1,0.1]:4 is populated, so neither bound is
	// replaced by 0 and the bucket interpolates linearly across the whole
	// width, as histogram_fraction does.
	zeroOnly := &FloatHistogram{
		Schema:        0,
		Count:         4,
		Sum:           0,
		ZeroThreshold: 0.1,
		ZeroCount:     4,
	}
	// Negative bucket [-1,-0.5):7 and zero bucket [-0.1,0.1]:4: with only
	// negative buckets 0 is the upper bound of the zero bucket, so le="0.0"
	// already includes all observations.
	negativeOnly := &FloatHistogram{
		Schema:          0,
		Count:           11,
		Sum:             -5,
		ZeroThreshold:   0.1,
		ZeroCount:       4,
		NegativeSpans:   []Span{{Offset: 0, Length: 1}},
		NegativeBuckets: []float64{7},
	}
	// Float histograms may carry a Count below the bucket sum (e.g. after
	// float rounding in recording rules); +Inf must stay monotonic.
	countBelowBuckets := &FloatHistogram{
		Schema:          0,
		Count:           9.5,
		Sum:             10,
		PositiveSpans:   []Span{{Offset: 0, Length: 2}},
		PositiveBuckets: []float64{5, 5},
	}
	lset := labels.FromStrings("__name__", "test_metric", "job", "a")
	bucket := func(le string, v float64) sample {
		return sample{lset: labels.FromStrings("__name__", "test_metric_bucket", "job", "a", "le", le), val: v}
	}
	// Expected interpolated fraction of bucket (lower, upper] below v, as
	// done by histogram_fraction for exponential buckets.
	expFrac := func(lower, upper, v float64) float64 {
		return Bucket[float64]{Lower: lower, Upper: upper}.FractionBelow(v, false)
	}

	for _, tc := range []struct {
		name     string
		h        *FloatHistogram
		bounds   []float64
		suffix   string
		expected []sample
	}{
		{
			name:   "bounds on exponential boundaries are exact",
			h:      posOnly,
			bounds: []float64{1, 2, 4},
			expected: []sample{
				bucket("1.0", 10), bucket("2.0", 30), bucket("4.0", 60), bucket("+Inf", 60),
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "a"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_sum", "job", "a"), val: 100},
			},
		},
		{
			name:   "bounds below, inside and above populated buckets",
			h:      posOnly,
			bounds: []float64{0.25, 0.5, 1.5, 3, 8},
			suffix: ClassicSuffixBucket,
			expected: []sample{
				bucket("0.25", 0),
				bucket("0.5", 0),
				bucket("1.5", 10+20*expFrac(1, 2, 1.5)),
				bucket("3.0", 30+30*expFrac(2, 4, 3)),
				bucket("8.0", 60),
				bucket("+Inf", 60),
			},
		},
		{
			name:     "no finite bounds only emits +Inf",
			h:        posOnly,
			bounds:   []float64{},
			suffix:   ClassicSuffixBucket,
			expected: []sample{bucket("+Inf", 60)},
		},
		{
			name:   "zero bucket of a positive-only histogram interpolates linearly from 0",
			h:      withZero,
			bounds: []float64{0.05, 0.1, 0.3, 1},
			suffix: ClassicSuffixBucket,
			expected: []sample{
				bucket("0.05", 2.5), bucket("0.1", 5), bucket("0.3", 5), bucket("1.0", 15), bucket("+Inf", 15),
			},
		},
		{
			name:   "negative buckets",
			h:      withNegative,
			bounds: []float64{-2, -0.75, -0.5, 0, 0.1, 1},
			suffix: ClassicSuffixBucket,
			expected: []sample{
				bucket("-2.0", 0),
				bucket("-0.75", 7*expFrac(-1, -0.5, -0.75)),
				bucket("-0.5", 7),
				bucket("0.0", 7+5*0.5),
				bucket("0.1", 12),
				bucket("1.0", 22),
				bucket("+Inf", 22),
			},
		},
		{
			name:   "NaN observations are counted in +Inf and _count only",
			h:      withNaN,
			bounds: []float64{1},
			expected: []sample{
				bucket("1.0", 10), bucket("+Inf", 12),
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "a"), val: 12},
				{lset: labels.FromStrings("__name__", "test_metric_sum", "job", "a"), val: math.NaN()},
			},
		},
		{
			name:   "zero bucket of a zero-only histogram interpolates linearly across its full width",
			h:      zeroOnly,
			bounds: []float64{-0.05, 0, 0.05, 0.1},
			suffix: ClassicSuffixBucket,
			expected: []sample{
				bucket("-0.05", 1), bucket("0.0", 2), bucket("0.05", 3), bucket("0.1", 4), bucket("+Inf", 4),
			},
		},
		{
			name:   "zero bucket of a negative-only histogram ends at 0",
			h:      negativeOnly,
			bounds: []float64{-0.1, -0.05, 0, 0.05},
			suffix: ClassicSuffixBucket,
			expected: []sample{
				bucket("-0.1", 7), bucket("-0.05", 9), bucket("0.0", 11), bucket("0.05", 11), bucket("+Inf", 11),
			},
		},
		{
			name:   "+Inf is at least the bucket sum when Count is lower",
			h:      countBelowBuckets,
			bounds: []float64{1, 2},
			expected: []sample{
				bucket("1.0", 5), bucket("2.0", 10), bucket("+Inf", 10),
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "a"), val: 9.5},
				{lset: labels.FromStrings("__name__", "test_metric_sum", "job", "a"), val: 10},
			},
		},
		{
			name:   "count only ignores bounds",
			h:      posOnly,
			bounds: nil,
			suffix: ClassicSuffixCount,
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "a"), val: 60},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.h.Validate())
			for _, useCache := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache=%v", useCache), func(t *testing.T) {
					var cache *ClassicSeriesCache
					if useCache {
						cache = &ClassicSeriesCache{}
					}
					lb := labels.NewBuilder(labels.EmptyLabels())
					// Convert twice to exercise cache reuse.
					for range 2 {
						var got []sample
						require.NoError(t, ConvertExponentialToClassic(tc.h, tc.bounds, lset, lb, tc.suffix, cache, func(l labels.Labels, v float64) error {
							got = append(got, sample{lset: l, val: v})
							return nil
						}))
						require.Len(t, got, len(tc.expected))
						for i := range tc.expected {
							require.True(t, labels.Equal(tc.expected[i].lset, got[i].lset), "labels mismatch at index %d: expected %v, got %v", i, tc.expected[i].lset, got[i].lset)
							if math.IsNaN(tc.expected[i].val) {
								require.True(t, math.IsNaN(got[i].val))
								continue
							}
							require.InDelta(t, tc.expected[i].val, got[i].val, 1e-9, "series %s", got[i].lset)
						}
					}
				})
			}
		})
	}

	t.Run("rejects NHCB", func(t *testing.T) {
		nhcb := &FloatHistogram{Schema: CustomBucketsSchema, CustomValues: []float64{1}, Count: 1, PositiveSpans: []Span{{Length: 1}}, PositiveBuckets: []float64{1}}
		err := ConvertExponentialToClassic(nhcb, []float64{1}, lset, labels.NewBuilder(labels.EmptyLabels()), "", nil, func(labels.Labels, float64) error { return nil })
		require.Error(t, err)
	})
}

func TestAppendExponentialBounds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		h         *FloatHistogram
		maxSchema int32
		expected  []float64
	}{
		{
			name: "schema at or below max emits lower and upper of populated buckets",
			h: &FloatHistogram{
				Schema:          0,
				PositiveSpans:   []Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 1}},
				PositiveBuckets: []float64{10, 0, 30},
			},
			maxSchema: 2,
			expected:  []float64{0.5, 1, 4, 8},
		},
		{
			name: "higher schema is reduced to max schema",
			h: &FloatHistogram{
				Schema:          3,
				PositiveSpans:   []Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []float64{15, 15},
			},
			maxSchema: 2,
			expected:  []float64{0.8408964152537144, 1, 1, 1.189207115002721},
		},
		{
			name: "zero and negative buckets",
			h: &FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.1,
				ZeroCount:       1,
				NegativeSpans:   []Span{{Offset: 1, Length: 1}},
				NegativeBuckets: []float64{7},
			},
			maxSchema: 2,
			expected:  []float64{-2, -1, 0.1, -0.1},
		},
		{
			name:      "empty histogram",
			h:         &FloatHistogram{Schema: 0},
			maxSchema: 2,
			expected:  nil,
		},
		{
			name:      "custom buckets schema is ignored",
			h:         &FloatHistogram{Schema: CustomBucketsSchema, CustomValues: []float64{1}, PositiveSpans: []Span{{Length: 1}}, PositiveBuckets: []float64{1}},
			maxSchema: 2,
			expected:  nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, AppendExponentialBounds(nil, tc.h, tc.maxSchema))
		})
	}
}
