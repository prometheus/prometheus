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
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/local/chunk"
	"github.com/prometheus/prometheus/storage/metric"
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
	s, ok = sm.m[fp]
	// Note that the RUnlock is not done via defer for performance reasons.
	// TODO(beorn7): Once https://github.com/golang/go/issues/14939 is
	// fixed, revert to the usual defer idiom.
	sm.mtx.RUnlock()
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

// sortedFPs returns a sorted slice of all the fingerprints in the seriesMap.
func (sm *seriesMap) sortedFPs() model.Fingerprints {
	sm.mtx.RLock()
	fps := make(model.Fingerprints, 0, len(sm.m))
	for fp := range sm.m {
		fps = append(fps, fp)
	}
	sm.mtx.RUnlock()

	// Sorting could take some time, so do it outside of the lock.
	sort.Sort(fps)
	return fps
}

type memorySeries struct {
	metric model.Metric
	// Sorted by start time, overlapping chunk ranges are forbidden.
	chunkDescs []*chunk.Desc
	// The index (within chunkDescs above) of the first chunk.Desc that
	// points to a non-persisted chunk. If all chunks are persisted, then
	// persistWatermark == len(chunkDescs).
	persistWatermark int
	// The modification time of the series file. The zero value of time.Time
	// is used to mark an unknown modification time.
	modTime time.Time
	// The chunkDescs in memory might not have all the chunkDescs for the
	// chunks that are persisted to disk. The missing chunkDescs are all
	// contiguous and at the tail end. chunkDescsOffset is the index of the
	// chunk on disk that corresponds to the first chunk.Desc in memory. If
	// it is 0, the chunkDescs are all loaded. A value of -1 denotes a
	// special case: There are chunks on disk, but the offset to the
	// chunkDescs in memory is unknown. Also, in this special case, there is
	// no overlap between chunks on disk and chunks in memory (implying that
	// upon first persisting of a chunk in memory, the offset has to be
	// set).
	chunkDescsOffset int
	// The savedFirstTime field is used as a fallback when the
	// chunkDescsOffset is not 0. It can be used to save the FirstTime of the
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
func newMemorySeries(m model.Metric, chunkDescs []*chunk.Desc, modTime time.Time) (*memorySeries, error) {
	var err error
	firstTime := model.Earliest
	lastTime := model.Earliest
	if len(chunkDescs) > 0 {
		firstTime = chunkDescs[0].FirstTime()
		if lastTime, err = chunkDescs[len(chunkDescs)-1].LastTime(); err != nil {
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
		newHead := chunk.NewDesc(chunk.New(), v.Timestamp)
		s.chunkDescs = append(s.chunkDescs, newHead)
		s.headChunkClosed = false
	} else if s.headChunkUsedByIterator && s.head().RefCount() > 1 {
		// We only need to clone the head chunk if the current head
		// chunk was used in an iterator at all and if the refCount is
		// still greater than the 1 we always have because the head
		// chunk is not yet persisted. The latter is just an
		// approximation. We will still clone unnecessarily if an older
		// iterator using a previous version of the head chunk is still
		// around and keep the head chunk pinned. We needed to track
		// pins by version of the head chunk, which is probably not
		// worth the effort.
		chunk.Ops.WithLabelValues(chunk.Clone).Inc()
		// No locking needed here because a non-persisted head chunk can
		// not get evicted concurrently.
		s.head().C = s.head().C.Clone()
		s.headChunkUsedByIterator = false
	}

	chunks, err := s.head().Add(v)
	if err != nil {
		return 0, err
	}
	s.head().C = chunks[0]

	for _, c := range chunks[1:] {
		s.chunkDescs = append(s.chunkDescs, chunk.NewDesc(c, c.FirstTime()))
	}

	// Populate lastTime of now-closed chunks.
	for _, cd := range s.chunkDescs[len(s.chunkDescs)-len(chunks) : len(s.chunkDescs)-1] {
		if err := cd.MaybePopulateLastTime(); err != nil {
			return 0, err
		}
	}

	s.lastTime = v.Timestamp
	s.lastSampleValue = v.Value
	s.lastSampleValueSet = true
	return len(chunks) - 1, nil
}

// maybeCloseHeadChunk closes the head chunk if it has not been touched for the
// provided duration. It returns whether the head chunk was closed.  If the head
// chunk is already closed, the method is a no-op and returns false.
//
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) maybeCloseHeadChunk(timeout time.Duration) (bool, error) {
	if s.headChunkClosed {
		return false, nil
	}
	if time.Now().Sub(s.lastTime.Time()) > timeout {
		s.headChunkClosed = true
		// Since we cannot modify the head chunk from now on, we
		// don't need to bother with cloning anymore.
		s.headChunkUsedByIterator = false
		return true, s.head().MaybePopulateLastTime()
	}
	return false, nil
}

// evictChunkDescs evicts chunkDescs. lenToEvict is the index within the current
// chunkDescs of the oldest chunk that is not evicted.
func (s *memorySeries) evictChunkDescs(lenToEvict int) {
	if lenToEvict < 1 {
		return
	}
	if s.chunkDescsOffset < 0 {
		panic("chunk desc eviction requested with unknown chunk desc offset")
	}
	lenToKeep := len(s.chunkDescs) - lenToEvict
	s.savedFirstTime = s.firstTime()
	s.chunkDescsOffset += lenToEvict
	s.persistWatermark -= lenToEvict
	chunk.DescOps.WithLabelValues(chunk.Evict).Add(float64(lenToEvict))
	chunk.NumMemDescs.Sub(float64(lenToEvict))
	s.chunkDescs = append(
		make([]*chunk.Desc, 0, lenToKeep),
		s.chunkDescs[lenToEvict:]...,
	)
	s.dirty = true
}

// dropChunks removes chunkDescs older than t. The caller must have locked the
// fingerprint of the series.
func (s *memorySeries) dropChunks(t model.Time) error {
	keepIdx := len(s.chunkDescs)
	for i, cd := range s.chunkDescs {
		lt, err := cd.LastTime()
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
		make([]*chunk.Desc, 0, len(s.chunkDescs)-keepIdx),
		s.chunkDescs[keepIdx:]...,
	)
	s.persistWatermark -= keepIdx
	if s.persistWatermark < 0 {
		panic("dropped unpersisted chunks from memory")
	}
	if s.chunkDescsOffset != -1 {
		s.chunkDescsOffset += keepIdx
	}
	chunk.NumMemDescs.Sub(float64(keepIdx))
	s.dirty = true
	return nil
}

// preloadChunks is an internal helper method.
func (s *memorySeries) preloadChunks(
	indexes []int, fp model.Fingerprint, mss *MemorySeriesStorage,
) (SeriesIterator, error) {
	loadIndexes := []int{}
	pinnedChunkDescs := make([]*chunk.Desc, 0, len(indexes))
	for _, idx := range indexes {
		cd := s.chunkDescs[idx]
		pinnedChunkDescs = append(pinnedChunkDescs, cd)
		cd.Pin(mss.evictRequests) // Have to pin everything first to prevent immediate eviction on chunk loading.
		if cd.IsEvicted() {
			loadIndexes = append(loadIndexes, idx)
		}
	}
	chunk.Ops.WithLabelValues(chunk.Pin).Add(float64(len(pinnedChunkDescs)))

	if len(loadIndexes) > 0 {
		if s.chunkDescsOffset == -1 {
			panic("requested loading chunks from persistence in a situation where we must not have persisted data for chunk descriptors in memory")
		}
		chunks, err := mss.loadChunks(fp, loadIndexes, s.chunkDescsOffset)
		if err != nil {
			// Unpin the chunks since we won't return them as pinned chunks now.
			for _, cd := range pinnedChunkDescs {
				cd.Unpin(mss.evictRequests)
			}
			chunk.Ops.WithLabelValues(chunk.Unpin).Add(float64(len(pinnedChunkDescs)))
			return nopIter, err
		}
		for i, c := range chunks {
			s.chunkDescs[loadIndexes[i]].SetChunk(c)
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
	pinnedChunkDescs []*chunk.Desc,
	quarantine func(error),
	evictRequests chan<- chunk.EvictRequest,
) SeriesIterator {
	chunks := make([]chunk.Chunk, 0, len(pinnedChunkDescs))
	for _, cd := range pinnedChunkDescs {
		// It's OK to directly access cd.c here (without locking) as the
		// series FP is locked and the chunk is pinned.
		chunks = append(chunks, cd.C)
	}
	return &memorySeriesIterator{
		chunks:           chunks,
		chunkIts:         make([]chunk.Iterator, len(chunks)),
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
	// If we have a lastSamplePair in the series, and this last samplePair
	// is in the interval, just take it in a singleSampleSeriesIterator. No
	// need to pin or load anything.
	lastSample := s.lastSamplePair()
	if !through.Before(lastSample.Timestamp) &&
		!from.After(lastSample.Timestamp) &&
		lastSample != model.ZeroSamplePair {
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
		firstChunkDescTime = s.chunkDescs[0].FirstTime()
	}
	if s.chunkDescsOffset != 0 && from.Before(firstChunkDescTime) {
		cds, err := mss.loadChunkDescs(fp, s.persistWatermark)
		if err != nil {
			return nopIter, err
		}
		if s.chunkDescsOffset != -1 && len(cds) != s.chunkDescsOffset {
			return nopIter, fmt.Errorf(
				"unexpected number of chunk descs loaded for fingerprint %v: expected %d, got %d",
				fp, s.chunkDescsOffset, len(cds),
			)
		}
		s.persistWatermark += len(cds)
		s.chunkDescs = append(cds, s.chunkDescs...)
		s.chunkDescsOffset = 0
		if len(s.chunkDescs) > 0 {
			firstChunkDescTime = s.chunkDescs[0].FirstTime()
		}
	}

	if len(s.chunkDescs) == 0 || through.Before(firstChunkDescTime) {
		return nopIter, nil
	}

	// Find first chunk with start time after "from".
	fromIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].FirstTime().After(from)
	})
	// Find first chunk with start time after "through".
	throughIdx := sort.Search(len(s.chunkDescs), func(i int) bool {
		return s.chunkDescs[i].FirstTime().After(through)
	})
	if fromIdx == len(s.chunkDescs) {
		// Even the last chunk starts before "from". Find out if the
		// series ends before "from" and we don't need to do anything.
		lt, err := s.chunkDescs[len(s.chunkDescs)-1].LastTime()
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
	if fromIdx > throughIdx {
		// Guard against nonsensical result. The caller will quarantine the series with a meaningful log entry.
		return nopIter, fmt.Errorf("fromIdx=%d is greater than throughIdx=%d, likely caused by data corruption", fromIdx, throughIdx)
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
func (s *memorySeries) head() *chunk.Desc {
	return s.chunkDescs[len(s.chunkDescs)-1]
}

// firstTime returns the timestamp of the first sample in the series.
//
// The caller must have locked the fingerprint of the memorySeries.
func (s *memorySeries) firstTime() model.Time {
	if s.chunkDescsOffset == 0 && len(s.chunkDescs) > 0 {
		return s.chunkDescs[0].FirstTime()
	}
	return s.savedFirstTime
}

// lastSamplePair returns the last ingested SamplePair. It returns
// model.ZeroSamplePair if this memorySeries has never received a sample (via the add
// method), which is the case for freshly unarchived series or newly created
// ones and also for all series after a server restart. However, in that case,
// series will most likely be considered stale anyway.
//
// The caller must have locked the fingerprint of the memorySeries.
func (s *memorySeries) lastSamplePair() model.SamplePair {
	if !s.lastSampleValueSet {
		return model.ZeroSamplePair
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
func (s *memorySeries) chunksToPersist() []*chunk.Desc {
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
	// Last chunk.Iterator used by ValueAtOrBeforeTime.
	chunkIt chunk.Iterator
	// Caches chunkIterators.
	chunkIts []chunk.Iterator
	// The actual sample chunks.
	chunks []chunk.Chunk
	// Call to quarantine the series this iterator belongs to.
	quarantine func(error)
	// The metric corresponding to the iterator.
	metric model.Metric
	// Chunks that were pinned for this iterator.
	pinnedChunkDescs []*chunk.Desc
	// Where to send evict requests when unpinning pinned chunks.
	evictRequests chan<- chunk.EvictRequest
}

// ValueAtOrBeforeTime implements SeriesIterator.
func (it *memorySeriesIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	// The most common case. We are iterating through a chunk.
	if it.chunkIt != nil {
		containsT, err := it.chunkIt.Contains(t)
		if err != nil {
			it.quarantine(err)
			return model.ZeroSamplePair
		}
		if containsT {
			if it.chunkIt.FindAtOrBefore(t) {
				return it.chunkIt.Value()
			}
			if it.chunkIt.Err() != nil {
				it.quarantine(it.chunkIt.Err())
			}
			return model.ZeroSamplePair
		}
	}

	if len(it.chunks) == 0 {
		return model.ZeroSamplePair
	}

	// Find the last chunk where FirstTime() is before or equal to t.
	l := len(it.chunks) - 1
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[l-i].FirstTime().After(t)
	})
	if i == len(it.chunks) {
		// Even the first chunk starts after t.
		return model.ZeroSamplePair
	}
	it.chunkIt = it.chunkIterator(l - i)
	if it.chunkIt.FindAtOrBefore(t) {
		return it.chunkIt.Value()
	}
	if it.chunkIt.Err() != nil {
		it.quarantine(it.chunkIt.Err())
	}
	return model.ZeroSamplePair
}

// RangeValues implements SeriesIterator.
func (it *memorySeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	// Find the first chunk for which the first sample is within the interval.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].FirstTime().Before(in.OldestInclusive)
	})
	// Only now check the last timestamp of the previous chunk (which is
	// fairly expensive).
	if i > 0 {
		lt, err := it.chunkIterator(i - 1).LastTimestamp()
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
		if c.FirstTime().After(in.NewestInclusive) {
			break
		}
		chValues, err := chunk.RangeValues(it.chunkIterator(i+j), in)
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

// chunkIterator returns the chunk.Iterator for the chunk at position i (and
// creates it if needed).
func (it *memorySeriesIterator) chunkIterator(i int) chunk.Iterator {
	chunkIt := it.chunkIts[i]
	if chunkIt == nil {
		chunkIt = it.chunks[i].NewIterator()
		it.chunkIts[i] = chunkIt
	}
	return chunkIt
}

func (it *memorySeriesIterator) Close() {
	for _, cd := range it.pinnedChunkDescs {
		cd.Unpin(it.evictRequests)
	}
	chunk.Ops.WithLabelValues(chunk.Unpin).Add(float64(len(it.pinnedChunkDescs)))
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
		return model.ZeroSamplePair
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
	return model.ZeroSamplePair
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
