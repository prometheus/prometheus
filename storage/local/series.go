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
	// The timestamp of the last sample in this series. Needed for fast
	// access for federation and to ensure timestamp monotonicity during
	// ingestion.
	lastTime model.Time
	// The last ingested sample value. Needed for fast access for
	// federation.
	lastSampleValue model.SampleValue
	// Whether lastSampleValue has been set already.
	lastSampleValueSet bool
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
func newMemorySeries(m model.Metric, chunkDescs []*chunkDesc, modTime time.Time) (*memorySeries, error) {
	var err error
	firstTime := model.Earliest
	lastTime := model.Earliest
	if len(chunkDescs) > 0 {
		firstTime = chunkDescs[0].firstTime()
		if lastTime, err = chunkDescs[len(chunkDescs)-1].lastTime(); err != nil {
			return nil, err
		}
	}
	return &memorySeries{
		metric:           m,
		chunkDescs:       chunkDescs,
		headChunkClosed:  len(chunkDescs) > 0,
		savedFirstTime:   firstTime,
		lastTime:         lastTime,
		persistWatermark: len(chunkDescs),
		modTime:          modTime,
	}, nil
}

// add adds a sample pair to the series. It returns the number of newly
// completed chunks (which are now eligible for persistence).
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) add(v model.SamplePair) (int, error) {
	if len(s.chunkDescs) == 0 || s.headChunkClosed {
		newHead := newChunkDesc(newChunk(), v.Timestamp)
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

	chunks, err := s.head().add(v)
	if err != nil {
		return 0, err
	}
	s.head().c = chunks[0]

	for _, c := range chunks[1:] {
		s.chunkDescs = append(s.chunkDescs, newChunkDesc(c, c.firstTime()))
	}

	// Populate lastTime of now-closed chunks.
	for _, cd := range s.chunkDescs[len(s.chunkDescs)-len(chunks) : len(s.chunkDescs)-1] {
		cd.maybePopulateLastTime()
	}

	s.lastTime = v.Timestamp
	s.lastSampleValue = v.Value
	s.lastSampleValueSet = true
	return len(chunks) - 1, nil
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
		s.head().maybePopulateLastTime()
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
func (s *memorySeries) dropChunks(t model.Time) error {
	keepIdx := len(s.chunkDescs)
	for i, cd := range s.chunkDescs {
		lt, err := cd.lastTime()
		if err != nil {
			return err
		}
		if !lt.Before(t) {
			keepIdx = i
			break
		}
	}
	if keepIdx == len(s.chunkDescs) && !s.headChunkClosed {
		// Never drop an open head chunk.
		keepIdx--
	}
	if keepIdx <= 0 {
		// Nothing to drop.
		return nil
	}
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
	return nil
}

// preloadChunks is an internal helper method.
func (s *memorySeries) preloadChunks(
	indexes []int, fp model.Fingerprint, mss *MemorySeriesStorage,
) (SeriesIterator, error) {
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
			return nopIter, err
		}
		for i, c := range chunks {
			s.chunkDescs[loadIndexes[i]].setChunk(c)
		}
	}

	if !s.headChunkClosed && indexes[len(indexes)-1] == len(s.chunkDescs)-1 {
		s.headChunkUsedByIterator = true
	}

	curriedQuarantineSeries := func(err error) {
		mss.quarantineSeries(fp, s.metric, err)
	}

	iter := &boundedIterator{
		it:    s.newIterator(pinnedChunkDescs, curriedQuarantineSeries, mss.evictRequests),
		start: model.Now().Add(-mss.dropAfter),
	}

	return iter, nil
}

// newIterator returns a new SeriesIterator for the provided chunkDescs (which
// must be pinned).
//
// The caller must have locked the fingerprint of the memorySeries.
func (s *memorySeries) newIterator(
	pinnedChunkDescs []*chunkDesc,
	quarantine func(error),
	evictRequests chan<- evictRequest,
) SeriesIterator {
	chunks := make([]chunk, 0, len(pinnedChunkDescs))
	for _, cd := range pinnedChunkDescs {
		// It's OK to directly access cd.c here (without locking) as the
		// series FP is locked and the chunk is pinned.
		chunks = append(chunks, cd.c)
	}
	return &memorySeriesIterator{
		chunks:           chunks,
		chunkIts:         make([]chunkIterator, len(chunks)),
		quarantine:       quarantine,
		metric:           s.metric,
		pinnedChunkDescs: pinnedChunkDescs,
		evictRequests:    evictRequests,
	}
}

