// Copyright 2017 The Prometheus Authors
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
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
)

var (
	// ErrNotFound is returned if a looked up resource was not found.
	ErrNotFound = errors.Errorf("not found")

	// ErrOutOfOrderSample is returned if an appended sample has a
	// timestamp larger than the most recent sample.
	ErrOutOfOrderSample = errors.New("out of order sample")

	// ErrAmendSample is returned if an appended sample has the same timestamp
	// as the most recent sample but a different value.
	ErrAmendSample = errors.New("amending sample")

	// ErrOutOfBounds is returned if an appended sample is out of the
	// writable time range.
	ErrOutOfBounds = errors.New("out of bounds")
)

// Head handles reads and writes of time series data within a time window.
type Head struct {
	chunkRange int64
	metrics    *headMetrics
	wal        WAL
	logger     log.Logger
	appendPool sync.Pool

	minTime, maxTime int64
	lastSeriesID     uint64

	// All series addressable by their ID or hash.
	series *stripeSeries

	symMtx  sync.RWMutex
	symbols map[string]struct{}
	values  map[string]stringset // label names to possible values

	postings *memPostings // postings lists for terms

	tombstones tombstoneReader
}

type headMetrics struct {
	activeAppenders     prometheus.Gauge
	series              prometheus.Gauge
	seriesCreated       prometheus.Counter
	seriesRemoved       prometheus.Counter
	chunks              prometheus.Gauge
	chunksCreated       prometheus.Gauge
	chunksRemoved       prometheus.Gauge
	gcDuration          prometheus.Summary
	minTime             prometheus.GaugeFunc
	maxTime             prometheus.GaugeFunc
	samplesAppended     prometheus.Counter
	walTruncateDuration prometheus.Summary
}

func newHeadMetrics(h *Head, r prometheus.Registerer) *headMetrics {
	m := &headMetrics{}

	m.activeAppenders = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_active_appenders",
		Help: "Number of currently active appender transactions",
	})
	m.series = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series",
		Help: "Total number of series in the head block.",
	})
	m.seriesCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series_created_total",
		Help: "Total number of series created in the head",
	})
	m.seriesRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series_removed_total",
		Help: "Total number of series removed in the head",
	})
	m.chunks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks",
		Help: "Total number of chunks in the head block.",
	})
	m.chunksCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks_created_total",
		Help: "Total number of chunks created in the head",
	})
	m.chunksRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks_removed_total",
		Help: "Total number of chunks removed in the head",
	})
	m.gcDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_head_gc_duration_seconds",
		Help: "Runtime of garbage collection in the head block.",
	})
	m.minTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsdb_head_max_time",
		Help: "Maximum timestamp of the head block.",
	}, func() float64 {
		return float64(h.MaxTime())
	})
	m.maxTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsdb_head_min_time",
		Help: "Minimum time bound of the head block.",
	}, func() float64 {
		return float64(h.MinTime())
	})
	m.walTruncateDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_wal_truncate_duration_seconds",
		Help: "Duration of WAL truncation.",
	})
	m.samplesAppended = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_head_samples_appended_total",
		Help: "Total number of appended sampledb.",
	})

	if r != nil {
		r.MustRegister(
			m.activeAppenders,
			m.chunks,
			m.chunksCreated,
			m.chunksRemoved,
			m.series,
			m.seriesCreated,
			m.seriesRemoved,
			m.minTime,
			m.maxTime,
			m.gcDuration,
			m.walTruncateDuration,
			m.samplesAppended,
		)
	}
	return m
}

// NewHead opens the head block in dir.
func NewHead(r prometheus.Registerer, l log.Logger, wal WAL, chunkRange int64) (*Head, error) {
	if l == nil {
		l = log.NewNopLogger()
	}
	if wal == nil {
		wal = NopWAL()
	}
	if chunkRange < 1 {
		return nil, errors.Errorf("invalid chunk range %d", chunkRange)
	}
	h := &Head{
		wal:        wal,
		logger:     l,
		chunkRange: chunkRange,
		minTime:    math.MinInt64,
		maxTime:    math.MinInt64,
		series:     newStripeSeries(),
		values:     map[string]stringset{},
		symbols:    map[string]struct{}{},
		postings:   newMemPostings(),
		tombstones: newEmptyTombstoneReader(),
	}
	h.metrics = newHeadMetrics(h, r)

	return h, nil
}

