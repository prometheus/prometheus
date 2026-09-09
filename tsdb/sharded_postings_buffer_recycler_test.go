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
	"math"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/compression"
)

func acquireShardPostingsReader(lifecycle *shardPostingsBufferLifecycle) *shardPostingsReaderLease {
	lease := &shardPostingsReaderLease{}
	lifecycle.acquireReader(lease)
	return lease
}

func TestNewShardedPostingsBufferRecycler_Disabled(t *testing.T) {
	require.Nil(t, NewShardedPostingsBufferRecycler(0, nil))
}

func TestShardedPostingsBufferRecycler_Get(t *testing.T) {
	recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
	recycler.put(make([]storage.SeriesRef, 0, 125))
	recycler.put(make([]storage.SeriesRef, 0, 110))
	recycler.put(make([]storage.SeriesRef, 0, 200))

	refs := recycler.get(100, 100)
	require.Len(t, refs, 100)
	require.Equal(t, 110, cap(refs), "use the smallest acceptable buffer")

	refs = recycler.get(100, 100)
	require.Len(t, refs, 100)
	require.Equal(t, 125, cap(refs))

	require.Nil(t, recycler.get(100, 100), "reject buffers more than 25%% oversized")
	require.Equal(t, 2.0, prom_testutil.ToFloat64(recycler.metrics.hits))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(recycler.metrics.misses))
}

func TestShardedPostingsBufferRecycler_LimitsAndLRU(t *testing.T) {
	t.Run("byte budget evicts least recently used", func(t *testing.T) {
		recycler := NewShardedPostingsBufferRecycler(3*seriesRefBytes, nil)
		first := []storage.SeriesRef{1}
		second := []storage.SeriesRef{2}
		recycler.put(first)
		recycler.put(second)

		used := recycler.get(1, 1)
		require.Equal(t, storage.SeriesRef(1), used[0])
		recycler.put(used)
		recycler.put(make([]storage.SeriesRef, 0, 2))

		require.Len(t, recycler.idle, 2)
		require.Equal(t, storage.SeriesRef(1), recycler.get(1, 1)[0])
		require.Equal(t, 1.0, prom_testutil.ToFloat64(recycler.metrics.evictions))
	})

	t.Run("buffer count is bounded", func(t *testing.T) {
		recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
		for range maxShardedPostingsIdleBuffers + 1 {
			recycler.put(make([]storage.SeriesRef, 0, 1))
		}
		require.Len(t, recycler.idle, maxShardedPostingsIdleBuffers)
		require.Equal(t, 1.0, prom_testutil.ToFloat64(recycler.metrics.evictions))
	})

	t.Run("oversized buffer is rejected", func(t *testing.T) {
		recycler := NewShardedPostingsBufferRecycler(seriesRefBytes, nil)
		recycler.put(make([]storage.SeriesRef, 0, 2))
		require.Empty(t, recycler.idle)
		require.Equal(t, 1.0, prom_testutil.ToFloat64(recycler.metrics.rejections))
	})
}

func TestShardedPostingsBufferRecycler_ConcurrentGetPut(t *testing.T) {
	recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
	const (
		workers    = 16
		iterations = 1000
		capacity   = 64
	)
	for range workers {
		recycler.put(make([]storage.SeriesRef, 0, capacity))
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range iterations {
				refs := recycler.get(capacity, capacity)
				if refs == nil {
					t.Error("unexpected recycler miss")
					return
				}
				recycler.put(refs)
			}
		})
	}
	wg.Wait()
	require.Len(t, recycler.idle, workers)
	require.LessOrEqual(t, recycler.retainedBytes, recycler.maxRetainedBytes)
}

