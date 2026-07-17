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
	"math/rand"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

var shardBucketDirtySortSink storage.SeriesRef

// expandShardCandidates returns the sorted bucket candidates for a shard request.
func expandShardCandidates(t *testing.T, s *shardBucketPostings, shardIndex, shardCount uint64) []storage.SeriesRef {
	t.Helper()
	lists, _ := s.postingsFor(shardIndex, shardCount)
	refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
	require.NoError(t, err)
	return refs
}

func deletedShardHashMap(tb testing.TB, deleted map[storage.SeriesRef]struct{}, refHashes map[storage.SeriesRef]uint64) map[storage.SeriesRef]uint64 {
	tb.Helper()
	out := make(map[storage.SeriesRef]uint64, len(deleted))
	for ref := range deleted {
		hash, ok := refHashes[ref]
		require.True(tb, ok, "missing shard hash for deleted ref %d", ref)
		out[ref] = hash
	}
	require.Len(tb, out, len(deleted))
	return out
}

func identityDeletedShardHashes(refs ...storage.SeriesRef) map[storage.SeriesRef]uint64 {
	out := make(map[storage.SeriesRef]uint64, len(refs))
	for _, ref := range refs {
		out[ref] = uint64(ref)
	}
	return out
}

type shardBucketSnapshot struct {
	refsPtr *storage.SeriesRef
	refsLen int
	dirty   int
}

// snapshotShardBuckets captures slice identity, length, and dirty state; callers
// must prevent concurrent bucket mutations.
func snapshotShardBuckets(s *shardBucketPostings) []shardBucketSnapshot {
	out := make([]shardBucketSnapshot, len(s.buckets))
	for i := range s.buckets {
		bucket := &s.buckets[i]
		out[i] = shardBucketSnapshot{
			refsPtr: unsafe.SliceData(bucket.refs),
			refsLen: len(bucket.refs),
			dirty:   bucket.dirty,
		}
	}
	return out
}

