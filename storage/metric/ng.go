package metric

import (
	"io"
	"sort"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/raw/leveldb"

	dto "github.com/prometheus/prometheus/model/generated"
)

type IngestionBatch map[clientmodel.Fingerprint]*SampleSet

// Ingester consumes batches of pending samples and stores them accordingly.
type Ingester interface {
	// Ingest consumes the samples an takes whatever action is necessary.
	Ingest(IngestionBatch) error
}

// IndexedIngester indexes incoming samples before passing them to the
// base Ingester.  If the indexing fails, the underlying ingestion will
// be cancelled.
type IndexedIngester struct {
	io.Closer
	Ingester

	Indexer MetricIndexer
}

func (i *IndexedIngester) Ingest(b IngestionBatch) error {
	metricMap := FingerprintMetricMapping{}

	for fp, ss := range b {
		metricMap[fp] = ss.Metric
	}

	if err := i.Indexer.IndexMetrics(metricMap); err != nil {
		glog.Warning("could not index metrics; not ingesting: %s", err)
		return err
	}

	return i.Ingester.Ingest(b)
}

func (i *IndexedIngester) Close() error {
	if closer, ok := i.Ingester.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	if closer, ok := i.Indexer.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}

	return nil
}

type ChunkIterator interface {
	io.Closer

	Ok() bool

	Next()
	Seek(*clientmodel.Fingerprint, time.Time)
	Get() (*clientmodel.Fingerprint, Values, error)
}

type Arena interface {
	Ingester

	GetIterator(*clientmodel.Fingerprint, time.Time) ChunkIterator
}

type Stream interface {
	Clear()
	Add(...*SamplePair)
	Snapshot() StreamSnapshot
}

type StreamSnapshot interface {
	io.Closer

	Values() Values
}

type ArrayStreamSnapshot struct {
	mu sync.RWMutex

	values Values
}

func (s *ArrayStreamSnapshot) Values() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(Values, len(s.values))
	copy(out, s.values)

	return out
}

func (s *ArrayStreamSnapshot) update(v Values) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = v
}

func (s *ArrayStreamSnapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = nil

	return nil
}

type ArrayStreamJournal struct {
	Uncopied []*ArrayStreamSnapshot
}

type ArrayStream struct {
	Stream

	Values Values

	Journal chan *ArrayStreamJournal
}

func (s *ArrayStream) refreshSnapshots(j *ArrayStreamJournal) {
	if len(j.Uncopied) == 0 {
		return
	}

	copied := make(Values, len(s.Values))
	copy(copied, s.Values)

	for _, snapshot := range j.Uncopied {
		snapshot.update(copied)
	}
	j.Uncopied = []*ArrayStreamSnapshot{}
}

func (s *ArrayStream) Clear() {
	journal := <-s.Journal
	s.refreshSnapshots(journal)
	defer func() {
		s.Journal <- journal
	}()

	s.Values = Values{}
}

func (s *ArrayStream) Add(vs ...*SamplePair) {
	journal := <-s.Journal
	s.refreshSnapshots(journal)
	defer func() {
		s.Journal <- journal
	}()

	s.Values = append(s.Values, vs...)

	sort.Sort(s.Values)
}

func (s *ArrayStream) Snapshot() StreamSnapshot {
	journal := <-s.Journal
	defer func() {
		s.Journal <- journal
	}()

	snapshot := &ArrayStreamSnapshot{
		values: s.Values,
	}

	journal.Uncopied = append(journal.Uncopied, snapshot)

	return snapshot
}

type MemoryArena struct {
	Arena

	values map[clientmodel.Fingerprint]Stream

	StreamFac func() Stream
}

func (a *MemoryArena) Ingest(b IngestionBatch) error {
	for fp, values := range b {
		series, ok := a.values[fp]
		if !ok {
			series = a.StreamFac()
			a.values[fp] = series
		}
		series.Add(values.Values...)
	}

	return nil
}

