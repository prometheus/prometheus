// Copyright 2014 The Prometheus Authors
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
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	// chunkDescEvictionFactor is a factor used for chunkDesc eviction (as opposed
	// to evictions of chunks, see method evictOlderThan. A chunk takes about 20x
	// more memory than a chunkDesc. With a chunkDescEvictionFactor of 10, not more
	// than a third of the total memory taken by a series will be used for
	// chunkDescs.
	chunkDescEvictionFactor = 10

	headChunkTimeout = time.Hour // Close head chunk if not touched for that long.
)

// fingerprintSeriesPair pairs a fingerprint with a memorySeries pointer.
type fingerprintSeriesPair struct {
	fp     model.Fingerprint
	series *memorySeries
}

// seriesMap maps fingerprints to memory series. All its methods are
// goroutine-safe. A SeriesMap is effectively is a goroutine-safe version of
// map[model.Fingerprint]*memorySeries.
type seriesMap struct {
	mtx sync.RWMutex
	m   map[model.Fingerprint]*memorySeries
}

// newSeriesMap returns a newly allocated empty seriesMap. To create a seriesMap
// based on a prefilled map, use an explicit initializer.
func newSeriesMap() *seriesMap {
	return &seriesMap{m: make(map[model.Fingerprint]*memorySeries)}
}

// length returns the number of mappings in the seriesMap.
func (sm *seriesMap) length() int {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	return len(sm.m)
}

// get returns a memorySeries for a fingerprint. Return values have the same
// semantics as the native Go map.
func (sm *seriesMap) get(fp model.Fingerprint) (s *memorySeries, ok bool) {
	sm.mtx.RLock()
	defer sm.mtx.RUnlock()

	s, ok = sm.m[fp]
	return
}

// put adds a mapping to the seriesMap. It panics if s == nil.
func (sm *seriesMap) put(fp model.Fingerprint, s *memorySeries) {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

	if s == nil {
		panic("tried to add nil pointer to seriesMap")
	}
	sm.m[fp] = s
}

// del removes a mapping from the series Map.
func (sm *seriesMap) del(fp model.Fingerprint) {
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
func (sm *seriesMap) fpIter() <-chan model.Fingerprint {
	ch := make(chan model.Fingerprint)
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
	metric model.Metric
	// Sorted by start time, overlapping chunk ranges are forbidden.
	chunkDescs []*chunkDesc
	// The index (within chunkDescs above) of the first chunkDesc that
	// points to a non-persisted chunk. If all chunks are persisted, then
	// persistWatermark == len(chunkDescs).
	persistWatermark int
	// The modification time of the series file. The zero value of time.Time
	// is used to mark an unknown modification time.
	modTime time.Time
	// The chunkDescs in memory might not have all the chunkDescs for the
	// chunks that are persisted to disk. The missing chunkDescs are all
	// contiguous and at the tail end. chunkDescsOffset is the index of the
	// chunk on disk that corresponds to the first chunkDesc in memory. If
	// it is 0, the chunkDescs are all loaded. A value of -1 denotes a
	// special case: There are chunks on disk, but the offset to the
	// chunkDescs in memory is unknown. Also, in this special case, there is
	// no overlap between chunks on disk and chunks in memory (implying that
	// upon first persisting of a chunk in memory, the offset has to be
	// set).
	chunkDescsOffset int
	// The savedFirstTime field is used as a fallback when the
	// chunkDescsOffset is not 0. It can be used to save the firstTime of the
	// first chunk before its chunk desc is evicted. In doubt, this field is
	// just set to the oldest possible timestamp.
	savedFirstTime model.Time
	// The timestamp of the last sample in this series. Needed for fast access to
	// ensure timestamp monotonicity during ingestion.
	lastTime model.Time
	// Whether the current head chunk has already been finished.  If true,
	// the current head chunk must not be modified anymore.
	headChunkClosed bool
	// Whether the current head chunk is used by an iterator. In that case,
	// a non-closed head chunk has to be cloned before more samples are
	// appended.
	headChunkUsedByIterator bool
	// Whether the series is inconsistent with the last checkpoint in a way
	// that would require a disk seek during crash recovery.
	dirty bool
}

// newMemorySeries returns a pointer to a newly allocated memorySeries for the
// given metric. chunkDescs and modTime in the new series are set according to
// the provided parameters. chunkDescs can be nil or empty if this is a
// genuinely new time series (i.e. not one that is being unarchived). In that
// case, headChunkClosed is set to false, and firstTime and lastTime are both
// set to model.Earliest. The zero value for modTime can be used if the
// modification time of the series file is unknown (e.g. if this is a genuinely
// new series).
func newMemorySeries(m model.Metric, chunkDescs []*chunkDesc, modTime time.Time) *memorySeries {
	firstTime := model.Earliest
	lastTime := model.Earliest
	if len(chunkDescs) > 0 {
		firstTime = chunkDescs[0].firstTime()
		lastTime = chunkDescs[len(chunkDescs)-1].lastTime()
	}
	return &memorySeries{
		metric:           m,
		chunkDescs:       chunkDescs,
		headChunkClosed:  len(chunkDescs) > 0,
		savedFirstTime:   firstTime,
		lastTime:         lastTime,
		persistWatermark: len(chunkDescs),
		modTime:          modTime,
	}
}

// add adds a sample pair to the series. It returns the number of newly
// completed chunks (which are now eligible for persistence).
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) add(v *model.SamplePair) int {
	if len(s.chunkDescs) == 0 || s.headChunkClosed {
		newHead := newChunkDesc(newChunk())
		s.chunkDescs = append(s.chunkDescs, newHead)
		s.headChunkClosed = false
	} else if s.headChunkUsedByIterator && s.head().refCount() > 1 {
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
		s.head().c = s.head().c.clone()
		s.headChunkUsedByIterator = false
	}

	chunks := s.head().add(v)
	s.head().c = chunks[0]

	for _, c := range chunks[1:] {
		s.chunkDescs = append(s.chunkDescs, newChunkDesc(c))
	}

	s.lastTime = v.Timestamp
	return len(chunks) - 1
}

