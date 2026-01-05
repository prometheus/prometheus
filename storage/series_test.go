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

package storage

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestListSeriesIterator(t *testing.T) {
	it := NewListSeriesIterator(samples{
		fSample{0, 0},
		fSample{1, 1},
		fSample{1, 1.5},
		fSample{2, 2},
		fSample{3, 3},
	})

	// Seek to the first sample with ts=1.
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	ts, v := it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1., v)

	// Seek one further, next sample still has ts=1.
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1.5, v)

	// Seek again to 1 and make sure we stay where we are.
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	ts, v = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1.5, v)

	// Another seek.
	require.Equal(t, chunkenc.ValFloat, it.Seek(3))
	ts, v = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, 3., v)

	// And we don't go back.
	require.Equal(t, chunkenc.ValFloat, it.Seek(2))
	ts, v = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, 3., v)

	// Seek beyond the end.
	require.Equal(t, chunkenc.ValNone, it.Seek(5))
	// And we don't go back. (This exposes issue #10027.)
	require.Equal(t, chunkenc.ValNone, it.Seek(2))
}

// TestChunkSeriesSetToSeriesSet test the property of SeriesSet that says
// returned series should be iterable even after Next is called.
func TestChunkSeriesSetToSeriesSet(t *testing.T) {
	series := []struct {
		lbs     labels.Labels
		samples []chunks.Sample
	}{
		{
			lbs: labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []chunks.Sample{
				fSample{t: 1, f: 1},
				fSample{t: 2, f: 2},
				fSample{t: 3, f: 3},
				fSample{t: 4, f: 4},
			},
		}, {
			lbs: labels.FromStrings("__name__", "up", "instance", "localhost:8081"),
			samples: []chunks.Sample{
				fSample{t: 1, f: 2},
				fSample{t: 2, f: 3},
				fSample{t: 3, f: 4},
				fSample{t: 4, f: 5},
				fSample{t: 5, f: 6},
				fSample{t: 6, f: 7},
			},
		},
	}
	var chunkSeries []ChunkSeries
	for _, s := range series {
		chunkSeries = append(chunkSeries, NewListChunkSeriesFromSamples(s.lbs, s.samples))
	}
	css := NewMockChunkSeriesSet(chunkSeries...)

	ss := NewSeriesSetFromChunkSeriesSet(css)
	var ssSlice []Series
	for ss.Next() {
		ssSlice = append(ssSlice, ss.At())
	}
	require.Len(t, ssSlice, 2)
	var iter chunkenc.Iterator
	for i, s := range ssSlice {
		require.Equal(t, series[i].lbs, s.Labels())
		iter = s.Iterator(iter)
		j := 0
		for iter.Next() == chunkenc.ValFloat {
			ts, v := iter.At()
			require.EqualValues(t, fSample{t: ts, f: v}, series[i].samples[j])
			j++
		}
	}
}

type histogramTest struct {
	samples                     []chunks.Sample
	expectedSamples             []chunks.Sample
	expectedCounterResetHeaders []chunkenc.CounterResetHeader
}

