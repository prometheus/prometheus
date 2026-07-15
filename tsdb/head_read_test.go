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

package tsdb

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/compression"
)

// TestMemSeries_chunk runs a series of tests on memSeries.chunk() calls.
// It will simulate various conditions to ensure all code paths in that function are covered.
func TestMemSeries_chunk(t *testing.T) {
	const chunkRange int64 = 100
	const chunkStep int64 = 5

	appendSamples := func(t *testing.T, s *memSeries, start, end int64, cdm *chunks.ChunkDiskMapper) {
		for i := start; i < end; i += chunkStep {
			ok, _ := s.append(0, i, float64(i), 0, chunkOpts{
				chunkDiskMapper: cdm,
				chunkRange:      chunkRange,
				samplesPerChunk: DefaultSamplesPerChunk,
			})
			require.True(t, ok, "sample append failed")
		}
	}

	type setupFn func(*testing.T, *memSeries, *chunks.ChunkDiskMapper)

	type callOutput uint8
	const (
		outOpenHeadChunk   callOutput = iota // memSeries.chunk() call returned memSeries.headChunks with headChunk=true & isOpen=true
		outClosedHeadChunk                   // memSeries.chunk() call returned memSeries.headChunks with headChunk=true & isOpen=false
		outMmappedChunk                      // memSeries.chunk() call returned a chunk from memSeries.mmappedChunks with headChunk=false
		outErr                               // memSeries.chunk() call returned an error
	)

	tests := []struct {
		name     string
		setup    setupFn            // optional function called just before the test memSeries.chunk() call
		inputID  chunks.HeadChunkID // requested chunk id for memSeries.chunk() call
		expected callOutput
	}{
		{
			name:     "call ix=0 on empty memSeries",
			inputID:  0,
			expected: outErr,
		},
		{
			name:     "call ix=1 on empty memSeries",
			inputID:  1,
			expected: outErr,
		},
		{
			name: "firstChunkID > ix",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, chunkRange-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				s.firstChunkID = 5
			},
			inputID:  1,
			expected: outErr,
		},
		{
			name: "call ix=0 on memSeries with no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, chunkRange-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  0,
			expected: outOpenHeadChunk,
		},
		{
			name: "call ix=1 on memSeries with no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, chunkRange-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  1,
			expected: outErr,
		},
		{
			name: "call ix=10 on memSeries with no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, chunkRange-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  10,
			expected: outErr,
		},
		{
			name: "call ix=0 on memSeries with 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  0,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=1 on memSeries with 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  1,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=3 on memSeries with 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  3,
			expected: outOpenHeadChunk,
		},
		{
			name: "call ix=0 on memSeries with 3 mmapped chunks and no headChunk",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				s.setHeadChunks(nil, 0)
			},
			inputID:  0,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=2 on memSeries with 3 mmapped chunks and no headChunk",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				s.setHeadChunks(nil, 0)
			},
			inputID:  2,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=3 on memSeries with 3 mmapped chunks and no headChunk",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				s.setHeadChunks(nil, 0)
			},
			inputID:  3,
			expected: outErr,
		},
		{
			name: "call ix=1 on memSeries with 3 mmapped chunks and closed ChunkDiskMapper",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				cdm.Close()
			},
			inputID:  1,
			expected: outErr,
		},
		{
			name: "call ix=3 on memSeries with 3 mmapped chunks and closed ChunkDiskMapper",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*4)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
				cdm.Close()
			},
			inputID:  3,
			expected: outOpenHeadChunk,
		},
		{
			name: "call ix=0 on memSeries with 3 head chunks and no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*3, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*3)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  0,
			expected: outClosedHeadChunk,
		},
		{
			name: "call ix=1 on memSeries with 3 head chunks and no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*3, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*3)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  1,
			expected: outClosedHeadChunk,
		},
		{
			name: "call ix=10 on memSeries with 3 head chunks and no mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*3, cdm)
				require.Empty(t, s.mmappedChunks, "wrong number of mmappedChunks")
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Equal(t, int64(0), s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*3)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  10,
			expected: outErr,
		},
		{
			name: "call ix=0 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  0,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=2 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  2,
			expected: outMmappedChunk,
		},
		{
			name: "call ix=3 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  3,
			expected: outClosedHeadChunk,
		},
		{
			name: "call ix=5 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  5,
			expected: outOpenHeadChunk,
		},
		{
			name: "call ix=6 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  6,
			expected: outErr,
		},

		{
			name: "call ix=10 on memSeries with 3 head chunks and 3 mmapped chunks",
			setup: func(t *testing.T, s *memSeries, cdm *chunks.ChunkDiskMapper) {
				appendSamples(t, s, 0, chunkRange*4, cdm)
				s.mmapChunks(cdm)
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, 1, s.headChunks.len(), "wrong number of headChunks")

				appendSamples(t, s, chunkRange*4, chunkRange*6, cdm)
				require.Equal(t, 3, s.headChunks.len(), "wrong number of headChunks")
				require.Len(t, s.mmappedChunks, 3, "wrong number of mmappedChunks")
				require.Equal(t, chunkRange*3, s.headChunks.oldest().minTime, "wrong minTime on last headChunks element")
				require.Equal(t, (chunkRange*6)-chunkStep, s.headChunks.maxTime, "wrong maxTime on first headChunks element")
			},
			inputID:  10,
			expected: outErr,
		},
	}

	memChunkPool := &sync.Pool{
		New: func() any {
			return &memChunk{}
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, chunkDiskMapper.Close())
			}()

			series := newMemSeries(labels.EmptyLabels(), 1, 0, true, false)

			if tc.setup != nil {
				tc.setup(t, series, chunkDiskMapper)
			}

			chk, headChunk, isOpen, err := series.chunk(tc.inputID, chunkDiskMapper, memChunkPool, nil)
			switch tc.expected {
			case outOpenHeadChunk:
				require.NoError(t, err, "unexpected error")
				require.True(t, headChunk, "expected a chunk with headChunk=true but got headChunk=%v", headChunk)
				require.True(t, isOpen, "expected a chunk with isOpen=true but got isOpen=%v", isOpen)
			case outClosedHeadChunk:
				require.NoError(t, err, "unexpected error")
				require.True(t, headChunk, "expected a chunk with headChunk=true but got headChunk=%v", headChunk)
				require.False(t, isOpen, "expected a chunk with isOpen=false but got isOpen=%v", isOpen)
			case outMmappedChunk:
				require.NoError(t, err, "unexpected error")
				require.False(t, headChunk, "expected a chunk with headChunk=false but got gc=%v", headChunk)
			case outErr:
				require.Nil(t, chk, "got a non-nil chunk reference returned with an error")
				require.Error(t, err)
			}
		})
	}

	t.Run("head chunk count mismatch", func(t *testing.T) {
		// A drifted headChunkCount (larger than the actual list length) must
		// yield ErrNotFound, not a panic or a nil chunk with a nil error.
		s := &memSeries{ref: 1}
		s.headChunkCount.Store(2) // Drifted: the list is empty.

		// ix == count-1: the walk is skipped entirely (offset 0).
		c, headChunk, isOpen, err := s.chunk(1, nil, nil, nil)
		require.ErrorIs(t, err, storage.ErrNotFound)
		require.Nil(t, c)
		require.False(t, headChunk)
		require.False(t, isOpen)

		// ix < count-1: the walk runs off the end of the shorter list.
		c, _, _, err = s.chunk(0, nil, nil, nil)
		require.ErrorIs(t, err, storage.ErrNotFound)
		require.Nil(t, c)
	})
}

