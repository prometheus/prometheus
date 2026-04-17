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
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
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
				s.headChunks = nil
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
				s.headChunks = nil
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
				s.headChunks = nil
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

func TestHeadChunkReaderCache(t *testing.T) {
	t.Run("cache_hit", func(t *testing.T) {
		// Verify that a second chunk lookup for the same (unchanged) series
		// returns the cached head-chunks slice without re-collecting.
		opts := DefaultHeadOptions()
		opts.ChunkRange = 100
		opts.ChunkDirRoot = t.TempDir()
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })

		app := h.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "test")
		for i := int64(0); i < 500; i += 5 {
			_, err := app.Append(0, lbls, i, float64(i))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		s := h.series.getByID(1)
		require.NotNil(t, s)
		s.Lock()
		require.Greater(t, s.headChunks.len(), 1, "need multiple head chunks for the test")
		newestCID := s.firstChunkID + chunks.HeadChunkID(len(s.mmappedChunks)) + chunks.HeadChunkID(s.headChunks.len()) - 1
		newestRef := chunks.NewHeadChunkRef(s.ref, newestCID)
		s.Unlock()

		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.enableCache = true

		// First call: populates the cache.
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, cr.cachedHeadChunks)
		cachedSlice := cr.cachedHeadChunks

		// Second call (same series, no changes): must reuse the cached slice.
		_, _, err = cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.Same(t, &cachedSlice[0], &cr.cachedHeadChunks[0], "expected cache hit — slice backing array should be identical")
	})

	t.Run("invalidated_after_mmap", func(t *testing.T) {
		// Regression test: after mmapChunks(), the head-chunks cache must be
		// invalidated even though s.headChunks (the pointer) doesn't change.
		// mmapChunks severs the linked list (prev=nil, len=1) but
		// keeps the same head pointer, so a pointer-only check would return
		// a stale cache with chunks that have been mmapped.

		opts := DefaultHeadOptions()
		opts.ChunkRange = 100
		opts.ChunkDirRoot = t.TempDir()
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })

		// Append enough samples to create multiple head chunks.
		// With ChunkRange=100 and DefaultSamplesPerChunk=120, each chunk
		// holds ~20 samples (range/step = 100/5). We want >=3 head chunks.
		app := h.Appender(t.Context())
		lbls := labels.FromStrings("__name__", "test")
		for i := int64(0); i < 500; i += 5 {
			_, err := app.Append(0, lbls, i, float64(i))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		// Look up the series and verify we have multiple head chunks.
		s := h.series.getByID(1)
		require.NotNil(t, s)
		s.Lock()
		require.Greater(t, s.headChunks.len(), 1, "need multiple head chunks for the test")
		headChunksLenBefore := s.headChunks.len()
		newestChunkMinTime := s.headChunks.minTime
		// The chunk ID for the newest head chunk:
		// firstChunkID + len(mmapped) + headChunksLen - 1
		newestCID := s.firstChunkID + chunks.HeadChunkID(len(s.mmappedChunks)) + chunks.HeadChunkID(s.headChunks.len()) - 1
		newestRef := chunks.NewHeadChunkRef(s.ref, newestCID)
		s.Unlock()

		// Create a headChunkReader with cache enabled and query the newest head chunk to populate the cache.
		cr, err := h.chunksRange(0, 10000, nil)
		require.NoError(t, err)
		cr.enableCache = true

		chk1, _, err := cr.chunk(chunks.Meta{Ref: chunks.ChunkRef(newestRef)}, false)
		require.NoError(t, err)
		require.NotNil(t, chk1)

		// Verify cache is populated.
		require.NotNil(t, cr.cachedHeadChunks)
		require.Len(t, cr.cachedHeadChunks, headChunksLenBefore)

		// Now mmap all but the newest head chunk — this severs the linked list.
		s.Lock()
		headPtrBefore := s.headChunks
		s.mmapChunks(h.chunkDiskMapper)
		require.Equal(t, 1, s.headChunks.len(), "after mmap, should have exactly 1 head chunk")
		require.Same(t, headPtrBefore, s.headChunks, "head pointer should not change (the bug scenario)")
		require.Equal(t, newestChunkMinTime, s.headChunks.minTime, "newest chunk should be the remaining head chunk")

		// Recompute the newest chunk ID after mmap: more mmapped chunks now.
		newestCID = s.firstChunkID + chunks.HeadChunkID(len(s.mmappedChunks)) + chunks.HeadChunkID(s.headChunks.len()) - 1
		newestRef = chunks.NewHeadChunkRef(s.ref, newestCID)
		s.Unlock()

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

var benchSink *memChunk

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
			hc := collectHeadChunks(s.headChunks, nil)
			b.ReportAllocs()
			for b.Loop() {
				for i := range n {
					benchSink, _, _, _ = s.chunk(chunks.HeadChunkID(i), nil, nil, hc)
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
