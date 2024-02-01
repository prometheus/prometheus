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

package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestMergeQuerierWithChainMerger(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		primaryQuerierSeries []Series
		querierSeries        [][]Series
		extraQueriers        []Querier

		expected SeriesSet
	}{
		{
			name:                 "one primary querier with no series",
			primaryQuerierSeries: []Series{},
			expected:             NewMockSeriesSet(),
		},
		{
			name:          "one secondary querier with no series",
			querierSeries: [][]Series{{}},
			expected:      NewMockSeriesSet(),
		},
		{
			name:          "many secondary queriers with no series",
			querierSeries: [][]Series{{}, {}, {}, {}, {}, {}, {}},
			expected:      NewMockSeriesSet(),
		},
		{
			name:                 "mix of queriers with no series",
			primaryQuerierSeries: []Series{},
			querierSeries:        [][]Series{{}, {}, {}, {}, {}, {}, {}},
			expected:             NewMockSeriesSet(),
		},
		// Test rest of cases on secondary queriers as the different between primary vs secondary is just error handling.
		{
			name: "one querier, two series",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			),
		},
		{
			name: "two queriers, one different series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			),
		},
		{
			name: "two time unsorted queriers, two series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}, fSample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}, fSample{4, 4}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}, fSample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}},
				),
			),
		},
		{
			name: "five queriers, only two queriers have two time unsorted series each",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}, fSample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}, fSample{4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}, fSample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}},
				),
			),
		},
		{
			name: "two queriers, only two queriers have two time unsorted series each, with 3 noop and one nil querier together",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}, fSample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}, fSample{4, 4}}),
			}, {}},
			extraQueriers: []Querier{NoopQuerier(), NoopQuerier(), nil, NoopQuerier()},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}, fSample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}},
				),
			),
		},
		{
			name: "two queriers, with two series, one is overlapping",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 21}, fSample{3, 31}, fSample{5, 5}, fSample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 22}, fSample{3, 32}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}, fSample{4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 21}, fSample{3, 31}, fSample{5, 5}, fSample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, math.NaN()}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{1, 1}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, math.NaN()}, fSample{1, 1}}),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p Querier
			if tc.primaryQuerierSeries != nil {
				p = &mockQuerier{toReturn: tc.primaryQuerierSeries}
			}
			var qs []Querier
			for _, in := range tc.querierSeries {
				qs = append(qs, &mockQuerier{toReturn: in})
			}
			qs = append(qs, tc.extraQueriers...)

			mergedQuerier := NewMergeQuerier([]Querier{p}, qs, ChainedSeriesMerge).Select(context.Background(), false, nil)

			// Get all merged series upfront to make sure there are no incorrectly retained shared
			// buffers causing bugs.
			var mergedSeries []Series
			for mergedQuerier.Next() {
				mergedSeries = append(mergedSeries, mergedQuerier.At())
			}
			require.NoError(t, mergedQuerier.Err())

			for _, actualSeries := range mergedSeries {
				require.True(t, tc.expected.Next(), "Expected Next() to be true")
				expectedSeries := tc.expected.At()
				require.Equal(t, expectedSeries.Labels(), actualSeries.Labels())

				expSmpl, expErr := ExpandSamples(expectedSeries.Iterator(nil), nil)
				actSmpl, actErr := ExpandSamples(actualSeries.Iterator(nil), nil)
				require.Equal(t, expErr, actErr)
				require.Equal(t, expSmpl, actSmpl)
			}
			require.False(t, tc.expected.Next(), "Expected Next() to be false")
		})
	}
}

func TestMergeChunkQuerierWithNoVerticalChunkSeriesMerger(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		primaryChkQuerierSeries []ChunkSeries
		chkQuerierSeries        [][]ChunkSeries
		extraQueriers           []ChunkQuerier

		expected ChunkSeriesSet
	}{
		{
			name:                    "one primary querier with no series",
			primaryChkQuerierSeries: []ChunkSeries{},
			expected:                NewMockChunkSeriesSet(),
		},
		{
			name:             "one secondary querier with no series",
			chkQuerierSeries: [][]ChunkSeries{{}},
			expected:         NewMockChunkSeriesSet(),
		},
		{
			name:             "many secondary queriers with no series",
			chkQuerierSeries: [][]ChunkSeries{{}, {}, {}, {}, {}, {}, {}},
			expected:         NewMockChunkSeriesSet(),
		},
		{
			name:                    "mix of queriers with no series",
			primaryChkQuerierSeries: []ChunkSeries{},
			chkQuerierSeries:        [][]ChunkSeries{{}, {}, {}, {}, {}, {}, {}},
			expected:                NewMockChunkSeriesSet(),
		},
		// Test rest of cases on secondary queriers as the different between primary vs secondary is just error handling.
		{
			name: "one querier, two series",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			),
		},
		{
			name: "two secondaries, one different series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			),
		},
		{
			name: "two secondaries, two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}}, []chunks.Sample{fSample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}}, []chunks.Sample{fSample{4, 4}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{5, 5}},
					[]chunks.Sample{fSample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}},
					[]chunks.Sample{fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{4, 4}},
				),
			),
		},
		{
			name: "five secondaries, only two have two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{}, {}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}}, []chunks.Sample{fSample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}}, []chunks.Sample{fSample{4, 4}}),
			}, {}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{5, 5}},
					[]chunks.Sample{fSample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}},
					[]chunks.Sample{fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{4, 4}},
				),
			),
		},
		{
			name: "two secondaries, with two not in time order series each, with 3 noop queries and one nil together",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{5, 5}}, []chunks.Sample{fSample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}}, []chunks.Sample{fSample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{3, 3}}, []chunks.Sample{fSample{4, 4}}),
			}},
			extraQueriers: []ChunkQuerier{NoopChunkedQuerier(), NoopChunkedQuerier(), nil, NoopChunkedQuerier()},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{1, 1}, fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{5, 5}},
					[]chunks.Sample{fSample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0}, fSample{1, 1}},
					[]chunks.Sample{fSample{2, 2}},
					[]chunks.Sample{fSample{3, 3}},
					[]chunks.Sample{fSample{4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, math.NaN()}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{1, 1}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, math.NaN()}}, []chunks.Sample{fSample{1, 1}}),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p ChunkQuerier
			if tc.primaryChkQuerierSeries != nil {
				p = &mockChunkQurier{toReturn: tc.primaryChkQuerierSeries}
			}

			var qs []ChunkQuerier
			for _, in := range tc.chkQuerierSeries {
				qs = append(qs, &mockChunkQurier{toReturn: in})
			}
			qs = append(qs, tc.extraQueriers...)

			merged := NewMergeChunkQuerier([]ChunkQuerier{p}, qs, NewCompactingChunkSeriesMerger(nil)).Select(context.Background(), false, nil)
			for merged.Next() {
				require.True(t, tc.expected.Next(), "Expected Next() to be true")
				actualSeries := merged.At()
				expectedSeries := tc.expected.At()
				require.Equal(t, expectedSeries.Labels(), actualSeries.Labels())

				expChks, expErr := ExpandChunks(expectedSeries.Iterator(nil))
				actChks, actErr := ExpandChunks(actualSeries.Iterator(nil))
				require.Equal(t, expErr, actErr)
				require.Equal(t, expChks, actChks)

			}
			require.NoError(t, merged.Err())
			require.False(t, tc.expected.Next(), "Expected Next() to be false")
		})
	}
}

