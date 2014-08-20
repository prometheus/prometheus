package storage_ng

import (
	"sort"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

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
	cd.chunk.close()
	cd.chunk = nil
}

type memorySeries struct {
	mtx sync.Mutex

	metric clientmodel.Metric
	// Sorted by start time, no overlapping chunk ranges allowed.
	chunkDescs       chunkDescs
	chunkDescsLoaded bool
}

func newMemorySeries(m clientmodel.Metric) *memorySeries {
	return &memorySeries{
		metric: m,
		// TODO: should we set this to nil initially and only create a chunk when
		// adding? But right now, we also only call newMemorySeries when adding, so
		// it turns out to be the same.
		chunkDescs: chunkDescs{
			// TODO: should there be a newChunkDesc() function?
			&chunkDesc{
				chunk:    newDeltaEncodedChunk(d1, d0, true),
				refCount: 1,
			},
		},
		chunkDescsLoaded: true,
	}
}

func (s *memorySeries) add(v *metric.SamplePair, persistQueue chan *persistRequest) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	chunks := s.head().add(v)

	s.head().chunk = chunks[0]
	if len(chunks) > 1 {
		fp := s.metric.Fingerprint()

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

func (s *memorySeries) evictOlderThan(t clientmodel.Timestamp) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// For now, always drop the entire range from oldest to t.
	for _, cd := range s.chunkDescs {
		if !cd.lastTime().Before(t) {
			break
		}
		if cd.chunk == nil {
			continue
		}
		cd.evictOnUnpin()
	}
}

func (s *memorySeries) purgeOlderThan(t clientmodel.Timestamp, p Persistence) (dropSeries bool, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if err := p.DropChunks(s.metric.Fingerprint(), t); err != nil {
		return false, err
	}

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

	return len(s.chunkDescs) == 0, nil
}

func (s *memorySeries) close() {
	for _, cd := range s.chunkDescs {
		if cd.chunk != nil {
			cd.evictNow()
		}
		// TODO: need to handle unwritten heads here.
	}
}

// TODO: in this method (and other places), we just fudge around with chunkDesc
// internals without grabbing the chunkDesc lock. Study how this needs to be
// protected against other accesses that don't hold the series lock.
func (s *memorySeries) preloadChunks(indexes []int, p Persistence) (chunkDescs, error) {
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
		chunks, err := p.LoadChunks(fp, loadIndexes)
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
func (s *memorySeries) preloadChunksAtTime(t clientmodel.Timestamp, p Persistence) (chunkDescs, error) {
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

func (s *memorySeries) loadChunkDescs(p Persistence) error {
	cds, err := p.LoadChunkDescs(s.metric.Fingerprint(), s.chunkDescs[0].firstTime())
	if err != nil {
		return err
	}
	s.chunkDescs = append(cds, s.chunkDescs...)
	s.chunkDescsLoaded = true
	return nil
}

func (s *memorySeries) preloadChunksForRange(from clientmodel.Timestamp, through clientmodel.Timestamp, p Persistence) (chunkDescs, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if !s.chunkDescsLoaded && from.Before(s.chunkDescs[0].firstTime()) {
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

type memorySeriesIterator struct {
	mtx     *sync.Mutex
	chunkIt chunkIterator
	chunks  chunks
}

func (s *memorySeries) newIterator() SeriesIterator {
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
		mtx:    &s.mtx,
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

func (it *memorySeriesIterator) GetValueAtTime(t clientmodel.Timestamp) metric.Values {
	it.mtx.Lock()
	defer it.mtx.Unlock()

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

func (it *memorySeriesIterator) GetBoundaryValues(in metric.Interval) metric.Values {
	// TODO: implement real GetBoundaryValues here.
	return it.GetRangeValues(in)
}

func (it *memorySeriesIterator) GetRangeValues(in metric.Interval) metric.Values {
	it.mtx.Lock()
	defer it.mtx.Unlock()

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
