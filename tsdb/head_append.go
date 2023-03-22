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
	"github.com/prometheus/prometheus/model/metadata"
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

func (a *initAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.AppendHistogram(ref, l, t, h, fh)
	}
	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.AppendHistogram(ref, l, t, h, fh)
}

func (a *initAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.UpdateMetadata(ref, l, m)
	}

	a.app = a.head.appender()
	return a.app.UpdateMetadata(ref, l, m)
}

// initTime initializes a head with the first timestamp. This only needs to be called
// for a completely fresh head with an empty WAL.
func (h *Head) initTime(t int64) {
	if !h.minTime.CompareAndSwap(math.MaxInt64, t) {
		return
	}
	// Ensure that max time is initialized to at least the min time we just set.
	// Concurrent appenders may already have set it to a higher value.
	h.maxTime.CompareAndSwap(math.MinInt64, t)
}

func (a *initAppender) GetRef(lset labels.Labels, hash uint64) (storage.SeriesRef, labels.Labels) {
	if g, ok := a.app.(storage.GetRef); ok {
		return g.GetRef(lset, hash)
	}
	return 0, labels.EmptyLabels()
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
	minValidTime := h.appendableMinValidTime()
	appendID, cleanupAppendIDsBelow := h.iso.newAppendID(minValidTime) // Every appender gets an ID that is cleared upon commit/rollback.

	// Allocate the exemplars buffer only if exemplars are enabled.
	var exemplarsBuf []exemplarWithSeriesRef
	if h.opts.EnableExemplarStorage {
		exemplarsBuf = h.getExemplarBuffer()
	}

	return &headAppender{
		head:                  h,
		minValidTime:          minValidTime,
		mint:                  math.MaxInt64,
		maxt:                  math.MinInt64,
		headMaxt:              h.MaxTime(),
		oooTimeWindow:         h.opts.OutOfOrderTimeWindow.Load(),
		samples:               h.getAppendBuffer(),
		sampleSeries:          h.getSeriesBuffer(),
		exemplars:             exemplarsBuf,
		histograms:            h.getHistogramBuffer(),
		floatHistograms:       h.getFloatHistogramBuffer(),
		metadata:              h.getMetadataBuffer(),
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

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
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

func (h *Head) getHistogramBuffer() []record.RefHistogramSample {
	b := h.histogramsPool.Get()
	if b == nil {
		return make([]record.RefHistogramSample, 0, 512)
	}
	return b.([]record.RefHistogramSample)
}

func (h *Head) putHistogramBuffer(b []record.RefHistogramSample) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.histogramsPool.Put(b[:0])
}

func (h *Head) getFloatHistogramBuffer() []record.RefFloatHistogramSample {
	b := h.floatHistogramsPool.Get()
	if b == nil {
		return make([]record.RefFloatHistogramSample, 0, 512)
	}
	return b.([]record.RefFloatHistogramSample)
}

func (h *Head) putFloatHistogramBuffer(b []record.RefFloatHistogramSample) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.floatHistogramsPool.Put(b[:0])
}

func (h *Head) getMetadataBuffer() []record.RefMetadata {
	b := h.metadataPool.Get()
	if b == nil {
		return make([]record.RefMetadata, 0, 512)
	}
	return b.([]record.RefMetadata)
}

func (h *Head) putMetadataBuffer(b []record.RefMetadata) {
	//nolint:staticcheck // Ignore SA6002 safe to ignore and actually fixing it has some performance penalty.
	h.metadataPool.Put(b[:0])
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
	head          *Head
	minValidTime  int64 // No samples below this timestamp are allowed.
	mint, maxt    int64
	headMaxt      int64 // We track it here to not take the lock for every sample appended.
	oooTimeWindow int64 // Use the same for the entire append, and don't load the atomic for each sample.

	series               []record.RefSeries               // New series held by this appender.
	samples              []record.RefSample               // New float samples held by this appender.
	sampleSeries         []*memSeries                     // Float series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	histograms           []record.RefHistogramSample      // New histogram samples held by this appender.
	histogramSeries      []*memSeries                     // HistogramSamples series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	floatHistograms      []record.RefFloatHistogramSample // New float histogram samples held by this appender.
	floatHistogramSeries []*memSeries                     // FloatHistogramSamples series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	metadata             []record.RefMetadata             // New metadata held by this appender.
	metadataSeries       []*memSeries                     // Series corresponding to the metadata held by this appender.
	exemplars            []exemplarWithSeriesRef          // New exemplars held by this appender.

	appendID, cleanupAppendIDsBelow uint64
	closed                          bool
}