func TestHistogramSeriesToChunks(t *testing.T) {
	h1 := &histogram.Histogram{
		Count:         7,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{2, 1}, // Abs: 2, 3
	}
	// h1 but with an extra empty bucket at offset -10.
	// This can happen if h1 is from a recoded chunk, where a later histogram had a bucket at offset -10.
	h1ExtraBuckets := &histogram.Histogram{
		Count:         7,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: -10, Length: 1},
			{Offset: 9, Length: 2},
		},
		PositiveBuckets: []int64{0, 2, 1}, // Abs: 0, 2, 3
	}
	h1Recoded := &histogram.Histogram{
		Count:         7,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -3, 0}, // Abs: 2, 3, 0, 0
	}
	// Appendable to h1.
	h2 := &histogram.Histogram{
		Count:         12,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -2, 3}, // Abs: 2, 3, 1, 4
	}
	// Implicit counter reset by reduction in buckets, not appendable.
	h2down := &histogram.Histogram{
		Count:         10,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 3}, // Abs: 1, 2, 1, 4
	}

	fh1 := &histogram.FloatHistogram{
		Count:         6,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []float64{3, 1},
	}
	// fh1 but with an extra empty bucket at offset -10.
	// This can happen if fh1 is from a recoded chunk, where a later histogram had a bucket at offset -10.
	fh1ExtraBuckets := &histogram.FloatHistogram{
		Count:         6,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: -10, Length: 1},
			{Offset: 9, Length: 2},
		},
		PositiveBuckets: []float64{0, 3, 1},
	}
	fh1Recoded := &histogram.FloatHistogram{
		Count:         6,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{3, 1, 0, 0},
	}
	// Appendable to fh1.
	fh2 := &histogram.FloatHistogram{
		Count:         17,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{4, 2, 7, 2},
	}
	// Implicit counter reset by reduction in buckets, not appendable.
	fh2down := &histogram.FloatHistogram{
		Count:         15,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100,
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{2, 2, 7, 2},
	}

	// Gauge histogram.
	gh1 := &histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Count:            7,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{2, 1}, // Abs: 2, 3
	}
	// gh1 recoded to add extra empty buckets at end.
	gh1Recoded := &histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Count:            7,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -3, 0}, // Abs: 2, 3, 0, 0
	}
	gh2 := &histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Count:            12,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -2, 3}, // Abs: 2, 3, 1, 4
	}

	// Float gauge histogram.
	gfh1 := &histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Count:            6,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []float64{3, 1},
	}
	// gfh1 recoded to add an extra empty buckets at end.
	gfh1Recoded := &histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Count:            6,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{3, 1, 0, 0},
	}
	gfh2 := &histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Count:            17,
		ZeroCount:        2,
		ZeroThreshold:    0.001,
		Sum:              100,
		Schema:           0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{4, 2, 7, 2},
	}

	staleHistogram := &histogram.Histogram{
		Sum: math.Float64frombits(value.StaleNaN),
	}
	staleFloatHistogram := &histogram.FloatHistogram{
		Sum: math.Float64frombits(value.StaleNaN),
	}

	tests := map[string]histogramTest{
		"single histogram to single chunk": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
		"two histograms encoded to a single chunk": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1},
				hSample{t: 2, h: h2},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1Recoded},
				hSample{t: 2, h: h2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
		"two histograms encoded to two chunks": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h2},
				hSample{t: 2, h: h1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h2},
				hSample{t: 2, h: h1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.CounterReset},
		},
		"histogram and stale sample encoded to two chunks": {
			samples: []chunks.Sample{
				hSample{t: 1, h: staleHistogram},
				hSample{t: 2, h: h1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: staleHistogram},
				hSample{t: 2, h: h1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.UnknownCounterReset},
		},
		"histogram and reduction in bucket encoded to two chunks": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1},
				hSample{t: 2, h: h2down},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1},
				hSample{t: 2, h: h2down},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.CounterReset},
		},
		// Float histograms.
		"single float histogram to single chunk": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
		"two float histograms encoded to a single chunk": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
				fhSample{t: 2, fh: fh2},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh1Recoded},
				fhSample{t: 2, fh: fh2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
		"two float histograms encoded to two chunks": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh2},
				fhSample{t: 2, fh: fh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh2},
				fhSample{t: 2, fh: fh1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.CounterReset},
		},
		"float histogram and stale sample encoded to two chunks": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: staleFloatHistogram},
				fhSample{t: 2, fh: fh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: staleFloatHistogram},
				fhSample{t: 2, fh: fh1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.UnknownCounterReset},
		},
		"float histogram and reduction in bucket encoded to two chunks": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
				fhSample{t: 2, fh: fh2down},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
				fhSample{t: 2, fh: fh2down},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.CounterReset},
		},
		// Mixed.
		"histogram and float histogram encoded to two chunks": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1},
				fhSample{t: 2, fh: fh2},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1},
				fhSample{t: 2, fh: fh2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.UnknownCounterReset},
		},
		"float histogram and histogram encoded to two chunks": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
				hSample{t: 2, h: h2},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh1},
				hSample{t: 2, h: h2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.UnknownCounterReset},
		},
		"histogram and stale float histogram encoded to two chunks": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1},
				fhSample{t: 2, fh: staleFloatHistogram},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1},
				fhSample{t: 2, fh: staleFloatHistogram},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset, chunkenc.UnknownCounterReset},
		},
		"single gauge histogram encoded to one chunk": {
			samples: []chunks.Sample{
				hSample{t: 1, h: gh1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: gh1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"two gauge histograms encoded to one chunk when counter increases": {
			samples: []chunks.Sample{
				hSample{t: 1, h: gh1},
				hSample{t: 2, h: gh2},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: gh1Recoded},
				hSample{t: 2, h: gh2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"two gauge histograms encoded to one chunk when counter decreases": {
			samples: []chunks.Sample{
				hSample{t: 1, h: gh2},
				hSample{t: 2, h: gh1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: gh2},
				hSample{t: 2, h: gh1Recoded},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"single gauge float histogram encoded to one chunk": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: gfh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: gfh1},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"two float gauge histograms encoded to one chunk when counter increases": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: gfh1},
				fhSample{t: 2, fh: gfh2},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: gfh1Recoded},
				fhSample{t: 2, fh: gfh2},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"two float gauge histograms encoded to one chunk when counter decreases": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: gfh2},
				fhSample{t: 2, fh: gfh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: gfh2},
				fhSample{t: 2, fh: gfh1Recoded},
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.GaugeType},
		},
		"histogram with extra empty bucket followed by histogram encodes to one chunk": {
			samples: []chunks.Sample{
				hSample{t: 1, h: h1ExtraBuckets},
				hSample{t: 2, h: h1},
			},
			expectedSamples: []chunks.Sample{
				hSample{t: 1, h: h1ExtraBuckets},
				hSample{t: 2, h: h1ExtraBuckets}, // Recoded to add the missing buckets.
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
		"float histogram with extra empty bucket followed by float histogram encodes to one chunk": {
			samples: []chunks.Sample{
				fhSample{t: 1, fh: fh1ExtraBuckets},
				fhSample{t: 2, fh: fh1},
			},
			expectedSamples: []chunks.Sample{
				fhSample{t: 1, fh: fh1ExtraBuckets},
				fhSample{t: 2, fh: fh1ExtraBuckets}, // Recoded to add the missing buckets.
			},
			expectedCounterResetHeaders: []chunkenc.CounterResetHeader{chunkenc.UnknownCounterReset},
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			testHistogramsSeriesToChunks(t, test)
		})
	}
}

