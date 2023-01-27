// Copyright 2021 The Prometheus Authors
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
)

type floatResult struct {
	t int64
	h *histogram.FloatHistogram
}

func TestFloatHistogramChunkSameBuckets(t *testing.T) {
	c := NewFloatHistogramChunk()
	var exp []floatResult

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)
	h := &histogram.Histogram{
		Count:         15,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-100,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0}, // counts: 1, 2, 1, 1 (total 5)
		NegativeSpans: []histogram.Span{
			{Offset: 1, Length: 1},
			{Offset: 2, Length: 3},
		},
		NegativeBuckets: []int64{2, 1, -1, -1}, // counts: 2, 3, 2, 1 (total 8)
	}
	app.AppendFloatHistogram(ts, h.ToFloat())
	exp = append(exp, floatResult{t: ts, h: h.ToFloat()})
	require.Equal(t, 1, c.NumSamples())

	// Add an updated histogram.
	ts += 16
	h = h.Copy()
	h.Count = 32
	h.ZeroCount++
	h.Sum = 24.4
	h.PositiveBuckets = []int64{5, -2, 1, -2} // counts: 5, 3, 4, 2 (total 14)
	h.NegativeBuckets = []int64{4, -1, 1, -1} // counts: 4, 3, 4, 4 (total 15)
	app.AppendFloatHistogram(ts, h.ToFloat())
	expH := h.ToFloat()
	expH.CounterResetHint = histogram.NotCounterReset
	exp = append(exp, floatResult{t: ts, h: expH})
	require.Equal(t, 2, c.NumSamples())

	// Add update with new appender.
	app, err = c.Appender()
	require.NoError(t, err)

	ts += 14
	h = h.Copy()
	h.Count = 54
	h.ZeroCount += 2
	h.Sum = 24.4
	h.PositiveBuckets = []int64{6, 1, -3, 6} // counts: 6, 7, 4, 10 (total 27)
	h.NegativeBuckets = []int64{5, 1, -2, 3} // counts: 5, 6, 4, 7 (total 22)
	app.AppendFloatHistogram(ts, h.ToFloat())
	expH = h.ToFloat()
	expH.CounterResetHint = histogram.NotCounterReset
	exp = append(exp, floatResult{t: ts, h: expH})
	require.Equal(t, 3, c.NumSamples())

	// 1. Expand iterator in simple case.
	it := c.Iterator(nil)
	require.NoError(t, it.Err())
	var act []floatResult
	for it.Next() == ValFloatHistogram {
		fts, fh := it.AtFloatHistogram()
		act = append(act, floatResult{t: fts, h: fh})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)

	// 2. Expand second iterator while reusing first one.
	it2 := c.Iterator(it)
	var act2 []floatResult
	for it2.Next() == ValFloatHistogram {
		fts, fh := it2.AtFloatHistogram()
		act2 = append(act2, floatResult{t: fts, h: fh})
	}
	require.NoError(t, it2.Err())
	require.Equal(t, exp, act2)

	// 3. Now recycle an iterator that was never used to access anything.
	itX := c.Iterator(nil)
	for itX.Next() == ValFloatHistogram {
		// Just iterate through without accessing anything.
	}
	it3 := c.iterator(itX)
	var act3 []floatResult
	for it3.Next() == ValFloatHistogram {
		fts, fh := it3.AtFloatHistogram()
		act3 = append(act3, floatResult{t: fts, h: fh})
	}
	require.NoError(t, it3.Err())
	require.Equal(t, exp, act3)

	// 4. Test iterator Seek.
	mid := len(exp) / 2
	it4 := c.Iterator(nil)
	var act4 []floatResult
	require.Equal(t, ValFloatHistogram, it4.Seek(exp[mid].t))
	// Below ones should not matter.
	require.Equal(t, ValFloatHistogram, it4.Seek(exp[mid].t))
	require.Equal(t, ValFloatHistogram, it4.Seek(exp[mid].t))
	fts, fh := it4.AtFloatHistogram()
	act4 = append(act4, floatResult{t: fts, h: fh})
	for it4.Next() == ValFloatHistogram {
		fts, fh := it4.AtFloatHistogram()
		act4 = append(act4, floatResult{t: fts, h: fh})
	}
	require.NoError(t, it4.Err())
	require.Equal(t, exp[mid:], act4)
	require.Equal(t, ValNone, it4.Seek(exp[len(exp)-1].t+1))
}

