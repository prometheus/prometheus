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
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			),
		},
		{
			name: "two queriers, one different series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			),
		},
		{
			name: "two time unsorted queriers, two series each",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}, fSample{0, 6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}, fSample{0, 4, 4}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}, fSample{0, 6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "five queriers, only two queriers have two time unsorted series each",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}, fSample{0, 6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}, fSample{0, 4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}, fSample{0, 6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "two queriers, only two queriers have two time unsorted series each, with 3 noop and one nil querier together",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}, fSample{0, 6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}, fSample{0, 4, 4}}),
			}, {}},
			extraQueriers: []Querier{NoopQuerier(), NoopQuerier(), nil, NoopQuerier()},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}, fSample{0, 6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "two queriers, with two series, one is overlapping",
			querierSeries: [][]Series{{}, {}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 21}, fSample{0, 3, 31}, fSample{0, 5, 5}, fSample{0, 6, 6}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}}),
			}, {
				NewListSeries(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 22}, fSample{0, 3, 32}}),
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}, fSample{0, 4, 4}}),
			}, {}},
			expected: NewMockSeriesSet(
				NewListSeries(
					labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 21}, fSample{0, 3, 31}, fSample{0, 5, 5}, fSample{0, 6, 6}},
				),
				NewListSeries(
					labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			querierSeries: [][]Series{{
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, math.NaN()}}),
			}, {
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 1, 1}}),
			}},
			expected: NewMockSeriesSet(
				NewListSeries(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, math.NaN()}, fSample{0, 1, 1}}),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p []Querier
			if tc.primaryQuerierSeries != nil {
				p = append(p, &mockQuerier{toReturn: tc.primaryQuerierSeries})
			}
			var qs []Querier
			for _, in := range tc.querierSeries {
				qs = append(qs, &mockQuerier{toReturn: in})
			}
			qs = append(qs, tc.extraQueriers...)

			mergedQuerier := NewMergeQuerier(p, qs, ChainedSeriesMerge).Select(context.Background(), false, nil)

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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			),
		},
		{
			name: "two secondaries, one different series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			),
		},
		{
			name: "two secondaries, two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}}, []chunks.Sample{fSample{0, 4, 4}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 5, 5}},
					[]chunks.Sample{fSample{0, 6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}},
					[]chunks.Sample{fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "five secondaries, only two have two not in time order series each",
			chkQuerierSeries: [][]ChunkSeries{{}, {}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}}, []chunks.Sample{fSample{0, 4, 4}}),
			}, {}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 5, 5}},
					[]chunks.Sample{fSample{0, 6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}},
					[]chunks.Sample{fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "two secondaries, with two not in time order series each, with 3 noop queries and one nil together",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}}, []chunks.Sample{fSample{0, 2, 2}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 3, 3}}, []chunks.Sample{fSample{0, 4, 4}}),
			}},
			extraQueriers: []ChunkQuerier{NoopChunkedQuerier(), NoopChunkedQuerier(), nil, NoopChunkedQuerier()},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
					[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 5, 5}},
					[]chunks.Sample{fSample{0, 6, 6}},
				),
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"),
					[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}},
					[]chunks.Sample{fSample{0, 2, 2}},
					[]chunks.Sample{fSample{0, 3, 3}},
					[]chunks.Sample{fSample{0, 4, 4}},
				),
			),
		},
		{
			name: "two queries, one with NaN samples series",
			chkQuerierSeries: [][]ChunkSeries{{
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, math.NaN()}}),
			}, {
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 1, 1}}),
			}},
			expected: NewMockChunkSeriesSet(
				NewListChunkSeriesFromSamples(labels.FromStrings("foo", "bar"), []chunks.Sample{fSample{0, 0, math.NaN()}}, []chunks.Sample{fSample{0, 1, 1}}),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p []ChunkQuerier
			if tc.primaryChkQuerierSeries != nil {
				p = append(p, &mockChunkQuerier{toReturn: tc.primaryChkQuerierSeries})
			}

			var qs []ChunkQuerier
			for _, in := range tc.chkQuerierSeries {
				qs = append(qs, &mockChunkQuerier{toReturn: in})
			}
			qs = append(qs, tc.extraQueriers...)

			merged := NewMergeChunkQuerier(p, qs, NewCompactingChunkSeriesMerger(nil)).Select(context.Background(), false, nil)
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
	h := tsdbutil.GenerateTestHistogram(ts + 1)
	h.CounterResetHint = hint
	return hSample{st: -ts, t: ts, h: h}
}