func TestShardBucketPostings_PostingsFor(t *testing.T) {
	t.Run("exact shard counts partition refs across bucket counts", func(t *testing.T) {
		t.Parallel()
		// Power-of-two shard counts up to the bucket count are served exactly by
		// whole buckets and must equal brute-force shard membership.
		const numRefs = 5000
		rng := rand.New(rand.NewSource(7))
		for _, bucketCount := range []int{64, 128} {
			s := newShardBucketPostings(bucketCount)
			refHashes := make(map[storage.SeriesRef]uint64, numRefs)
			for ref := chunks.HeadSeriesRef(1); ref <= numRefs; ref++ {
				h := rng.Uint64()
				s.add(ref, h)
				refHashes[storage.SeriesRef(ref)] = h
			}
			require.Equal(t, numRefs, s.numSeries())

			for _, shardCount := range []uint64{2, 4, 64, 128} {
				if shardCount > uint64(bucketCount) {
					continue
				}
				want := map[uint64][]storage.SeriesRef{}
				for ref := storage.SeriesRef(1); ref <= numRefs; ref++ {
					sh := refHashes[ref] % shardCount
					want[sh] = append(want[sh], ref) // appended in ref order => sorted.
				}

				for shardIndex := range shardCount {
					got := expandShardCandidates(t, s, shardIndex, shardCount)
					require.Equal(t, want[shardIndex], got, "bucketCount=%d shardCount=%d shard=%d", bucketCount, shardCount, shardIndex)
				}
			}
		}
	})

	t.Run("reports when shard hash filtering is needed", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(64)

		// A zero shard count selects nothing.
		lists, subFiltered := s.postingsFor(0, 0)
		require.Nil(t, lists)
		require.False(t, subFiltered)

		for _, tc := range []struct {
			shardCount uint64
			wantFilter bool
		}{
			{shardCount: 1},
			{shardCount: 2},
			{shardCount: 4},
			{shardCount: 8},
			{shardCount: 16},
			{shardCount: 32},
			{shardCount: 64},
			{shardCount: 128, wantFilter: true},
			{shardCount: 256, wantFilter: true},
		} {
			_, needsShardHashFilter := s.postingsFor(0, tc.shardCount)
			require.Equal(t, tc.wantFilter, needsShardHashFilter, "shardCount %d", tc.shardCount)
		}
	})

	t.Run("over-bucket candidates stay sorted through resort and remove", func(t *testing.T) {
		t.Parallel()
		// Regression guard: out-of-order adds mark buckets dirty (re-sorted on
		// read) and remove rebuilds buckets. The bucket layer returns sorted
		// candidate refs; the head layer performs final over-bucket filtering.
		s := newShardBucketPostings(64)
		refHashes := map[storage.SeriesRef]uint64{}
		rng := rand.New(rand.NewSource(99))
		for _, ref := range []chunks.HeadSeriesRef{50, 10, 90, 30, 70, 20, 100, 40, 80, 60, 5, 95, 15, 85, 25} {
			h := rng.Uint64()
			s.add(ref, h)
			refHashes[storage.SeriesRef(ref)] = h
		}
		deleted := map[storage.SeriesRef]struct{}{10: {}, 70: {}, 95: {}, 25: {}}
		s.remove(deletedShardHashMap(t, deleted, refHashes))
		for ref := range deleted {
			delete(refHashes, ref)
		}

		const shardCount = uint64(128) // Exceeds 64 buckets => candidate refs need final filtering.
		for shardIndex := range shardCount {
			got := expandShardCandidates(t, s, shardIndex, shardCount)
			var want []storage.SeriesRef
			for ref, h := range refHashes {
				if h%64 == shardIndex%64 {
					want = append(want, ref)
				}
			}
			slices.Sort(want)
			require.Equal(t, want, got)
		}
	})

	t.Run("nil means disabled", func(t *testing.T) {
		t.Parallel()
		var s *shardBucketPostings

		lists, subFiltered := s.postingsFor(0, 4)
		require.Nil(t, lists)
		require.False(t, subFiltered)
		require.Zero(t, s.numSeries())
		s.add(1, 1)                             // Must not panic.
		s.remove(identityDeletedShardHashes(1)) // Must not panic.
	})

	t.Run("out-of-order adds are served sorted", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		for _, ref := range []chunks.HeadSeriesRef{5, 3, 9, 1, 7} {
			s.add(ref, 0)
		}

		got := expandShardCandidates(t, s, 0, 1)
		require.Equal(t, []storage.SeriesRef{1, 3, 5, 7, 9}, got)
	})

	t.Run("snapshots stay valid after a later resort", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		s.add(1, 0)
		s.add(3, 0)

		snapshotLists, _ := s.postingsFor(0, 1)
		s.add(2, 0)

		require.Equal(t, []storage.SeriesRef{1, 2, 3}, expandShardCandidates(t, s, 0, 1))
		snapshot, err := index.ExpandPostings(index.Merge(t.Context(), snapshotLists...))
		require.NoError(t, err)
		require.Equal(t, []storage.SeriesRef{1, 3}, snapshot)
	})

	t.Run("append proceeds while dirty sorting is unlocked", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		s.add(4, 0)
		s.add(2, 0)

		sortStarted := make(chan struct{})
		allowSort := make(chan struct{})
		s.sortUnlockedHook = func() {
			close(sortStarted)
			<-allowSort
		}
		type readResult struct {
			refs []storage.SeriesRef
			err  error
		}
		readDone := make(chan readResult, 1)
		go func() {
			lists, _ := s.postingsFor(0, 1)
			refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
			readDone <- readResult{refs: refs, err: err}
		}()
		<-sortStarted

		addDone := make(chan struct{})
		go func() {
			s.add(1, 0)
			close(addDone)
		}()
		select {
		case <-addDone:
		case <-time.After(time.Second):
			close(allowSort)
			<-readDone
			<-addDone
			t.Fatal("append blocked while dirty bucket was sorted")
		}
		close(allowSort)
		result := <-readDone

		require.NoError(t, result.err)
		require.Equal(t, []storage.SeriesRef{2, 4}, result.refs)
		s.sortUnlockedHook = nil
		require.Equal(t, []storage.SeriesRef{1, 2, 4}, expandShardCandidates(t, s, 0, 1))
	})

	t.Run("append proceeds while sort tail grows unlocked", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		s.add(4, 0)
		s.add(2, 0)

		const burst = replacementTailHeadroom + 1
		s.sortUnlockedHook = func() {
			for i := range burst {
				s.add(chunks.HeadSeriesRef(100+i), 0)
			}
		}
		tailCarryStarted := make(chan struct{})
		allowTailCarry := make(chan struct{})
		s.sortTailCarryHook = func() {
			close(tailCarryStarted)
			<-allowTailCarry
		}
		type readResult struct {
			refs []storage.SeriesRef
			err  error
		}
		readDone := make(chan readResult, 1)
		go func() {
			lists, _ := s.postingsFor(0, 1)
			refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
			readDone <- readResult{refs: refs, err: err}
		}()
		<-tailCarryStarted

		addDone := make(chan struct{})
		go func() {
			s.add(1, 0)
			close(addDone)
		}()
		select {
		case <-addDone:
		case <-time.After(time.Second):
			close(allowTailCarry)
			<-addDone
			<-readDone
			t.Fatal("append blocked while the sort tail was carried")
		}
		close(allowTailCarry)
		result := <-readDone

		require.NoError(t, result.err)
		require.Equal(t, []storage.SeriesRef{2, 4}, result.refs)
		s.sortUnlockedHook = nil
		s.sortTailCarryHook = nil
		want := []storage.SeriesRef{1, 2, 4}
		for i := range burst {
			want = append(want, storage.SeriesRef(100+i))
		}
		require.Equal(t, want, expandShardCandidates(t, s, 0, 1))
	})

	t.Run("read sorts only candidate dirty buckets", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(4)
		for _, tc := range []struct {
			ref  chunks.HeadSeriesRef
			hash uint64
		}{
			{ref: 10, hash: 0},
			{ref: 2, hash: 0},
			{ref: 11, hash: 1},
			{ref: 3, hash: 1},
		} {
			s.add(tc.ref, tc.hash)
		}
		require.NotEqual(t, cleanShardBucket, s.buckets[0].dirty)
		require.NotEqual(t, cleanShardBucket, s.buckets[1].dirty)

		require.Equal(t, []storage.SeriesRef{2, 10}, expandShardCandidates(t, s, 0, 4))
		require.Equal(t, cleanShardBucket, s.buckets[0].dirty)
		require.NotEqual(t, cleanShardBucket, s.buckets[1].dirty)

		require.Equal(t, []storage.SeriesRef{3, 11}, expandShardCandidates(t, s, 1, 4))
		require.Equal(t, cleanShardBucket, s.buckets[1].dirty)
	})
}