// maybeCloseHeadChunk closes the head chunk if it has not been touched for the
// duration of headChunkTimeout. It returns whether the head chunk was closed.
// If the head chunk is already closed, the method is a no-op and returns false.
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) maybeCloseHeadChunk() bool {
	if s.headChunkClosed {
		return false
	}
	if time.Now().Sub(s.lastTime.Time()) > headChunkTimeout {
		s.headChunkClosed = true
		// Since we cannot modify the head chunk from now on, we
		// don't need to bother with cloning anymore.
		s.headChunkUsedByIterator = false
		return true
	}
	return false
}

// evictChunkDescs evicts chunkDescs if there are chunkDescEvictionFactor times
// more than non-evicted chunks. iOldestNotEvicted is the index within the
// current chunkDescs of the oldest chunk that is not evicted.
func (s *memorySeries) evictChunkDescs(iOldestNotEvicted int) {
	lenToKeep := chunkDescEvictionFactor * (len(s.chunkDescs) - iOldestNotEvicted)
	if lenToKeep < len(s.chunkDescs) {
		s.savedFirstTime = s.firstTime()
		lenEvicted := len(s.chunkDescs) - lenToKeep
		s.chunkDescsOffset += lenEvicted
		s.persistWatermark -= lenEvicted
		chunkDescOps.WithLabelValues(evict).Add(float64(lenEvicted))
		numMemChunkDescs.Sub(float64(lenEvicted))
		s.chunkDescs = append(
			make([]*chunkDesc, 0, lenToKeep),
			s.chunkDescs[lenEvicted:]...,
		)
		s.dirty = true
	}
}

// dropChunks removes chunkDescs older than t. The caller must have locked the
// fingerprint of the series.
func (s *memorySeries) dropChunks(t model.Time) {
	keepIdx := len(s.chunkDescs)
	for i, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			keepIdx = i
			break
		}
	}
	if keepIdx > 0 {
		s.chunkDescs = append(
			make([]*chunkDesc, 0, len(s.chunkDescs)-keepIdx),
			s.chunkDescs[keepIdx:]...,
		)
		s.persistWatermark -= keepIdx
		if s.persistWatermark < 0 {
			panic("dropped unpersisted chunks from memory")
		}
		if s.chunkDescsOffset != -1 {
			s.chunkDescsOffset += keepIdx
		}
		numMemChunkDescs.Sub(float64(keepIdx))
		s.dirty = true
	}
}

// preloadChunks is an internal helper method.
func (s *memorySeries) preloadChunks(
	indexes []int, fp model.Fingerprint, mss *memorySeriesStorage,
) ([]*chunkDesc, error) {
	loadIndexes := []int{}
	pinnedChunkDescs := make([]*chunkDesc, 0, len(indexes))
	for _, idx := range indexes {
		cd := s.chunkDescs[idx]
		pinnedChunkDescs = append(pinnedChunkDescs, cd)
		cd.pin(mss.evictRequests) // Have to pin everything first to prevent immediate eviction on chunk loading.
		if cd.isEvicted() {
			loadIndexes = append(loadIndexes, idx)
		}
	}
	chunkOps.WithLabelValues(pin).Add(float64(len(pinnedChunkDescs)))

	if len(loadIndexes) > 0 {
		if s.chunkDescsOffset == -1 {
			panic("requested loading chunks from persistence in a situation where we must not have persisted data for chunk descriptors in memory")
		}
		chunks, err := mss.loadChunks(fp, loadIndexes, s.chunkDescsOffset)
		if err != nil {
			// Unpin the chunks since we won't return them as pinned chunks now.
			for _, cd := range pinnedChunkDescs {
				cd.unpin(mss.evictRequests)
			}
			chunkOps.WithLabelValues(unpin).Add(float64(len(pinnedChunkDescs)))
			return nil, err
		}
		for i, c := range chunks {
			s.chunkDescs[loadIndexes[i]].setChunk(c)
		}
	}
	return pinnedChunkDescs, nil
}

