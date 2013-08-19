package metric

import (
	"io"
	"sort"
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"
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

	Next()
	HasNext() bool
	Get() (*clientmodel.Fingerprint, Values, error)
}

type ChunksForTimesBatch map[clientmodel.Fingerprint][]time.Time

type Arena interface {
	Ingester

	GetChunksForTimes(ChunksForTimesBatch) ChunkIterator
}

type Stream interface {
	Clear()
	Add(...*SamplePair)
	Clone() Values
}

type ArrayStream struct {
	Stream

	Values Values
}

func (s *ArrayStream) Clear() {
	s.Values = Values{}
}

func (s *ArrayStream) Add(vs ...*SamplePair) {
	s.Values = append(s.Values, vs...)

	sort.Sort(s.Values)
}

func (s *ArrayStream) Clone() Values {
	out := make(Values, len(s.Values))
	copy(out, s.Values)
	return out
}

type BufferedStream struct {
	Stream

	Buf Values

	Limit int
}

func (s *BufferedStream) Add(vs ...*SamplePair) {
	s.Buf = append(s.Buf, vs...)

	if len(s.Buf) > s.Limit {
		s.Flush()
	}
}

func (s *BufferedStream) Flush() {
	s.Stream.Add(s.Buf...)
	s.Buf = Values{}
}

func (s *BufferedStream) Clone() Values {
	s.Flush()

	return s.Stream.Clone()
}

type MemoryArena struct {
	Arena

	values map[clientmodel.Fingerprint]Stream
}

func (a *MemoryArena) Ingest(b IngestionBatch) error {
	for fp, values := range b {
		series, ok := a.values[fp]
		if !ok {
			series = new(ArrayStream)
			a.values[fp] = series
		}
		series.Add(values.Values...)
	}

	return nil
}

func (a *MemoryArena) Close() error {
	a.values = map[clientmodel.Fingerprint]Stream{}

	return nil
}

type memoryArenaChunkIterator struct {
	m   *MemoryArena
	fp  *clientmodel.Fingerprint
	fps clientmodel.Fingerprints
}

func (i *memoryArenaChunkIterator) Close() error {
	i.m = nil
	i.fp = nil
	i.fps = nil

	return nil
}

func (i *memoryArenaChunkIterator) HasNext() bool {
	return len(i.fps) > 0
}

func (i *memoryArenaChunkIterator) Next() {
	i.fp, i.fps = i.fps[0], i.fps[1:]
}

func (i *memoryArenaChunkIterator) Get() (fp *clientmodel.Fingerprint, v Values, err error) {
	stream, ok := i.m.values[*i.fp]
	if !ok {
		return i.fp, nil, nil
	}

	return i.fp, stream.Clone(), nil
}

func (a *MemoryArena) GetChunksForTimes(b ChunksForTimesBatch) ChunkIterator {
	it := &memoryArenaChunkIterator{
		m: a,
	}

	for fp := range b {
		it.fps = append(it.fps, &fp)
	}

	sort.Sort(it.fps)

	return it
}

// Forthcoming wrapper for LevelDB / Levigo / You-Name-It
type IterableKeyValueStore interface {
	Put(IngestionBatch) error
	SeekTo(clientmodel.Fingerprint, time.Time)
}

// Mediate key-value store into an arena.
type KeyValueArena struct {
	Arena

	Store IterableKeyValueStore
}