func TestShardBucketPostings_Remove(t *testing.T) {
	t.Run("drops deleted refs and keeps reader snapshots intact", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(4)
		for ref := chunks.HeadSeriesRef(1); ref <= 10; ref++ {
			s.add(ref, uint64(ref))
		}

		snapshotLists, _ := s.postingsFor(0, 2)

		s.remove(identityDeletedShardHashes(2, 4, 7))
		require.Equal(t, 7, s.numSeries())

		snapshot, err := index.ExpandPostings(index.Merge(t.Context(), snapshotLists...))
		require.NoError(t, err)
		after := expandShardCandidates(t, s, 0, 2)
		require.Equal(t, []storage.SeriesRef{2, 4, 6, 8, 10}, snapshot)
		require.Equal(t, []storage.SeriesRef{6, 8, 10}, after)
	})

	t.Run("rebuilds only touched buckets", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(8)
		for ref := storage.SeriesRef(1); ref <= 64; ref++ {
			s.add(chunks.HeadSeriesRef(ref), uint64(ref))
		}

		before := snapshotShardBuckets(s)
		s.remove(identityDeletedShardHashes(1, 9, 5, 13))

		after := snapshotShardBuckets(s)
		expectedTouched := map[int]struct{}{1: {}, 5: {}}
		for i := range s.buckets {
			if _, ok := expectedTouched[i]; ok {
				require.Less(t, after[i].refsLen, before[i].refsLen, "bucket %d should be rebuilt", i)
				continue
			}
			require.Equal(t, before[i], after[i], "bucket %d should not be rebuilt", i)
		}
	})

	t.Run("skips buckets without deleted refs", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(2)
		for ref := storage.SeriesRef(1); ref <= 8; ref++ {
			s.add(chunks.HeadSeriesRef(ref), uint64(ref))
		}

		before := snapshotShardBuckets(s)[0]
		// The deleted refs map to bucket 0 but are not present in it.
		s.remove(map[storage.SeriesRef]uint64{1000: 0, 1002: 2})
		require.Equal(t, before, snapshotShardBuckets(s)[0])
		require.Equal(t, []storage.SeriesRef{2, 4, 6, 8}, expandShardCandidates(t, s, 0, 2))
	})

	t.Run("carries over refs appended during the unlocked rebuild", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		for ref := storage.SeriesRef(1); ref <= 6; ref++ {
			s.add(chunks.HeadSeriesRef(ref), 0)
		}

		hookRuns := 0
		s.removeUnlockedHook = func() {
			// Appends landing between the survivor rebuild and the bucket
			// re-lock must be carried over, and can never be deleted refs.
			s.add(7, 0)
			s.add(8, 0)
			hookRuns++
		}
		s.remove(map[storage.SeriesRef]uint64{2: 0, 4: 0})

		require.Equal(t, 1, hookRuns)
		require.Equal(t, []storage.SeriesRef{1, 3, 5, 6, 7, 8}, expandShardCandidates(t, s, 0, 1))
	})

	t.Run("clean readers proceed during the unlocked rebuild", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		for ref := chunks.HeadSeriesRef(1); ref <= 6; ref++ {
			s.add(ref, 0)
		}

		rebuildStarted := make(chan struct{})
		allowCommit := make(chan struct{})
		s.removeUnlockedHook = func() {
			close(rebuildStarted)
			<-allowCommit
		}
		removeDone := make(chan struct{})
		go func() {
			s.remove(map[storage.SeriesRef]uint64{2: 0, 4: 0})
			close(removeDone)
		}()
		<-rebuildStarted

		var snapshotLists []index.Postings
		readDone := make(chan struct{})
		go func() {
			snapshotLists, _ = s.postingsFor(0, 1)
			close(readDone)
		}()
		select {
		case <-readDone:
		case <-time.After(time.Second):
			close(allowCommit)
			<-removeDone
			<-readDone
			t.Fatal("clean read blocked during unlocked rebuild")
		}
		close(allowCommit)
		<-removeDone

		snapshot, err := index.ExpandPostings(index.Merge(t.Context(), snapshotLists...))
		require.NoError(t, err)
		require.Equal(t, []storage.SeriesRef{1, 2, 3, 4, 5, 6}, snapshot)
		require.Equal(t, []storage.SeriesRef{1, 3, 5, 6}, expandShardCandidates(t, s, 0, 1))
	})
}

