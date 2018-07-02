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
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/index"
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

	postings *index.MemPostings // postings lists for terms

	tombstones memTombstones
}

type headMetrics struct {
	activeAppenders     prometheus.Gauge
	series              prometheus.Gauge
	seriesCreated       prometheus.Counter
	seriesRemoved       prometheus.Counter
	seriesNotFound      prometheus.Counter
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
		Name: "prometheus_tsdb_head_active_appenders",
		Help: "Number of currently active appender transactions",
	})
	m.series = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_series",
		Help: "Total number of series in the head block.",
	})
	m.seriesCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_series_created_total",
		Help: "Total number of series created in the head",
	})
	m.seriesRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_series_removed_total",
		Help: "Total number of series removed in the head",
	})
	m.seriesNotFound = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_head_series_not_found",
		Help: "Total number of requests for series that were not found.",
	})
	m.chunks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_chunks",
		Help: "Total number of chunks in the head block.",
	})
	m.chunksCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_chunks_created_total",
		Help: "Total number of chunks created in the head",
	})
	m.chunksRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_chunks_removed_total",
		Help: "Total number of chunks removed in the head",
	})
	m.gcDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "prometheus_tsdb_head_gc_duration_seconds",
		Help: "Runtime of garbage collection in the head block.",
	})
	m.maxTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_max_time",
		Help: "Maximum timestamp of the head block.",
	}, func() float64 {
		return float64(h.MaxTime())
	})
	m.minTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_min_time",
		Help: "Minimum time bound of the head block.",
	}, func() float64 {
		return float64(h.MinTime())
	})
	m.walTruncateDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "prometheus_tsdb_wal_truncate_duration_seconds",
		Help: "Duration of WAL truncation.",
	})
	m.samplesAppended = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_head_samples_appended_total",
		Help: "Total number of appended samples.",
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
			m.seriesNotFound,
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
		postings:   index.NewUnorderedMemPostings(),
		tombstones: memTombstones{},
	}
	h.metrics = newHeadMetrics(h, r)

	return h, nil
}

// processWALSamples adds a partition of samples it receives to the head and passes
// them on to other workers.
// Samples before the mint timestamp are discarded.
func (h *Head) processWALSamples(
	mint int64,
	partition, total uint64,
	input <-chan []RefSample, output chan<- []RefSample,
) (unknownRefs uint64) {
	defer close(output)

	for samples := range input {
		for _, s := range samples {
			if s.T < mint || s.Ref%total != partition {
				continue
			}
			ms := h.series.getByID(s.Ref)
			if ms == nil {
				unknownRefs++
				continue
			}
			_, chunkCreated := ms.append(s.T, s.V)
			if chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
		}
		output <- samples
	}
	return unknownRefs
}

