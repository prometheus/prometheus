// Copyright 2021 The Prometheus Authors
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
	"context"
	"fmt"
	"math"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

// initAppender is a helper to initialize the time bounds of the head
// upon the first sample it receives.
type initAppender struct {
	app  storage.Appender
	head *Head
}

var _ storage.GetRef = &initAppender{}

func (a *initAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.Append(ref, lset, t, v)
	}

	a.head.initTime(t)
	a.app = a.head.appender()
	return a.app.Append(ref, lset, t, v)
}

func (a *initAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	// Check if exemplar storage is enabled.
	if !a.head.opts.EnableExemplarStorage || a.head.opts.MaxExemplars.Load() <= 0 {
		return 0, nil
	}

	if a.app != nil {
		return a.app.AppendExemplar(ref, l, e)
	}
	// We should never reach here given we would call Append before AppendExemplar
	// and we probably want to always base head/WAL min time on sample times.
	a.head.initTime(e.Ts)
	a.app = a.head.appender()

	return a.app.AppendExemplar(ref, l, e)
}

func (a *initAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.AppendHistogram(ref, l, t, h)
	}
	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.AppendHistogram(ref, l, t, h)
}

// initTime initializes a head with the first timestamp. This only needs to be called
// for a completely fresh head with an empty WAL.
func (h *Head) initTime(t int64) {
	if !h.minTime.CAS(math.MaxInt64, t) {
		return
	}
	// Ensure that max time is initialized to at least the min time we just set.
	// Concurrent appenders may already have set it to a higher value.
	h.maxTime.CAS(math.MinInt64, t)
}

func (a *initAppender) GetRef(lset labels.Labels) (storage.SeriesRef, labels.Labels) {
	if g, ok := a.app.(storage.GetRef); ok {
		return g.GetRef(lset)
	}
	return 0, nil
}

func (a *initAppender) Commit() error {
	if a.app == nil {
		a.head.metrics.activeAppenders.Dec()
		return nil
	}
	return a.app.Commit()
}

func (a *initAppender) Rollback() error {
	if a.app == nil {
		a.head.metrics.activeAppenders.Dec()
		return nil
	}
	return a.app.Rollback()
}

// Appender returns a new Appender on the database.
func (h *Head) Appender(_ context.Context) storage.Appender {
	h.metrics.activeAppenders.Inc()

	// The head cache might not have a starting point yet. The init appender
	// picks up the first appended timestamp as the base.
	if h.MinTime() == math.MaxInt64 {
		return &initAppender{
			head: h,
		}
	}
	return h.appender()
}

func (h *Head) appender() *headAppender {
	appendID, cleanupAppendIDsBelow := h.iso.newAppendID() // Every appender gets an ID that is cleared upon commit/rollback.

	// Allocate the exemplars buffer only if exemplars are enabled.
	var exemplarsBuf []exemplarWithSeriesRef
	if h.opts.EnableExemplarStorage {
		exemplarsBuf = h.getExemplarBuffer()
	}

	return &headAppender{
		head:                  h,
		minValidTime:          h.appendableMinValidTime(),
		mint:                  math.MaxInt64,
		maxt:                  math.MinInt64,
		samples:               h.getAppendBuffer(),
		sampleSeries:          h.getSeriesBuffer(),
		exemplars:             exemplarsBuf,
		histograms:            h.getHistogramBuffer(),
		appendID:              appendID,
		cleanupAppendIDsBelow: cleanupAppendIDsBelow,
	}
}

// appendableMinValidTime returns the minimum valid timestamp for appends,
// such that samples stay ahead of prior blocks and the head compaction window.
func (h *Head) appendableMinValidTime() int64 {
	// This boundary ensures that no samples will be added to the compaction window.
	// This allows race-free, concurrent appending and compaction.
	cwEnd := h.MaxTime() - h.chunkRange.Load()/2

	// This boundary ensures that we avoid overlapping timeframes from one block to the next.
	// While not necessary for correctness, it means we're not required to use vertical compaction.
	minValid := h.minValidTime.Load()

	return max(cwEnd, minValid)
}

// AppendableMinValidTime returns the minimum valid time for samples to be appended to the Head.
// Returns false if Head hasn't been initialized yet and the minimum time isn't known yet.
func (h *Head) AppendableMinValidTime() (int64, bool) {
	if h.MinTime() == math.MaxInt64 {
		return 0, false
	}

	return h.appendableMinValidTime(), true
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (h *Head) getAppendBuffer() []record.RefSample {
	b := h.appendPool.Get()
	if b == nil {
		return make([]record.RefSample, 0, 512)
	}
	return b.([]record.RefSample)
}

func (h *Head) putAppendBuffer(b []record.RefSample) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.appendPool.Put(b[:0])
}

