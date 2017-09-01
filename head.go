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
	"encoding/binary"
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
	mtx        sync.RWMutex
	metrics    *headMetrics
	wal        WAL
	logger     log.Logger
	appendPool sync.Pool

	minTime, maxTime int64
	lastSeriesID     uint32

	// descs holds all chunk descs for the head block. Each chunk implicitly
	// is assigned the index as its ID.
	series map[uint32]*memSeries
	// hashes contains a collision map of label set hashes of chunks
	// to their chunk descs.
	hashes map[uint64][]*memSeries

	symbols  map[string]struct{}
	values   map[string]stringset // label names to possible values
	postings *memPostings         // postings lists for terms

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
		wal = NopWAL{}
	}
	if chunkRange < 1 {
		return nil, errors.Errorf("invalid chunk range %d", chunkRange)
	}
	h := &Head{
		wal:        wal,
		logger:     l,
		chunkRange: chunkRange,
		minTime:    math.MaxInt64,
		maxTime:    math.MinInt64,
		series:     map[uint32]*memSeries{},
		hashes:     map[uint64][]*memSeries{},
		values:     map[string]stringset{},
		symbols:    map[string]struct{}{},
		postings:   &memPostings{m: make(map[term][]uint32)},
		tombstones: newEmptyTombstoneReader(),
	}
	h.metrics = newHeadMetrics(h, r)

	return h, h.readWAL()
}