func floatHistogramSample(ts int64, hint histogram.CounterResetHint) fhSample {
	fh := tsdbutil.GenerateTestFloatHistogram(ts + 1)
	fh.CounterResetHint = hint
	return fhSample{st: -ts, t: ts, fh: fh}
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
		},
		{
			name: "two overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 8, 8}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 7, 7}, fSample{0, 8, 8}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
		},
		{
			name: "two duplicated",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
		},
		{
			name: "three overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 4, 4}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}, fSample{0, 5, 5}, fSample{0, 6, 6}}),
		},
		{
			name: "three in chained overlap",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 4, 4}, fSample{0, 6, 66}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 6, 6}, fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 4, 4}, fSample{0, 5, 5}, fSample{0, 6, 66}, fSample{0, 10, 10}}),
		},
		{
			name: "three in chained overlap complex",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 10, 10}, fSample{0, 15, 15}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 20, 20}}, []chunks.Sample{fSample{0, 25, 25}, fSample{0, 30, 30}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 18, 18}, fSample{0, 26, 26}}, []chunks.Sample{fSample{0, 31, 31}, fSample{0, 35, 35}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 2, 2}, fSample{0, 5, 5}, fSample{0, 10, 10}, fSample{0, 15, 15}, fSample{0, 18, 18}, fSample{0, 20, 20}, fSample{0, 25, 25}, fSample{0, 26, 26}, fSample{0, 30, 30}},
				[]chunks.Sample{fSample{0, 31, 31}, fSample{0, 35, 35}},
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 12, 12}}, []chunks.Sample{fSample{0, 14, 14}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{histogramSample(0)},
				[]chunks.Sample{fSample{0, 1, 1}},
				[]chunks.Sample{histogramSample(5), histogramSample(10)},
				[]chunks.Sample{fSample{0, 12, 12}, fSample{0, 14, 14}},
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 12, 12}}, []chunks.Sample{fSample{0, 14, 14}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{floatHistogramSample(0)},
				[]chunks.Sample{fSample{0, 1, 1}},
				[]chunks.Sample{floatHistogramSample(5), floatHistogramSample(10)},
				[]chunks.Sample{fSample{0, 12, 12}, fSample{0, 14, 14}},
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}}),
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
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
		},
		{
			name: "two overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 8, 8}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}}, []chunks.Sample{fSample{0, 3, 3}, fSample{0, 8, 8}},
				[]chunks.Sample{fSample{0, 7, 7}, fSample{0, 9, 9}}, []chunks.Sample{fSample{0, 10, 10}},
			),
		},
		{
			name: "two duplicated",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}},
				[]chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}},
			),
		},
		{
			name: "three overlapping",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 6, 6}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 4, 4}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}},
				[]chunks.Sample{fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 6, 6}},
				[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 4, 4}},
			),
		},
		{
			name: "three in chained overlap",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 4, 4}, fSample{0, 6, 66}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 6, 6}, fSample{0, 10, 10}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 1, 1}, fSample{0, 2, 2}, fSample{0, 3, 3}, fSample{0, 5, 5}},
				[]chunks.Sample{fSample{0, 4, 4}, fSample{0, 6, 66}},
				[]chunks.Sample{fSample{0, 6, 6}, fSample{0, 10, 10}},
			),
		},
		{
			name: "three in chained overlap complex",
			input: []ChunkSeries{
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 0, 0}, fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 10, 10}, fSample{0, 15, 15}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 2, 2}, fSample{0, 20, 20}}, []chunks.Sample{fSample{0, 25, 25}, fSample{0, 30, 30}}),
				NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"), []chunks.Sample{fSample{0, 18, 18}, fSample{0, 26, 26}}, []chunks.Sample{fSample{0, 31, 31}, fSample{0, 35, 35}}),
			},
			expected: NewListChunkSeriesFromSamples(labels.FromStrings("bar", "baz"),
				[]chunks.Sample{fSample{0, 0, 0}, fSample{0, 5, 5}}, []chunks.Sample{fSample{0, 10, 10}, fSample{0, 15, 15}},
				[]chunks.Sample{fSample{0, 2, 2}, fSample{0, 20, 20}}, []chunks.Sample{fSample{0, 25, 25}, fSample{0, 30, 30}},
				[]chunks.Sample{fSample{0, 18, 18}, fSample{0, 26, 26}}, []chunks.Sample{fSample{0, 31, 31}, fSample{0, 35, 35}},
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
	mtx sync.Mutex

	toReturn []Series // Response for Select.

	closed                bool
	labelNamesCalls       int
	labelNamesRequested   []labelNameRequest
	sortedSeriesRequested []bool

	resp     []string // Response for LabelNames and LabelValues; turned into Select response if toReturn is not supplied.
	warnings annotations.Annotations
	err      error
}

type labelNameRequest struct {
	name     string
	matchers []*labels.Matcher
}

type seriesByLabel []Series

func (a seriesByLabel) Len() int           { return len(a) }
func (a seriesByLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a seriesByLabel) Less(i, j int) bool { return labels.Compare(a[i].Labels(), a[j].Labels()) < 0 }

func (m *mockQuerier) Select(_ context.Context, sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) SeriesSet {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.sortedSeriesRequested = append(m.sortedSeriesRequested, sortSeries)

	var ret []Series
	if len(m.toReturn) > 0 {
		ret = make([]Series, len(m.toReturn))
		copy(ret, m.toReturn)
	} else if len(m.resp) > 0 {
		ret = make([]Series, 0, len(m.resp))
		for _, l := range m.resp {
			ret = append(ret, NewListSeries(labels.FromStrings("test", l), nil))
		}
	}
	if sortSeries {
		sort.Sort(seriesByLabel(ret))
	}

	return &mockSeriesSet{idx: -1, series: ret, warnings: m.warnings, err: m.err}
}

