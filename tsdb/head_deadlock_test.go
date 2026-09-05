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

// Regression tests for https://github.com/prometheus/prometheus/issues/19445
//
// ABBA lock-ordering deadlock between:
//   - mmapHeadChunksInStripe: holds stripe[i].RLock  → waits for series.Lock()
//   - gcSeries check callback: holds series.Lock()   → waits for stripe[j].Lock() (write)
//
// With the un-fixed code the tests hang permanently; with the fix they
// complete within the deadline.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/compression"
)

// TestDeadlockGCSeriesAndMmapChunks exercises the concurrent path that Mimir
// hits when it calls CompactSelectedSeries from a goroutine while the background
// DB.run() loop is running mmapHeadChunks.
//
// Before the fix the test hangs within the first few iterations.
// After the fix it completes well within the 30-second deadline.
func TestDeadlockGCSeriesAndMmapChunks(t *testing.T) {
	// Use a tiny stripe size so hash-shard ≠ ref-stripe is very common,
	// maximising the chance that gcSeries must acquire a second stripe lock.
	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.StripeSize = 2

	head, wal := newTestHeadWithOptions(t, compression.None, opts)
	t.Cleanup(func() { _ = wal.Close() })
	require.NoError(t, head.Init(0))

	// Append enough samples so every series has headChunkCount >= 2 (mmap-eligible).
	const numSeries = 50
	const samplesPerSeries = DefaultSamplesPerChunk + 5

	refs := make([]storage.SeriesRef, 0, numSeries)
	for i := range numSeries {
		lset := labels.FromStrings("__name__", "deadlock_test", "id", string(rune('a'+i)))
		app := head.Appender(context.Background())
		var ref storage.SeriesRef
		for j := range samplesPerSeries {
			r, err := app.Append(ref, lset, int64(j)*1000, float64(j))
			require.NoError(t, err)
			ref = r
		}
		require.NoError(t, app.Commit())
		refs = append(refs, ref)
	}

	const rounds = 300
	deadline := time.After(30 * time.Second)
	done := make(chan struct{})

	// Goroutine A: mmapHeadChunks — stripe RLock → series.Lock
	// Goroutine B: gcSeries      — (before fix) series.Lock → stripe write Lock
	//
	// Running them concurrently without synchronisation maximises interleaving
	// and makes the deadlock deterministic on any scheduler.
	go func() {
		defer close(done)

		var wg sync.WaitGroup
		for range rounds {
			wg.Add(2)

			go func() {
				defer wg.Done()
				head.mmapHeadChunks()
			}()

			go func() {
				defer wg.Done()
				// maxt=-1 means nothing is evicted, but all lock paths are taken.
				_ = head.gcSeries(refs, -1, func(*memSeries) bool { return false })
			}()

			wg.Wait()
		}
	}()

	select {
	case <-done:
		// No deadlock.
	case <-deadline:
		t.Fatal("deadlock detected: mmapHeadChunks and gcSeries are permanently blocked; " +
			"regression for https://github.com/prometheus/prometheus/issues/19445")
	}
}

// TestDeadlockGCAndMmapChunks_CrossStripe is the same scenario with StripeSize=4
// to cover series whose ref-stripe ≠ hash-stripe (the cross-stripe case that
// requires two stripe locks in the GC callback).
func TestDeadlockGCAndMmapChunks_CrossStripe(t *testing.T) {
	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.StripeSize = 4

	head, wal := newTestHeadWithOptions(t, compression.None, opts)
	t.Cleanup(func() { _ = wal.Close() })
	require.NoError(t, head.Init(0))

	const numSeries = 100
	const samplesPerSeries = DefaultSamplesPerChunk + 1

	refs := make([]storage.SeriesRef, 0, numSeries)
	for i := range numSeries {
		lset := labels.FromStrings("pod", string(rune('a'+i%26)), "idx", string(rune('0'+i/26)))
		app := head.Appender(context.Background())
		var ref storage.SeriesRef
		for j := range samplesPerSeries {
			r, err := app.Append(ref, lset, int64(j)*1000, float64(j))
			require.NoError(t, err)
			ref = r
		}
		require.NoError(t, app.Commit())
		refs = append(refs, ref)
	}

	deadline := time.After(30 * time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for range 200 {
			wg.Add(2)
			go func() { defer wg.Done(); head.mmapHeadChunks() }()
			go func() {
				defer wg.Done()
				_ = head.gcSeries(refs, -1, func(*memSeries) bool { return false })
			}()
			wg.Wait()
		}
	}()

	select {
	case <-done:
	case <-deadline:
		t.Fatal("deadlock detected (cross-stripe): " +
			"regression for https://github.com/prometheus/prometheus/issues/19445")
	}
}

// TestDeadlockStripeSeries_GC covers the gc() → stripeSeries.gc() path whose
// check callback had the identical ABBA pattern.
func TestDeadlockStripeSeries_GC(t *testing.T) {
	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.StripeSize = 2

	head, wal := newTestHeadWithOptions(t, compression.None, opts)
	t.Cleanup(func() { _ = wal.Close() })
	require.NoError(t, head.Init(0))

	const numSeries = 40
	const samplesPerSeries = DefaultSamplesPerChunk + 2

	app := head.Appender(context.Background())
	for i := range numSeries {
		lset := labels.FromStrings("gc_test", string(rune('a'+i)))
		for j := range samplesPerSeries {
			_, err := app.Append(0, lset, int64(j)*1000, float64(j))
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	deadline := time.After(30 * time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for range 200 {
			wg.Add(2)
			go func() { defer wg.Done(); head.mmapHeadChunks() }()
			go func() { defer wg.Done(); head.gc() }()
			wg.Wait()
		}
	}()

	select {
	case <-done:
	case <-deadline:
		t.Fatal("deadlock detected (stripeSeries.gc path): " +
			"regression for https://github.com/prometheus/prometheus/issues/19445")
	}
}
