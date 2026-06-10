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
	"slices"
	"sync"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

// DefaultShardedPostingsBuckets is the default number of shard hash buckets
// the head indexes series into when sharding is enabled.
const DefaultShardedPostingsBuckets = 64

// shardBucketPostings holds one sorted list of series refs per shard hash
// bucket, so that postings can be filtered by shard through sorted-list
// intersection instead of a per-series lookup. Memory is proportional to the
// number of series held: one ref per series.
//
// A series is added to the list of bucket shardHash % len(buckets) when it is
// created, before it becomes visible in the head postings index, and refs are
// removed after deleted series have been removed from the postings index.
// Together this guarantees the invariant readers depend on: every ref
// readable from the postings index is present in its bucket list. Refs of
// deleted series may linger until the next removal and are resolved by
// readers like any other stale postings entry.
//
// Refs increase monotonically, so adds normally append in sorted position;
// out-of-order adds (concurrent creations racing, snapshot replay) mark the
// bucket dirty and it is re-sorted into a fresh slice the next time it is
// read. List contents visible to a reader are never mutated in place, so
// returned postings remain valid after the lock is released.
type shardBucketPostings struct {
	mtx     sync.RWMutex
	buckets [][]storage.SeriesRef
	dirty   []bool // Buckets appended out of order; re-sorted on next read.
}

func newShardBucketPostings(buckets int) *shardBucketPostings {
	return &shardBucketPostings{
		buckets: make([][]storage.SeriesRef, buckets),
		dirty:   make([]bool, buckets),
	}
}

// add records a newly created series in its shard hash bucket.
func (s *shardBucketPostings) add(ref chunks.HeadSeriesRef, shardHash uint64) {
	b := shardHash % uint64(len(s.buckets))
	s.mtx.Lock()
	list := s.buckets[b]
	if n := len(list); n > 0 && list[n-1] >= storage.SeriesRef(ref) {
		s.dirty[b] = true
	}
	s.buckets[b] = append(list, storage.SeriesRef(ref))
	s.mtx.Unlock()
}

// remove drops the given deleted series refs from the bucket lists. Buckets
// containing any deleted ref are replaced with filtered copies, so reader
// snapshots stay intact. A nil receiver (sharding disabled) is a no-op.
func (s *shardBucketPostings) remove(deleted map[storage.SeriesRef]struct{}) {
	if s == nil || len(deleted) == 0 {
		return
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for b, list := range s.buckets {
		first := -1
		for i, ref := range list {
			if _, ok := deleted[ref]; ok {
				first = i
				break
			}
		}
		if first < 0 {
			continue
		}
		repl := make([]storage.SeriesRef, first, len(list)-1)
		copy(repl, list[:first])
		for _, ref := range list[first+1:] {
			if _, ok := deleted[ref]; !ok {
				repl = append(repl, ref)
			}
		}
		s.buckets[b] = repl
	}
}

// postingsFor returns one sorted postings list per bucket belonging to the
// given shard, or ok == false when the shard count does not divide the
// bucket count, in which case the caller must filter per series. A nil
// receiver (sharding disabled) always returns false.
func (s *shardBucketPostings) postingsFor(shardIndex, shardCount uint64) ([]index.Postings, bool) {
	if s == nil || shardCount == 0 || uint64(len(s.buckets))%shardCount != 0 {
		return nil, false
	}
	s.mtx.RLock()
	for s.anyDirtyLocked(shardIndex, shardCount) {
		s.mtx.RUnlock()
		s.sortDirty()
		s.mtx.RLock()
	}
	lists := make([]index.Postings, 0, uint64(len(s.buckets))/shardCount)
	for b := shardIndex; b < uint64(len(s.buckets)); b += shardCount {
		lists = append(lists, index.NewListPostings(s.buckets[b]))
	}
	s.mtx.RUnlock()
	return lists, true
}

// numSeries returns the total number of refs held, including refs of deleted
// series that have not been removed yet. A nil receiver returns 0.
func (s *shardBucketPostings) numSeries() int {
	if s == nil {
		return 0
	}
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	n := 0
	for _, list := range s.buckets {
		n += len(list)
	}
	return n
}

// anyDirtyLocked reports whether any bucket of the given shard needs
// re-sorting. The caller must hold the read lock.
func (s *shardBucketPostings) anyDirtyLocked(shardIndex, shardCount uint64) bool {
	for b := shardIndex; b < uint64(len(s.buckets)); b += shardCount {
		if s.dirty[b] {
			return true
		}
	}
	return false
}

// sortDirty replaces dirty buckets with sorted copies.
func (s *shardBucketPostings) sortDirty() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for b, d := range s.dirty {
		if !d {
			continue
		}
		sorted := slices.Clone(s.buckets[b])
		slices.Sort(sorted)
		s.buckets[b] = sorted
		s.dirty[b] = false
	}
}

// shardFilterPostings filters the input postings to the refs present in the
// shard's bucket postings. The input is advanced strictly sequentially —
// never seeked — because it may be an arbitrarily complex postings tree
// whose Seek is expensive; the bucket side consists of flat sorted lists
// whose forward Seek is cheap.
type shardFilterPostings struct {
	input, buckets index.Postings
	cur            storage.SeriesRef
}

func newShardFilterPostings(input, buckets index.Postings) *shardFilterPostings {
	return &shardFilterPostings{input: input, buckets: buckets}
}

func (f *shardFilterPostings) Next() bool {
	for f.input.Next() {
		ref := f.input.At()
		if !f.buckets.Seek(ref) {
			// The buckets are exhausted: no later input ref can belong to
			// the shard either.
			return false
		}
		if f.buckets.At() == ref {
			f.cur = ref
			return true
		}
		// ref is not in the shard. Input refs below the buckets' position
		// make the Seek above a no-op, so this loop never seeks backward.
	}
	return false
}

// Seek advances sequentially: callers of sharded postings drain them with
// Next, so an input-preserving linear Seek is preferred over seeking the
// (potentially expensive) input.
func (f *shardFilterPostings) Seek(v storage.SeriesRef) bool {
	// cur == 0 means no successful Next yet: 0 is never a valid series ref.
	if f.cur != 0 && f.cur >= v {
		return true
	}
	for f.Next() {
		if f.cur >= v {
			return true
		}
	}
	return false
}

func (f *shardFilterPostings) At() storage.SeriesRef {
	return f.cur
}

func (f *shardFilterPostings) Err() error {
	if err := f.input.Err(); err != nil {
		return err
	}
	return f.buckets.Err()
}