func histogramSample(ts int64, hint histogram.CounterResetHint) hSample {
	h := tsdbutil.GenerateTestHistogram(int(ts + 1))
	h.CounterResetHint = hint
	return hSample{t: ts, h: h}
}

func floatHistogramSample(ts int64, hint histogram.CounterResetHint) fhSample {
	fh := tsdbutil.GenerateTestFloatHistogram(int(ts + 1))
	fh.CounterResetHint = hint
	return fhSample{t: ts, fh: fh}
}

// Shorthands for counter reset hints.
const (
	uk = histogram.UnknownCounterReset
	cr = histogram.CounterReset
	nr = histogram.NotCounterReset
	ga = histogram.GaugeType
)

func TestCompactingChunkSeriesMerger(t *testing.T) {
	m := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)

	// histogramSample returns a histogram that is unique to the ts.
	histogramSample := func(ts int64) hSample {
		return histogramSample(ts, uk)
	}

	floatHistogramSample := func(ts int64) fhSample {
		return floatHistogramSample(ts, uk)
	}

	for _, tc := range []struct {
		name     string
		input    []ChunkSeries
		expected ChunkSeries
	}{
		{
			name: "single empty series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
		},
		{
			name: "single series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
		},
		{
			name: "two empty series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
		},
		{
			name: "two non overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{5, 5}}, []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
		},
		{
			name: "two overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{8, 8}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{7, 7}, fSample{8, 8}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
		},
		{
			name: "two duplicated",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
		},
		{
			name: "three overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0}, fSample{4, 4}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0}, fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}, fSample{5, 5}, fSample{6, 6}}),
		},
		{
			name: "three in chained overlap",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{4, 4}, fSample{6, 66}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{6, 6}, fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{4, 4}, fSample{5, 5}, fSample{6, 66}, fSample{10, 10}}),
		},
		{
			name: "three in chained overlap complex",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0}, fSample{5, 5}}, []chunks.Sample{fSample{10, 10}, fSample{15, 15}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{20, 20}}, []chunks.Sample{fSample{25, 25}, fSample{30, 30}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{18, 18}, fSample{26, 26}}, []chunks.Sample{fSample{31, 31}, fSample{35, 35}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 0}, fSample{2, 2}, fSample{5, 5}, fSample{10, 10}, fSample{15, 15}, fSample{18, 18}, fSample{20, 20}, fSample{25, 25}, fSample{26, 26}, fSample{30, 30}},
				[]chunks.Sample{fSample{31, 31}, fSample{35, 35}},
			),
		},
		{
			name: "110 overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(0, 110)), // [0 - 110)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(60, 50)), // [60 - 110)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				chunks.GenerateSamples(0, 110),
			),
		},
		{
			name: "150 overlapping samples, split chunk",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(0, 90)),  // [0 - 90)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(60, 90)), // [90 - 150)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				chunks.GenerateSamples(0, 120),
				chunks.GenerateSamples(120, 30),
			),
		},
		{
			name: "histogram chunks overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{histogramSample(0), histogramSample(5)}, []chunks.Sample{histogramSample(10), histogramSample(15)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{histogramSample(2), histogramSample(20)}, []chunks.Sample{histogramSample(25), histogramSample(30)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{histogramSample(18), histogramSample(26)}, []chunks.Sample{histogramSample(31), histogramSample(35)}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{histogramSample(0), histogramSample(2), histogramSample(5), histogramSample(10), histogramSample(15), histogramSample(18), histogramSample(20), histogramSample(25), histogramSample(26), histogramSample(30)},
				[]chunks.Sample{histogramSample(31), histogramSample(35)},
			),
		},
		{
			name: "histogram chunks overlapping with float chunks",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{histogramSample(0), histogramSample(5)}, []chunks.Sample{histogramSample(10), histogramSample(15)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{12, 12}}, []chunks.Sample{fSample{14, 14}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{histogramSample(0)},
				[]chunks.Sample{fSample{1, 1}},
				[]chunks.Sample{histogramSample(5), histogramSample(10)},
				[]chunks.Sample{fSample{12, 12}, fSample{14, 14}},
				[]chunks.Sample{histogramSample(15)},
			),
		},
		{
			name: "float histogram chunks overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{floatHistogramSample(0), floatHistogramSample(5)}, []chunks.Sample{floatHistogramSample(10), floatHistogramSample(15)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{floatHistogramSample(2), floatHistogramSample(20)}, []chunks.Sample{floatHistogramSample(25), floatHistogramSample(30)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{floatHistogramSample(18), floatHistogramSample(26)}, []chunks.Sample{floatHistogramSample(31), floatHistogramSample(35)}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{floatHistogramSample(0), floatHistogramSample(2), floatHistogramSample(5), floatHistogramSample(10), floatHistogramSample(15), floatHistogramSample(18), floatHistogramSample(20), floatHistogramSample(25), floatHistogramSample(26), floatHistogramSample(30)},
				[]chunks.Sample{floatHistogramSample(31), floatHistogramSample(35)},
			),
		},
		{
			name: "float histogram chunks overlapping with float chunks",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{floatHistogramSample(0), floatHistogramSample(5)}, []chunks.Sample{floatHistogramSample(10), floatHistogramSample(15)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{12, 12}}, []chunks.Sample{fSample{14, 14}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{floatHistogramSample(0)},
				[]chunks.Sample{fSample{1, 1}},
				[]chunks.Sample{floatHistogramSample(5), floatHistogramSample(10)},
				[]chunks.Sample{fSample{12, 12}, fSample{14, 14}},
				[]chunks.Sample{floatHistogramSample(15)},
			),
		},
		{
			name: "float histogram chunks overlapping with histogram chunks",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{floatHistogramSample(0), floatHistogramSample(5)}, []chunks.Sample{floatHistogramSample(10), floatHistogramSample(15)}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{histogramSample(1), histogramSample(12)}, []chunks.Sample{histogramSample(14)}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{floatHistogramSample(0)},
				[]chunks.Sample{histogramSample(1)},
				[]chunks.Sample{floatHistogramSample(5), floatHistogramSample(10)},
				[]chunks.Sample{histogramSample(12), histogramSample(14)},
				[]chunks.Sample{floatHistogramSample(15)},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged := m(tc.input...)
			require.Equal(t, tc.expected.Labels(), merged.Labels())
			actChks, actErr := ExpandChunks(merged.Iterator(nil))
			expChks, expErr := ExpandChunks(tc.expected.Iterator(nil))

			require.Equal(t, expErr, actErr)
			require.Equal(t, expChks, actChks)

			actSamples := chunks.ChunkMetasToSamples(actChks)
			expSamples := chunks.ChunkMetasToSamples(expChks)
			require.Equal(t, expSamples, actSamples)
		})
	}
}