func (h *Head) getExemplarBuffer() []exemplarWithSeriesRef {
	b := h.exemplarsPool.Get()
	if b == nil {
		return make([]exemplarWithSeriesRef, 0, 512)
	}
	return b.([]exemplarWithSeriesRef)
}

func (h *Head) putExemplarBuffer(b []exemplarWithSeriesRef) {
	if b == nil {
		return
	}

	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.exemplarsPool.Put(b[:0])
}

func (h *Head) getHistogramBuffer() []record.RefHistogram {
	b := h.histogramsPool.Get()
	if b == nil {
		return make([]record.RefHistogram, 0, 512)
	}
	return b.([]record.RefHistogram)
}

func (h *Head) putHistogramBuffer(b []record.RefHistogram) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.histogramsPool.Put(b[:0])
}

func (h *Head) getSeriesBuffer() []*memSeries {
	b := h.seriesPool.Get()
	if b == nil {
		return make([]*memSeries, 0, 512)
	}
	return b.([]*memSeries)
}

func (h *Head) putSeriesBuffer(b []*memSeries) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.seriesPool.Put(b[:0])
}

func (h *Head) getBytesBuffer() []byte {
	b := h.bytesPool.Get()
	if b == nil {
		return make([]byte, 0, 1024)
	}
	return b.([]byte)
}

func (h *Head) putBytesBuffer(b []byte) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.bytesPool.Put(b[:0])
}

type exemplarWithSeriesRef struct {
	ref      storage.SeriesRef
	exemplar exemplar.Exemplar
}

type headAppender struct {
	head         *Head
	minValidTime int64 // No samples below this timestamp are allowed.
	mint, maxt   int64

	series          []record.RefSeries      // New series held by this appender.
	samples         []record.RefSample      // New float samples held by this appender.
	exemplars       []exemplarWithSeriesRef // New exemplars held by this appender.
	sampleSeries    []*memSeries            // Float series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	histograms      []record.RefHistogram   // New histogram samples held by this appender.
	histogramSeries []*memSeries            // Histogram series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).

	appendID, cleanupAppendIDsBelow uint64
	closed                          bool
}

func (a *headAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.Inc()
		return 0, storage.ErrOutOfBounds
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		// Ensure no empty labels have gotten through.
		lset = lset.WithoutEmpty()
		if len(lset) == 0 {
			return 0, errors.Wrap(ErrInvalidSample, "empty labelset")
		}

		if l, dup := lset.HasDuplicateLabelNames(); dup {
			return 0, errors.Wrap(ErrInvalidSample, fmt.Sprintf(`label name "%s" is not unique`, l))
		}

		var created bool
		var err error
		s, created, err = a.head.getOrCreate(lset.Hash(), lset)
		if err != nil {
			return 0, err
		}
		if created {
			a.series = append(a.series, record.RefSeries{
				Ref:    s.ref,
				Labels: lset,
			})
		}
	}

	if value.IsStaleNaN(v) && s.isHistogramSeries {
		return a.AppendHistogram(ref, lset, t, &histogram.Histogram{Sum: v})
	}

	s.Lock()
	if err := s.appendable(t, v); err != nil {
		s.Unlock()
		if err == storage.ErrOutOfOrderSample {
			a.head.metrics.outOfOrderSamples.Inc()
		}
		return 0, err
	}
	s.pendingCommit = true
	s.Unlock()

	if t < a.mint {
		a.mint = t
	}
	if t > a.maxt {
		a.maxt = t
	}

	a.samples = append(a.samples, record.RefSample{
		Ref: s.ref,
		T:   t,
		V:   v,
	})
	a.sampleSeries = append(a.sampleSeries, s)
	return storage.SeriesRef(s.ref), nil
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
		return storage.ErrOutOfOrderSample
	}
	// We are allowing exact duplicates as we can encounter them in valid cases
	// like federation and erroring out at that time would be extremely noisy.
	if math.Float64bits(s.sampleBuf[3].v) != math.Float64bits(v) {
		return storage.ErrDuplicateSampleForTimestamp
	}
	return nil
}