func TestShardedPostingsBufferRecycler_Metrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	recycler := NewShardedPostingsBufferRecycler(1<<20, prometheus.WrapRegistererWithPrefix("test_", reg))
	recycler.put(make([]storage.SeriesRef, 0, 16))
	require.NotNil(t, recycler.get(8, 16))
	lifecycle := newShardPostingsBufferLifecycle(recycler)
	lease := acquireShardPostingsReader(lifecycle)
	lifecycle.retire(make([]storage.SeriesRef, 0, 8))
	t.Cleanup(lease.close)

	require.NoError(t, prom_testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_tsdb_sharded_postings_buffer_recycler_hits_total Total number of shard repair buffers reused by the recycler.
# TYPE test_tsdb_sharded_postings_buffer_recycler_hits_total counter
test_tsdb_sharded_postings_buffer_recycler_hits_total 1
# HELP test_tsdb_sharded_postings_buffer_recycler_pending_retirement_buffers Number of displaced shard repair buffers still visible to readers.
# TYPE test_tsdb_sharded_postings_buffer_recycler_pending_retirement_buffers gauge
test_tsdb_sharded_postings_buffer_recycler_pending_retirement_buffers 1
# HELP test_tsdb_sharded_postings_buffer_recycler_pending_retirement_capacity_bytes Capacity in bytes of displaced shard repair buffers still visible to readers.
# TYPE test_tsdb_sharded_postings_buffer_recycler_pending_retirement_capacity_bytes gauge
test_tsdb_sharded_postings_buffer_recycler_pending_retirement_capacity_bytes 64
# HELP test_tsdb_sharded_postings_buffer_recycler_retained_buffers Number of idle shard repair buffers retained by the recycler.
# TYPE test_tsdb_sharded_postings_buffer_recycler_retained_buffers gauge
test_tsdb_sharded_postings_buffer_recycler_retained_buffers 0
`),
		"test_tsdb_sharded_postings_buffer_recycler_hits_total",
		"test_tsdb_sharded_postings_buffer_recycler_pending_retirement_buffers",
		"test_tsdb_sharded_postings_buffer_recycler_pending_retirement_capacity_bytes",
		"test_tsdb_sharded_postings_buffer_recycler_retained_buffers",
	))
}

func TestShardPostingsBufferLifecycle(t *testing.T) {
	t.Run("waits for every reader that could hold the buffer", func(t *testing.T) {
		recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
		lifecycle := newShardPostingsBufferLifecycle(recycler)
		first := acquireShardPostingsReader(lifecycle)
		second := acquireShardPostingsReader(lifecycle)
		refs := make([]storage.SeriesRef, 8, 16)

		lifecycle.retire(refs)
		require.Empty(t, recycler.idle)
		require.Equal(t, uint64(1), recycler.pendingBuffers.Load())

		first.close()
		require.Empty(t, recycler.idle)
		second.close()
		require.Len(t, recycler.idle, 1)
		require.Zero(t, recycler.pendingBuffers.Load())
		second.close()
		require.Len(t, recycler.idle, 1, "Close must be idempotent")
	})

	t.Run("later reader does not delay retirement", func(t *testing.T) {
		recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
		lifecycle := newShardPostingsBufferLifecycle(recycler)
		first := acquireShardPostingsReader(lifecycle)
		refs := make([]storage.SeriesRef, 8, 16)
		data := unsafe.SliceData(refs)
		lifecycle.retire(refs)

		later := acquireShardPostingsReader(lifecycle)
		first.close()
		reused := recycler.get(8, 16)
		require.Equal(t, data, unsafe.SliceData(reused))
		later.close()
	})
}

func TestShardBucketPostings_RecyclerRetiresExposedBuffers(t *testing.T) {
	recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
	lifecycle := newShardPostingsBufferLifecycle(recycler)
	s := newShardBucketPostings(1)
	s.lifecycle = lifecycle
	s.repairStats = &shardBucketRepairStats{}
	input, dirtyStart := dirtySortInput(64, 0, true)
	bucket := &s.buckets[0]
	bucket.refs = slicesWithCapacity(input, len(input)+1)
	bucket.dirty = dirtyStart

	lease := acquireShardPostingsReader(lifecycle)
	s.sortUnlockedHook = func() {
		for i := range replacementTailHeadroom + 1 {
			s.add(1_000_000+chunks.HeadSeriesRef(i), 0)
		}
	}
	lists, _ := s.postingsFor(0, 1)
	require.True(t, lists[0].Next())
	require.Equal(t, uint64(2), recycler.pendingBuffers.Load(), "retire the old bucket and returned snapshot")
	require.Equal(t, uint64(2), s.repairStats.allocations.Load(), "allocate the snapshot and overflow replacement")
	require.Empty(t, recycler.idle)

	lease.close()
	require.Zero(t, recycler.pendingBuffers.Load())
	require.Len(t, recycler.idle, 2)
}

func TestShardBucketPostings_RecyclerHonorsOverlappingReaders(t *testing.T) {
	recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
	lifecycle := newShardPostingsBufferLifecycle(recycler)
	s := newShardBucketPostings(1)
	s.lifecycle = lifecycle
	input, dirtyStart := dirtySortInput(64, 0, true)
	bucket := &s.buckets[0]
	bucket.refs = slicesWithCapacity(input, len(input)+1)
	bucket.dirty = dirtyStart

	firstReader := acquireShardPostingsReader(lifecycle)
	firstLists, _ := s.postingsFor(0, 1)
	firstSnapshot := firstLists[0]
	require.Equal(t, uint64(1), recycler.pendingBuffers.Load())

	s.add(1, 0)
	secondReader := acquireShardPostingsReader(lifecycle)
	secondLists, _ := s.postingsFor(0, 1)
	require.NotNil(t, secondLists)
	require.Equal(t, uint64(2), recycler.pendingBuffers.Load())
	require.True(t, firstSnapshot.Next(), "the first reader's snapshot remains usable")

	firstReader.close()
	require.Equal(t, uint64(1), recycler.pendingBuffers.Load())
	require.Len(t, recycler.idle, 1, "only the pre-first-reader buffer is reusable")

	secondReader.close()
	require.Zero(t, recycler.pendingBuffers.Load())
	require.Len(t, recycler.idle, 2)
}

func TestHeadIndexReader_RecyclesShardRepairBuffersOnClose(t *testing.T) {
	recycler := NewShardedPostingsBufferRecycler(1<<20, nil)
	opts := newTestHeadDefaultOptions(1000, false)
	opts.EnableSharding = true
	opts.ShardedPostingsBuckets = 1
	opts.ShardedPostingsBufferRecycler = recycler
	head, _ := newTestHeadWithOptions(t, compression.None, opts)

	input, dirtyStart := dirtySortInput(64, 0, true)
	bucket := &head.shardBuckets.buckets[0]
	bucket.refs = slicesWithCapacity(input, len(input)+1)
	bucket.dirty = dirtyStart

	ir := head.indexRange(math.MinInt64, math.MaxInt64)
	p := ir.ShardedPostings(index.NewListPostings(input), 0, 2)
	require.NotNil(t, p)
	require.Equal(t, uint64(1), recycler.pendingBuffers.Load())
	require.Empty(t, recycler.idle)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.shardBucketRepairs))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.shardBucketAllocations))
	require.Positive(t, prom_testutil.ToFloat64(head.metrics.shardBucketAllocatedBytes))

	require.NoError(t, ir.Close())
	require.NoError(t, ir.Close())
	require.Zero(t, recycler.pendingBuffers.Load())
	require.Len(t, recycler.idle, 1)
}

func slicesWithCapacity(refs []storage.SeriesRef, capacity int) []storage.SeriesRef {
	out := make([]storage.SeriesRef, len(refs), capacity)
	copy(out, refs)
	return out
}
