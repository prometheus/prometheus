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
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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

// TestSeriesSetToChunkSet test the property of SeriesSet that says
// returned series should be iterable even after Next is called.
func TestChunkSeriesSetToSeriesSet(t *testing.T) {
	series := []struct {
		lbs     labels.Labels
		samples []tsdbutil.Sample
	}{
		{
			lbs: labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				fSample{t: 1, f: 1},
				fSample{t: 2, f: 2},
				fSample{t: 3, f: 3},
				fSample{t: 4, f: 4},
			},
		}, {
			lbs: labels.FromStrings("__name__", "up", "instance", "localhost:8081"),
			samples: []tsdbutil.Sample{
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
		require.EqualValues(t, series[i].lbs, s.Labels())
		iter = s.Iterator(iter)
		j := 0
		for iter.Next() == chunkenc.ValFloat {
			ts, v := iter.At()
			require.EqualValues(t, series[i].samples[j], fSample{t: ts, f: v})
			j++
		}
	}
}

type histogramTest struct {
	name                 string
	lbs                  labels.Labels
	samples              []tsdbutil.Sample
	expectedChunks       int
	expectedCounterReset bool
}

func TestHistogramSeriesToChunks(t *testing.T) {
	h1 := &histogram.Histogram{
		Count:         3,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100, // Does not matter.
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
		},
		PositiveBuckets: []int64{2, 1}, // Abs: 2, 3, 1, 4
	}
	h2 := &histogram.Histogram{
		Count:         12,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           100, // Does not matter.
		Schema:        0,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 1, -2, 3}, // Abs: 2, 3, 1, 4
	}
	staleHistogram := &histogram.Histogram{
		Sum: math.Float64frombits(value.StaleNaN),
	}
	tests := []histogramTest{
		{
			name: "single histogram to single chunk",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, h: h1},
			},
			expectedChunks: 1,
		},
		{
			name: "two histograms encoded to a single chunk",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, h: h1},
				&sample{t: 2, h: h2},
			},
			expectedChunks: 1,
		},
		{
			name: "two histograms encoded to two chunks",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, h: h2},
				&sample{t: 2, h: h1},
			},
			expectedChunks:       2,
			expectedCounterReset: true,
		},
		{
			name: "histogram and stale sample encoded to two chunks",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, h: staleHistogram},
				&sample{t: 2, h: h1},
			},
			expectedChunks: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("histograms", func(t *testing.T) {
				testHistogramsSeriesToChunks(t, test)
			})
			t.Run("float_histograms", func(t *testing.T) {
				// Convert all histograms to float histograms.
				for i := range test.samples {
					test.samples[i].(*sample).fh = test.samples[i].H().ToFloat()
					test.samples[i].(*sample).h = nil
				}
				testHistogramsSeriesToChunks(t, test)
			})
		})
	}
	t.Run("mixed_histograms", func(t *testing.T) {
		mixedTests := []histogramTest{{
			name: "histogram and float histogram encoded to two chunks",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, h: h1},
				&sample{t: 2, fh: h2.ToFloat()},
			},
			expectedChunks: 2,
		}, {
			name: "histogram and float histogram encoded to two chunks",
			lbs:  labels.FromStrings("__name__", "up", "instance", "localhost:8080"),
			samples: []tsdbutil.Sample{
				&sample{t: 1, fh: h1.ToFloat()},
				&sample{t: 2, h: h2},
			},
			expectedChunks: 2,
		}}
		for _, test := range mixedTests {
			testHistogramsSeriesToChunks(t, test)
		}
	})
}

func testHistogramsSeriesToChunks(t *testing.T, test histogramTest) {
	series := NewListSeries(test.lbs, test.samples)
	encoder := NewSeriesToChunkEncoder(series)
	require.EqualValues(t, test.lbs, encoder.Labels())

	chks, err := ExpandChunks(encoder.Iterator(nil))
	require.NoError(t, err)
	require.Equal(t, test.expectedChunks, len(chks))

	// Decode all encoded samples and assert they are equal to the original ones.
	encodedSamples := expandHistogramSamples(chks)
	require.Equal(t, len(test.samples), len(encodedSamples))
	for i, encodedSample := range encodedSamples {
		var testSample *histogram.FloatHistogram
		if test.samples[i].H() != nil {
			testSample = test.samples[i].H().ToFloat()
		} else {
			testSample = test.samples[i].FH()
		}

		if value.IsStaleNaN(testSample.Sum) {
			require.True(t, value.IsStaleNaN(encodedSample.Sum))
			continue
		}
		require.True(t, encodedSample.Compact(0).Equals(testSample))
	}

	// If a counter reset hint is expected, it can only be found in the second chunk.
	// Otherwise, we assert an unknown counter reset hint in all chunks.
	if test.expectedCounterReset {
		require.Equal(t, chunkenc.UnknownCounterReset, getCounterResetHint(chks[0]))
		require.Equal(t, chunkenc.CounterReset, getCounterResetHint(chks[1]))
	} else {
		for _, chk := range chks {
			require.Equal(t, chunkenc.UnknownCounterReset, getCounterResetHint(chk))
		}
	}
}

func expandHistogramSamples(chunks []chunks.Meta) []*histogram.FloatHistogram {
	if len(chunks) == 0 {
		return nil
	}

	return append(
		expandHistogramChunk(chunks[0].Chunk.Iterator(nil)),
		expandHistogramSamples(chunks[1:])...,
	)
}

func expandHistogramChunk(it chunkenc.Iterator) []*histogram.FloatHistogram {
	floatHistograms := make([]*histogram.FloatHistogram, 0)
	for it.Next() != chunkenc.ValNone {
		_, fh := it.AtFloatHistogram()
		floatHistograms = append(floatHistograms, fh)
	}
	return floatHistograms
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