// Mimics the scenario described for expandSpansForward.
func TestFloatHistogramChunkBucketChanges(t *testing.T) {
	c := Chunk(NewFloatHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts1 := int64(1234567890)
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
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // counts: 6, 3, 3, 2, 4, 5, 1 (total 24)
		NegativeSpans:   []histogram.Span{{Offset: 1, Length: 1}},
		NegativeBuckets: []int64{1},
	}

	app.AppendFloatHistogram(ts1, h1.ToFloat())
	require.Equal(t, 1, c.NumSamples())

	// Add a new histogram that has expanded buckets.
	ts2 := ts1 + 16
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
	// Existing histogram should get values converted from the above to:
	//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
	// so the new histogram should have new counts >= these per-bucket counts, e.g.:
	h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // 7 5 1 3 1 0 2 5 5 0 1 (total 30)
	// Existing histogram should get values converted from the above to:
	//   0 1 (previous values with some new empty buckets in between)
	// so the new histogram should have new counts >= these per-bucket counts, e.g.:
	h2.NegativeBuckets = []int64{2, -1} // 2 1 (total 3)
	// This is how span changes will be handled.
	hApp, _ := app.(*FloatHistogramAppender)
	posInterjections, negInterjections, ok, cr := hApp.Appendable(h2.ToFloat())
	require.Greater(t, len(posInterjections), 0)
	require.Greater(t, len(negInterjections), 0)
	require.True(t, ok) // Only new buckets came in.
	require.False(t, cr)
	c, app = hApp.Recode(posInterjections, negInterjections, h2.PositiveSpans, h2.NegativeSpans)
	app.AppendFloatHistogram(ts2, h2.ToFloat())

	require.Equal(t, 2, c.NumSamples())

	// Because the 2nd histogram has expanded buckets, we should expect all
	// histograms (in particular the first) to come back using the new spans
	// metadata as well as the expanded buckets.
	h1.PositiveSpans = h2.PositiveSpans
	h1.PositiveBuckets = []int64{6, -3, -3, 3, -3, 0, 2, 2, 1, -5, 1}
	h1.NegativeSpans = h2.NegativeSpans
	h1.NegativeBuckets = []int64{0, 1}
	expH2 := h2.ToFloat()
	expH2.CounterResetHint = histogram.NotCounterReset
	exp := []floatResult{
		{t: ts1, h: h1.ToFloat()},
		{t: ts2, h: expH2},
	}
	it := c.Iterator(nil)
	var act []floatResult
	for it.Next() == ValFloatHistogram {
		fts, fh := it.AtFloatHistogram()
		act = append(act, floatResult{t: fts, h: fh})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)
}

