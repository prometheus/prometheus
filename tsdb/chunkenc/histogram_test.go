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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

type result struct {
	t  int64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func TestFirstHistogramExplicitCounterReset(t *testing.T) {
	tests := map[string]struct {
		hint      histogram.CounterResetHint
		expHeader CounterResetHeader
		expHint   histogram.CounterResetHint
	}{
		"CounterReset": {
			hint:      histogram.CounterReset,
			expHeader: CounterReset,
			expHint:   histogram.UnknownCounterReset,
		},
		"NotCounterReset": {
			hint:      histogram.NotCounterReset,
			expHeader: UnknownCounterReset,
			expHint:   histogram.UnknownCounterReset,
		},
		"UnknownCounterReset": {
			hint:      histogram.UnknownCounterReset,
			expHeader: UnknownCounterReset,
			expHint:   histogram.UnknownCounterReset,
		},
		"Gauge": {
			hint:      histogram.GaugeType,
			expHeader: GaugeType,
			expHint:   histogram.GaugeType,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			h := &histogram.Histogram{
				CounterResetHint: test.hint,
			}
			chk := NewHistogramChunk()
			app, err := chk.Appender()
			require.NoError(t, err)
			newChk, recoded, newApp, err := app.AppendHistogram(nil, 0, h, false)
			require.NoError(t, err)
			require.Nil(t, newChk)
			require.False(t, recoded)
			require.Equal(t, app, newApp)
			require.Equal(t, test.expHeader, chk.GetCounterResetHeader())
			assertFirstIntHistogramSampleHint(t, chk, test.expHint)
		})
	}
}

func TestHistogramChunkSameBuckets(t *testing.T) {
	c := NewHistogramChunk()
	var exp []result

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
	chk, _, app, err := app.AppendHistogram(nil, ts, h, false)
	require.NoError(t, err)
	require.Nil(t, chk)
	exp = append(exp, result{t: ts, h: h, fh: h.ToFloat(nil)})
	require.Equal(t, 1, c.NumSamples())

	// Add an updated histogram.
	ts += 16
	h = h.Copy()
	h.Count = 32
	h.ZeroCount++
	h.Sum = 24.4
	h.PositiveBuckets = []int64{5, -2, 1, -2} // counts: 5, 3, 4, 2 (total 14)
	h.NegativeBuckets = []int64{4, -1, 1, -1} // counts: 4, 3, 4, 4 (total 15)
	chk, _, _, err = app.AppendHistogram(nil, ts, h, false)
	require.NoError(t, err)
	require.Nil(t, chk)
	hExp := h.Copy()
	hExp.CounterResetHint = histogram.NotCounterReset
	exp = append(exp, result{t: ts, h: hExp, fh: hExp.ToFloat(nil)})
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
	chk, _, _, err = app.AppendHistogram(nil, ts, h, false)
	require.NoError(t, err)
	require.Nil(t, chk)
	hExp = h.Copy()
	hExp.CounterResetHint = histogram.NotCounterReset
	exp = append(exp, result{t: ts, h: hExp, fh: hExp.ToFloat(nil)})
	require.Equal(t, 3, c.NumSamples())

	// 1. Expand iterator in simple case.
	it := c.Iterator(nil)
	require.NoError(t, it.Err())
	var act []result
	for it.Next() == ValHistogram {
		ts, h := it.AtHistogram(nil)
		fts, fh := it.AtFloatHistogram(nil)
		require.Equal(t, ts, fts)
		act = append(act, result{t: ts, h: h, fh: fh})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)

	// 2. Expand second iterator while reusing first one.
	it2 := c.Iterator(it)
	var act2 []result
	for it2.Next() == ValHistogram {
		ts, h := it2.AtHistogram(nil)
		fts, fh := it2.AtFloatHistogram(nil)
		require.Equal(t, ts, fts)
		act2 = append(act2, result{t: ts, h: h, fh: fh})
	}
	require.NoError(t, it2.Err())
	require.Equal(t, exp, act2)

	// 3. Now recycle an iterator that was never used to access anything.
	itX := c.Iterator(nil)
	for itX.Next() == ValHistogram {
		// Just iterate through without accessing anything.
	}
	it3 := c.iterator(itX)
	var act3 []result
	for it3.Next() == ValHistogram {
		ts, h := it3.AtHistogram(nil)
		fts, fh := it3.AtFloatHistogram(nil)
		require.Equal(t, ts, fts)
		act3 = append(act3, result{t: ts, h: h, fh: fh})
	}
	require.NoError(t, it3.Err())
	require.Equal(t, exp, act3)

	// 4. Test iterator Seek.
	mid := len(exp) / 2
	it4 := c.Iterator(nil)
	var act4 []result
	require.Equal(t, ValHistogram, it4.Seek(exp[mid].t))
	// Below ones should not matter.
	require.Equal(t, ValHistogram, it4.Seek(exp[mid].t))
	require.Equal(t, ValHistogram, it4.Seek(exp[mid].t))
	ts, h = it4.AtHistogram(nil)
	fts, fh := it4.AtFloatHistogram(nil)
	require.Equal(t, ts, fts)
	act4 = append(act4, result{t: ts, h: h, fh: fh})
	for it4.Next() == ValHistogram {
		ts, h := it4.AtHistogram(nil)
		fts, fh := it4.AtFloatHistogram(nil)
		require.Equal(t, ts, fts)
		act4 = append(act4, result{t: ts, h: h, fh: fh})
	}
	require.NoError(t, it4.Err())
	require.Equal(t, exp[mid:], act4)
	require.Equal(t, ValNone, it4.Seek(exp[len(exp)-1].t+1))
}