func TestCompactingChunkSeriesMergerHistogramCounterResetHint(t *testing.T) {
	m := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)

	for sampleType, sampleFunc := range map[string]func(int64, histogram.CounterResetHint) chunks.Sample{
		"histogram":       func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return histogramSample(ts, hint) },
		"float histogram": func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return floatHistogramSample(ts, hint) },
	} {
		for name, tc := range map[string]struct {
			input    []ChunkSeries
			expected ChunkSeries
		}{
			"histogram counter reset hint kept in single series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
				),
			},
			"histogram not counter reset hint kept in single series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
				),
			},
			"histogram counter reset hint kept in multiple equal series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
					),
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
				),
			},
			"histogram not counter reset hint kept in multiple equal series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
					),
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
				),
			},
			"histogram counter reset hint dropped from differing series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, cr), sampleFunc(15, uk)},
					),
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, cr), sampleFunc(12, uk), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, cr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, uk), sampleFunc(12, uk), sampleFunc(15, uk)},
				),
			},
			"histogram counter not reset hint dropped from differing series": {
				input: []ChunkSeries{
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, nr), sampleFunc(15, uk)},
					),
					NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
						[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
						[]chunks.Sample{sampleFunc(10, nr), sampleFunc(12, uk), sampleFunc(15, uk)},
					),
				},
				expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{sampleFunc(0, nr), sampleFunc(5, uk)},
					[]chunks.Sample{sampleFunc(10, uk), sampleFunc(12, uk), sampleFunc(15, uk)},
				),
			},
		} {
			t.Run(sampleType+"/"+name, func(t *testing.T) {
				merged := m(tc.input...)
				require.Equal(t, tc.expected.Labels(), merged.Labels())
				actChks, actErr := ExpandChunks(merged.Iterator(nil))
				expChks, expErr := ExpandChunks(tc.expected.Iterator(nil))

				require.Equal(t, expErr, actErr)
				require.Equal(t, expChks, actChks)

				actSamples := chunks.ChunkMetasToSamples(actChks)
				expSamples := chunks.ChunkMetasToSamples(expChks)
				require.Equal(t, expSamples, actSamples)
			})
		}
	}
}