// appendableHistogram checks whether the given sample is valid for appending to the series.
func (s *memSeries) appendableHistogram(t int64, h *histogram.Histogram) error {
	c := s.head()
	if c == nil {
		return nil
	}

	if t > c.maxTime {
		return nil
	}
	if t < c.maxTime {
		return storage.ErrOutOfOrderSample
	}
	// TODO(beorn7): do it for histogram.
	// We are allowing exact duplicates as we can encounter them in valid cases
	// like federation and erroring out at that time would be extremely noisy.
	//if math.Float64bits(s.sampleBuf[3].v) != math.Float64bits(v) {
	//	return storage.ErrDuplicateSampleForTimestamp
	//}
	return nil
}

// AppendExemplar for headAppender assumes the series ref already exists, and so it doesn't
// use getOrCreate or make any of the lset sanity checks that Append does.
func (a *headAppender) AppendExemplar(ref storage.SeriesRef, lset labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	// Check if exemplar storage is enabled.
	if !a.head.opts.EnableExemplarStorage || a.head.opts.MaxExemplars.Load() <= 0 {
		return 0, nil
	}

	// Get Series
	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		s = a.head.series.getByHash(lset.Hash(), lset)
		if s != nil {
			ref = storage.SeriesRef(s.ref)
		}
	}
	if s == nil {
		return 0, fmt.Errorf("unknown HeadSeriesRef when trying to add exemplar: %d", ref)
	}

	// Ensure no empty labels have gotten through.
	e.Labels = e.Labels.WithoutEmpty()

	err := a.head.exemplars.ValidateExemplar(s.lset, e)
	if err != nil {
		if err == storage.ErrDuplicateExemplar || err == storage.ErrExemplarsDisabled {
			// Duplicate, don't return an error but don't accept the exemplar.
			return 0, nil
		}
		return 0, err
	}

	a.exemplars = append(a.exemplars, exemplarWithSeriesRef{ref, e})

	return storage.SeriesRef(s.ref), nil
}

func (a *headAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram) (storage.SeriesRef, error) {
	if t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.Inc()
		return 0, storage.ErrOutOfBounds
	}

	if err := ValidateHistogram(h); err != nil {
		return 0, err
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		// Ensure no empty labels have gotten through.
		lset = lset.WithoutEmpty()
		if len(lset) == 0 {
			return 0, errors.Wrap(ErrInvalidSample, "empty labelset")
		}

		if l, dup := lset.HasDuplicateLabelNames(); dup {
			return 0, errors.Wrap(ErrInvalidSample, fmt.Sprintf(`label name "%s" is not unique`, l))
		}

		var created bool
		var err error
		s, created, err = a.head.getOrCreate(lset.Hash(), lset)
		if err != nil {
			return 0, err
		}
		s.isHistogramSeries = true
		if created {
			a.series = append(a.series, record.RefSeries{
				Ref:    s.ref,
				Labels: lset,
			})
		}
	}

	s.Lock()
	if err := s.appendableHistogram(t, h); err != nil {
		s.Unlock()
		if err == storage.ErrOutOfOrderSample {
			a.head.metrics.outOfOrderSamples.Inc()
		}
		return 0, err
	}
	s.pendingCommit = true
	s.Unlock()

	if t < a.mint {
		a.mint = t
	}
	if t > a.maxt {
		a.maxt = t
	}

	a.histograms = append(a.histograms, record.RefHistogram{
		Ref: s.ref,
		T:   t,
		H:   h,
	})
	a.histogramSeries = append(a.histogramSeries, s)
	return storage.SeriesRef(s.ref), nil
}

func ValidateHistogram(h *histogram.Histogram) error {
	if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
		return errors.Wrap(err, "negative side")
	}
	if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
		return errors.Wrap(err, "positive side")
	}

	negativeCount, err := checkHistogramBuckets(h.NegativeBuckets)
	if err != nil {
		return errors.Wrap(err, "negative side")
	}
	positiveCount, err := checkHistogramBuckets(h.PositiveBuckets)
	if err != nil {
		return errors.Wrap(err, "positive side")
	}

	if c := negativeCount + positiveCount; c > h.Count {
		return errors.Wrap(
			storage.ErrHistogramCountNotBigEnough,
			fmt.Sprintf("%d observations found in buckets, but overall count is %d", c, h.Count),
		)
	}

	return nil
}

func checkHistogramSpans(spans []histogram.Span, numBuckets int) error {
	var spanBuckets int
	for n, span := range spans {
		if n > 0 && span.Offset < 0 {
			return errors.Wrap(
				storage.ErrHistogramSpanNegativeOffset,
				fmt.Sprintf("span number %d with offset %d", n+1, span.Offset),
			)
		}
		spanBuckets += int(span.Length)
	}
	if spanBuckets != numBuckets {
		return errors.Wrap(
			storage.ErrHistogramSpansBucketsMismatch,
			fmt.Sprintf("spans need %d buckets, have %d buckets", spanBuckets, numBuckets),
		)
	}
	return nil
}