func (h *Head) ReadWAL() error {
	r := h.wal.Reader()
	mint := h.MinTime()

	seriesFunc := func(series []RefSeries) error {
		for _, s := range series {
			h.create(s.Labels.Hash(), s.Labels)
		}
		return nil
	}
	samplesFunc := func(samples []RefSample) error {
		for _, s := range samples {
			if s.T < mint {
				continue
			}
			ms := h.series.getByID(s.Ref)
			if ms == nil {
				return errors.Errorf("unknown series reference %d; abort WAL restore", s.Ref)
			}
			_, chunkCreated := ms.append(s.T, s.V)
			if chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
		}

		return nil
	}
	deletesFunc := func(stones []Stone) error {
		for _, s := range stones {
			for _, itv := range s.intervals {
				if itv.Maxt < mint {
					continue
				}
				h.tombstones.add(s.ref, itv)
			}
		}

		return nil
	}

	if err := r.Read(seriesFunc, samplesFunc, deletesFunc); err != nil {
		return errors.Wrap(err, "consume WAL")
	}
	return nil
}

// Truncate removes all data before mint from the head block and truncates its WAL.
func (h *Head) Truncate(mint int64) error {
	initialize := h.MinTime() == math.MinInt64

	if mint%h.chunkRange != 0 {
		return errors.Errorf("truncating at %d not aligned", mint)
	}
	if h.MinTime() >= mint {
		return nil
	}
	atomic.StoreInt64(&h.minTime, mint)

	// Ensure that max time is at least as high as min time.
	for h.MaxTime() < mint {
		atomic.CompareAndSwapInt64(&h.maxTime, h.MaxTime(), mint)
	}

	// This was an initial call to Truncate after loading blocks on startup.
	// We haven't read back the WAL yet, so do not attempt to truncate it.
	if initialize {
		return nil
	}

	start := time.Now()

	h.gc()
	h.logger.Log("msg", "head GC completed", "duration", time.Since(start))
	h.metrics.gcDuration.Observe(time.Since(start).Seconds())

	start = time.Now()

	p, err := h.indexRange(mint, math.MaxInt64).Postings("", "")
	if err != nil {
		return err
	}

	if err := h.wal.Truncate(mint, p); err == nil {
		h.logger.Log("msg", "WAL truncation completed", "duration", time.Since(start))
	} else {
		h.logger.Log("msg", "WAL truncation failed", "err", err, "duration", time.Since(start))
	}
	h.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())

	return nil
}

// initTime initializes a head with the first timestamp. This only needs to be called
// for a compltely fresh head with an empty WAL.
// Returns true if the initialization took an effect.
func (h *Head) initTime(t int64) (initialized bool) {
	// In the init state, the head has a high timestamp of math.MinInt64.
	mint, _ := rangeForTimestamp(t, h.chunkRange)

	if !atomic.CompareAndSwapInt64(&h.minTime, math.MinInt64, mint) {
		return false
	}
	// Ensure that max time is initialized to at least the min time we just set.
	// Concurrent appenders may already have set it to a higher value.
	atomic.CompareAndSwapInt64(&h.maxTime, math.MinInt64, t)

	return true
}

// initAppender is a helper to initialize the time bounds of a the head
// upon the first sample it receives.
type initAppender struct {
	app  Appender
	head *Head
}

func (a *initAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	if a.app != nil {
		return a.app.Add(lset, t, v)
	}
	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.Add(lset, t, v)
}

func (a *initAppender) AddFast(ref uint64, t int64, v float64) error {
	if a.app == nil {
		return ErrNotFound
	}
	return a.app.AddFast(ref, t, v)
}