// preloadChunksForInstant preloads chunks for the latest value in the given
// range. If the last sample saved in the memorySeries itself is the latest
// value in the given range, it will in fact preload zero chunks and just take
// that value.
func (s *memorySeries) preloadChunksForInstant(
	fp model.Fingerprint,
	from model.Time, through model.Time,
	mss *MemorySeriesStorage,
) (SeriesIterator, error) {
	// If we have a lastSamplePair in the series, and thas last samplePair
	// is in the interval, just take it in a singleSampleSeriesIterator. No
	// need to pin or load anything.
	lastSample := s.lastSamplePair()
	if !through.Before(lastSample.Timestamp) &&
		!from.After(lastSample.Timestamp) &&
		lastSample != ZeroSamplePair {
		iter := &boundedIterator{
			it: &singleSampleSeriesIterator{
				samplePair: lastSample,
				metric:     s.metric,
			},
			start: model.Now().Add(-mss.dropAfter),
		}
		return iter, nil
	}
	// If we are here, we are out of luck and have to delegate to the more
	// expensive method.
	return s.preloadChunksForRange(fp, from, through, mss)
}

// preloadChunksForRange loads chunks for the given range from the persistence.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) preloadChunksForRange(
	fp model.Fingerprint,
	from model.Time, through model.Time,
	mss *MemorySeriesStorage,
) (SeriesIterator, error) {
	firstChunkDescTime := model.Latest
	if len(s.chunkDescs) > 0 {
		firstChunkDescTime = s.chunkDescs[0].firstTime()
	}
	if s.chunkDescsOffset != 0 && from.Before(firstChunkDescTime) {
		cds, err := mss.loadChunkDescs(fp, s.persistWatermark)
		if err != nil {
			return nopIter, err
		}
		s.chunkDescs = append(cds, s.chunkDescs...)
		s.chunkDescsOffset = 0
		s.persistWatermark += len(cds)
		firstChunkDescTime = s.chunkDescs[0].firstTime()
	}

	if len(s.chunkDescs) == 0 || through.Before(firstChunkDescTime) {
		return nopIter, nil
	}

	// Find first chunk with start time after "from".
	fromIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(from)
	})
	// Find first chunk with start time after "through".
	throughIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].firstTime().After(through)
	})
	if fromIdx == len(s.chunkDescs) {
		// Even the last chunk starts before "from". Find out if the
		// series ends before "from" and we don't need to do anything.
		lt, err := s.chunkDescs[len(s.chunkDescs)-1].lastTime()
		if err != nil {
			return nopIter, err
		}
		if lt.Before(from) {
			return nopIter, nil
		}
	}
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

// head returns a pointer to the head chunk descriptor. The caller must have
// locked the fingerprint of the memorySeries. This method will panic if this
// series has no chunk descriptors.
func (s *memorySeries) head() *chunkDesc {
	return s.chunkDescs[len(s.chunkDescs)-1]
}

// firstTime returns the timestamp of the first sample in the series.
//
// The caller must have locked the fingerprint of the memorySeries.
func (s *memorySeries) firstTime() model.Time {
	if s.chunkDescsOffset == 0 && len(s.chunkDescs) > 0 {
		return s.chunkDescs[0].firstTime()
	}
	return s.savedFirstTime
}

// lastSamplePair returns the last ingested SamplePair. It returns
// ZeroSamplePair if this memorySeries has never received a sample (via the add
// method), which is the case for freshly unarchived series or newly created
// ones and also for all series after a server restart. However, in that case,
// series will most likely be considered stale anyway.
//
// The caller must have locked the fingerprint of the memorySeries.
func (s *memorySeries) lastSamplePair() model.SamplePair {
	if !s.lastSampleValueSet {
		return ZeroSamplePair
	}
	return model.SamplePair{
		Timestamp: s.lastTime,
		Value:     s.lastSampleValue,
	}
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
	// Last chunkIterator used by ValueAtOrBeforeTime.
	chunkIt chunkIterator
	// Caches chunkIterators.
	chunkIts []chunkIterator
	// The actual sample chunks.
	chunks []chunk
	// Call to quarantine the series this iterator belongs to.
	quarantine func(error)
	// The metric corresponding to the iterator.
	metric model.Metric
	// Chunks that were pinned for this iterator.
	pinnedChunkDescs []*chunkDesc
	// Where to send evict requests when unpinning pinned chunks.
	evictRequests chan<- evictRequest
}