func (h *Head) readWAL() error {
	r := h.wal.Reader(h.MinTime())

	seriesFunc := func(series []labels.Labels) error {
		for _, lset := range series {
			h.create(lset.Hash(), lset)
		}
		return nil
	}
	samplesFunc := func(samples []RefSample) error {
		for _, s := range samples {
			ms, ok := h.series[uint32(s.Ref)]
			if !ok {
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

func (h *Head) String() string {
	return "<head>"
}

// Truncate removes all data before mint from the head block and truncates its WAL.
func (h *Head) Truncate(mint int64) {
	if h.minTime >= mint {
		return
	}
	atomic.StoreInt64(&h.minTime, mint)

	start := time.Now()

	h.gc()
	h.logger.Log("msg", "head GC completed", "duration", time.Since(start))
	h.metrics.gcDuration.Observe(time.Since(start).Seconds())

	start = time.Now()

	if err := h.wal.Truncate(mint); err == nil {
		h.logger.Log("msg", "WAL truncation completed", "duration", time.Since(start))
	} else {
		h.logger.Log("msg", "WAL truncation failed", "err", err, "duration", time.Since(start))
	}
	h.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())
}

// initTime initializes a head with the first timestamp. This only needs to be called
// for a compltely fresh head with an empty WAL.
// Returns true if the initialization took an effect.
func (h *Head) initTime(t int64) (initialized bool) {
	// In the init state, the head has a high timestamp of math.MinInt64.
	if h.MaxTime() != math.MinInt64 {
		return false
	}
	mint, _ := rangeForTimestamp(t, h.chunkRange)

	if !atomic.CompareAndSwapInt64(&h.maxTime, math.MinInt64, t) {
		return false
	}
	atomic.StoreInt64(&h.minTime, mint-h.chunkRange)
	return true
}

// initAppender is a helper to initialize the time bounds of a the head
// upon the first sample it receives.
type initAppender struct {
	app  Appender
	head *Head
}

func (a *initAppender) Add(lset labels.Labels, t int64, v float64) (string, error) {
	if a.app != nil {
		return a.app.Add(lset, t, v)
	}
	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.Add(lset, t, v)
}

func (a *initAppender) AddFast(ref string, t int64, v float64) error {
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
	if h.MaxTime() == math.MinInt64 {
		return &initAppender{head: h}
	}
	return h.appender()
}

func (h *Head) appender() *headAppender {
	h.mtx.RLock()

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

	newSeries []*hashedLabels
	newLabels []labels.Labels
	newHashes map[uint64]uint64

	samples       []RefSample
	highTimestamp int64
}

type hashedLabels struct {
	ref    uint64
	hash   uint64
	labels labels.Labels
}

func (a *headAppender) Add(lset labels.Labels, t int64, v float64) (string, error) {
	if t < a.mint {
		return "", ErrOutOfBounds
	}

	hash := lset.Hash()
	refb := make([]byte, 8)

	// Series exists already in the block.
	if ms := a.head.get(hash, lset); ms != nil {
		binary.BigEndian.PutUint64(refb, uint64(ms.ref))
		return string(refb), a.AddFast(string(refb), t, v)
	}
	// Series was added in this transaction previously.
	if ref, ok := a.newHashes[hash]; ok {
		binary.BigEndian.PutUint64(refb, ref)
		// XXX(fabxc): there's no fast path for multiple samples for the same new series
		// in the same transaction. We always return the invalid empty ref. It's has not
		// been a relevant use case so far and is not worth the trouble.
		return "", a.AddFast(string(refb), t, v)
	}

	// The series is completely new.
	if a.newSeries == nil {
		a.newHashes = map[uint64]uint64{}
	}
	// First sample for new series.
	ref := uint64(len(a.newSeries))

	a.newSeries = append(a.newSeries, &hashedLabels{
		ref:    ref,
		hash:   hash,
		labels: lset,
	})
	// First bit indicates its a series created in this transaction.
	ref |= (1 << 63)

	a.newHashes[hash] = ref
	binary.BigEndian.PutUint64(refb, ref)

	return "", a.AddFast(string(refb), t, v)
}

func (a *headAppender) AddFast(ref string, t int64, v float64) error {
	if len(ref) != 8 {
		return errors.Wrap(ErrNotFound, "invalid ref length")
	}
	var (
		refn = binary.BigEndian.Uint64(yoloBytes(ref))
		id   = uint32(refn)
		inTx = refn&(1<<63) != 0
	)
	// Distinguish between existing series and series created in
	// this transaction.
	if inTx {
		if id > uint32(len(a.newSeries)-1) {
			return errors.Wrap(ErrNotFound, "transaction series ID too high")
		}
		// TODO(fabxc): we also have to validate here that the
		// sample sequence is valid.
		// We also have to revalidate it as we switch locks and create
		// the new series.
	} else {
		ms, ok := a.head.series[id]
		if !ok {
			return errors.Wrap(ErrNotFound, "unknown series")
		}
		if err := ms.appendable(t, v); err != nil {
			return err
		}
	}
	if t < a.mint {
		return ErrOutOfBounds
	}

	if t > a.highTimestamp {
		a.highTimestamp = t
	}

	a.samples = append(a.samples, RefSample{
		Ref: refn,
		T:   t,
		V:   v,
	})
	return nil
}

func (a *headAppender) createSeries() error {
	if len(a.newSeries) == 0 {
		return nil
	}
	a.newLabels = make([]labels.Labels, 0, len(a.newSeries))
	base0 := len(a.head.series)

	a.head.mtx.RUnlock()
	defer a.head.mtx.RLock()
	a.head.mtx.Lock()
	defer a.head.mtx.Unlock()

	base1 := len(a.head.series)

	for _, l := range a.newSeries {
		// We switched locks and have to re-validate that the series were not
		// created by another goroutine in the meantime.
		if base1 > base0 {
			if ms := a.head.get(l.hash, l.labels); ms != nil {
				l.ref = uint64(ms.ref)
				continue
			}
		}
		// Series is still new.
		a.newLabels = append(a.newLabels, l.labels)

		s := a.head.create(l.hash, l.labels)
		l.ref = uint64(s.ref)
	}

	// Write all new series to the WAL.
	if err := a.head.wal.LogSeries(a.newLabels); err != nil {
		return errors.Wrap(err, "WAL log series")
	}

	return nil
}

func (a *headAppender) Commit() error {
	defer a.head.mtx.RUnlock()

	defer a.head.metrics.activeAppenders.Dec()
	defer a.head.putAppendBuffer(a.samples)

	if err := a.createSeries(); err != nil {
		return err
	}

	// We have to update the refs of samples for series we just created.
	for i := range a.samples {
		s := &a.samples[i]
		if s.Ref&(1<<63) != 0 {
			s.Ref = a.newSeries[(s.Ref<<1)>>1].ref
		}
	}

	// Write all new samples to the WAL and add them to the
	// in-mem database on success.
	if err := a.head.wal.LogSamples(a.samples); err != nil {
		return errors.Wrap(err, "WAL log samples")
	}

	total := uint64(len(a.samples))

	for _, s := range a.samples {
		series, ok := a.head.series[uint32(s.Ref)]
		if !ok {
			return errors.Errorf("series with ID %d not found", s.Ref)
		}
		ok, chunkCreated := series.append(s.T, s.V)
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
	a.head.mtx.RUnlock()

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
		series := h.series[p.At()]

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
	var (
		seriesRemoved int
		chunksRemoved int
	)
	// Only data strictly lower than this timestamp must be deleted.
	mint := h.MinTime()

	deletedHashes := map[uint64][]uint32{}

	h.mtx.RLock()

	for hash, ss := range h.hashes {
		for _, s := range ss {
			s.mtx.Lock()
			chunksRemoved += s.truncateChunksBefore(mint)

			if len(s.chunks) == 0 {
				deletedHashes[hash] = append(deletedHashes[hash], s.ref)
			}
			s.mtx.Unlock()
		}
	}

	deletedIDs := make(map[uint32]struct{}, len(deletedHashes))

	h.mtx.RUnlock()

	h.mtx.Lock()
	defer h.mtx.Unlock()

	for hash, ids := range deletedHashes {

		inIDs := func(id uint32) bool {
			for _, o := range ids {
				if o == id {
					return true
				}
			}
			return false
		}
		var rem []*memSeries

		for _, s := range h.hashes[hash] {
			if !inIDs(s.ref) {
				rem = append(rem, s)
				continue
			}
			deletedIDs[s.ref] = struct{}{}
			// We switched locks and the series might have received new samples by now,
			// check again.
			s.mtx.Lock()
			chkCount := len(s.chunks)
			s.mtx.Unlock()

			if chkCount > 0 {
				continue
			}
			delete(h.series, s.ref)
			seriesRemoved++
		}
		if len(rem) > 0 {
			h.hashes[hash] = rem
		} else {
			delete(h.hashes, hash)
		}
	}

	for t, p := range h.postings.m {
		repl := make([]uint32, 0, len(p))

		for _, id := range p {
			if _, ok := deletedIDs[id]; !ok {
				repl = append(repl, id)
			}
		}

		if len(repl) == 0 {
			delete(h.postings.m, t)
		} else {
			h.postings.m[t] = repl
		}
	}

	symbols := make(map[string]struct{}, len(h.symbols))
	values := make(map[string]stringset, len(h.values))

	for t := range h.postings.m {
		symbols[t.name] = struct{}{}
		symbols[t.value] = struct{}{}

		ss, ok := values[t.name]
		if !ok {
			ss = stringset{}
			values[t.name] = ss
		}
		ss.set(t.value)
	}

	h.symbols = symbols
	h.values = values

	h.metrics.seriesRemoved.Add(float64(seriesRemoved))
	h.metrics.series.Sub(float64(seriesRemoved))
	h.metrics.chunksRemoved.Add(float64(chunksRemoved))
	h.metrics.chunks.Sub(float64(chunksRemoved))
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

// Chunk returns the chunk for the reference number.
func (h *headChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	s := h.head.series[uint32(ref>>32)]

	s.mtx.RLock()
	cid := int((ref << 32) >> 32)
	c := s.chunk(cid)
	s.mtx.RUnlock()

	// Do not expose chunks that are outside of the specified range.
	if !intervalOverlap(c.minTime, c.maxTime, h.mint, h.maxt) {
		return nil, ErrNotFound
	}

	return &safeChunk{
		Chunk: c.chunk,
		s:     s,
		cid:   cid,
	}, nil
}

type safeChunk struct {
	chunks.Chunk
	s   *memSeries
	cid int
}

func (c *safeChunk) Iterator() chunks.Iterator {
	c.s.mtx.RLock()
	defer c.s.mtx.RUnlock()
	return c.s.iterator(c.cid)
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
	return h.head.symbols, nil
}

// LabelValues returns the possible label values
func (h *headIndexReader) LabelValues(names ...string) (StringTuples, error) {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	if len(names) != 1 {
		return nil, errInvalidSize
	}
	var sl []string

	for s := range h.head.values[names[0]] {
		sl = append(sl, s)
	}
	sort.Strings(sl)

	return &stringTuples{l: len(names), s: sl}, nil
}

// Postings returns the postings list iterator for the label pair.
func (h *headIndexReader) Postings(name, value string) (Postings, error) {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	return h.head.postings.get(term{name: name, value: value}), nil
}

func (h *headIndexReader) SortedPostings(p Postings) Postings {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	ep := make([]uint32, 0, 1024)

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
		a, ok1 := h.head.series[ep[i]]
		b, ok2 := h.head.series[ep[j]]

		if !ok1 || !ok2 {
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
func (h *headIndexReader) Series(ref uint32, lbls *labels.Labels, chks *[]ChunkMeta) error {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	s := h.head.series[ref]
	if s == nil {
		return ErrNotFound
	}
	*lbls = append((*lbls)[:0], s.lset...)

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	*chks = (*chks)[:0]

	for i, c := range s.chunks {
		// Do not expose chunks that are outside of the specified range.
		if !intervalOverlap(c.minTime, c.maxTime, h.mint, h.maxt) {
			continue
		}
		*chks = append(*chks, ChunkMeta{
			MinTime: c.minTime,
			MaxTime: c.maxTime,
			Ref:     (uint64(ref) << 32) | uint64(s.chunkID(i)),
		})
	}

	return nil
}

func (h *headIndexReader) LabelIndices() ([][]string, error) {
	h.head.mtx.RLock()
	defer h.head.mtx.RUnlock()

	res := [][]string{}

	for s := range h.head.values {
		res = append(res, []string{s})
	}
	return res, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *Head) get(hash uint64, lset labels.Labels) *memSeries {
	series := h.hashes[hash]

	for _, s := range series {
		if s.lset.Equals(lset) {
			return s
		}
	}
	return nil
}

func (h *Head) create(hash uint64, lset labels.Labels) *memSeries {
	h.metrics.series.Inc()
	h.metrics.seriesCreated.Inc()

	id := atomic.AddUint32(&h.lastSeriesID, 1)

	s := newMemSeries(lset, id, h.chunkRange)
	h.series[id] = s

	h.hashes[hash] = append(h.hashes[hash], s)

	for _, l := range lset {
		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)

		h.postings.add(s.ref, term{name: l.Name, value: l.Value})

		h.symbols[l.Name] = struct{}{}
		h.symbols[l.Value] = struct{}{}
	}

	h.postings.add(id, term{})

	return s
}

type sample struct {
	t int64
	v float64
}

type memSeries struct {
	mtx sync.RWMutex

	ref          uint32
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

func newMemSeries(lset labels.Labels, id uint32, chunkRange int64) *memSeries {
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
	if len(s.chunks) == 0 {
		return nil
	}
	c := s.head()

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
	return s.chunks[id-s.firstChunkID]
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

	s.mtx.Lock()

	var c *memChunk

	if len(s.chunks) == 0 {
		c = s.cut(t)
		chunkCreated = true
	}
	c = s.head()
	if c.maxTime >= t {
		s.mtx.Unlock()
		return false, chunkCreated
	}
	if c.samples > samplesPerChunk/4 && t >= s.nextAt {
		c = s.cut(t)
		chunkCreated = true
	}
	s.app.Append(t, v)

	c.maxTime = t
	c.samples++

	if c.samples == samplesPerChunk/4 {
		_, maxt := rangeForTimestamp(c.minTime, s.chunkRange)
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, maxt)
	}

	s.lastValue = v

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, v: v}

	s.mtx.Unlock()

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

	// TODO(fabxc): !!! Test this and everything around chunk ID != list pos.
	if id-s.firstChunkID < len(s.chunks)-1 {
		return c.chunk.Iterator()
	}
	// Serve the last 4 samples for the last chunk from the series buffer
	// as their compressed bytes may be mutated by added samples.
	it := &memSafeIterator{
		Iterator: c.chunk.Iterator(),
		i:        -1,
		total:    c.samples,
		buf:      s.sampleBuf,
	}
	return it
}

func (s *memSeries) head() *memChunk {
	return s.chunks[len(s.chunks)-1]
}

type memChunk struct {
	chunk            chunks.Chunk
	minTime, maxTime int64
	samples          int
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
