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

// DefaultShardedPostingsBuckets is the default number of shard hash buckets the
// head indexes series into when sharding is enabled. It is a power of two large
// enough that common query shard counts divide it and so use the exact
// (non-sub-filtered) shard postings path.
const DefaultShardedPostingsBuckets = 128

func isPowerOfTwo(v uint64) bool {
	return v != 0 && v&(v-1) == 0
}

// shardBucketPostings holds, per shard hash bucket, one sorted list of series
// refs and the matching shard hashes, so that postings can be filtered by shard
// through sorted-list intersection instead of a per-series lookup. When the
// requested power-of-two shard count exceeds the bucket count, a bucket holds
// series from several shards; the stored hashes then let the bucket be
// sub-filtered (hash % shardCount == shardIndex) without resolving each series.
// Memory is proportional to the number of series held: one ref and one hash per
// series.
//
// A series is added to bucket shardHash % len(buckets) when it is created,
// before it becomes visible in the head postings index, and refs are removed
// after deleted series have been removed from the postings index. Together this
// guarantees the invariant readers depend on: every ref readable from the
// postings index is present in its bucket list. Refs of deleted series may
// linger until the next removal and are resolved by readers like any other
// stale postings entry.
//
// Refs increase monotonically, so adds normally append in sorted position;
// out-of-order adds (concurrent creations racing, snapshot replay) mark the
// bucket dirty and its refs and hashes are re-sorted together into fresh slices
// the next time it is read. The dirty index tracks the earliest dirty suffix;
// mostly sorted buckets sort that suffix and merge it with the sorted prefix.
// List contents visible to a reader are never mutated in place, so returned
// postings remain valid after the lock is released.
type shardBucketPostings struct {
	mtx     sync.RWMutex
	buckets [][]storage.SeriesRef
	hashes  [][]uint64 // hashes[b][i] is the shard hash of buckets[b][i]; kept aligned and co-sorted by ref.
	dirty   []int      // Earliest out-of-order suffix index per bucket, or cleanShardBucket when clean.
}

const cleanShardBucket = -1

func newShardBucketPostings(buckets int) *shardBucketPostings {
	dirty := make([]int, buckets)
	for i := range dirty {
		dirty[i] = cleanShardBucket
	}
	return &shardBucketPostings{
		buckets: make([][]storage.SeriesRef, buckets),
		hashes:  make([][]uint64, buckets),
		dirty:   dirty,
	}
}

// add records a newly created series in its shard hash bucket.
func (s *shardBucketPostings) add(ref chunks.HeadSeriesRef, shardHash uint64) {
	if s == nil || len(s.buckets) == 0 {
		return
	}
	// len(s.buckets) is validated to be a power of two, so this is
	// shardHash % len(s.buckets) without runtime division.
	b := shardHash & uint64(len(s.buckets)-1)
	s.mtx.Lock()
	list := s.buckets[b]
	if n := len(list); n > 0 && list[n-1] >= storage.SeriesRef(ref) {
		if s.dirty[b] == cleanShardBucket || n < s.dirty[b] {
			s.dirty[b] = n
		}
	}
	s.buckets[b] = append(list, storage.SeriesRef(ref))
	s.hashes[b] = append(s.hashes[b], shardHash)
	s.mtx.Unlock()
}

// remove drops the given deleted series refs from the bucket lists. Buckets
// containing any deleted ref are replaced with filtered copies, refs and hashes
// in lockstep, so reader snapshots stay intact. A nil receiver (sharding
// disabled) is a no-op.
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
		hashes := s.hashes[b]
		// Size the rebuilt slices to the surviving count, not the pre-removal
		// length: keeping cap at len(list)-1 leaves the dropped refs' capacity
		// resident until the next rebuild, which under series churn keeps the
		// index several times larger than the live series it holds.
		survivors := first
		for i := first + 1; i < len(list); i++ {
			if _, ok := deleted[list[i]]; !ok {
				survivors++
			}
		}
		repl := make([]storage.SeriesRef, first, survivors)
		replH := make([]uint64, first, survivors)
		copy(repl, list[:first])
		copy(replH, hashes[:first])
		for i := first + 1; i < len(list); i++ {
			if _, ok := deleted[list[i]]; !ok {
				repl = append(repl, list[i])
				replH = append(replH, hashes[i])
			}
		}
		s.buckets[b] = repl
		s.hashes[b] = replH
		if s.dirty[b] != cleanShardBucket {
			if len(repl) == 0 {
				s.dirty[b] = cleanShardBucket
			} else {
				s.dirty[b] = 0
			}
		}
	}
}

