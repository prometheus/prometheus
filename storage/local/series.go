// Copyright 2014 Prometheus Team
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

package local

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// chunkDescEvictionFactor is a factor used for chunkDesc eviction (as opposed
// to evictions of chunks, see method evictOlderThan. A chunk takes about 20x
// more memory than a chunkDesc. With a chunkDescEvictionFactor of 10, not more
// than a third of the total memory taken by a series will be used for
// chunkDescs.
const chunkDescEvictionFactor = 10

// fingerprintSeriesPair pairs a fingerprint with a memorySeries pointer.
type fingerprintSeriesPair struct {
	fp     clientmodel.Fingerprint
	series *memorySeries
}

// seriesMap maps fingerprints to memory series. All its methods are
// goroutine-safe. A SeriesMap is effectively is a goroutine-safe version of
// map[clientmodel.Fingerprint]*memorySeries.
type seriesMap struct {
	mtx sync.RWMutex
	m   map[clientmodel.Fingerprint]*memorySeries
}

// newSeriesMap returns a newly allocated empty seriesMap. To create a seriesMap
// based on a prefilled map, use an explicit initializer.
func newSeriesMap() *seriesMap {
	return &seriesMap{m: make(map[clientmodel.Fingerprint]*memorySeries)}
}

// length returns the number of mappings in the seriesMap.
func (sm *seriesMap) length() int {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	return len(sm.m)
}

// get returns a memorySeries for a fingerprint. Return values have the same
// semantics as the native Go map.
func (sm *seriesMap) get(fp clientmodel.Fingerprint) (s *memorySeries, ok bool) {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	s, ok = sm.m[fp]
	return
}

// put adds a mapping to the seriesMap. It panics if s == nil.
func (sm *seriesMap) put(fp clientmodel.Fingerprint, s *memorySeries) {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	if s == nil {
		panic("tried to add nil pointer to seriesMap")
	}
	sm.m[fp] = s
}

// del removes a mapping from the series Map.
func (sm *seriesMap) del(fp clientmodel.Fingerprint) {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	delete(sm.m, fp)
}

// iter returns a channel that produces all mappings in the seriesMap. The
// channel will be closed once all fingerprints have been received. Not
// consuming all fingerprints from the channel will leak a goroutine. The
// semantics of concurrent modification of seriesMap is the similar as the one
// for iterating over a map with a 'range' clause. However, if the next element
// in iteration order is removed after the current element has been received
// from the channel, it will still be produced by the channel.
func (sm *seriesMap) iter() <-chan fingerprintSeriesPair {
	ch := make(chan fingerprintSeriesPair)
	go func() {
		sm.mtx.RLock()
		for fp, s := range sm.m {
			sm.mtx.RUnlock()
			ch <- fingerprintSeriesPair{fp, s}
			sm.mtx.RLock()
		}
		sm.mtx.RUnlock()
		close(ch)
	}()
	return ch
}

// fpIter returns a channel that produces all fingerprints in the seriesMap. The
// channel will be closed once all fingerprints have been received. Not
// consuming all fingerprints from the channel will leak a goroutine. The
// semantics of concurrent modification of seriesMap is the similar as the one
// for iterating over a map with a 'range' clause. However, if the next element
// in iteration order is removed after the current element has been received
// from the channel, it will still be produced by the channel.
func (sm *seriesMap) fpIter() <-chan clientmodel.Fingerprint {
	ch := make(chan clientmodel.Fingerprint)
	go func() {
		sm.mtx.RLock()
		for fp := range sm.m {
			sm.mtx.RUnlock()
			ch <- fp
			sm.mtx.RLock()
		}
		sm.mtx.RUnlock()
		close(ch)
	}()
	return ch
}

type memorySeries struct {
	metric clientmodel.Metric
	// Sorted by start time, overlapping chunk ranges are forbidden.
	chunkDescs []*chunkDesc
	// The chunkDescs in memory might not have all the chunkDescs for the
	// chunks that are persisted to disk. The missing chunkDescs are all
	// contiguous and at the tail end. chunkDescsOffset is the index of the
	// chunk on disk that corresponds to the first chunkDesc in memory. If
	// it is 0, the chunkDescs are all loaded. A value of -1 denotes a
	// special case: There are chunks on disk, but the offset to the
	// chunkDescs in memory is unknown. Also, there is no overlap between
	// chunks on disk and chunks in memory (implying that upon first
	// persiting of a chunk in memory, the offset has to be set).
	chunkDescsOffset int
	// Whether the current head chunk has already been scheduled to be
	// persisted. If true, the current head chunk must not be modified
	// anymore.
	headChunkPersisted bool
	// Whether the current head chunk is used by an iterator. In that case,
	// a non-persisted head chunk has to be cloned before more samples are
	// appended.
	headChunkUsedByIterator bool
}