func checkHistogramBuckets(buckets []int64) (uint64, error) {
	if len(buckets) == 0 {
		return 0, nil
	}

	var count uint64
	var last int64

	for i := 0; i < len(buckets); i++ {
		c := last + buckets[i]
		if c < 0 {
			return 0, errors.Wrap(
				storage.ErrHistogramNegativeBucketCount,
				fmt.Sprintf("bucket number %d has observation count of %d", i+1, c),
			)
		}
		last = c
		count += uint64(c)
	}

	return count, nil
}

var _ storage.GetRef = &headAppender{}

func (a *headAppender) GetRef(lset labels.Labels) (storage.SeriesRef, labels.Labels) {
	s := a.head.series.getByHash(lset.Hash(), lset)
	if s == nil {
		return 0, nil
	}
	// returned labels must be suitable to pass to Append()
	return storage.SeriesRef(s.ref), s.lset
}

// log writes all headAppender's data to the WAL.
func (a *headAppender) log() error {
	if a.head.wal == nil {
		return nil
	}

	buf := a.head.getBytesBuffer()
	defer func() { a.head.putBytesBuffer(buf) }()

	var rec []byte
	var enc record.Encoder

	if len(a.series) > 0 {
		rec = enc.Series(a.series, buf)
		buf = rec[:0]

		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log series")
		}
	}
	if len(a.samples) > 0 {
		rec = enc.Samples(a.samples, buf)
		buf = rec[:0]

		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log samples")
		}
	}
	if len(a.exemplars) > 0 {
		rec = enc.Exemplars(exemplarsForEncoding(a.exemplars), buf)
		buf = rec[:0]

		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log exemplars")
		}
	}
	if len(a.histograms) > 0 {
		rec = enc.Histograms(a.histograms, buf)
		buf = rec[:0]
		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log histograms")
		}
	}
	return nil
}

func exemplarsForEncoding(es []exemplarWithSeriesRef) []record.RefExemplar {
	ret := make([]record.RefExemplar, 0, len(es))
	for _, e := range es {
		ret = append(ret, record.RefExemplar{
			Ref:    chunks.HeadSeriesRef(e.ref),
			T:      e.exemplar.Ts,
			V:      e.exemplar.Value,
			Labels: e.exemplar.Labels,
		})
	}
	return ret
}

// Commit writes to the WAL and adds the data to the Head.
func (a *headAppender) Commit() (err error) {
	if a.closed {
		return ErrAppenderClosed
	}
	defer func() { a.closed = true }()

	if err := a.log(); err != nil {
		_ = a.Rollback() // Most likely the same error will happen again.
		return errors.Wrap(err, "write to WAL")
	}

	// No errors logging to WAL, so pass the exemplars along to the in memory storage.
	for _, e := range a.exemplars {
		s := a.head.series.getByID(chunks.HeadSeriesRef(e.ref))
		// We don't instrument exemplar appends here, all is instrumented by storage.
		if err := a.head.exemplars.AddExemplar(s.lset, e.exemplar); err != nil {
			if err == storage.ErrOutOfOrderExemplar {
				continue
			}
			level.Debug(a.head.logger).Log("msg", "Unknown error while adding exemplar", "err", err)
		}
	}

	defer a.head.metrics.activeAppenders.Dec()
	defer a.head.putAppendBuffer(a.samples)
	defer a.head.putSeriesBuffer(a.sampleSeries)
	defer a.head.putExemplarBuffer(a.exemplars)
	defer a.head.putHistogramBuffer(a.histograms)
	defer a.head.iso.closeAppend(a.appendID)

	total := len(a.samples)
	var series *memSeries
	for i, s := range a.samples {
		series = a.sampleSeries[i]
		series.Lock()
		ok, chunkCreated := series.append(s.T, s.V, a.appendID, a.head.chunkDiskMapper)
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()

		if !ok {
			total--
			a.head.metrics.outOfOrderSamples.Inc()
		}
		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}
	}

	total += len(a.histograms) // TODO: different metric?
	for i, s := range a.histograms {
		series = a.histogramSeries[i]
		series.Lock()
		ok, chunkCreated := series.appendHistogram(s.T, s.H, a.appendID, a.head.chunkDiskMapper)
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()

		if ok {
			a.head.metrics.histogramSamplesTotal.Inc()
		} else {
			total--
			a.head.metrics.outOfOrderSamples.Inc()
		}
		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}
	}

	a.head.metrics.samplesAppended.Add(float64(total))
	a.head.updateMinMaxTime(a.mint, a.maxt)

	return nil
}

