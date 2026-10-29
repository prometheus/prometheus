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

package scrape

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestSynthesizeFloat_ValidCounter(t *testing.T) {
	st := &stCache{}

	// Scrape loop anchors first sample.
	v, ct, skip := st.synthesizeFloat(10.0, 1000)
	require.Equal(t, 10.0, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.Equal(t, 10.0, st.f.prev)
	require.Equal(t, 10.0, st.f.starting)

	// Second scrape, no reset.
	v, ct, skip = st.synthesizeFloat(15.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, 15.0, st.f.prev)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Third scrape, no reset.
	v, ct, skip = st.synthesizeFloat(20.0, 3000)
	require.Equal(t, 10.0, v)
	require.Equal(t, 20.0, st.f.prev)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)
}

func TestSynthesizeFloat_CounterReset(t *testing.T) {
	st := &stCache{}

	// Scrape loop anchors first sample.
	v, ct, skip := st.synthesizeFloat(100.0, 1000)
	require.Equal(t, 100.0, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.Equal(t, 100.0, st.f.prev)
	require.Equal(t, 100.0, st.f.starting)

	// First reset (value goes down).
	v, ct, skip = st.synthesizeFloat(5.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, int64(1999), ct)
	require.False(t, skip)
	require.Equal(t, 5.0, st.f.prev)
	require.Equal(t, 0.0, st.f.starting)

	// Increment.
	v, ct, skip = st.synthesizeFloat(15.0, 3000)
	require.Equal(t, 15.0, v)
	require.Equal(t, int64(1999), ct)
	require.False(t, skip)
}

func TestSynthesizeFloatHistogram_ValidAndReset(t *testing.T) {
	st := &stCache{}

	fh1 := &histogram.FloatHistogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
	}

	// Scrape loop anchors first sample.
	v, ct, skip := st.synthesizeFloatHistogram(fh1, 1000)
	require.Equal(t, fh1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.NotNil(t, st.h)
	require.NotNil(t, st.h.prev)
	require.NotNil(t, st.h.starting)

	// Next scrape.
	fh2 := &histogram.FloatHistogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 5,
	}
	v, ct, skip = st.synthesizeFloatHistogram(fh2, 2000)
	require.Equal(t, 15.0, v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, 3.0, v.ZeroCount)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Reset scrape (values go down).
	fh3 := &histogram.FloatHistogram{
		Count:     5,
		Sum:       12.0,
		ZeroCount: 1,
	}
	v, ct, skip = st.synthesizeFloatHistogram(fh3, 3000)
	require.Equal(t, 5.0, v.Count)
	require.Equal(t, 12.0, v.Sum)
	require.Equal(t, 1.0, v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
	require.Nil(t, st.h.starting)

	// Next scrape.
	fh4 := &histogram.FloatHistogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 5,
	}
	v, ct, skip = st.synthesizeFloatHistogram(fh4, 4000)
	require.Equal(t, 25.0, v.Count)
	require.Equal(t, 120.0, v.Sum)
	require.Equal(t, 5.0, v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
}

func TestSynthesizeHistogram_ValidAndReset(t *testing.T) {
	st := &stCache{}

	h1 := &histogram.Histogram{
		Count:           10,
		Sum:             50.5,
		ZeroCount:       2,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{8},
	}

	// Scrape loop anchors first sample.
	v, ct, skip := st.synthesizeHistogram(h1, 1000)
	require.Equal(t, h1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.NotNil(t, st.h)

	// Next scrape.
	h2 := &histogram.Histogram{
		Count:           25,
		Sum:             120.0,
		ZeroCount:       5,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{20},
	}
	v, ct, skip = st.synthesizeHistogram(h2, 2000)
	require.Equal(t, uint64(15), v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, uint64(3), v.ZeroCount)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Reset scrape (values go down).
	h3 := &histogram.Histogram{
		Count:           5,
		Sum:             12.0,
		ZeroCount:       1,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{4},
	}
	v, ct, skip = st.synthesizeHistogram(h3, 3000)
	require.Equal(t, uint64(5), v.Count)
	require.Equal(t, 12.0, v.Sum)
	require.Equal(t, uint64(1), v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
	require.Nil(t, st.h.starting)

	// Next scrape.
	h4 := &histogram.Histogram{
		Count:           25,
		Sum:             120.0,
		ZeroCount:       5,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{20},
	}
	v, ct, skip = st.synthesizeHistogram(h4, 4000)
	require.Equal(t, uint64(25), v.Count)
	require.Equal(t, 120.0, v.Sum)
	require.Equal(t, uint64(5), v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
}

func TestSynthesizeFloatHistogram_SubtractionMapping(t *testing.T) {
	st := &stCache{}

	// Create an anchor.
	fh1 := &histogram.FloatHistogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2}, // Buckets at index 0, 1
		},
		PositiveBuckets: []float64{3.0, 5.0}, // total 8
	}

	// Scrape loop anchors first sample.
	v, ct, skip := st.synthesizeFloatHistogram(fh1, 1000)
	require.Equal(t, fh1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)

	// Next scrape has a NEW span layout because a previous 0 bucket got populated
	// We expect NO reset, just proper subtraction.
	fh2 := &histogram.FloatHistogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3}, // Buckets at index 0, 1, 2
		},
		PositiveBuckets: []float64{4.0, 7.0, 6.0}, // The previous 0 bucket is now index 2 with 6.0 counts!
	}

	// Expect subtraction:
	// fh1 buckets [0, 1]: 3.0, 5.0
	// fh2 buckets [0, 1, 2]: 4.0, 7.0, 6.0
	// Result: [4-3, 7-5, 6-0] -> [1.0, 2.0, 6.0]
	// Count: 25 - 10 = 15
	// Sum: 120 - 50.5 = 69.5
	// ZeroCount: 3 - 2 = 1.0
	v, ct, skip = st.synthesizeFloatHistogram(fh2, 2000)

	require.False(t, skip)
	require.Equal(t, int64(1000), ct)
	require.Equal(t, 15.0, v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, 1.0, v.ZeroCount)
	require.Len(t, v.PositiveSpans, 1)
	require.Equal(t, uint32(3), v.PositiveSpans[0].Length)
	require.Equal(t, []float64{1.0, 2.0, 6.0}, v.PositiveBuckets)
}

