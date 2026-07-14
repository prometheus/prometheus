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

		genBefore := s.buckets[0].gen
		// The deleted refs map to bucket 0 by shard hash but are not present
		// in it: removal must leave the bucket untouched, without taking its
		// write lock (observable through the unchanged generation).
		s.remove(map[storage.SeriesRef]uint64{1000: 0, 1002: 2})
		require.Equal(t, genBefore, s.buckets[0].gen)
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

	t.Run("falls back when the bucket is replaced during the unlocked rebuild", func(t *testing.T) {
		t.Parallel()
		s := newShardBucketPostings(1)
		// Out-of-order adds leave the bucket dirty, so a read during the
		// unlocked rebuild re-sorts it and replaces the refs slice.
		for _, ref := range []chunks.HeadSeriesRef{2, 6, 4, 8} {
			s.add(ref, 0)
		}
		require.NotEqual(t, cleanShardBucket, s.buckets[0].dirty)

		genBefore := s.buckets[0].gen
		s.removeUnlockedHook = func() {
			// Sorts the dirty bucket, bumping the generation.
			expandShardCandidates(t, s, 0, 1)
		}
		s.remove(map[storage.SeriesRef]uint64{4: 0})

		// One bump from the re-sort, one from the fallback rebuild.
		require.Equal(t, genBefore+2, s.buckets[0].gen)
		require.Equal(t, []storage.SeriesRef{2, 6, 8}, expandShardCandidates(t, s, 0, 1))
	})
}

func TestShardBucketPostings_Concurrency(t *testing.T) {
	t.Run("multi-bucket read holds earlier candidate bucket while waiting for later bucket", func(t *testing.T) {
		s := newShardBucketPostings(4)
		s.add(1, 0)
		s.add(3, 2)
		require.Equal(t, []storage.SeriesRef{1, 3}, expandShardCandidates(t, s, 0, 2))

		later := &s.buckets[2]
		later.mtx.Lock()
		laterLocked := true
		defer func() {
			if laterLocked {
				later.mtx.Unlock()
			}
		}()

		type readResult struct {
			refs []storage.SeriesRef
			err  error
		}
		readDone := make(chan readResult, 1)
		go func() {
			lists, _ := s.postingsFor(0, 2)
			refs, err := index.ExpandPostings(index.Merge(t.Context(), lists...))
			readDone <- readResult{refs: refs, err: err}
		}()

		require.Eventually(t, func() bool {
			if s.buckets[0].mtx.TryLock() {
				s.buckets[0].mtx.Unlock()
				return false
			}
			return true
		}, time.Second, time.Millisecond)

		addDone := make(chan struct{})
		go func() {
			s.add(5, 4)
			close(addDone)
		}()

		select {
		case <-addDone:
			t.Fatal("add to earlier candidate bucket completed while multi-bucket read was blocked on later bucket")
		case <-time.After(50 * time.Millisecond):
		}

		later.mtx.Unlock()
		laterLocked = false

		select {
		case result := <-readDone:
			require.NoError(t, result.err)
			require.Equal(t, []storage.SeriesRef{1, 3}, result.refs)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for multi-bucket read")
		}

		select {
		case <-addDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for blocked add")
		}
		require.Equal(t, []storage.SeriesRef{1, 3, 5}, expandShardCandidates(t, s, 0, 2))
	})

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
			bucket := shardBucket{}
			bucket.mtx.Lock()
			defer bucket.mtx.Unlock()

			b.ReportAllocs()
			for b.Loop() {
				bucket.refs = baseRefs
				bucket.dirty = dirtyStart
				sortDirtyBucketLocked(&bucket)
				shardBucketDirtySortSink = bucket.refs[len(bucket.refs)/2]
			}
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
