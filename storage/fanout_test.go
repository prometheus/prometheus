// Copyright 2017 The Prometheus Authors
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
	"fmt"
	"math"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMergeStringSlices(t *testing.T) {
	for _, tc := range []struct {
		input    [][]string
		expected []string
	}{
		{},
		{[][]string{{"foo"}}, []string{"foo"}},
		{[][]string{{"foo"}, {"bar"}}, []string{"bar", "foo"}},
		{[][]string{{"foo"}, {"bar"}, {"baz"}}, []string{"bar", "baz", "foo"}},
	} {
		testutil.Equals(t, tc.expected, mergeStringSlices(tc.input))
	}
}

func TestMergeTwoStringSlices(t *testing.T) {
	for _, tc := range []struct {
		a, b, expected []string
	}{
		{[]string{}, []string{}, []string{}},
		{[]string{"foo"}, nil, []string{"foo"}},
		{nil, []string{"bar"}, []string{"bar"}},
		{[]string{"foo"}, []string{"bar"}, []string{"bar", "foo"}},
		{[]string{"foo"}, []string{"bar", "baz"}, []string{"bar", "baz", "foo"}},
		{[]string{"foo"}, []string{"foo"}, []string{"foo"}},
	} {
		testutil.Equals(t, tc.expected, mergeTwoStringSlices(tc.a, tc.b))
	}
}

func TestChainedMergeSeriesSet(t *testing.T) {
	for _, tc := range []struct {
		input    []SeriesSet
		expected SeriesSet
	}{
		{
			input:    []SeriesSet{NewMockSeriesSet()},
			expected: NewMockSeriesSet(),
		},
		{
			input: []SeriesSet{NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			)},
			expected: NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			),
		},
		{
			input: []SeriesSet{NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
			), NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			)},
			expected: NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			),
		},
		{
			input: []SeriesSet{NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}}, []tsdbutil.Sample{sample{6, 6}}),
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			), NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}}, []tsdbutil.Sample{sample{4, 4}}),
			)},
			expected: NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{5, 5}},
					[]tsdbutil.Sample{sample{6, 6}},
				),
				NewTestSeries(labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
					[]tsdbutil.Sample{sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{4, 4}},
				),
			),
		},
		{
			input: []SeriesSet{NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}}),
			), NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{1, 1}}),
			)},
			expected: NewMockSeriesSet(
				NewTestSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}}, []tsdbutil.Sample{sample{1, 1}}),
			),
		},
	} {
		t.Run("", func(t *testing.T) {
			merged := NewChainedMergeSeriesSet(tc.input, nil)
			for merged.Next() {
				testutil.Assert(t, tc.expected.Next(), "Expected Next() to be true")
				actualSeries := merged.At()
				expectedSeries := tc.expected.At()
				testutil.Equals(t, expectedSeries.Labels(), actualSeries.Labels())

				expSmpl, expErr := ExpandSamples(expectedSeries.SampleIterator())
				actSmpl, actErr := ExpandSamples(actualSeries.SampleIterator())
				testutil.Equals(t, expErr, actErr)
				testutil.Equals(t, expSmpl, actSmpl)

				expChks, expErr := ExpandChunks(expectedSeries.ChunkIterator())
				actChks, actErr := ExpandChunks(actualSeries.ChunkIterator())
				testutil.Equals(t, expErr, actErr)
				testutil.Equals(t, expChks, actChks)

			}
			testutil.Ok(t, merged.Err())
			testutil.Assert(t, !tc.expected.Next(), "Expected Next() to be false")
		})
	}
}

func TestChainSampleIterator(t *testing.T) {
	for _, tc := range []struct {
		input    []chunkenc.Iterator
		expected []tsdbutil.Sample
	}{
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{1, 1}}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
		},
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{1, 1}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{2, 2}, sample{3, 3}}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}},
		},
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{3, 3}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{1, 1}, sample{4, 4}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{2, 2}, sample{5, 5}}),
			},
			expected: []tsdbutil.Sample{
				sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}},
		},
		// Overlap.
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{1, 1}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{2, 2}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{2, 2}, sample{3, 3}}),
				NewSampleIterator(tsdbutil.SampleSlice{}),
				NewSampleIterator(tsdbutil.SampleSlice{}),
				NewSampleIterator(tsdbutil.SampleSlice{}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}},
		},
	} {
		merged := newChainSampleIterator(tc.input)
		actual, err := ExpandSamples(merged)
		testutil.Ok(t, err)
		testutil.Equals(t, tc.expected, actual)
	}
}

