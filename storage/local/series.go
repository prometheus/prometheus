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
	"sort"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

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

// put adds a mapping to the seriesMap.
func (sm *seriesMap) put(fp clientmodel.Fingerprint, s *memorySeries) {
	sm.mtx.Lock()
	defer sm.mtx.Unlock()

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

type chunkDescs []*chunkDesc

type chunkDesc struct {
	sync.Mutex
	chunk          chunk
	refCount       int
	evict          bool
	firstTimeField clientmodel.Timestamp // TODO: stupid name, reorganize.
	lastTimeField  clientmodel.Timestamp
}

func (cd *chunkDesc) add(s *metric.SamplePair) chunks {
	cd.Lock()
	defer cd.Unlock()

	return cd.chunk.add(s)
}

func (cd *chunkDesc) pin() {
	cd.Lock()
	defer cd.Unlock()

	numPinnedChunks.Inc()
	cd.refCount++
}

func (cd *chunkDesc) unpin() {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount == 0 {
		panic("cannot unpin already unpinned chunk")
	}
	numPinnedChunks.Dec()
	cd.refCount--
	if cd.refCount == 0 && cd.evict {
		cd.evictNow()
	}
}

func (cd *chunkDesc) firstTime() clientmodel.Timestamp {
	if cd.chunk == nil {
		return cd.firstTimeField
	}
	return cd.chunk.firstTime()
}

func (cd *chunkDesc) lastTime() clientmodel.Timestamp {
	if cd.chunk == nil {
		return cd.lastTimeField
	}
	return cd.chunk.lastTime()
}

func (cd *chunkDesc) contains(t clientmodel.Timestamp) bool {
	return !t.Before(cd.firstTime()) && !t.After(cd.lastTime())
}

func (cd *chunkDesc) open(c chunk) {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount != 0 || cd.chunk != nil {
		panic("cannot open already pinned chunk")
	}
	cd.evict = false
	cd.chunk = c
	numPinnedChunks.Inc()
	cd.refCount++
}

func (cd *chunkDesc) evictOnUnpin() {
	cd.Lock()
	defer cd.Unlock()

	if cd.refCount == 0 {
		cd.evictNow()
	}
	cd.evict = true
}

func (cd *chunkDesc) evictNow() {
	cd.firstTimeField = cd.chunk.firstTime()
	cd.lastTimeField = cd.chunk.lastTime()
	cd.chunk = nil
}

type memorySeries struct {
	metric clientmodel.Metric
	// Sorted by start time, overlapping chunk ranges are forbidden.
	chunkDescs chunkDescs
	// Whether chunkDescs for chunks on disk are all loaded.  If false, some
	// (or all) chunkDescs are only on disk. These chunks are all contiguous
	// and at the tail end.
	chunkDescsLoaded bool
	// Whether the current head chunk has already been persisted (or at
	// least has been scheduled to be persisted). If true, the current head
	// chunk must not be modified anymore.
	headChunkPersisted bool
}

// newMemorySeries returns a pointer to a newly allocated memorySeries for the
// given metric. reallyNew defines if the memorySeries is a genuinely new series
// or (if false) a series for a metric being unarchived, i.e. a series that
// existed before but has been evicted from memory.
func newMemorySeries(m clientmodel.Metric, reallyNew bool) *memorySeries {
	return &memorySeries{
		metric:             m,
		chunkDescsLoaded:   reallyNew,
		headChunkPersisted: !reallyNew,
	}
}

// add adds a sample pair to the series.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) add(fp clientmodel.Fingerprint, v *metric.SamplePair, persistQueue chan *persistRequest) {
	if len(s.chunkDescs) == 0 || s.headChunkPersisted {
		newHead := &chunkDesc{
			chunk:    newDeltaEncodedChunk(d1, d0, true),
			refCount: 1,
		}
		s.chunkDescs = append(s.chunkDescs, newHead)
		s.headChunkPersisted = false
	}

	chunks := s.head().add(v)

	s.head().chunk = chunks[0]
	if len(chunks) > 1 {
		queuePersist := func(cd *chunkDesc) {
			persistQueue <- &persistRequest{
				fingerprint: fp,
				chunkDesc:   cd,
			}
		}

		queuePersist(s.head())

		for i, c := range chunks[1:] {
			cd := &chunkDesc{
				chunk:    c,
				refCount: 1,
			}
			s.chunkDescs = append(s.chunkDescs, cd)
			// The last chunk is still growing.
			if i < len(chunks[1:])-1 {
				queuePersist(cd)
			}
		}
	}
}

// persistHeadChunk queues the head chunk for persisting if not already done.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) persistHeadChunk(fp clientmodel.Fingerprint, persistQueue chan *persistRequest) {
	if s.headChunkPersisted {
		return
	}
	s.headChunkPersisted = true
	persistQueue <- &persistRequest{
		fingerprint: fp,
		chunkDesc:   s.head(),
	}
}