func (m *mockQuerier) LabelValues(_ context.Context, name string, _ *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	m.mtx.Lock()
	m.labelNamesRequested = append(m.labelNamesRequested, labelNameRequest{
		name:     name,
		matchers: matchers,
	})
	m.mtx.Unlock()
	return m.resp, m.warnings, m.err
}

func (m *mockQuerier) LabelNames(context.Context, *LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	m.mtx.Lock()
	m.labelNamesCalls++
	m.mtx.Unlock()
	return m.resp, m.warnings, m.err
}

func (m *mockQuerier) Close() error {
	m.closed = true
	return nil
}

type mockChunkQuerier struct {
	LabelQuerier

	toReturn []ChunkSeries
}

type chunkSeriesByLabel []ChunkSeries

func (a chunkSeriesByLabel) Len() int      { return len(a) }
func (a chunkSeriesByLabel) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a chunkSeriesByLabel) Less(i, j int) bool {
	return labels.Compare(a[i].Labels(), a[j].Labels()) < 0
}

func (m *mockChunkQuerier) Select(_ context.Context, sortSeries bool, _ *SelectHints, _ ...*labels.Matcher) ChunkSeriesSet {
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

	warnings annotations.Annotations
	err      error
}

func NewMockSeriesSet(series ...Series) SeriesSet {
	return &mockSeriesSet{
		idx:    -1,
		series: series,
	}
}

func (m *mockSeriesSet) Next() bool {
	if m.err != nil {
		return false
	}
	m.idx++
	return m.idx < len(m.series)
}

func (m *mockSeriesSet) At() Series { return m.series[m.idx] }

func (m *mockSeriesSet) Err() error { return m.err }

func (m *mockSeriesSet) Warnings() annotations.Annotations { return m.warnings }

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

func (*mockChunkSeriesSet) Err() error { return nil }

func (*mockChunkSeriesSet) Warnings() annotations.Annotations { return nil }

func TestChainSampleIterator(t *testing.T) {
	for sampleType, sampleFunc := range map[string]func(int64) chunks.Sample{
		"float":           func(ts int64) chunks.Sample { return fSample{-ts, ts, float64(ts)} },
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
		"float":           func(ts int64) chunks.Sample { return fSample{-ts, ts, float64(ts)} },
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
					actual = append(actual, fSample{merged.AtST(), t, f})
				case chunkenc.ValHistogram:
					t, h := merged.AtHistogram(nil)
					actual = append(actual, hSample{merged.AtST(), t, h})
				case chunkenc.ValFloatHistogram:
					t, fh := merged.AtFloatHistogram(nil)
					actual = append(actual, fhSample{merged.AtST(), t, fh})
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
		NewListSeriesIterator(samples{fSample{0, 0, 0.1}, fSample{0, 1, 1.1}, fSample{0, 2, 2.1}}),
		errIterator{errors.New("something went wrong")},
	})

	require.Equal(t, chunkenc.ValNone, merged.Seek(0))
	require.EqualError(t, merged.Err(), "something went wrong")
}