func TestConcatenatingChunkSeriesMerger(t *testing.T) {
	m := NewConcatenatingChunkSeriesMerger()

	for _, tc := range []struct {
		name     string
		input    []ChunkSeries
		expected ChunkSeries
	}{
		{
			name: "single empty series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
		},
		{
			name: "single series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}}),
		},
		{
			name: "two empty series",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), nil, nil),
		},
		{
			name: "two non overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{5, 5}}, []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
		},
		{
			name: "two overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{8, 8}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{1, 1}, fSample{2, 2}}, []chunks.Sample{fSample{3, 3}, fSample{8, 8}},
				[]chunks.Sample{fSample{7, 7}, fSample{9, 9}}, []chunks.Sample{fSample{10, 10}},
			),
		},
		{
			name: "two duplicated",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}},
				[]chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{5, 5}},
			),
		},
		{
			name: "three overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0}, fSample{4, 4}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}},
				[]chunks.Sample{fSample{2, 2}, fSample{3, 3}, fSample{6, 6}},
				[]chunks.Sample{fSample{0, 0}, fSample{4, 4}},
			),
		},
		{
			name: "three in chained overlap",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{4, 4}, fSample{6, 66}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{6, 6}, fSample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{1, 1}, fSample{2, 2}, fSample{3, 3}, fSample{5, 5}},
				[]chunks.Sample{fSample{4, 4}, fSample{6, 66}},
				[]chunks.Sample{fSample{6, 6}, fSample{10, 10}},
			),
		},
		{
			name: "three in chained overlap complex",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0}, fSample{5, 5}}, []chunks.Sample{fSample{10, 10}, fSample{15, 15}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{2, 2}, fSample{20, 20}}, []chunks.Sample{fSample{25, 25}, fSample{30, 30}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{18, 18}, fSample{26, 26}}, []chunks.Sample{fSample{31, 31}, fSample{35, 35}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 0}, fSample{5, 5}}, []chunks.Sample{fSample{10, 10}, fSample{15, 15}},
				[]chunks.Sample{fSample{2, 2}, fSample{20, 20}}, []chunks.Sample{fSample{25, 25}, fSample{30, 30}},
				[]chunks.Sample{fSample{18, 18}, fSample{26, 26}}, []chunks.Sample{fSample{31, 31}, fSample{35, 35}},
			),
		},
		{
			name: "110 overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(0, 110)), // [0 - 110)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(60, 50)), // [60 - 110)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				chunks.GenerateSamples(0, 110),
				chunks.GenerateSamples(60, 50),
			),
		},
		{
			name: "150 overlapping samples, simply concatenated and no splits",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(0, 90)),  // [0 - 90)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), chunks.GenerateSamples(60, 90)), // [90 - 150)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				chunks.GenerateSamples(0, 90),
				chunks.GenerateSamples(60, 90),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged := m(tc.input...)
			require.Equal(t, tc.expected.Labels(), merged.Labels())
			actChks, actErr := ExpandChunks(merged.Iterator(nil))
			expChks, expErr := ExpandChunks(tc.expected.Iterator(nil))

			require.Equal(t, expErr, actErr)
			require.Equal(t, expChks, actChks)
		})
	}
}

func TestConcatenatingChunkIterator(t *testing.T) {
	chunk1, err := chunks.ChunkFromSamples([]chunks.Sample{fSample{t: 1, f: 10}})
	require.NoError(t, err)
	chunk2, err := chunks.ChunkFromSamples([]chunks.Sample{fSample{t: 2, f: 20}})
	require.NoError(t, err)
	chunk3, err := chunks.ChunkFromSamples([]chunks.Sample{fSample{t: 3, f: 30}})
	require.NoError(t, err)

	testError := errors.New("something went wrong")

	testCases := map[string]struct {
		iterators      []chunks.Iterator
		expectedChunks []chunks.Meta
		expectedError  error
	}{
		"many successful iterators": {
			iterators: []chunks.Iterator{
				NewListChunkSeriesIterator(chunk1, chunk2),
				NewListChunkSeriesIterator(chunk3),
			},
			expectedChunks: []chunks.Meta{chunk1, chunk2, chunk3},
		},
		"single failing iterator": {
			iterators: []chunks.Iterator{
				errChunksIterator{err: testError},
			},
			expectedError: testError,
		},
		"some failing and some successful iterators": {
			iterators: []chunks.Iterator{
				NewListChunkSeriesIterator(chunk1, chunk2),
				errChunksIterator{err: testError},
				NewListChunkSeriesIterator(chunk3),
			},
			expectedChunks: []chunks.Meta{chunk1, chunk2}, // Should stop before advancing to last iterator.
			expectedError:  testError,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			it := concatenatingChunkIterator{iterators: testCase.iterators}
			var chks []chunks.Meta

			for it.Next() {
				chks = append(chks, it.At())
			}

			require.Equal(t, testCase.expectedChunks, chks)

			if testCase.expectedError == nil {
				require.NoError(t, it.Err())
			} else {
				require.EqualError(t, it.Err(), testCase.expectedError.Error())
			}
		})
	}
}

type mockQuerier struct {
	LabelQuerier

	toReturn []Series
}

type seriesByLabel []Series

func (a seriesByLabel) Len() int           { return len(a) }
func (a seriesByLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a seriesByLabel) Less(i, j int) bool { return labels.Compare(a[i].Labels(), a[j].Labels()) < 0 }

func (m *mockQuerier) Select(_ context.Context, sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) SeriesSet {
	cpy := make([]Series, len(m.toReturn))
	copy(cpy, m.toReturn)
	if sortSeries {
		sort.Sort(seriesByLabel(cpy))
	}

	return NewMockSeriesSet(cpy...)
}

type mockChunkQurier struct {
	LabelQuerier

	toReturn []ChunkSeries
}

type chunkSeriesByLabel []ChunkSeries

func (a chunkSeriesByLabel) Len() int      { return len(a) }
func (a chunkSeriesByLabel) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a chunkSeriesByLabel) Less(i, j int) bool {
	return labels.Compare(a[i].Labels(), a[j].Labels()) < 0
}

