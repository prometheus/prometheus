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

// DefaultShardedPostingsBuckets is the default shard hash bucket count, chosen
// so common power-of-two shard counts use exact bucket routing.
const DefaultShardedPostingsBuckets = 128

func isPowerOfTwo(v uint64) bool {
	return v != 0 && v&(v-1) == 0
}

// shardBucketPostings indexes Head series refs by shard hash bucket and returns
// sorted bucket postings, allowing ShardedPostings to intersect candidates
// without per-series lookups. Series are added before becoming visible in Head
// postings and removed afterward, so every visible ref is present; stale refs
// may remain until removal and are skipped by readers.
//
// Bucket slices are replaced rather than mutated within their existing length,
// so read snapshots remain valid after locks are released.
type shardBucketPostings struct {
	buckets []shardBucket

	// removeUnlockedHook, when non-nil, runs after a removal's unlocked
	// survivor rebuild and before the bucket is re-locked. Tests use it to
	// interleave appends and re-sorts into that window; it is nil in
	// production.
	removeUnlockedHook func()
}

// shardBucket holds one bucket's refs and must not be copied after initialization.
type shardBucket struct {
	mtx   sync.RWMutex
	refs  []storage.SeriesRef
	dirty int // Earliest out-of-order suffix index, or cleanShardBucket when clean.
	// gen counts replacements of refs (re-sorts and removals). Appends do not
	// change it: they only extend refs, and refs within a previously observed
	// length are never mutated in place. Removal uses it to detect whether a
	// snapshot's prefix is still current after rebuilding survivors unlocked.
	gen uint64
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
	if n := len(bucket.refs); n > 0 && bucket.refs[n-1] >= storage.SeriesRef(ref) {
		if bucket.dirty == cleanShardBucket {
			bucket.dirty = n
		}
	}
	bucket.refs = append(bucket.refs, storage.SeriesRef(ref))
	bucket.mtx.Unlock()
}

// removeTailHeadroom reserves capacity for refs appended during an unlocked
// removal rebuild, usually avoiding reallocation when they are carried over.
const removeTailHeadroom = 16

// remove drops deleted refs from their shard hash buckets. Survivors are rebuilt
// outside bucket locks and swapped in without invalidating reader snapshots. A
// nil receiver or empty deleted map is a no-op.
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
		s.removeFromBucket(&s.buckets[b], deletedRefs)
	}
}

// removeFromBucket rebuilds survivors from an unlocked snapshot. Later appends
// are carried over; a concurrent slice replacement triggers a locked rebuild.
func (s *shardBucketPostings) removeFromBucket(bucket *shardBucket, deletedRefs map[storage.SeriesRef]struct{}) {
	bucket.mtx.RLock()
	snap := bucket.refs
	snapGen := bucket.gen
	bucket.mtx.RUnlock()

	survivors, found := survivingRefs(snap, deletedRefs)
	if !found {
		// No deleted ref is in this bucket, and refs appended after the
		// snapshot cannot be deleted ones either: nothing to rebuild.
		return
	}

	if h := s.removeUnlockedHook; h != nil {
		h()
	}

	bucket.mtx.Lock()
	defer bucket.mtx.Unlock()

	if bucket.gen == snapGen {
		// The snapshot prefix is still current: carry over refs appended
		// during the unlocked rebuild and swap.
		bucket.refs = append(survivors, bucket.refs[len(snap):]...)
	} else {
		// The refs were replaced while unlocked, so the snapshot-derived
		// survivors are stale. Replacements are rare (concurrent re-sorts and
		// removals), so rebuilding under the lock keeps the previous behavior
		// as the worst case.
		replacement, found := survivingRefs(bucket.refs, deletedRefs)
		if !found {
			return
		}
		bucket.refs = replacement
	}
	bucket.gen++
	if bucket.dirty != cleanShardBucket {
		// The recorded dirty suffix index does not survive the rebuild.
		if len(bucket.refs) == 0 {
			bucket.dirty = cleanShardBucket
		} else {
			bucket.dirty = 0
		}
	}
}

// survivingRefs returns a fresh slice without deletedRefs and reports whether
// any ref was removed. The result reserves removeTailHeadroom for later appends.
func survivingRefs(refs []storage.SeriesRef, deletedRefs map[storage.SeriesRef]struct{}) ([]storage.SeriesRef, bool) {
	first := -1
	for i, ref := range refs {
		if _, ok := deletedRefs[ref]; ok {
			first = i
			break
		}
	}
	if first < 0 {
		return nil, false
	}
	surviving := first
	for i := first + 1; i < len(refs); i++ {
		if _, ok := deletedRefs[refs[i]]; !ok {
			surviving++
		}
	}
	out := make([]storage.SeriesRef, first, surviving+removeTailHeadroom)
	copy(out, refs[:first])
	for i := first + 1; i < len(refs); i++ {
		if _, ok := deletedRefs[refs[i]]; !ok {
			out = append(out, refs[i])
		}
	}
	return out, true
}

// postingsFor returns sorted candidate bucket postings for one shard. Bucket
// and shard counts must be powers of two. With B buckets and N shards, N <= B
// uses buckets shardIndex, shardIndex+N, ... without sub-filtering; N > B uses
// bucket shardIndex%B and requests shard hash sub-filtering.
//
// A nil receiver (sharding disabled) or a zero shard count returns no lists.
// Candidate buckets are snapshotted under lock as one consistent view.
func (s *shardBucketPostings) postingsFor(shardIndex, shardCount uint64) (lists []index.Postings, needsShardHashFilter bool) {
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
		needsShardHashFilter = true
	}

	writeLocked := s.lockCandidateBuckets(base, step)
	lists = make([]index.Postings, 0, bucketCount/step)
	for b := base; b < bucketCount; b += step {
		bucket := &s.buckets[b]
		lists = append(lists, index.NewListPostings(bucket.refs))
	}
	s.unlockCandidateBuckets(base, step, writeLocked)
	return lists, needsShardHashFilter
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

// sortDirtyBucketLocked replaces a dirty bucket with a sorted copy. The caller
// must hold bucket.mtx for writing.
func sortDirtyBucketLocked(bucket *shardBucket) {
	dirtyStart := bucket.dirty
	if dirtyStart == cleanShardBucket {
		return
	}
	refs := bucket.refs
	if dirtyStart < len(refs)/2 {
		bucket.refs = sortRefsFull(refs)
	} else {
		bucket.refs = sortRefsSuffix(refs, dirtyStart)
	}
	bucket.gen++
	bucket.dirty = cleanShardBucket
}

func sortRefsFull(refs []storage.SeriesRef) []storage.SeriesRef {
	sortedRefs := slices.Clone(refs)
	slices.Sort(sortedRefs)
	return sortedRefs
}

func sortRefsSuffix(refs []storage.SeriesRef, dirtyStart int) []storage.SeriesRef {
	sortedRefs := slices.Clone(refs)
	if dirtyStart <= 0 {
		slices.Sort(sortedRefs)
		return sortedRefs
	}
	if dirtyStart >= len(sortedRefs) {
		return sortedRefs
	}

	minDirtyRef := slices.Min(sortedRefs[dirtyStart:])
	suffixStart, _ := slices.BinarySearch(sortedRefs[:dirtyStart], minDirtyRef)
	if suffixStart >= len(sortedRefs)-1 {
		return sortedRefs
	}
	slices.Sort(sortedRefs[suffixStart:])
	return sortedRefs
}