func TestFloatHistogramChunkAppendable(t *testing.T) {
	c := Chunk(NewFloatHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)
	h1 := &histogram.FloatHistogram{
		Count:         5,
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
		PositiveBuckets: []float64{6, 3, 3, 2, 4, 5, 1},
	}

	app.AppendFloatHistogram(ts, h1.Copy())
	require.Equal(t, 1, c.NumSamples())
	hApp, _ := app.(*FloatHistogramAppender)

	{ // Schema change.
		h2 := h1.Copy()
		h2.Schema++
		_, _, ok, _ := hApp.Appendable(h2)
		require.False(t, ok)
	}

	{ // Zero threshold change.
		h2 := h1.Copy()
		h2.ZeroThreshold += 0.1
		_, _, ok, _ := hApp.Appendable(h2)
		require.False(t, ok)
	}

	{ // New histogram that has more buckets.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Count += 9
		h2.ZeroCount++
		h2.Sum = 30
		h2.PositiveBuckets = []float64{7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 1}

		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Greater(t, len(posInterjections), 0)
		require.Equal(t, 0, len(negInterjections))
		require.True(t, ok) // Only new buckets came in.
		require.False(t, cr)
	}

	{ // New histogram that has a bucket missing.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 5, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 21
		h2.PositiveBuckets = []float64{6, 3, 2, 4, 5, 1}

		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}

	{ // New histogram that has a counter reset while buckets are same.
		h2 := h1.Copy()
		h2.Sum = 23
		h2.PositiveBuckets = []float64{6, 2, 3, 2, 4, 5, 1}

		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}

	{ // New histogram that has a counter reset while new buckets were added.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Sum = 29
		h2.PositiveBuckets = []float64{7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 0}

		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}

	{
		// New histogram that has a counter reset while new buckets were
		// added before the first bucket and reset on first bucket.  (to
		// catch the edge case where the new bucket should be forwarded
		// ahead until first old bucket at start)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: -3, Length: 2},
			{Offset: 1, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 26
		h2.PositiveBuckets = []float64{1, 2, 5, 3, 3, 2, 4, 5, 1}

		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}
}

func TestFloatHistogramChunkAppendableGauge(t *testing.T) {
	c := Chunk(NewFloatHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)
	h1 := &histogram.FloatHistogram{
		Count:         5,
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
		PositiveBuckets: []float64{6, 3, 3, 2, 4, 5, 1},
	}

	app.AppendFloatHistogram(ts, h1.Copy())
	require.Equal(t, 1, c.NumSamples())
	c.(*FloatHistogramChunk).SetCounterResetHeader(GaugeType)

	{ // Schema change.
		h2 := h1.Copy()
		h2.Schema++
		hApp, _ := app.(*FloatHistogramAppender)
		_, _, _, _, _, _, ok := hApp.AppendableGauge(h2)
		require.False(t, ok)
	}

	{ // Zero threshold change.
		h2 := h1.Copy()
		h2.ZeroThreshold += 0.1
		hApp, _ := app.(*FloatHistogramAppender)
		_, _, _, _, _, _, ok := hApp.AppendableGauge(h2)
		require.False(t, ok)
	}

	{ // New histogram that has more buckets.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Count += 9
		h2.ZeroCount++
		h2.Sum = 30
		h2.PositiveBuckets = []float64{7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 1}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Greater(t, len(pI), 0)
		require.Len(t, nI, 0)
		require.Len(t, pBackwardI, 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}

	{ // New histogram that has buckets missing.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 1},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Count -= 4
		h2.Sum--
		h2.PositiveBuckets = []float64{6, 3, 3, 2, 5, 1}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Len(t, pI, 0)
		require.Len(t, nI, 0)
		require.Greater(t, len(pBackwardI), 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}

	{ // New histogram that has a bucket missing and new buckets.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 5, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 21
		h2.PositiveBuckets = []float64{6, 3, 2, 4, 5, 1}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Greater(t, len(pI), 0)
		require.Greater(t, len(pBackwardI), 0)
		require.Len(t, nI, 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}

	{ // New histogram that has a counter reset while buckets are same.
		h2 := h1.Copy()
		h2.Sum = 23
		h2.PositiveBuckets = []float64{6, 2, 3, 2, 4, 5, 1}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Len(t, pI, 0)
		require.Len(t, nI, 0)
		require.Len(t, pBackwardI, 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}

	{ // New histogram that has a counter reset while new buckets were added.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Sum = 29
		h2.PositiveBuckets = []float64{7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 0}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Greater(t, len(pI), 0)
		require.Len(t, nI, 0)
		require.Len(t, pBackwardI, 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}

	{
		// New histogram that has a counter reset while new buckets were
		// added before the first bucket and reset on first bucket.
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: -3, Length: 2},
			{Offset: 1, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 26
		h2.PositiveBuckets = []float64{1, 2, 5, 3, 3, 2, 4, 5, 1}

		hApp, _ := app.(*FloatHistogramAppender)
		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.AppendableGauge(h2)
		require.Greater(t, len(pI), 0)
		require.Len(t, nI, 0)
		require.Len(t, pBackwardI, 0)
		require.Len(t, nBackwardI, 0)
		require.True(t, ok)
	}
}