// TestMemSeries_chunk_FastPath verifies that the O(1) indexed lookup via a
// pre-collected headChunks slice returns identical results to the linked-list
// fallback (headChunks=nil) for every chunk in a series with mixed mmapped
// and head chunks.
func TestMemSeries_chunk_FastPath(t *testing.T) {
	const chunkRange int64 = 100
	const chunkStep int64 = 5
	appendSamples := func(t *testing.T, s *memSeries, start, end int64, cdm *chunks.ChunkDiskMapper) {
		for i := start; i < end; i += chunkStep {
			ok, _ := s.append(0, i, float64(i), 0, chunkOpts{
				chunkDiskMapper: cdm,
				chunkRange:      chunkRange,
				samplesPerChunk: DefaultSamplesPerChunk,
			})
			require.True(t, ok, "sample append failed")
		}
	}

	dir := t.TempDir()
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()
	memChunkPool := &sync.Pool{New: func() any { return &memChunk{} }}

	series := newMemSeries(labels.EmptyLabels(), 1, 0, true, false)

	// Build 3 mmapped + 3 head chunks.
	appendSamples(t, series, 0, chunkRange*4, chunkDiskMapper)
	series.mmapChunks(chunkDiskMapper)
	require.Len(t, series.mmappedChunks, 3)
	require.Equal(t, 1, series.headChunks.len())
	appendSamples(t, series, chunkRange*4, chunkRange*6, chunkDiskMapper)
	require.Equal(t, 3, series.headChunks.len())
	require.Len(t, series.mmappedChunks, 3)

	hc := collectHeadChunks(series.headChunks, nil)

	headChunkCount := int(series.headChunkCount.Load())
	require.Equal(t, 3, headChunkCount, "expected 3 head chunks")
	totalChunks := len(series.mmappedChunks) + headChunkCount
	for ix := range totalChunks {
		id := chunks.HeadChunkID(ix)

		// Linked-list fallback.
		chkLL, headChunkLL, isOpenLL, errLL := series.chunk(id, chunkDiskMapper, memChunkPool, nil)
		// Fast path with pre-collected slice.
		chkFP, headChunkFP, isOpenFP, errFP := series.chunk(id, chunkDiskMapper, memChunkPool, hc)

		require.Equal(t, errLL, errFP, "ix=%d: error mismatch", ix)
		require.Equal(t, headChunkLL, headChunkFP, "ix=%d: headChunk mismatch", ix)
		require.Equal(t, isOpenLL, isOpenFP, "ix=%d: isOpen mismatch", ix)
		if ix < len(series.mmappedChunks) {
			// Mmapped chunks are loaded from disk into fresh memChunks, so
			// pointer equality is not expected — compare the time range instead.
			require.Equal(t, chkLL.minTime, chkFP.minTime, "ix=%d: minTime mismatch", ix)
			require.Equal(t, chkLL.maxTime, chkFP.maxTime, "ix=%d: maxTime mismatch", ix)
		} else {
			// Head chunks should be pointer-identical.
			require.Same(t, chkLL, chkFP, "ix=%d: head chunk pointer mismatch", ix)
		}
	}

	// Out-of-range ID via the fast path must return ErrNotFound.
	_, _, _, err = series.chunk(chunks.HeadChunkID(totalChunks), chunkDiskMapper, memChunkPool, hc)
	require.ErrorIs(t, err, storage.ErrNotFound)
}