func (a *headAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	// For OOO inserts, this restriction is irrelevant and will be checked later once we confirm the sample is an in-order append.
	// If OOO inserts are disabled, we may as well as check this as early as we can and avoid more work.
	if a.oooTimeWindow == 0 && t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
		return 0, storage.ErrOutOfBounds
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		// Ensure no empty labels have gotten through.
		lset = lset.WithoutEmpty()
		if lset.IsEmpty() {
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

	if value.IsStaleNaN(v) {
		if s.lastHistogramValue != nil {
			return a.AppendHistogram(ref, lset, t, &histogram.Histogram{Sum: v}, nil)
		} else if s.lastFloatHistogramValue != nil {
			return a.AppendHistogram(ref, lset, t, nil, &histogram.FloatHistogram{Sum: v})
		}
	}

	s.Lock()
	// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
	// to skip that sample from the WAL and write only in the WBL.
	_, delta, err := s.appendable(t, v, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if err == nil {
		s.pendingCommit = true
	}
	s.Unlock()
	if delta > 0 {
		a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
	}
	if err != nil {
		switch err {
		case storage.ErrOutOfOrderSample:
			a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
		case storage.ErrTooOldSample:
			a.head.metrics.tooOldSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
		}
		return 0, err
	}

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

// appendable checks whether the given sample is valid for appending to the series. (if we return false and no error)
// The sample belongs to the out of order chunk if we return true and no error.
// An error signifies the sample cannot be handled.
func (s *memSeries) appendable(t int64, v float64, headMaxt, minValidTime, oooTimeWindow int64) (isOOO bool, oooDelta int64, err error) {
	// Check if we can append in the in-order chunk.
	if t >= minValidTime {
		if s.head() == nil {
			// The series has no sample and was freshly created.
			return false, 0, nil
		}
		msMaxt := s.maxTime()
		if t > msMaxt {
			return false, 0, nil
		}
		if t == msMaxt {
			// We are allowing exact duplicates as we can encounter them in valid cases
			// like federation and erroring out at that time would be extremely noisy.
			// This only checks against the latest in-order sample.
			// The OOO headchunk has its own method to detect these duplicates.
			if math.Float64bits(s.lastValue) != math.Float64bits(v) {
				return false, 0, storage.ErrDuplicateSampleForTimestamp
			}
			// Sample is identical (ts + value) with most current (highest ts) sample in sampleBuf.
			return false, 0, nil
		}
	}

	// The sample cannot go in the in-order chunk. Check if it can go in the out-of-order chunk.
	if oooTimeWindow > 0 && t >= headMaxt-oooTimeWindow {
		return true, headMaxt - t, nil
	}

	// The sample cannot go in both in-order and out-of-order chunk.
	if oooTimeWindow > 0 {
		return true, headMaxt - t, storage.ErrTooOldSample
	}
	if t < minValidTime {
		return false, headMaxt - t, storage.ErrOutOfBounds
	}
	return false, headMaxt - t, storage.ErrOutOfOrderSample
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

	// We are allowing exact duplicates as we can encounter them in valid cases
	// like federation and erroring out at that time would be extremely noisy.
	if !h.Equals(s.lastHistogramValue) {
		return storage.ErrDuplicateSampleForTimestamp
	}
	return nil
}

// appendableFloatHistogram checks whether the given sample is valid for appending to the series.
func (s *memSeries) appendableFloatHistogram(t int64, fh *histogram.FloatHistogram) error {
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
	if !fh.Equals(s.lastFloatHistogramValue) {
		return storage.ErrDuplicateSampleForTimestamp
	}
	return nil
}

// AppendExemplar for headAppender assumes the series ref already exists, and so it doesn't
// use getOrCreate or make any of the lset validity checks that Append does.
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

func (a *headAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if !a.head.opts.EnableNativeHistograms.Load() {
		return 0, storage.ErrNativeHistogramsDisabled
	}

	if t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
		return 0, storage.ErrOutOfBounds
	}

	if h != nil {
		if err := ValidateHistogram(h); err != nil {
			return 0, err
		}
	}

	if fh != nil {
		if err := ValidateFloatHistogram(fh); err != nil {
			return 0, err
		}
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		// Ensure no empty labels have gotten through.
		lset = lset.WithoutEmpty()
		if lset.IsEmpty() {
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
			if h != nil {
				s.lastHistogramValue = &histogram.Histogram{}
			} else if fh != nil {
				s.lastFloatHistogramValue = &histogram.FloatHistogram{}
			}
			a.series = append(a.series, record.RefSeries{
				Ref:    s.ref,
				Labels: lset,
			})
		}
	}

	if h != nil {
		s.Lock()
		if err := s.appendableHistogram(t, h); err != nil {
			s.Unlock()
			if err == storage.ErrOutOfOrderSample {
				a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			}
			return 0, err
		}
		s.pendingCommit = true
		s.Unlock()
		a.histograms = append(a.histograms, record.RefHistogramSample{
			Ref: s.ref,
			T:   t,
			H:   h,
		})
		a.histogramSeries = append(a.histogramSeries, s)
	} else if fh != nil {
		s.Lock()
		if err := s.appendableFloatHistogram(t, fh); err != nil {
			s.Unlock()
			if err == storage.ErrOutOfOrderSample {
				a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			}
			return 0, err
		}
		s.pendingCommit = true
		s.Unlock()
		a.floatHistograms = append(a.floatHistograms, record.RefFloatHistogramSample{
			Ref: s.ref,
			T:   t,
			FH:  fh,
		})
		a.floatHistogramSeries = append(a.floatHistogramSeries, s)
	}

	if t < a.mint {
		a.mint = t
	}
	if t > a.maxt {
		a.maxt = t
	}

	return storage.SeriesRef(s.ref), nil
}

// UpdateMetadata for headAppender assumes the series ref already exists, and so it doesn't
// use getOrCreate or make any of the lset sanity checks that Append does.
func (a *headAppender) UpdateMetadata(ref storage.SeriesRef, lset labels.Labels, meta metadata.Metadata) (storage.SeriesRef, error) {
	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		s = a.head.series.getByHash(lset.Hash(), lset)
		if s != nil {
			ref = storage.SeriesRef(s.ref)
		}
	}
	if s == nil {
		return 0, fmt.Errorf("unknown series when trying to add metadata with HeadSeriesRef: %d and labels: %s", ref, lset)
	}

	s.RLock()
	hasNewMetadata := s.meta == nil || *s.meta != meta
	s.RUnlock()

	if hasNewMetadata {
		a.metadata = append(a.metadata, record.RefMetadata{
			Ref:  s.ref,
			Type: record.GetMetricType(meta.Type),
			Unit: meta.Unit,
			Help: meta.Help,
		})
		a.metadataSeries = append(a.metadataSeries, s)
	}

	return ref, nil
}

func ValidateHistogram(h *histogram.Histogram) error {
	if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
		return errors.Wrap(err, "negative side")
	}
	if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
		return errors.Wrap(err, "positive side")
	}
	var nCount, pCount uint64
	err := checkHistogramBuckets(h.NegativeBuckets, &nCount, true)
	if err != nil {
		return errors.Wrap(err, "negative side")
	}
	err = checkHistogramBuckets(h.PositiveBuckets, &pCount, true)
	if err != nil {
		return errors.Wrap(err, "positive side")
	}

	if c := nCount + pCount; c > h.Count {
		return errors.Wrap(
			storage.ErrHistogramCountNotBigEnough,
			fmt.Sprintf("%d observations found in buckets, but the Count field is %d", c, h.Count),
		)
	}

	return nil
}