// ReadWAL initializes the head by consuming the write ahead log.
func (h *Head) ReadWAL() error {
	defer h.postings.EnsureOrder()

	r := h.wal.Reader()
	mint := h.MinTime()

	// Track number of samples that referenced a series we don't know about
	// for error reporting.
	var unknownRefs uint64

	// Start workers that each process samples for a partition of the series ID space.
	// They are connected through a ring of channels which ensures that all sample batches
	// read from the WAL are processed in order.
	var (
		wg         sync.WaitGroup
		n          = runtime.GOMAXPROCS(0)
		firstInput = make(chan []RefSample, 300)
		input      = firstInput
	)
	wg.Add(n)

	for i := 0; i < n; i++ {
		output := make(chan []RefSample, 300)

		go func(i int, input <-chan []RefSample, output chan<- []RefSample) {
			unknown := h.processWALSamples(mint, uint64(i), uint64(n), input, output)
			atomic.AddUint64(&unknownRefs, unknown)
			wg.Done()
		}(i, input, output)

		// The output feeds the next worker goroutine. For the last worker,
		// it feeds the initial input again to reuse the RefSample slices.
		input = output
	}

	// TODO(fabxc): series entries spread between samples can starve the sample workers.
	// Even with bufferd channels, this can impact startup time with lots of series churn.
	// We must not paralellize series creation itself but could make the indexing asynchronous.
	seriesFunc := func(series []RefSeries) {
		for _, s := range series {
			h.getOrCreateWithID(s.Ref, s.Labels.Hash(), s.Labels)

			if h.lastSeriesID < s.Ref {
				h.lastSeriesID = s.Ref
			}
		}
	}
	samplesFunc := func(samples []RefSample) {
		// We split up the samples into chunks of 5000 samples or less.
		// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
		// cause thousands of very large in flight buffers occupying large amounts
		// of unused memory.
		for len(samples) > 0 {
			n := 5000
			if len(samples) < n {
				n = len(samples)
			}
			var buf []RefSample
			select {
			case buf = <-input:
			default:
			}
			firstInput <- append(buf[:0], samples[:n]...)
			samples = samples[n:]
		}
	}
	deletesFunc := func(stones []Stone) {
		for _, s := range stones {
			for _, itv := range s.intervals {
				if itv.Maxt < mint {
					continue
				}
				h.tombstones.add(s.ref, itv)
			}
		}
	}

	err := r.Read(seriesFunc, samplesFunc, deletesFunc)

	// Signal termination to first worker and wait for last one to close its output channel.
	close(firstInput)
	for range input {
	}
	wg.Wait()

	if err != nil {
		return errors.Wrap(err, "consume WAL")
	}
	if unknownRefs > 0 {
		level.Warn(h.logger).Log("msg", "unknown series references in WAL samples", "count", unknownRefs)
	}
	return nil
}