func (a *MemoryArena) GetIterator(fp *clientmodel.Fingerprint, t time.Time) ChunkIterator {
	snap := &MemoryArenaSnapshot{
		values: map[clientmodel.Fingerprint]StreamSnapshot{},
	}

	fps := clientmodel.Fingerprints{}

	for fp, stream := range a.values {
		snap.values[fp] = stream.Snapshot()
		fps = append(fps, &fp)
	}

	sort.Sort(fps)

	it := &memoryArenaChunkIterator{
		snapshot: snap,
		fps:      fps,
	}

	return it
}

func (a *MemoryArena) Close() error {
	a.values = nil

	return nil
}

type memoryArenaChunkIterator struct {
	snapshot *MemoryArenaSnapshot
	fps      clientmodel.Fingerprints
	fp       *clientmodel.Fingerprint
}

func (i *memoryArenaChunkIterator) Seek(fp *clientmodel.Fingerprint, t time.Time) {
	idx := sort.Search(len(i.fps), func(idx int) bool {
		return fp.Less(i.fps[idx])
	})

	if idx == len(i.fps) {
		i.fp = nil
		return
	}

	i.fps = i.fps[idx:]

	i.fp = i.fps[0]
}

func (i *memoryArenaChunkIterator) Close() error {
	return i.snapshot.Close()
}

func (i *memoryArenaChunkIterator) Ok() bool {
	return i.fp != nil
}

func (i *memoryArenaChunkIterator) Next() {
	i.fp = i.fps[1]
	i.fps = i.fps[1:]
}

func (i *memoryArenaChunkIterator) Get() (fp *clientmodel.Fingerprint, v Values, err error) {
	return i.fp, i.snapshot.values[*i.fp].Values(), nil
}

type MemoryArenaSnapshot struct {
	values map[clientmodel.Fingerprint]StreamSnapshot
}

func (s *MemoryArenaSnapshot) Close() error {
	for _, snapshot := range s.values {
		snapshot.Close()
	}

	s.values = nil

	return nil
}

type LevigoArena struct {
	Arena

	DB *leveldb.LevelDBPersistence
}

func (a *LevigoArena) Ingest(b IngestionBatch) error {
	commit := leveldb.NewBatch()
	defer commit.Close()

	for fp, samples := range b {
		sort.Sort(samples.Values)

		key := new(dto.SampleKey)
		base := &SampleKey{
			Fingerprint:    &fp,
			FirstTimestamp: samples.Values[0].Timestamp,
			LastTimestamp:  samples.Values[len(samples.Values)-1].Timestamp,
			SampleCount:    uint32(len(samples.Values)),
		}
		base.Dump(key)

		value := new(dto.SampleValueSeries)
		samples.Values.dump(value)

		commit.Put(key, value)
	}

	return a.DB.Commit(commit)
}

type levigoChunkIterator struct {
	it leveldb.Iterator

	ok bool
}

func (i *levigoChunkIterator) Ok() bool {
	return i.ok
}

func (i *levigoChunkIterator) Next() {
	if !i.ok {
		panic("illegal iterator usage")
	}

	i.ok = i.it.Next()
}

func (i *levigoChunkIterator) seek(fp *clientmodel.Fingerprint, t time.Time) {
	key := new(dto.SampleKey)
	base := &SampleKey{
		Fingerprint:    fp,
		FirstTimestamp: t,
	}
	base.Dump(key)
	buf, err := proto.Marshal(key)
	if err != nil {
		panic("failed to serialize key")
	}
	i.ok = i.it.Seek(buf)
}

func (i *levigoChunkIterator) Seek(fp *clientmodel.Fingerprint, t time.Time) {
	if !i.ok {
		panic("illegal iterator usage")
	}

	i.seek(fp, t)
}

func (i *levigoChunkIterator) Get() (*clientmodel.Fingerprint, Values, error) {
	if !i.ok {
		panic("illegal iterator usage")
	}

	buf := i.it.Key()
	k := new(dto.SampleKey)
	if err := proto.Unmarshal(buf, k); err != nil {
		return nil, nil, err
	}
	key := new(SampleKey)
	key.Load(k)

	buf = i.it.Value()
	v := new(dto.SampleValueSeries)
	if err := proto.Unmarshal(buf, v); err != nil {
		return nil, nil, err
	}

	return key.Fingerprint, NewValuesFromDTO(v), nil
}