// append adds the sample (t, v) to the series. The caller also has to provide
// the appendID for isolation. (The appendID can be zero, which results in no
// isolation for this append.)
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
func (s *memSeries) append(t int64, v float64, appendID uint64, chunkDiskMapper *chunks.ChunkDiskMapper) (sampleInOrder, chunkCreated bool) {
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncXOR, chunkDiskMapper)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}
	s.app.Append(t, v)
	s.isHistogramSeries = false

	c.maxTime = t

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, v: v}

	if appendID > 0 {
		s.txs.add(appendID)
	}

	return true, chunkCreated
}

// appendHistogram adds the histogram.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
func (s *memSeries) appendHistogram(t int64, h *histogram.Histogram, appendID uint64, chunkDiskMapper *chunks.ChunkDiskMapper) (sampleInOrder, chunkCreated bool) {
	// Head controls the execution of recoding, so that we own the proper
	// chunk reference afterwards.  We check for Appendable before
	// appendPreprocessor because in case it ends up creating a new chunk,
	// we need to know if there was also a counter reset or not to set the
	// meta properly.
	app, _ := s.app.(*chunkenc.HistogramAppender)
	var (
		positiveInterjections, negativeInterjections []chunkenc.Interjection
		okToAppend, counterReset                     bool
	)
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncHistogram, chunkDiskMapper)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}

	if app != nil {
		positiveInterjections, negativeInterjections, okToAppend, counterReset = app.Appendable(h)
	}

	if !chunkCreated {
		// We have 3 cases here
		// - !okToAppend -> We need to cut a new chunk.
		// - okToAppend but we have interjections → Existing chunk needs
		//   recoding before we can append our histogram.
		// - okToAppend and no interjections → Chunk is ready to support our histogram.
		if !okToAppend || counterReset {
			c = s.cutNewHeadChunk(t, chunkenc.EncHistogram, chunkDiskMapper)
			chunkCreated = true
		} else if len(positiveInterjections) > 0 || len(negativeInterjections) > 0 {
			// New buckets have appeared. We need to recode all
			// prior histogram samples within the chunk before we
			// can process this one.
			chunk, app := app.Recode(
				positiveInterjections, negativeInterjections,
				h.PositiveSpans, h.NegativeSpans,
			)
			c.chunk = chunk
			s.app = app
		}
	}

	if chunkCreated {
		hc := s.headChunk.chunk.(*chunkenc.HistogramChunk)
		header := chunkenc.UnknownCounterReset
		if counterReset {
			header = chunkenc.CounterReset
		} else if okToAppend {
			header = chunkenc.NotCounterReset
		}
		hc.SetCounterResetHeader(header)
	}

	s.app.AppendHistogram(t, h)
	s.isHistogramSeries = true

	c.maxTime = t

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, h: h}

	if appendID > 0 {
		s.txs.add(appendID)
	}

	return true, chunkCreated
}

// appendPreprocessor takes care of cutting new chunks and m-mapping old chunks.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
// This should be called only when appending data.
func (s *memSeries) appendPreprocessor(
	t int64, e chunkenc.Encoding, chunkDiskMapper *chunks.ChunkDiskMapper,
) (c *memChunk, sampleInOrder, chunkCreated bool) {
	// Based on Gorilla white papers this offers near-optimal compression ratio
	// so anything bigger that this has diminishing returns and increases
	// the time range within which we have to decompress all samples.
	const samplesPerChunk = 120

	c = s.head()

	if c == nil {
		if len(s.mmappedChunks) > 0 && s.mmappedChunks[len(s.mmappedChunks)-1].maxTime >= t {
			// Out of order sample. Sample timestamp is already in the mmapped chunks, so ignore it.
			return c, false, false
		}
		// There is no chunk in this series yet, create the first chunk for the sample.
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper)
		chunkCreated = true
	}

	// Out of order sample.
	if c.maxTime >= t {
		return c, false, chunkCreated
	}

	if c.chunk.Encoding() != e {
		// The chunk encoding expected by this append is different than the head chunk's
		// encoding. So we cut a new chunk with the expected encoding.
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper)
		chunkCreated = true
	}

	numSamples := c.chunk.NumSamples()
	if numSamples == 0 {
		// It could be the new chunk created after reading the chunk snapshot,
		// hence we fix the minTime of the chunk here.
		c.minTime = t
		s.nextAt = rangeForTimestamp(c.minTime, s.chunkRange)
	}

	// If we reach 25% of a chunk's desired sample count, predict an end time
	// for this chunk that will try to make samples equally distributed within
	// the remaining chunks in the current chunk range.
	// At latest it must happen at the timestamp set when the chunk was cut.
	if numSamples == samplesPerChunk/4 {
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, s.nextAt)
	}
	// If numSamples > samplesPerChunk*2 then our previous prediction was invalid,
	// most likely because samples rate has changed and now they are arriving more frequently.
	// Since we assume that the rate is higher, we're being conservative and cutting at 2*samplesPerChunk
	// as we expect more chunks to come.
	// Note that next chunk will have its nextAt recalculated for the new rate.
	if t >= s.nextAt || numSamples >= samplesPerChunk*2 {
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper)
		chunkCreated = true
	}
	return c, true, chunkCreated
}