// Mimics the scenario described for expandIntSpansAndBuckets.
func TestHistogramChunkBucketChanges(t *testing.T) {
	c := Chunk(NewHistogramChunk())

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

	chk, _, app, err := app.AppendHistogram(nil, ts1, h1, false)
	require.NoError(t, err)
	require.Nil(t, chk)
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
	hApp, _ := app.(*HistogramAppender)
	posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
	require.NotEmpty(t, posInterjections)
	require.NotEmpty(t, negInterjections)
	require.Empty(t, backwardPositiveInserts)
	require.Empty(t, backwardNegativeInserts)
	require.True(t, ok) // Only new buckets came in.
	require.Equal(t, NotCounterReset, cr)
	c, app = hApp.recode(posInterjections, negInterjections, h2.PositiveSpans, h2.NegativeSpans)
	chk, _, _, err = app.AppendHistogram(nil, ts2, h2, false)
	require.NoError(t, err)
	require.Nil(t, chk)

	require.Equal(t, 2, c.NumSamples())

	// Because the 2nd histogram has expanded buckets, we should expect all
	// histograms (in particular the first) to come back using the new spans
	// metadata as well as the expanded buckets.
	h1.PositiveSpans = h2.PositiveSpans
	h1.PositiveBuckets = []int64{6, -3, -3, 3, -3, 0, 2, 2, 1, -5, 1}
	h1.NegativeSpans = h2.NegativeSpans
	h1.NegativeBuckets = []int64{0, 1}
	hExp := h2.Copy()
	hExp.CounterResetHint = histogram.NotCounterReset
	exp := []result{
		{t: ts1, h: h1, fh: h1.ToFloat(nil)},
		{t: ts2, h: hExp, fh: hExp.ToFloat(nil)},
	}
	it := c.Iterator(nil)
	var act []result
	for it.Next() == ValHistogram {
		ts, h := it.AtHistogram(nil)
		fts, fh := it.AtFloatHistogram(nil)
		require.Equal(t, ts, fts)
		act = append(act, result{t: ts, h: h, fh: fh})
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, act)
}