func TestShardBucketPostings_Concurrency(t *testing.T) {
	t.Run("adds, removes and reads", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(8)

		const (
			writers       = 4
			refsPerWriter = 10_000
		)

		var wg sync.WaitGroup
		// Writers add interleaved refs (out of order across goroutines) and
		// remove half of their own again, racing bucket re-sorts and prunes.
		for w := range writers {
			wg.Go(func() {
				for i := range refsPerWriter {
					ref := chunks.HeadSeriesRef(i*writers + w + 1)
					s.add(ref, uint64(ref))
					if i%2 == 0 {
						s.remove(identityDeletedShardHashes(storage.SeriesRef(ref)))
					}
				}
			})
		}
		// Readers verify every captured shard list is sorted and every returned
		// ref belongs to the requested shard. Since the test adds ref == hash,
		// shard membership remains checkable even while remove and add race.
		for range 2 {
			wg.Go(func() {
				for range 200 {
					for _, shardCount := range []uint64{4, 8} {
						for shardIndex := range shardCount {
							lists, _ := s.postingsFor(shardIndex, shardCount)
							refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
							if err != nil {
								panic(err)
							}
							if !slices.IsSorted(refs) {
								panic("shard list not sorted")
							}
							for _, ref := range refs {
								if uint64(ref)%shardCount != shardIndex {
									panic(fmt.Sprintf("ref %d returned for shard %d of %d", ref, shardIndex, shardCount))
								}
							}
						}
					}
				}
			})
		}
		wg.Wait()

		// After the dust settles: odd writer-iterations stay, even ones are removed.
		require.Equal(t, writers*refsPerWriter/2, s.numSeries())
		var got []storage.SeriesRef
		for shardIndex := range uint64(4) {
			got = append(got, expandShardCandidates(t, s, shardIndex, 4)...)
		}
		require.Len(t, got, writers*refsPerWriter/2)
	})

	t.Run("multi-bucket reads with remove and dirty sort", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(16)

		const (
			numSeed    = 4000
			writers    = 2
			iterations = 3000
			shardCount = uint64(4)
		)
		for ref := chunks.HeadSeriesRef(1); ref <= numSeed; ref++ {
			s.add(ref, uint64(ref))
		}

		recordErr, firstErr := shardBucketFirstError()
		done := make(chan struct{})
		var readersWG, writersWG sync.WaitGroup

		for r := range 4 {
			readersWG.Go(func() {
				shard := uint64(r) % shardCount
				for {
					select {
					case <-done:
						return
					default:
					}

					lists, _ := s.postingsFor(shard, shardCount)
					refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
					if err != nil {
						recordErr(err)
						return
					}
					if !slices.IsSorted(refs) {
						recordErr(fmt.Errorf("shard %d refs are not sorted", shard))
						return
					}
					for _, ref := range refs {
						if uint64(ref)%shardCount != shard {
							recordErr(fmt.Errorf("ref %d returned for shard %d of %d", ref, shard, shardCount))
							return
						}
					}
					shard = (shard + 1) % shardCount
				}
			})
		}

		for w := range writers {
			writersWG.Go(func() {
				for i := range iterations {
					bucket := chunks.HeadSeriesRef((i + w) % 16)
					high := chunks.HeadSeriesRef(1_000_000 + (w*iterations+i)*32 + int(bucket))
					low := high - 16
					s.add(high, uint64(high))
					s.add(low, uint64(low))
					if i%3 == 0 {
						s.remove(identityDeletedShardHashes(storage.SeriesRef(low)))
					}
				}
			})
		}

		writersWG.Wait()
		close(done)
		readersWG.Wait()
		require.NoError(t, firstErr())
	})
}

func dirtySortInput(n, dirtyTail int, interleaved bool) ([]storage.SeriesRef, int) {
	refs := make([]storage.SeriesRef, 0, n)
	if interleaved {
		for i := 0; len(refs) < n; i++ {
			refs = append(refs, storage.SeriesRef(i+1))
			if len(refs) == n {
				break
			}
			refs = append(refs, storage.SeriesRef(n+i+1))
		}
		return refs, 1
	}

	sorted := n - dirtyTail
	for i := 1; i <= sorted; i++ {
		refs = append(refs, storage.SeriesRef(i))
	}
	for i := range dirtyTail {
		refs = append(refs, storage.SeriesRef(sorted/2+i*2+1))
	}
	return refs, sorted
}

func BenchmarkShardBucketPostings_DirtySort(b *testing.B) {
	for _, tc := range []struct {
		name        string
		n           int
		dirtyTail   int
		interleaved bool
	}{
		{name: "mostly-sorted-tail", n: 65_536, dirtyTail: 64},
		{name: "interleaved", n: 65_536, interleaved: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			baseRefs, dirtyStart := dirtySortInput(tc.n, tc.dirtyTail, tc.interleaved)

			b.ReportAllocs()
			for b.Loop() {
				sortedRefs := sortDirtyRefs(baseRefs, dirtyStart)
				shardBucketDirtySortSink = sortedRefs[len(sortedRefs)/2]
			}
		})
	}
}