// ValueAtOrBeforeTime implements SeriesIterator.
func (it *memorySeriesIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	// The most common case. We are iterating through a chunk.
	if it.chunkIt != nil {
		containsT, err := it.chunkIt.contains(t)
		if err != nil {
			it.quarantine(err)
			return ZeroSamplePair
		}
		if containsT {
			if it.chunkIt.findAtOrBefore(t) {
				return it.chunkIt.value()
			}
			if it.chunkIt.err() != nil {
				it.quarantine(it.chunkIt.err())
			}
			return ZeroSamplePair
		}
	}

	if len(it.chunks) == 0 {
		return ZeroSamplePair
	}

	// Find the last chunk where firstTime() is before or equal to t.
	l := len(it.chunks) - 1
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[l-i].firstTime().After(t)
	})
	if i == len(it.chunks) {
		// Even the first chunk starts after t.
		return ZeroSamplePair
	}
	it.chunkIt = it.chunkIterator(l - i)
	if it.chunkIt.findAtOrBefore(t) {
		return it.chunkIt.value()
	}
	if it.chunkIt.err() != nil {
		it.quarantine(it.chunkIt.err())
	}
	return ZeroSamplePair
}

// RangeValues implements SeriesIterator.
func (it *memorySeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	// Find the first chunk for which the first sample is within the interval.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].firstTime().Before(in.OldestInclusive)
	})
	// Only now check the last timestamp of the previous chunk (which is
	// fairly expensive).
	if i > 0 {
		lt, err := it.chunkIterator(i - 1).lastTimestamp()
		if err != nil {
			it.quarantine(err)
			return nil
		}
		if !lt.Before(in.OldestInclusive) {
			i--
		}
	}

	values := []model.SamplePair{}
	for j, c := range it.chunks[i:] {
		if c.firstTime().After(in.NewestInclusive) {
			break
		}
		chValues, err := rangeValues(it.chunkIterator(i+j), in)
		if err != nil {
			it.quarantine(err)
			return nil
		}
		values = append(values, chValues...)
	}
	return values
}

func (it *memorySeriesIterator) Metric() metric.Metric {
	return metric.Metric{Metric: it.metric}
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

func (it *memorySeriesIterator) Close() {
	for _, cd := range it.pinnedChunkDescs {
		cd.unpin(it.evictRequests)
	}
	chunkOps.WithLabelValues(unpin).Add(float64(len(it.pinnedChunkDescs)))
}

// singleSampleSeriesIterator implements Series Iterator. It is a "shortcut
// iterator" that returns a single sample only. The sample is saved in the
// iterator itself, so no chunks need to be pinned.
type singleSampleSeriesIterator struct {
	samplePair model.SamplePair
	metric     model.Metric
}

// ValueAtTime implements SeriesIterator.
func (it *singleSampleSeriesIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	if it.samplePair.Timestamp.After(t) {
		return ZeroSamplePair
	}
	return it.samplePair
}

// RangeValues implements SeriesIterator.
func (it *singleSampleSeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	if it.samplePair.Timestamp.After(in.NewestInclusive) ||
		it.samplePair.Timestamp.Before(in.OldestInclusive) {
		return []model.SamplePair{}
	}
	return []model.SamplePair{it.samplePair}
}

func (it *singleSampleSeriesIterator) Metric() metric.Metric {
	return metric.Metric{Metric: it.metric}
}

// Close implements SeriesIterator.
func (it *singleSampleSeriesIterator) Close() {}

// nopSeriesIterator implements Series Iterator. It never returns any values.
type nopSeriesIterator struct{}

// ValueAtTime implements SeriesIterator.
func (i nopSeriesIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	return ZeroSamplePair
}

// RangeValues implements SeriesIterator.
func (i nopSeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	return []model.SamplePair{}
}

// Metric implements SeriesIterator.
func (i nopSeriesIterator) Metric() metric.Metric {
	return metric.Metric{}
}

// Close implements SeriesIterator.
func (i nopSeriesIterator) Close() {}

var nopIter nopSeriesIterator // A nopSeriesIterator for convenience. Can be shared.