/*
func (s *memorySeries) preloadChunksAtTime(t model.Time, p *persistence) (chunkDescs, error) {
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
	from model.Time, through model.Time,
	fp model.Fingerprint, mss *memorySeriesStorage,
) ([]*chunkDesc, error) {
	firstChunkDescTime := model.Latest
	if len(s.chunkDescs) > 0 {
		firstChunkDescTime = s.chunkDescs[0].firstTime()
	}
	if s.chunkDescsOffset != 0 && from.Before(firstChunkDescTime) {
		cds, err := mss.loadChunkDescs(fp, s.persistWatermark)
		if err != nil {
			return nil, err
		}
		s.chunkDescs = append(cds, s.chunkDescs...)
		s.chunkDescsOffset = 0
		s.persistWatermark += len(cds)
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
	return s.preloadChunks(pinIndexes, fp, mss)
}

// newIterator returns a new SeriesIterator. The caller must have locked the
// fingerprint of the memorySeries.
func (s *memorySeries) newIterator() SeriesIterator {
	chunks := make([]chunk, 0, len(s.chunkDescs))
	for i, cd := range s.chunkDescs {
		if chunk := cd.chunk(); chunk != nil {
			if i == len(s.chunkDescs)-1 && !s.headChunkClosed {
				s.headChunkUsedByIterator = true
			}
			chunks = append(chunks, chunk)
		}
	}

	return &memorySeriesIterator{
		chunks:   chunks,
		chunkIts: make([]chunkIterator, len(chunks)),
	}
}

// head returns a pointer to the head chunk descriptor. The caller must have
// locked the fingerprint of the memorySeries. This method will panic if this
// series has no chunk descriptors.
func (s *memorySeries) head() *chunkDesc {
	return s.chunkDescs[len(s.chunkDescs)-1]
}

// firstTime returns the timestamp of the first sample in the series. The caller
// must have locked the fingerprint of the memorySeries.
func (s *memorySeries) firstTime() model.Time {
	if s.chunkDescsOffset == 0 && len(s.chunkDescs) > 0 {
		return s.chunkDescs[0].firstTime()
	}
	return s.savedFirstTime
}

// chunksToPersist returns a slice of chunkDescs eligible for persistence. It's
// the caller's responsibility to actually persist the returned chunks
// afterwards. The method sets the persistWatermark and the dirty flag
// accordingly.
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) chunksToPersist() []*chunkDesc {
	newWatermark := len(s.chunkDescs)
	if !s.headChunkClosed {
		newWatermark--
	}
	if newWatermark == s.persistWatermark {
		return nil
	}
	cds := s.chunkDescs[s.persistWatermark:newWatermark]
	s.dirty = true
	s.persistWatermark = newWatermark
	return cds
}

// memorySeriesIterator implements SeriesIterator.
type memorySeriesIterator struct {
	chunkIt  chunkIterator   // Last chunkIterator used by ValueAtTime.
	chunkIts []chunkIterator // Caches chunkIterators.
	chunks   []chunk
}

// ValueAtTime implements SeriesIterator.
func (it *memorySeriesIterator) ValueAtTime(t model.Time) []model.SamplePair {
	// The most common case. We are iterating through a chunk.
	if it.chunkIt != nil && it.chunkIt.contains(t) {
		return it.chunkIt.valueAtTime(t)
	}

	if len(it.chunks) == 0 {
		return nil
	}

	// Before or exactly on the first sample of the series.
	it.chunkIt = it.chunkIterator(0)
	ts := it.chunkIt.timestampAtIndex(0)
	if !t.After(ts) {
		// return first value of first chunk
		return []model.SamplePair{{
			Timestamp: ts,
			Value:     it.chunkIt.sampleValueAtIndex(0),
		}}
	}

	// After or exactly on the last sample of the series.
	it.chunkIt = it.chunkIterator(len(it.chunks) - 1)
	ts = it.chunkIt.lastTimestamp()
	if !t.Before(ts) {
		// return last value of last chunk
		return []model.SamplePair{{
			Timestamp: ts,
			Value:     it.chunkIt.sampleValueAtIndex(it.chunkIt.length() - 1),
		}}
	}

	// Find last chunk where firstTime() is before or equal to t.
	l := len(it.chunks) - 1
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[l-i].firstTime().After(t)
	})
	if i == len(it.chunks) {
		panic("out of bounds")
	}
	it.chunkIt = it.chunkIterator(l - i)
	ts = it.chunkIt.lastTimestamp()
	if t.After(ts) {
		// We ended up between two chunks.
		sp1 := model.SamplePair{
			Timestamp: ts,
			Value:     it.chunkIt.sampleValueAtIndex(it.chunkIt.length() - 1),
		}
		it.chunkIt = it.chunkIterator(l - i + 1)
		return []model.SamplePair{
			sp1,
			{
				Timestamp: it.chunkIt.timestampAtIndex(0),
				Value:     it.chunkIt.sampleValueAtIndex(0),
			},
		}
	}
	return it.chunkIt.valueAtTime(t)
}

// BoundaryValues implements SeriesIterator.
func (it *memorySeriesIterator) BoundaryValues(in metric.Interval) []model.SamplePair {
	// Find the first chunk for which the first sample is within the interval.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].firstTime().Before(in.OldestInclusive)
	})
	// Only now check the last timestamp of the previous chunk (which is
	// fairly expensive).
	if i > 0 && !it.chunkIterator(i-1).lastTimestamp().Before(in.OldestInclusive) {
		i--
	}

	values := make([]model.SamplePair, 0, 2)
	for j, c := range it.chunks[i:] {
		if c.firstTime().After(in.NewestInclusive) {
			if len(values) == 1 {
				// We found the first value before but are now
				// already past the last value. The value we
				// want must be the last value of the previous
				// chunk. So backtrack...
				chunkIt := it.chunkIterator(i + j - 1)
				values = append(values, model.SamplePair{
					Timestamp: chunkIt.lastTimestamp(),
					Value:     chunkIt.lastSampleValue(),
				})
			}
			break
		}
		chunkIt := it.chunkIterator(i + j)
		if len(values) == 0 {
			firstValues := chunkIt.valueAtTime(in.OldestInclusive)
			switch len(firstValues) {
			case 2:
				values = append(values, firstValues[1])
			case 1:
				values = firstValues
			default:
				panic("unexpected return from valueAtTime")
			}
		}
		if chunkIt.lastTimestamp().After(in.NewestInclusive) {
			values = append(values, chunkIt.valueAtTime(in.NewestInclusive)[0])
			break
		}
	}
	if len(values) == 1 {
		// We found exactly one value. In that case, add the most recent we know.
		chunkIt := it.chunkIterator(len(it.chunks) - 1)
		values = append(values, model.SamplePair{
			Timestamp: chunkIt.lastTimestamp(),
			Value:     chunkIt.lastSampleValue(),
		})
	}
	if len(values) == 2 && values[0].Equal(&values[1]) {
		return values[:1]
	}
	return values
}

// RangeValues implements SeriesIterator.
func (it *memorySeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	// Find the first chunk for which the first sample is within the interval.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].firstTime().Before(in.OldestInclusive)
	})
	// Only now check the last timestamp of the previous chunk (which is
	// fairly expensive).
	if i > 0 && !it.chunkIterator(i-1).lastTimestamp().Before(in.OldestInclusive) {
		i--
	}

	values := []model.SamplePair{}
	for j, c := range it.chunks[i:] {
		if c.firstTime().After(in.NewestInclusive) {
			break
		}
		values = append(values, it.chunkIterator(i+j).rangeValues(in)...)
	}
	return values
}

// chunkIterator returns the chunkIterator for the chunk at position i (and
// creates it if needed).
func (it *memorySeriesIterator) chunkIterator(i int) chunkIterator {
	chunkIt := it.chunkIts[i]
	if chunkIt == nil {
		chunkIt = it.chunks[i].newIterator()
		it.chunkIts[i] = chunkIt
	}
	return chunkIt
}

// nopSeriesIterator implements Series Iterator. It never returns any values.
type nopSeriesIterator struct{}

// ValueAtTime implements SeriesIterator.
func (i nopSeriesIterator) ValueAtTime(t model.Time) []model.SamplePair {
	return []model.SamplePair{}
}

// BoundaryValues implements SeriesIterator.
func (i nopSeriesIterator) BoundaryValues(in metric.Interval) []model.SamplePair {
	return []model.SamplePair{}
}

// RangeValues implements SeriesIterator.
func (i nopSeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	return []model.SamplePair{}
}