// newMemorySeries returns a pointer to a newly allocated memorySeries for the
// given metric. reallyNew defines if the memorySeries is a genuinely new series
// or (if false) a series for a metric being unarchived, i.e. a series that
// existed before but has been evicted from memory.
func newMemorySeries(m clientmodel.Metric, reallyNew bool) *memorySeries {
	s := memorySeries{
		metric:             m,
		headChunkPersisted: !reallyNew,
	}
	if !reallyNew {
		s.chunkDescsOffset = -1
	}
	return &s
}

// add adds a sample pair to the series.
// It returns chunkDescs that must be queued to be persisted.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) add(fp clientmodel.Fingerprint, v *metric.SamplePair) []*chunkDesc {
	if len(s.chunkDescs) == 0 || s.headChunkPersisted {
		newHead := newChunkDesc(newDeltaEncodedChunk(d1, d0, true))
		s.chunkDescs = append(s.chunkDescs, newHead)
		s.headChunkPersisted = false
	} else if s.headChunkUsedByIterator && s.head().getRefCount() > 1 {
		// We only need to clone the head chunk if the current head
		// chunk was used in an iterator at all and if the refCount is
		// still greater than the 1 we always have because the head
		// chunk is not yet persisted. The latter is just an
		// approximation. We will still clone unnecessarily if an older
		// iterator using a previous version of the head chunk is still
		// around and keep the head chunk pinned. We needed to track
		// pins by version of the head chunk, which is probably not
		// worth the effort.
		chunkOps.WithLabelValues(clone).Inc()
		// No locking needed here because a non-persisted head chunk can
		// not get evicted concurrently.
		s.head().chunk = s.head().chunk.clone()
		s.headChunkUsedByIterator = false
	}

	chunks := s.head().add(v)
	s.head().chunk = chunks[0]

	var chunkDescsToPersist []*chunkDesc
	if len(chunks) > 1 {
		chunkDescsToPersist = append(chunkDescsToPersist, s.head())
		for i, c := range chunks[1:] {
			cd := newChunkDesc(c)
			s.chunkDescs = append(s.chunkDescs, cd)
			// The last chunk is still growing.
			if i < len(chunks[1:])-1 {
				chunkDescsToPersist = append(chunkDescsToPersist, cd)
			}
		}
	}
	return chunkDescsToPersist
}