func TestHistogramChunkAppendable(t *testing.T) {
	eh := &histogram.Histogram{
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

	cbh := &histogram.Histogram{
		Count:  24,
		Sum:    18.4,
		Schema: histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		},
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // counts: 6, 3, 3, 2, 4, 5, 1 (total 24)
		CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}

	setup := func(h *histogram.Histogram) (Chunk, *HistogramAppender, int64, *histogram.Histogram) {
		c := Chunk(NewHistogramChunk())

		// Create fresh appender and add the first histogram.
		app, err := c.Appender()
		require.NoError(t, err)
		require.Equal(t, 0, c.NumSamples())

		ts := int64(1234567890)

		chk, _, app, err := app.AppendHistogram(nil, ts, h.Copy(), false)
		require.NoError(t, err)
		require.Nil(t, chk)
		require.Equal(t, 1, c.NumSamples())
		require.Equal(t, UnknownCounterReset, c.(*HistogramChunk).GetCounterResetHeader())
		return c, app.(*HistogramAppender), ts, h
	}

	{ // Schema change.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Schema++
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset, histogram.UnknownCounterReset)
	}

	{ // Zero threshold change.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.ZeroThreshold += 0.1
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset, histogram.UnknownCounterReset)
	}

	{ // New histogram that has more buckets.
		c, hApp, ts, h1 := setup(eh)
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

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.NotEmpty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.True(t, ok) // Only new buckets came in.
		require.Equal(t, NotCounterReset, cr)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)
	}

	{ // New histogram that has a bucket missing.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 5, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 21
		h2.PositiveBuckets = []int64{6, -3, -1, 2, 1, -4} // counts: 6, 3, 2, 4, 5, 1 (total 21)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, CounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // New histogram that has buckets missing but the buckets missing were empty.
		emptyBucketH := eh.Copy()
		emptyBucketH.PositiveBuckets = []int64{6, -6, 1, 1, -2, 1, 1} // counts: 6, 0, 1, 2, 0, 1, 2 (total 12)
		c, hApp, ts, h1 := setup(emptyBucketH)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{ // Missing buckets at offset 1 and 9.
			{Offset: 0, Length: 1},
			{Offset: 3, Length: 1},
			{Offset: 3, Length: 1},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 1},
		}
		savedH2Spans := h2.PositiveSpans
		h2.PositiveBuckets = []int64{7, -5, 1, 0, 1} // counts: 7, 2, 3, 3, 4 (total 18)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.NotEmpty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.True(t, ok)
		require.Equal(t, NotCounterReset, cr)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)

		// Check that h2 was recoded.
		require.Equal(t, []int64{7, -7, 2, 1, -3, 3, 1}, h2.PositiveBuckets) // counts: 7, 0, 2, 3 , 0, 3, 4 (total 18)
		require.Equal(t, emptyBucketH.PositiveSpans, h2.PositiveSpans)
		require.NotEqual(t, savedH2Spans, h2.PositiveSpans, "recoding must make a copy")
	}

	{ // New histogram that has new buckets AND buckets missing but the buckets missing were empty.
		emptyBucketH := eh.Copy()
		emptyBucketH.PositiveBuckets = []int64{6, -6, 1, 1, -2, 1, 1} // counts: 6, 0, 1, 2, 0, 1, 2 (total 12)
		c, hApp, ts, h1 := setup(emptyBucketH)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{ // Missing buckets at offset 1 and 9.
			{Offset: 0, Length: 1},
			{Offset: 3, Length: 1},
			{Offset: 3, Length: 1},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 2},
		}
		savedH2Spans := h2.PositiveSpans
		h2.PositiveBuckets = []int64{7, -5, 1, 0, 1, 1} // counts: 7, 2, 3, 3, 4, 5 (total 23)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.NotEmpty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.NotEmpty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.True(t, ok)
		require.Equal(t, NotCounterReset, cr)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)

		// Check that h2 was recoded.
		require.Equal(t, []int64{7, -7, 2, 1, -3, 3, 1, 1}, h2.PositiveBuckets) // counts: 7, 0, 2, 3 , 0, 3, 5 (total 23)
		require.Equal(t, []histogram.Span{
			{Offset: 0, Length: 2}, // Added empty bucket.
			{Offset: 2, Length: 1}, // Existing - offset adjusted.
			{Offset: 3, Length: 2}, // Added empty bucket.
			{Offset: 3, Length: 1}, // Existing - offset adjusted.
			{Offset: 1, Length: 2}, // Existing.
		}, h2.PositiveSpans)
		require.NotEqual(t, savedH2Spans, h2.PositiveSpans, "recoding must make a copy")
	}

	{ // New histogram that has a counter reset while buckets are same.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Sum = 23
		h2.PositiveBuckets = []int64{6, -4, 1, -1, 2, 1, -4} // counts: 6, 2, 3, 2, 4, 5, 1 (total 23)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, CounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // New histogram that has a counter reset while new buckets were added.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
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

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, CounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{
		// New histogram that has a counter reset while new buckets were
		// added before the first bucket and reset on first bucket.  (to
		// catch the edge case where the new bucket should be forwarded
		// ahead until first old bucket at start)
		c, hApp, ts, h1 := setup(eh)
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
		// Existing histogram should get values converted from the above to:
		//   0, 0, 6, 3, 3, 2, 4, 5, 1
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{1, 1, 3, -2, 0, -1, 2, 1, -4} // counts: 1, 2, 5, 3, 3, 2, 4, 5, 1 (total 26)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, CounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // New histogram that has an explicit counter reset.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.CounterResetHint = histogram.CounterReset

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // Start new chunk explicitly, and append a new histogram that is considered appendable to the previous chunk.
		_, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy() // Identity is appendable.

		nextChunk := NewHistogramChunk()
		app, err := nextChunk.Appender()
		require.NoError(t, err)
		newChunk, recoded, newApp, err := app.AppendHistogram(hApp, ts+1, h2, false)
		require.NoError(t, err)
		require.Nil(t, newChunk)
		require.False(t, recoded)
		require.Equal(t, app, newApp)
		assertSampleCount(t, nextChunk, 1, ValHistogram)
		require.Equal(t, NotCounterReset, nextChunk.GetCounterResetHeader())
		assertFirstIntHistogramSampleHint(t, nextChunk, histogram.UnknownCounterReset)
	}

	{ // Start new chunk explicitly, and append a new histogram that is not considered appendable to the previous chunk.
		_, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Count-- // Make this not appendable due to counter reset.

		nextChunk := NewHistogramChunk()
		app, err := nextChunk.Appender()
		require.NoError(t, err)
		newChunk, recoded, newApp, err := app.AppendHistogram(hApp, ts+1, h2, false)
		require.NoError(t, err)
		require.Nil(t, newChunk)
		require.False(t, recoded)
		require.Equal(t, app, newApp)
		assertSampleCount(t, nextChunk, 1, ValHistogram)
		require.Equal(t, CounterReset, nextChunk.GetCounterResetHeader())
		assertFirstIntHistogramSampleHint(t, nextChunk, histogram.UnknownCounterReset)
	}

	{ // Start new chunk explicitly, and append a new histogram that would need recoding if we added it to the chunk.
		_, hApp, ts, h1 := setup(eh)
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

		nextChunk := NewHistogramChunk()
		app, err := nextChunk.Appender()
		require.NoError(t, err)
		newChunk, recoded, newApp, err := app.AppendHistogram(hApp, ts+1, h2, false)
		require.NoError(t, err)
		require.Nil(t, newChunk)
		require.False(t, recoded)
		require.Equal(t, app, newApp)
		assertSampleCount(t, nextChunk, 1, ValHistogram)
		require.Equal(t, NotCounterReset, nextChunk.GetCounterResetHeader())
		assertFirstIntHistogramSampleHint(t, nextChunk, histogram.UnknownCounterReset)
	}

	{
		// Start a new chunk with a histogram that has an empty bucket.
		// Add a histogram that has the same bucket missing.
		// This should be appendable and can happen if we are merging from chunks
		// where the first sample came from a recoded chunk that added the
		// empty bucket.
		h1 := eh.Copy()
		// Add a bucket that is empty -10 offsets from the first bucket.
		h1.PositiveSpans = make([]histogram.Span, len(eh.PositiveSpans)+1)
		h1.PositiveSpans[0] = histogram.Span{Offset: eh.PositiveSpans[0].Offset - 10, Length: 1}
		h1.PositiveSpans[1] = histogram.Span{Offset: eh.PositiveSpans[0].Offset + 9, Length: eh.PositiveSpans[0].Length}
		for i, v := range eh.PositiveSpans[1:] {
			h1.PositiveSpans[i+2] = v
		}
		h1.PositiveBuckets = make([]int64, len(eh.PositiveBuckets)+1)
		h1.PositiveBuckets[0] = 0
		for i, v := range eh.PositiveBuckets {
			h1.PositiveBuckets[i+1] = v
		}

		c, hApp, ts, _ := setup(h1)
		h2 := eh.Copy()

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.NotEmpty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.True(t, ok)
		require.Equal(t, NotCounterReset, cr)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)
	}

	{ // Custom buckets, no change.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)
	}

	{ // Custom buckets, increase in bucket counts but no change in layout.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.Count++
		h2.PositiveBuckets = []int64{6, -3, 0, -1, 2, 1, -3}
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)
	}

	{ // Custom buckets, decrease in bucket counts but no change in layout.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.Count--
		h2.PositiveBuckets = []int64{6, -3, 0, -1, 2, 1, -5}
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // Custom buckets, change only in custom bounds.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.CustomValues = []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}
		_, _, _, _, ok, _ := hApp.appendable(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, CounterReset, histogram.UnknownCounterReset)
	}

	{ // Custom buckets, with more buckets.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Count += 6
		h2.Sum = 30
		// Existing histogram should get values converted from the above to:
		//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // 7 5 1 3 1 0 2 5 5 0 1 (total 30)

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.NotEmpty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.True(t, ok) // Only new buckets came in.
		require.Equal(t, NotCounterReset, cr)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset)
	}

	{ // New histogram with a different schema.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Schema = 2

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, UnknownCounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset, histogram.UnknownCounterReset)
	}

	{ // New histogram with a different schema.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.ZeroThreshold = 1e-120

		posInterjections, negInterjections, backwardPositiveInserts, backwardNegativeInserts, ok, cr := hApp.appendable(h2)
		require.Empty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, backwardPositiveInserts)
		require.Empty(t, backwardNegativeInserts)
		require.False(t, ok) // Need to cut a new chunk.
		require.Equal(t, UnknownCounterReset, cr)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, UnknownCounterReset, histogram.UnknownCounterReset)
	}
}

