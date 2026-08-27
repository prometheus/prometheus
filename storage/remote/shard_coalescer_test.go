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

package remote

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestShardCoalescer_SampleFirstArrival(t *testing.T) {
	var dropped atomic.Int64
	coalescer := newShardCoalescer(2048, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	ref := chunks.HeadSeriesRef(100)
	ts := int64(1000)

	batch := []timeSeries{
		{
			seriesRef: ref,
			timestamp: ts,
			sType:     tSample,
			value:     42.0,
		},
	}

	ex := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "abc123"),
		Value:  42.0,
		Ts:     ts + 10, // within 50ms window
		HasTs:  true,
	}

	attached := coalescer.TryAttachToBatch(batch, ref, ex)
	require.True(t, attached)
	require.Len(t, batch[0].exemplars, 1)
	require.Equal(t, ex, batch[0].exemplars[0])
	require.Equal(t, 0, coalescer.PendingCount())
	require.Equal(t, int64(0), dropped.Load())
}

func TestShardCoalescer_ExemplarFirstArrival(t *testing.T) {
	var dropped atomic.Int64
	coalescer := newShardCoalescer(2048, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	ref := chunks.HeadSeriesRef(200)
	ts := int64(1000)

	ex := exemplar.Exemplar{
		Labels: labels.FromStrings("span_id", "xyz789"),
		Value:  15.5,
		Ts:     ts,
		HasTs:  true,
	}

	// No batch item exists yet
	var batch []timeSeries
	attached := coalescer.TryAttachToBatch(batch, ref, ex)
	require.False(t, attached)

	coalescer.AddPendingExemplar(ref, ex)
	require.Equal(t, 1, coalescer.PendingCount())

	// Sample arrives within 50ms
	matched := coalescer.TryAttachMatchingExemplars(ref, ts+25)
	require.Len(t, matched, 1)
	require.Equal(t, ex, matched[0])
	require.Equal(t, 0, coalescer.PendingCount())
	require.Equal(t, int64(0), dropped.Load())

	// Second sample for same ref should not find anything
	matchedAgain := coalescer.TryAttachMatchingExemplars(ref, ts+25)
	require.Nil(t, matchedAgain)
}

func TestShardCoalescer_RingBufferWrapAround_SlotGenerations(t *testing.T) {
	var dropped atomic.Int64
	// Small ring buffer to test wrap-around easily
	capacity := 4
	coalescer := newShardCoalescer(capacity, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	// Fill buffer completely (slots 0..3)
	for i := 1; i <= 4; i++ {
		ex := exemplar.Exemplar{
			Labels: labels.FromStrings("idx", strconv.Itoa(i)),
			Value:  float64(i),
			Ts:     1000,
			HasTs:  true,
		}
		coalescer.AddPendingExemplar(chunks.HeadSeriesRef(i), ex)
	}
	require.Equal(t, 4, coalescer.PendingCount())
	require.Equal(t, int64(0), dropped.Load())

	// Overwrite slot 0 with ref 5
	ex5 := exemplar.Exemplar{
		Labels: labels.FromStrings("idx", "5"),
		Value:  5.0,
		Ts:     1000,
		HasTs:  true,
	}
	coalescer.AddPendingExemplar(chunks.HeadSeriesRef(5), ex5)
	require.Equal(t, 4, coalescer.PendingCount())
	require.Equal(t, int64(1), dropped.Load(), "overwritten slot 0 (ref 1) must be dropped")

	// Sample for ref 1 arrives - should NOT match ref 5 due to generation validation
	matched1 := coalescer.TryAttachMatchingExemplars(chunks.HeadSeriesRef(1), 1000)
	require.Nil(t, matched1, "ref 1 slot was overwritten and generation changed; must return nil")

	// Sample for ref 5 arrives - should match ref 5
	matched5 := coalescer.TryAttachMatchingExemplars(chunks.HeadSeriesRef(5), 1000)
	require.Len(t, matched5, 1)
	require.Equal(t, ex5, matched5[0])

	// Other slots (refs 2, 3, 4) should still be valid
	for i := 2; i <= 4; i++ {
		matched := coalescer.TryAttachMatchingExemplars(chunks.HeadSeriesRef(i), 1000)
		require.Len(t, matched, 1)
		require.Equal(t, float64(i), matched[0].Value)
	}
	require.Equal(t, 0, coalescer.PendingCount())
}

func TestShardCoalescer_RejectCrossScrapeExemplarMatching(t *testing.T) {
	var dropped atomic.Int64
	coalescer := newShardCoalescer(2048, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	ref := chunks.HeadSeriesRef(300)

	// Case 1: Exemplar-first, sample arrives > 50ms later
	ex1 := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "old"),
		Value:  1.0,
		Ts:     1000,
		HasTs:  true,
	}
	coalescer.AddPendingExemplar(ref, ex1)

	// Sample arrives at T=1051 (51ms later > 50ms)
	matched := coalescer.TryAttachMatchingExemplars(ref, 1051)
	require.Nil(t, matched, "should reject matching with delta > 50ms")
	require.Equal(t, int64(1), dropped.Load(), "stale exemplar must be dropped")
	require.Equal(t, 0, coalescer.PendingCount())

	// Case 2: Exact boundary at 50ms matches
	ref2 := chunks.HeadSeriesRef(301)
	ex2 := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "boundary"),
		Value:  2.0,
		Ts:     1000,
		HasTs:  true,
	}
	coalescer.AddPendingExemplar(ref2, ex2)
	matchedBoundary := coalescer.TryAttachMatchingExemplars(ref2, 1050)
	require.Len(t, matchedBoundary, 1, "boundary at exactly 50ms must match")
	require.Equal(t, ex2, matchedBoundary[0])

	// Case 3: Sample-first, exemplar arrives > 50ms later
	batch := []timeSeries{
		{
			seriesRef: chunks.HeadSeriesRef(302),
			timestamp: 1000,
			sType:     tSample,
			value:     3.0,
		},
	}
	ex3 := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "late"),
		Value:  3.0,
		Ts:     1060, // 60ms difference
		HasTs:  true,
	}
	attached := coalescer.TryAttachToBatch(batch, chunks.HeadSeriesRef(302), ex3)
	require.False(t, attached, "should reject attaching exemplar to batch sample with delta > 50ms")
	require.Empty(t, batch[0].exemplars)
}