func ValidateFloatHistogram(h *histogram.FloatHistogram) error {
	if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
		return errors.Wrap(err, "negative side")
	}
	if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
		return errors.Wrap(err, "positive side")
	}
	var nCount, pCount float64
	err := checkHistogramBuckets(h.NegativeBuckets, &nCount, false)
	if err != nil {
		return errors.Wrap(err, "negative side")
	}
	err = checkHistogramBuckets(h.PositiveBuckets, &pCount, false)
	if err != nil {
		return errors.Wrap(err, "positive side")
	}

	if c := nCount + pCount; c > h.Count {
		return errors.Wrap(
			storage.ErrHistogramCountNotBigEnough,
			fmt.Sprintf("%f observations found in buckets, but the Count field is %f", c, h.Count),
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

func checkHistogramBuckets[BC histogram.BucketCount, IBC histogram.InternalBucketCount](buckets []IBC, count *BC, deltas bool) error {
	if len(buckets) == 0 {
		return nil
	}

	var last IBC
	for i := 0; i < len(buckets); i++ {
		var c IBC
		if deltas {
			c = last + buckets[i]
		} else {
			c = buckets[i]
		}
		if c < 0 {
			return errors.Wrap(
				storage.ErrHistogramNegativeBucketCount,
				fmt.Sprintf("bucket number %d has observation count of %v", i+1, c),
			)
		}
		last = c
		*count += BC(c)
	}

	return nil
}

var _ storage.GetRef = &headAppender{}

func (a *headAppender) GetRef(lset labels.Labels, hash uint64) (storage.SeriesRef, labels.Labels) {
	s := a.head.series.getByHash(hash, lset)
	if s == nil {
		return 0, labels.EmptyLabels()
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
	if len(a.metadata) > 0 {
		rec = enc.Metadata(a.metadata, buf)
		buf = rec[:0]

		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log metadata")
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
		rec = enc.HistogramSamples(a.histograms, buf)
		buf = rec[:0]
		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log histograms")
		}
	}
	if len(a.floatHistograms) > 0 {
		rec = enc.FloatHistogramSamples(a.floatHistograms, buf)
		buf = rec[:0]
		if err := a.head.wal.Log(rec); err != nil {
			return errors.Wrap(err, "log float histograms")
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
// TODO(codesome): Refactor this method to reduce indentation and make it more readable.
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
	defer a.head.putFloatHistogramBuffer(a.floatHistograms)
	defer a.head.putMetadataBuffer(a.metadata)
	defer a.head.iso.closeAppend(a.appendID)

	var (
		samplesAppended = len(a.samples)
		oooAccepted     int   // number of samples out of order but accepted: with ooo enabled and within time window
		oooRejected     int   // number of samples rejected due to: out of order but OOO support disabled.
		tooOldRejected  int   // number of samples rejected due to: that are out of order but too old (OOO support enabled, but outside time window)
		oobRejected     int   // number of samples rejected due to: out of bounds: with t < minValidTime (OOO support disabled)
		inOrderMint     int64 = math.MaxInt64
		inOrderMaxt     int64 = math.MinInt64
		ooomint         int64 = math.MaxInt64
		ooomaxt         int64 = math.MinInt64
		wblSamples      []record.RefSample
		oooMmapMarkers  map[chunks.HeadSeriesRef]chunks.ChunkDiskMapperRef
		oooRecords      [][]byte
		oooCapMax       = a.head.opts.OutOfOrderCapMax.Load()
		chunkRange      = a.head.chunkRange.Load()
		series          *memSeries
		enc             record.Encoder
	)
	defer func() {
		for i := range oooRecords {
			a.head.putBytesBuffer(oooRecords[i][:0])
		}
	}()
	collectOOORecords := func() {
		if a.head.wbl == nil {
			// WBL is not enabled. So no need to collect.
			wblSamples = nil
			oooMmapMarkers = nil
			return
		}
		// The m-map happens before adding a new sample. So we collect
		// the m-map markers first, and then samples.
		// WBL Graphically:
		//   WBL Before this Commit(): [old samples before this commit for chunk 1]
		//   WBL After this Commit():  [old samples before this commit for chunk 1][new samples in this commit for chunk 1]mmapmarker1[samples for chunk 2]mmapmarker2[samples for chunk 3]
		if oooMmapMarkers != nil {
			markers := make([]record.RefMmapMarker, 0, len(oooMmapMarkers))
			for ref, mmapRef := range oooMmapMarkers {
				markers = append(markers, record.RefMmapMarker{
					Ref:     ref,
					MmapRef: mmapRef,
				})
			}
			r := enc.MmapMarkers(markers, a.head.getBytesBuffer())
			oooRecords = append(oooRecords, r)
		}

		if len(wblSamples) > 0 {
			r := enc.Samples(wblSamples, a.head.getBytesBuffer())
			oooRecords = append(oooRecords, r)
		}

		wblSamples = nil
		oooMmapMarkers = nil
	}
	for i, s := range a.samples {
		series = a.sampleSeries[i]
		series.Lock()

		oooSample, _, err := series.appendable(s.T, s.V, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		switch err {
		case storage.ErrOutOfOrderSample:
			samplesAppended--
			oooRejected++
		case storage.ErrOutOfBounds:
			samplesAppended--
			oobRejected++
		case storage.ErrTooOldSample:
			samplesAppended--
			tooOldRejected++
		case nil:
			// Do nothing.
		default:
			samplesAppended--
		}

		var ok, chunkCreated bool

		if err == nil && oooSample {
			// Sample is OOO and OOO handling is enabled
			// and the delta is within the OOO tolerance.
			var mmapRef chunks.ChunkDiskMapperRef
			ok, chunkCreated, mmapRef = series.insert(s.T, s.V, a.head.chunkDiskMapper, oooCapMax)
			if chunkCreated {
				r, ok := oooMmapMarkers[series.ref]
				if !ok || r != 0 {
					// !ok means there are no markers collected for these samples yet. So we first flush the samples
					// before setting this m-map marker.

					// r != 0 means we have already m-mapped a chunk for this series in the same Commit().
					// Hence, before we m-map again, we should add the samples and m-map markers
					// seen till now to the WBL records.
					collectOOORecords()
				}

				if oooMmapMarkers == nil {
					oooMmapMarkers = make(map[chunks.HeadSeriesRef]chunks.ChunkDiskMapperRef)
				}
				oooMmapMarkers[series.ref] = mmapRef
			}
			if ok {
				wblSamples = append(wblSamples, s)
				if s.T < ooomint {
					ooomint = s.T
				}
				if s.T > ooomaxt {
					ooomaxt = s.T
				}
				oooAccepted++
			} else {
				// Sample is an exact duplicate of the last sample.
				// NOTE: We can only detect updates if they clash with a sample in the OOOHeadChunk,
				// not with samples in already flushed OOO chunks.
				// TODO(codesome): Add error reporting? It depends on addressing https://github.com/prometheus/prometheus/discussions/10305.
				samplesAppended--
			}
		} else if err == nil {
			ok, chunkCreated = series.append(s.T, s.V, a.appendID, a.head.chunkDiskMapper, chunkRange)
			if ok {
				if s.T < inOrderMint {
					inOrderMint = s.T
				}
				if s.T > inOrderMaxt {
					inOrderMaxt = s.T
				}
			} else {
				// The sample is an exact duplicate, and should be silently dropped.
				samplesAppended--
			}
		}

		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}

		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()
	}

	histogramsTotal := len(a.histograms)
	histoOOORejected := 0
	for i, s := range a.histograms {
		series = a.histogramSeries[i]
		series.Lock()
		ok, chunkCreated := series.appendHistogram(s.T, s.H, a.appendID, a.head.chunkDiskMapper, chunkRange)
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()

		if ok {
			if s.T < inOrderMint {
				inOrderMint = s.T
			}
			if s.T > inOrderMaxt {
				inOrderMaxt = s.T
			}
		} else {
			histogramsTotal--
			histoOOORejected++
		}
		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}
	}

	histogramsTotal += len(a.floatHistograms)
	for i, s := range a.floatHistograms {
		series = a.floatHistogramSeries[i]
		series.Lock()
		ok, chunkCreated := series.appendFloatHistogram(s.T, s.FH, a.appendID, a.head.chunkDiskMapper, chunkRange)
		series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
		series.pendingCommit = false
		series.Unlock()

		if ok {
			if s.T < inOrderMint {
				inOrderMint = s.T
			}
			if s.T > inOrderMaxt {
				inOrderMaxt = s.T
			}
		} else {
			histogramsTotal--
			histoOOORejected++
		}
		if chunkCreated {
			a.head.metrics.chunks.Inc()
			a.head.metrics.chunksCreated.Inc()
		}
	}

	for i, m := range a.metadata {
		series = a.metadataSeries[i]
		series.Lock()
		series.meta = &metadata.Metadata{Type: record.ToTextparseMetricType(m.Type), Unit: m.Unit, Help: m.Help}
		series.Unlock()
	}

	a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(oooRejected))
	a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Add(float64(histoOOORejected))
	a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(oobRejected))
	a.head.metrics.tooOldSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(tooOldRejected))
	a.head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeFloat).Add(float64(samplesAppended))
	a.head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram).Add(float64(histogramsTotal))
	a.head.metrics.outOfOrderSamplesAppended.Add(float64(oooAccepted))
	a.head.updateMinMaxTime(inOrderMint, inOrderMaxt)
	a.head.updateMinOOOMaxOOOTime(ooomint, ooomaxt)

	collectOOORecords()
	if a.head.wbl != nil {
		if err := a.head.wbl.Log(oooRecords...); err != nil {
			// TODO(codesome): Currently WBL logging of ooo samples is best effort here since we cannot try logging
			// until we have found what samples become OOO. We can try having a metric for this failure.
			// Returning the error here is not correct because we have already put the samples into the memory,
			// hence the append/insert was a success.
			level.Error(a.head.logger).Log("msg", "Failed to log out of order samples into the WAL", "err", err)
		}
	}
	return nil
}

