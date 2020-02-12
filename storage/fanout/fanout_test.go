// Copyright 2020 The Prometheus Authors
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

package fanout

import (
	"context"
	"fmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"math"
	"sort"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestSelectSorted(t *testing.T) {

	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0

	priStorage := teststorage.New(t)
	defer priStorage.Close()
	app1, _ := priStorage.Appender()
	app1.Add(inputLabel, 0, 0)
	inputTotalSize++
	app1.Add(inputLabel, 1000, 1)
	inputTotalSize++
	app1.Add(inputLabel, 2000, 2)
	inputTotalSize++
	err := app1.Commit()
	testutil.Ok(t, err)

	remoteStorage1 := teststorage.New(t)
	defer remoteStorage1.Close()
	app2, _ := remoteStorage1.Appender()
	app2.Add(inputLabel, 3000, 3)
	inputTotalSize++
	app2.Add(inputLabel, 4000, 4)
	inputTotalSize++
	app2.Add(inputLabel, 5000, 5)
	inputTotalSize++
	err = app2.Commit()
	testutil.Ok(t, err)

	remoteStorage2 := teststorage.New(t)
	defer remoteStorage2.Close()

	app3, _ := remoteStorage2.Appender()
	app3.Add(inputLabel, 6000, 6)
	inputTotalSize++
	app3.Add(inputLabel, 7000, 7)
	inputTotalSize++
	app3.Add(inputLabel, 8000, 8)
	inputTotalSize++

	err = app3.Commit()
	testutil.Ok(t, err)

	fanoutStorage := NewFanout(nil, priStorage, remoteStorage1, remoteStorage2)

	querier, err := fanoutStorage.Querier(context.Background(), 0, 8000)
	testutil.Ok(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
	testutil.Ok(t, err)

	seriesSet, _, err := querier.SelectSorted(nil, matcher)
	testutil.Ok(t, err)

	result := make(map[int64]float64)
	var labelsResult labels.Labels
	for seriesSet.Next() {
		series := seriesSet.At()
		seriesLabels := series.Labels()
		labelsResult = seriesLabels
		iterator := series.Iterator()
		for iterator.Next() {
			timestamp, value := iterator.At()
			result[timestamp] = value
		}
	}

	testutil.Equals(t, labelsResult, outputLabel)
	testutil.Equals(t, inputTotalSize, len(result))

}

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
		input    []storage.SeriesSet
		expected storage.SeriesSet
	}{
		{
			input:    []storage.SeriesSet{newMockSeriesSet()},
			expected: newMockSeriesSet(),
		},

		{
			input: []storage.SeriesSet{newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			)},
			expected: newMockSeriesSet(
				newMockSeries(labels.FromStrings("bar", "baz"), []sample{{1, 1}, {2, 2}}),
				newMockSeries(labels.FromStrings("foo", "bar"), []sample{{0, 0}, {1, 1}}),
			),
		},

		{
			input: []storage.SeriesSet{newMockSeriesSet(
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
			input: []storage.SeriesSet{newMockSeriesSet(
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
			input: []storage.SeriesSet{newMockSeriesSet(
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
		input    []storage.SeriesIterator
		expected []sample
	}{
		{
			input: []storage.SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
			},
			expected: []sample{{0, 0}, {1, 1}},
		},
		{
			input: []storage.SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
				newListSeriesIterator([]sample{{2, 2}, {3, 3}}),
			},
			expected: []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}},
		},
		{
			input: []storage.SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {3, 3}}),
				newListSeriesIterator([]sample{{1, 1}, {4, 4}}),
				newListSeriesIterator([]sample{{2, 2}, {5, 5}}),
			},
			expected: []sample{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}},
		},
		{
			input: []storage.SeriesIterator{
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
		input    []storage.SeriesIterator
		seek     int64
		expected []sample
	}{
		{
			input: []storage.SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}, {2, 2}}),
			},
			seek:     1,
			expected: []sample{{1, 1}, {2, 2}},
		},
		{
			input: []storage.SeriesIterator{
				newListSeriesIterator([]sample{{0, 0}, {1, 1}}),
				newListSeriesIterator([]sample{{2, 2}, {3, 3}}),
			},
			seek:     2,
			expected: []sample{{2, 2}, {3, 3}},
		},
		{
			input: []storage.SeriesIterator{
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

func drainSamples(iter storage.SeriesIterator) []sample {
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
	series []storage.Series
}

func newMockSeriesSet(series ...storage.Series) storage.SeriesSet {
	return &mockSeriesSet{
		idx:    -1,
		series: series,
	}
}

func (m *mockSeriesSet) Next() bool {
	m.idx++
	return m.idx < len(m.series)
}

func (m *mockSeriesSet) At() storage.Series {
	return m.series[m.idx]
}

func (m *mockSeriesSet) Err() error {
	return nil
}

var result []sample

func makeSeriesSet(numSeries, numSamples int) storage.SeriesSet {
	series := []storage.Series{}
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

func makeMergeSeriesSet(numSeriesSets, numSeries, numSamples int) storage.SeriesSet {
	seriesSets := []storage.SeriesSet{}
	for i := 0; i < numSeriesSets; i++ {
		seriesSets = append(seriesSets, makeSeriesSet(numSeries, numSamples))
	}
	return NewMergeSeriesSet(seriesSets, nil)
}

func benchmarkDrain(seriesSet storage.SeriesSet, b *testing.B) {
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

type mockSeries struct {
	labels   func() labels.Labels
	iterator func() storage.SeriesIterator
}

func newMockSeries(lset labels.Labels, samples []sample) storage.Series {
	return &mockSeries{
		labels: func() labels.Labels {
			return lset
		},
		iterator: func() storage.SeriesIterator {
			return newListSeriesIterator(samples)
		},
	}
}

func (m *mockSeries) Labels() labels.Labels            { return m.labels() }
func (m *mockSeries) Iterator() storage.SeriesIterator { return m.iterator() }

type mockSeriesIterator struct {
	seek func(int64) bool
	at   func() (int64, float64)
	next func() bool
	err  func() error
}

func (m *mockSeriesIterator) Seek(t int64) bool    { return m.seek(t) }
func (m *mockSeriesIterator) At() (int64, float64) { return m.at() }
func (m *mockSeriesIterator) Next() bool           { return m.next() }
func (m *mockSeriesIterator) Err() error           { return m.err() }

type listSeriesIterator struct {
	list []sample
	idx  int
}

func newListSeriesIterator(list []sample) *listSeriesIterator {
	return &listSeriesIterator{list: list, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.list[it.idx]
	return s.t, s.v
}

func (it *listSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Seek(t int64) bool {
	if it.idx == -1 {
		it.idx = 0
	}
	// Do binary search between current position and end.
	it.idx = sort.Search(len(it.list)-it.idx, func(i int) bool {
		s := it.list[i+it.idx]
		return s.t >= t
	})

	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Err() error {
	return nil
}

type sample struct {
	t int64
	v float64
}