func BenchmarkShardBucketPostings_DirtyRepairBufferReuse(b *testing.B) {
	baseRefs, dirtyStart := dirtySortInput(65_536, 0, true)
	for _, mode := range []struct {
		name string
		cold bool
		warm bool
	}{
		{name: "recycler=off"},
		{name: "recycler=cold", cold: true},
		{name: "recycler=warm", warm: true},
	} {
		b.Run(mode.name, func(b *testing.B) {
			stats := &shardBucketRepairStats{}
			s := newShardBucketPostings(1)
			s.repairStats = stats
			var recycler *ShardedPostingsBufferRecycler
			var lifecycle *shardPostingsBufferLifecycle
			if mode.warm {
				recycler = NewShardedPostingsBufferRecycler(uint64(len(baseRefs))*seriesRefBytes*4, nil)
				lifecycle = newShardPostingsBufferLifecycle(recycler)
				recycler.put(make([]storage.SeriesRef, 0, len(baseRefs)+replacementTailHeadroom))
			}
			s.lifecycle = lifecycle
			var installed []storage.SeriesRef

			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				lifecycle.put(installed)
				if mode.cold {
					recycler = NewShardedPostingsBufferRecycler(uint64(len(baseRefs))*seriesRefBytes*4, nil)
					lifecycle = newShardPostingsBufferLifecycle(recycler)
					s.lifecycle = lifecycle
				}
				input := make([]storage.SeriesRef, len(baseRefs), len(baseRefs)+1)
				copy(input, baseRefs)
				bucket := &s.buckets[0]
				bucket.refs = input
				bucket.dirty = dirtyStart
				b.StartTimer()

				lease := acquireShardPostingsReader(lifecycle)
				lists, _ := s.postingsFor(0, 1)
				if !lists[0].Next() {
					b.Fatal("empty dirty bucket snapshot")
				}
				shardBucketDirtySortSink = lists[0].At()
				lease.close()

				b.StopTimer()
				installed = bucket.refs
				b.StartTimer()
			}
			b.ReportMetric(float64(stats.allocations.Load())/float64(b.N), "repair-buffer-allocs/op")
			b.ReportMetric(float64(stats.allocatedBytes.Load())/float64(b.N), "repair-buffer-bytes/op")
		})
	}
}

func BenchmarkShardedPostingsBufferRecycler_ConcurrentGetPut(b *testing.B) {
	const capacity = 65_536 + replacementTailHeadroom
	recycler := NewShardedPostingsBufferRecycler(uint64(maxShardedPostingsIdleBuffers*capacity)*seriesRefBytes, nil)
	for range maxShardedPostingsIdleBuffers {
		recycler.put(make([]storage.SeriesRef, 0, capacity))
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			refs := recycler.get(capacity, capacity)
			if refs == nil {
				refs = make([]storage.SeriesRef, capacity)
			}
			shardBucketDirtySortSink = refs[0]
			recycler.put(refs)
		}
	})
}

func BenchmarkShardBucketPostings_ConcurrentDirtySort(b *testing.B) {
	const concurrentRefBase chunks.HeadSeriesRef = 1 << 40

	baseRefs, dirtyStart := dirtySortInput(65_536, 0, true)
	for _, tc := range []struct {
		name       string
		tailGrowth bool
	}{
		{name: "during-sort"},
		{name: "during-tail-growth", tailGrowth: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			tailCapacity := 1
			if tc.tailGrowth {
				tailCapacity = replacementTailHeadroom + 2
			}
			inputRefs := make([]storage.SeriesRef, len(baseRefs), len(baseRefs)+tailCapacity)
			copy(inputRefs, baseRefs)
			addLatency := make([]int64, 0, b.N)
			b.ReportAllocs()

			for b.Loop() {
				b.StopTimer()
				s := newShardBucketPostings(1)
				bucket := &s.buckets[0]
				bucket.mtx.Lock()
				bucket.refs = inputRefs
				bucket.dirty = dirtyStart
				bucket.mtx.Unlock()

				addDone := make(chan int64, 1)
				startMeasuredAdd := func(ref chunks.HeadSeriesRef) {
					addAttempted := make(chan struct{})
					go func() {
						start := time.Now()
						close(addAttempted)
						s.add(ref, 0)
						addDone <- time.Since(start).Nanoseconds()
					}()
					<-addAttempted
					runtime.Gosched()
				}
				if tc.tailGrowth {
					s.sortUnlockedHook = func() {
						for i := range replacementTailHeadroom + 1 {
							s.add(concurrentRefBase+chunks.HeadSeriesRef(i), 0)
						}
					}
					s.sortTailCarryHook = func() {
						startMeasuredAdd(concurrentRefBase << 1)
					}
				} else {
					s.sortUnlockedHook = func() {
						startMeasuredAdd(concurrentRefBase)
					}
				}
				b.StartTimer()

				lists, _ := s.postingsFor(0, 1)
				if !lists[0].Next() {
					b.Fatal("empty dirty bucket snapshot")
				}
				shardBucketDirtySortSink = lists[0].At()
				addLatency = append(addLatency, <-addDone)
			}
			b.StopTimer()

			slices.Sort(addLatency)
			b.ReportMetric(float64(shardBucketPercentile(addLatency, 50, 100)), "add-latency-p50-ns")
			b.ReportMetric(float64(shardBucketPercentile(addLatency, 99, 100)), "add-latency-p99-ns")
		})
	}
}