func assertNewHistogramChunkOnAppend(t *testing.T, oldChunk Chunk, hApp *HistogramAppender, ts int64, h *histogram.Histogram, expectHeader CounterResetHeader, expectHint histogram.CounterResetHint) {
	oldChunkBytes := oldChunk.Bytes()
	newChunk, recoded, newAppender, err := hApp.AppendHistogram(nil, ts, h, false)
	require.Equal(t, oldChunkBytes, oldChunk.Bytes()) // Sanity check that previous chunk is untouched.
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.False(t, recoded)
	require.NotEqual(t, oldChunk, newChunk)
	require.Equal(t, expectHeader, newChunk.(*HistogramChunk).GetCounterResetHeader())
	require.NotNil(t, newAppender)
	require.NotEqual(t, hApp, newAppender)
	assertSampleCount(t, newChunk, 1, ValHistogram)
	assertFirstIntHistogramSampleHint(t, newChunk, expectHint)
}

func assertNoNewHistogramChunkOnAppend(t *testing.T, currChunk Chunk, hApp *HistogramAppender, ts int64, h *histogram.Histogram, expectHeader CounterResetHeader) {
	prevChunkBytes := currChunk.Bytes()
	newChunk, recoded, newAppender, err := hApp.AppendHistogram(nil, ts, h, false)
	require.Greater(t, len(currChunk.Bytes()), len(prevChunkBytes)) // Check that current chunk is bigger than previously.
	require.NoError(t, err)
	require.Nil(t, newChunk)
	require.False(t, recoded)
	require.Equal(t, expectHeader, currChunk.(*HistogramChunk).GetCounterResetHeader())
	require.NotNil(t, newAppender)
	require.Equal(t, hApp, newAppender)
	assertSampleCount(t, currChunk, 2, ValHistogram)
}

func assertRecodedHistogramChunkOnAppend(t *testing.T, prevChunk Chunk, hApp *HistogramAppender, ts int64, h *histogram.Histogram, expectHeader CounterResetHeader) {
	prevChunkBytes := prevChunk.Bytes()
	newChunk, recoded, newAppender, err := hApp.AppendHistogram(nil, ts, h, false)
	require.Equal(t, prevChunkBytes, prevChunk.Bytes()) // Sanity check that previous chunk is untouched. This may change in the future if we implement in-place recoding.
	require.NoError(t, err)
	require.NotNil(t, newChunk)
	require.True(t, recoded)
	require.NotEqual(t, prevChunk, newChunk)
	require.Equal(t, expectHeader, newChunk.(*HistogramChunk).GetCounterResetHeader())
	require.NotNil(t, newAppender)
	require.NotEqual(t, hApp, newAppender)
	assertSampleCount(t, newChunk, 2, ValHistogram)
}

func assertSampleCount(t *testing.T, c Chunk, exp int64, vtype ValueType) {
	count := int64(0)
	it := c.Iterator(nil)
	require.NoError(t, it.Err())
	for it.Next() == vtype {
		count++
	}
	require.NoError(t, it.Err())
	require.Equal(t, exp, count)
}