// insert is like append, except it inserts. Used for OOO samples.
func (s *memSeries) insert(t int64, v float64, chunkDiskMapper chunkDiskMapper, oooCapMax int64) (inserted, chunkCreated bool, mmapRef chunks.ChunkDiskMapperRef) {
	if s.ooo == nil {
		s.ooo = &memSeriesOOOFields{}
	}
	c := s.ooo.oooHeadChunk
	if c == nil || c.chunk.NumSamples() == int(oooCapMax) {
		// Note: If no new samples come in then we rely on compaction to clean up stale in-memory OOO chunks.
		c, mmapRef = s.cutNewOOOHeadChunk(t, chunkDiskMapper)
		chunkCreated = true
	}

	ok := c.chunk.Insert(t, v)
	if ok {
		if chunkCreated || t < c.minTime {
			c.minTime = t
		}
		if chunkCreated || t > c.maxTime {
			c.maxTime = t
		}
	}
	return ok, chunkCreated, mmapRef
}

// append adds the sample (t, v) to the series. The caller also has to provide
// the appendID for isolation. (The appendID can be zero, which results in no
// isolation for this append.)
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
func (s *memSeries) append(t int64, v float64, appendID uint64, chunkDiskMapper chunkDiskMapper, chunkRange int64) (sampleInOrder, chunkCreated bool) {
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncXOR, chunkDiskMapper, chunkRange)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}
	s.app.Append(t, v)

	c.maxTime = t

	s.lastValue = v
	s.lastHistogramValue = nil
	s.lastFloatHistogramValue = nil

	if appendID > 0 {
		s.txs.add(appendID)
	}

	return true, chunkCreated
}