func (m *mockChunkQurier) Select(_ context.Context, sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) ChunkSeriesSet {
	cpy := make([]ChunkSeries, len(m.toReturn))
	copy(cpy, m.toReturn)
	if sortSeries {
		sort.Sort(chunkSeriesByLabel(cpy))
	}

	return NewMockChunkSeriesSet(cpy...)
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

func (m *mockSeriesSet) At() Series { return m.series[m.idx] }

func (m *mockSeriesSet) Err() error { return nil }

func (m *mockSeriesSet) Warnings() annotations.Annotations { return nil }

type mockChunkSeriesSet struct {
	idx    int
	series []ChunkSeries
}

func NewMockChunkSeriesSet(series ...ChunkSeries) ChunkSeriesSet {
	return &mockChunkSeriesSet{
		idx:    -1,
		series: series,
	}
}

func (m *mockChunkSeriesSet) Next() bool {
	m.idx++
	return m.idx < len(m.series)
}

func (m *mockChunkSeriesSet) At() ChunkSeries { return m.series[m.idx] }

func (m *mockChunkSeriesSet) Err() error { return nil }

func (m *mockChunkSeriesSet) Warnings() annotations.Annotations { return nil }

func TestChainSampleIterator(t *testing.T) {
	for sampleType, sampleFunc := range map[string]func(int64) chunks.Sample{
		"float":           func(ts int64) chunks.Sample { return fSample{ts, float64(ts)} },
		"histogram":       func(ts int64) chunks.Sample { return histogramSample(ts, uk) },
		"float histogram": func(ts int64) chunks.Sample { return floatHistogramSample(ts, uk) },
	} {
		for name, tc := range map[string]struct {
			input    []chunkenc.Iterator
			expected []chunks.Sample
		}{
			"single iterator": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1)}),
				},
				expected: []chunks.Sample{sampleFunc(0), sampleFunc(1)},
			},
			"non overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1)}),
					NewListSeriesIterator(samples{sampleFunc(2), sampleFunc(3)}),
				},
				expected: []chunks.Sample{sampleFunc(0), sampleFunc(1), sampleFunc(2), sampleFunc(3)},
			},
			"overlapping but distinct iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(3)}),
					NewListSeriesIterator(samples{sampleFunc(1), sampleFunc(4)}),
					NewListSeriesIterator(samples{sampleFunc(2), sampleFunc(5)}),
				},
				expected: []chunks.Sample{
					sampleFunc(0), sampleFunc(1), sampleFunc(2), sampleFunc(3), sampleFunc(4), sampleFunc(5),
				},
			},
			"overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1)}),
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(2)}),
					NewListSeriesIterator(samples{sampleFunc(2), sampleFunc(3)}),
					NewListSeriesIterator(samples{}),
					NewListSeriesIterator(samples{}),
					NewListSeriesIterator(samples{}),
				},
				expected: []chunks.Sample{sampleFunc(0), sampleFunc(1), sampleFunc(2), sampleFunc(3)},
			},
		} {
			t.Run(sampleType+"/"+name, func(t *testing.T) {
				merged := ChainSampleIteratorFromIterators(nil, tc.input)
				actual, err := ExpandSamples(merged, nil)
				require.NoError(t, err)
				require.Equal(t, tc.expected, actual)
			})
		}
	}
}

func TestChainSampleIteratorHistogramCounterResetHint(t *testing.T) {
	for sampleType, sampleFunc := range map[string]func(int64, histogram.CounterResetHint) chunks.Sample{
		"histogram":       func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return histogramSample(ts, hint) },
		"float histogram": func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return floatHistogramSample(ts, hint) },
	} {
		for name, tc := range map[string]struct {
			input    []chunkenc.Iterator
			expected []chunks.Sample
		}{
			"single iterator": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, cr), sampleFunc(2, uk)}),
				},
				expected: []chunks.Sample{sampleFunc(0, uk), sampleFunc(1, cr), sampleFunc(2, uk)},
			},
			"single iterator gauge": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, ga), sampleFunc(1, ga), sampleFunc(2, ga)}),
				},
				expected: []chunks.Sample{sampleFunc(0, ga), sampleFunc(1, ga), sampleFunc(2, ga)},
			},
			"overlapping iterators gauge": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, ga), sampleFunc(1, ga), sampleFunc(2, ga), sampleFunc(4, ga)}),
					NewListSeriesIterator(samples{sampleFunc(0, ga), sampleFunc(1, ga), sampleFunc(3, ga), sampleFunc(5, ga)}),
				},
				expected: []chunks.Sample{sampleFunc(0, ga), sampleFunc(1, ga), sampleFunc(2, ga), sampleFunc(3, ga), sampleFunc(4, ga), sampleFunc(5, ga)},
			},
			"non overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, uk)}),
					NewListSeriesIterator(samples{sampleFunc(2, cr), sampleFunc(3, cr)}),
				},
				expected: []chunks.Sample{sampleFunc(0, uk), sampleFunc(1, uk), sampleFunc(2, uk), sampleFunc(3, cr)},
			},
			"overlapping but distinct iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(3, uk), sampleFunc(5, cr)}),
					NewListSeriesIterator(samples{sampleFunc(1, uk), sampleFunc(2, cr), sampleFunc(4, cr)}),
				},
				expected: []chunks.Sample{
					sampleFunc(0, uk), sampleFunc(1, uk), sampleFunc(2, cr), sampleFunc(3, uk), sampleFunc(4, uk), sampleFunc(5, uk),
				},
			},
			"overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, cr), sampleFunc(2, cr)}),
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, cr), sampleFunc(2, cr)}),
				},
				expected: []chunks.Sample{sampleFunc(0, uk), sampleFunc(1, uk), sampleFunc(2, uk)},
			},
		} {
			t.Run(sampleType+"/"+name, func(t *testing.T) {
				merged := ChainSampleIteratorFromIterators(nil, tc.input)
				actual, err := ExpandSamples(merged, nil)
				require.NoError(t, err)
				require.Equal(t, tc.expected, actual)
			})
		}
	}
}