func TestHistogramChunkAppendableWithEmptySpan(t *testing.T) {
	tests := map[string]struct {
		h1 *histogram.Histogram
		h2 *histogram.Histogram
	}{
		"empty span in old and new histogram": {
			h1: &histogram.Histogram{
				Schema:        0,
				Count:         21,
				Sum:           1234.5,
				ZeroThreshold: 0.001,
				ZeroCount:     4,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 1, -1, 1, 0, 0, 0},
			},
			h2: &histogram.Histogram{
				Schema:        0,
				Count:         37,
				Sum:           2345.6,
				ZeroThreshold: 0.001,
				ZeroCount:     5,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
			},
		},
		"empty span in old histogram": {
			h1: &histogram.Histogram{
				Schema:        0,
				Count:         21,
				Sum:           1234.5,
				ZeroThreshold: 0.001,
				ZeroCount:     4,
				PositiveSpans: []histogram.Span{
					{Offset: 1, Length: 0}, // This span will disappear.
					{Offset: 2, Length: 4},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 1, -1, 1, 0, 0, 0},
			},
			h2: &histogram.Histogram{
				Schema:        0,
				Count:         37,
				Sum:           2345.6,
				ZeroThreshold: 0.001,
				ZeroCount:     5,
				PositiveSpans: []histogram.Span{
					{Offset: 3, Length: 4},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
			},
		},
		"empty span in new histogram": {
			h1: &histogram.Histogram{
				Schema:        0,
				Count:         21,
				Sum:           1234.5,
				ZeroThreshold: 0.001,
				ZeroCount:     4,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 3, Length: 3},
				},
				PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 1, -1, 1, 0, 0, 0},
			},
			h2: &histogram.Histogram{
				Schema:        0,
				Count:         37,
				Sum:           2345.6,
				ZeroThreshold: 0.001,
				ZeroCount:     5,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 1, Length: 0}, // This span is new.
					{Offset: 2, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
			},
		},
		"two empty spans mixing offsets": {
			h1: &histogram.Histogram{
				Schema:        0,
				Count:         21,
				Sum:           1234.5,
				ZeroThreshold: 0.001,
				ZeroCount:     4,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 0},
					{Offset: 4, Length: 3},
				},
				PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 1, -1, 1, 0, 0, 0},
			},
			h2: &histogram.Histogram{
				Schema:        0,
				Count:         37,
				Sum:           2345.6,
				ZeroThreshold: 0.001,
				ZeroCount:     5,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 3, Length: 0},
					{Offset: 1, Length: 0},
					{Offset: 4, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 1, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 2, Length: 3},
				},
				NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
			},
		},
		"empty span in old and new custom buckets histogram": {
			h1: &histogram.Histogram{
				Schema: histogram.CustomBucketsSchema,
				Count:  7,
				Sum:    1234.5,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
			h2: &histogram.Histogram{
				Schema: histogram.CustomBucketsSchema,
				Count:  10,
				Sum:    2345.6,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := Chunk(NewHistogramChunk())

			// Create fresh appender and add the first histogram.
			app, err := c.Appender()
			require.NoError(t, err)
			require.Equal(t, 0, c.NumSamples())

			_, _, _, err = app.AppendHistogram(nil, 1, tc.h1, true)
			require.NoError(t, err)
			require.Equal(t, 1, c.NumSamples())
			hApp, _ := app.(*HistogramAppender)

			pI, nI, bpI, bnI, okToAppend, counterReset := hApp.appendable(tc.h2)
			require.Empty(t, pI)
			require.Empty(t, nI)
			require.Empty(t, bpI)
			require.Empty(t, bnI)
			require.True(t, okToAppend)
			require.Equal(t, NotCounterReset, counterReset)
		})
	}
}

func TestAtFloatHistogram(t *testing.T) {
	input := []histogram.Histogram{
		{
			Schema:        0,
			Count:         21,
			Sum:           1234.5,
			ZeroThreshold: 0.001,
			ZeroCount:     4,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []int64{1, 1, -1, 0, 0, 0, 0},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []int64{1, 1, -1, 1, 0, 0, 0},
		},
		{
			Schema:        0,
			Count:         36,
			Sum:           2345.6,
			ZeroThreshold: 0.001,
			ZeroCount:     5,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []int64{1, 3, -2, 5, -2, 0, -3},
		},
		{
			Schema:        0,
			Count:         36,
			Sum:           1111.1,
			ZeroThreshold: 0.001,
			ZeroCount:     5,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []int64{1, 2, -2, 2, -1, 0, 0},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []int64{1, 3, -2, 5, -1, 0, -3},
		},
	}

	expOutput := []*histogram.FloatHistogram{
		{
			Schema:        0,
			Count:         21,
			Sum:           1234.5,
			ZeroThreshold: 0.001,
			ZeroCount:     4,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []float64{1, 2, 1, 1, 1, 1, 1},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []float64{1, 2, 1, 2, 2, 2, 2},
		},
		{
			CounterResetHint: histogram.NotCounterReset,
			Schema:           0,
			Count:            36,
			Sum:              2345.6,
			ZeroThreshold:    0.001,
			ZeroCount:        5,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []float64{1, 3, 1, 2, 1, 1, 1},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []float64{1, 4, 2, 7, 5, 5, 2},
		},
		{
			CounterResetHint: histogram.NotCounterReset,
			Schema:           0,
			Count:            36,
			Sum:              1111.1,
			ZeroThreshold:    0.001,
			ZeroCount:        5,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 0, Length: 3},
			},
			PositiveBuckets: []float64{1, 3, 1, 3, 2, 2, 2},
			NegativeSpans: []histogram.Span{
				{Offset: 1, Length: 4},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 3},
			},
			NegativeBuckets: []float64{1, 4, 2, 7, 6, 6, 3},
		},
	}

	chk := NewHistogramChunk()
	app, err := chk.Appender()
	require.NoError(t, err)
	for i := range input {
		newc, _, _, err := app.AppendHistogram(nil, int64(i), &input[i], false)
		require.NoError(t, err)
		require.Nil(t, newc)
	}
	it := chk.Iterator(nil)
	i := int64(0)
	for it.Next() != ValNone {
		ts, h := it.AtFloatHistogram(nil)
		require.Equal(t, i, ts)
		require.Equal(t, expOutput[i], h, "histogram %d unequal", i)
		i++
	}
}