func (a *initAppender) Commit() error {
	if a.app == nil {
		return nil
	}
	return a.app.Commit()
}

func (a *initAppender) Rollback() error {
	if a.app == nil {
		return nil
	}
	return a.app.Rollback()
}

// Appender returns a new Appender on the database.
func (h *Head) Appender() Appender {
	h.metrics.activeAppenders.Inc()

	// The head cache might not have a starting point yet. The init appender
	// picks up the first appended timestamp as the base.
	if h.MinTime() == math.MinInt64 {
		return &initAppender{head: h}
	}
	return h.appender()
}

func (h *Head) appender() *headAppender {
	return &headAppender{
		head:          h,
		mint:          h.MaxTime() - h.chunkRange/2,
		samples:       h.getAppendBuffer(),
		highTimestamp: math.MinInt64,
	}
}

func (h *Head) getAppendBuffer() []RefSample {
	b := h.appendPool.Get()
	if b == nil {
		return make([]RefSample, 0, 512)
	}
	return b.([]RefSample)
}

func (h *Head) putAppendBuffer(b []RefSample) {
	h.appendPool.Put(b[:0])
}

type headAppender struct {
	head *Head
	mint int64

	series        []RefSeries
	samples       []RefSample
	highTimestamp int64
}

func (a *headAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	if t < a.mint {
		return 0, ErrOutOfBounds
	}
	hash := lset.Hash()

	s := a.head.series.getByHash(hash, lset)

	if s == nil {
		s = a.head.create(hash, lset)

		a.series = append(a.series, RefSeries{
			Ref:    s.ref,
			Labels: lset,
			hash:   hash,
		})
	}
	return s.ref, a.AddFast(s.ref, t, v)
}

func (a *headAppender) AddFast(ref uint64, t int64, v float64) error {
	s := a.head.series.getByID(ref)

	if s == nil {
		return errors.Wrap(ErrNotFound, "unknown series")
	}
	s.Lock()
	err := s.appendable(t, v)
	s.Unlock()

	if err != nil {
		return err
	}
	if t < a.mint {
		return ErrOutOfBounds
	}
	if t > a.highTimestamp {
		a.highTimestamp = t
	}

	a.samples = append(a.samples, RefSample{
		Ref:    ref,
		T:      t,
		V:      v,
		series: s,
	})
	return nil
}

func (a *headAppender) Commit() error {
	defer a.Rollback()

	if err := a.head.wal.LogSeries(a.series); err != nil {
		return err
	}
	if err := a.head.wal.LogSamples(a.samples); err != nil {
		return errors.Wrap(err, "WAL log samples")
	}

	total := len(a.samples)

	for _, s := range a.samples {
		s.series.Lock()
		ok, chunkCreated := s.series.append(s.T, s.V)
		s.series.Unlock()

		if !ok {
			total--
		}
		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}
	}

	a.head.metrics.samplesAppended.Add(float64(total))

	for {
		ht := a.head.MaxTime()
		if a.highTimestamp <= ht {
			break
		}
		if atomic.CompareAndSwapInt64(&a.head.maxTime, ht, a.highTimestamp) {
			break
		}
	}

	return nil
}

func (a *headAppender) Rollback() error {
	a.head.metrics.activeAppenders.Dec()
	a.head.putAppendBuffer(a.samples)

	return nil
}

// Delete all samples in the range of [mint, maxt] for series that satisfy the given
// label matchers.
func (h *Head) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	// Do not delete anything beyond the currently valid range.
	mint, maxt = clampInterval(mint, maxt, h.MinTime(), h.MaxTime())

	ir := h.indexRange(mint, maxt)

	pr := newPostingsReader(ir)
	p, absent := pr.Select(ms...)

	var stones []Stone