// postingsFor returns sorted postings lists that together cover exactly the
// series of the given shard. Both bucket count and shard count must be powers of
// two. When shardCount <= bucketCount, each list is a whole bucket. When
// shardCount > bucketCount, the shard maps to one candidate bucket, which is
// wrapped in a sub-filter on the shard hash and subFiltered is true (the caller
// may account for the more expensive path).
//
// A series is in shard i of N iff shardHash % N == i and in bucket j of B iff
// shardHash % B == j. If N <= B and both are powers of two, N divides B, so the
// shard is exactly buckets i, i+N, ... . If N > B, B divides N, so all series in
// shard i must be in the single bucket i % B, and that bucket is sub-filtered by
// shardHash % N == i.
//
// A nil receiver (sharding disabled) or a zero shard count returns no lists.
func (s *shardBucketPostings) postingsFor(shardIndex, shardCount uint64) (lists []index.Postings, subFiltered bool) {
	if s == nil || shardCount == 0 {
		return nil, false
	}
	bucketCount := uint64(len(s.buckets))
	if bucketCount == 0 {
		return nil, false
	}

	base := shardIndex
	step := shardCount
	if shardCount > bucketCount {
		// bucketCount is a power of two here, so this is
		// shardIndex % bucketCount without runtime division.
		base = shardIndex & (bucketCount - 1)
		step = bucketCount
		subFiltered = true
	}

	s.mtx.RLock()
	for s.anyDirtyLocked(base, step) {
		s.mtx.RUnlock()
		s.sortDirty(base, step)
		s.mtx.RLock()
	}
	lists = make([]index.Postings, 0, bucketCount/step)
	for b := base; b < bucketCount; b += step {
		if subFiltered {
			// Capture both slice headers under the read lock so the sub-filter
			// reads a consistent (ref, hash) pair even if the bucket is later
			// re-sorted or rebuilt.
			lists = append(lists, newShardHashFilterPostings(s.buckets[b], s.hashes[b], shardIndex, shardCount))
		} else {
			lists = append(lists, index.NewListPostings(s.buckets[b]))
		}
	}
	s.mtx.RUnlock()
	return lists, subFiltered
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

// anyDirtyLocked reports whether any candidate bucket (those stepped by step
// from base) needs re-sorting. The caller must hold the read lock.
func (s *shardBucketPostings) anyDirtyLocked(base, step uint64) bool {
	for b := base; b < uint64(len(s.buckets)); b += step {
		if s.dirty[b] != cleanShardBucket {
			return true
		}
	}
	return false
}

// sortDirty replaces dirty candidate buckets with sorted copies, permuting each
// bucket's hashes by the same order so refs and hashes stay aligned.
func (s *shardBucketPostings) sortDirty(base, step uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for b := base; b < uint64(len(s.buckets)); b += step {
		dirtyStart := s.dirty[b]
		if dirtyStart == cleanShardBucket {
			continue
		}
		refs := s.buckets[b]
		hashes := s.hashes[b]
		var sortedRefs []storage.SeriesRef
		var sortedHashes []uint64
		if dirtyStart < len(refs)/2 {
			sortedRefs, sortedHashes = sortRefHashesFull(refs, hashes)
		} else {
			sortedRefs, sortedHashes = sortRefHashesSuffix(refs, hashes, dirtyStart)
		}
		s.buckets[b] = sortedRefs
		s.hashes[b] = sortedHashes
		s.dirty[b] = cleanShardBucket
	}
}

func sortRefHashesFull(refs []storage.SeriesRef, hashes []uint64) ([]storage.SeriesRef, []uint64) {
	order := make([]int, len(refs))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(x, y int) int {
		switch {
		case refs[x] < refs[y]:
			return -1
		case refs[x] > refs[y]:
			return 1
		default:
			return 0
		}
	})
	sortedRefs := make([]storage.SeriesRef, len(refs))
	sortedHashes := make([]uint64, len(refs))
	for newPos, old := range order {
		sortedRefs[newPos] = refs[old]
		sortedHashes[newPos] = hashes[old]
	}
	return sortedRefs, sortedHashes
}

