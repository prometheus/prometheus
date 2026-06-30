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
	"sort"
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
// after deleted series have been removed from the postings index. Deletions
// carry each removed ref's shard hash, so only affected buckets are rebuilt.
// Together this guarantees the invariant readers depend on: every ref readable
// from the postings index is present in its bucket list. Refs of deleted series
// may linger until the targeted removal and are resolved by readers like any
// other stale postings entry.
//
// Refs increase monotonically, so adds normally append in sorted position;
// out-of-order adds (concurrent creations racing, snapshot replay) mark that
// bucket dirty and its refs and hashes are re-sorted together into fresh slices
// the next time it is read. The dirty index tracks the earliest dirty suffix;
// mostly sorted buckets sort that suffix and merge it with the sorted prefix.
// Candidate bucket snapshots are captured while all candidate buckets are
// locked. Refs and hashes within a captured slice length are never mutated in
// place after capture, so returned postings remain valid after the locks are
// released; appends may still reuse backing capacity beyond the captured length.
type shardBucketPostings struct {
	buckets []shardBucket
}

// Bucket state holds one bucket's refs and aligned shard hashes. It contains a
// mutex and must not be copied after initialization.
type shardBucket struct {
	mtx    sync.RWMutex
	refs   []storage.SeriesRef
	hashes []uint64 // hashes[i] is the shard hash of refs[i]; kept aligned and co-sorted by ref.
	dirty  int      // Earliest out-of-order suffix index, or cleanShardBucket when clean.
}

const cleanShardBucket = -1