Outer:
	for p.Next() {
		series := h.series.getByID(p.At())

		for _, abs := range absent {
			if series.lset.Get(abs) != "" {
				continue Outer
			}
		}

		// Delete only until the current values and not beyond.
		t0, t1 := clampInterval(mint, maxt, series.minTime(), series.maxTime())
		stones = append(stones, Stone{p.At(), Intervals{{t0, t1}}})
	}

	if p.Err() != nil {
		return p.Err()
	}
	if err := h.wal.LogDeletes(stones); err != nil {
		return err
	}
	for _, s := range stones {
		h.tombstones.add(s.ref, s.intervals[0])
	}
	return nil
}

// gc removes data before the minimum timestmap from the head.
func (h *Head) gc() {
	// Only data strictly lower than this timestamp must be deleted.
	mint := h.MinTime()

	// Drop old chunks and remember series IDs and hashes if they can be
	// deleted entirely.
	deleted, chunksRemoved := h.series.gc(mint)
	seriesRemoved := len(deleted)

	h.metrics.seriesRemoved.Add(float64(seriesRemoved))
	h.metrics.series.Sub(float64(seriesRemoved))
	h.metrics.chunksRemoved.Add(float64(chunksRemoved))
	h.metrics.chunks.Sub(float64(chunksRemoved))

	// Remove deleted series IDs from the postings lists. First do a collection
	// run where we rebuild all postings that have something to delete
	h.postings.mtx.RLock()

	type replEntry struct {
		idx int
		l   []uint64
	}
	collected := map[labels.Label]replEntry{}

	for t, p := range h.postings.m {
		repl := replEntry{idx: len(p)}

		for i, id := range p {
			if _, ok := deleted[id]; ok {
				// First ID that got deleted, initialize replacement with
				// all remaining IDs so far.
				if repl.l == nil {
					repl.l = make([]uint64, 0, len(p))
					repl.l = append(repl.l, p[:i]...)
				}
				continue
			}
			// Only add to the replacement once we know we have to do it.
			if repl.l != nil {
				repl.l = append(repl.l, id)
			}
		}
		if repl.l != nil {
			collected[t] = repl
		}
	}

	h.postings.mtx.RUnlock()

	// Replace all postings that have changed. Append all IDs that may have
	// been added while we switched locks.
	h.postings.mtx.Lock()

	for t, repl := range collected {
		l := append(repl.l, h.postings.m[t][repl.idx:]...)

		if len(l) > 0 {
			h.postings.m[t] = l
		} else {
			delete(h.postings.m, t)
		}
	}

	h.postings.mtx.Unlock()

	// Rebuild symbols and label value indices from what is left in the postings terms.
	h.postings.mtx.RLock()

	symbols := make(map[string]struct{}, len(h.symbols))
	values := make(map[string]stringset, len(h.values))

	for t := range h.postings.m {
		symbols[t.Name] = struct{}{}
		symbols[t.Value] = struct{}{}

		ss, ok := values[t.Name]
		if !ok {
			ss = stringset{}
			values[t.Name] = ss
		}
		ss.set(t.Value)
	}

	h.postings.mtx.RUnlock()

	h.symMtx.Lock()

	h.symbols = symbols
	h.values = values

	h.symMtx.Unlock()
}

func (h *Head) Tombstones() TombstoneReader {
	return h.tombstones
}

// Index returns an IndexReader against the block.
func (h *Head) Index() IndexReader {
	return h.indexRange(math.MinInt64, math.MaxInt64)
}

func (h *Head) indexRange(mint, maxt int64) *headIndexReader {
	if hmin := h.MinTime(); hmin > mint {
		mint = hmin
	}
	return &headIndexReader{head: h, mint: mint, maxt: maxt}
}

// Chunks returns a ChunkReader against the block.
func (h *Head) Chunks() ChunkReader {
	return h.chunksRange(math.MinInt64, math.MaxInt64)
}

func (h *Head) chunksRange(mint, maxt int64) *headChunkReader {
	if hmin := h.MinTime(); hmin > mint {
		mint = hmin
	}
	return &headChunkReader{head: h, mint: mint, maxt: maxt}
}