func sortRefHashesSuffix(refs []storage.SeriesRef, hashes []uint64, dirtyStart int) ([]storage.SeriesRef, []uint64) {
	if dirtyStart <= 0 {
		return sortRefHashesFull(refs, hashes)
	}
	if dirtyStart >= len(refs) {
		return slices.Clone(refs), slices.Clone(hashes)
	}

	tailRefs, tailHashes := sortRefHashesFull(refs[dirtyStart:], hashes[dirtyStart:])
	sortedRefs := make([]storage.SeriesRef, len(refs))
	sortedHashes := make([]uint64, len(refs))
	i, j, out := 0, 0, 0
	for i < dirtyStart && j < len(tailRefs) {
		if refs[i] <= tailRefs[j] {
			sortedRefs[out] = refs[i]
			sortedHashes[out] = hashes[i]
			i++
		} else {
			sortedRefs[out] = tailRefs[j]
			sortedHashes[out] = tailHashes[j]
			j++
		}
		out++
	}
	for i < dirtyStart {
		sortedRefs[out] = refs[i]
		sortedHashes[out] = hashes[i]
		i++
		out++
	}
	for j < len(tailRefs) {
		sortedRefs[out] = tailRefs[j]
		sortedHashes[out] = tailHashes[j]
		j++
		out++
	}
	return sortedRefs, sortedHashes
}

// shardHashFilterPostings yields the refs of one bucket whose shard hash maps to
// the requested shard, i.e. hash % shardCount == shardIndex. It is used when the
// shard count exceeds the bucket count, so a bucket holds series from several
// shards. refs is sorted ascending and hashes is aligned to it, so the emitted
// refs stay sorted.
type shardHashFilterPostings struct {
	refs       []storage.SeriesRef
	hashes     []uint64
	shardIndex uint64
	shardCount uint64
	idx        int
	cur        storage.SeriesRef
}

func newShardHashFilterPostings(refs []storage.SeriesRef, hashes []uint64, shardIndex, shardCount uint64) *shardHashFilterPostings {
	return &shardHashFilterPostings{refs: refs, hashes: hashes, shardIndex: shardIndex, shardCount: shardCount, idx: -1}
}

func (it *shardHashFilterPostings) Next() bool {
	for it.idx++; it.idx < len(it.refs); it.idx++ {
		if it.hashes[it.idx]%it.shardCount == it.shardIndex {
			it.cur = it.refs[it.idx]
			return true
		}
	}
	it.cur = 0
	return false
}

func (it *shardHashFilterPostings) Seek(v storage.SeriesRef) bool {
	// cur == 0 means no successful Next yet (idx < 0) or exhausted: 0 is never a
	// valid series ref.
	if it.cur != 0 && it.cur >= v {
		return true
	}
	// Binary search the sorted refs from the next position for the first ref
	// >= v, then skip-filter forward from there.
	lo := it.idx + 1
	lo = max(lo, 0)
	if lo < len(it.refs) {
		off, _ := slices.BinarySearch(it.refs[lo:], v)
		it.idx = lo + off - 1
	} else {
		it.idx = len(it.refs) - 1
	}
	return it.Next()
}

func (it *shardHashFilterPostings) At() storage.SeriesRef {
	return it.cur
}

func (*shardHashFilterPostings) Err() error {
	return nil
}

// shardFilterPostings filters the input postings to the refs present in the
// shard's bucket postings. Its Next path advances the input strictly
// sequentially because it may be an arbitrarily complex postings tree whose
// Seek is expensive; the bucket side consists of flat sorted lists whose forward
// Seek is cheap. Explicit Seek uses both sides to skip ahead.
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

// Seek uses both input and bucket seeking because callers that explicitly seek
// are asking to skip ahead. The hot query path drains with Next, which still
// avoids seeking the potentially expensive input postings tree.
func (f *shardFilterPostings) Seek(v storage.SeriesRef) bool {
	// cur == 0 means no successful Next yet: 0 is never a valid series ref.
	if f.cur != 0 && f.cur >= v {
		return true
	}
	target := v
	if target == 0 {
		target = 1
	}
	for {
		if !f.input.Seek(target) {
			return false
		}
		ref := f.input.At()
		if !f.buckets.Seek(ref) {
			return false
		}
		bucketRef := f.buckets.At()
		if bucketRef == ref {
			f.cur = ref
			return true
		}
		target = bucketRef
	}
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