func TestHistogramChunkAppendableGauge(t *testing.T) {
	eh := &histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Count:            5,
		ZeroCount:        2,
		Sum:              18.4,
		ZeroThreshold:    1e-125,
		Schema:           1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		},
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // {6, 3, 3, 2, 4, 5, 1}
	}

	cbh := &histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Count:            24,
		Sum:              18.4,
		Schema:           histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 2, Length: 1},
			{Offset: 3, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		},
		PositiveBuckets: []int64{6, -3, 0, -1, 2, 1, -4}, // {6, 3, 3, 2, 4, 5, 1}
		CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}

	setup := func(h *histogram.Histogram) (Chunk, *HistogramAppender, int64, *histogram.Histogram) {
		c := Chunk(NewHistogramChunk())

		// Create fresh appender and add the first histogram.
		app, err := c.Appender()
		require.NoError(t, err)
		require.Equal(t, 0, c.NumSamples())

		ts := int64(1234567890)

		chk, _, app, err := app.AppendHistogram(nil, ts, h.Copy(), false)
		require.NoError(t, err)
		require.Nil(t, chk)
		require.Equal(t, 1, c.NumSamples())
		require.Equal(t, GaugeType, c.(*HistogramChunk).GetCounterResetHeader())

		return c, app.(*HistogramAppender), ts, h
	}

	{ // Schema change.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Schema++
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType, histogram.GaugeType)
	}

	{ // Zero threshold change.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.ZeroThreshold += 0.1
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType, histogram.GaugeType)
	}

	{ // New histogram that has more buckets.
		c, hApp, ts, h1 := setup(eh)
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
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // {7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 1}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.NotEmpty(t, pI)
		require.Empty(t, nI)
		require.Empty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // New histogram that has buckets missing.
		c, hApp, ts, h1 := setup(eh)
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
		h2.PositiveBuckets = []int64{6, -3, 0, -1, 3, -4} // {6, 3, 3, 2, 5, 1}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.Empty(t, pI)
		require.Empty(t, nI)
		require.NotEmpty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // New histogram that has a bucket missing and new buckets.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 5, Length: 2},
			{Offset: 3, Length: 1},
			{Offset: 1, Length: 1},
		}
		h2.Sum = 21
		h2.PositiveBuckets = []int64{6, -3, -1, 2, 1, -4} // {6, 3, 2, 4, 5, 1}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.NotEmpty(t, pI)
		require.NotEmpty(t, pBackwardI)
		require.Empty(t, nI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // New histogram that has a counter reset while buckets are same.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.Sum = 23
		h2.PositiveBuckets = []int64{6, -4, 1, -1, 2, 1, -4} // {6, 2, 3, 2, 4, 5, 1}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.Empty(t, pI)
		require.Empty(t, nI)
		require.Empty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // New histogram that has a counter reset while new buckets were added.
		c, hApp, ts, h1 := setup(eh)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Sum = 29
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 0} // {7, 5, 1, 3, 1, 0, 2, 5, 5, 0, 0}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.NotEmpty(t, pI)
		require.Empty(t, nI)
		require.Empty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{
		// New histogram that has a counter reset while new buckets were
		// added before the first bucket and reset on first bucket.
		c, hApp, ts, h1 := setup(eh)
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
		h2.PositiveBuckets = []int64{1, 1, 3, -2, 0, -1, 2, 1, -4} // {1, 2, 5, 3, 3, 2, 4, 5, 1}

		pI, nI, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.NotEmpty(t, pI)
		require.Empty(t, nI)
		require.Empty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok)

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // Custom buckets, no change.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // Custom buckets, increase in bucket counts but no change in layout.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.Count++
		h2.PositiveBuckets = []int64{6, -3, 0, -1, 2, 1, -3}
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // Custom buckets, decrease in bucket counts but no change in layout.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.Count--
		h2.PositiveBuckets = []int64{6, -3, 0, -1, 2, 1, -5}
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.True(t, ok)

		assertNoNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}

	{ // Custom buckets, change only in custom bounds.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.CustomValues = []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}
		_, _, _, _, _, _, ok := hApp.appendableGauge(h2)
		require.False(t, ok)

		assertNewHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType, histogram.GaugeType)
	}

	{ // Custom buckets, with more buckets.
		c, hApp, ts, h1 := setup(cbh)
		h2 := h1.Copy()
		h2.PositiveSpans = []histogram.Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 1},
			{Offset: 1, Length: 4},
			{Offset: 3, Length: 3},
		}
		h2.Count += 6
		h2.Sum = 30
		// Existing histogram should get values converted from the above to:
		//   6 3 0 3 0 0 2 4 5 0 1 (previous values with some new empty buckets in between)
		// so the new histogram should have new counts >= these per-bucket counts, e.g.:
		h2.PositiveBuckets = []int64{7, -2, -4, 2, -2, -1, 2, 3, 0, -5, 1} // 7 5 1 3 1 0 2 5 5 0 1 (total 30)

		posInterjections, negInterjections, pBackwardI, nBackwardI, _, _, ok := hApp.appendableGauge(h2)
		require.NotEmpty(t, posInterjections)
		require.Empty(t, negInterjections)
		require.Empty(t, pBackwardI)
		require.Empty(t, nBackwardI)
		require.True(t, ok) // Only new buckets came in.

		assertRecodedHistogramChunkOnAppend(t, c, hApp, ts+1, h2, GaugeType)
	}
}

