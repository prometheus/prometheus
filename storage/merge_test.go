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
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			),
		},
		{
			name: "two queriers, one different series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			),
		},
		{
			name: "two time unsorted queriers, two series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}, sample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}, sample{4, 4}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}, sample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}},
				),
			),
		},
		{
			name: "five queriers, only two queriers have two time unsorted series each",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}, sample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}, sample{4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}, sample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}},
				),
			),
		},
		{
			name: "two queriers, only two queriers have two time unsorted series each, with 3 noop and one nil querier together",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}, sample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}, sample{4, 4}}),
			}, {}},
			extraQueriers: []Querier{NoopQuerier(), NoopQuerier(), nil, NoopQuerier()},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}, sample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}},
				),
			),
		},
		{
			name: "two queriers, with two series, one is overlapping",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{2, 21}, sample{3, 31}, sample{5, 5}, sample{6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 22}, sample{3, 32}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}, sample{4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 21}, sample{3, 31}, sample{5, 5}, sample{6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{1, 1}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}, sample{1, 1}}),
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

			mergedQuerier := NewMergeQuerier([]Querier{p}, qs, ChainedSeriesMerge).Select(false, nil)

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

				expSmpl, expErr := ExpandSamples(expectedSeries.Iterator(), nil)
				actSmpl, actErr := ExpandSamples(actualSeries.Iterator(), nil)
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			),
		},
		{
			name: "two secondaries, one different series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			),
		},
		{
			name: "two secondaries, two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}}, []tsdbutil.Sample{sample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}}, []tsdbutil.Sample{sample{4, 4}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{5, 5}},
					[]tsdbutil.Sample{sample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
					[]tsdbutil.Sample{sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{4, 4}},
				),
			),
		},
		{
			name: "five secondaries, only two have two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{}, {}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}}, []tsdbutil.Sample{sample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}}, []tsdbutil.Sample{sample{4, 4}}),
			}, {}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{5, 5}},
					[]tsdbutil.Sample{sample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
					[]tsdbutil.Sample{sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{4, 4}},
				),
			),
		},
		{
			name: "two secondaries, with two not in time order series each, with 3 noop queries and one nil together",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{5, 5}}, []tsdbutil.Sample{sample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}}, []tsdbutil.Sample{sample{2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{3, 3}}, []tsdbutil.Sample{sample{4, 4}}),
			}},
			extraQueriers: []ChunkQuerier{NoopChunkedQuerier(), NoopChunkedQuerier(), nil, NoopChunkedQuerier()},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{5, 5}},
					[]tsdbutil.Sample{sample{6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
					[]tsdbutil.Sample{sample{2, 2}},
					[]tsdbutil.Sample{sample{3, 3}},
					[]tsdbutil.Sample{sample{4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{1, 1}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []tsdbutil.Sample{sample{0, math.NaN()}}, []tsdbutil.Sample{sample{1, 1}}),
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

			merged := NewMergeChunkQuerier([]ChunkQuerier{p}, qs, NewCompactingChunkSeriesMerger(nil)).Select(false, nil)
			for merged.Next() {
				require.True(t, tc.expected.Next(), "Expected Next() to be true")
				actualSeries := merged.At()
				expectedSeries := tc.expected.At()
				require.Equal(t, expectedSeries.Labels(), actualSeries.Labels())

				expChks, expErr := ExpandChunks(expectedSeries.Iterator())
				actChks, actErr := ExpandChunks(actualSeries.Iterator())
				require.Equal(t, expErr, actErr)
				require.Equal(t, expChks, actChks)

			}
			require.NoError(t, merged.Err())
			require.False(t, tc.expected.Next(), "Expected Next() to be false")
		})
	}
}