// MinTime returns the lowest time bound on visible data in the head.
func (h *Head) MinTime() int64 {
	return atomic.LoadInt64(&h.minTime)
}

// MaxTime returns the highest timestamp seen in data of the head.
func (h *Head) MaxTime() int64 {
	return atomic.LoadInt64(&h.maxTime)
}

type headChunkReader struct {
	head       *Head
	mint, maxt int64
}

func (h *headChunkReader) Close() error {
	return nil
}

// packChunkID packs a seriesID and a chunkID within it into a global 8 byte ID.
// It panicks if the seriesID exceeds 5 bytes or the chunk ID 3 bytes.
func packChunkID(seriesID, chunkID uint64) uint64 {
	if seriesID > (1<<40)-1 {
		panic("series ID exceeds 5 bytes")
	}
	if chunkID > (1<<24)-1 {
		panic("chunk ID exceeds 3 bytes")
	}
	return (seriesID << 24) | chunkID
}

func unpackChunkID(id uint64) (seriesID, chunkID uint64) {
	return id >> 24, (id << 40) >> 40
}

// Chunk returns the chunk for the reference number.
func (h *headChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	sid, cid := unpackChunkID(ref)

	s := h.head.series.getByID(sid)

	s.Lock()
	c := s.chunk(int(cid))
	s.Unlock()

	// Do not expose chunks that are outside of the specified range.
	if c == nil || !intervalOverlap(c.minTime, c.maxTime, h.mint, h.maxt) {
		return nil, ErrNotFound
	}
	return &safeChunk{
		Chunk: c.chunk,
		s:     s,
		cid:   int(cid),
	}, nil
}

type safeChunk struct {
	chunks.Chunk
	s   *memSeries
	cid int
}

func (c *safeChunk) Iterator() chunks.Iterator {
	c.s.Lock()
	it := c.s.iterator(c.cid)
	c.s.Unlock()
	return it
}

// func (c *safeChunk) Appender() (chunks.Appender, error) { panic("illegal") }
// func (c *safeChunk) Bytes() []byte                      { panic("illegal") }
// func (c *safeChunk) Encoding() chunks.Encoding          { panic("illegal") }

type rangeHead struct {
	head       *Head
	mint, maxt int64
}

func (h *rangeHead) Index() IndexReader {
	return h.head.indexRange(h.mint, h.maxt)
}

func (h *rangeHead) Chunks() ChunkReader {
	return h.head.chunksRange(h.mint, h.maxt)
}

func (h *rangeHead) Tombstones() TombstoneReader {
	return newEmptyTombstoneReader()
}

type headIndexReader struct {
	head       *Head
	mint, maxt int64
}

func (h *headIndexReader) Close() error {
	return nil
}

func (h *headIndexReader) Symbols() (map[string]struct{}, error) {
	h.head.symMtx.RLock()
	defer h.head.symMtx.RUnlock()

	res := make(map[string]struct{}, len(h.head.symbols))

	for s := range h.head.symbols {
		res[s] = struct{}{}
	}
	return res, nil
}

// LabelValues returns the possible label values
func (h *headIndexReader) LabelValues(names ...string) (StringTuples, error) {
	if len(names) != 1 {
		return nil, errInvalidSize
	}
	var sl []string

	h.head.symMtx.RLock()
	defer h.head.symMtx.RUnlock()

	for s := range h.head.values[names[0]] {
		sl = append(sl, s)
	}
	sort.Strings(sl)

	return &stringTuples{l: len(names), s: sl}, nil
}

// Postings returns the postings list iterator for the label pair.
func (h *headIndexReader) Postings(name, value string) (Postings, error) {
	return h.head.postings.get(name, value), nil
}