func TestChainSampleIteratorNextImmediatelyFailingIterator(t *testing.T) {
	merged := ChainSampleIteratorFromIterators(nil, []chunkenc.Iterator{
		NewListSeriesIterator(samples{fSample{0, 0, 0.1}, fSample{0, 1, 1.1}, fSample{0, 2, 2.1}}),
		errIterator{errors.New("something went wrong")},
	})

	require.Equal(t, chunkenc.ValNone, merged.Next())
	require.EqualError(t, merged.Err(), "something went wrong")

	// Next() does some special handling for the first iterator, so make sure it handles the first iterator returning an error too.
	merged = ChainSampleIteratorFromIterators(nil, []chunkenc.Iterator{
		errIterator{errors.New("something went wrong")},
		NewListSeriesIterator(samples{fSample{0, 0, 0.1}, fSample{0, 1, 1.1}, fSample{0, 2, 2.1}}),
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
					actual = append(actual, fSample{merged.AtST(), t, f})
				case chunkenc.ValHistogram:
					t, h := merged.AtHistogram(nil)
					actual = append(actual, hSample{merged.AtST(), t, h})
				case chunkenc.ValFloatHistogram:
					t, fh := merged.AtFloatHistogram(nil)
					actual = append(actual, fhSample{merged.AtST(), t, fh})
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
	for j := range numSeries {
		labels := labels.FromStrings("foo", fmt.Sprintf("bar%d", j))
		samples := []chunks.Sample{}
		for k := range numSamples {
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
	return &seriesSetAdapter{newGenericMergeSeriesSet(seriesSets, 0, (&seriesMergerAdapter{VerticalSeriesMergeFunc: ChainedSeriesMerge}).Merge)}
}

func benchmarkDrain(b *testing.B, makeSeriesSet func() SeriesSet) {
	var err error
	var t int64
	var v float64
	var iter chunkenc.Iterator
	for b.Loop() {
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

func BenchmarkMergeLabelValuesWithLimit(b *testing.B) {
	var queriers []genericQuerier

	for i := range 5 {
		var lbls []string
		for j := range 100000 {
			lbls = append(lbls, fmt.Sprintf("querier_%d_label_%d", i, j))
		}
		q := &mockQuerier{resp: lbls}
		queriers = append(queriers, newGenericQuerierFrom(q))
	}

	mergeQuerier := &mergeGenericQuerier{
		queriers: queriers, // Assume querying 5 blocks.
		mergeFn: func(l ...Labels) Labels {
			return l[0]
		},
	}

	b.Run("benchmark", func(*testing.B) {
		ctx := context.Background()
		hints := &LabelHints{
			Limit: 1000,
		}
		mergeQuerier.LabelValues(ctx, "name", hints)
	})
}

func visitMockQueriers(t *testing.T, qr Querier, f func(t *testing.T, q *mockQuerier)) int {
	count := 0
	switch x := qr.(type) {
	case *mockQuerier:
		count++
		f(t, x)
	case *querierAdapter:
		count += visitMockQueriersInGenericQuerier(t, x.genericQuerier, f)
	}
	return count
}

func visitMockQueriersInGenericQuerier(t *testing.T, g genericQuerier, f func(t *testing.T, q *mockQuerier)) int {
	count := 0
	switch x := g.(type) {
	case *mergeGenericQuerier:
		for _, q := range x.queriers {
			count += visitMockQueriersInGenericQuerier(t, q, f)
		}
	case *genericQuerierAdapter:
		// Visitor for chunkQuerier not implemented.
		count += visitMockQueriers(t, x.q, f)
	case *secondaryQuerier:
		count += visitMockQueriersInGenericQuerier(t, x.genericQuerier, f)
	}
	return count
}

func TestMergeQuerierWithSecondaries_ErrorHandling(t *testing.T) {
	var (
		errStorage  = errors.New("storage error")
		warnStorage = errors.New("storage warning")
		ctx         = context.Background()
	)
	for _, tcase := range []struct {
		name        string
		primaries   []Querier
		secondaries []Querier
		limit       int

		expectedSelectsSeries []labels.Labels
		expectedLabels        []string

		expectedWarnings annotations.Annotations
		expectedErrs     [4]error
	}{
		{
			name:      "one successful primary querier",
			primaries: []Querier{&mockQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil}},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels: []string{"a", "b"},
		},
		{
			name: "multiple successful primary queriers",
			primaries: []Querier{
				&mockQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil},
				&mockQuerier{resp: []string{"b", "c"}, warnings: nil, err: nil},
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
			primaries:    []Querier{&mockQuerier{warnings: nil, err: errStorage}},
			expectedErrs: [4]error{errStorage, errStorage, errStorage, errStorage},
		},
		{
			name: "one successful primary querier with successful secondaries",
			primaries: []Querier{
				&mockQuerier{resp: []string{"a", "b"}, warnings: nil, err: nil},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: nil},
				&mockQuerier{resp: []string{"c"}, warnings: nil, err: nil},
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
			primaries: []Querier{
				&mockQuerier{resp: []string{}, warnings: nil, err: nil},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: nil},
				&mockQuerier{resp: []string{"c"}, warnings: nil, err: nil},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "b"),
				labels.FromStrings("test", "c"),
			},
			expectedLabels: []string{"b", "c"},
		},
		{
			name: "one failed primary querier with successful secondaries",
			primaries: []Querier{
				&mockQuerier{warnings: nil, err: errStorage},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: nil},
				&mockQuerier{resp: []string{"c"}, warnings: nil, err: nil},
			},
			expectedErrs: [4]error{errStorage, errStorage, errStorage, errStorage},
		},
		{
			name:      "nil primary querier with failed secondary",
			primaries: nil,
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: errStorage},
			},
			expectedLabels:   []string{},
			expectedWarnings: annotations.New().Add(errStorage),
		},
		{
			name:      "nil primary querier with two failed secondaries",
			primaries: nil,
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: errStorage},
				&mockQuerier{resp: []string{"c"}, warnings: nil, err: errStorage},
			},
			expectedLabels:   []string{},
			expectedWarnings: annotations.New().Add(errStorage),
		},
		{
			name: "one successful primary querier with failed secondaries",
			primaries: []Querier{
				&mockQuerier{resp: []string{"a"}, warnings: nil, err: nil},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: nil, err: errStorage},
				&mockQuerier{resp: []string{"c"}, warnings: nil, err: errStorage},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
			},
			expectedLabels:   []string{"a"},
			expectedWarnings: annotations.New().Add(errStorage),
		},
		{
			name: "successful queriers with warnings",
			primaries: []Querier{
				&mockQuerier{resp: []string{"a"}, warnings: annotations.New().Add(warnStorage), err: nil},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b"}, warnings: annotations.New().Add(warnStorage), err: nil},
			},
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels:   []string{"a", "b"},
			expectedWarnings: annotations.New().Add(warnStorage),
		},
		{
			name: "successful queriers with limit",
			primaries: []Querier{
				&mockQuerier{resp: []string{"a", "d"}, warnings: annotations.New().Add(warnStorage), err: nil},
			},
			secondaries: []Querier{
				&mockQuerier{resp: []string{"b", "c"}, warnings: annotations.New().Add(warnStorage), err: nil},
			},
			limit: 2,
			expectedSelectsSeries: []labels.Labels{
				labels.FromStrings("test", "a"),
				labels.FromStrings("test", "b"),
			},
			expectedLabels:   []string{"a", "b"},
			expectedWarnings: annotations.New().Add(warnStorage),
		},
	} {
		var labelHints *LabelHints
		var selectHints *SelectHints
		if tcase.limit > 0 {
			labelHints = &LabelHints{
				Limit: tcase.limit,
			}
			selectHints = &SelectHints{
				Limit: tcase.limit,
			}
		}

		t.Run(tcase.name, func(t *testing.T) {
			q := NewMergeQuerier(tcase.primaries, tcase.secondaries, func(s ...Series) Series { return s[0] })

			t.Run("Select", func(t *testing.T) {
				res := q.Select(context.Background(), false, selectHints)
				var lbls []labels.Labels
				for res.Next() {
					lbls = append(lbls, res.At().Labels())
				}
				require.Subset(t, tcase.expectedWarnings, res.Warnings())
				require.Equal(t, tcase.expectedErrs[0], res.Err())
				require.ErrorIs(t, res.Err(), tcase.expectedErrs[0], "expected error doesn't match")
				require.Equal(t, tcase.expectedSelectsSeries, lbls)

				n := visitMockQueriers(t, q, func(t *testing.T, m *mockQuerier) {
					// Single queries should be unsorted; merged queries sorted.
					exp := len(tcase.primaries)+len(tcase.secondaries) > 1
					require.Equal(t, []bool{exp}, m.sortedSeriesRequested)
				})
				// Check we visited all queriers.
				require.Equal(t, len(tcase.primaries)+len(tcase.secondaries), n)
			})
			t.Run("LabelNames", func(t *testing.T) {
				res, w, err := q.LabelNames(ctx, labelHints)
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[1], "expected error doesn't match")
				requireEqualSlice(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				visitMockQueriers(t, q, func(t *testing.T, m *mockQuerier) {
					require.Equal(t, 1, m.labelNamesCalls)
				})
			})
			t.Run("LabelValues", func(t *testing.T) {
				res, w, err := q.LabelValues(ctx, "test", labelHints)
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[2], "expected error doesn't match")
				requireEqualSlice(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				visitMockQueriers(t, q, func(t *testing.T, m *mockQuerier) {
					require.Equal(t, []labelNameRequest{{name: "test"}}, m.labelNamesRequested)
				})
			})
			t.Run("LabelValuesWithMatchers", func(t *testing.T) {
				matcher := labels.MustNewMatcher(labels.MatchEqual, "otherLabel", "someValue")
				res, w, err := q.LabelValues(ctx, "test2", labelHints, matcher)
				require.Subset(t, tcase.expectedWarnings, w)
				require.ErrorIs(t, err, tcase.expectedErrs[3], "expected error doesn't match")
				requireEqualSlice(t, tcase.expectedLabels, res)

				if err != nil {
					return
				}
				visitMockQueriers(t, q, func(t *testing.T, m *mockQuerier) {
					require.Equal(t, []labelNameRequest{
						{name: "test"},
						{name: "test2", matchers: []*labels.Matcher{matcher}},
					}, m.labelNamesRequested)
				})
			})
		})
	}
}