func TestCompactingChunkSeriesMerger(t *testing.T) {
	m := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)

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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}}),
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}, sample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{7, 7}, sample{9, 9}}, []tsdbutil.Sample{sample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}, sample{5, 5}}, []tsdbutil.Sample{sample{7, 7}, sample{9, 9}}, []tsdbutil.Sample{sample{10, 10}}),
		},
		{
			name: "two overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}, sample{8, 8}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{7, 7}, sample{9, 9}}, []tsdbutil.Sample{sample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}}, []tsdbutil.Sample{sample{3, 3}, sample{7, 7}, sample{8, 8}, sample{9, 9}}, []tsdbutil.Sample{sample{10, 10}}),
		},
		{
			name: "two duplicated",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{5, 5}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}}),
		},
		{
			name: "three overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{0, 0}, sample{4, 4}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}, sample{6, 6}}),
		},
		{
			name: "three in chained overlap",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{4, 4}, sample{6, 66}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{6, 6}, sample{10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}, sample{6, 66}, sample{10, 10}}),
		},
		{
			name: "three in chained overlap complex",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{0, 0}, sample{5, 5}}, []tsdbutil.Sample{sample{10, 10}, sample{15, 15}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{2, 2}, sample{20, 20}}, []tsdbutil.Sample{sample{25, 25}, sample{30, 30}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []tsdbutil.Sample{sample{18, 18}, sample{26, 26}}, []tsdbutil.Sample{sample{31, 31}, sample{35, 35}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]tsdbutil.Sample{sample{0, 0}, sample{2, 2}, sample{5, 5}, sample{10, 10}, sample{15, 15}, sample{18, 18}, sample{20, 20}, sample{25, 25}, sample{26, 26}, sample{30, 30}},
				[]tsdbutil.Sample{sample{31, 31}, sample{35, 35}},
			),
		},
		{
			name: "110 overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), tsdbutil.GenerateSamples(0, 110)), // [0 - 110)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), tsdbutil.GenerateSamples(60, 50)), // [60 - 110)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				tsdbutil.GenerateSamples(0, 110),
			),
		},
		{
			name: "150 overlapping samples, split chunk",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), tsdbutil.GenerateSamples(0, 90)),  // [0 - 90)
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), tsdbutil.GenerateSamples(60, 90)), // [90 - 150)
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				tsdbutil.GenerateSamples(0, 120),
				tsdbutil.GenerateSamples(120, 30),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged := m(tc.input...)
			require.Equal(t, tc.expected.Labels(), merged.Labels())
			actChks, actErr := ExpandChunks(merged.Iterator())
			expChks, expErr := ExpandChunks(tc.expected.Iterator())

			require.Equal(t, expErr, actErr)
			require.Equal(t, expChks, actChks)
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

func (m *mockQuerier) Select(sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) SeriesSet {
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

func (m *mockChunkQurier) Select(sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) ChunkSeriesSet {
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

func (m *mockSeriesSet) Warnings() Warnings { return nil }

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

func (m *mockChunkSeriesSet) Warnings() Warnings { return nil }

func TestChainSampleIterator(t *testing.T) {
	for _, tc := range []struct {
		input    []chunkenc.Iterator
		expected []tsdbutil.Sample
	}{
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}},
		},
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}}),
				NewListSeriesIterator(samples{sample{2, 2}, sample{3, 3}}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}},
		},
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{3, 3}}),
				NewListSeriesIterator(samples{sample{1, 1}, sample{4, 4}}),
				NewListSeriesIterator(samples{sample{2, 2}, sample{5, 5}}),
			},
			expected: []tsdbutil.Sample{
				sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}},
		},
		// Overlap.
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}}),
				NewListSeriesIterator(samples{sample{0, 0}, sample{2, 2}}),
				NewListSeriesIterator(samples{sample{2, 2}, sample{3, 3}}),
				NewListSeriesIterator(samples{}),
				NewListSeriesIterator(samples{}),
				NewListSeriesIterator(samples{}),
			},
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}},
		},
	} {
		merged := newChainSampleIterator(tc.input)
		actual, err := ExpandSamples(merged, nil)
		require.NoError(t, err)
		require.Equal(t, tc.expected, actual)
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
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			},
			seek:     1,
			expected: []tsdbutil.Sample{sample{1, 1}, sample{2, 2}},
		},
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}}),
				NewListSeriesIterator(samples{sample{2, 2}, sample{3, 3}}),
			},
			seek:     2,
			expected: []tsdbutil.Sample{sample{2, 2}, sample{3, 3}},
		},
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{3, 3}}),
				NewListSeriesIterator(samples{sample{1, 1}, sample{4, 4}}),
				NewListSeriesIterator(samples{sample{2, 2}, sample{5, 5}}),
			},
			seek:     2,
			expected: []tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{4, 4}, sample{5, 5}},
		},
		{
			input: []chunkenc.Iterator{
				NewListSeriesIterator(samples{sample{0, 0}, sample{2, 2}, sample{3, 3}}),
				NewListSeriesIterator(samples{sample{0, 0}, sample{1, 1}, sample{2, 2}}),
			},
			seek:     0,
			expected: []tsdbutil.Sample{sample{0, 0}, sample{1, 1}, sample{2, 2}, sample{3, 3}},
		},
	} {
		merged := newChainSampleIterator(tc.input)
		actual := []tsdbutil.Sample{}
		if merged.Seek(tc.seek) {
			t, v := merged.At()
			actual = append(actual, sample{t, v})
		}
		s, err := ExpandSamples(merged, nil)
		require.NoError(t, err)
		actual = append(actual, s...)
		require.Equal(t, tc.expected, actual)
	}
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
		series = append(series, NewListSeries(labels, samples))
	}
	return NewMockSeriesSet(series...)
}