// evictOlderThan marks for eviction all chunks whose latest sample is older
// than the given timestamp. It returns allEvicted as true if all chunks in the
// series were immediately evicted (i.e. all chunks are older than the
// timestamp, and none of the chunks was pinned).
//
// The method also evicts chunkDescs if there are chunkDescEvictionFactor times
// more chunkDescs in the series than unevicted chunks. (The number of unevicted
// chunks is considered the number of chunks between (and including) the oldest
// chunk that could not be evicted immediately and the newest chunk in the
// series, even if chunks in between were evicted.)
//
// Special considerations for the head chunk: If it has not been scheduled to be
// persisted yet but is old enough for eviction, this method returns a pointer
// to the descriptor of the head chunk to be persisted. (Otherwise, the method
// returns nil.) The caller is then responsible for persisting the head
// chunk. The internal state of this memorySeries is already set accordingly by
// this method. Calling evictOlderThan for a series with a non-persisted head
// chunk that is old enough for eviction will never evict all chunks
// immediately, even if no chunk is pinned for other reasons, because the head
// chunk is not persisted yet. A series old enough for archiving will require at
// least two eviction runs to become ready for archiving: In the first run, its
// head chunk is requested to be persisted. The next call of evictOlderThan will
// then return true, provided that the series hasn't received new samples in the
// meantime, the head chunk has now been persisted, and no chunk is pinned for
// other reasons.
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) evictOlderThan(t clientmodel.Timestamp) (allEvicted bool, headChunkToPersist *chunkDesc) {
	allEvicted = true
	iOldestNotEvicted := -1

	defer func() {
		// Evict chunkDescs if there are chunkDescEvictionFactor times
		// more than non-evicted chunks and we are not going to archive
		// the whole series anyway.
		if iOldestNotEvicted != -1 {
			lenToKeep := chunkDescEvictionFactor * (len(s.chunkDescs) - iOldestNotEvicted)
			if lenToKeep < len(s.chunkDescs) {
				lenEvicted := len(s.chunkDescs) - lenToKeep
				s.chunkDescsOffset += lenEvicted
				chunkDescOps.WithLabelValues(evict).Add(float64(lenEvicted))
				atomic.AddInt64(&numMemChunkDescs, -int64(lenEvicted))
				s.chunkDescs = append(
					make([]*chunkDesc, 0, lenToKeep),
					s.chunkDescs[lenEvicted:]...,
				)
			}
		}
	}()

	// For now, always drop the entire range from oldest to t.
	for i, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			if iOldestNotEvicted == -1 {
				iOldestNotEvicted = i
			}
			return false, nil
		}
		if cd.isEvicted() {
			continue
		}
		if !s.headChunkPersisted && i == len(s.chunkDescs)-1 {
			// This is a non-persisted head chunk that is old enough
			// for eviction. Request it to be persisted:
			headChunkToPersist = cd
			s.headChunkPersisted = true
			// Since we cannot modify the head chunk from now on, we
			// don't need to bother with cloning anymore.
			s.headChunkUsedByIterator = false
		}
		if !cd.evictOnUnpin() {
			if iOldestNotEvicted == -1 {
				iOldestNotEvicted = i
			}
			allEvicted = false
		}
	}
	return allEvicted, headChunkToPersist
}

// purgeOlderThan removes chunkDescs older than t. It also evicts the chunks of
// those chunkDescs (although that's probably not even necessary). It returns
// the number of purged chunkDescs and true if all chunkDescs have been purged.
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) purgeOlderThan(t clientmodel.Timestamp) (int, bool) {
	keepIdx := len(s.chunkDescs)
	for i, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			keepIdx = i
			break
		}
		s.chunkDescs[i].evictOnUnpin()
	}
	if keepIdx > 0 {
		s.chunkDescs = append(make([]*chunkDesc, 0, len(s.chunkDescs)-keepIdx), s.chunkDescs[keepIdx:]...)
		atomic.AddInt64(&numMemChunkDescs, -int64(keepIdx))
	}
	return keepIdx, len(s.chunkDescs) == 0
}

// preloadChunks is an internal helper method.
func (s *memorySeries) preloadChunks(indexes []int, p *persistence) ([]*chunkDesc, error) {
	loadIndexes := []int{}
	pinnedChunkDescs := make([]*chunkDesc, 0, len(indexes))
	for _, idx := range indexes {
		cd := s.chunkDescs[idx]
		pinnedChunkDescs = append(pinnedChunkDescs, cd)
		cd.pin() // Have to pin everything first to prevent concurrent evictOnUnpin later.
		if cd.isEvicted() {
			loadIndexes = append(loadIndexes, idx)
		}
	}
	chunkOps.WithLabelValues(pin).Add(float64(len(pinnedChunkDescs)))

	if len(loadIndexes) > 0 {
		if s.chunkDescsOffset == -1 {
			panic("requested loading chunks from persistence in a situation where we must not have persisted data for chunk descriptors in memory")
		}
		fp := s.metric.Fingerprint()
		chunks, err := p.loadChunks(fp, loadIndexes, s.chunkDescsOffset)
		if err != nil {
			// Unpin the chunks since we won't return them as pinned chunks now.
			for _, cd := range pinnedChunkDescs {
				cd.unpin()
			}
			chunkOps.WithLabelValues(unpin).Add(float64(len(pinnedChunkDescs)))
			return nil, err
		}
		for i, c := range chunks {
			s.chunkDescs[loadIndexes[i]].setChunk(c)
		}
		chunkOps.WithLabelValues(load).Add(float64(len(chunks)))
		atomic.AddInt64(&numMemChunks, int64(len(chunks)))
	}

	return pinnedChunkDescs, nil
}

