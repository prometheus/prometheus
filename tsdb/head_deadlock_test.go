// Copyright 2024 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestHead_mmapHeadChunks_Panic(t *testing.T) {
	// Setup Head
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = t.TempDir()

	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, h.Close())
	}()

	require.NoError(t, h.Init(0))

	// Create a series and append enough data to create multiple head chunks
	// so that mmapHeadChunks will attempt to mmap them.
	app := h.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("a", "b"), 100, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	s := h.series.getByHash(labels.FromStrings("a", "b").Hash(), labels.FromStrings("a", "b"))
	require.NotNil(t, s)

	s.Lock()
	prevCount := s.headChunkCount.Load()
	s.cutNewHeadChunk(200, chunkenc.EncXOR, 1000)
	h.onChunkCreated(s, prevCount)
	s.Unlock()

	require.GreaterOrEqual(t, int(s.headChunkCount.Load()), 2)
	require.Equal(t, int32(1), h.series.mmapReady[h.series.refStripe(s.ref)].Load())

	// Force a panic during mmap by setting the disk mapper to nil.
	// This simulates a catastrophic I/O failure or an internal panic (e.g. handleChunkWriteError panic).
	// We must save the old mapper to restore it for h.Close() later.
	oldMapper := h.chunkDiskMapper
	h.chunkDiskMapper = nil

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.mmapHeadChunks()
	}()
	require.True(t, panicked, "expected mmapHeadChunks to panic")

	// Restore mapper so h.Close() in defer doesn't panic
	h.chunkDiskMapper = oldMapper

	// Ensure that locks are released properly despite the panic.
	// If the locks were leaked, this will block and cause a test timeout.
	stripe := h.series.refStripe(s.ref)

	lockAcquired := make(chan struct{})
	go func() {
		h.series.locks[stripe].Lock()
		//nolint:staticcheck // Intentionally empty critical section to verify lock acquisition.
		h.series.locks[stripe].Unlock()

		s.Lock()
		//nolint:staticcheck // Intentionally empty critical section to verify lock acquisition.
		s.Unlock()

		close(lockAcquired)
	}()

	select {
	case <-lockAcquired:
		// Success: locks were not leaked.
	case <-time.After(2 * time.Second):
		t.Fatal("Locks were leaked after panic in mmapHeadChunks")
	}
}