func TestHistogramAppendOnlyErrors(t *testing.T) {
	t.Run("schema change error", func(t *testing.T) {
		c := Chunk(NewHistogramChunk())

		// Create fresh appender and add the first histogram.
		app, err := c.Appender()
		require.NoError(t, err)

		h := tsdbutil.GenerateTestHistogram(0)
		var isRecoded bool
		c, isRecoded, app, err = app.AppendHistogram(nil, 1, h, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.NoError(t, err)

		// Add erroring histogram.
		h2 := h.Copy()
		h2.Schema++
		c, isRecoded, _, err = app.AppendHistogram(nil, 2, h2, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.EqualError(t, err, "histogram schema change")
	})
	t.Run("counter reset error", func(t *testing.T) {
		c := Chunk(NewHistogramChunk())

		// Create fresh appender and add the first histogram.
		app, err := c.Appender()
		require.NoError(t, err)

		h := tsdbutil.GenerateTestHistogram(0)
		var isRecoded bool
		c, isRecoded, app, err = app.AppendHistogram(nil, 1, h, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.NoError(t, err)

		// Add erroring histogram.
		h2 := h.Copy()
		h2.CounterResetHint = histogram.CounterReset
		c, isRecoded, _, err = app.AppendHistogram(nil, 2, h2, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.EqualError(t, err, "histogram counter reset")
	})
	t.Run("counter reset error with custom buckets", func(t *testing.T) {
		c := Chunk(NewHistogramChunk())

		// Create fresh appender and add the first histogram.
		app, err := c.Appender()
		require.NoError(t, err)

		h := tsdbutil.GenerateTestCustomBucketsHistogram(0)
		var isRecoded bool
		c, isRecoded, app, err = app.AppendHistogram(nil, 1, h, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.NoError(t, err)

		// Add erroring histogram.
		h2 := h.Copy()
		h2.CustomValues = []float64{0, 1, 2, 3, 4, 5, 6, 7}
		c, isRecoded, _, err = app.AppendHistogram(nil, 2, h2, true)
		require.Nil(t, c)
		require.False(t, isRecoded)
		require.EqualError(t, err, "histogram counter reset")
	})
}

func TestHistogramUniqueSpansAfterNextWithAtHistogram(t *testing.T) {
	// Create two histograms with the same schema and spans.
	h1 := &histogram.Histogram{
		Schema:        1,
		ZeroThreshold: 1e-100,
		Count:         10,
		ZeroCount:     2,
		Sum:           15.0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4},
		NegativeSpans: []histogram.Span{
			{Offset: 1, Length: 1},
		},
		NegativeBuckets: []int64{2},
	}

	h2 := h1.Copy()

	// Create a chunk and append both histograms.
	c := NewHistogramChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 0, h1, false)
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 1, h2, false)
	require.NoError(t, err)

	// Create an iterator and advance to the first histogram.
	it := c.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, rh1 := it.AtHistogram(nil)

	// Advance to the second histogram and retrieve it.
	require.Equal(t, ValHistogram, it.Next())
	_, rh2 := it.AtHistogram(nil)

	require.Equal(t, rh1.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh1.NegativeSpans, h1.NegativeSpans, "Returned negative spans are as expected")
	require.Equal(t, rh2.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh2.NegativeSpans, h1.NegativeSpans, "Returned negative spans are as expected")

	// Check that the spans for h1 and h2 are unique slices.
	require.NotSame(t, &rh1.PositiveSpans[0], &rh2.PositiveSpans[0], "PositiveSpans should be unique between histograms")
	require.NotSame(t, &rh1.NegativeSpans[0], &rh2.NegativeSpans[0], "NegativeSpans should be unique between histograms")
}

func TestHistogramUniqueSpansAfterNextWithAtFloatHistogram(t *testing.T) {
	// Create two histograms with the same schema and spans.
	h1 := &histogram.Histogram{
		Schema:        1,
		ZeroThreshold: 1e-100,
		Count:         10,
		ZeroCount:     2,
		Sum:           15.0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4},
		NegativeSpans: []histogram.Span{
			{Offset: 1, Length: 1},
		},
		NegativeBuckets: []int64{2},
	}

	h2 := h1.Copy()

	// Create a chunk and append both histograms.
	c := NewHistogramChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 0, h1, false)
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 1, h2, false)
	require.NoError(t, err)

	// Create an iterator and advance to the first histogram.
	it := c.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, rh1 := it.AtFloatHistogram(nil)

	// Advance to the second histogram and retrieve it.
	require.Equal(t, ValHistogram, it.Next())
	_, rh2 := it.AtFloatHistogram(nil)

	require.Equal(t, rh1.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh1.NegativeSpans, h1.NegativeSpans, "Returned negative spans are as expected")
	require.Equal(t, rh2.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh2.NegativeSpans, h1.NegativeSpans, "Returned negative spans are as expected")

	// Check that the spans for h1 and h2 are unique slices.
	require.NotSame(t, &rh1.PositiveSpans[0], &rh2.PositiveSpans[0], "PositiveSpans should be unique between histograms")
	require.NotSame(t, &rh1.NegativeSpans[0], &rh2.NegativeSpans[0], "NegativeSpans should be unique between histograms")
}

func TestHistogramCustomValuesInternedAfterNextWithAtHistogram(t *testing.T) {
	// Create two histograms with the same schema and custom values.
	h1 := &histogram.Histogram{
		Schema: -53,
		Count:  10,
		Sum:    15.0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4},
		CustomValues:    []float64{10, 11, 12, 13},
	}

	h2 := h1.Copy()

	// Create a chunk and append both histograms.
	c := NewHistogramChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 0, h1, false)
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 1, h2, false)
	require.NoError(t, err)

	// Create an iterator and advance to the first histogram.
	it := c.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, rh1 := it.AtHistogram(nil)

	// Advance to the second histogram and retrieve it.
	require.Equal(t, ValHistogram, it.Next())
	_, rh2 := it.AtHistogram(nil)

	require.Equal(t, rh1.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh1.CustomValues, h1.CustomValues, "Returned custom values are as expected")
	require.Equal(t, rh2.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh2.CustomValues, h1.CustomValues, "Returned custom values are as expected")

	// Check that the spans and custom values for h1 and h2 are unique slices.
	require.NotSame(t, &rh1.PositiveSpans[0], &rh2.PositiveSpans[0], "PositiveSpans should be unique between histograms")
	require.Same(t, &rh1.CustomValues[0], &rh2.CustomValues[0], "CustomValues should be unique between histograms")
}