func TestHeadIndexReader_PostingsForLabelMatching(t *testing.T) {
	testPostingsForLabelMatching(t, 0, func(t *testing.T, series []labels.Labels) IndexReader {
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = t.TempDir()
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, h.Close())
		})
		app := h.Appender(context.Background())
		for _, s := range series {
			app.Append(0, s, 0, 0)
		}
		require.NoError(t, app.Commit())

		ir, err := h.Index()
		require.NoError(t, err)
		return ir
	})
}

// newCacheTestHead creates a Head whose single series ("__name__"="test",
// ref 1) has at least three head chunks and no mmapped chunks, and returns the
// head, the series, and the ref of its newest head chunk (chunk IDs are stable
// across truncation).
func newCacheTestHead(t *testing.T) (*Head, *memSeries, chunks.HeadChunkRef) {
	opts := DefaultHeadOptions()
	opts.ChunkRange = 100
	h, _ := newTestHeadWithOptions(t, compression.None, opts)

	// Append enough samples to create multiple head chunks. With
	// ChunkRange=100 and DefaultSamplesPerChunk=120, each chunk holds ~20
	// samples (range/step = 100/5).
	app := h.Appender(t.Context())
	lbls := labels.FromStrings("__name__", "test")
	for i := int64(0); i < 500; i += 5 {
		_, err := app.Append(0, lbls, i, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	s := h.series.getByID(1)
	require.NotNil(t, s)
	// Snapshot values under the lock and assert after unlocking: a failing
	// require while the series lock is held would deadlock the head Close in
	// the test cleanup.
	s.Lock()
	headChunksLen := s.headChunks.len()
	mmappedLen := len(s.mmappedChunks)
	newestCID := s.firstChunkID + chunks.HeadChunkID(mmappedLen) + chunks.HeadChunkID(headChunksLen) - 1
	newestRef := chunks.NewHeadChunkRef(s.ref, newestCID)
	s.Unlock()
	require.Greater(t, headChunksLen, 2, "need at least 3 head chunks for the test")
	require.Zero(t, mmappedLen, "test requires a series with no mmapped chunks")

	return h, s, newestRef
}

func TestHeadChunkReaderCache(t *testing.T) {
	t.Run("cache_hit", func(t *testing.T) {
		// Verify that a second chunk lookup for the same (unchanged) series
		// returns the cached head-chunks slice without re-collecting.
		h, _, newestRef := newCacheTestHead(t)

		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.EnableChunkCache()

		// First call: populates the cache.
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, cr.cachedHeadChunks)

		// Plant a sentinel: a cache hit returns the slice untouched, while a
		// re-collection overwrites index 0 with the series' real oldest chunk.
		// The newest-chunk lookup below never dereferences index 0, so the
		// sentinel is safe.
		sentinel := &memChunk{}
		cr.cachedHeadChunks[0] = sentinel

		// Second call (same series, no changes): must take the cache-hit path.
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.Same(t, sentinel, cr.cachedHeadChunks[0], "expected cache hit — the cached slice should not have been re-collected")
	})

	t.Run("invalidated_after_mmap", func(t *testing.T) {
		// Regression test: after mmapChunks(), the head-chunks cache must be
		// invalidated even though s.headChunks (the pointer) doesn't change.
		// mmapChunks severs the linked list (prev=nil, len=1) but
		// keeps the same head pointer, so a pointer-only check would return
		// a stale cache with chunks that have been mmapped.

		h, s, newestRef := newCacheTestHead(t)
		s.Lock()
		headChunksLenBefore := s.headChunks.len()
		newestChunkMinTime := s.headChunks.minTime
		s.Unlock()

		// Create a headChunkReader with cache enabled and query the newest head chunk to populate the cache.
		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.EnableChunkCache()

		chk1, _, err := cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, chk1)

		// Verify cache is populated.
		require.NotNil(t, cr.cachedHeadChunks)
		require.Len(t, cr.cachedHeadChunks, headChunksLenBefore)

		// Now mmap all but the newest head chunk — this severs the linked
		// list. Snapshot under the lock, assert after unlocking (a failing
		// require under the series lock would deadlock the cleanup's Close).
		s.Lock()
		headPtrBefore := s.headChunks
		s.mmapChunks(h.chunkDiskMapper)
		headChunksLenAfter := s.headChunks.len()
		headPtrAfter := s.headChunks
		newestMinTimeAfter := s.headChunks.minTime
		// Recompute the newest chunk ID after mmap: more mmapped chunks now.
		newestCID := s.firstChunkID + chunks.HeadChunkID(len(s.mmappedChunks)) + chunks.HeadChunkID(s.headChunks.len()) - 1
		newestRef = chunks.NewHeadChunkRef(s.ref, newestCID)
		s.Unlock()

		require.Equal(t, 1, headChunksLenAfter, "after mmap, should have exactly 1 head chunk")
		require.Same(t, headPtrBefore, headPtrAfter, "mmapChunks must keep the head pointer — the precondition that would blind a pointer-only cache check")
		require.Equal(t, newestChunkMinTime, newestMinTimeAfter, "newest chunk should be the remaining head chunk")

		// Query the newest head chunk again. With the bug, the stale cache
		// would be used and return a wrong (mmapped) chunk.
		chk2, _, err := cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, chk2)

		// After mmap, only 1 head chunk remains. The prev==nil guard skips
		// the cache entirely, so the stale slice is preserved but not consulted.
		require.Len(t, cr.cachedHeadChunks, headChunksLenBefore, "stale cache not cleared, but also not used")

		// Verify the returned chunk is actually the newest head chunk, not a stale cached entry.
		it := chk2.Iterator(nil)
		require.Equal(t, chunkenc.ValFloat, it.Next())
		ts, _ := it.At()
		require.Equal(t, newestChunkMinTime, ts, "returned chunk should be the newest head chunk, not a stale cached entry")
	})

	t.Run("invalidated_after_truncation", func(t *testing.T) {
		// Regression test: truncateChunksBefore can remove older head chunks
		// while keeping the same head pointer and, for a series with no
		// mmapped chunks, the same mmapped-chunk count — only firstChunkID
		// advances. The cache must fingerprint firstChunkID as well,
		// otherwise chunk IDs are resolved against the stale slice and the
		// wrong chunk is returned.

		h, s, newestRef := newCacheTestHead(t)
		s.Lock()
		newestChunkMinTime := s.headChunks.minTime
		headChunksLenBefore := s.headChunks.len()
		firstChunkIDBefore := s.firstChunkID
		s.Unlock()

		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.EnableChunkCache()

		// First call: populates the cache.
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, cr.cachedHeadChunks)

		// Truncate the oldest head chunk: the head pointer and the mmapped
		// chunk count (0) are unchanged, but firstChunkID advances. Snapshot
		// under the lock, assert after unlocking (a failing require under the
		// series lock would deadlock the cleanup's Close).
		s.Lock()
		headPtrBefore := s.headChunks
		oldestMaxTime := s.headChunks.oldest().maxTime
		s.truncateChunksBefore(oldestMaxTime+1, 0)
		headPtrAfter := s.headChunks
		mmappedLenAfter := len(s.mmappedChunks)
		firstChunkIDAfter := s.firstChunkID
		s.Unlock()

		require.Same(t, headPtrBefore, headPtrAfter, "truncation must keep the head pointer")
		require.Zero(t, mmappedLenAfter, "mmapped-chunk count must stay 0 so only firstChunkID distinguishes the truncated state")
		require.Greater(t, firstChunkIDAfter, firstChunkIDBefore, "truncation should advance firstChunkID")

		// Query the newest head chunk again. With a stale cache, the
		// advanced firstChunkID indexes the old slice at the wrong position
		// and returns an older chunk.
		chk, _, err := cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, chk)
		require.Len(t, cr.cachedHeadChunks, headChunksLenBefore-1, "the stale cache must be re-collected after truncation")

		it := chk.Iterator(nil)
		require.Equal(t, chunkenc.ValFloat, it.Next())
		ts, _ := it.At()
		require.Equal(t, newestChunkMinTime, ts, "returned chunk should be the newest head chunk, not a stale cached entry")
	})

	t.Run("cache_cap_release", func(t *testing.T) {
		// The cache must not carry an oversized backing array from one series
		// to the next (the headChunksBufMaxCap policy).
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1
		h, _ := newTestHeadWithOptions(t, compression.None, opts)

		// The big series gets more than headChunksBufMaxCap head chunks
		// (ChunkRange=1 cuts a chunk per sample); the small series a few.
		app := h.Appender(t.Context())
		big := labels.FromStrings("__name__", "big")
		small := labels.FromStrings("__name__", "small")
		for i := range int64(headChunksBufMaxCap) + 10 {
			_, err := app.Append(0, big, i, float64(i))
			require.NoError(t, err)
		}
		for i := range int64(5) {
			_, err := app.Append(0, small, i, float64(i))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		bigSeries := h.series.getByHash(big.Hash(), big)
		require.NotNil(t, bigSeries)
		smallSeries := h.series.getByHash(small.Hash(), small)
		require.NotNil(t, smallSeries)

		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.EnableChunkCache()

		// Populate the cache with the big series: its cap exceeds the bound.
		bigRef := chunks.NewHeadChunkRef(bigSeries.ref, bigSeries.firstChunkID)
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(bigRef)}, false)
		require.NoError(t, err)
		require.Greater(t, cap(cr.cachedHeadChunks), headChunksBufMaxCap)

		// Switching to the small series must release the oversized array and
		// pre-size the replacement from the series' chunk count.
		smallRef := chunks.NewHeadChunkRef(smallSeries.ref, smallSeries.firstChunkID)
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(smallRef)}, false)
		require.NoError(t, err)
		require.LessOrEqual(t, cap(cr.cachedHeadChunks), headChunksBufMaxCap)
	})

	t.Run("released_on_close", func(t *testing.T) {
		// A closed reader must retain no cache key or chunk data.
		h, _, newestRef := newCacheTestHead(t)

		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.EnableChunkCache()

		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, cr.cachedHeadChunks)

		require.NoError(t, cr.Close())
		require.Equal(t, headChunkCacheKey{}, cr.cachedKey)
		require.Nil(t, cr.cachedHeadChunks)
	})

	t.Run("buffer_cap_release", func(t *testing.T) {
		// Test that headChunksBuf is released when its capacity exceeds
		// headChunksBufMaxCap, preventing unbounded memory retention.
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1
		opts.ChunkDirRoot = t.TempDir()
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })

		// Append enough samples to create >headChunksBufMaxCap head chunks.
		// With ChunkRange=1, each sample goes to a new chunk.
		app := h.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "cap_test")
		for i := range int64(headChunksBufMaxCap) + 10 {
			_, err := app.Append(0, lbls, i, float64(i))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		s := h.series.getByID(1)
		require.NotNil(t, s)
		s.Lock()
		require.Greater(t, s.headChunks.len(), headChunksBufMaxCap, "need >%d head chunks", headChunksBufMaxCap)
		s.Unlock()

		// Call Series() via headIndexReader — this populates headChunksBuf.
		ir := h.indexRange(0, int64(headChunksBufMaxCap)+10)
		var builder labels.ScratchBuilder
		var chks []chunks.Meta
		require.NoError(t, ir.Series(1, &builder, &chks))
		require.Greater(t, len(chks), headChunksBufMaxCap)

		// Buffer should have been released because cap exceeded threshold.
		require.Nil(t, ir.headChunksBuf, "headChunksBuf should be released when cap > headChunksBufMaxCap")
	})

	t.Run("buffer_cap_release_ooo_index_reader", func(t *testing.T) {
		// Same as buffer_cap_release but exercises HeadAndOOOIndexReader.Series,
		// which has its own headChunksBufMaxCap check.
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1
		opts.ChunkDirRoot = t.TempDir()
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })

		app := h.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "cap_test_ooo")
		for i := range int64(headChunksBufMaxCap) + 10 {
			_, err := app.Append(0, lbls, i, float64(i))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		s := h.series.getByID(1)
		require.NotNil(t, s)
		s.Lock()
		require.Greater(t, s.headChunks.len(), headChunksBufMaxCap, "need >%d head chunks", headChunksBufMaxCap)
		s.Unlock()

		// Call Series() via HeadAndOOOIndexReader (non-OOO series, so the else branch is taken).
		maxt := int64(headChunksBufMaxCap) + 10
		ir := NewHeadAndOOOIndexReader(h, 0, 0, maxt, 0)
		var builder labels.ScratchBuilder
		var chks []chunks.Meta
		require.NoError(t, ir.Series(1, &builder, &chks))
		require.Greater(t, len(chks), headChunksBufMaxCap)

		// Buffer should have been released because cap exceeded threshold.
		require.Nil(t, ir.headChunksBuf, "headChunksBuf should be released when cap > headChunksBufMaxCap")
	})
}