func (h *headIndexReader) SortedPostings(p Postings) Postings {
	ep := make([]uint64, 0, 128)

	for p.Next() {
		ep = append(ep, p.At())
	}
	if err := p.Err(); err != nil {
		return errPostings{err: errors.Wrap(err, "expand postings")}
	}
	var err error

	sort.Slice(ep, func(i, j int) bool {
		if err != nil {
			return false
		}
		a := h.head.series.getByID(ep[i])
		b := h.head.series.getByID(ep[j])

		if a == nil || b == nil {
			err = errors.Errorf("series not found")
			return false
		}
		return labels.Compare(a.lset, b.lset) < 0
	})
	if err != nil {
		return errPostings{err: err}
	}
	return newListPostings(ep)
}

// Series returns the series for the given reference.
func (h *headIndexReader) Series(ref uint64, lbls *labels.Labels, chks *[]ChunkMeta) error {
	s := h.head.series.getByID(ref)

	if s == nil {
		return ErrNotFound
	}
	*lbls = append((*lbls)[:0], s.lset...)

	s.Lock()
	defer s.Unlock()

	*chks = (*chks)[:0]

	for i, c := range s.chunks {
		// Do not expose chunks that are outside of the specified range.
		if !intervalOverlap(c.minTime, c.maxTime, h.mint, h.maxt) {
			continue
		}
		*chks = append(*chks, ChunkMeta{
			MinTime: c.minTime,
			MaxTime: c.maxTime,
			Ref:     packChunkID(s.ref, uint64(s.chunkID(i))),
		})
	}

	return nil
}

func (h *headIndexReader) LabelIndices() ([][]string, error) {
	h.head.symMtx.RLock()
	defer h.head.symMtx.RUnlock()

	res := [][]string{}

	for s := range h.head.values {
		res = append(res, []string{s})
	}
	return res, nil
}

func (h *Head) create(hash uint64, lset labels.Labels) *memSeries {
	h.metrics.series.Inc()
	h.metrics.seriesCreated.Inc()

	// Optimistically assume that we are the first one to create the series.
	id := atomic.AddUint64(&h.lastSeriesID, 1)
	s := newMemSeries(lset, id, h.chunkRange)

	s, created := h.series.getOrSet(hash, s)
	// Skip indexing if we didn't actually create the series.
	if !created {
		return s
	}

	h.postings.add(id, lset)

	h.symMtx.Lock()
	defer h.symMtx.Unlock()

	for _, l := range lset {
		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)

		h.symbols[l.Name] = struct{}{}
		h.symbols[l.Value] = struct{}{}
	}

	return s
}

// seriesHashmap is a simple hashmap for memSeries by their label set. It is built
// on top of a regular hashmap and holds a slice of series to resolve hash collisions.
// Its methods require the hash to be submitted with it to avoid re-computations throughout
// the code.
type seriesHashmap map[uint64][]*memSeries

func (m seriesHashmap) get(hash uint64, lset labels.Labels) *memSeries {
	for _, s := range m[hash] {
		if s.lset.Equals(lset) {
			return s
		}
	}
	return nil
}

func (m seriesHashmap) set(hash uint64, s *memSeries) {
	l := m[hash]
	for i, prev := range l {
		if prev.lset.Equals(s.lset) {
			l[i] = s
			return
		}
	}
	m[hash] = append(l, s)
}

func (m seriesHashmap) del(hash uint64, lset labels.Labels) {
	var rem []*memSeries
	for _, s := range m[hash] {
		if !s.lset.Equals(lset) {
			rem = append(rem, s)
		}
	}
	if len(rem) == 0 {
		delete(m, hash)
	} else {
		m[hash] = rem
	}
}

// stripeSeries locks modulo ranges of IDs and hashes to reduce lock contention.
// The locks are padded to not be on the same cache line. Filling the badded space
// with the maps was profiled to be slower – likely due to the additional pointer
// dereferences.
type stripeSeries struct {
	series [stripeSize]map[uint64]*memSeries
	hashes [stripeSize]seriesHashmap
	locks  [stripeSize]stripeLock
}

const (
	stripeSize = 1 << 14
	stripeMask = stripeSize - 1
)