func makeMergeSeriesSet(numSeriesSets, numSeries, numSamples int) SeriesSet {
	seriesSets := []genericSeriesSet{}
	for i := 0; i < numSeriesSets; i++ {
		seriesSets = append(seriesSets, &genericSeriesSetAdapter{makeSeriesSet(numSeries, numSamples)})
	}
	return &seriesSetAdapter{newGenericMergeSeriesSet(seriesSets, (&seriesMergerAdapter{VerticalSeriesMergeFunc: ChainedSeriesMerge}).Merge)}
}

func benchmarkDrain(seriesSet SeriesSet, b *testing.B) {
	var err error
	for n := 0; n < b.N; n++ {
		for seriesSet.Next() {
			result, err = ExpandSamples(seriesSet.At().Iterator(), nil)
			require.NoError(b, err)
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

type mockGenericQuerier struct {
	mtx sync.Mutex

	closed                bool
	labelNamesCalls       int
	labelNamesRequested   []labelNameRequest
	sortedSeriesRequested []bool

	resp     []string
	warnings Warnings
	err      error
}

type labelNameRequest struct {
	name     string
	matchers []*labels.Matcher
}

func (m *mockGenericQuerier) Select(b bool, _ *SelectHints, _ ...*labels.Matcher) genericSeriesSet {
	m.mtx.Lock()
	m.sortedSeriesRequested = append(m.sortedSeriesRequested, b)
	m.mtx.Unlock()
	return &mockGenericSeriesSet{resp: m.resp, warnings: m.warnings, err: m.err}
}

func (m *mockGenericQuerier) LabelValues(name string, matchers ...*labels.Matcher) ([]string, Warnings, error) {
	m.mtx.Lock()
	m.labelNamesRequested = append(m.labelNamesRequested, labelNameRequest{
		name:     name,
		matchers: matchers,
	})
	m.mtx.Unlock()
	return m.resp, m.warnings, m.err
}

func (m *mockGenericQuerier) LabelNames() ([]string, Warnings, error) {
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
	warnings Warnings
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

func (m *mockGenericSeriesSet) Err() error         { return m.err }
func (m *mockGenericSeriesSet) Warnings() Warnings { return m.warnings }

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
	)
	for _, tcase := range []struct {
		name     string
		queriers []genericQuerier

		expectedSelectsSeries []labels.Labels
		expectedLabels        []string

		expectedWarnings [4]Warnings
		expectedErrs     [4]error
	}{
		{},
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
			expectedLabels: []string{"a"},
			expectedWarnings: [4]Warnings{
				[]error{errStorage, errStorage},
				[]error{errStorage, errStorage},
				[]error{errStorage, errStorage},
				[]error{errStorage, errStorage},
			},
		},
		{
			name: "successful queriers with warnings",
			queriers: []genericQuerier{
				&mockGenericQuerier{resp: []string{"a"}, warnings: []error{warnStorage}, err: nil},
				&secondaryQuerier{genericQuerier: &mockGenericQuerier{resp: []string{"b"}, warnings: []error{warnStorage}, err: nil}},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels: []string{"a", "b"},
			expectedWarnings: [4]Warnings{
				[]error{warnStorage, warnStorage},
				[]error{warnStorage, warnStorage},
				[]error{warnStorage, warnStorage},
				[]error{warnStorage, warnStorage},
			},
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			q := &mergeGenericQuerier{
				queriers: tcase.queriers,
				mergeFn:  func(l ...Labels) Labels { return l[0] },
			}

			t.Run("Select", func(t *testing.T) {
				res := q.Select(false, nil)
				var lbls []labels.Labels
				for res.Next() {
					lbls = append(lbls, res.At().Labels())
				}
				require.Equal(t, tcase.expectedWarnings[0], res.Warnings())
				require.Equal(t, tcase.expectedErrs[0], res.Err())
				require.True(t, errors.Is(res.Err(), tcase.expectedErrs[0]), "expected error doesn't match")
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
				res, w, err := q.LabelNames()
				require.Equal(t, tcase.expectedWarnings[1], w)
				require.True(t, errors.Is(err, tcase.expectedErrs[1]), "expected error doesn't match")
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
				res, w, err := q.LabelValues("test")
				require.Equal(t, tcase.expectedWarnings[2], w)
				require.True(t, errors.Is(err, tcase.expectedErrs[2]), "expected error doesn't match")
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
				res, w, err := q.LabelValues("test2", matcher)
				require.Equal(t, tcase.expectedWarnings[3], w)
				require.True(t, errors.Is(err, tcase.expectedErrs[3]), "expected error doesn't match")
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
