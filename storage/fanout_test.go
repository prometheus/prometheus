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

func TestMergeSeriesSet(t *testing.T) {
	for _, tc := range []struct {
		input    []SeriesSet
		expected SeriesSet
	}{
		{
			input:    []SeriesSet{newMockSeriesSet()},
			expected: newMockSeriesSet(),
		},

		{
			input: []SeriesSet{newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			)},
			expected: newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			),
		},

		{
			input: []SeriesSet{newMockSeriesSet(
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			), newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
			)},
			expected: newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			),
		},

		{
			input: []SeriesSet{newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			), newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{3, 3}, {4, 4}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{2, 2}, {3, 3}}),
			)},
			expected: newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}, {3, 3}, {4, 4}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}}),
			),
		},
		{
			input: []SeriesSet{newMockSeriesSet(
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, math.NaN()}}),
			), newMockSeriesSet(
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, math.NaN()}}),
			)},
			expected: newMockSeriesSet(
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, math.NaN()}}),
			),
		},
	} {
		merged := NewMergeSeriesSet(tc.input, nil)
		for merged.Next() {
			testutil.Assert(t, tc.expected.Next(), "Expected Next() to be true")
			actualSeries := merged.At()
			expectedSeries := tc.expected.At()
			testutil.Equals(t, expectedSeries.Labels(), actualSeries.Labels())
			testutil.Equals(t, drainSamples(expectedSeries.Iterator()), drainSamples(actualSeries.Iterator()))
		}
		testutil.Assert(t, !tc.expected.Next(), "Expected Next() to be false")
	}
}

func TestMergeIterator(t *testing.T) {
	for _, tc := range []struct {
		input    []SeriesIterator
		expected []sample
	}{
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
			},
			expected: []sample{{0, 0}, {1, 1}},
		},
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
				newListSeriesIterator([]sample{{2, 2}, {3, 3}}),
			},
			expected: []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}},
		},
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {3, 3}}),
				newListSeriesIterator([]sample{{1, 1}, {4, 4}}),
				newListSeriesIterator([]sample{{2, 2}, {5, 5}}),
			},
			expected: []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}},
		},
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
				newListSeriesIterator([]sample{{0, 0}, {2, 2}}),
				newListSeriesIterator([]sample{{2, 2}, {3, 3}}),
			},
			expected: []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}},
		},
	} {
		merged := newMergeIterator(tc.input)
		actual := drainSamples(merged)
		testutil.Equals(t, tc.expected, actual)
	}
}

func TestMergeIteratorSeek(t *testing.T) {
	for _, tc := range []struct {
		input    []SeriesIterator
		seek     int64
		expected []sample
	}{
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}, {2, 2}}),
			},
			seek:     1,
			expected: []sample{{1, 1}, {2, 2}},
		},
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
				newListSeriesIterator([]sample{{2, 2}, {3, 3}}),
			},
			seek:     2,
			expected: []sample{{2, 2}, {3, 3}},
		},
		{
			input: []SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {3, 3}}),
				newListSeriesIterator([]sample{{1, 1}, {4, 4}}),
				newListSeriesIterator([]sample{{2, 2}, {5, 5}}),
			},
			seek:     2,
			expected: []sample{{2, 2}, {3, 3}, {4, 4}, {5, 5}},
		},
	} {
		merged := newMergeIterator(tc.input)
		actual := []sample{}
		if merged.Seek(tc.seek) {
			t, v := merged.At()
			actual = append(actual, sample{t, v})
		}
		actual = append(actual, drainSamples(merged)...)
		testutil.Equals(t, tc.expected, actual)
	}
}

func drainSamples(iter SeriesIterator) []sample {
	result := []sample{}
	for iter.Next() {
		t, v := iter.At()
		// NaNs can't be compared normally, so substitute for another value.
		if math.IsNaN(v) {
			v = -42
		}
		result = append(result, sample{t, v})
	}
	return result
}

type mockSeriesSet struct {
	idx    int
	series []Series
}

func newMockSeriesSet(series ...Series) SeriesSet {
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

var result []sample

func makeSeriesSet(numSeries, numSamples int) SeriesSet {
	series := []Series{}
	for j := 0; j < numSeries; j++ {
		labels := labels.Labels{{Name: "foo", Value: fmt.Sprintf("bar%d", j)}}
		samples := []sample{}
		for k := 0; k < numSamples; k++ {
			samples = append(samples, sample{t: int64(k), v: float64(k)})
		}
		series = append(series, newMockSeries(labels, samples))
	}
	return newMockSeriesSet(series...)
}

func makeMergeSeriesSet(numSeriesSets, numSeries, numSamples int) SeriesSet {
	seriesSets := []SeriesSet{}
	for i := 0; i < numSeriesSets; i++ {
		seriesSets = append(seriesSets, makeSeriesSet(numSeries, numSamples))
	}
	return NewMergeSeriesSet(seriesSets, nil)
}

func benchmarkDrain(seriesSet SeriesSet, b *testing.B) {
	for n := 0; n < b.N; n++ {
		for seriesSet.Next() {
			result = drainSamples(seriesSet.At().Iterator())
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
