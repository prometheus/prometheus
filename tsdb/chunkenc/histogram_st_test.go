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

type histogramSTSample struct {
	st, t int64
	h     *histogram.Histogram
}

func BenchmarkHistogramSTWrite(b *testing.B) {
	const n = 120
	hs := tsdbutil.GenerateTestHistograms(n)

	b.ReportAllocs()

	for b.Loop() {
		c := NewHistogramSTChunk()
		app, _ := c.Appender()
		for i, h := range hs {
			_, _, app, _ = app.AppendHistogram(nil, 500, int64(i)*15000, h, false)
		}
	}
}

func BenchmarkHistogramSTRead(b *testing.B) {
	const n = 120
	hs := tsdbutil.GenerateTestHistograms(n)

	c := NewHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(b, err)
	for i, h := range hs {
		_, _, app, err = app.AppendHistogram(nil, 500, int64(i)*15000, h, false)
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

// requireHistogramSTSamples appends the given histogram samples to a new
// HistogramSTChunk, then verifies all samples round-trip correctly through
// the iterator.
func requireHistogramSTSamples(t *testing.T, samples []histogramSTSample) {
	t.Helper()

	c := NewHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	for _, s := range samples {
		_, _, app, err = app.AppendHistogram(nil, s.st, s.t, s.h, false)
		require.NoError(t, err)
	}

	require.Equal(t, len(samples), c.NumSamples())

	it := c.Iterator(nil)
	for _, s := range samples {
		require.Equal(t, ValHistogram, it.Next())
		require.Equal(t, s.t, it.AtT())
		require.Equal(t, s.st, it.AtST())
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}

func TestHistogramSTChunkST(t *testing.T) {
	testChunkSTHandling(t, ValHistogram, func() Chunk { return NewHistogramSTChunk() })
}

func TestHistogramSTBasic(t *testing.T) {
	hs := tsdbutil.GenerateTestHistograms(5)
	requireHistogramSTSamples(t, []histogramSTSample{
		{st: 0, t: 1000, h: hs[0]},
		{st: 0, t: 2000, h: hs[1]},
		{st: 0, t: 3000, h: hs[2]},
		{st: 0, t: 4000, h: hs[3]},
		{st: 0, t: 5000, h: hs[4]},
	})
}

func TestHistogramSTChunkAppendAndIterate(t *testing.T) {
	hs := tsdbutil.GenerateTestHistograms(5)
	requireHistogramSTSamples(t, []histogramSTSample{
		{st: 100, t: 1000, h: hs[0]},
		{st: 100, t: 2000, h: hs[1]},
		{st: 200, t: 3000, h: hs[2]},
		{st: 200, t: 4000, h: hs[3]},
		{st: 300, t: 5000, h: hs[4]},
	})
}

func TestHistogramSTChunkCounterReset(t *testing.T) {
	c := NewHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	h1 := &histogram.Histogram{
		Count:         10,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-125,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{6, -3},
	}

	_, _, app, err = app.AppendHistogram(nil, 100, 1000, h1, false)
	require.NoError(t, err)

	h2 := h1.Copy()
	h2.Count = 3
	h2.Sum = 5.0
	h2.PositiveBuckets = []int64{2, -1}
	h2.CounterResetHint = histogram.CounterReset

	newChunk, recoded, newApp, err := app.AppendHistogram(nil, 200, 2000, h2, false)
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.False(t, recoded)

	stChunk, ok := newChunk.(*HistogramSTChunk)
	require.True(t, ok)
	require.Equal(t, CounterReset, stChunk.GetCounterResetHeader())
	require.Equal(t, 1, stChunk.NumSamples())

	// Verify ST is preserved in the new chunk.
	it := stChunk.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	require.Equal(t, int64(2000), it.AtT())
	require.Equal(t, int64(200), it.AtST())
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	h3 := h2.Copy()
	h3.CounterResetHint = histogram.UnknownCounterReset
	h3.Count = 8
	h3.Sum = 10.0
	h3.PositiveBuckets = []int64{4, -2}
	_, _, _, err = newApp.AppendHistogram(nil, 300, 3000, h3, false)
	require.NoError(t, err)
	require.Equal(t, 2, stChunk.NumSamples())
}

func TestHistogramSTChunkRecode(t *testing.T) {
	c := NewHistogramSTChunk()
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

	_, _, app, err = app.AppendHistogram(nil, 100, 1000, h1, false)
	require.NoError(t, err)

	// Force recode with expanded bucket layout.
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

	// Ensure recode produced a new chunk
	newChunk, recoded, newApp, err := app.AppendHistogram(nil, 200, 2000, h2, false)
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.True(t, recoded)

	stChunk, ok := newChunk.(*HistogramSTChunk)
	require.True(t, ok)
	require.Equal(t, 2, stChunk.NumSamples())

	it := stChunk.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	require.Equal(t, int64(1000), it.AtT())
	require.Equal(t, int64(100), it.AtST())

	require.Equal(t, ValHistogram, it.Next())
	require.Equal(t, int64(2000), it.AtT())
	require.Equal(t, int64(200), it.AtST())

	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	h3 := h2.Copy()
	h3.Count = 40
	h3.Sum = 35
	for i := range h3.PositiveBuckets {
		h3.PositiveBuckets[i]++
	}
	_, _, _, err = newApp.AppendHistogram(nil, 300, 3000, h3, false)
	require.NoError(t, err)
	require.Equal(t, 3, stChunk.NumSamples())
}

func TestHistogramST_MoreThan127Samples(t *testing.T) {
	c := NewHistogramSTChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	h := &histogram.Histogram{
		Count:         5,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-125,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{6, -3},
	}

	const numSamples = maxFirstSTChangeOn + 3 // 130

	for i := range int(numSamples) {
		hi := h.Copy()
		hi.Count = uint64(5 + i)
		hi.Sum = float64(18 + i)
		_, _, app, err = app.AppendHistogram(nil, 500, int64(1000+i*1000), hi, false)
		require.NoError(t, err)
	}
	require.Equal(t, int(numSamples), c.NumSamples())

	it := c.Iterator(nil)
	for i := range int(numSamples) {
		require.Equal(t, ValHistogram, it.Next())
		require.Equal(t, int64(1000+i*1000), it.AtT())
		require.Equal(t, int64(500), it.AtST())
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())

	// Test ST changing after the boundary.
	c2 := NewHistogramSTChunk()
	app2, err := c2.Appender()
	require.NoError(t, err)

	for i := range int(maxFirstSTChangeOn + 1) {
		hi := h.Copy()
		hi.Count = uint64(5 + i)
		hi.Sum = float64(18 + i)
		_, _, app2, err = app2.AppendHistogram(nil, 0, int64(1000+i*1000), hi, false)
		require.NoError(t, err)
	}

	for i := range 3 {
		hi := h.Copy()
		hi.Count = uint64(200 + i)
		hi.Sum = float64(200 + i)
		_, _, app2, err = app2.AppendHistogram(nil, 100, int64(200000+i*1000), hi, false)
		require.NoError(t, err)
	}

	it2 := c2.Iterator(nil)
	for i := range int(maxFirstSTChangeOn + 1) {
		require.Equal(t, ValHistogram, it2.Next())
		require.Equal(t, int64(1000+i*1000), it2.AtT())
		require.Equal(t, int64(0), it2.AtST())
	}
	for i := range 3 {
		require.Equal(t, ValHistogram, it2.Next())
		require.Equal(t, int64(200000+i*1000), it2.AtT())
		require.Equal(t, int64(100), it2.AtST())
	}
	require.Equal(t, ValNone, it2.Next())
	require.NoError(t, it2.Err())
}

func TestHistogramSTChunkMixedST(t *testing.T) {
	hs := tsdbutil.GenerateTestHistograms(5)
	requireHistogramSTSamples(t, []histogramSTSample{
		{st: 0, t: 1000, h: hs[0]},
		{st: 0, t: 2000, h: hs[1]},
		{st: 100, t: 3000, h: hs[2]},
		{st: 0, t: 4000, h: hs[3]},
		{st: 200, t: 5000, h: hs[4]},
	})
}