func TestChainSampleIteratorSeek(t *testing.T) {
	for sampleType, sampleFunc := range map[string]func(int64) chunks.Sample{
		"float":           func(ts int64) chunks.Sample { return fSample{ts, float64(ts)} },
		"histogram":       func(ts int64) chunks.Sample { return histogramSample(ts, uk) },
		"float histogram": func(ts int64) chunks.Sample { return floatHistogramSample(ts, uk) },
	} {
		for name, tc := range map[string]struct {
			input    []chunkenc.Iterator
			seek     int64
			expected []chunks.Sample
		}{
			"single iterator": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1), sampleFunc(2)}),
				},
				seek:     1,
				expected: []chunks.Sample{sampleFunc(1), sampleFunc(2)},
			},
			"non overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1)}),
					NewListSeriesIterator(samples{sampleFunc(2), sampleFunc(3)}),
				},
				seek:     2,
				expected: []chunks.Sample{sampleFunc(2), sampleFunc(3)},
			},
			"overlapping but distinct iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(3)}),
					NewListSeriesIterator(samples{sampleFunc(1), sampleFunc(4)}),
					NewListSeriesIterator(samples{sampleFunc(2), sampleFunc(5)}),
				},
				seek:     2,
				expected: []chunks.Sample{sampleFunc(2), sampleFunc(3), sampleFunc(4), sampleFunc(5)},
			},
			"overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(2), sampleFunc(3)}),
					NewListSeriesIterator(samples{sampleFunc(0), sampleFunc(1), sampleFunc(2)}),
				},
				seek:     0,
				expected: []chunks.Sample{sampleFunc(0), sampleFunc(1), sampleFunc(2), sampleFunc(3)},
			},
		} {
			t.Run(sampleType+"/"+name, func(t *testing.T) {
				merged := ChainSampleIteratorFromIterators(nil, tc.input)
				actual := []chunks.Sample{}
				switch merged.Seek(tc.seek) {
				case chunkenc.ValFloat:
					t, f := merged.At()
					actual = append(actual, fSample{t, f})
				case chunkenc.ValHistogram:
					t, h := merged.AtHistogram(nil)
					actual = append(actual, hSample{t, h})
				case chunkenc.ValFloatHistogram:
					t, fh := merged.AtFloatHistogram(nil)
					actual = append(actual, fhSample{t, fh})
				}
				s, err := ExpandSamples(merged, nil)
				require.NoError(t, err)
				actual = append(actual, s...)
				require.Equal(t, tc.expected, actual)
			})
		}
	}
}

func TestChainSampleIteratorSeekFailingIterator(t *testing.T) {
	merged := ChainSampleIteratorFromIterators(nil, []chunkenc.Iterator{
		NewListSeriesIterator(samples{fSample{0, 0.1}, fSample{1, 1.1}, fSample{2, 2.1}}),
		errIterator{errors.New("something went wrong")},
	})

	require.Equal(t, chunkenc.ValNone, merged.Seek(0))
	require.EqualError(t, merged.Err(), "something went wrong")
}

func TestChainSampleIteratorNextImmediatelyFailingIterator(t *testing.T) {
	merged := ChainSampleIteratorFromIterators(nil, []chunkenc.Iterator{
		NewListSeriesIterator(samples{fSample{0, 0.1}, fSample{1, 1.1}, fSample{2, 2.1}}),
		errIterator{errors.New("something went wrong")},
	})

	require.Equal(t, chunkenc.ValNone, merged.Next())
	require.EqualError(t, merged.Err(), "something went wrong")

	// Next() does some special handling for the first iterator, so make sure it handles the first iterator returning an error too.
	merged = ChainSampleIteratorFromIterators(nil, []chunkenc.Iterator{
		errIterator{errors.New("something went wrong")},
		NewListSeriesIterator(samples{fSample{0, 0.1}, fSample{1, 1.1}, fSample{2, 2.1}}),
	})

	require.Equal(t, chunkenc.ValNone, merged.Next())
	require.EqualError(t, merged.Err(), "something went wrong")
}

func TestChainSampleIteratorSeekHistogramCounterResetHint(t *testing.T) {
	for sampleType, sampleFunc := range map[string]func(int64, histogram.CounterResetHint) chunks.Sample{
		"histogram":       func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return histogramSample(ts, hint) },
		"float histogram": func(ts int64, hint histogram.CounterResetHint) chunks.Sample { return floatHistogramSample(ts, hint) },
	} {
		for name, tc := range map[string]struct {
			input    []chunkenc.Iterator
			seek     int64
			expected []chunks.Sample
		}{
			"single iterator": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, cr), sampleFunc(2, uk)}),
				},
				seek:     1,
				expected: []chunks.Sample{sampleFunc(1, uk), sampleFunc(2, uk)},
			},
			"non overlapping iterators": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, uk)}),
					NewListSeriesIterator(samples{sampleFunc(2, cr), sampleFunc(3, cr)}),
				},
				seek:     2,
				expected: []chunks.Sample{sampleFunc(2, uk), sampleFunc(3, cr)},
			},
			"non overlapping iterators seek to internal reset": {
				input: []chunkenc.Iterator{
					NewListSeriesIterator(samples{sampleFunc(0, cr), sampleFunc(1, uk)}),
					NewListSeriesIterator(samples{sampleFunc(2, cr), sampleFunc(3, cr)}),
				},
				seek:     3,
				expected: []chunks.Sample{sampleFunc(3, uk)},
			},
		} {
			t.Run(sampleType+"/"+name, func(t *testing.T) {
				merged := ChainSampleIteratorFromIterators(nil, tc.input)
				actual := []chunks.Sample{}
				switch merged.Seek(tc.seek) {
				case chunkenc.ValFloat:
					t, f := merged.At()
					actual = append(actual, fSample{t, f})
				case chunkenc.ValHistogram:
					t, h := merged.AtHistogram(nil)
					actual = append(actual, hSample{t, h})
				case chunkenc.ValFloatHistogram:
					t, fh := merged.AtFloatHistogram(nil)
					actual = append(actual, fhSample{t, fh})
				}
				s, err := ExpandSamples(merged, nil)
				require.NoError(t, err)
				actual = append(actual, s...)
				require.Equal(t, tc.expected, actual)
			})
		}
	}
}