func TestChainSampleIteratorSeek(t *testing.T) {
	for _, tc := range []struct {
		input    []chunkenc.Iterator
		seek     int64
		expected []tsdbutil.Sample
	}{
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			},
			seek:     1,
			expected: []tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
		},
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{1, 1}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{2, 2}, sample{3, 3}}),
			},
			seek:     2,
			expected: []tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
		},
		{
			input: []chunkenc.Iterator{
				NewSampleIterator(tsdbutil.SampleSlice{sample{0, 0}, sample{3, 3}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{1, 1}, sample{4, 4}}),
				NewSampleIterator(tsdbutil.SampleSlice{sample{2, 2}, sample{5, 5}}),
			},
			seek:     2,
			expected: []tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}},
		},
	} {
		merged := newChainSampleIterator(tc.input)
		actual := []tsdbutil.Sample{}
		if merged.Seek(tc.seek) {
			t, v := merged.At()
			actual = append(actual, sample{t, v})
		}
		s, err := ExpandSamples(merged)
		testutil.Ok(t, err)
		actual = append(actual, s...)
		testutil.Equals(t, tc.expected, actual)
	}
}

func TestChainChunkIterator(t *testing.T) {
	for _, tc := range []struct {
		input    []chunks.Iterator
		expected []chunks.Meta
	}{
		{
			input: []chunks.Iterator{
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
				)),
			},
			expected: []chunks.Meta{tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
			)},
		},
		{
			input: []chunks.Iterator{
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
				)),
			},
			expected: []chunks.Meta{tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
			)},
		},
		{
			input: []chunks.Iterator{
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{3, 3}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{1, 1}, sample{4, 4}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{2, 2}, sample{5, 5}},
				)),
			},
			expected: []chunks.Meta{tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{3, 3}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{1, 1}, sample{4, 4}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{2, 2}, sample{5, 5}},
			)},
		},
		{
			// Overlap
			input: []chunks.Iterator{
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{2, 2}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
				)),
				NewChunksIterator(tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{0, 0}, sample{5, 5}},
				), tsdbutil.ChunkFromSamples(
					[]tsdbutil.Sample{sample{1, 1}, sample{t: 4, v: 4}},
				)),
				NewChunksIterator(),
				NewChunksIterator(),
			},
			expected: []chunks.Meta{tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{2, 2}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{0, 0}, sample{5, 5}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{1, 1}, sample{t: 4, v: 4}},
			), tsdbutil.ChunkFromSamples(
				[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
			)},
		},
	} {
		t.Run("", func(t *testing.T) {
			merged := newChainChunkIterator(tc.input)
			actual, err := ExpandChunks(merged)
			testutil.Ok(t, err)
			testutil.Equals(t, tc.expected, actual)
		})
	}
}

type mockSeriesSet struct {
	idx    int
	series []Series
}

func NewMockSeriesSet(series ...Series) SeriesSet {
	return &mockSeriesSet{
		idx:    -1,
		series: series,
	}
}

func (m *mockSeriesSet) Next() bool {
	m.idx++
	return m.idx < len(m.series)
}

func (m *mockSeriesSet) At() Series {
	return m.series[m.idx]
}

func (m *mockSeriesSet) Err() error {
	return nil
}

var result []tsdbutil.Sample

func makeSeriesSet(numSeries, numSamples int) SeriesSet {
	series := []Series{}
	for j := 0; j < numSeries; j++ {
		labels := labels.Labels{{Name: "foo", Value: fmt.Sprintf("bar%d", j)}}
		samples := []tsdbutil.Sample{}
		for k := 0; k < numSamples; k++ {
			samples = append(samples, sample{t: int64(k), v: float64(k)})
		}
		series = append(series, NewTestSeries(labels, samples))
	}
	return NewMockSeriesSet(series...)
}

func makeMergeSeriesSet(numSeriesSets, numSeries, numSamples int) SeriesSet {
	seriesSets := []SeriesSet{}
	for i := 0; i < numSeriesSets; i++ {
		seriesSets = append(seriesSets, makeSeriesSet(numSeries, numSamples))
	}
	return NewChainedMergeSeriesSet(seriesSets, nil)
}

func benchmarkDrain(seriesSet SeriesSet, b *testing.B) {
	var err error
	for n := 0; n < b.N; n++ {
		for seriesSet.Next() {
			result, err = ExpandSamples(seriesSet.At().SampleIterator())
			testutil.Ok(b, err)
		}
	}
}

func BenchmarkNoMergeSeriesSet_100_100(b *testing.B) {
	seriesSet := makeSeriesSet(100, 100)
	benchmarkDrain(seriesSet, b)
}

func BenchmarkMergeSeriesSet(b *testing.B) {
	for _, bm := range []struct {
		numSeriesSets, numSeries, numSamples int
	}{
		{1, 100, 100},
		{10, 100, 100},
		{100, 100, 100},
	} {
		seriesSet := makeMergeSeriesSet(bm.numSeriesSets, bm.numSeries, bm.numSamples)
		b.Run(fmt.Sprintf("%d_%d_%d", bm.numSeriesSets, bm.numSeries, bm.numSamples), func(b *testing.B) {
			benchmarkDrain(seriesSet, b)
		})
	}
}