// BenchmarkShardBucketPostings_Footprint reports bucket capacity and fixed
// overhead per live series after churn.
func BenchmarkShardBucketPostings_Footprint(b *testing.B) {
	const live = 100_000
	for _, churn := range []int{0, 1} {
		b.Run(fmt.Sprintf("churn=%dx", churn), func(b *testing.B) {
			var capEntries, entries int
			var fixedBytes uintptr
			for b.Loop() {
				s := newShardBucketPostings(DefaultShardedPostingsBuckets)
				rng := rand.New(rand.NewSource(1))
				total := (1 + churn) * live
				refHashes := make(map[storage.SeriesRef]uint64, total)
				for ref := 1; ref <= total; ref++ {
					hash := rng.Uint64()
					refHashes[storage.SeriesRef(ref)] = hash
					s.add(chunks.HeadSeriesRef(ref), hash)
				}
				// Remove churn*live of them, leaving `live` live.
				deleted := make(map[storage.SeriesRef]uint64, churn*live)
				for ref := 1; ref <= churn*live; ref++ {
					deleted[storage.SeriesRef(ref)] = refHashes[storage.SeriesRef(ref)]
				}
				s.remove(deleted)

				capEntries, entries = 0, s.numSeries()
				fixedBytes = unsafe.Sizeof(shardBucket{}) * uintptr(len(s.buckets))
				for i := range s.buckets {
					bucket := &s.buckets[i]
					capEntries += cap(bucket.refs)
				}
			}
			// Each entry costs one storage.SeriesRef.
			b.ReportMetric(float64(capEntries)*float64(unsafe.Sizeof(storage.SeriesRef(0)))/float64(live), "capbytes/live")
			b.ReportMetric(float64(capEntries)/float64(entries), "cap/len")
			b.ReportMetric(float64(fixedBytes)/float64(live), "fixedbytes/live")
		})
	}
}

type shardBucketDeletePattern struct {
	name          string
	every         int
	sparseBuckets []uint64
}

const shardBucketBenchmarkHashCount = 1 << 16

var (
	shardBucketBenchmarkShardCounts = [...]uint64{16, 64, 128}
	shardBucketDeletePatterns       = [...]shardBucketDeletePattern{
		{name: "dense/churn=1/2", every: 2},
		{name: "dense/churn=1/8", every: 8},
		{name: "sparse/spread4/churn=1/8", every: 8, sparseBuckets: []uint64{0, 32, 64, 96}},
	}
)

func newShardBucketBenchmarkHashes() []uint64 {
	hashes := make([]uint64, shardBucketBenchmarkHashCount)
	rng := rand.New(rand.NewSource(1))
	for i := range hashes {
		hashes[i] = rng.Uint64()
	}
	return hashes
}

func newPopulatedShardBucketPostings(numSeries int, hashes []uint64) *shardBucketPostings {
	s := newShardBucketPostings(DefaultShardedPostingsBuckets)
	for i := range numSeries {
		s.add(chunks.HeadSeriesRef(i+1), hashes[i%len(hashes)])
	}
	return s
}

func shardBucketBenchmarkHash(hashes []uint64, ref storage.SeriesRef) uint64 {
	return hashes[(int(ref)-1)%len(hashes)]
}

func shardBucketBenchmarkBucket(hashes []uint64, ref storage.SeriesRef) uint64 {
	return shardBucketBenchmarkHash(hashes, ref) & uint64(DefaultShardedPostingsBuckets-1)
}

func shardBucketDeletedRefs(tb testing.TB, numSeries int, hashes []uint64, pattern shardBucketDeletePattern) (map[storage.SeriesRef]uint64, map[uint64]struct{}) {
	tb.Helper()

	var deleted map[storage.SeriesRef]uint64
	touched := map[uint64]struct{}{}
	if len(pattern.sparseBuckets) == 0 {
		deleted = make(map[storage.SeriesRef]uint64, numSeries/pattern.every)
		for ref := storage.SeriesRef(1); ref <= storage.SeriesRef(numSeries); ref++ {
			if int(ref)%pattern.every != 0 {
				continue
			}
			deleted[ref] = shardBucketBenchmarkHash(hashes, ref)
			touched[shardBucketBenchmarkBucket(hashes, ref)] = struct{}{}
		}
		validateShardBucketDeletedRefs(tb, deleted, touched, allShardBucketIDs())
		return deleted, touched
	}

	selectedBuckets := make(map[uint64]struct{}, len(pattern.sparseBuckets))
	for _, b := range pattern.sparseBuckets {
		selectedBuckets[b] = struct{}{}
	}
	counters := make(map[uint64]int, len(pattern.sparseBuckets))
	deleted = make(map[storage.SeriesRef]uint64, numSeries*len(pattern.sparseBuckets)/DefaultShardedPostingsBuckets/pattern.every)
	for ref := storage.SeriesRef(1); ref <= storage.SeriesRef(numSeries); ref++ {
		bucket := shardBucketBenchmarkBucket(hashes, ref)
		if _, ok := selectedBuckets[bucket]; !ok {
			continue
		}
		counters[bucket]++
		if counters[bucket]%pattern.every != 0 {
			continue
		}
		deleted[ref] = shardBucketBenchmarkHash(hashes, ref)
		touched[bucket] = struct{}{}
	}
	validateShardBucketDeletedRefs(tb, deleted, touched, pattern.sparseBuckets)
	return deleted, touched
}

func allShardBucketIDs() []uint64 {
	ids := make([]uint64, DefaultShardedPostingsBuckets)
	for i := range ids {
		ids[i] = uint64(i)
	}
	return ids
}

func validateShardBucketDeletedRefs(tb testing.TB, deleted map[storage.SeriesRef]uint64, touched map[uint64]struct{}, expected []uint64) {
	tb.Helper()
	require.NotEmpty(tb, deleted)

	got := make([]uint64, 0, len(touched))
	for b := range touched {
		got = append(got, b)
	}
	slices.Sort(got)
	require.Equal(tb, expected, got)
}

type shardBucketReadSample struct {
	snapshotNS int64
	readNS     int64
}