func TestSynthesizeHistogram_SubtractionMapping(t *testing.T) {
	st := &stCache{}

	// Create an anchor
	h1 := &histogram.Histogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2}, // Buckets at index 0, 1
		},
		PositiveBuckets: []int64{3, 2}, // Absolute: 3, 5
	}

	// Scrape loop anchors first sample
	v, ct, skip := st.synthesizeHistogram(h1, 1000)
	require.Equal(t, h1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)

	// Next scrape has a NEW span layout because a previous 0 bucket got populated
	h2 := &histogram.Histogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3}, // Buckets at index 0, 1, 2
		},
		PositiveBuckets: []int64{4, 3, 4}, // Absolute: 4, 7, 11
	}

	// Expect absolute subtraction result:
	// fh1 buckets absolute: 3, 5
	// fh2 buckets absolute: 4, 7, 11
	// Result absolute: 1, 2, 11
	// Back to Delta -> [1, 1, 9]
	v, ct, skip = st.synthesizeHistogram(h2, 2000)

	require.False(t, skip)
	require.Equal(t, int64(1000), ct)
	require.Equal(t, uint64(15), v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, uint64(1), v.ZeroCount)
	require.Len(t, v.PositiveSpans, 1)
}

func TestSynthesizeFloatHistogram_BucketReset(t *testing.T) {
	st := &stCache{}

	fh1 := &histogram.FloatHistogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2}, // Buckets at index 0, 1
		},
		PositiveBuckets: []float64{3.0, 5.0}, // total 8
	}

	v, ct, skip := st.synthesizeFloatHistogram(fh1, 1000)
	_ = v
	_ = ct
	require.True(t, skip)

	// Next scrape: total count up to 12. Sum up to 60.0. Zero count stable.
	// BUT Bucket 0 goes from 3.0 down to 2.0. Bucket 1 goes from 5.0 to 8.0.
	fh2 := &histogram.FloatHistogram{
		Count:     12,
		Sum:       60.0,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []float64{2.0, 8.0},
	}

	v, ct, skip = st.synthesizeFloatHistogram(fh2, 2000)

	require.False(t, skip)
	// Since it resets, ct should be 1999.
	require.Equal(t, int64(1999), ct)
	// v should equal fh2 exactly since it resets
	require.Equal(t, 12.0, v.Count)
	require.Equal(t, 60.0, v.Sum)
	require.Equal(t, 2.0, v.ZeroCount)
	require.Equal(t, []float64{2.0, 8.0}, v.PositiveBuckets)
	require.Nil(t, st.h.starting)
}