/*
func (s *memorySeries) preloadChunksAtTime(t clientmodel.Timestamp, p *persistence) (chunkDescs, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if len(s.chunkDescs) == 0 {
		return nil, nil
	}

	var pinIndexes []int
	// Find first chunk where lastTime() is after or equal to t.
	i := sort.Search(len(s.chunkDescs), func(i int) bool {
		return !s.chunkDescs[i].lastTime().Before(t)
	})
	switch i {
	case 0:
		pinIndexes = []int{0}
	case len(s.chunkDescs):
		pinIndexes = []int{i - 1}
	default:
		if s.chunkDescs[i].contains(t) {
			pinIndexes = []int{i}
		} else {
			pinIndexes = []int{i - 1, i}
		}
	}

	return s.preloadChunks(pinIndexes, p)
}
*/

// preloadChunksForRange loads chunks for the given range from the persistence.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) preloadChunksForRange(
	from clientmodel.Timestamp, through clientmodel.Timestamp,
	fp clientmodel.Fingerprint, p *persistence,
) ([]*chunkDesc, error) {
	firstChunkDescTime := clientmodel.Timestamp(math.MaxInt64)
	if len(s.chunkDescs) > 0 {
		firstChunkDescTime = s.chunkDescs[0].firstTime()
	}
	if s.chunkDescsOffset != 0 && from.Before(firstChunkDescTime) {
		cds, err := p.loadChunkDescs(fp, firstChunkDescTime)
		if err != nil {
			return nil, err
		}
		s.chunkDescs = append(cds, s.chunkDescs...)
		s.chunkDescsOffset = 0
	}

	if len(s.chunkDescs) == 0 {
		return nil, nil
	}

	// Find first chunk with start time after "from".
	fromIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(from)
	})
	// Find first chunk with start time after "through".
	throughIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(through)
	})
	if fromIdx > 0 {
		fromIdx--
	}
	if throughIdx == len(s.chunkDescs) {
		throughIdx--
	}

	pinIndexes := make([]int, 0, throughIdx-fromIdx+1)
	for i := fromIdx; i <= throughIdx; i++ {
		pinIndexes = append(pinIndexes, i)
	}
	return s.preloadChunks(pinIndexes, p)
}

// newIterator returns a new SeriesIterator. The caller must have locked the
// fingerprint of the memorySeries.
func (s *memorySeries) newIterator(lockFunc, unlockFunc func()) SeriesIterator {
	chunks := make([]chunk, 0, len(s.chunkDescs))
	for i, cd := range s.chunkDescs {
		if !cd.isEvicted() {
			if i == len(s.chunkDescs)-1 && !s.headChunkPersisted {
				s.headChunkUsedByIterator = true
			}
			chunks = append(chunks, cd.chunk)
		}
	}

	return &memorySeriesIterator{
		lock:   lockFunc,
		unlock: unlockFunc,
		chunks: chunks,
	}
}

// head returns a pointer to the head chunk descriptor. The caller must have
// locked the fingerprint of the memorySeries.
func (s *memorySeries) head() *chunkDesc {
	return s.chunkDescs[len(s.chunkDescs)-1]
}

// values returns all values in the series. The caller must have locked the
// fingerprint of the memorySeries.
func (s *memorySeries) values() metric.Values {
	var values metric.Values
	for _, cd := range s.chunkDescs {
		for sample := range cd.chunk.values() {
			values = append(values, *sample)
		}
	}
	return values
}

// firstTime returns the timestamp of the first sample in the series. The caller
// must have locked the fingerprint of the memorySeries.
func (s *memorySeries) firstTime() clientmodel.Timestamp {
	return s.chunkDescs[0].firstTime()
}

// lastTime returns the timestamp of the last sample in the series. The caller
// must have locked the fingerprint of the memorySeries.
func (s *memorySeries) lastTime() clientmodel.Timestamp {
	return s.head().lastTime()
}

// memorySeriesIterator implements SeriesIterator.
type memorySeriesIterator struct {
	lock, unlock func()
	chunkIt      chunkIterator
	chunks       []chunk
}