// Benchmark sinks prevent the compiler from eliding the measured calls.
var (
	benchSinkChunk  *memChunk
	benchSinkChunks []*memChunk
	benchSinkMeta   []chunks.Meta
	benchSinkInt    int
)

// BenchmarkSeriesChunkIteration measures iterating all N head chunks of a series
// oldest-to-newest (the real query pattern) using the cached head-chunks slice.
func BenchmarkSeriesChunkIteration(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64, 256} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			s := &memSeries{
				ref:          1,
				firstChunkID: 0,
				headChunks:   buildHeadChunksLight(n),
			}
			s.setHeadChunks(s.headChunks, uint32(n))
			hc := collectHeadChunks(s.headChunks, nil)
			b.ReportAllocs()
			for b.Loop() {
				for i := range n {
					benchSinkChunk, _, _, _ = s.chunk(chunks.HeadChunkID(i), nil, nil, hc)
				}
			}
		})
	}
}

// buildHeadChunksLight creates a memChunk linked list without allocating chunk
// encodings. Suitable for benchmarks that only need the linked-list structure
// and time ranges.
func buildHeadChunksLight(n int) *memChunk {
	var head *memChunk
	for i := range n {
		head = &memChunk{
			minTime: int64(i) * 1000,
			maxTime: int64(i)*1000 + 999,
			prev:    head,
		}
	}
	return head
}