func TestSynthesizeHistogram_BucketReset(t *testing.T) {
	st := &stCache{}

	h1 := &histogram.Histogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2}, // Buckets at index 0, 1
		},
		PositiveBuckets: []int64{3, 2}, // Absolute: 3, 5
	}

	v, ct, skip := st.synthesizeHistogram(h1, 1000)
	_ = v
	_ = ct
	require.True(t, skip)

	h2 := &histogram.Histogram{
		Count:     12,
		Sum:       60.0,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{2, 6}, // Absolute: 2, 8. Bucket 0 went from 3 to 2 -> Reset!
	}

	v, ct, skip = st.synthesizeHistogram(h2, 2000)

	require.False(t, skip)
	require.Equal(t, int64(1999), ct)
	require.Equal(t, uint64(12), v.Count)
	require.Equal(t, 60.0, v.Sum)
	require.Equal(t, []int64{2, 6}, v.PositiveBuckets)
	require.Nil(t, st.h.starting)
}

func TestSynthesizeHistogram_InvalidSample(t *testing.T) {
	// The second span has a negative offset.
	invalidFH := &histogram.FloatHistogram{
		Count:           5,
		Sum:             10,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}, {Offset: -2, Length: 1}},
		PositiveBuckets: []float64{2, 3},
	}
	invalidH := &histogram.Histogram{
		Count:           5,
		Sum:             10,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}, {Offset: -2, Length: 1}},
		PositiveBuckets: []int64{2, 1},
	}
	validFH := func(count float64) *histogram.FloatHistogram {
		return &histogram.FloatHistogram{
			Count:           count,
			Sum:             count,
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []float64{count},
		}
	}
	validH := func(count uint64) *histogram.Histogram {
		return &histogram.Histogram{
			Count:           count,
			Sum:             float64(count),
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []int64{int64(count)},
		}
	}

	t.Run("float histogram: invalid first sample does not become the anchor", func(t *testing.T) {
		st := &stCache{}
		// The invalid sample is returned unchanged, to be rejected when it is appended.
		v, ct, skip := st.synthesizeFloatHistogram(invalidFH.Copy(), 1000)
		require.Equal(t, invalidFH, v)
		require.Equal(t, int64(0), ct)
		require.False(t, skip)

		// The next valid sample becomes the anchor.
		_, ct, skip = st.synthesizeFloatHistogram(validFH(5), 2000)
		require.Equal(t, int64(2000), ct)
		require.True(t, skip)
	})

	t.Run("float histogram: invalid later sample leaves the anchor in place", func(t *testing.T) {
		st := &stCache{}
		_, _, skip := st.synthesizeFloatHistogram(validFH(5), 1000)
		require.True(t, skip)

		v, ct, skip := st.synthesizeFloatHistogram(invalidFH.Copy(), 2000)
		require.Equal(t, invalidFH, v)
		require.Equal(t, int64(1000), ct)
		require.False(t, skip)

		// The next valid sample is adjusted against the original anchor.
		v, ct, skip = st.synthesizeFloatHistogram(validFH(8), 3000)
		require.Equal(t, 3.0, v.Count)
		require.Equal(t, int64(1000), ct)
		require.False(t, skip)
	})

	t.Run("integer histogram: invalid first sample does not become the anchor", func(t *testing.T) {
		st := &stCache{}
		v, ct, skip := st.synthesizeHistogram(invalidH.Copy(), 1000)
		require.Equal(t, invalidH, v)
		require.Equal(t, int64(0), ct)
		require.False(t, skip)

		_, ct, skip = st.synthesizeHistogram(validH(5), 2000)
		require.Equal(t, int64(2000), ct)
		require.True(t, skip)
	})

	t.Run("integer histogram: invalid later sample leaves the anchor in place", func(t *testing.T) {
		st := &stCache{}
		_, _, skip := st.synthesizeHistogram(validH(5), 1000)
		require.True(t, skip)

		v, ct, skip := st.synthesizeHistogram(invalidH.Copy(), 2000)
		require.Equal(t, invalidH, v)
		require.Equal(t, int64(1000), ct)
		require.False(t, skip)

		v, ct, skip = st.synthesizeHistogram(validH(8), 3000)
		require.Equal(t, uint64(3), v.Count)
		require.Equal(t, int64(1000), ct)
		require.False(t, skip)
	})
}