// appendHistogram adds the histogram.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
func (s *memSeries) appendHistogram(t int64, h *histogram.Histogram, appendID uint64, chunkDiskMapper chunkDiskMapper, chunkRange int64) (sampleInOrder, chunkCreated bool) {
	// Head controls the execution of recoding, so that we own the proper
	// chunk reference afterwards. We check for Appendable from appender before
	// appendPreprocessor because in case it ends up creating a new chunk,
	// we need to know if there was also a counter reset or not to set the
	// meta properly.
	app, _ := s.app.(*chunkenc.HistogramAppender)
	var (
		pForwardInserts, nForwardInserts   []chunkenc.Insert
		pBackwardInserts, nBackwardInserts []chunkenc.Insert
		pMergedSpans, nMergedSpans         []histogram.Span
		okToAppend, counterReset, gauge    bool
	)
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncHistogram, chunkDiskMapper, chunkRange)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}
	switch h.CounterResetHint {
	case histogram.GaugeType:
		gauge = true
		if app != nil {
			pForwardInserts, nForwardInserts,
				pBackwardInserts, nBackwardInserts,
				pMergedSpans, nMergedSpans,
				okToAppend = app.AppendableGauge(h)
		}
	case histogram.CounterReset:
		// The caller tells us this is a counter reset, even if it
		// doesn't look like one.
		counterReset = true
	default:
		if app != nil {
			pForwardInserts, nForwardInserts, okToAppend, counterReset = app.Appendable(h)
		}
	}

	if !chunkCreated {
		if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
			h.PositiveSpans = pMergedSpans
			h.NegativeSpans = nMergedSpans
			app.RecodeHistogram(h, pBackwardInserts, nBackwardInserts)
		}
		// We have 3 cases here
		// - !okToAppend -> We need to cut a new chunk.
		// - okToAppend but we have inserts → Existing chunk needs
		//   recoding before we can append our histogram.
		// - okToAppend and no inserts → Chunk is ready to support our histogram.
		if !okToAppend || counterReset {
			c = s.cutNewHeadChunk(t, chunkenc.EncHistogram, chunkDiskMapper, chunkRange)
			chunkCreated = true
		} else if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
			// New buckets have appeared. We need to recode all
			// prior histogram samples within the chunk before we
			// can process this one.
			chunk, app := app.Recode(
				pForwardInserts, nForwardInserts,
				h.PositiveSpans, h.NegativeSpans,
			)
			c.chunk = chunk
			s.app = app
		}
	}

	if chunkCreated {
		hc := s.headChunk.chunk.(*chunkenc.HistogramChunk)
		header := chunkenc.UnknownCounterReset
		switch {
		case gauge:
			header = chunkenc.GaugeType
		case counterReset:
			header = chunkenc.CounterReset
		case okToAppend:
			header = chunkenc.NotCounterReset
		}
		hc.SetCounterResetHeader(header)
	}

	s.app.AppendHistogram(t, h)

	c.maxTime = t

	s.lastHistogramValue = h
	s.lastFloatHistogramValue = nil

	if appendID > 0 {
		s.txs.add(appendID)
	}

	return true, chunkCreated
}