func (i *levigoChunkIterator) Close() error {
	i.it.Close()

	return nil
}

func (a *LevigoArena) GetIterator(fp *clientmodel.Fingerprint, t time.Time) ChunkIterator {
	it := a.DB.NewIterator(true)

	chunkIterator := &levigoChunkIterator{
		it: it,
	}

	chunkIterator.seek(fp, t)

	return chunkIterator
}

// RELIC BELOW

// type State int

// const (
// 	Inactive State = iota
// 	Dumping
// 	Purging
// 	Resampling
// )

// type StreamState struct {
// 	State State
// 	Buf   []int
// }

// type StatefulStream struct {
// 	Values []int
// 	State  chan *StreamState
// }

// func NewStatefulStream() *StatefulStream {
// 	s := &StatefulStream{
// 		State: make(chan *StreamState, 1),
// 	}

// 	s.State <- &StreamState{
// 		State: Inactive,
// 	}

// 	return s
// }

// func (s *StatefulStream) flush(state *StreamState) {
// 	s.Values = append(s.Values, state.Buf...)
// 	sort.Ints(s.Values)
// 	state.Buf = []int{}
// }

// func (s *StatefulStream) Add(vs ...int) (ok bool) {
// 	state := <-s.State
// 	defer func() {
// 		s.State <- state
// 	}()

// 	state.Buf = append(state.Buf, vs...)
// 	if state.State != Inactive {
// 		return false
// 	}

// 	s.flush(state)

// 	return true
// }

// func (s *StatefulStream) Resample() (ok bool) {
// 	state := <-s.State
// 	if state.State != Inactive {
// 		s.State <- state
// 		return false
// 	}

// 	state.State = Resampling
// 	s.State <- state

// 	work := make([]int, len(s.Values))
// 	copy(work, s.Values)
// 	time.Sleep(5 * time.Second)
// 	// Resample

// 	state = <-s.State
// 	state.State = Inactive
// 	// Set new values.
// 	s.flush(state)
// 	s.State <- state

// 	return true
// }

// func (s *StatefulStream) Purge() (ok bool) {
// 	state := <-s.State
// 	if state.State != Inactive {
// 		s.State <- state
// 		return false
// 	}

// 	state.State = Purging
// 	s.State <- state

// 	s.Values = []int{}
// 	time.Sleep(1 * time.Second)
// 	// Purge

// 	state = <-s.State
// 	state.State = Inactive
// 	// Set new values.
// 	s.flush(state)
// 	s.State <- state

// 	return true
// }

// func (s *StatefulStream) Dump() (ok bool) {
// 	state := <-s.State
// 	if state.State != Inactive {
// 		s.State <- state
// 		return false
// 	}

// 	state.State = Dumping
// 	s.State <- state

// 	work := make([]int, len(s.Values))
// 	copy(work, s.Values)
// 	time.Sleep(5 * time.Second)
// 	// Dump

// 	state = <-s.State
// 	state.State = Inactive
// 	// Set new values.
// 	s.flush(state)
// 	s.State <- state

// 	return true
// }

// func main() {
// 	s := NewStatefulStream()

// 	_ = new(sync.WaitGroup)
// 	for i := 0; i < 20; i++ {
// 		go func(i int) {
// 			for {
// 				s.Add([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}...)
// 				time.Sleep(5 * time.Millisecond)
// 			}
// 		}(i)
// 	}

// 	go func() {
// 		for {
// 			s.Resample()
// 			time.Sleep(15 * time.Second)
// 		}
// 	}()

// 	go func() {
// 		for {
// 			s.Purge()
// 			time.Sleep(15 * time.Second)
// 		}
// 	}()

// 	for {
// 		s.Dump()
// 		time.Sleep(15 * time.Second)
// 	}
// }
