// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"encoding/binary"
	"path/filepath"
	"sync"
	"time"

	"github.com/fabxc/tsdb/chunks"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

// DefaultOptions used for the DB.
var DefaultOptions = &Options{
	StalenessDelta: 5 * time.Minute,
}

// Options of the DB storage.
type Options struct {
	StalenessDelta time.Duration
}

// DB is a time series storage.
type DB struct {
	logger log.Logger
	opts   *Options

	memChunks   *memChunks
	persistence *persistence
	indexer     *indexer
	stopc       chan struct{}
}

// Open or create a new DB.
func Open(path string, l log.Logger, opts *Options) (*DB, error) {
	if opts == nil {
		opts = DefaultOptions
	}

	indexer, err := newMetricIndexer(filepath.Join(path, "index"), defaultIndexerQsize, defaultIndexerTimeout)
	if err != nil {
		return nil, err
	}
	persistence, err := newPersistence(filepath.Join(path, "chunks"), defaultIndexerQsize, defaultIndexerTimeout)
	if err != nil {
		return nil, err
	}

	mchunks := newMemChunks(l, indexer, persistence, 10, opts.StalenessDelta)
	indexer.mc = mchunks
	persistence.mc = mchunks

	c := &DB{
		logger:      l,
		opts:        opts,
		memChunks:   mchunks,
		persistence: persistence,
		indexer:     indexer,
		stopc:       make(chan struct{}),
	}
	go c.memChunks.run(c.stopc)

	return c, nil
}

// Close the storage and persist all writes.
func (c *DB) Close() error {
	close(c.stopc)
	// TODO(fabxc): blocking further writes here necessary?
	c.indexer.wait()
	c.persistence.wait()

	err0 := c.indexer.close()
	err1 := c.persistence.close()
	if err0 != nil {
		return err0
	}
	return err1
}

// Append ingestes the samples in the scrape into the storage.
func (c *DB) Append(scrape *Scrape) error {
	// Sequentially add samples to in-memory chunks.
	// TODO(fabxc): evaluate cost of making this atomic.
	for _, s := range scrape.m {
		if err := c.memChunks.append(s.met, scrape.ts, s.val); err != nil {
			// TODO(fabxc): collect in multi error.
			return err
		}
		// TODO(fabxc): increment ingested samples metric.
	}
	return nil
}

// memChunks holds the chunks that are currently being appended to.
type memChunks struct {
	logger         log.Logger
	stalenessDelta time.Duration

	mtx sync.RWMutex
	// Chunks by their ID as accessed when retrieving a chunk ID from
	// an index query.
	chunks map[ChunkID]*chunkDesc
	// The highest time slice chunks currently have. A new chunk can not
	// be in a higher slice before all chunks with lower IDs have been
	// added to the slice.
	highTime model.Time

	// Power of 2 of chunk shards.
	num uint8
	// Memory chunks sharded by leading bits of the chunk's metric's
	// fingerprints. Used to quickly find chunks for new incoming samples
	// where the metric is known but the chunk ID is not.
	shards []*memChunksShard

	indexer     *indexer
	persistence *persistence
}

// newMemChunks returns a new memChunks sharded by n locks.
func newMemChunks(l log.Logger, ix *indexer, p *persistence, n uint8, staleness time.Duration) *memChunks {
	c := &memChunks{
		logger:         l,
		stalenessDelta: staleness,
		num:            n,
		chunks:         map[ChunkID]*chunkDesc{},
		persistence:    p,
		indexer:        ix,
	}

	if n > 63 {
		panic("invalid shard power")
	}

	// Initialize 2^n shards.
	for i := 0; i < 1<<n; i++ {
		c.shards = append(c.shards, &memChunksShard{
			descs: map[model.Fingerprint][]*chunkDesc{},
			csize: 1024,
		})
	}
	return c
}