func makeSeries(numSeries, numSamples int) []Series {
	series := []Series{}
	for j := 0; j < numSeries; j++ {
		labels := labels.FromStrings("foo", fmt.Sprintf("bar%d", j))
		samples := []chunks.Sample{}
		for k := 0; k < numSamples; k++ {
			samples = append(samples, fSample{t: int64(k), f: float64(k)})
		}
		series = append(series, NewListSeries(labels, samples))
	}
	return series
}

func makeMergeSeriesSet(serieses [][]Series) SeriesSet {
	seriesSets := make([]genericSeriesSet, len(serieses))
	for i, s := range serieses {
		seriesSets[i] = &genericSeriesSetAdapter{NewMockSeriesSet(s...)}
	}
	return &seriesSetAdapter{newGenericMergeSeriesSet(seriesSets, (&seriesMergerAdapter{VerticalSeriesMergeFunc: ChainedSeriesMerge}).Merge)}
}

func benchmarkDrain(b *testing.B, makeSeriesSet func() SeriesSet) {
	var err error
	var t int64
	var v float64
	var iter chunkenc.Iterator
	for n := 0; n < b.N; n++ {
		seriesSet := makeSeriesSet()
		for seriesSet.Next() {
			iter = seriesSet.At().Iterator(iter)
			for iter.Next() == chunkenc.ValFloat {
				t, v = iter.At()
			}
			err = iter.Err()
		}
		require.NoError(b, err)
		require.NotEqual(b, t, v) // To ensure the inner loop doesn't get optimised away.
	}
}

func BenchmarkNoMergeSeriesSet_100_100(b *testing.B) {
	series := makeSeries(100, 100)
	benchmarkDrain(b, func() SeriesSet { return NewMockSeriesSet(series...) })
}

func BenchmarkMergeSeriesSet(b *testing.B) {
	for _, bm := range []struct {
		numSeriesSets, numSeries, numSamples int
	}{
		{1, 100, 100},
		{10, 100, 100},
		{100, 100, 100},
	} {
		serieses := [][]Series{}
		for i := 0; i < bm.numSeriesSets; i++ {
			serieses = append(serieses, makeSeries(bm.numSeries, bm.numSamples))
		}
		b.Run(fmt.Sprintf("%d_%d_%d", bm.numSeriesSets, bm.numSeries, bm.numSamples), func(b *testing.B) {
			benchmarkDrain(b, func() SeriesSet { return makeMergeSeriesSet(serieses) })
		})
	}
}

type mockGenericQuerier struct {
	mtx sync.Mutex

	closed                bool
	labelNamesCalls       int
	labelNamesRequested   []labelNameRequest
	sortedSeriesRequested []bool

	resp     []string
	warnings annotations.Annotations
	err      error
}

type labelNameRequest struct {
	name     string
	matchers []*labels.Matcher
}

func (m *mockGenericQuerier) Select(_ context.Context, b bool, _ *SelectHints, _ ...*labels.Matcher) genericSeriesSet {
	m.mtx.Lock()
	m.sortedSeriesRequested = append(m.sortedSeriesRequested, b)
	m.mtx.Unlock()
	return &mockGenericSeriesSet{resp: m.resp, warnings: m.warnings, err: m.err}
}

func (m *mockGenericQuerier) LabelValues(_ context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	m.mtx.Lock()
	m.labelNamesRequested = append(m.labelNamesRequested, labelNameRequest{
		name:     name,
		matchers: matchers,
	})
	m.mtx.Unlock()
	return m.resp, m.warnings, m.err
}

func (m *mockGenericQuerier) LabelNames(context.Context, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	m.mtx.Lock()
	m.labelNamesCalls++
	m.mtx.Unlock()
	return m.resp, m.warnings, m.err
}

func (m *mockGenericQuerier) Close() error {
	m.closed = true
	return nil
}

type mockGenericSeriesSet struct {
	resp     []string
	warnings annotations.Annotations
	err      error

	curr int
}

func (m *mockGenericSeriesSet) Next() bool {
	if m.err != nil {
		return false
	}
	if m.curr >= len(m.resp) {
		return false
	}
	m.curr++
	return true
}

func (m *mockGenericSeriesSet) Err() error                        { return m.err }
func (m *mockGenericSeriesSet) Warnings() annotations.Annotations { return m.warnings }

func (m *mockGenericSeriesSet) At() Labels {
	return mockLabels(m.resp[m.curr-1])
}

type mockLabels string

func (l mockLabels) Labels() labels.Labels {
	return labels.FromStrings("test", string(l))
}