func newShardBucketPostings(buckets int) *shardBucketPostings {
	shardBuckets := make([]shardBucket, buckets)
	for i := range shardBuckets {
		shardBuckets[i].dirty = cleanShardBucket
	}
	return &shardBucketPostings{
		buckets: shardBuckets,
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
	bucket := &s.buckets[b]
	bucket.mtx.Lock()
	list := bucket.refs
	if n := len(list); n > 0 && list[n-1] >= storage.SeriesRef(ref) {
		if bucket.dirty == cleanShardBucket {
			bucket.dirty = n
		}
	}
	bucket.refs = append(list, storage.SeriesRef(ref))
	bucket.hashes = append(bucket.hashes, shardHash)
	bucket.mtx.Unlock()
}

// remove drops deleted series refs from the bucket lists. The map is keyed by
// deleted ref with the ref's shard hash as value, allowing removal to rebuild
// only affected buckets. Rebuilt buckets replace refs and hashes in lockstep, so
// reader snapshots stay intact. A nil receiver or empty deleted map is a no-op.
func (s *shardBucketPostings) remove(deleted map[storage.SeriesRef]uint64) {
	if s == nil || len(s.buckets) == 0 || len(deleted) == 0 {
		return
	}

	deletedByBucket := make(map[uint64]map[storage.SeriesRef]struct{}, min(len(deleted), len(s.buckets)))
	for ref, shardHash := range deleted {
		// len(s.buckets) is validated to be a power of two, so this is
		// shardHash % len(s.buckets) without runtime division.
		b := shardHash & uint64(len(s.buckets)-1)
		refs := deletedByBucket[b]
		if refs == nil {
			refs = map[storage.SeriesRef]struct{}{}
			deletedByBucket[b] = refs
		}
		refs[ref] = struct{}{}
	}

	for b, deletedRefs := range deletedByBucket {
		bucket := &s.buckets[b]
		bucket.mtx.Lock()
		list := bucket.refs
		first := -1
		for i, ref := range list {
			if _, ok := deletedRefs[ref]; ok {
				first = i
				break
			}
		}
		if first >= 0 {
			hashes := bucket.hashes
			// Size the rebuilt slices to the surviving count, not the pre-removal
			// length: keeping cap at len(list)-1 leaves the dropped refs' capacity
			// resident until the next rebuild, which under series churn keeps the
			// index several times larger than the live series it holds.
			survivors := first
			for i := first + 1; i < len(list); i++ {
				if _, ok := deletedRefs[list[i]]; !ok {
					survivors++
				}
			}
			survivingRefs := make([]storage.SeriesRef, first, survivors)
			survivingHashes := make([]uint64, first, survivors)
			copy(survivingRefs, list[:first])
			copy(survivingHashes, hashes[:first])
			for i := first + 1; i < len(list); i++ {
				if _, ok := deletedRefs[list[i]]; !ok {
					survivingRefs = append(survivingRefs, list[i])
					survivingHashes = append(survivingHashes, hashes[i])
				}
			}
			bucket.refs = survivingRefs
			bucket.hashes = survivingHashes
			if bucket.dirty != cleanShardBucket {
				if len(survivingRefs) == 0 {
					bucket.dirty = cleanShardBucket
				} else {
					bucket.dirty = 0
				}
			}
		}
		bucket.mtx.Unlock()
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
// Candidate bucket snapshots are captured while all candidate buckets are locked
// so multi-bucket results are internally consistent.
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

	writeLocked := s.lockCandidateBuckets(base, step)
	lists = make([]index.Postings, 0, bucketCount/step)
	for b := base; b < bucketCount; b += step {
		bucket := &s.buckets[b]
		lists = append(lists, postingsForBucketLocked(bucket, subFiltered, shardIndex, shardCount))
	}
	s.unlockCandidateBuckets(base, step, writeLocked)
	return lists, subFiltered
}

func (s *shardBucketPostings) lockCandidateBuckets(base, step uint64) []uint64 {
	var writeLocked []uint64
	for b := base; b < uint64(len(s.buckets)); b += step {
		bucket := &s.buckets[b]
		bucket.mtx.RLock()
		if bucket.dirty == cleanShardBucket {
			continue
		}
		bucket.mtx.RUnlock()
		bucket.mtx.Lock()
		sortDirtyBucketLocked(bucket)
		writeLocked = append(writeLocked, b)
	}
	return writeLocked
}

func (s *shardBucketPostings) unlockCandidateBuckets(base, step uint64, writeLocked []uint64) {
	if len(writeLocked) == 0 {
		for b := base; b < uint64(len(s.buckets)); b += step {
			s.buckets[b].mtx.RUnlock()
		}
		return
	}
	nextWriteLocked := 0
	for b := base; b < uint64(len(s.buckets)); b += step {
		if nextWriteLocked < len(writeLocked) && writeLocked[nextWriteLocked] == b {
			s.buckets[b].mtx.Unlock()
			nextWriteLocked++
			continue
		}
		s.buckets[b].mtx.RUnlock()
	}
}

func postingsForBucketLocked(bucket *shardBucket, subFiltered bool, shardIndex, shardCount uint64) index.Postings {
	if subFiltered {
		return newShardHashFilterPostings(bucket.refs, bucket.hashes, shardIndex, shardCount)
	}
	return index.NewListPostings(bucket.refs)
}

// numSeries returns the total number of refs held, including refs of deleted
// series that have not been removed yet. A nil receiver returns 0.
func (s *shardBucketPostings) numSeries() int {
	if s == nil {
		return 0
	}
	n := 0
	for b := range s.buckets {
		bucket := &s.buckets[b]
		bucket.mtx.RLock()
		n += len(bucket.refs)
		bucket.mtx.RUnlock()
	}
	return n
}

// sortDirtyBucketLocked replaces a dirty bucket with sorted copies, permuting
// its hashes by the same order so refs and hashes stay aligned. The caller must
// hold bucket.mtx for writing.
func sortDirtyBucketLocked(bucket *shardBucket) {
	dirtyStart := bucket.dirty
	if dirtyStart == cleanShardBucket {
		return
	}
	refs := bucket.refs
	hashes := bucket.hashes
	var sortedRefs []storage.SeriesRef
	var sortedHashes []uint64
	if dirtyStart < len(refs)/2 {
		sortedRefs, sortedHashes = sortRefHashesFull(refs, hashes)
	} else {
		sortedRefs, sortedHashes = sortRefHashesSuffix(refs, hashes, dirtyStart)
	}
	bucket.refs = sortedRefs
	bucket.hashes = sortedHashes
	bucket.dirty = cleanShardBucket
}

type refsAndHashes struct {
	refs   []storage.SeriesRef
	hashes []uint64
}

func (r *refsAndHashes) Len() int {
	return len(r.refs)
}

func (r *refsAndHashes) Less(i, j int) bool {
	return r.refs[i] < r.refs[j]
}

func (r *refsAndHashes) Swap(i, j int) {
	r.refs[i], r.refs[j] = r.refs[j], r.refs[i]
	r.hashes[i], r.hashes[j] = r.hashes[j], r.hashes[i]
}

func sortRefHashesFull(refs []storage.SeriesRef, hashes []uint64) ([]storage.SeriesRef, []uint64) {
	sortedRefs := slices.Clone(refs)
	sortedHashes := slices.Clone(hashes)
	sort.Sort(&refsAndHashes{refs: sortedRefs, hashes: sortedHashes})
	return sortedRefs, sortedHashes
}

func sortRefHashesSuffix(refs []storage.SeriesRef, hashes []uint64, dirtyStart int) ([]storage.SeriesRef, []uint64) {
	sortedRefs := slices.Clone(refs)
	sortedHashes := slices.Clone(hashes)
	if dirtyStart <= 0 {
		sort.Sort(&refsAndHashes{refs: sortedRefs, hashes: sortedHashes})
		return sortedRefs, sortedHashes
	}
	if dirtyStart >= len(sortedRefs) {
		return sortedRefs, sortedHashes
	}

	minDirtyRef := slices.Min(sortedRefs[dirtyStart:])
	suffixStart, _ := slices.BinarySearch(sortedRefs[:dirtyStart], minDirtyRef)
	if suffixStart >= len(sortedRefs)-1 {
		return sortedRefs, sortedHashes
	}
	sort.Sort(&refsAndHashes{refs: sortedRefs[suffixStart:], hashes: sortedHashes[suffixStart:]})
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