func (mc *memChunks) run(stopc <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	f := func() error {
		for _, cs := range mc.shards {
			mc.gc(cs)
		}
		// Wait for persistence and indexing to finish before reindexing
		// memory chunks for the new time slice.
		mc.persistence.wait()
		mc.indexer.wait()

		mc.mtx.Lock()
		defer mc.mtx.Unlock()

		curTimeSlice := timeSlice(model.Now())
		// If the next time slice is in the future, we are done.
		if curTimeSlice <= mc.highTime {
			return nil
		}

		ids := make(ChunkIDs, 0, len(mc.chunks))
		for id := range mc.chunks {
			ids = append(ids, id)
		}

		if err := mc.indexer.reindexTime(ids, curTimeSlice); err != nil {
			return err
		}
		mc.highTime = curTimeSlice
		return nil
	}

	for {
		select {
		case <-ticker.C:
			if err := f(); err != nil {
				mc.logger.With("err", err).Error("memory chunk maintenance failed")
			}
		case <-stopc:
			return
		}
	}
}

// gc writes stale and incomplete chunks to persistence and removes them
// from the shard.
func (mc *memChunks) gc(cs *memChunksShard) {
	cs.RLock()
	defer cs.RUnlock()

	mint := model.Now().Add(-mc.stalenessDelta)

	for fp, cdescs := range cs.descs {
		for _, cd := range cdescs {
			// If the last sample was added before the staleness delta, consider
			// the chunk inactive and persist it.
			if cd.lastSample.Timestamp.Before(mint) {
				mc.persistence.enqueue(cd)
				cs.del(fp, cd)
			}
		}
	}
	return
}

func (mc *memChunks) append(m model.Metric, ts model.Time, v model.SampleValue) error {
	fp := m.FastFingerprint()
	cs := mc.shards[fp>>(64-mc.num)]

	cs.Lock()
	defer cs.Unlock()

	chkd, created := cs.get(fp, m)
	if created {
		mc.indexer.enqueue(chkd)
	}
	if err := chkd.append(ts, v); err != chunks.ErrChunkFull {
		return err
	}
	// Chunk was full, remove it so a new head chunk can be created.
	// TODO(fabxc): should we just remove them during maintenance if we set a 'persisted'
	// flag?
	// If we shutdown we work down the persistence queue before exiting, so we should
	// lose no data. If we crash, the last snapshot will still have the chunk. Theoretically,
	// deleting it here should not be a problem.
	cs.del(fp, chkd)

	mc.persistence.enqueue(chkd)

	// Create a new chunk lazily and continue.
	chkd, created = cs.get(fp, m)
	if !created {
		// Bug if the chunk was not newly created.
		panic("expected newly created chunk")
	}
	mc.indexer.enqueue(chkd)

	return chkd.append(ts, v)
}

type memChunksShard struct {
	sync.RWMutex

	// chunks holds chunk descriptors for one or more chunks
	// with a given fingerprint.
	descs map[model.Fingerprint][]*chunkDesc
	csize int
}

// get returns the chunk descriptor for the given fingerprint/metric combination.
// If none exists, a new chunk descriptor is created and true is returned.
func (cs *memChunksShard) get(fp model.Fingerprint, m model.Metric) (*chunkDesc, bool) {
	chks := cs.descs[fp]
	for _, cd := range chks {
		if cd != nil && cd.met.Equal(m) {
			return cd, false
		}
	}
	// None of the given chunks was for the metric, create a new one.
	cd := &chunkDesc{
		met:   m,
		chunk: chunks.NewPlainChunk(cs.csize),
	}
	// Try inserting chunk in existing whole before appending.
	for i, c := range chks {
		if c == nil {
			chks[i] = cd
			return cd, true
		}
	}
	cs.descs[fp] = append(chks, cd)
	return cd, true
}

// del frees the field of the chunk descriptor for the fingerprint.
func (cs *memChunksShard) del(fp model.Fingerprint, chkd *chunkDesc) {
	for i, d := range cs.descs[fp] {
		if d == chkd {
			cs.descs[fp][i] = nil
			return
		}
	}
}

// ChunkID is a unique identifier for a chunks.
type ChunkID uint64

func (id ChunkID) bytes() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

// ChunkIDs is a sortable list of chunk IDs.
type ChunkIDs []ChunkID