// Check slice but ignore difference between nil and empty.
func requireEqualSlice[T any](t require.TestingT, a, b []T, msgAndArgs ...any) {
	if len(a) == 0 {
		require.Empty(t, b, msgAndArgs...)
	} else {
		require.Equal(t, a, b, msgAndArgs...)
	}
}

type errIterator struct {
	err error
}

func (errIterator) Next() chunkenc.ValueType {
	return chunkenc.ValNone
}

func (errIterator) Seek(int64) chunkenc.ValueType {
	return chunkenc.ValNone
}

func (errIterator) At() (int64, float64) {
	return 0, 0
}

func (errIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (errIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (errIterator) AtT() int64 {
	return 0
}

func (errIterator) AtST() int64 {
	return 0
}

func (e errIterator) Err() error {
	return e.err
}

// searchQuerier is a Querier that also implements Searcher, for testing merge behaviour.
type searchQuerier struct {
	mockQuerier
	names       []SearchResult
	values      []SearchResult
	searchWarns annotations.Annotations
	err         error
}

func (q *searchQuerier) SearchLabelNames(_ context.Context, _ *SearchHints, _ ...*labels.Matcher) SearchResultSet {
	if q.err != nil {
		return ErrSearchResultSet(q.err)
	}
	return NewSearchResultSetFromSlice(q.names, q.searchWarns)
}

func (q *searchQuerier) SearchLabelValues(_ context.Context, _ string, _ *SearchHints, _ ...*labels.Matcher) SearchResultSet {
	if q.err != nil {
		return ErrSearchResultSet(q.err)
	}
	return NewSearchResultSetFromSlice(q.values, q.searchWarns)
}

// partialErrSearchQuerier is a Querier+Searcher that yields partial results
// then returns an error, for testing secondary querier mid-stream failure.
type partialErrSearchQuerier struct {
	mockQuerier
	names  []SearchResult
	values []SearchResult
	err    error
}

func (q *partialErrSearchQuerier) SearchLabelNames(_ context.Context, _ *SearchHints, _ ...*labels.Matcher) SearchResultSet {
	return newErrAfterResultsSet(q.names, q.err)
}

func (q *partialErrSearchQuerier) SearchLabelValues(_ context.Context, _ string, _ *SearchHints, _ ...*labels.Matcher) SearchResultSet {
	return newErrAfterResultsSet(q.values, q.err)
}

// errAfterResultsSet is a SearchResultSet that yields results then returns an error.
type errAfterResultsSet struct {
	results []SearchResult
	idx     int // starts at -1; incremented by Next.
	err     error
}

func newErrAfterResultsSet(results []SearchResult, err error) *errAfterResultsSet {
	return &errAfterResultsSet{results: results, err: err, idx: -1}
}

func (s *errAfterResultsSet) Next() bool {
	s.idx++
	return s.idx < len(s.results)
}

func (s *errAfterResultsSet) At() SearchResult { return s.results[s.idx] }

func (*errAfterResultsSet) Warnings() annotations.Annotations { return nil }

func (s *errAfterResultsSet) Err() error {
	if s.idx >= len(s.results) {
		return s.err
	}
	return nil
}

func (*errAfterResultsSet) Close() error { return nil }

func collectSearchResults(t *testing.T, rs SearchResultSet) []SearchResult {
	t.Helper()
	var got []SearchResult
	for rs.Next() {
		got = append(got, rs.At())
	}
	require.NoError(t, rs.Err())
	require.NoError(t, rs.Close())
	return got
}

func TestMergeQuerierSearch(t *testing.T) {
	ctx := t.Context()

	// newMerged creates a merged querier from two queriers, ensuring the querierAdapter
	// path is always taken (NewMergeQuerier returns the single querier directly if only
	// one is provided).
	newMerged := func(a, b Querier) Querier {
		return NewMergeQuerier([]Querier{a, b}, nil, ChainedSeriesMerge)
	}

	t.Run("results from both searchers are merged", func(t *testing.T) {
		q1 := &searchQuerier{
			names:  []SearchResult{{Value: "env", Score: 0.9}},
			values: []SearchResult{{Value: "prod", Score: 1.0}},
		}
		q2 := &searchQuerier{
			names:  []SearchResult{{Value: "job", Score: 0.5}},
			values: []SearchResult{{Value: "dev", Score: 0.8}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Len(t, got, 2)

		got = collectSearchResults(t, merged.(Searcher).SearchLabelValues(ctx, "env", nil))
		require.Len(t, got, 2)
	})

	t.Run("duplicate values keep max score", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 0.6}, {Value: "job", Score: 0.9}},
		}
		q2 := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 0.8}, {Value: "region", Score: 0.5}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Len(t, got, 3)
		scores := make(map[string]float64, len(got))
		for _, r := range got {
			scores[r.Value] = r.Score
		}
		// "env" appears in both; max score wins.
		require.Equal(t, 0.8, scores["env"])
		require.Equal(t, 0.9, scores["job"])
		require.Equal(t, 0.5, scores["region"])
	})

	t.Run("natural order is preserved for merged searchers", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "region", Score: 0.5}, {Value: "zone", Score: 0.4}},
		}
		q2 := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 0.8}, {Value: "job", Score: 0.9}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Equal(t, []SearchResult{
			{Value: "env", Score: 0.8},
			{Value: "job", Score: 0.9},
			{Value: "region", Score: 0.5},
			{Value: "zone", Score: 0.4},
		}, got)
	})

	t.Run("secondary search errors become warnings", func(t *testing.T) {
		primary := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 1.0}},
		}
		secondary := &searchQuerier{err: errors.New("secondary search failed")}
		merged := NewMergeQuerier([]Querier{primary}, []Querier{secondary}, ChainedSeriesMerge)
		defer merged.Close()

		rs := merged.(Searcher).SearchLabelNames(ctx, nil)
		got := collectSearchResults(t, rs)
		require.Equal(t, []SearchResult{{Value: "env", Score: 1.0}}, got)
		warnings := rs.Warnings().AsErrors()
		require.Len(t, warnings, 1)
		require.Contains(t, warnings[0].Error(), "secondary search failed")
	})

	t.Run("secondary partial label names followed by error become warnings", func(t *testing.T) {
		primary := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 1.0}},
		}
		secondary := &partialErrSearchQuerier{
			names: []SearchResult{{Value: "zone", Score: 0.5}},
			err:   errors.New("partial secondary failure"),
		}
		merged := NewMergeQuerier([]Querier{primary}, []Querier{secondary}, ChainedSeriesMerge)
		defer merged.Close()

		rs := merged.(Searcher).SearchLabelNames(ctx, nil)
		for rs.Next() {
		}
		require.NoError(t, rs.Err())
		warnings := rs.Warnings().AsErrors()
		require.Len(t, warnings, 1)
		require.Contains(t, warnings[0].Error(), "partial secondary failure")
		require.NoError(t, rs.Close())
	})

	t.Run("warnings accessible after early Close", func(t *testing.T) {
		// Exercises the internal warningsOnErrorSearchSet lifecycle when
		// the caller closes before exhaustion. This cannot be tested
		// through the public API because the error-to-warning conversion
		// only fires on exhaustion.
		inner := newErrAfterResultsSet(
			[]SearchResult{{Value: "zone", Score: 0.5}},
			errors.New("early close failure"),
		)
		rs := warningsOnErrorSearchResultSet(inner)
		require.True(t, rs.Next())
		// Do not call Next again; close early.
		require.NoError(t, rs.Close())
		// Inner set has no warnings yet (error only fires after exhaustion),
		// but Close must not panic and Warnings must be callable.
		require.NoError(t, rs.Err())
		_ = rs.Warnings()
		// Calling Close a second time must be a no-op.
		require.NoError(t, rs.Close())
	})

	t.Run("secondary partial label values followed by error become warnings", func(t *testing.T) {
		primary := &searchQuerier{
			values: []SearchResult{{Value: "prod", Score: 1.0}},
		}
		secondary := &partialErrSearchQuerier{
			values: []SearchResult{{Value: "staging", Score: 0.5}},
			err:    errors.New("partial secondary failure"),
		}
		merged := NewMergeQuerier([]Querier{primary}, []Querier{secondary}, ChainedSeriesMerge)
		defer merged.Close()

		rs := merged.(Searcher).SearchLabelValues(ctx, "env", nil)
		for rs.Next() {
		}
		require.NoError(t, rs.Err())
		warnings := rs.Warnings().AsErrors()
		require.Len(t, warnings, 1)
		require.Contains(t, warnings[0].Error(), "partial secondary failure")
		require.NoError(t, rs.Close())
	})

	t.Run("non-searcher querier is skipped", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "env", Score: 1.0}},
		}
		q2 := &mockQuerier{resp: []string{"should_be_ignored"}}
		// Verify precondition: mockQuerier does not implement Searcher.
		_, isSearcher := (Querier)(q2).(Searcher)
		require.False(t, isSearcher)
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Len(t, got, 1)
		require.Equal(t, "env", got[0].Value)
	})

	t.Run("no searcher queriers returns empty", func(t *testing.T) {
		// Two non-searcher queriers force the querierAdapter path.
		q1 := &mockQuerier{resp: []string{"a", "b"}}
		q2 := &mockQuerier{resp: []string{"c", "d"}}
		// Verify precondition.
		_, isSearcher := (Querier)(q1).(Searcher)
		require.False(t, isSearcher)
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Empty(t, got)
	})

	t.Run("OrderByScoreDesc overrides natural ordering", func(t *testing.T) {
		// Each searcher must emit in the requested order; score-desc
		// rearranges the overall result stream. The inputs below are
		// already score-desc within each searcher.
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "job", Score: 0.9}, {Value: "env", Score: 0.3}},
		}
		q2 := &searchQuerier{
			names: []SearchResult{{Value: "region", Score: 0.5}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		hints := &SearchHints{OrderBy: OrderByScoreDesc}
		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, hints))
		require.Equal(t, []SearchResult{
			{Value: "job", Score: 0.9},
			{Value: "region", Score: 0.5},
			{Value: "env", Score: 0.3},
		}, got)
	})

	t.Run("OrderByScoreDesc error preserves warnings", func(t *testing.T) {
		var ws annotations.Annotations
		ws.Add(errors.New("prior warning"))
		q1 := &searchQuerier{
			names:       []SearchResult{{Value: "env", Score: 1.0}},
			searchWarns: ws,
		}
		q2 := &searchQuerier{err: errors.New("score path failure")}
		merged := newMerged(q1, q2)
		defer merged.Close()

		hints := &SearchHints{OrderBy: OrderByScoreDesc}
		rs := merged.(Searcher).SearchLabelNames(ctx, hints)
		// Drain to completion so that the failing searcher's error
		// surfaces. Streaming may emit some results before the error.
		for rs.Next() {
		}
		require.Error(t, rs.Err())
		require.Contains(t, rs.Err().Error(), "score path failure")
		warnings := rs.Warnings().AsErrors()
		require.Len(t, warnings, 1)
		require.Contains(t, warnings[0].Error(), "prior warning")
		require.NoError(t, rs.Close())
	})

	t.Run("OrderByValueDesc reverses natural ordering", func(t *testing.T) {
		// Each searcher must emit in descending-Value order.
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "job", Score: 1.0}, {Value: "env", Score: 1.0}},
		}
		q2 := &searchQuerier{
			names: []SearchResult{{Value: "region", Score: 1.0}, {Value: "cluster", Score: 1.0}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		hints := &SearchHints{OrderBy: OrderByValueDesc}
		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, hints))
		require.Equal(t, []SearchResult{
			{Value: "region", Score: 1.0},
			{Value: "job", Score: 1.0},
			{Value: "env", Score: 1.0},
			{Value: "cluster", Score: 1.0},
		}, got)
	})

	t.Run("primary error preserves warnings from prior searchers", func(t *testing.T) {
		var ws annotations.Annotations
		ws.Add(errors.New("prior warning"))
		q1 := &searchQuerier{
			names:       []SearchResult{{Value: "env", Score: 1.0}},
			searchWarns: ws,
		}
		q2 := &searchQuerier{err: errors.New("primary failure")}
		merged := newMerged(q1, q2)
		defer merged.Close()

		rs := merged.(Searcher).SearchLabelNames(ctx, nil)
		// Iteration should see no results because the merge errored.
		require.False(t, rs.Next())
		require.Error(t, rs.Err())
		require.Contains(t, rs.Err().Error(), "primary failure")
		// Warnings from the successful first searcher must be preserved.
		warnings := rs.Warnings().AsErrors()
		require.Len(t, warnings, 1)
		require.Contains(t, warnings[0].Error(), "prior warning")
		require.NoError(t, rs.Close())
	})

	t.Run("limit is applied at merge level", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{{Value: "a", Score: 1.0}, {Value: "b", Score: 0.9}},
		}
		q2 := &searchQuerier{
			names: []SearchResult{{Value: "c", Score: 0.8}},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, &SearchHints{Limit: 2}))
		require.Len(t, got, 2)
		// Alphabetical order, limited to 2.
		require.Equal(t, "a", got[0].Value)
		require.Equal(t, "b", got[1].Value)
	})

	t.Run("empty searchers", func(t *testing.T) {
		q1 := &searchQuerier{}
		q2 := &searchQuerier{}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Empty(t, got)
	})

	t.Run("single searcher passthrough", func(t *testing.T) {
		// NewMergeQuerier with one querier returns it directly, so use two
		// where one is empty to force the merge path.
		q1 := &searchQuerier{
			names: []SearchResult{
				{Value: "a", Score: 1.0},
				{Value: "b", Score: 1.0},
				{Value: "c", Score: 1.0},
			},
		}
		q2 := &searchQuerier{}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Equal(t, []SearchResult{
			{Value: "a", Score: 1.0},
			{Value: "b", Score: 1.0},
			{Value: "c", Score: 1.0},
		}, got)
	})

	t.Run("overlapping values are deduplicated", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{
				{Value: "a", Score: 1.0},
				{Value: "b", Score: 1.0},
			},
		}
		q2 := &searchQuerier{
			names: []SearchResult{
				{Value: "b", Score: 1.0},
				{Value: "c", Score: 1.0},
			},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, nil))
		require.Equal(t, []SearchResult{
			{Value: "a", Score: 1.0},
			{Value: "b", Score: 1.0},
			{Value: "c", Score: 1.0},
		}, got)
	})

	t.Run("interleaved values with limit", func(t *testing.T) {
		q1 := &searchQuerier{
			names: []SearchResult{
				{Value: "a", Score: 1.0},
				{Value: "c", Score: 1.0},
				{Value: "e", Score: 1.0},
			},
		}
		q2 := &searchQuerier{
			names: []SearchResult{
				{Value: "b", Score: 1.0},
				{Value: "d", Score: 1.0},
			},
		}
		merged := newMerged(q1, q2)
		defer merged.Close()

		got := collectSearchResults(t, merged.(Searcher).SearchLabelNames(ctx, &SearchHints{Limit: 3}))
		require.Equal(t, []SearchResult{
			{Value: "a", Score: 1.0},
			{Value: "b", Score: 1.0},
			{Value: "c", Score: 1.0},
		}, got)
	})
}