func benchmarkShardBucketRead(ctx context.Context, s *shardBucketPostings, shardIndex, shardCount uint64) (shardBucketReadSample, error) {
	start := time.Now()
	lists, _ := s.postingsFor(shardIndex, shardCount)
	snapshotNS := time.Since(start).Nanoseconds()
	p := index.Merge(ctx, lists...)
	for p.Next() {
	}
	if err := p.Err(); err != nil {
		return shardBucketReadSample{}, err
	}
	return shardBucketReadSample{
		snapshotNS: snapshotNS,
		readNS:     time.Since(start).Nanoseconds(),
	}, nil
}

func collectShardBucketBaselineSamples(b *testing.B, s *shardBucketPostings, shardCount uint64, readers int) []shardBucketReadSample {
	b.Helper()

	const baselineSamples = 256

	samples := make([]shardBucketReadSample, baselineSamples)
	var next atomic.Int64
	recordErr, firstErr := shardBucketFirstError()
	var wg sync.WaitGroup
	for r := range readers {
		wg.Go(func() {
			shard := uint64(r) % shardCount
			for {
				i := int(next.Inc() - 1)
				if i >= len(samples) {
					return
				}
				sample, err := benchmarkShardBucketRead(b.Context(), s, shard, shardCount)
				if err != nil {
					recordErr(err)
					return
				}
				samples[i] = sample
				shard = (shard + uint64(readers)) % shardCount
			}
		})
	}
	wg.Wait()
	if err := firstErr(); err != nil {
		b.Fatal(err)
	}
	return samples
}

func shardBucketFirstError() (func(error), func() error) {
	var mu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	return recordErr, func() error {
		mu.Lock()
		defer mu.Unlock()
		return firstErr
	}
}

func shardBucketReadSampleDurations(samples []shardBucketReadSample) ([]int64, []int64) {
	snapshots := make([]int64, len(samples))
	reads := make([]int64, len(samples))
	for i, sample := range samples {
		snapshots[i] = sample.snapshotNS
		reads[i] = sample.readNS
	}
	slices.Sort(snapshots)
	slices.Sort(reads)
	return snapshots, reads
}

func shardBucketPercentile(sorted []int64, numerator, denominator int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted)*numerator + denominator - 1) / denominator
	if i <= 0 {
		return sorted[0]
	}
	if i > len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[i-1]
}

func reportShardBucketDurationMetrics(b *testing.B, name string, baseline, samples []int64) {
	b.Helper()

	baselineP99 := shardBucketPercentile(baseline, 99, 100)
	p99 := shardBucketPercentile(samples, 99, 100)
	b.ReportMetric(float64(baselineP99), "baseline-"+name+"-p99-ns")
	b.ReportMetric(float64(shardBucketPercentile(samples, 50, 100)), name+"-p50-ns")
	b.ReportMetric(float64(p99), name+"-p99-ns")
	b.ReportMetric(float64(samples[len(samples)-1]), name+"-max-ns")
	if baselineP99 > 0 {
		b.ReportMetric(float64(p99)/float64(baselineP99), name+"-p99-over-baseline-ratio")
	}
}

func reportShardBucketReadMetrics(b *testing.B, baseline, samples []shardBucketReadSample, readers int) {
	b.Helper()

	if len(samples) == 0 {
		b.Fatal("no concurrent remove/read samples collected")
	}

	baselineSnapshots, baselineReads := shardBucketReadSampleDurations(baseline)
	snapshots, reads := shardBucketReadSampleDurations(samples)
	reportShardBucketDurationMetrics(b, "snapshot", baselineSnapshots, snapshots)
	reportShardBucketDurationMetrics(b, "read", baselineReads, reads)
	b.ReportMetric(float64(len(samples))/float64(b.N), "samples/remove")
	b.ReportMetric(float64(len(samples)), "samples-total")
	b.ReportMetric(float64(readers), "reader-goroutines")
}

func benchmarkShardBucketConcurrentRemoveRead(b *testing.B, s *shardBucketPostings, deleted map[storage.SeriesRef]uint64, shardCount uint64, readers int) []shardBucketReadSample {
	b.Helper()

	const samplesPerReader = 32_768

	readerSamples := make([][]shardBucketReadSample, readers)
	for i := range readerSamples {
		readerSamples[i] = make([]shardBucketReadSample, 0, samplesPerReader)
	}

	var measuring atomic.Bool
	var overflow atomic.Bool
	done := make(chan struct{})
	steadyStart := make(chan struct{})
	recordErr, firstErr := shardBucketFirstError()
	var wg, warmup, steadyReady sync.WaitGroup
	for r := range readers {
		warmup.Add(1)
		steadyReady.Add(1)
		wg.Go(func() {
			shard := uint64(r) % shardCount
			if _, err := benchmarkShardBucketRead(b.Context(), s, shard, shardCount); err != nil {
				recordErr(err)
				warmup.Done()
				steadyReady.Done()
				return
			}
			warmup.Done()

			select {
			case <-steadyStart:
			case <-done:
				steadyReady.Done()
				return
			}

			ready := false
			for {
				select {
				case <-done:
					if !ready {
						steadyReady.Done()
					}
					return
				default:
				}

				measured := measuring.Load()
				sample, err := benchmarkShardBucketRead(b.Context(), s, shard, shardCount)
				if err != nil {
					recordErr(err)
					if !ready {
						steadyReady.Done()
					}
					return
				}
				if measured {
					if len(readerSamples[r]) == cap(readerSamples[r]) {
						overflow.Store(true)
					} else {
						readerSamples[r] = append(readerSamples[r], sample)
					}
				}
				if !ready {
					ready = true
					steadyReady.Done()
				}
				shard = (shard + uint64(readers)) % shardCount
			}
		})
	}

	warmup.Wait()
	if err := firstErr(); err != nil {
		close(done)
		close(steadyStart)
		wg.Wait()
		b.Fatal(err)
	}
	close(steadyStart)
	steadyReady.Wait()
	if err := firstErr(); err != nil {
		close(done)
		wg.Wait()
		b.Fatal(err)
	}

	b.StartTimer()
	measuring.Store(true)
	s.remove(deleted)
	b.StopTimer()
	measuring.Store(false)
	close(done)
	wg.Wait()
	if err := firstErr(); err != nil {
		b.Fatal(err)
	}
	if overflow.Load() {
		b.Fatalf("concurrent remove/read sample buffer overflowed with capacity %d per reader", samplesPerReader)
	}

	var samples []shardBucketReadSample
	for _, reader := range readerSamples {
		samples = append(samples, reader...)
	}
	return samples
}