func (c ChunkIDs) Len() int           { return len(c) }
func (c ChunkIDs) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c ChunkIDs) Less(i, j int) bool { return c[i] < c[j] }

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	id    ChunkID
	met   model.Metric
	chunk chunks.Chunk

	// Caching fields.
	firstTime  model.Time
	lastSample model.SamplePair

	app chunks.Appender // Current appender for the chunks.
}

func (cd *chunkDesc) append(ts model.Time, v model.SampleValue) error {
	if cd.app == nil {
		cd.app = cd.chunk.Appender()
		// TODO(fabxc): set correctly once loading from snapshot is added.
		cd.firstTime = ts
	}
	cd.lastSample.Timestamp = ts
	cd.lastSample.Value = v

	return cd.app.Append(ts, v)
}

// Scrape gathers samples for a single timestamp.
type Scrape struct {
	ts model.Time
	m  []sample
}

type sample struct {
	met model.Metric
	val model.SampleValue
}

// Reset resets the scrape data and initializes it for a new scrape at
// the given time. The underlying memory remains allocated for the next scrape.
func (s *Scrape) Reset(ts model.Time) {
	s.ts = ts
	s.m = s.m[:0]
}

// Dump returns all samples that are part of the scrape.
func (s *Scrape) Dump() []*model.Sample {
	d := make([]*model.Sample, 0, len(s.m))
	for _, sa := range s.m {
		d = append(d, &model.Sample{
			Metric:    sa.met,
			Timestamp: s.ts,
			Value:     sa.val,
		})
	}
	return d
}

// Add adds a sample value for the given metric to the scrape.
func (s *Scrape) Add(m model.Metric, v model.SampleValue) {
	for ln, lv := range m {
		if len(lv) == 0 {
			delete(m, ln)
		}
	}
	// TODO(fabxc): pre-sort added samples into the correct buckets
	// of fingerprint shards so we only have to lock each memChunkShard once.
	s.m = append(s.m, sample{met: m, val: v})
}

type chunkBatchProcessor struct {
	processf func(...*chunkDesc) error

	mtx    sync.RWMutex
	logger log.Logger
	q      []*chunkDesc

	qcap    int
	timeout time.Duration

	timer   *time.Timer
	trigger chan struct{}
	empty   chan struct{}
}

func newChunkBatchProcessor(l log.Logger, cap int, to time.Duration) *chunkBatchProcessor {
	if l == nil {
		l = log.NewNopLogger()
	}
	p := &chunkBatchProcessor{
		logger:  l,
		qcap:    cap,
		timeout: to,
		timer:   time.NewTimer(to),
		trigger: make(chan struct{}, 1),
		empty:   make(chan struct{}),
	}
	// Start with closed channel so we don't block on wait if nothing
	// has ever been indexed.
	close(p.empty)

	go p.run()
	return p
}

func (p *chunkBatchProcessor) run() {
	for {
		// Process pending indexing batch if triggered
		// or timeout since last indexing has passed.
		select {
		case <-p.trigger:
		case <-p.timer.C:
		}

		if err := p.process(); err != nil {
			p.logger.
				With("err", err).With("num", len(p.q)).
				Error("batch failed, dropping chunks descs")
		}
	}
}

func (p *chunkBatchProcessor) process() error {
	// TODO(fabxc): locking the entire time will cause lock contention.
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if len(p.q) == 0 {
		return nil
	}
	// Leave chunk descs behind whether successful or not.
	defer func() {
		p.q = p.q[:0]
		close(p.empty)
	}()

	return p.processf(p.q...)
}

func (p *chunkBatchProcessor) enqueue(cds ...*chunkDesc) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if len(p.q) == 0 {
		p.timer.Reset(p.timeout)
		p.empty = make(chan struct{})
	}

	p.q = append(p.q, cds...)
	if len(p.q) > p.qcap {
		select {
		case p.trigger <- struct{}{}:
		default:
			// If we cannot send a signal is already set.
		}
	}
}

// wait blocks until the queue becomes empty.
func (p *chunkBatchProcessor) wait() {
	p.mtx.RLock()
	c := p.empty
	p.mtx.RUnlock()
	<-c
}