// evictOlderThan evicts chunks whose latest sample is older than the given timestamp.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) evictOlderThan(t clientmodel.Timestamp) (allEvicted bool) {
	// For now, always drop the entire range from oldest to t.
	for _, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			return false
		}
		if cd.chunk == nil {
			continue
		}
		cd.evictOnUnpin()
	}
	return true
}

// purgeOlderThan returns true if all chunks have been purged.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) purgeOlderThan(t clientmodel.Timestamp) bool {
	keepIdx := len(s.chunkDescs)
	for i, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			keepIdx = i
			break
		}
	}

	for i := 0; i < keepIdx; i++ {
		if s.chunkDescs[i].chunk != nil {
			s.chunkDescs[i].evictOnUnpin()
		}
	}
	s.chunkDescs = s.chunkDescs[keepIdx:]
	return len(s.chunkDescs) == 0
}

// preloadChunks is an internal helper method.
// TODO: in this method (and other places), we just fudge around with chunkDesc
// internals without grabbing the chunkDesc lock. Study how this needs to be
// protected against other accesses that don't hold the fp lock.
func (s *memorySeries) preloadChunks(indexes []int, p *persistence) (chunkDescs, error) {
	loadIndexes := []int{}
	pinnedChunkDescs := make(chunkDescs, 0, len(indexes))
	for _, idx := range indexes {
		pinnedChunkDescs = append(pinnedChunkDescs, s.chunkDescs[idx])
		if s.chunkDescs[idx].chunk == nil {
			loadIndexes = append(loadIndexes, idx)
		} else {
			s.chunkDescs[idx].pin()
		}
	}

	if len(loadIndexes) > 0 {
		fp := s.metric.Fingerprint()
		chunks, err := p.loadChunks(fp, loadIndexes)
		if err != nil {
			// Unpin any pinned chunks that were already loaded.
			for _, cd := range pinnedChunkDescs {
				if cd.chunk != nil {
					cd.unpin()
				}
			}
			return nil, err
		}
		for i, c := range chunks {
			cd := s.chunkDescs[loadIndexes[i]]
			cd.open(c)
		}
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

// loadChunkDescs is an internal helper method.
func (s *memorySeries) loadChunkDescs(p *persistence) error {
	cds, err := p.loadChunkDescs(s.metric.Fingerprint(), s.chunkDescs[0].firstTime())
	if err != nil {
		return err
	}
	s.chunkDescs = append(cds, s.chunkDescs...)
	s.chunkDescsLoaded = true
	return nil
}

// preloadChunksForRange loads chunks for the given range from the persistence.
// The caller must have locked the fingerprint of the series.
func (s *memorySeries) preloadChunksForRange(from clientmodel.Timestamp, through clientmodel.Timestamp, p *persistence) (chunkDescs, error) {
	if !s.chunkDescsLoaded && (len(s.chunkDescs) == 0 || from.Before(s.chunkDescs[0].firstTime())) {
		if err := s.loadChunkDescs(p); err != nil {
			return nil, err
		}
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

// memorySeriesIterator implements SeriesIterator.
type memorySeriesIterator struct {
	lock, unlock func()
	chunkIt      chunkIterator
	chunks       chunks
}

func (s *memorySeries) newIterator(lockFunc, unlockFunc func()) SeriesIterator {
	chunks := make(chunks, 0, len(s.chunkDescs))
	for i, cd := range s.chunkDescs {
		if cd.chunk != nil {
			if i == len(s.chunkDescs)-1 {
				chunks = append(chunks, cd.chunk.clone())
			} else {
				chunks = append(chunks, cd.chunk)
			}
		}
	}

	return &memorySeriesIterator{
		lock:   lockFunc,
		unlock: unlockFunc,
		chunks: chunks,
	}
}

func (s *memorySeries) head() *chunkDesc {
	return s.chunkDescs[len(s.chunkDescs)-1]
}

func (s *memorySeries) values() metric.Values {
	var values metric.Values
	for _, cd := range s.chunkDescs {
		for sample := range cd.chunk.values() {
			values = append(values, *sample)
		}
	}
	return values
}

func (s *memorySeries) firstTime() clientmodel.Timestamp {
	return s.chunkDescs[0].firstTime()
}

func (s *memorySeries) lastTime() clientmodel.Timestamp {
	return s.head().lastTime()
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
	return it.GetRangeValues(in)

	// TODO: The following doesn't work as expected. Fix it.
	it.lock()
	defer it.unlock()

	// Find the first relevant chunk.
	i := sort.Search(len(it.chunks), func(i int) bool {
		return !it.chunks[i].lastTime().Before(in.OldestInclusive)
	})
	values := metric.Values{}
	for ; i < len(it.chunks); i++ {
		c := it.chunks[i]
		var chunkIt chunkIterator
		if c.firstTime().After(in.NewestInclusive) {
			if len(values) == 1 {
				// We found the first value already, but are now
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