func TestShardCoalescer_SupportAllMetricTypes(t *testing.T) {
	var dropped atomic.Int64
	coalescer := newShardCoalescer(2048, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	// Float Sample
	batchSample := []timeSeries{
		{
			seriesRef: 401,
			timestamp: 1000,
			sType:     tSample,
			value:     100.0,
		},
	}
	ex1 := exemplar.Exemplar{Labels: labels.FromStrings("type", "sample"), Value: 100.0, Ts: 1000, HasTs: true}
	require.True(t, coalescer.TryAttachToBatch(batchSample, 401, ex1))
	require.Len(t, batchSample[0].exemplars, 1)

	// Int Histogram
	h := &histogram.Histogram{Schema: 1, Count: 10}
	batchHist := []timeSeries{
		{
			seriesRef: 402,
			timestamp: 1000,
			sType:     tHistogram,
			histogram: h,
		},
	}
	ex2 := exemplar.Exemplar{Labels: labels.FromStrings("type", "histogram"), Value: 5.0, Ts: 1000, HasTs: true}
	require.True(t, coalescer.TryAttachToBatch(batchHist, 402, ex2))
	require.Len(t, batchHist[0].exemplars, 1)

	// Float Histogram
	fh := &histogram.FloatHistogram{Schema: 1, Count: 15}
	batchFloatHist := []timeSeries{
		{
			seriesRef:      403,
			timestamp:      1000,
			sType:          tFloatHistogram,
			floatHistogram: fh,
		},
	}
	ex3 := exemplar.Exemplar{Labels: labels.FromStrings("type", "floathistogram"), Value: 7.5, Ts: 1000, HasTs: true}
	require.True(t, coalescer.TryAttachToBatch(batchFloatHist, 403, ex3))
	require.Len(t, batchFloatHist[0].exemplars, 1)

	// Exemplar-first for all 3 types
	coalescer.AddPendingExemplar(404, ex1)
	coalescer.AddPendingExemplar(405, ex2)
	coalescer.AddPendingExemplar(406, ex3)

	m1 := coalescer.TryAttachMatchingExemplars(404, 1000)
	require.Len(t, m1, 1)
	m2 := coalescer.TryAttachMatchingExemplars(405, 1000)
	require.Len(t, m2, 1)
	m3 := coalescer.TryAttachMatchingExemplars(406, 1000)
	require.Len(t, m3, 1)

	require.Equal(t, int64(0), dropped.Load())
	require.Equal(t, 0, coalescer.PendingCount())
}

func TestShardCoalescer_EvictOlderThanAndFlush(t *testing.T) {
	var dropped atomic.Int64
	coalescer := newShardCoalescer(2048, func(e exemplar.Exemplar) {
		dropped.Add(1)
	})

	coalescer.AddPendingExemplar(501, exemplar.Exemplar{Ts: 1000, HasTs: true})
	coalescer.AddPendingExemplar(502, exemplar.Exemplar{Ts: 2000, HasTs: true})
	coalescer.AddPendingExemplar(503, exemplar.Exemplar{Ts: 3000, HasTs: true})
	require.Equal(t, 3, coalescer.PendingCount())

	// Evict older than 2500 (cutoff 2500 - 50 = 2450): Ts 1000 and 2000 will be evicted
	evicted := coalescer.EvictOlderThan(2500)
	require.Equal(t, 2, evicted)
	require.Equal(t, int64(2), dropped.Load())
	require.Equal(t, 1, coalescer.PendingCount())

	// Remaining 503 can be matched
	m := coalescer.TryAttachMatchingExemplars(503, 3000)
	require.Len(t, m, 1)
	require.Equal(t, 0, coalescer.PendingCount())

	// Add new and test FlushAndClear
	coalescer.AddPendingExemplar(504, exemplar.Exemplar{Ts: 4000, HasTs: true})
	coalescer.AddPendingExemplar(505, exemplar.Exemplar{Ts: 4000, HasTs: true})
	require.Equal(t, 2, coalescer.PendingCount())

	flushedDropped := coalescer.FlushAndClear()
	require.Equal(t, 2, flushedDropped)
	require.Equal(t, int64(4), dropped.Load())
	require.Equal(t, 0, coalescer.PendingCount())
}

func BenchmarkShardCoalescer_SampleFirst(b *testing.B) {
	coalescer := newShardCoalescer(2048, nil)
	lbls := labels.FromStrings("trace_id", "1234567890abcdef")
	ex := exemplar.Exemplar{Labels: lbls, Value: 123.45, Ts: 1000, HasTs: true}

	batch := make([]timeSeries, 100)
	for i := range batch {
		batch[i] = timeSeries{
			seriesRef: chunks.HeadSeriesRef(i + 1),
			timestamp: 1000,
			sType:     tSample,
			value:     float64(i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref := chunks.HeadSeriesRef((i % 100) + 1)
		coalescer.TryAttachToBatch(batch, ref, ex)
	}
}

func BenchmarkShardCoalescer_ExemplarFirst(b *testing.B) {
	coalescer := newShardCoalescer(2048, nil)
	lbls := labels.FromStrings("trace_id", "1234567890abcdef")
	ex := exemplar.Exemplar{Labels: lbls, Value: 123.45, Ts: 1000, HasTs: true}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref := chunks.HeadSeriesRef((i % 1000) + 1)
		coalescer.AddPendingExemplar(ref, ex)
		coalescer.TryAttachMatchingExemplars(ref, 1000)
	}
}

func BenchmarkShardCoalescer_RingWrapAround(b *testing.B) {
	coalescer := newShardCoalescer(2048, nil)
	lbls := labels.FromStrings("trace_id", "1234567890abcdef")
	ex := exemplar.Exemplar{Labels: lbls, Value: 123.45, Ts: 1000, HasTs: true}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref := chunks.HeadSeriesRef(i + 1)
		coalescer.AddPendingExemplar(ref, ex)
	}
}