// appendFloatHistogram adds the float histogram.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
func (s *memSeries) appendFloatHistogram(t int64, fh *histogram.FloatHistogram, appendID uint64, chunkDiskMapper chunkDiskMapper, chunkRange int64) (sampleInOrder, chunkCreated bool) {
	// Head controls the execution of recoding, so that we own the proper
	// chunk reference afterwards.  We check for Appendable from appender before
	// appendPreprocessor because in case it ends up creating a new chunk,
	// we need to know if there was also a counter reset or not to set the
	// meta properly.
	app, _ := s.app.(*chunkenc.FloatHistogramAppender)
	var (
		pForwardInserts, nForwardInserts   []chunkenc.Insert
		pBackwardInserts, nBackwardInserts []chunkenc.Insert
		pMergedSpans, nMergedSpans         []histogram.Span
		okToAppend, counterReset, gauge    bool
	)
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncFloatHistogram, chunkDiskMapper, chunkRange)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}
	switch fh.CounterResetHint {
	case histogram.GaugeType:
		gauge = true
		if app != nil {
			pForwardInserts, nForwardInserts,
				pBackwardInserts, nBackwardInserts,
				pMergedSpans, nMergedSpans,
				okToAppend = app.AppendableGauge(fh)
		}
	case histogram.CounterReset:
		// The caller tells us this is a counter reset, even if it
		// doesn't look like one.
		counterReset = true
	default:
		if app != nil {
			pForwardInserts, nForwardInserts, okToAppend, counterReset = app.Appendable(fh)
		}
	}

	if !chunkCreated {
		if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
			fh.PositiveSpans = pMergedSpans
			fh.NegativeSpans = nMergedSpans
			app.RecodeHistogramm(fh, pBackwardInserts, nBackwardInserts)
		}
		// We have 3 cases here
		// - !okToAppend -> We need to cut a new chunk.
		// - okToAppend but we have inserts → Existing chunk needs
		//   recoding before we can append our histogram.
		// - okToAppend and no inserts → Chunk is ready to support our histogram.
		if !okToAppend || counterReset {
			c = s.cutNewHeadChunk(t, chunkenc.EncFloatHistogram, chunkDiskMapper, chunkRange)
			chunkCreated = true
		} else if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
			// New buckets have appeared. We need to recode all
			// prior histogram samples within the chunk before we
			// can process this one.
			chunk, app := app.Recode(
				pForwardInserts, nForwardInserts,
				fh.PositiveSpans, fh.NegativeSpans,
			)
			c.chunk = chunk
			s.app = app
		}
	}

	if chunkCreated {
		hc := s.headChunk.chunk.(*chunkenc.FloatHistogramChunk)
		header := chunkenc.UnknownCounterReset
		switch {
		case gauge:
			header = chunkenc.GaugeType
		case counterReset:
			header = chunkenc.CounterReset
		case okToAppend:
			header = chunkenc.NotCounterReset
		}
		hc.SetCounterResetHeader(header)
	}

	s.app.AppendFloatHistogram(t, fh)

	c.maxTime = t

	s.lastFloatHistogramValue = fh
	s.lastHistogramValue = nil

	if appendID > 0 {
		s.txs.add(appendID)
	}

	return true, chunkCreated
}