func TestHistogramCustomValuesInternedAfterNextWithAtFloatHistogram(t *testing.T) {
	// Create two histograms with the same schema and custom values.
	h1 := &histogram.Histogram{
		Schema: -53,
		Count:  10,
		Sum:    15.0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4},
		CustomValues:    []float64{10, 11, 12, 13},
	}

	h2 := h1.Copy()

	// Create a chunk and append both histograms.
	c := NewHistogramChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 0, h1, false)
	require.NoError(t, err)

	_, _, _, err = app.AppendHistogram(nil, 1, h2, false)
	require.NoError(t, err)

	// Create an iterator and advance to the first histogram.
	it := c.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, rh1 := it.AtFloatHistogram(nil)

	// Advance to the second histogram and retrieve it.
	require.Equal(t, ValHistogram, it.Next())
	_, rh2 := it.AtFloatHistogram(nil)

	require.Equal(t, rh1.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh1.CustomValues, h1.CustomValues, "Returned custom values are as expected")
	require.Equal(t, rh2.PositiveSpans, h1.PositiveSpans, "Returned positive spans are as expected")
	require.Equal(t, rh2.CustomValues, h1.CustomValues, "Returned custom values are as expected")

	// Check that the spans and custom values for h1 and h2 are unique slices.
	require.NotSame(t, &rh1.PositiveSpans[0], &rh2.PositiveSpans[0], "PositiveSpans should be unique between histograms")
	require.Same(t, &rh1.CustomValues[0], &rh2.CustomValues[0], "CustomValues should be unique between histograms")
}

func BenchmarkAppendable(b *testing.B) {
	// Create a histogram with a bunch of spans and buckets.
	const (
		numSpans   = 1000
		spanLength = 10
	)
	h := &histogram.Histogram{
		Schema:        0,
		Count:         100,
		Sum:           1000,
		ZeroThreshold: 0.001,
		ZeroCount:     5,
	}
	for range numSpans {
		h.PositiveSpans = append(h.PositiveSpans, histogram.Span{Offset: 5, Length: spanLength})
		h.NegativeSpans = append(h.NegativeSpans, histogram.Span{Offset: 5, Length: spanLength})
		for j := range spanLength {
			h.PositiveBuckets = append(h.PositiveBuckets, int64(j))
			h.NegativeBuckets = append(h.NegativeBuckets, int64(j))
		}
	}

	c := Chunk(NewHistogramChunk())

	// Create fresh appender and add the first histogram.
	app, err := c.Appender()
	if err != nil {
		b.Fatal(err)
	}

	_, _, _, err = app.AppendHistogram(nil, 1, h, true)
	if err != nil {
		b.Fatal(err)
	}

	hApp := app.(*HistogramAppender)

	isAppendable := true
	for b.Loop() {
		_, _, _, _, ok, _ := hApp.appendable(h)
		isAppendable = isAppendable && ok
	}
	if !isAppendable {
		b.Fail()
	}
}

func assertFirstIntHistogramSampleHint(t *testing.T, chunk Chunk, expected histogram.CounterResetHint) {
	it := chunk.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, v := it.AtHistogram(nil)
	require.Equal(t, expected, v.CounterResetHint)
}

func TestIntHistogramEmptyBucketsWithGaps(t *testing.T) {
	h1 := &histogram.Histogram{
		PositiveSpans: []histogram.Span{
			{Offset: -19, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{0, 0, 0, 0},
	}
	require.NoError(t, h1.Validate())

	c := NewHistogramChunk()
	app, err := c.Appender()
	require.NoError(t, err)
	_, _, _, err = app.AppendHistogram(nil, 1, h1, false)
	require.NoError(t, err)

	h2 := &histogram.Histogram{
		PositiveSpans: []histogram.Span{
			{Offset: -19, Length: 1},
			{Offset: 4, Length: 1},
			{Offset: 3, Length: 1},
		},
		PositiveBuckets: []int64{0, 0, 0},
	}
	require.NoError(t, h2.Validate())

	newC, recoded, _, err := app.AppendHistogram(nil, 2, h2, false)
	require.NoError(t, err)
	require.True(t, recoded)
	require.NotNil(t, newC)

	it := newC.Iterator(nil)
	require.Equal(t, ValHistogram, it.Next())
	_, h := it.AtFloatHistogram(nil)
	require.NoError(t, h.Validate())
	require.Equal(t, ValHistogram, it.Next())
	_, h = it.AtFloatHistogram(nil)
	require.NoError(t, h.Validate())
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}

func TestHistogramIteratorFailIfSchemaInValid(t *testing.T) {
	for _, schema := range []int32{-101, 101} {
		t.Run(fmt.Sprintf("schema %d", schema), func(t *testing.T) {
			h := &histogram.Histogram{
				Schema:        schema,
				Count:         10,
				Sum:           15.0,
				ZeroThreshold: 1e-100,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 2, 3, 4},
			}

			c := NewHistogramChunk()
			app, err := c.Appender()
			require.NoError(t, err)

			_, _, _, err = app.AppendHistogram(nil, 1, h, false)
			require.NoError(t, err)

			it := c.Iterator(nil)
			require.Equal(t, ValNone, it.Next())
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)
		})
	}
}

func TestHistogramIteratorReduceSchema(t *testing.T) {
	for _, schema := range []int32{9, 52} {
		t.Run(fmt.Sprintf("schema %d", schema), func(t *testing.T) {
			h := &histogram.Histogram{
				Schema:        schema,
				Count:         10,
				Sum:           15.0,
				ZeroThreshold: 1e-100,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 2, 3, 4},
			}

			c := NewHistogramChunk()
			app, err := c.Appender()
			require.NoError(t, err)

			_, _, _, err = app.AppendHistogram(nil, 1, h, false)
			require.NoError(t, err)

			it := c.Iterator(nil)
			require.Equal(t, ValHistogram, it.Next())
			_, rh := it.AtHistogram(nil)
			require.Equal(t, histogram.ExponentialSchemaMax, rh.Schema)

			_, rfh := it.AtFloatHistogram(nil)
			require.Equal(t, histogram.ExponentialSchemaMax, rfh.Schema)
		})
	}
}