// Truncate removes all data before mint from the head block and truncates its WAL.
func (h *Head) Truncate(mint int64) error {
	initialize := h.MinTime() == math.MinInt64

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
	level.Info(h.logger).Log("msg", "head GC completed", "duration", time.Since(start))
	h.metrics.gcDuration.Observe(time.Since(start).Seconds())

	start = time.Now()

	keep := func(id uint64) bool {
		return h.series.getByID(id) != nil
	}
	if err := h.wal.Truncate(mint, keep); err == nil {
		level.Info(h.logger).Log("msg", "WAL truncation completed", "duration", time.Since(start))
	} else {
		level.Error(h.logger).Log("msg", "WAL truncation failed", "err", err, "duration", time.Since(start))
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

type rangeHead struct {
	head       *Head
	mint, maxt int64
}

func (h *rangeHead) Index() (IndexReader, error) {
	return h.head.indexRange(h.mint, h.maxt), nil
}

func (h *rangeHead) Chunks() (ChunkReader, error) {
	return h.head.chunksRange(h.mint, h.maxt), nil
}

func (h *rangeHead) Tombstones() (TombstoneReader, error) {
	return h.head.tombstones, nil
}

// initAppender is a helper to initialize the time bounds of the head
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
		head:    h,
		mint:    h.MaxTime() - h.chunkRange/2,
		maxt:    math.MinInt64,
		samples: h.getAppendBuffer(),
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
	head       *Head
	mint, maxt int64

	series  []RefSeries
	samples []RefSample
}

func (a *headAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	if t < a.mint {
		return 0, ErrOutOfBounds
	}

	s, created := a.head.getOrCreate(lset.Hash(), lset)
	if created {
		a.series = append(a.series, RefSeries{
			Ref:    s.ref,
			Labels: lset,
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
	if t > a.maxt {
		a.maxt = t
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
	defer a.head.metrics.activeAppenders.Dec()
	defer a.head.putAppendBuffer(a.samples)

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
		if a.maxt <= ht {
			break
		}
		if atomic.CompareAndSwapInt64(&a.head.maxTime, ht, a.maxt) {
			break
		}
	}

	return nil
}

func (a *headAppender) Rollback() error {
	a.head.metrics.activeAppenders.Dec()
	a.head.putAppendBuffer(a.samples)

	// Series are created in the head memory regardless of rollback. Thus we have
	// to log them to the WAL in any case.
	return a.head.wal.LogSeries(a.series)
}

// Delete all samples in the range of [mint, maxt] for series that satisfy the given
// label matchers.
func (h *Head) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	// Do not delete anything beyond the currently valid range.
	mint, maxt = clampInterval(mint, maxt, h.MinTime(), h.MaxTime())

	ir := h.indexRange(mint, maxt)

	p, err := PostingsForMatchers(ir, ms...)
	if err != nil {
		return errors.Wrap(err, "select series")
	}

	var stones []Stone

	for p.Next() {
		series := h.series.getByID(p.At())

		t0, t1 := series.minTime(), series.maxTime()
		if t0 == math.MinInt64 || t1 == math.MinInt64 {
			continue
		}
		// Delete only until the current values and not beyond.
		t0, t1 = clampInterval(mint, maxt, t0, t1)
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

// gc removes data before the minimum timestamp from the head.
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

	// Remove deleted series IDs from the postings lists.
	h.postings.Delete(deleted)

	// Rebuild symbols and label value indices from what is left in the postings terms.
	symbols := make(map[string]struct{})
	values := make(map[string]stringset, len(h.values))

	h.postings.Iter(func(t labels.Label, _ index.Postings) error {
		symbols[t.Name] = struct{}{}
		symbols[t.Value] = struct{}{}

		ss, ok := values[t.Name]
		if !ok {
			ss = stringset{}
			values[t.Name] = ss
		}
		ss.set(t.Value)
		return nil
	})

	h.symMtx.Lock()

	h.symbols = symbols
	h.values = values

	h.symMtx.Unlock()
}

// Tombstones returns a new reader over the head's tombstones
func (h *Head) Tombstones() (TombstoneReader, error) {
	return h.tombstones, nil
}

// Index returns an IndexReader against the block.
func (h *Head) Index() (IndexReader, error) {
	return h.indexRange(math.MinInt64, math.MaxInt64), nil
}

func (h *Head) indexRange(mint, maxt int64) *headIndexReader {
	if hmin := h.MinTime(); hmin > mint {
		mint = hmin
	}
	return &headIndexReader{head: h, mint: mint, maxt: maxt}
}

// Chunks returns a ChunkReader against the block.
func (h *Head) Chunks() (ChunkReader, error) {
	return h.chunksRange(math.MinInt64, math.MaxInt64), nil
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

// Close flushes the WAL and closes the head.
func (h *Head) Close() error {
	return h.wal.Close()
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
func (h *headChunkReader) Chunk(ref uint64) (chunkenc.Chunk, error) {
	sid, cid := unpackChunkID(ref)

	s := h.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, ErrNotFound
	}

	s.Lock()
	c := s.chunk(int(cid))

	// This means that the chunk has been garbage collected or is outside
	// the specified range.
	if c == nil || !c.OverlapsClosedInterval(h.mint, h.maxt) {
		s.Unlock()
		return nil, ErrNotFound
	}
	s.Unlock()

	return &safeChunk{
		Chunk: c.chunk,
		s:     s,
		cid:   int(cid),
	}, nil
}

type safeChunk struct {
	chunkenc.Chunk
	s   *memSeries
	cid int
}

func (c *safeChunk) Iterator() chunkenc.Iterator {
	c.s.Lock()
	it := c.s.iterator(c.cid)
	c.s.Unlock()
	return it
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
func (h *headIndexReader) LabelValues(names ...string) (index.StringTuples, error) {
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

	return index.NewStringTuples(sl, len(names))
}

// Postings returns the postings list iterator for the label pair.
func (h *headIndexReader) Postings(name, value string) (index.Postings, error) {
	return h.head.postings.Get(name, value), nil
}

func (h *headIndexReader) SortedPostings(p index.Postings) index.Postings {
	ep := make([]uint64, 0, 128)

	for p.Next() {
		ep = append(ep, p.At())
	}
	if err := p.Err(); err != nil {
		return index.ErrPostings(errors.Wrap(err, "expand postings"))
	}

	sort.Slice(ep, func(i, j int) bool {
		a := h.head.series.getByID(ep[i])
		b := h.head.series.getByID(ep[j])

		if a == nil || b == nil {
			level.Debug(h.head.logger).Log("msg", "looked up series not found")
			return false
		}
		return labels.Compare(a.lset, b.lset) < 0
	})
	return index.NewListPostings(ep)
}

// Series returns the series for the given reference.
func (h *headIndexReader) Series(ref uint64, lbls *labels.Labels, chks *[]chunks.Meta) error {
	s := h.head.series.getByID(ref)

	if s == nil {
		h.head.metrics.seriesNotFound.Inc()
		return ErrNotFound
	}
	*lbls = append((*lbls)[:0], s.lset...)

	s.Lock()
	defer s.Unlock()

	*chks = (*chks)[:0]

	for i, c := range s.chunks {
		// Do not expose chunks that are outside of the specified range.
		if !c.OverlapsClosedInterval(h.mint, h.maxt) {
			continue
		}
		*chks = append(*chks, chunks.Meta{
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

func (h *Head) getOrCreate(hash uint64, lset labels.Labels) (*memSeries, bool) {
	// Just using `getOrSet` below would be semantically sufficient, but we'd create
	// a new series on every sample inserted via Add(), which causes allocations
	// and makes our series IDs rather random and harder to compress in postings.
	s := h.series.getByHash(hash, lset)
	if s != nil {
		return s, false
	}

	// Optimistically assume that we are the first one to create the series.
	id := atomic.AddUint64(&h.lastSeriesID, 1)

	return h.getOrCreateWithID(id, hash, lset)
}

func (h *Head) getOrCreateWithID(id, hash uint64, lset labels.Labels) (*memSeries, bool) {
	s := newMemSeries(lset, id, h.chunkRange)

	s, created := h.series.getOrSet(hash, s)
	if !created {
		return s, false
	}

	h.metrics.series.Inc()
	h.metrics.seriesCreated.Inc()

	h.postings.Add(id, lset)

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

	return s, true
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
// The locks are padded to not be on the same cache line. Filling the padded space
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
// are goroutine safe and it is the caller's responsibility to lock it.
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

	app chunkenc.Appender // Current appender for the chunk.
}

func (s *memSeries) minTime() int64 {
	if len(s.chunks) == 0 {
		return math.MinInt64
	}
	return s.chunks[0].minTime
}

func (s *memSeries) maxTime() int64 {
	c := s.head()
	if c == nil {
		return math.MinInt64
	}
	return c.maxTime
}

func (s *memSeries) cut(mint int64) *memChunk {
	c := &memChunk{
		chunk:   chunkenc.NewXORChunk(),
		minTime: mint,
		maxTime: math.MinInt64,
	}
	s.chunks = append(s.chunks, c)

	// Set upper bound on when the next chunk must be started. An earlier timestamp
	// may be chosen dynamically at a later point.
	_, s.nextAt = rangeForTimestamp(mint, s.chunkRange)

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
	numSamples := c.chunk.NumSamples()

	// Out of order sample.
	if c.maxTime >= t {
		return false, chunkCreated
	}
	// If we reach 25% of a chunk's desired sample count, set a definitive time
	// at which to start the next chunk.
	// At latest it must happen at the timestamp set when the chunk was cut.
	if numSamples == samplesPerChunk/4 {
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, s.nextAt)
	}
	if t >= s.nextAt {
		c = s.cut(t)
		chunkCreated = true
	}
	s.app.Append(t, v)

	c.maxTime = t

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

func (s *memSeries) iterator(id int) chunkenc.Iterator {
	c := s.chunk(id)
	// TODO(fabxc): Work around! A querier may have retrieved a pointer to a series' chunk,
	// which got then garbage collected before it got accessed.
	// We must ensure to not garbage collect as long as any readers still hold a reference.
	if c == nil {
		return chunkenc.NewNopIterator()
	}

	if id-s.firstChunkID < len(s.chunks)-1 {
		return c.chunk.Iterator()
	}
	// Serve the last 4 samples for the last chunk from the sample buffer
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
	chunk            chunkenc.Chunk
	minTime, maxTime int64
}

// Returns true if the chunk overlaps [mint, maxt].
func (mc *memChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return mc.minTime <= maxt && mint <= mc.maxTime
}

type memSafeIterator struct {
	chunkenc.Iterator

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

type stringset map[string]struct{}

func (ss stringset) set(s string) {
	ss[s] = struct{}{}
}

func (ss stringset) has(s string) bool {
	_, ok := ss[s]
	return ok
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}