func BenchmarkShardBucketPostings_Add(b *testing.B) {
	hashes := newShardBucketBenchmarkHashes()

	b.Run("serial", func(b *testing.B) {
		s := newShardBucketPostings(DefaultShardedPostingsBuckets)
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			s.add(chunks.HeadSeriesRef(i+1), hashes[i%shardBucketBenchmarkHashCount])
		}
	})

	// Parallel exposes bucket-lock contention during concurrent series creation.
	b.Run("parallel", func(b *testing.B) {
		s := newShardBucketPostings(DefaultShardedPostingsBuckets)
		var goroutine atomic.Int64
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			// Disjoint monotonic ranges make only cross-goroutine interleaving
			// dirty buckets, matching globally increasing production series IDs.
			worker := goroutine.Inc() - 1
			ref := chunks.HeadSeriesRef((worker + 1) << 40)
			i := int(worker*9973) % shardBucketBenchmarkHashCount
			for pb.Next() {
				s.add(ref, hashes[i%shardBucketBenchmarkHashCount])
				ref++
				i++
			}
		})
	})
}

func BenchmarkShardBucketPostings_PostingsFor(b *testing.B) {
	const numSeries = 1_000_000
	hashes := newShardBucketBenchmarkHashes()

	// Population leaves buckets sorted, so this measures steady-state reads.
	for _, shardCount := range shardBucketBenchmarkShardCounts {
		b.Run(fmt.Sprintf("shardCount=%d", shardCount), func(b *testing.B) {
			s := newPopulatedShardBucketPostings(numSeries, hashes)
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				lists, _ := s.postingsFor(uint64(i)%shardCount, shardCount)
				if _, err := index.ExpandPostings(index.Merge(b.Context(), lists...)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkShardBucketPostings_Remove(b *testing.B) {
	const numSeries = 2_000_000
	hashes := newShardBucketBenchmarkHashes()

	// Repopulation is untimed; each iteration measures one removal.
	for _, pattern := range shardBucketDeletePatterns {
		b.Run(pattern.name, func(b *testing.B) {
			deleted, touched := shardBucketDeletedRefs(b, numSeries, hashes, pattern)
			b.ReportMetric(float64(len(touched)), "touched_buckets")
			for b.Loop() {
				b.StopTimer()
				s := newPopulatedShardBucketPostings(numSeries, hashes)
				b.StartTimer()
				s.remove(deleted)
			}
		})
	}
}

func BenchmarkShardBucketPostings_ConcurrentRemoveRead(b *testing.B) {
	const numSeries = 2_000_000
	hashes := newShardBucketBenchmarkHashes()
	readers := runtime.GOMAXPROCS(0)

	// Allocation metrics include readers; Remove remains the pure removal signal.
	for _, pattern := range shardBucketDeletePatterns {
		deleted, touched := shardBucketDeletedRefs(b, numSeries, hashes, pattern)
		for _, shardCount := range shardBucketBenchmarkShardCounts {
			b.Run(fmt.Sprintf("%s/shardCount=%d", pattern.name, shardCount), func(b *testing.B) {
				baseline := collectShardBucketBaselineSamples(b, newPopulatedShardBucketPostings(numSeries, hashes), shardCount, readers)
				samples := make([]shardBucketReadSample, 0, readers*128)
				var sampledRemoves, zeroSampleRemoves int
				b.ReportMetric(float64(len(touched)), "touched_buckets")
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					s := newPopulatedShardBucketPostings(numSeries, hashes)
					iterSamples := benchmarkShardBucketConcurrentRemoveRead(b, s, deleted, shardCount, readers)
					if len(iterSamples) == 0 {
						zeroSampleRemoves++
					} else {
						sampledRemoves++
					}
					samples = append(samples, iterSamples...)
					b.StartTimer()
				}
				b.StopTimer()
				// Fast sparse removals can finish before one reader sample.
				b.ReportMetric(float64(sampledRemoves), "sampled-removes")
				b.ReportMetric(float64(zeroSampleRemoves), "zero-sample-removes")
				reportShardBucketReadMetrics(b, baseline, samples, readers)
			})
		}
	}
}