// appendPreprocessor takes care of cutting new chunks and m-mapping old chunks.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
// This should be called only when appending data.
func (s *memSeries) appendPreprocessor(
	t int64, e chunkenc.Encoding, chunkDiskMapper chunkDiskMapper, chunkRange int64,
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
		// There is no head chunk in this series yet, create the first chunk for the sample.
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper, chunkRange)
		chunkCreated = true
	}

	// Out of order sample.
	if c.maxTime >= t {
		return c, false, chunkCreated
	}

	if c.chunk.Encoding() != e {
		// The chunk encoding expected by this append is different than the head chunk's
		// encoding. So we cut a new chunk with the expected encoding.
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper, chunkRange)
		chunkCreated = true
	}

	numSamples := c.chunk.NumSamples()
	if numSamples == 0 {
		// It could be the new chunk created after reading the chunk snapshot,
		// hence we fix the minTime of the chunk here.
		c.minTime = t
		s.nextAt = rangeForTimestamp(c.minTime, chunkRange)
	}

	// If we reach 25% of a chunk's desired sample count, predict an end time
	// for this chunk that will try to make samples equally distributed within
	// the remaining chunks in the current chunk range.
	// At latest it must happen at the timestamp set when the chunk was cut.
	if numSamples == samplesPerChunk/4 {
		maxNextAt := s.nextAt

		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, maxNextAt)
		s.nextAt = addJitterToChunkEndTime(s.shardHash, c.minTime, s.nextAt, maxNextAt, s.chunkEndTimeVariance)
	}
	// If numSamples > samplesPerChunk*2 then our previous prediction was invalid,
	// most likely because samples rate has changed and now they are arriving more frequently.
	// Since we assume that the rate is higher, we're being conservative and cutting at 2*samplesPerChunk
	// as we expect more chunks to come.
	// Note that next chunk will have its nextAt recalculated for the new rate.
	if t >= s.nextAt || numSamples >= samplesPerChunk*2 {
		c = s.cutNewHeadChunk(t, e, chunkDiskMapper, chunkRange)
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

// addJitterToChunkEndTime return chunk's nextAt applying a jitter based on the provided expected variance.
// The variance is applied to the estimated chunk duration (nextAt - chunkMinTime); the returned updated chunk
// end time is guaranteed to be between "chunkDuration - (chunkDuration*(variance/2))" to
// "chunkDuration + chunkDuration*(variance/2)", and never greater than maxNextAt.
func addJitterToChunkEndTime(seriesHash uint64, chunkMinTime, nextAt, maxNextAt int64, variance float64) int64 {
	if variance <= 0 {
		return nextAt
	}

	// Do not apply the jitter if the chunk is expected to be the last one of the chunk range.
	if nextAt >= maxNextAt {
		return nextAt
	}

	// Compute the variance to apply to the chunk end time. The variance is based on the series hash so that
	// different TSDBs ingesting the same exact samples (e.g. in a distributed system like Mimir) will have
	// the same chunks for a given period.
	chunkDuration := nextAt - chunkMinTime
	chunkDurationMaxVariance := int64(float64(chunkDuration) * variance)
	chunkDurationVariance := int64(seriesHash % uint64(chunkDurationMaxVariance))

	return min(maxNextAt, nextAt+chunkDurationVariance-(chunkDurationMaxVariance/2))
}

func (s *memSeries) cutNewHeadChunk(
	mint int64, e chunkenc.Encoding, chunkDiskMapper chunkDiskMapper, chunkRange int64,
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
	s.nextAt = rangeForTimestamp(mint, chunkRange)

	app, err := s.headChunk.chunk.Appender()
	if err != nil {
		panic(err)
	}
	s.app = app
	return s.headChunk
}

// cutNewOOOHeadChunk cuts a new OOO chunk and m-maps the old chunk.
// The caller must ensure that s.ooo is not nil.
func (s *memSeries) cutNewOOOHeadChunk(mint int64, chunkDiskMapper chunkDiskMapper) (*oooHeadChunk, chunks.ChunkDiskMapperRef) {
	ref := s.mmapCurrentOOOHeadChunk(chunkDiskMapper)

	s.ooo.oooHeadChunk = &oooHeadChunk{
		chunk:   NewOOOChunk(),
		minTime: mint,
		maxTime: math.MinInt64,
	}

	return s.ooo.oooHeadChunk, ref
}

func (s *memSeries) mmapCurrentOOOHeadChunk(chunkDiskMapper chunkDiskMapper) chunks.ChunkDiskMapperRef {
	if s.ooo == nil || s.ooo.oooHeadChunk == nil {
		// There is no head chunk, so nothing to m-map here.
		return 0
	}
	xor, _ := s.ooo.oooHeadChunk.chunk.ToXOR() // Encode to XorChunk which is more compact and implements all of the needed functionality.
	chunkRef := chunkDiskMapper.WriteChunk(s.ref, s.ooo.oooHeadChunk.minTime, s.ooo.oooHeadChunk.maxTime, xor, true, handleChunkWriteError)
	s.ooo.oooMmappedChunks = append(s.ooo.oooMmappedChunks, &mmappedChunk{
		ref:        chunkRef,
		numSamples: uint16(xor.NumSamples()),
		minTime:    s.ooo.oooHeadChunk.minTime,
		maxTime:    s.ooo.oooHeadChunk.maxTime,
	})
	s.ooo.oooHeadChunk = nil
	return chunkRef
}

func (s *memSeries) mmapCurrentHeadChunk(chunkDiskMapper chunkDiskMapper) {
	if s.headChunk == nil || s.headChunk.chunk.NumSamples() == 0 {
		// There is no head chunk, so nothing to m-map here.
		return
	}

	chunkRef := chunkDiskMapper.WriteChunk(s.ref, s.headChunk.minTime, s.headChunk.maxTime, s.headChunk.chunk, false, handleChunkWriteError)
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
	a.head.putFloatHistogramBuffer(a.floatHistograms)
	a.head.putMetadataBuffer(a.metadata)
	a.samples = nil
	a.exemplars = nil
	a.histograms = nil
	a.metadata = nil

	// Series are created in the head memory regardless of rollback. Thus we have
	// to log them to the WAL in any case.
	return a.log()
}