func BenchmarkAppendSeriesChunks(b *testing.B) {
	for _, numHeadChunks := range []int{1, 4, 16, 64, 256} {
		b.Run(fmt.Sprintf("head only/%d", numHeadChunks), func(b *testing.B) {
			s := &memSeries{
				ref:        1,
				headChunks: buildHeadChunksLight(numHeadChunks),
			}
			mint := int64(0)
			maxt := int64(numHeadChunks) * 1000
			chks := make([]chunks.Meta, 0, numHeadChunks)

			b.ReportAllocs()
			for b.Loop() {
				chks, _ = appendSeriesChunks(s, mint, maxt, chks[:0], nil)
			}
			benchSinkMeta = chks
		})

		b.Run(fmt.Sprintf("with mmapped/%d", numHeadChunks), func(b *testing.B) {
			// Same number of mmapped chunks as head chunks. Mmapped chunks are
			// strictly older than all head chunks, as in a real series.
			mmapped := make([]*mmappedChunk, numHeadChunks)
			for i := range numHeadChunks {
				mmapped[i] = &mmappedChunk{
					minTime: int64(i-numHeadChunks) * 1000,
					maxTime: int64(i-numHeadChunks)*1000 + 999,
				}
			}
			s := &memSeries{
				ref:           1,
				headChunks:    buildHeadChunksLight(numHeadChunks),
				mmappedChunks: mmapped,
			}
			mint := int64(-numHeadChunks) * 1000
			maxt := int64(numHeadChunks) * 1000
			chks := make([]chunks.Meta, 0, numHeadChunks*2)

			b.ReportAllocs()
			for b.Loop() {
				chks, _ = appendSeriesChunks(s, mint, maxt, chks[:0], nil)
			}
			benchSinkMeta = chks
		})
	}
}