// computeChunkEndTime estimates the end timestamp based the beginning of a
// chunk, its current timestamp and the upper bound up to which we insert data.
// It assumes that the time range is 1/4 full.
// Assuming that the samples will keep arriving at the same rate, it will make the
// remaining n chunks within this chunk range (before max) equally sized.
func computeChunkEndTime(start, cur, max int64) int64 {
	n := (max - start) / ((cur - start + 1) * 4)
	if n <= 1 {
		return max
	}
	return start + (max-start)/n
}

func (s *memSeries) cutNewHeadChunk(
	mint int64, e chunkenc.Encoding, chunkDiskMapper *chunks.ChunkDiskMapper,
) *memChunk {
	s.mmapCurrentHeadChunk(chunkDiskMapper)

	s.headChunk = &memChunk{
		minTime: mint,
		maxTime: math.MinInt64,
	}

	if chunkenc.IsValidEncoding(e) {
		var err error
		s.headChunk.chunk, err = chunkenc.NewEmptyChunk(e)
		if err != nil {
			panic(err) // This should never happen.
		}
	} else {
		s.headChunk.chunk = chunkenc.NewXORChunk()
	}

	// Set upper bound on when the next chunk must be started. An earlier timestamp
	// may be chosen dynamically at a later point.
	s.nextAt = rangeForTimestamp(mint, s.chunkRange)

	app, err := s.headChunk.chunk.Appender()
	if err != nil {
		panic(err)
	}
	s.app = app
	return s.headChunk
}

func (s *memSeries) mmapCurrentHeadChunk(chunkDiskMapper *chunks.ChunkDiskMapper) {
	if s.headChunk == nil || s.headChunk.chunk.NumSamples() == 0 {
		// There is no head chunk, so nothing to m-map here.
		return
	}

	chunkRef := chunkDiskMapper.WriteChunk(s.ref, s.headChunk.minTime, s.headChunk.maxTime, s.headChunk.chunk, handleChunkWriteError)
	s.mmappedChunks = append(s.mmappedChunks, &mmappedChunk{
		ref:        chunkRef,
		numSamples: uint16(s.headChunk.chunk.NumSamples()),
		minTime:    s.headChunk.minTime,
		maxTime:    s.headChunk.maxTime,
	})
}

func handleChunkWriteError(err error) {
	if err != nil && err != chunks.ErrChunkDiskMapperClosed {
		panic(err)
	}
}

// Rollback removes the samples and exemplars from headAppender and writes any series to WAL.
func (a *headAppender) Rollback() (err error) {
	if a.closed {
		return ErrAppenderClosed
	}
	defer func() { a.closed = true }()
	defer a.head.metrics.activeAppenders.Dec()
	defer a.head.iso.closeAppend(a.appendID)
	defer a.head.putSeriesBuffer(a.sampleSeries)

	var series *memSeries
	for i := range a.samples {
		series = a.sampleSeries[i]
		series.Lock()
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()
	}
	for i := range a.histograms {
		series = a.histogramSeries[i]
		series.Lock()
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()
	}
	a.head.putAppendBuffer(a.samples)
	a.head.putExemplarBuffer(a.exemplars)
	a.head.putHistogramBuffer(a.histograms)
	a.samples = nil
	a.exemplars = nil
	a.histograms = nil

	// Series are created in the head memory regardless of rollback. Thus we have
	// to log them to the WAL in any case.
	return a.log()
}