type stripeLock struct {
	sync.RWMutex
	// Padding to avoid multiple locks being on the same cache line.
	_ [40]byte
}

func newStripeSeries() *stripeSeries {
	s := &stripeSeries{}

	for i := range s.series {
		s.series[i] = map[uint64]*memSeries{}
	}
	for i := range s.hashes {
		s.hashes[i] = seriesHashmap{}
	}
	return s
}

// gc garbage collects old chunks that are strictly before mint and removes
// series entirely that have no chunks left.
func (s *stripeSeries) gc(mint int64) (map[uint64]struct{}, int) {
	var (
		deleted  = map[uint64]struct{}{}
		rmChunks = 0
	)
	// Run through all series and truncate old chunks. Mark those with no
	// chunks left as deleted and store their ID.
	for i := 0; i < stripeSize; i++ {
		s.locks[i].Lock()

		for hash, all := range s.hashes[i] {
			for _, series := range all {
				series.Lock()
				rmChunks += series.truncateChunksBefore(mint)

				if len(series.chunks) > 0 {
					series.Unlock()
					continue
				}

				// The series is gone entirely. We need to keep the series lock
				// and make sure we have acquired the stripe locks for hash and ID of the
				// series alike.
				// If we don't hold them all, there's a very small chance that a series receives
				// samples again while we are half-way into deleting it.
				j := int(series.ref & stripeMask)

				if i != j {
					s.locks[j].Lock()
				}

				deleted[series.ref] = struct{}{}
				s.hashes[i].del(hash, series.lset)
				delete(s.series[j], series.ref)

				if i != j {
					s.locks[j].Unlock()
				}

				series.Unlock()
			}
		}

		s.locks[i].Unlock()
	}

	return deleted, rmChunks
}

func (s *stripeSeries) getByID(id uint64) *memSeries {
	i := id & stripeMask

	s.locks[i].RLock()
	series := s.series[i][id]
	s.locks[i].RUnlock()

	return series
}

func (s *stripeSeries) getByHash(hash uint64, lset labels.Labels) *memSeries {
	i := hash & stripeMask

	s.locks[i].RLock()
	series := s.hashes[i].get(hash, lset)
	s.locks[i].RUnlock()

	return series
}

func (s *stripeSeries) getOrSet(hash uint64, series *memSeries) (*memSeries, bool) {
	i := hash & stripeMask

	s.locks[i].Lock()

	if prev := s.hashes[i].get(hash, series.lset); prev != nil {
		s.locks[i].Unlock()
		return prev, false
	}
	s.hashes[i].set(hash, series)

	s.hashes[i][hash] = append(s.hashes[i][hash], series)
	s.locks[i].Unlock()

	i = series.ref & stripeMask

	s.locks[i].Lock()
	s.series[i][series.ref] = series
	s.locks[i].Unlock()

	return series, true
}

type sample struct {
	t int64
	v float64
}

// memSeries is the in-memory representation of a series. None of its methods
// are goroutine safe and its the callers responsibility to lock it.
type memSeries struct {
	sync.Mutex

	ref          uint64
	lset         labels.Labels
	chunks       []*memChunk
	chunkRange   int64
	firstChunkID int

	nextAt    int64 // timestamp at which to cut the next chunk.
	lastValue float64
	sampleBuf [4]sample

	app chunks.Appender // Current appender for the chunk.
}

func (s *memSeries) minTime() int64 {
	return s.chunks[0].minTime
}

func (s *memSeries) maxTime() int64 {
	return s.head().maxTime
}

func (s *memSeries) cut(mint int64) *memChunk {
	c := &memChunk{
		chunk:   chunks.NewXORChunk(),
		minTime: mint,
		maxTime: math.MinInt64,
	}
	s.chunks = append(s.chunks, c)

	app, err := c.chunk.Appender()
	if err != nil {
		panic(err)
	}
	s.app = app
	return c
}

