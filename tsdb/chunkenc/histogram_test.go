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

func TestHistogramChunkSameBuckets(t *testing.T) {
	c := NewHistogramChunk()
	var exp []res

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)
	h := &histogram.Histogram{
		Count:         5,
		ZeroCount:     2,
		Sum:           18.4,
		ZeroThreshold: 1e-100,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0}, // counts: 1, 2, 1, 1 (total 5)
	}
	app.AppendHistogram(ts, h)
	exp = append(exp, res{t: ts, h: h})
	require.Equal(t, 1, c.NumSamples())

	// Add an updated histogram.
	ts += 16
	h = h.Copy()
	h.Count += 9
	h.ZeroCount++
	h.Sum = 24.4
	h.PositiveBuckets = []int64{5, -2, 1, -2} // counts: 5, 3, 4, 2 (total 14)
	app.AppendHistogram(ts, h)
	exp = append(exp, res{t: ts, h: h})
	require.Equal(t, 2, c.NumSamples())

	// Add update with new appender.
	app, err = c.Appender()
	require.NoError(t, err)

	ts += 14
	h = h.Copy()
	h.Count += 13
	h.ZeroCount += 2
	h.Sum = 24.4
	h.PositiveBuckets = []int64{6, 1, -3, 6} // counts: 6, 7, 4, 10 (total 27)
	app.AppendHistogram(ts, h)
	exp = append(exp, res{t: ts, h: h})
	require.Equal(t, 3, c.NumSamples())

	// 1. Expand iterator in simple case.
	it := c.iterator(nil)
	require.NoError(t, it.Err())
	var act []res
	for it.Next() == ValHistogram {
		ts, h := it.AtHistogram()
		act = append(act, res{t: ts, h: h})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)

	// 2. Expand second iterator while reusing first one.
	// it2 := c.Iterator(it1)
	// var res2 []pair
	// for it2.Next() {
	//	ts, v := it2.At()
	//	res2 = append(res2, pair{t: ts, v: v})
	//	}
	//	require.NoError(t, it2.Err())
	//	require.Equal(t, exp, res2)

	// 3. Test iterator Seek.
	//	mid := len(exp) / 2

	//	it3 := c.Iterator(nil)
	//	var res3 []pair
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	// Below ones should not matter.
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	//	ts, v = it3.At()
	//	res3 = append(res3, pair{t: ts, v: v})

	//	for it3.Next() {
	//		ts, v := it3.At()
	//		res3 = append(res3, pair{t: ts, v: v})
	//	}
	//	require.NoError(t, it3.Err())
	//	require.Equal(t, exp[mid:], res3)
	//	require.Equal(t, false, it3.Seek(exp[len(exp)-1].t+1))
}

type res struct {
	t int64
	h *histogram.Histogram
}

// Mimics the scenario described for compareSpans().
func TestHistogramChunkBucketChanges(t *testing.T) {
	c := Chunk(NewHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts1 := int64(1234567890)
	h1 := &histogram.Histogram{
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
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // counts: 6, 3, 3, 2, 4, 5, 1 (total 24)
	}

	app.AppendHistogram(ts1, h1)
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
	h2.Count += 9
	h2.ZeroCount++
	h2.Sum = 30
	// Existing histogram should get values converted from the above to:
	//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
	// so the new histogram should have new counts >= these per-bucket counts, e.g.:
	h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // 7 5 1 3 1 0 2 5 5 0 1 (total 30)

	// This is how span changes will be handled.
	hApp, _ := app.(*HistogramAppender)
	posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
	require.Greater(t, len(posInterjections), 0)
	require.Equal(t, 0, len(negInterjections))
	require.True(t, ok) // Only new buckets came in.
	require.False(t, cr)
	c, app = hApp.Recode(posInterjections, negInterjections, h2.PositiveSpans, h2.NegativeSpans)
	app.AppendHistogram(ts2, h2)

	require.Equal(t, 2, c.NumSamples())

	// Because the 2nd histogram has expanded buckets, we should expect all
	// histograms (in particular the first) to come back using the new spans
	// metadata as well as the expanded buckets.
	h1.PositiveSpans = h2.PositiveSpans
	h1.PositiveBuckets = []int64{6, -3, -3, 3, -3, 0, 2, 2, 1, -5, 1}
	exp := []res{
		{t: ts1, h: h1},
		{t: ts2, h: h2},
	}
	it := c.Iterator(nil)
	var act []res
	for it.Next() == ValHistogram {
		ts, h := it.AtHistogram()
		act = append(act, res{t: ts, h: h})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)
}

func TestHistoChunkAppendable(t *testing.T) {
	c := Chunk(NewHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)
	h1 := &histogram.Histogram{
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
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // counts: 6, 3, 3, 2, 4, 5, 1 (total 24)
	}

	app.AppendHistogram(ts, h1)
	require.Equal(t, 1, c.NumSamples())

	{ // New histogram that has more buckets.
		h2 := h1
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Count += 9
		h2.ZeroCount++
		h2.Sum = 30
		// Existing histogram should get values converted from the above to:
		//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // 7 5 1 3 1 0 2 5 5 0 1 (total 30)

		hApp, _ := app.(*HistogramAppender)
		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Greater(t, len(posInterjections), 0)
		require.Equal(t, 0, len(negInterjections))
		require.True(t, ok) // Only new buckets came in.
		require.False(t, cr)
	}

	{ // New histogram that has a bucket missing.
		h2 := h1
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 5, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 21
		h2.PositiveBuckets = []int64{6, -3, -1, 2, 1, -4} // counts: 6, 3, 2, 4, 5, 1 (total 21)

		hApp, _ := app.(*HistogramAppender)
		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}

	{ // New histogram that has a counter reset while buckets are same.
		h2 := h1
		h2.Sum = 23
		h2.PositiveBuckets = []int64{6, -4, 1, -1, 2, 1, -4} // counts: 6, 2, 3, 2, 4, 5, 1 (total 23)

		hApp, _ := app.(*HistogramAppender)
		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}

	{ // New histogram that has a counter reset while new buckets were added.
		h2 := h1
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Sum = 29
		// Existing histogram should get values converted from the above to:
		//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 0} // 7 5 1 3 1 0 2 5 5 0 0 (total 29)

		hApp, _ := app.(*HistogramAppender)
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
		h2 := h1
		h2.PositiveSpans = []histogram.Span{
			{Offset: -3, Length: 2},
			{Offset: 1, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 26
		// Existing histogram should get values converted from the above to:
		//   0, 0, 6, 3, 3, 2, 4, 5, 1
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{1, 1, 3, -2, 0, -1, 2, 1, -4} // counts: 1, 2, 5, 3, 3, 2, 4, 5, 1 (total 26)

		hApp, _ := app.(*HistogramAppender)
		posInterjections, negInterjections, ok, cr := hApp.Appendable(h2)
		require.Equal(t, 0, len(posInterjections))
		require.Equal(t, 0, len(negInterjections))
		require.False(t, ok) // Need to cut a new chunk.
		require.True(t, cr)
	}
}
