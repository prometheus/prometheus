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

package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type floatHistogramSTSample struct {
	st, t int64
	fh    *histogram.FloatHistogram
}

func BenchmarkFloatHistogramSTWrite(b *testing.B) {
	const n = 120
	fhs := tsdbutil.GenerateTestFloatHistograms(n)

	b.ReportAllocs()

	for b.Loop() {
		c := NewFloatHistogramSTChunk()
		app, _ := c.Appender()
		for i, fh := range fhs {
			_, _, app, _ = app.AppendFloatHistogram(nil, 500, int64(i)*15000, fh, false)
		}
	}
}

func BenchmarkFloatHistogramSTRead(b *testing.B) {
	const n = 120
	fhs := tsdbutil.GenerateTestFloatHistograms(n)

	c := NewFloatHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(b, err)
	for i, fh := range fhs {
		_, _, app, err = app.AppendFloatHistogram(nil, 500, int64(i)*15000, fh, false)
		require.NoError(b, err)
	}

	b.ReportAllocs()

	var it Iterator
	for b.Loop() {
		it = c.Iterator(it)
		for it.Next() != ValNone {
		}
	}
}

// requireFloatHistogramSTSamples appends the given float histogram samples to a
// new FloatHistogramSTChunk, then verifies all samples round-trip correctly
// through the iterator.
func requireFloatHistogramSTSamples(t *testing.T, samples []floatHistogramSTSample) {
	t.Helper()

	c := NewFloatHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	for _, s := range samples {
		_, _, app, err = app.AppendFloatHistogram(nil, s.st, s.t, s.fh, false)
		require.NoError(t, err)
	}

	require.Equal(t, len(samples), c.NumSamples())

	it := c.Iterator(nil)
	for i, s := range samples {
		require.Equal(t, ValFloatHistogram, it.Next(), "sample %d", i)
		require.Equal(t, s.t, it.AtT(), "sample %d: timestamp", i)
		require.Equal(t, s.st, it.AtST(), "sample %d: start timestamp", i)
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}

func TestFloatHistogramSTChunkST(t *testing.T) {
	testChunkSTHandling(t, ValFloatHistogram, func() Chunk { return NewFloatHistogramSTChunk() })
}

func TestFloatHistogramSTBasic(t *testing.T) {
	hs := tsdbutil.GenerateTestFloatHistograms(5)
	requireFloatHistogramSTSamples(t, []floatHistogramSTSample{
		{st: 0, t: 1000, fh: hs[0]},
		{st: 0, t: 2000, fh: hs[1]},
		{st: 0, t: 3000, fh: hs[2]},
		{st: 0, t: 4000, fh: hs[3]},
		{st: 0, t: 5000, fh: hs[4]},
	})
}

func TestFloatHistogramSTChunkAppendAndIterate(t *testing.T) {
	fhs := tsdbutil.GenerateTestFloatHistograms(5)
	requireFloatHistogramSTSamples(t, []floatHistogramSTSample{
		{st: 100, t: 1000, fh: fhs[0]},
		{st: 100, t: 2000, fh: fhs[1]},
		{st: 200, t: 3000, fh: fhs[2]},
		{st: 200, t: 4000, fh: fhs[3]},
		{st: 300, t: 5000, fh: fhs[4]},
	})
}

func TestFloatHistogramSTChunkCounterReset(t *testing.T) {
	c := NewFloatHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	fh1 := &histogram.FloatHistogram{
		Count:         10,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-125,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []float64{6, 3},
	}

	_, _, app, err = app.AppendFloatHistogram(nil, 100, 1000, fh1, false)
	require.NoError(t, err)

	fh2 := fh1.Copy()
	fh2.Count = 3
	fh2.Sum = 5.0
	fh2.PositiveBuckets = []float64{2, 1}
	fh2.CounterResetHint = histogram.CounterReset

	// Ensure counter reset produces new chunk.
	newChunk, recoded, newApp, err := app.AppendFloatHistogram(nil, 200, 2000, fh2, false)
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.False(t, recoded)

	stChunk, ok := newChunk.(*FloatHistogramSTChunk)
	require.True(t, ok)
	require.Equal(t, CounterReset, stChunk.GetCounterResetHeader())
	require.Equal(t, 1, stChunk.NumSamples())

	// Verify ST is preserved in the new chunk.
	it := stChunk.Iterator(nil)
	require.Equal(t, ValFloatHistogram, it.Next())
	require.Equal(t, int64(2000), it.AtT())
	require.Equal(t, int64(200), it.AtST())
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	fh3 := fh2.Copy()
	fh3.CounterResetHint = histogram.UnknownCounterReset
	fh3.Count = 8
	fh3.Sum = 10.0
	fh3.PositiveBuckets = []float64{4, 2}
	_, _, _, err = newApp.AppendFloatHistogram(nil, 300, 3000, fh3, false)
	require.NoError(t, err)
	require.Equal(t, 2, stChunk.NumSamples())
}

func TestFloatHistogramSTChunkRecode(t *testing.T) {
	c := NewFloatHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	h1 := &histogram.Histogram{
		Count:         27,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-125,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		},
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4},
		NegativeSpans:   []histogram.Span{{Offset: 1, Length: 1}},
		NegativeBuckets: []int64{1},
	}
	fh1 := h1.ToFloat(nil)

	_, _, app, err = app.AppendFloatHistogram(nil, 100, 1000, fh1, false)
	require.NoError(t, err)

	// Force recode with expanded bucket layout
	h2 := h1.Copy()
	h2.PositiveSpans = []histogram.Span{
		{Offset: 0, Length: 3},
		{Offset: 1, Length: 1},
		{Offset: 1, Length: 4},
		{Offset: 3, Length: 3},
	}
	h2.NegativeSpans = []histogram.Span{{Offset: 0, Length: 2}}
	h2.Count = 35
	h2.ZeroCount++
	h2.Sum = 30
	h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1}
	h2.NegativeBuckets = []int64{2, -1}
	fh2 := h2.ToFloat(nil)

	// Ensure recode produces a new chunk.
	newChunk, recoded, newApp, err := app.AppendFloatHistogram(nil, 200, 2000, fh2, false)
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.True(t, recoded)

	stChunk, ok := newChunk.(*FloatHistogramSTChunk)
	require.True(t, ok)
	require.Equal(t, 2, stChunk.NumSamples())

	it := stChunk.Iterator(nil)
	require.Equal(t, ValFloatHistogram, it.Next())
	require.Equal(t, int64(1000), it.AtT())
	require.Equal(t, int64(100), it.AtST())

	require.Equal(t, ValFloatHistogram, it.Next())
	require.Equal(t, int64(2000), it.AtT())
	require.Equal(t, int64(200), it.AtST())

	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	fh3 := fh2.Copy()
	fh3.Count = 40
	fh3.Sum = 35
	for i := range fh3.PositiveBuckets {
		fh3.PositiveBuckets[i]++
	}
	_, _, _, err = newApp.AppendFloatHistogram(nil, 300, 3000, fh3, false)
	require.NoError(t, err)
	require.Equal(t, 3, stChunk.NumSamples())
}