func unwrapMockGenericQuerier(t *testing.T, qr genericQuerier) *mockGenericQuerier {
	m, ok := qr.(*mockGenericQuerier)
	if !ok {
		s, ok := qr.(*secondaryQuerier)
		require.True(t, ok, "expected secondaryQuerier got something else")
		m, ok = s.genericQuerier.(*mockGenericQuerier)
		require.True(t, ok, "expected mockGenericQuerier got something else")
	}
	return m
}

func TestMergeGenericQuerierWithSecondaries_ErrorHandling(t *testing.T) {
	var (
		errStorage  = errors.New("storage error")
		warnStorage = errors.New("storage warning")
		ctx         = context.Background()
	)
	for _, tcase := range []struct {
		name     string
		queriers []genericQuerier

		expectedSelectsSeries []labels.Labels
		expectedLabels        []string

		expectedWarnings annotations.Annotations
		expectedErrs     [4]error
	}{
		{
			name:     "one successful primary querier",
			queriers: []genericQuerier{&mockGenericQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil}},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels: []string{"a", "b"},
		},
		{
			name: "multiple successful primary queriers",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil},
				&mockGenericQuerier{resp: []string{"b", "c"}, warnings: nil, err: nil},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
				labels.FromStrings("test", "c"),
			},
			expectedLabels: []string{"a", "b", "c"},
		},
		{
			name:         "one failed primary querier",
			queriers:     []genericQuerier{&mockGenericQuerier{warnings: nil, err: errStorage}},
			expectedErrs: [4]error{errStorage, errStorage, errStorage, errStorage},
		},
		{
			name: "one successful primary querier with successful secondaries",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: nil, err: nil}},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"c"}, warnings: nil, err: nil}},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
				labels.FromStrings("test", "c"),
			},
			expectedLabels: []string{"a", "b", "c"},
		},
		{
			name: "one successful primary querier with empty response and successful secondaries",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{}, warnings: nil, err: nil},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: nil, err: nil}},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"c"}, warnings: nil, err: nil}},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "b"),
				labels.FromStrings("test", "c"),
			},
			expectedLabels: []string{"b", "c"},
		},
		{
			name: "one failed primary querier with successful secondaries",
			queriers: []genericQuerier{
				&mockGenericQuerier{warnings: nil, err: errStorage},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: nil, err: nil}},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"c"}, warnings: nil, err: nil}},
			},
			expectedErrs: [4]error{errStorage, errStorage, errStorage, errStorage},
		},
		{
			name: "one successful primary querier with failed secondaries",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{"a"}, warnings: nil, err: nil},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: nil, err: errStorage}},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"c"}, warnings: nil, err: errStorage}},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
			},
			expectedLabels:   []string{"a"},
			expectedWarnings: annotations.New().Add(errStorage),
		},
		{
			name: "successful queriers with warnings",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{"a"}, warnings: annotations.New().Add(warnStorage), err: nil},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: annotations.New().Add(warnStorage), err: nil}},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels:   []string{"a", "b"},
			expectedWarnings: annotations.New().Add(warnStorage),
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			q := &mergeGenericQuerier{
				queriers: tcase.queriers,
				mergeFn:  func(l ...Labels) Labels { return l[0] },
			}

			t.Run("Select", func(t *testing.T) {
				res := q.Select(context.Background(), false, nil)
				var lbls []labels.Labels
				for res.Next() {
					lbls = append(lbls, res.At().Labels())
				}
				require.Subset(t, tcase.expectedWarnings, res.Warnings())
				require.Equal(t, tcase.expectedErrs[0], res.Err())
				require.ErrorIs(t, res.Err(), tcase.expectedErrs[0], "expected error doesn't match")
				require.Equal(t, tcase.expectedSelectsSeries, lbls)

				for _, qr := range q.queriers {
					m := unwrapMockGenericQuerier(t, qr)

					exp := []bool{true}
					if len(q.queriers) == 1 {
						exp[0] = false
					}
					require.Equal(t, exp, m.sortedSeriesRequested)
				}
			})
			t.Run("LabelNames", func(t *testing.T) {
				res, w, err := q.LabelNames(ctx)
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[1], "expected error doesn't match")
				require.Equal(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				for _, qr := range q.queriers {
					m := unwrapMockGenericQuerier(t, qr)

					require.Equal(t, 1, m.labelNamesCalls)
				}
			})
			t.Run("LabelValues", func(t *testing.T) {
				res, w, err := q.LabelValues(ctx, "test")
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[2], "expected error doesn't match")
				require.Equal(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				for _, qr := range q.queriers {
					m := unwrapMockGenericQuerier(t, qr)

					require.Equal(t, []labelNameRequest{{name: "test"}}, m.labelNamesRequested)
				}
			})
			t.Run("LabelValuesWithMatchers", func(t *testing.T) {
				matcher := labels.MustNewMatcher(labels.MatchEqual, "otherLabel", "someValue")
				res, w, err := q.LabelValues(ctx, "test2", matcher)
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[3], "expected error doesn't match")
				require.Equal(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				for _, qr := range q.queriers {
					m := unwrapMockGenericQuerier(t, qr)

					require.Equal(t, []labelNameRequest{
						{name: "test"},
						{name: "test2", matchers: []*labels.Matcher{matcher}},
					}, m.labelNamesRequested)
				}
			})
		})
	}
}

type errIterator struct {
	err error
}

func (e errIterator) Next() chunkenc.ValueType {
	return chunkenc.ValNone
}

func (e errIterator) Seek(t int64) chunkenc.ValueType {
	return chunkenc.ValNone
}

func (e errIterator) At() (int64, float64) {
	return 0, 0
}

func (e errIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (e errIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (e errIterator) AtT() int64 {
	return 0
}

func (e errIterator) Err() error {
	return e.err
}
