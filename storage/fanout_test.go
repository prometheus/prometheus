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
// limitations under the License.package remote

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
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
		require.Equal(t, tc.expected, mergeStringSlices(tc.input))
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
		require.Equal(t, tc.expected, mergeTwoStringSlices(tc.a, tc.b))
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
	} {
		merged := newMergeSeriesSet(tc.input)
		for merged.Next() {
			require.True(t, tc.expected.Next())
			actualSeries := merged.At()
			expectedSeries := tc.expected.At()
			require.Equal(t, expectedSeries.Labels(), actualSeries.Labels())
			require.Equal(t, drainSamples(expectedSeries.Iterator()), drainSamples(actualSeries.Iterator()))
		}
		require.False(t, tc.expected.Next())
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
	} {
		merged := newMergeIterator(tc.input)
		actual := drainSamples(merged)
		require.Equal(t, tc.expected, actual)
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
		require.Equal(t, tc.expected, actual)
	}
}

func drainSamples(iter SeriesIterator) []sample {
	result := []sample{}
	for iter.Next() {
		t, v := iter.At()
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

func newMockSeries(lset labels.Labels, samples []sample) Series {
	return &mockSeries{
		labels: func() labels.Labels {
			return lset
		},
		iterator: func() SeriesIterator {
			return newListSeriesIterator(samples)
		},
	}
}
