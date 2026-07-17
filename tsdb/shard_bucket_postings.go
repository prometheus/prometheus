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

	// removeUnlockedHook runs during an unlocked survivor rebuild in tests.
	removeUnlockedHook func()
	// sortUnlockedHook runs while preparing a dirty-bucket replacement in tests.
	sortUnlockedHook func()
	// sortTailCarryHook runs before carrying concurrent sort-tail appends in tests.
	sortTailCarryHook func()
}

// shardBucket holds one bucket's refs and must not be copied after initialization.
type shardBucket struct {
	mtx   sync.RWMutex
	refs  []storage.SeriesRef
	dirty int // Earliest out-of-order suffix index, or cleanShardBucket when clean.
	// replacementMtx serializes refs replacements. It is never acquired while
	// mtx is held.
	replacementMtx sync.Mutex
}

// cleanShardBucket is distinct from dirty index 0, which requires a full sort.
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

// replacementTailHeadroom reserves capacity for refs appended while a bucket
// replacement is prepared.
const replacementTailHeadroom = 16

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

// removeFromBucket rebuilds survivors outside the main lock and carries over
// later appends. replacementMtx keeps the snapshot prefix current until swap.
func (s *shardBucketPostings) removeFromBucket(bucket *shardBucket, deletedRefs map[storage.SeriesRef]struct{}) {
	bucket.replacementMtx.Lock()
	defer bucket.replacementMtx.Unlock()

	bucket.mtx.RLock()
	snap := bucket.refs
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

	bucket.refs = append(survivors, bucket.refs[len(snap):]...)
	if bucket.dirty != cleanShardBucket {
		// Removal invalidates the dirty suffix boundary. Sort the rebuilt
		// bucket in full on its next read.
		if len(bucket.refs) == 0 {
			bucket.dirty = cleanShardBucket
		} else {
			bucket.dirty = 0
		}
	}
}

// survivingRefs returns a fresh slice without deletedRefs and reports whether
// any ref was removed. The result reserves headroom for later appends.
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
	out := make([]storage.SeriesRef, first, surviving+replacementTailHeadroom)
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
// Each bucket is captured independently; snapshots remain valid after unlock.
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

	lists = make([]index.Postings, 0, bucketCount/step)
	for b := base; b < bucketCount; b += step {
		refs := s.snapshotShardBucket(&s.buckets[b])
		lists = append(lists, index.NewListPostings(refs))
	}
	return lists, needsShardHashFilter
}

// snapshotShardBucket returns sorted refs that remain valid after unlock.
func (s *shardBucketPostings) snapshotShardBucket(bucket *shardBucket) []storage.SeriesRef {
	bucket.mtx.RLock()
	if bucket.dirty == cleanShardBucket {
		refs := bucket.refs
		bucket.mtx.RUnlock()
		return refs
	}
	bucket.mtx.RUnlock()

	bucket.replacementMtx.Lock()
	defer bucket.replacementMtx.Unlock()

	bucket.mtx.RLock()
	if bucket.dirty == cleanShardBucket {
		refs := bucket.refs
		bucket.mtx.RUnlock()
		return refs
	}
	refs := bucket.refs
	dirtyStart := bucket.dirty
	bucket.mtx.RUnlock()

	if h := s.sortUnlockedHook; h != nil {
		h()
	}
	snapshotRefs := sortDirtyRefs(refs, dirtyStart)
	replacementRefs := snapshotRefs
	replacementDirty := cleanShardBucket
	capturedLen := len(refs)
	tailCarryHook := s.sortTailCarryHook

	for {
		bucket.mtx.Lock()
		currentRefs := bucket.refs
		tail := currentRefs[capturedLen:]
		if len(tail) <= cap(replacementRefs)-len(replacementRefs) {
			// The capacity check keeps this append non-allocating under mtx.
			replacementRefs, replacementDirty = appendShardBucketTail(replacementRefs, tail, replacementDirty)
			bucket.refs = replacementRefs
			bucket.dirty = replacementDirty
			bucket.mtx.Unlock()
			return snapshotRefs
		}
		bucket.mtx.Unlock()

		if tailCarryHook != nil {
			tailCarryHook()
			tailCarryHook = nil
		}
		replacementRefs, replacementDirty = appendShardBucketTail(replacementRefs, tail, replacementDirty)
		capturedLen = len(currentRefs)
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

// sortDirtyRefs returns a sorted copy using the recorded dirty suffix.
func sortDirtyRefs(refs []storage.SeriesRef, dirtyStart int) []storage.SeriesRef {
	if dirtyStart < len(refs)/2 {
		return sortRefsFull(refs)
	}
	return sortRefsSuffix(refs, dirtyStart)
}

func sortRefsFull(refs []storage.SeriesRef) []storage.SeriesRef {
	sortedRefs := cloneRefsForReplacement(refs)
	slices.Sort(sortedRefs)
	return sortedRefs
}

func sortRefsSuffix(refs []storage.SeriesRef, dirtyStart int) []storage.SeriesRef {
	sortedRefs := cloneRefsForReplacement(refs)
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

func cloneRefsForReplacement(refs []storage.SeriesRef) []storage.SeriesRef {
	out := make([]storage.SeriesRef, len(refs), len(refs)+replacementTailHeadroom)
	copy(out, refs)
	return out
}

func appendShardBucketTail(refs, tail []storage.SeriesRef, dirty int) ([]storage.SeriesRef, int) {
	start := len(refs)
	refs = append(refs, tail...)
	if dirty != cleanShardBucket {
		return refs, dirty
	}
	for i := max(1, start); i < len(refs); i++ {
		if refs[i-1] >= refs[i] {
			return refs, i
		}
	}
	return refs, cleanShardBucket
}