func TestFloatHistogramST_MoreThan127Samples(t *testing.T) {
	c := NewFloatHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	fh := &histogram.FloatHistogram{
		Count:         5,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-125,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []float64{6, 3},
	}

	const numSamples = maxFirstSTChangeOn + 3 // 130

	for i := range int(numSamples) {
		fhi := fh.Copy()
		fhi.Count = float64(5 + i)
		fhi.Sum = float64(18 + i)
		_, _, app, err = app.AppendFloatHistogram(nil, 500, int64(1000+i*1000), fhi, false)
		require.NoError(t, err)
	}
	require.Equal(t, int(numSamples), c.NumSamples())

	// Verify all samples round-trip correctly.
	it := c.Iterator(nil)
	for i := range int(numSamples) {
		require.Equal(t, ValFloatHistogram, it.Next())
		require.Equal(t, int64(1000+i*1000), it.AtT())
		require.Equal(t, int64(500), it.AtST())
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	c2 := NewFloatHistogramSTChunk()
	app2, err := c2.Appender()
	require.NoError(t, err)

	for i := range int(maxFirstSTChangeOn + 1) {
		fhi := fh.Copy()
		fhi.Count = float64(5 + i)
		fhi.Sum = float64(18 + i)
		_, _, app2, err = app2.AppendFloatHistogram(nil, 0, int64(1000+i*1000), fhi, false)
		require.NoError(t, err)
	}

	for i := range 3 {
		fhi := fh.Copy()
		fhi.Count = float64(200 + i)
		fhi.Sum = float64(200 + i)
		_, _, app2, err = app2.AppendFloatHistogram(nil, 100, int64(200000+i*1000), fhi, false)
		require.NoError(t, err)
	}

	it2 := c2.Iterator(nil)
	for i := range int(maxFirstSTChangeOn + 1) {
		require.Equal(t, ValFloatHistogram, it2.Next())
		require.Equal(t, int64(1000+i*1000), it2.AtT())
		require.Equal(t, int64(0), it2.AtST())
	}
	for i := range 3 {
		require.Equal(t, ValFloatHistogram, it2.Next())
		require.Equal(t, int64(200000+i*1000), it2.AtT())
		require.Equal(t, int64(100), it2.AtST())
	}
	require.Equal(t, ValNone, it2.Next())
	require.NoError(t, it2.Err())
}

func TestFloatHistogramSTChunkMixedST(t *testing.T) {
	fhs := tsdbutil.GenerateTestFloatHistograms(5)
	requireFloatHistogramSTSamples(t, []floatHistogramSTSample{
		{st: 0, t: 1000, fh: fhs[0]},
		{st: 0, t: 2000, fh: fhs[1]},
		{st: 100, t: 3000, fh: fhs[2]},
		{st: 0, t: 4000, fh: fhs[3]},
		{st: 200, t: 5000, fh: fhs[4]},
	})
}