func BenchmarkCollectHeadChunks(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64, 256} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			head := buildHeadChunksLight(n)

			b.ReportAllocs()
			for b.Loop() {
				benchSinkChunks = collectHeadChunks(head, make([]*memChunk, 0, n))
			}
		})
	}
}

func BenchmarkSeriesChunk(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64, 256} {
		for _, pos := range []struct {
			name string
			id   chunks.HeadChunkID
		}{
			{name: "oldest", id: 0}, // Worst case: the full list walk.
			{name: "middle", id: chunks.HeadChunkID(n / 2)},
		} {
			if n < 2 && pos.name == "middle" {
				// With a single chunk, "middle" is the same lookup as "oldest".
				continue
			}
			b.Run(fmt.Sprintf("%d/%s", n, pos.name), func(b *testing.B) {
				s := &memSeries{
					ref:          1,
					firstChunkID: 0,
					headChunks:   buildHeadChunksLight(n),
				}
				s.setHeadChunks(s.headChunks, uint32(n))

				b.ReportAllocs()
				for b.Loop() {
					c, _, _, err := s.chunk(pos.id, nil, nil, nil)
					if err != nil {
						b.Fatal(err)
					}
					benchSinkChunk = c
				}
			})
		}
	}
}

func BenchmarkTruncateChunksBefore(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64, 256} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			// mint truncates the oldest half of head chunks.
			mint := int64(n/2) * 1000
			head := buildHeadChunksLight(n)
			headChunks := collectHeadChunks(head, nil)
			removedHeadChunks := n / 2
			var boundary, removedTail *memChunk
			if removedHeadChunks > 0 {
				boundary = headChunks[removedHeadChunks]
				removedTail = headChunks[removedHeadChunks-1]
			}
			s := &memSeries{firstChunkID: 0}

			b.ReportAllocs()
			for b.Loop() {
				if boundary != nil {
					boundary.prev = removedTail
				}
				s.firstChunkID = 0
				s.mmappedChunks = nil
				s.setHeadChunks(head, uint32(n))
				benchSinkInt = s.truncateChunksBefore(mint, 0)
			}
		})
	}
}