func BenchmarkMergeSearchSets(b *testing.B) {
	// Build N searchers each returning M sorted results with no overlap.
	const nSearchers = 4
	const resultsPerSearcher = 10000
	queriers := make([]Querier, nSearchers)
	for i := range nSearchers {
		names := make([]SearchResult, resultsPerSearcher)
		for j := range resultsPerSearcher {
			names[j] = SearchResult{Value: fmt.Sprintf("label_%04d_%06d", i, j), Score: 1.0}
		}
		queriers[i] = &searchQuerier{names: names}
	}

	b.Run("no_limit", func(b *testing.B) {
		for b.Loop() {
			merged := NewMergeQuerier(queriers, nil, ChainedSeriesMerge)
			rs := merged.(Searcher).SearchLabelNames(context.Background(), nil)
			for rs.Next() {
			}
			require.NoError(b, rs.Err())
			rs.Close()
			merged.Close()
		}
	})

	b.Run("limit_100", func(b *testing.B) {
		hints := &SearchHints{Limit: 100}
		for b.Loop() {
			merged := NewMergeQuerier(queriers, nil, ChainedSeriesMerge)
			rs := merged.(Searcher).SearchLabelNames(context.Background(), hints)
			for rs.Next() {
			}
			require.NoError(b, rs.Err())
			rs.Close()
			merged.Close()
		}
	})
}