// GetValueAtTime implements SeriesIterator.
func (it *memorySeriesIterator) GetValueAtTime(t clientmodel.Timestamp) metric.Values {
	it.lock()
	defer it.unlock()

	// The most common case. We are iterating through a chunk.
	if it.chunkIt != nil && it.chunkIt.contains(t) {
		return it.chunkIt.getValueAtTime(t)
	}

	it.chunkIt = nil

	if len(it.chunks) == 0 {
		return nil
	}

	// Before or exactly on the first sample of the series.
	if !t.After(it.chunks[0].firstTime()) {
		// return first value of first chunk
		return it.chunks[0].newIterator().getValueAtTime(t)
	}
	// After or exactly on the last sample of the series.
	if !t.Before(it.chunks[len(it.chunks)-1].lastTime()) {
		// return last value of last chunk
		return it.chunks[len(it.chunks)-1].newIterator().getValueAtTime(t)
	}

	// Find first chunk where lastTime() is after or equal to t.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].lastTime().Before(t)
	})
	if i == len(it.chunks) {
		panic("out of bounds")
	}

	if t.Before(it.chunks[i].firstTime()) {
		// We ended up between two chunks.
		return metric.Values{
			it.chunks[i-1].newIterator().getValueAtTime(t)[0],
			it.chunks[i].newIterator().getValueAtTime(t)[0],
		}
	}
	// We ended up in the middle of a chunk. We might stay there for a while,
	// so save it as the current chunk iterator.
	it.chunkIt = it.chunks[i].newIterator()
	return it.chunkIt.getValueAtTime(t)
}

// GetBoundaryValues implements SeriesIterator.
func (it *memorySeriesIterator) GetBoundaryValues(in metric.Interval) metric.Values {
	it.lock()
	defer it.unlock()

	// Find the first relevant chunk.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].lastTime().Before(in.OldestInclusive)
	})
	values := make(metric.Values, 0, 2)
	for i, c := range it.chunks[i:] {
		var chunkIt chunkIterator
		if c.firstTime().After(in.NewestInclusive) {
			if len(values) == 1 {
				// We found the first value before, but are now
				// already past the last value. The value we
				// want must be the last value of the previous
				// chunk. So backtrack...
				chunkIt = it.chunks[i-1].newIterator()
				values = append(values, chunkIt.getValueAtTime(in.NewestInclusive)[0])
			}
			break
		}
		if len(values) == 0 {
			chunkIt = c.newIterator()
			firstValues := chunkIt.getValueAtTime(in.OldestInclusive)
			switch len(firstValues) {
			case 2:
				values = append(values, firstValues[1])
			case 1:
				values = firstValues
			default:
				panic("unexpected return from getValueAtTime")
			}
		}
		if c.lastTime().After(in.NewestInclusive) {
			if chunkIt == nil {
				chunkIt = c.newIterator()
			}
			values = append(values, chunkIt.getValueAtTime(in.NewestInclusive)[0])
			break
		}
	}
	if len(values) == 1 {
		// We found exactly one value. In that case, add the most recent we know.
		values = append(
			values,
			it.chunks[len(it.chunks)-1].newIterator().getValueAtTime(in.NewestInclusive)[0],
		)
	}
	if len(values) == 2 && values[0].Equal(&values[1]) {
		return values[:1]
	}
	return values
}

// GetRangeValues implements SeriesIterator.
func (it *memorySeriesIterator) GetRangeValues(in metric.Interval) metric.Values {
	it.lock()
	defer it.unlock()

	// Find the first relevant chunk.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].lastTime().Before(in.OldestInclusive)
	})
	values := metric.Values{}
	for _, c := range it.chunks[i:] {
		if c.firstTime().After(in.NewestInclusive) {
			break
		}
		// TODO: actually reuse an iterator between calls if we get multiple ranges
		// from the same chunk.
		values = append(values, c.newIterator().getRangeValues(in)...)
	}
	return values
}

// nopSeriesIterator implements Series Iterator. It never returns any values.
type nopSeriesIterator struct{}

// GetValueAtTime implements SeriesIterator.
func (_ nopSeriesIterator) GetValueAtTime(t clientmodel.Timestamp) metric.Values {
	return metric.Values{}
}

// GetBoundaryValues implements SeriesIterator.
func (_ nopSeriesIterator) GetBoundaryValues(in metric.Interval) metric.Values {
	return metric.Values{}
}

// GetRangeValues implements SeriesIterator.
func (_ nopSeriesIterator) GetRangeValues(in metric.Interval) metric.Values {
	return metric.Values{}
}