func testHistogramsSeriesToChunks(t *testing.T, test histogramTest) {
	lbs := labels.FromStrings("__name__", "up", "instance", "localhost:8080")
	copiedSamples := []chunks.Sample{}
	for _, s := range test.samples {
		switch cs := s.(type) {
		case hSample:
			copiedSamples = append(copiedSamples, hSample{t: cs.t, h: cs.h.Copy()})
		case fhSample:
			copiedSamples = append(copiedSamples, fhSample{t: cs.t, fh: cs.fh.Copy()})
		default:
			t.Error("internal error, unexpected type")
		}
	}
	series := NewListSeries(lbs, copiedSamples)
	encoder := NewSeriesToChunkEncoder(series)
	require.Equal(t, lbs, encoder.Labels())

	chks, err := ExpandChunks(encoder.Iterator(nil))
	require.NoError(t, err)
	require.Len(t, chks, len(test.expectedCounterResetHeaders))

	// Decode all encoded samples and assert they are equal to the original ones.
	encodedSamples := chunks.ChunkMetasToSamples(chks)
	require.Len(t, encodedSamples, len(test.expectedSamples))

	for i, s := range test.expectedSamples {
		encodedSample := encodedSamples[i]
		switch expectedSample := s.(type) {
		case hSample:
			require.Equalf(t, chunkenc.ValHistogram, encodedSample.Type(), "expect histogram at idx %d", i)
			h := encodedSample.H()
			// Ignore counter reset if not gauge here, will check on chunk level.
			if expectedSample.h.CounterResetHint != histogram.GaugeType {
				h.CounterResetHint = histogram.UnknownCounterReset
			}
			if value.IsStaleNaN(expectedSample.h.Sum) {
				require.Truef(t, value.IsStaleNaN(h.Sum), "at idx %d", i)
				continue
			}
			require.Equalf(t, *expectedSample.h, *h, "at idx %d", i)
		case fhSample:
			require.Equalf(t, chunkenc.ValFloatHistogram, encodedSample.Type(), "expect float histogram at idx %d", i)
			fh := encodedSample.FH()
			// Ignore counter reset if not gauge here, will check on chunk level.
			if expectedSample.fh.CounterResetHint != histogram.GaugeType {
				fh.CounterResetHint = histogram.UnknownCounterReset
			}
			if value.IsStaleNaN(expectedSample.fh.Sum) {
				require.Truef(t, value.IsStaleNaN(fh.Sum), "at idx %d", i)
				continue
			}
			require.Equalf(t, *expectedSample.fh, *fh, "at idx %d", i)
		default:
			t.Error("internal error, unexpected type")
		}
	}

	for i, expectedCounterResetHint := range test.expectedCounterResetHeaders {
		require.Equalf(t, expectedCounterResetHint, getCounterResetHint(chks[i]), "chunk at index %d", i)
	}
}

func getCounterResetHint(chunk chunks.Meta) chunkenc.CounterResetHeader {
	switch chk := chunk.Chunk.(type) {
	case *chunkenc.HistogramChunk:
		return chk.GetCounterResetHeader()
	case *chunkenc.FloatHistogramChunk:
		return chk.GetCounterResetHeader()
	}
	return chunkenc.UnknownCounterReset
}