func newMemSeries(lset labels.Labels, id uint64, chunkRange int64) *memSeries {
	s := &memSeries{
		lset:       lset,
		ref:        id,
		chunkRange: chunkRange,
		nextAt:     math.MinInt64,
	}
	return s
}

// appendable checks whether the given sample is valid for appending to the series.
func (s *memSeries) appendable(t int64, v float64) error {
	c := s.head()
	if c == nil {
		return nil
	}

	if t > c.maxTime {
		return nil
	}
	if t < c.maxTime {
		return ErrOutOfOrderSample
	}
	// We are allowing exact duplicates as we can encounter them in valid cases
	// like federation and erroring out at that time would be extremely noisy.
	if math.Float64bits(s.lastValue) != math.Float64bits(v) {
		return ErrAmendSample
	}
	return nil
}

func (s *memSeries) chunk(id int) *memChunk {
	ix := id - s.firstChunkID
	if ix < 0 || ix >= len(s.chunks) {
		return nil
	}
	return s.chunks[ix]
}

func (s *memSeries) chunkID(pos int) int {
	return pos + s.firstChunkID
}

// truncateChunksBefore removes all chunks from the series that have not timestamp
// at or after mint. Chunk IDs remain unchanged.
func (s *memSeries) truncateChunksBefore(mint int64) (removed int) {
	var k int
	for i, c := range s.chunks {
		if c.maxTime >= mint {
			break
		}
		k = i + 1
	}
	s.chunks = append(s.chunks[:0], s.chunks[k:]...)
	s.firstChunkID += k

	return k
}

// append adds the sample (t, v) to the series.
func (s *memSeries) append(t int64, v float64) (success, chunkCreated bool) {
	const samplesPerChunk = 120

	c := s.head()

	if c == nil {
		c = s.cut(t)
		chunkCreated = true
	}
	if c.maxTime >= t {
		return false, chunkCreated
	}
	if c.chunk.NumSamples() > samplesPerChunk/4 && t >= s.nextAt {
		c = s.cut(t)
		chunkCreated = true
	}
	s.app.Append(t, v)

	c.maxTime = t

	if c.chunk.NumSamples() == samplesPerChunk/4 {
		_, maxt := rangeForTimestamp(c.minTime, s.chunkRange)
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, maxt)
	}

	s.lastValue = v

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, v: v}

	return true, chunkCreated
}

// computeChunkEndTime estimates the end timestamp based the beginning of a chunk,
// its current timestamp and the upper bound up to which we insert data.
// It assumes that the time range is 1/4 full.
func computeChunkEndTime(start, cur, max int64) int64 {
	a := (max - start) / ((cur - start + 1) * 4)
	if a == 0 {
		return max
	}
	return start + (max-start)/a
}

func (s *memSeries) iterator(id int) chunks.Iterator {
	c := s.chunk(id)

	if id-s.firstChunkID < len(s.chunks)-1 {
		return c.chunk.Iterator()
	}
	// Serve the last 4 samples for the last chunk from the series buffer
	// as their compressed bytes may be mutated by added samples.
	it := &memSafeIterator{
		Iterator: c.chunk.Iterator(),
		i:        -1,
		total:    c.chunk.NumSamples(),
		buf:      s.sampleBuf,
	}
	return it
}

func (s *memSeries) head() *memChunk {
	if len(s.chunks) == 0 {
		return nil
	}
	return s.chunks[len(s.chunks)-1]
}

type memChunk struct {
	chunk            chunks.Chunk
	minTime, maxTime int64
}

type memSafeIterator struct {
	chunks.Iterator

	i     int
	total int
	buf   [4]sample
}

func (it *memSafeIterator) Next() bool {
	if it.i+1 >= it.total {
		return false
	}
	it.i++
	if it.total-it.i > 4 {
		return it.Iterator.Next()
	}
	return true
}

func (it *memSafeIterator) At() (int64, float64) {
	if it.total-it.i > 4 {
		return it.Iterator.At()
	}
	s := it.buf[4-(it.total-it.i)]
	return s.t, s.v
}
