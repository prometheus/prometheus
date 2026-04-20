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

func TestSynthesizeNumber_ValidCounter(t *testing.T) {
	st := &stCache{}

	// Scrape loop anchors first sample
	v, ct, skip := st.synthesizeFloat(10.0, 1000)
	require.Equal(t, 10.0, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.Equal(t, 10.0, st.f.prev)
	require.Equal(t, 10.0, st.f.starting)

	// Second scrape, no reset
	v, ct, skip = st.synthesizeFloat(15.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, 15.0, st.f.prev)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Third scrape, no reset
	v, ct, skip = st.synthesizeFloat(20.0, 3000)
	require.Equal(t, 10.0, v)
	require.Equal(t, 20.0, st.f.prev)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)
}

func TestSynthesizeNumber_CounterReset(t *testing.T) {
	st := &stCache{}

	// Scrape loop anchors first sample
	v, ct, skip := st.synthesizeFloat(100.0, 1000)
	require.Equal(t, 100.0, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.Equal(t, 100.0, st.f.prev)
	require.Equal(t, 100.0, st.f.starting)

	// First reset (value goes down)
	v, ct, skip = st.synthesizeFloat(5.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, int64(1999), ct)
	require.False(t, skip)
	require.Equal(t, 5.0, st.f.prev)
	require.Equal(t, 0.0, st.f.starting)

	// Increment
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

	// Scrape loop anchors first sample
	v, ct, skip := st.synthesizeFloatHistogram(fh1, 1000)
	require.Equal(t, fh1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.NotNil(t, st.h)
	require.NotNil(t, st.h.prev)
	require.NotNil(t, st.h.starting)

	// Next scrape
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

	// Reset scrape (values go down)
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
	require.Equal(t, 5.0, st.h.starting.Count)
	require.Equal(t, 12.0, st.h.starting.Sum)
}

func TestSynthesizeHistogram_ValidAndReset(t *testing.T) {
	st := &stCache{}

	h1 := &histogram.Histogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
	}

	// Scrape loop anchors first sample
	v, ct, skip := st.synthesizeHistogram(h1, 1000)
	require.Equal(t, h1, v)
	require.Equal(t, int64(1000), ct)
	require.True(t, skip)
	require.NotNil(t, st.h)

	// Next scrape
	h2 := &histogram.Histogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 5,
	}
	v, ct, skip = st.synthesizeHistogram(h2, 2000)
	require.Equal(t, uint64(15), v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, uint64(3), v.ZeroCount)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Reset scrape (values go down)
	h3 := &histogram.Histogram{
		Count:     5,
		Sum:       12.0,
		ZeroCount: 1,
	}
	v, ct, skip = st.synthesizeHistogram(h3, 3000)
	require.Equal(t, uint64(5), v.Count)
	require.Equal(t, 12.0, v.Sum)
	require.Equal(t, uint64(1), v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
	require.Equal(t, 5.0, st.h.starting.Count)
	require.Equal(t, 12.0, st.h.starting.Sum)
}

func TestSynthesizeFloatHistogram_SubtractionMapping(t *testing.T) {
	st := &stCache{}

	// Create an anchor
	fh1 := &histogram.FloatHistogram{
		Count:     10,
		Sum:       50.5,
		ZeroCount: 2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2}, // Buckets at index 0, 1
		},
		PositiveBuckets: []float64{3.0, 5.0}, // total 8
	}

	// Scrape loop anchors first sample
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
		PositiveBuckets: []int64{4, 3, -1}, // Absolute: 4, 7, 6
	}

	// Expect absolute subtraction result:
	// fh1 buckets absolute: 3, 5
	// fh2 buckets absolute: 4, 7, 6
	// Result absolute: 1, 2, 6
	// Back to Delta -> [1, 1, 4]
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

	// Check if ref was updated
	require.Equal(t, 12.0, st.h.starting.Count)
	require.Equal(t, []float64{2.0, 8.0}, st.h.starting.PositiveBuckets)
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
	require.Equal(t, 12.0, st.h.starting.Count)
}
