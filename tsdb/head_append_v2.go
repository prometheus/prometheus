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
	"errors"
	"fmt"
	"log/slog"
	"math"

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

func (a *initAppender) SetOptions(opts *storage.AppendOptions) {
	if a.app != nil {
		a.app.SetOptions(opts)
	}
}

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

func (a *initAppender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.AppendHistogramSTZeroSample(ref, l, t, st, h, fh)
	}
	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.AppendHistogramSTZeroSample(ref, l, t, st, h, fh)
}

func (a *initAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.UpdateMetadata(ref, l, m)
	}

	a.app = a.head.appender()
	return a.app.UpdateMetadata(ref, l, m)
}

func (a *initAppender) AppendSTZeroSample(ref storage.SeriesRef, lset labels.Labels, t, st int64) (storage.SeriesRef, error) {
	if a.app != nil {
		return a.app.AppendSTZeroSample(ref, lset, t, st)
	}

	a.head.initTime(t)
	a.app = a.head.appender()

	return a.app.AppendSTZeroSample(ref, lset, t, st)
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
func (h *Head) Appender(context.Context) storage.Appender {
	h.metrics.activeAppenders.Inc()

	// The head cache might not have a starting point yet. The init appender
	// picks up the first appended timestamp as the base.
	if !h.initialized() {
		return &initAppender{
			head: h,
		}
	}
	return h.appender()
}

func (h *Head) appender() *headAppender {
	minValidTime := h.appendableMinValidTime()
	appendID, cleanupAppendIDsBelow := h.iso.newAppendID(minValidTime) // Every appender gets an ID that is cleared upon commit/rollback.
	return &headAppender{
		head:                  h,
		minValidTime:          minValidTime,
		mint:                  math.MaxInt64,
		maxt:                  math.MinInt64,
		headMaxt:              h.MaxTime(),
		oooTimeWindow:         h.opts.OutOfOrderTimeWindow.Load(),
		seriesRefs:            h.getRefSeriesBuffer(),
		series:                h.getSeriesBuffer(),
		typesInBatch:          h.getTypeMap(),
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
	if !h.initialized() {
		return 0, false
	}

	return h.appendableMinValidTime(), true
}

func (h *Head) getRefSeriesBuffer() []record.RefSeries {
	b := h.refSeriesPool.Get()
	if b == nil {
		return make([]record.RefSeries, 0, 512)
	}
	return b
}

func (h *Head) putRefSeriesBuffer(b []record.RefSeries) {
	h.refSeriesPool.Put(b[:0])
}

func (h *Head) getFloatBuffer() []record.RefSample {
	b := h.floatsPool.Get()
	if b == nil {
		return make([]record.RefSample, 0, 512)
	}
	return b
}

func (h *Head) putFloatBuffer(b []record.RefSample) {
	h.floatsPool.Put(b[:0])
}

func (h *Head) getExemplarBuffer() []exemplarWithSeriesRef {
	b := h.exemplarsPool.Get()
	if b == nil {
		return make([]exemplarWithSeriesRef, 0, 512)
	}
	return b
}

func (h *Head) putExemplarBuffer(b []exemplarWithSeriesRef) {
	if b == nil {
		return
	}
	for i := range b { // Zero out to avoid retaining label data.
		b[i].exemplar.Labels = labels.EmptyLabels()
	}

	h.exemplarsPool.Put(b[:0])
}

func (h *Head) getHistogramBuffer() []record.RefHistogramSample {
	b := h.histogramsPool.Get()
	if b == nil {
		return make([]record.RefHistogramSample, 0, 512)
	}
	return b
}

func (h *Head) putHistogramBuffer(b []record.RefHistogramSample) {
	h.histogramsPool.Put(b[:0])
}

func (h *Head) getFloatHistogramBuffer() []record.RefFloatHistogramSample {
	b := h.floatHistogramsPool.Get()
	if b == nil {
		return make([]record.RefFloatHistogramSample, 0, 512)
	}
	return b
}

func (h *Head) putFloatHistogramBuffer(b []record.RefFloatHistogramSample) {
	h.floatHistogramsPool.Put(b[:0])
}

func (h *Head) getMetadataBuffer() []record.RefMetadata {
	b := h.metadataPool.Get()
	if b == nil {
		return make([]record.RefMetadata, 0, 512)
	}
	return b
}

func (h *Head) putMetadataBuffer(b []record.RefMetadata) {
	h.metadataPool.Put(b[:0])
}

func (h *Head) getSeriesBuffer() []*memSeries {
	b := h.seriesPool.Get()
	if b == nil {
		return make([]*memSeries, 0, 512)
	}
	return b
}

func (h *Head) putSeriesBuffer(b []*memSeries) {
	for i := range b { // Zero out to avoid retaining data.
		b[i] = nil
	}
	h.seriesPool.Put(b[:0])
}

func (h *Head) getTypeMap() map[chunks.HeadSeriesRef]sampleType {
	b := h.typeMapPool.Get()
	if b == nil {
		return make(map[chunks.HeadSeriesRef]sampleType)
	}
	return b
}

func (h *Head) putTypeMap(b map[chunks.HeadSeriesRef]sampleType) {
	clear(b)
	h.typeMapPool.Put(b)
}

func (h *Head) getBytesBuffer() []byte {
	b := h.bytesPool.Get()
	if b == nil {
		return make([]byte, 0, 1024)
	}
	return b
}

func (h *Head) putBytesBuffer(b []byte) {
	h.bytesPool.Put(b[:0])
}

type exemplarWithSeriesRef struct {
	ref      storage.SeriesRef
	exemplar exemplar.Exemplar
}

// sampleType describes sample types we need to distinguish for append batching.
// We need separate types for everything that goes into a different WAL record
// type or into a different chunk encoding.
type sampleType byte

const (
	stNone                       sampleType = iota // To mark that the sample type does not matter.
	stFloat                                        // All simple floats (counters, gauges, untyped). Goes to `floats`.
	stHistogram                                    // Native integer histograms with a standard exponential schema. Goes to `histograms`.
	stCustomBucketHistogram                        // Native integer histograms with custom bucket boundaries. Goes to `histograms`.
	stFloatHistogram                               // Native float histograms. Goes to `floatHistograms`.
	stCustomBucketFloatHistogram                   // Native float histograms with custom bucket boundaries. Goes to `floatHistograms`.
)

// appendBatch is used to partition all the appended data into batches that are
// "type clean", i.e. every series receives only samples of one type within the
// batch. Types in this regard are defined by the sampleType enum above.
// TODO(beorn7): The same concept could be extended to make sure every series in
// the batch has at most one metadata record. This is currently not implemented
// because it is unclear if it is needed at all. (Maybe we will remove metadata
// records altogether, see issue #15911.)
type appendBatch struct {
	floats               []record.RefSample               // New float samples held by this appender.
	floatSeries          []*memSeries                     // Float series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	histograms           []record.RefHistogramSample      // New histogram samples held by this appender.
	histogramSeries      []*memSeries                     // HistogramSamples series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	floatHistograms      []record.RefFloatHistogramSample // New float histogram samples held by this appender.
	floatHistogramSeries []*memSeries                     // FloatHistogramSamples series corresponding to the samples held by this appender (using corresponding slice indices - same series may appear more than once).
	metadata             []record.RefMetadata             // New metadata held by this appender.
	metadataSeries       []*memSeries                     // Series corresponding to the metadata held by this appender.
	exemplars            []exemplarWithSeriesRef          // New exemplars held by this appender.
}

// close returns all the slices to the pools in Head and nil's them.
func (b *appendBatch) close(h *Head) {
	h.putFloatBuffer(b.floats)
	b.floats = nil
	h.putSeriesBuffer(b.floatSeries)
	b.floatSeries = nil
	h.putHistogramBuffer(b.histograms)
	b.histograms = nil
	h.putSeriesBuffer(b.histogramSeries)
	b.histogramSeries = nil
	h.putFloatHistogramBuffer(b.floatHistograms)
	b.floatHistograms = nil
	h.putSeriesBuffer(b.floatHistogramSeries)
	b.floatHistogramSeries = nil
	h.putMetadataBuffer(b.metadata)
	b.metadata = nil
	h.putSeriesBuffer(b.metadataSeries)
	b.metadataSeries = nil
	h.putExemplarBuffer(b.exemplars)
	b.exemplars = nil
}

type headAppender struct {
	head          *Head
	minValidTime  int64 // No samples below this timestamp are allowed.
	mint, maxt    int64
	headMaxt      int64 // We track it here to not take the lock for every sample appended.
	oooTimeWindow int64 // Use the same for the entire append, and don't load the atomic for each sample.

	seriesRefs []record.RefSeries // New series records held by this appender.
	series     []*memSeries       // New series held by this appender (using corresponding slices indexes from seriesRefs)
	batches    []*appendBatch     // Holds all the other data to append. (In regular cases, there should be only one of these.)

	typesInBatch map[chunks.HeadSeriesRef]sampleType // Which (one) sample type each series holds in the most recent batch.

	appendID, cleanupAppendIDsBelow uint64
	closed                          bool
	hints                           *storage.AppendOptions
}

func (a *headAppender) SetOptions(opts *storage.AppendOptions) {
	a.hints = opts
}

func (a *headAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	// Fail fast if OOO is disabled and the sample is out of bounds.
	// Otherwise a full check will be done later to decide if the sample is in-order or out-of-order.
	if a.oooTimeWindow == 0 && t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
		return 0, storage.ErrOutOfBounds
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, _, err = a.getOrCreate(lset)
		if err != nil {
			return 0, err
		}
	}

	if value.IsStaleNaN(v) {
		// If we have added a sample before with this same appender, we
		// can check the previously used type and turn a stale float
		// sample into a stale histogram sample or stale float histogram
		// sample as appropriate. This prevents an unnecessary creation
		// of a new batch. However, since other appenders might append
		// to the same series concurrently, this is not perfect but just
		// an optimization for the more likely case.
		switch a.typesInBatch[s.ref] {
		case stHistogram, stCustomBucketHistogram:
			return a.AppendHistogram(ref, lset, t, &histogram.Histogram{Sum: v}, nil)
		case stFloatHistogram, stCustomBucketFloatHistogram:
			return a.AppendHistogram(ref, lset, t, nil, &histogram.FloatHistogram{Sum: v})
		}
		// Note that a series reference not yet in the map will come out
		// as stNone, but since we do not handle that case separately,
		// we do not need to check for the difference between "unknown
		// series" and "known series with stNone".
	}

	s.Lock()
	defer s.Unlock()
	// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
	// to skip that sample from the WAL and write only in the WBL.
	isOOO, delta, err := s.appendable(t, v, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if err == nil {
		if isOOO && a.hints != nil && a.hints.DiscardOutOfOrder {
			a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
			return 0, storage.ErrOutOfOrderSample
		}
		s.pendingCommit = true
	}
	if delta > 0 {
		a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
	}
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrOutOfOrderSample):
			a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
		case errors.Is(err, storage.ErrTooOldSample):
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

	b := a.getCurrentBatch(stFloat, s.ref)
	b.floats = append(b.floats, record.RefSample{
		Ref: s.ref,
		T:   t,
		V:   v,
	})
	b.floatSeries = append(b.floatSeries, s)
	return storage.SeriesRef(s.ref), nil
}

// AppendSTZeroSample appends synthetic zero sample for st timestamp. It returns
// error when sample can't be appended. See
// storage.StartTimestampAppender.AppendSTZeroSample for further documentation.
func (a *headAppender) AppendSTZeroSample(ref storage.SeriesRef, lset labels.Labels, t, st int64) (storage.SeriesRef, error) {
	if st >= t {
		return 0, storage.ErrSTNewerThanSample
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, _, err = a.getOrCreate(lset)
		if err != nil {
			return 0, err
		}
	}

	// Check if ST wouldn't be OOO vs samples we already might have for this series.
	// NOTE(bwplotka): This will be often hit as it's expected for long living
	// counters to share the same ST.
	s.Lock()
	isOOO, _, err := s.appendable(st, 0, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if err == nil {
		s.pendingCommit = true
	}
	s.Unlock()
	if err != nil {
		return 0, err
	}
	if isOOO {
		return storage.SeriesRef(s.ref), storage.ErrOutOfOrderST
	}

	if st > a.maxt {
		a.maxt = st
	}
	b := a.getCurrentBatch(stFloat, s.ref)
	b.floats = append(b.floats, record.RefSample{Ref: s.ref, T: st, V: 0.0})
	b.floatSeries = append(b.floatSeries, s)
	return storage.SeriesRef(s.ref), nil
}

func (a *headAppender) getOrCreate(lset labels.Labels) (s *memSeries, created bool, err error) {
	// Ensure no empty labels have gotten through.
	lset = lset.WithoutEmpty()
	if lset.IsEmpty() {
		return nil, false, fmt.Errorf("empty labelset: %w", ErrInvalidSample)
	}
	if l, dup := lset.HasDuplicateLabelNames(); dup {
		return nil, false, fmt.Errorf(`label name "%s" is not unique: %w`, l, ErrInvalidSample)
	}
	s, created, err = a.head.getOrCreate(lset.Hash(), lset, true)
	if err != nil {
		return nil, false, err
	}
	if created {
		a.seriesRefs = append(a.seriesRefs, record.RefSeries{
			Ref:    s.ref,
			Labels: lset,
		})
		a.series = append(a.series, s)
	}
	return s, created, nil
}

// getCurrentBatch returns the current batch if it fits the provided sampleType
// for the provided series. Otherwise, it adds a new batch and returns it.
func (a *headAppender) getCurrentBatch(st sampleType, s chunks.HeadSeriesRef) *appendBatch {
	h := a.head

	newBatch := func() *appendBatch {
		b := appendBatch{
			floats:               h.getFloatBuffer(),
			floatSeries:          h.getSeriesBuffer(),
			histograms:           h.getHistogramBuffer(),
			histogramSeries:      h.getSeriesBuffer(),
			floatHistograms:      h.getFloatHistogramBuffer(),
			floatHistogramSeries: h.getSeriesBuffer(),
			metadata:             h.getMetadataBuffer(),
			metadataSeries:       h.getSeriesBuffer(),
		}

		// Allocate the exemplars buffer only if exemplars are enabled.
		if h.opts.EnableExemplarStorage {
			b.exemplars = h.getExemplarBuffer()
		}
		clear(a.typesInBatch)
		switch st {
		case stHistogram, stFloatHistogram, stCustomBucketHistogram, stCustomBucketFloatHistogram:
			// We only record histogram sample types in the map.
			// Floats are implicit.
			a.typesInBatch[s] = st
		}
		a.batches = append(a.batches, &b)
		return &b
	}

	// First batch ever. Create it.
	if len(a.batches) == 0 {
		return newBatch()
	}

	// TODO(beorn7): If we ever see that the a.typesInBatch map grows so
	// large that it matters for total memory consumption, we could limit
	// the batch size here, i.e. cut a new batch even without a type change.
	// Something like:
	//     if len(a.typesInBatch > limit) {
	//         return newBatch()
	//     }

	lastBatch := a.batches[len(a.batches)-1]
	if st == stNone {
		// Type doesn't matter, last batch will always do.
		return lastBatch
	}
	prevST, ok := a.typesInBatch[s]
	switch {
	case prevST == st:
		// An old series of some histogram type with the same type being appended.
		// Continue the batch.
		return lastBatch
	case !ok && st == stFloat:
		// A new float series, or an old float series that gets floats appended.
		// Note that we do not track stFloat in typesInBatch.
		// Continue the batch.
		return lastBatch
	case st == stFloat:
		// A float being appended to a histogram series.
		// Start a new batch.
		return newBatch()
	case !ok:
		// A new series of some histogram type, or some histogram type
		// being appended to on old float series. Even in the latter
		// case, we don't need to start a new batch because histograms
		// after floats are fine.
		// Add new sample type to the map and continue batch.
		a.typesInBatch[s] = st
		return lastBatch
	default:
		// One histogram type changed to another.
		// Start a new batch.
		return newBatch()
	}
}

// appendable checks whether the given sample is valid for appending to the series.
// If the sample is valid and in-order, it returns false with no error.
// If the sample belongs to the out-of-order chunk, it returns true with no error.
// If the sample cannot be handled, it returns an error.
func (s *memSeries) appendable(t int64, v float64, headMaxt, minValidTime, oooTimeWindow int64) (isOOO bool, oooDelta int64, err error) {
	// Check if we can append in the in-order chunk.
	if t >= minValidTime {
		if s.headChunks == nil {
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
			if s.lastHistogramValue != nil || s.lastFloatHistogramValue != nil {
				return false, 0, storage.NewDuplicateHistogramToFloatErr(t, v)
			}
			if math.Float64bits(s.lastValue) != math.Float64bits(v) {
				return false, 0, storage.NewDuplicateFloatErr(t, s.lastValue, v)
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

// appendableHistogram checks whether the given histogram sample is valid for appending to the series. (if we return false and no error)
// The sample belongs to the out of order chunk if we return true and no error.
// An error signifies the sample cannot be handled.
func (s *memSeries) appendableHistogram(t int64, h *histogram.Histogram, headMaxt, minValidTime, oooTimeWindow int64) (isOOO bool, oooDelta int64, err error) {
	// Check if we can append in the in-order chunk.
	if t >= minValidTime {
		if s.headChunks == nil {
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
			if !h.Equals(s.lastHistogramValue) {
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

// appendableFloatHistogram checks whether the given float histogram sample is valid for appending to the series. (if we return false and no error)
// The sample belongs to the out of order chunk if we return true and no error.
// An error signifies the sample cannot be handled.
func (s *memSeries) appendableFloatHistogram(t int64, fh *histogram.FloatHistogram, headMaxt, minValidTime, oooTimeWindow int64) (isOOO bool, oooDelta int64, err error) {
	// Check if we can append in the in-order chunk.
	if t >= minValidTime {
		if s.headChunks == nil {
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
			if !fh.Equals(s.lastFloatHistogramValue) {
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

	err := a.head.exemplars.ValidateExemplar(s.labels(), e)
	if err != nil {
		if errors.Is(err, storage.ErrDuplicateExemplar) || errors.Is(err, storage.ErrExemplarsDisabled) {
			// Duplicate, don't return an error but don't accept the exemplar.
			return 0, nil
		}
		return 0, err
	}

	b := a.getCurrentBatch(stNone, chunks.HeadSeriesRef(ref))
	b.exemplars = append(b.exemplars, exemplarWithSeriesRef{ref, e})

	return storage.SeriesRef(s.ref), nil
}

func (a *headAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	// Fail fast if OOO is disabled and the sample is out of bounds.
	// Otherwise a full check will be done later to decide if the sample is in-order or out-of-order.
	if a.oooTimeWindow == 0 && t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
		return 0, storage.ErrOutOfBounds
	}

	if h != nil {
		if err := h.Validate(); err != nil {
			return 0, err
		}
	}

	if fh != nil {
		if err := fh.Validate(); err != nil {
			return 0, err
		}
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, _, err = a.getOrCreate(lset)
		if err != nil {
			return 0, err
		}
	}

	switch {
	case h != nil:
		s.Lock()
		// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
		// to skip that sample from the WAL and write only in the WBL.
		_, delta, err := s.appendableHistogram(t, h, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err != nil {
			s.pendingCommit = true
		}
		s.Unlock()
		if delta > 0 {
			a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
		}
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderSample):
				a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			case errors.Is(err, storage.ErrTooOldSample):
				a.head.metrics.tooOldSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			}
			return 0, err
		}
		st := stHistogram
		if h.UsesCustomBuckets() {
			st = stCustomBucketHistogram
		}
		b := a.getCurrentBatch(st, s.ref)
		b.histograms = append(b.histograms, record.RefHistogramSample{
			Ref: s.ref,
			T:   t,
			H:   h,
		})
		b.histogramSeries = append(b.histogramSeries, s)
	case fh != nil:
		s.Lock()
		// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
		// to skip that sample from the WAL and write only in the WBL.
		_, delta, err := s.appendableFloatHistogram(t, fh, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err == nil {
			s.pendingCommit = true
		}
		s.Unlock()
		if delta > 0 {
			a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
		}
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderSample):
				a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			case errors.Is(err, storage.ErrTooOldSample):
				a.head.metrics.tooOldSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
			}
			return 0, err
		}
		st := stFloatHistogram
		if fh.UsesCustomBuckets() {
			st = stCustomBucketFloatHistogram
		}
		b := a.getCurrentBatch(st, s.ref)
		b.floatHistograms = append(b.floatHistograms, record.RefFloatHistogramSample{
			Ref: s.ref,
			T:   t,
			FH:  fh,
		})
		b.floatHistogramSeries = append(b.floatHistogramSeries, s)
	}

	if t < a.mint {
		a.mint = t
	}
	if t > a.maxt {
		a.maxt = t
	}

	return storage.SeriesRef(s.ref), nil
}

func (a *headAppender) AppendHistogramSTZeroSample(ref storage.SeriesRef, lset labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if st >= t {
		return 0, storage.ErrSTNewerThanSample
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, _, err = a.getOrCreate(lset)
		if err != nil {
			return 0, err
		}
	}

	switch {
	case h != nil:
		zeroHistogram := &histogram.Histogram{
			// The STZeroSample represents a counter reset by definition.
			CounterResetHint: histogram.CounterReset,
			// Replicate other fields to avoid needless chunk creation.
			Schema:        h.Schema,
			ZeroThreshold: h.ZeroThreshold,
			CustomValues:  h.CustomValues,
		}
		s.Lock()
		// For STZeroSamples OOO is not allowed.
		// We set it to true to make this implementation as close as possible to the float implementation.
		isOOO, _, err := s.appendableHistogram(st, zeroHistogram, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err != nil {
			s.Unlock()
			if errors.Is(err, storage.ErrOutOfOrderSample) {
				return 0, storage.ErrOutOfOrderST
			}

			return 0, err
		}

		// OOO is not allowed because after the first scrape, ST will be the same for most (if not all) future samples.
		// This is to prevent the injected zero from being marked as OOO forever.
		if isOOO {
			s.Unlock()
			return 0, storage.ErrOutOfOrderST
		}

		s.pendingCommit = true
		s.Unlock()
		sTyp := stHistogram
		if h.UsesCustomBuckets() {
			sTyp = stCustomBucketHistogram
		}
		b := a.getCurrentBatch(sTyp, s.ref)
		b.histograms = append(b.histograms, record.RefHistogramSample{
			Ref: s.ref,
			T:   st,
			H:   zeroHistogram,
		})
		b.histogramSeries = append(b.histogramSeries, s)
	case fh != nil:
		zeroFloatHistogram := &histogram.FloatHistogram{
			// The STZeroSample represents a counter reset by definition.
			CounterResetHint: histogram.CounterReset,
			// Replicate other fields to avoid needless chunk creation.
			Schema:        fh.Schema,
			ZeroThreshold: fh.ZeroThreshold,
			CustomValues:  fh.CustomValues,
		}
		s.Lock()
		// We set it to true to make this implementation as close as possible to the float implementation.
		isOOO, _, err := s.appendableFloatHistogram(st, zeroFloatHistogram, a.headMaxt, a.minValidTime, a.oooTimeWindow) // OOO is not allowed for STZeroSamples.
		if err != nil {
			s.Unlock()
			if errors.Is(err, storage.ErrOutOfOrderSample) {
				return 0, storage.ErrOutOfOrderST
			}

			return 0, err
		}

		// OOO is not allowed because after the first scrape, ST will be the same for most (if not all) future samples.
		// This is to prevent the injected zero from being marked as OOO forever.
		if isOOO {
			s.Unlock()
			return 0, storage.ErrOutOfOrderST
		}

		s.pendingCommit = true
		s.Unlock()
		sTyp := stFloatHistogram
		if fh.UsesCustomBuckets() {
			sTyp = stCustomBucketFloatHistogram
		}
		b := a.getCurrentBatch(sTyp, s.ref)
		b.floatHistograms = append(b.floatHistograms, record.RefFloatHistogramSample{
			Ref: s.ref,
			T:   st,
			FH:  zeroFloatHistogram,
		})
		b.floatHistogramSeries = append(b.floatHistogramSeries, s)
	}

	if st > a.maxt {
		a.maxt = st
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

	s.Lock()
	hasNewMetadata := s.meta == nil || *s.meta != meta
	s.Unlock()

	if hasNewMetadata {
		b := a.getCurrentBatch(stNone, s.ref)
		b.metadata = append(b.metadata, record.RefMetadata{
			Ref:  s.ref,
			Type: record.GetMetricType(meta.Type),
			Unit: meta.Unit,
			Help: meta.Help,
		})
		b.metadataSeries = append(b.metadataSeries, s)
	}

	return ref, nil
}

var _ storage.GetRef = &headAppender{}

func (a *headAppender) GetRef(lset labels.Labels, hash uint64) (storage.SeriesRef, labels.Labels) {
	s := a.head.series.getByHash(hash, lset)
	if s == nil {
		return 0, labels.EmptyLabels()
	}
	// returned labels must be suitable to pass to Append()
	return storage.SeriesRef(s.ref), s.labels()
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

	if len(a.seriesRefs) > 0 {
		rec = enc.Series(a.seriesRefs, buf)
		buf = rec[:0]

		if err := a.head.wal.Log(rec); err != nil {
			return fmt.Errorf("log series: %w", err)
		}
	}
	for _, b := range a.batches {
		if len(b.metadata) > 0 {
			rec = enc.Metadata(b.metadata, buf)
			buf = rec[:0]

			if err := a.head.wal.Log(rec); err != nil {
				return fmt.Errorf("log metadata: %w", err)
			}
		}
		// It's important to do (float) Samples before histogram samples
		// to end up with the correct order.
		if len(b.floats) > 0 {
			rec = enc.Samples(b.floats, buf)
			buf = rec[:0]

			if err := a.head.wal.Log(rec); err != nil {
				return fmt.Errorf("log samples: %w", err)
			}
		}
		if len(b.histograms) > 0 {
			var customBucketsHistograms []record.RefHistogramSample
			rec, customBucketsHistograms = enc.HistogramSamples(b.histograms, buf)
			buf = rec[:0]
			if len(rec) > 0 {
				if err := a.head.wal.Log(rec); err != nil {
					return fmt.Errorf("log histograms: %w", err)
				}
			}

			if len(customBucketsHistograms) > 0 {
				rec = enc.CustomBucketsHistogramSamples(customBucketsHistograms, buf)
				if err := a.head.wal.Log(rec); err != nil {
					return fmt.Errorf("log custom buckets histograms: %w", err)
				}
			}
		}
		if len(b.floatHistograms) > 0 {
			var customBucketsFloatHistograms []record.RefFloatHistogramSample
			rec, customBucketsFloatHistograms = enc.FloatHistogramSamples(b.floatHistograms, buf)
			buf = rec[:0]
			if len(rec) > 0 {
				if err := a.head.wal.Log(rec); err != nil {
					return fmt.Errorf("log float histograms: %w", err)
				}
			}

			if len(customBucketsFloatHistograms) > 0 {
				rec = enc.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, buf)
				if err := a.head.wal.Log(rec); err != nil {
					return fmt.Errorf("log custom buckets float histograms: %w", err)
				}
			}
		}
		// Exemplars should be logged after samples (float/native histogram/etc),
		// otherwise it might happen that we send the exemplars in a remote write
		// batch before the samples, which in turn means the exemplar is rejected
		// for missing series, since series are created due to samples.
		if len(b.exemplars) > 0 {
			rec = enc.Exemplars(exemplarsForEncoding(b.exemplars), buf)
			buf = rec[:0]

			if err := a.head.wal.Log(rec); err != nil {
				return fmt.Errorf("log exemplars: %w", err)
			}
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

type appenderCommitContext struct {
	floatsAppended     int
	histogramsAppended int
	// Number of samples out of order but accepted: with ooo enabled and within time window.
	oooFloatsAccepted    int
	oooHistogramAccepted int
	// Number of samples rejected due to: out of order but OOO support disabled.
	floatOOORejected int
	histoOOORejected int
	// Number of samples rejected due to: out of order but too old (OOO support enabled, but outside time window).
	floatTooOldRejected int
	histoTooOldRejected int
	// Number of samples rejected due to: out of bounds: with t < minValidTime (OOO support disabled).
	floatOOBRejected    int
	histoOOBRejected    int
	inOrderMint         int64
	inOrderMaxt         int64
	oooMinT             int64
	oooMaxT             int64
	wblSamples          []record.RefSample
	wblHistograms       []record.RefHistogramSample
	wblFloatHistograms  []record.RefFloatHistogramSample
	oooMmapMarkers      map[chunks.HeadSeriesRef][]chunks.ChunkDiskMapperRef
	oooMmapMarkersCount int
	oooRecords          [][]byte
	oooCapMax           int64
	appendChunkOpts     chunkOpts
	enc                 record.Encoder
}

// commitExemplars adds all exemplars from the provided batch to the head's exemplar storage.
func (a *headAppender) commitExemplars(b *appendBatch) {
	// No errors logging to WAL, so pass the exemplars along to the in memory storage.
	for _, e := range b.exemplars {
		s := a.head.series.getByID(chunks.HeadSeriesRef(e.ref))
		if s == nil {
			// This is very unlikely to happen, but we have seen it in the wild.
			// It means that the series was truncated between AppendExemplar and Commit.
			// See TestHeadCompactionWhileAppendAndCommitExemplar.
			continue
		}
		// We don't instrument exemplar appends here, all is instrumented by storage.
		if err := a.head.exemplars.AddExemplar(s.labels(), e.exemplar); err != nil {
			if errors.Is(err, storage.ErrOutOfOrderExemplar) {
				continue
			}
			a.head.logger.Debug("Unknown error while adding exemplar", "err", err)
		}
	}
}

func (acc *appenderCommitContext) collectOOORecords(a *headAppender) {
	if a.head.wbl == nil {
		// WBL is not enabled. So no need to collect.
		acc.wblSamples = nil
		acc.wblHistograms = nil
		acc.wblFloatHistograms = nil
		acc.oooMmapMarkers = nil
		acc.oooMmapMarkersCount = 0
		return
	}

	// The m-map happens before adding a new sample. So we collect
	// the m-map markers first, and then samples.
	// WBL Graphically:
	//	WBL Before this Commit(): [old samples before this commit for chunk 1]
	//	WBL After this Commit():  [old samples before this commit for chunk 1][new samples in this commit for chunk 1]mmapmarker1[samples for chunk 2]mmapmarker2[samples for chunk 3]
	if acc.oooMmapMarkers != nil {
		markers := make([]record.RefMmapMarker, 0, acc.oooMmapMarkersCount)
		for ref, mmapRefs := range acc.oooMmapMarkers {
			for _, mmapRef := range mmapRefs {
				markers = append(markers, record.RefMmapMarker{
					Ref:     ref,
					MmapRef: mmapRef,
				})
			}
		}
		r := acc.enc.MmapMarkers(markers, a.head.getBytesBuffer())
		acc.oooRecords = append(acc.oooRecords, r)
	}

	if len(acc.wblSamples) > 0 {
		r := acc.enc.Samples(acc.wblSamples, a.head.getBytesBuffer())
		acc.oooRecords = append(acc.oooRecords, r)
	}
	if len(acc.wblHistograms) > 0 {
		r, customBucketsHistograms := acc.enc.HistogramSamples(acc.wblHistograms, a.head.getBytesBuffer())
		if len(r) > 0 {
			acc.oooRecords = append(acc.oooRecords, r)
		}
		if len(customBucketsHistograms) > 0 {
			r := acc.enc.CustomBucketsHistogramSamples(customBucketsHistograms, a.head.getBytesBuffer())
			acc.oooRecords = append(acc.oooRecords, r)
		}
	}
	if len(acc.wblFloatHistograms) > 0 {
		r, customBucketsFloatHistograms := acc.enc.FloatHistogramSamples(acc.wblFloatHistograms, a.head.getBytesBuffer())
		if len(r) > 0 {
			acc.oooRecords = append(acc.oooRecords, r)
		}
		if len(customBucketsFloatHistograms) > 0 {
			r := acc.enc.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, a.head.getBytesBuffer())
			acc.oooRecords = append(acc.oooRecords, r)
		}
	}

	acc.wblSamples = nil
	acc.wblHistograms = nil
	acc.wblFloatHistograms = nil
	acc.oooMmapMarkers = nil
}

// handleAppendableError processes errors encountered during sample appending and updates
// the provided counters accordingly.
//
// Parameters:
// - err: The error encountered during appending.
// - appended: Pointer to the counter tracking the number of successfully appended samples.
// - oooRejected: Pointer to the counter tracking the number of out-of-order samples rejected.
// - oobRejected: Pointer to the counter tracking the number of out-of-bounds samples rejected.
// - tooOldRejected: Pointer to the counter tracking the number of too-old samples rejected.
func handleAppendableError(err error, appended, oooRejected, oobRejected, tooOldRejected *int) {
	switch {
	case errors.Is(err, storage.ErrOutOfOrderSample):
		*appended--
		*oooRejected++
	case errors.Is(err, storage.ErrOutOfBounds):
		*appended--
		*oobRejected++
	case errors.Is(err, storage.ErrTooOldSample):
		*appended--
		*tooOldRejected++
	default:
		*appended--
	}
}

// commitFloats processes and commits the samples in the provided batch to the
// series. It handles both in-order and out-of-order samples, updating the
// appenderCommitContext with the results of the append operations.
//
// The function iterates over the samples in the headAppender and attempts to append each sample
// to its corresponding series. It handles various error cases such as out-of-order samples,
// out-of-bounds samples, and too-old samples, updating the appenderCommitContext accordingly.
//
// For out-of-order samples, it checks if the sample can be inserted into the series and updates
// the out-of-order mmap markers if necessary. It also updates the write-ahead log (WBL) samples
// and the minimum and maximum timestamps for out-of-order samples.
//
// For in-order samples, it attempts to append the sample to the series and updates the minimum
// and maximum timestamps for in-order samples.
//
// The function also increments the chunk metrics if a new chunk is created and performs cleanup
// operations on the series after appending the samples.
//
// There are also specific functions to commit histograms and float histograms.
func (a *headAppender) commitFloats(b *appendBatch, acc *appenderCommitContext) {
	var ok, chunkCreated bool
	var series *memSeries

	for i, s := range b.floats {
		series = b.floatSeries[i]
		series.Lock()

		if value.IsStaleNaN(s.V) {
			// If a float staleness marker had been appended for a
			// series that got a histogram or float histogram
			// appended before via this same appender, it would not
			// show up here because we had already converted it. We
			// end up here for two reasons: (1) This is the very
			// first sample for this series appended via this
			// appender. (2) A float sample was appended to this
			// series before via this same appender.
			//
			// In either case, we need to check the previous sample
			// in the memSeries to append the appropriately typed
			// staleness marker. This is obviously so in case (1).
			// In case (2), we would usually expect a float sample
			// as the previous sample, but there might be concurrent
			// appends that have added a histogram sample in the
			// meantime. (This will probably lead to OOO shenanigans
			// anyway, but that's a different story.)
			//
			// If the last sample in the memSeries is indeed a
			// float, we don't have to do anything special here and
			// just go on with the normal commit for a float sample.
			// However, if the last sample in the memSeries is a
			// histogram or float histogram, we have to convert the
			// staleness marker to a histogram (or float histogram,
			// respectively), and just add it at the end of the
			// histograms (or float histograms) in the same batch,
			// to be committed later in commitHistograms (or
			// commitFloatHistograms). The latter is fine because we
			// know there is no other histogram (or float histogram)
			// sample for this same series in this same batch
			// (because any such sample would have triggered a new
			// batch).
			switch {
			case series.lastHistogramValue != nil:
				b.histograms = append(b.histograms, record.RefHistogramSample{
					Ref: series.ref,
					T:   s.T,
					H:   &histogram.Histogram{Sum: s.V},
				})
				b.histogramSeries = append(b.histogramSeries, series)
				// This sample was counted as a float but is now a histogram.
				acc.floatsAppended--
				acc.histogramsAppended++
				series.Unlock()
				continue
			case series.lastFloatHistogramValue != nil:
				b.floatHistograms = append(b.floatHistograms, record.RefFloatHistogramSample{
					Ref: series.ref,
					T:   s.T,
					FH:  &histogram.FloatHistogram{Sum: s.V},
				})
				b.floatHistogramSeries = append(b.floatHistogramSeries, series)
				// This sample was counted as a float but is now a float histogram.
				acc.floatsAppended--
				acc.histogramsAppended++
				series.Unlock()
				continue
			}
		}
		oooSample, _, err := series.appendable(s.T, s.V, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err != nil {
			handleAppendableError(err, &acc.floatsAppended, &acc.floatOOORejected, &acc.floatOOBRejected, &acc.floatTooOldRejected)
		}

		switch {
		case err != nil:
			// Do nothing here.
		case oooSample:
			// Sample is OOO and OOO handling is enabled
			// and the delta is within the OOO tolerance.
			var mmapRefs []chunks.ChunkDiskMapperRef
			ok, chunkCreated, mmapRefs = series.insert(s.T, s.V, nil, nil, a.head.chunkDiskMapper, acc.oooCapMax, a.head.logger)
			if chunkCreated {
				r, ok := acc.oooMmapMarkers[series.ref]
				if !ok || r != nil {
					// !ok means there are no markers collected for these samples yet. So we first flush the samples
					// before setting this m-map marker.

					// r != nil means we have already m-mapped a chunk for this series in the same Commit().
					// Hence, before we m-map again, we should add the samples and m-map markers
					// seen till now to the WBL records.
					acc.collectOOORecords(a)
				}

				if acc.oooMmapMarkers == nil {
					acc.oooMmapMarkers = make(map[chunks.HeadSeriesRef][]chunks.ChunkDiskMapperRef)
				}
				if len(mmapRefs) > 0 {
					acc.oooMmapMarkers[series.ref] = mmapRefs
					acc.oooMmapMarkersCount += len(mmapRefs)
				} else {
					// No chunk was written to disk, so we need to set an initial marker for this series.
					acc.oooMmapMarkers[series.ref] = []chunks.ChunkDiskMapperRef{0}
					acc.oooMmapMarkersCount++
				}
			}
			if ok {
				acc.wblSamples = append(acc.wblSamples, s)
				if s.T < acc.oooMinT {
					acc.oooMinT = s.T
				}
				if s.T > acc.oooMaxT {
					acc.oooMaxT = s.T
				}
				acc.oooFloatsAccepted++
			} else {
				// Sample is an exact duplicate of the last sample.
				// NOTE: We can only detect updates if they clash with a sample in the OOOHeadChunk,
				// not with samples in already flushed OOO chunks.
				// TODO(codesome): Add error reporting? It depends on addressing https://github.com/prometheus/prometheus/discussions/10305.
				acc.floatsAppended--
			}
		default:
			newlyStale := !value.IsStaleNaN(series.lastValue) && value.IsStaleNaN(s.V)
			staleToNonStale := value.IsStaleNaN(series.lastValue) && !value.IsStaleNaN(s.V)
			ok, chunkCreated = series.append(s.T, s.V, a.appendID, acc.appendChunkOpts)
			if ok {
				if s.T < acc.inOrderMint {
					acc.inOrderMint = s.T
				}
				if s.T > acc.inOrderMaxt {
					acc.inOrderMaxt = s.T
				}
				if newlyStale {
					a.head.numStaleSeries.Inc()
				}
				if staleToNonStale {
					a.head.numStaleSeries.Dec()
				}
			} else {
				// The sample is an exact duplicate, and should be silently dropped.
				acc.floatsAppended--
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
}

// For details on the commitHistograms function, see the commitFloats docs.
func (a *headAppender) commitHistograms(b *appendBatch, acc *appenderCommitContext) {
	var ok, chunkCreated bool
	var series *memSeries

	for i, s := range b.histograms {
		series = b.histogramSeries[i]
		series.Lock()

		// At this point, we could encounter a histogram staleness
		// marker that should better be a float staleness marker or a
		// float histogram staleness marker. This can only happen with
		// concurrent appenders appending to the same series _and_ doing
		// so in a mixed-type scenario. This case is expected to be very
		// rare, so we do not bother here to convert the staleness
		// marker. The worst case is that we need to cut a new chunk
		// just for the staleness marker.

		oooSample, _, err := series.appendableHistogram(s.T, s.H, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err != nil {
			handleAppendableError(err, &acc.histogramsAppended, &acc.histoOOORejected, &acc.histoOOBRejected, &acc.histoTooOldRejected)
		}

		switch {
		case err != nil:
			// Do nothing here.
		case oooSample:
			// Sample is OOO and OOO handling is enabled
			// and the delta is within the OOO tolerance.
			var mmapRefs []chunks.ChunkDiskMapperRef
			ok, chunkCreated, mmapRefs = series.insert(s.T, 0, s.H, nil, a.head.chunkDiskMapper, acc.oooCapMax, a.head.logger)
			if chunkCreated {
				r, ok := acc.oooMmapMarkers[series.ref]
				if !ok || r != nil {
					// !ok means there are no markers collected for these samples yet. So we first flush the samples
					// before setting this m-map marker.

					// r != 0 means we have already m-mapped a chunk for this series in the same Commit().
					// Hence, before we m-map again, we should add the samples and m-map markers
					// seen till now to the WBL records.
					acc.collectOOORecords(a)
				}

				if acc.oooMmapMarkers == nil {
					acc.oooMmapMarkers = make(map[chunks.HeadSeriesRef][]chunks.ChunkDiskMapperRef)
				}
				if len(mmapRefs) > 0 {
					acc.oooMmapMarkers[series.ref] = mmapRefs
					acc.oooMmapMarkersCount += len(mmapRefs)
				} else {
					// No chunk was written to disk, so we need to set an initial marker for this series.
					acc.oooMmapMarkers[series.ref] = []chunks.ChunkDiskMapperRef{0}
					acc.oooMmapMarkersCount++
				}
			}
			if ok {
				acc.wblHistograms = append(acc.wblHistograms, s)
				if s.T < acc.oooMinT {
					acc.oooMinT = s.T
				}
				if s.T > acc.oooMaxT {
					acc.oooMaxT = s.T
				}
				acc.oooHistogramAccepted++
			} else {
				// Sample is an exact duplicate of the last sample.
				// NOTE: We can only detect updates if they clash with a sample in the OOOHeadChunk,
				// not with samples in already flushed OOO chunks.
				// TODO(codesome): Add error reporting? It depends on addressing https://github.com/prometheus/prometheus/discussions/10305.
				acc.histogramsAppended--
			}
		default:
			newlyStale := value.IsStaleNaN(s.H.Sum)
			staleToNonStale := false
			if series.lastHistogramValue != nil {
				newlyStale = newlyStale && !value.IsStaleNaN(series.lastHistogramValue.Sum)
				staleToNonStale = value.IsStaleNaN(series.lastHistogramValue.Sum) && !value.IsStaleNaN(s.H.Sum)
			}
			ok, chunkCreated = series.appendHistogram(s.T, s.H, a.appendID, acc.appendChunkOpts)
			if ok {
				if s.T < acc.inOrderMint {
					acc.inOrderMint = s.T
				}
				if s.T > acc.inOrderMaxt {
					acc.inOrderMaxt = s.T
				}
				if newlyStale {
					a.head.numStaleSeries.Inc()
				}
				if staleToNonStale {
					a.head.numStaleSeries.Dec()
				}
			} else {
				acc.histogramsAppended--
				acc.histoOOORejected++
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
}

// For details on the commitFloatHistograms function, see the commitFloats docs.
func (a *headAppender) commitFloatHistograms(b *appendBatch, acc *appenderCommitContext) {
	var ok, chunkCreated bool
	var series *memSeries

	for i, s := range b.floatHistograms {
		series = b.floatHistogramSeries[i]
		series.Lock()

		// At this point, we could encounter a float histogram staleness
		// marker that should better be a float staleness marker or an
		// integer histogram staleness marker. This can only happen with
		// concurrent appenders appending to the same series _and_ doing
		// so in a mixed-type scenario. This case is expected to be very
		// rare, so we do not bother here to convert the staleness
		// marker. The worst case is that we need to cut a new chunk
		// just for the staleness marker.

		oooSample, _, err := series.appendableFloatHistogram(s.T, s.FH, a.headMaxt, a.minValidTime, a.oooTimeWindow)
		if err != nil {
			handleAppendableError(err, &acc.histogramsAppended, &acc.histoOOORejected, &acc.histoOOBRejected, &acc.histoTooOldRejected)
		}

		switch {
		case err != nil:
			// Do nothing here.
		case oooSample:
			// Sample is OOO and OOO handling is enabled
			// and the delta is within the OOO tolerance.
			var mmapRefs []chunks.ChunkDiskMapperRef
			ok, chunkCreated, mmapRefs = series.insert(s.T, 0, nil, s.FH, a.head.chunkDiskMapper, acc.oooCapMax, a.head.logger)
			if chunkCreated {
				r, ok := acc.oooMmapMarkers[series.ref]
				if !ok || r != nil {
					// !ok means there are no markers collected for these samples yet. So we first flush the samples
					// before setting this m-map marker.

					// r != 0 means we have already m-mapped a chunk for this series in the same Commit().
					// Hence, before we m-map again, we should add the samples and m-map markers
					// seen till now to the WBL records.
					acc.collectOOORecords(a)
				}

				if acc.oooMmapMarkers == nil {
					acc.oooMmapMarkers = make(map[chunks.HeadSeriesRef][]chunks.ChunkDiskMapperRef)
				}
				if len(mmapRefs) > 0 {
					acc.oooMmapMarkers[series.ref] = mmapRefs
					acc.oooMmapMarkersCount += len(mmapRefs)
				} else {
					// No chunk was written to disk, so we need to set an initial marker for this series.
					acc.oooMmapMarkers[series.ref] = []chunks.ChunkDiskMapperRef{0}
					acc.oooMmapMarkersCount++
				}
			}
			if ok {
				acc.wblFloatHistograms = append(acc.wblFloatHistograms, s)
				if s.T < acc.oooMinT {
					acc.oooMinT = s.T
				}
				if s.T > acc.oooMaxT {
					acc.oooMaxT = s.T
				}
				acc.oooHistogramAccepted++
			} else {
				// Sample is an exact duplicate of the last sample.
				// NOTE: We can only detect updates if they clash with a sample in the OOOHeadChunk,
				// not with samples in already flushed OOO chunks.
				// TODO(codesome): Add error reporting? It depends on addressing https://github.com/prometheus/prometheus/discussions/10305.
				acc.histogramsAppended--
			}
		default:
			newlyStale := value.IsStaleNaN(s.FH.Sum)
			staleToNonStale := false
			if series.lastFloatHistogramValue != nil {
				newlyStale = newlyStale && !value.IsStaleNaN(series.lastFloatHistogramValue.Sum)
				staleToNonStale = value.IsStaleNaN(series.lastFloatHistogramValue.Sum) && !value.IsStaleNaN(s.FH.Sum)
			}
			ok, chunkCreated = series.appendFloatHistogram(s.T, s.FH, a.appendID, acc.appendChunkOpts)
			if ok {
				if s.T < acc.inOrderMint {
					acc.inOrderMint = s.T
				}
				if s.T > acc.inOrderMaxt {
					acc.inOrderMaxt = s.T
				}
				if newlyStale {
					a.head.numStaleSeries.Inc()
				}
				if staleToNonStale {
					a.head.numStaleSeries.Dec()
				}
			} else {
				acc.histogramsAppended--
				acc.histoOOORejected++
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
}

// commitMetadata commits the metadata for each series in the provided batch.
// It iterates over the metadata slice and updates the corresponding series
// with the new metadata information. The series is locked during the update
// to ensure thread safety.
func commitMetadata(b *appendBatch) {
	var series *memSeries
	for i, m := range b.metadata {
		series = b.metadataSeries[i]
		series.Lock()
		series.meta = &metadata.Metadata{Type: record.ToMetricType(m.Type), Unit: m.Unit, Help: m.Help}
		series.Unlock()
	}
}

func (a *headAppender) unmarkCreatedSeriesAsPendingCommit() {
	for _, s := range a.series {
		s.Lock()
		s.pendingCommit = false
		s.Unlock()
	}
}

// Commit writes to the WAL and adds the data to the Head.
// TODO(codesome): Refactor this method to reduce indentation and make it more readable.
func (a *headAppender) Commit() (err error) {
	if a.closed {
		return ErrAppenderClosed
	}

	h := a.head

	defer func() {
		if a.closed {
			// Don't double-close in case Rollback() was called.
			return
		}
		h.putRefSeriesBuffer(a.seriesRefs)
		h.putSeriesBuffer(a.series)
		h.putTypeMap(a.typesInBatch)
		a.closed = true
	}()

	if err := a.log(); err != nil {
		_ = a.Rollback() // Most likely the same error will happen again.
		return fmt.Errorf("write to WAL: %w", err)
	}

	if h.writeNotified != nil {
		h.writeNotified.Notify()
	}

	acc := &appenderCommitContext{
		inOrderMint: math.MaxInt64,
		inOrderMaxt: math.MinInt64,
		oooMinT:     math.MaxInt64,
		oooMaxT:     math.MinInt64,
		oooCapMax:   h.opts.OutOfOrderCapMax.Load(),
		appendChunkOpts: chunkOpts{
			chunkDiskMapper: h.chunkDiskMapper,
			chunkRange:      h.chunkRange.Load(),
			samplesPerChunk: h.opts.SamplesPerChunk,
		},
	}

	for _, b := range a.batches {
		acc.floatsAppended += len(b.floats)
		acc.histogramsAppended += len(b.histograms) + len(b.floatHistograms)
		a.commitExemplars(b)
		defer b.close(h)
	}
	defer h.metrics.activeAppenders.Dec()
	defer h.iso.closeAppend(a.appendID)

	defer func() {
		for i := range acc.oooRecords {
			h.putBytesBuffer(acc.oooRecords[i][:0])
		}
	}()

	for _, b := range a.batches {
		// Do not change the order of these calls. We depend on it for
		// correct commit order of samples and for the staleness marker
		// handling.
		a.commitFloats(b, acc)
		a.commitHistograms(b, acc)
		a.commitFloatHistograms(b, acc)
		commitMetadata(b)
	}
	// Unmark all series as pending commit after all samples have been committed.
	a.unmarkCreatedSeriesAsPendingCommit()

	h.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(acc.floatOOORejected))
	h.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeHistogram).Add(float64(acc.histoOOORejected))
	h.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(acc.floatOOBRejected))
	h.metrics.tooOldSamples.WithLabelValues(sampleMetricTypeFloat).Add(float64(acc.floatTooOldRejected))
	h.metrics.samplesAppended.WithLabelValues(sampleMetricTypeFloat).Add(float64(acc.floatsAppended))
	h.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram).Add(float64(acc.histogramsAppended))
	h.metrics.outOfOrderSamplesAppended.WithLabelValues(sampleMetricTypeFloat).Add(float64(acc.oooFloatsAccepted))
	h.metrics.outOfOrderSamplesAppended.WithLabelValues(sampleMetricTypeHistogram).Add(float64(acc.oooHistogramAccepted))
	h.updateMinMaxTime(acc.inOrderMint, acc.inOrderMaxt)
	h.updateMinOOOMaxOOOTime(acc.oooMinT, acc.oooMaxT)

	acc.collectOOORecords(a)
	if h.wbl != nil {
		if err := h.wbl.Log(acc.oooRecords...); err != nil {
			// TODO(codesome): Currently WBL logging of ooo samples is best effort here since we cannot try logging
			// until we have found what samples become OOO. We can try having a metric for this failure.
			// Returning the error here is not correct because we have already put the samples into the memory,
			// hence the append/insert was a success.
			h.logger.Error("Failed to log out of order samples into the WAL", "err", err)
		}
	}
	return nil
}

// insert is like append, except it inserts. Used for OOO samples.
func (s *memSeries) insert(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, chunkDiskMapper *chunks.ChunkDiskMapper, oooCapMax int64, logger *slog.Logger) (inserted, chunkCreated bool, mmapRefs []chunks.ChunkDiskMapperRef) {
	if s.ooo == nil {
		s.ooo = &memSeriesOOOFields{}
	}
	c := s.ooo.oooHeadChunk
	if c == nil || c.chunk.NumSamples() == int(oooCapMax) {
		// Note: If no new samples come in then we rely on compaction to clean up stale in-memory OOO chunks.
		c, mmapRefs = s.cutNewOOOHeadChunk(t, chunkDiskMapper, logger)
		chunkCreated = true
	}

	ok := c.chunk.Insert(t, v, h, fh)
	if ok {
		if chunkCreated || t < c.minTime {
			c.minTime = t
		}
		if chunkCreated || t > c.maxTime {
			c.maxTime = t
		}
	}
	return ok, chunkCreated, mmapRefs
}

// chunkOpts are chunk-level options that are passed when appending to a memSeries.
type chunkOpts struct {
	chunkDiskMapper *chunks.ChunkDiskMapper
	chunkRange      int64
	samplesPerChunk int
}

// append adds the sample (t, v) to the series. The caller also has to provide
// the appendID for isolation. (The appendID can be zero, which results in no
// isolation for this append.)
// Series lock must be held when calling.
func (s *memSeries) append(t int64, v float64, appendID uint64, o chunkOpts) (sampleInOrder, chunkCreated bool) {
	c, sampleInOrder, chunkCreated := s.appendPreprocessor(t, chunkenc.EncXOR, o)
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
// In case of recoding the existing chunk, a new chunk is allocated and the old chunk is dropped.
// To keep the meaning of prometheus_tsdb_head_chunks and prometheus_tsdb_head_chunks_created_total
// consistent, we return chunkCreated=false in this case.
func (s *memSeries) appendHistogram(t int64, h *histogram.Histogram, appendID uint64, o chunkOpts) (sampleInOrder, chunkCreated bool) {
	// Head controls the execution of recoding, so that we own the proper
	// chunk reference afterwards and mmap used up chunks.

	// Ignoring ok is ok, since we don't want to compare to the wrong previous appender anyway.
	prevApp, _ := s.app.(*chunkenc.HistogramAppender)

	c, sampleInOrder, chunkCreated := s.histogramsAppendPreprocessor(t, chunkenc.EncHistogram, o)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}

	var (
		newChunk chunkenc.Chunk
		recoded  bool
	)

	if !chunkCreated {
		// Ignore the previous appender if we continue the current chunk.
		prevApp = nil
	}

	newChunk, recoded, s.app, _ = s.app.AppendHistogram(prevApp, t, h, false) // false=request a new chunk if needed

	s.lastHistogramValue = h
	s.lastFloatHistogramValue = nil

	if appendID > 0 {
		s.txs.add(appendID)
	}

	if newChunk == nil { // Sample was appended to existing chunk or is the first sample in a new chunk.
		c.maxTime = t
		return true, chunkCreated
	}

	if recoded { // The appender needed to recode the chunk.
		c.maxTime = t
		c.chunk = newChunk
		return true, false
	}

	s.headChunks = &memChunk{
		chunk:   newChunk,
		minTime: t,
		maxTime: t,
		prev:    s.headChunks,
	}
	s.nextAt = rangeForTimestamp(t, o.chunkRange)
	return true, true
}

// appendFloatHistogram adds the float histogram.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
// In case of recoding the existing chunk, a new chunk is allocated and the old chunk is dropped.
// To keep the meaning of prometheus_tsdb_head_chunks and prometheus_tsdb_head_chunks_created_total
// consistent, we return chunkCreated=false in this case.
func (s *memSeries) appendFloatHistogram(t int64, fh *histogram.FloatHistogram, appendID uint64, o chunkOpts) (sampleInOrder, chunkCreated bool) {
	// Head controls the execution of recoding, so that we own the proper
	// chunk reference afterwards and mmap used up chunks.

	// Ignoring ok is ok, since we don't want to compare to the wrong previous appender anyway.
	prevApp, _ := s.app.(*chunkenc.FloatHistogramAppender)

	c, sampleInOrder, chunkCreated := s.histogramsAppendPreprocessor(t, chunkenc.EncFloatHistogram, o)
	if !sampleInOrder {
		return sampleInOrder, chunkCreated
	}

	var (
		newChunk chunkenc.Chunk
		recoded  bool
	)

	if !chunkCreated {
		// Ignore the previous appender if we continue the current chunk.
		prevApp = nil
	}

	newChunk, recoded, s.app, _ = s.app.AppendFloatHistogram(prevApp, t, fh, false) // False means request a new chunk if needed.

	s.lastHistogramValue = nil
	s.lastFloatHistogramValue = fh

	if appendID > 0 {
		s.txs.add(appendID)
	}

	if newChunk == nil { // Sample was appended to existing chunk or is the first sample in a new chunk.
		c.maxTime = t
		return true, chunkCreated
	}

	if recoded { // The appender needed to recode the chunk.
		c.maxTime = t
		c.chunk = newChunk
		return true, false
	}

	s.headChunks = &memChunk{
		chunk:   newChunk,
		minTime: t,
		maxTime: t,
		prev:    s.headChunks,
	}
	s.nextAt = rangeForTimestamp(t, o.chunkRange)
	return true, true
}

// appendPreprocessor takes care of cutting new XOR chunks and m-mapping old ones. XOR chunks are cut based on the
// number of samples they contain with a soft cap in bytes.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
// This should be called only when appending data.
func (s *memSeries) appendPreprocessor(t int64, e chunkenc.Encoding, o chunkOpts) (c *memChunk, sampleInOrder, chunkCreated bool) {
	// We target chunkenc.MaxBytesPerXORChunk as a hard for the size of an XOR chunk. We must determine whether to cut
	// a new head chunk without knowing the size of the next sample, however, so we assume the next sample will be a
	// maximally-sized sample (19 bytes).
	const maxBytesPerXORChunk = chunkenc.MaxBytesPerXORChunk - 19

	c = s.headChunks

	if c == nil {
		if len(s.mmappedChunks) > 0 && s.mmappedChunks[len(s.mmappedChunks)-1].maxTime >= t {
			// Out of order sample. Sample timestamp is already in the mmapped chunks, so ignore it.
			return c, false, false
		}
		// There is no head chunk in this series yet, create the first chunk for the sample.
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	// Out of order sample.
	if c.maxTime >= t {
		return c, false, chunkCreated
	}

	// Check the chunk size, unless we just created it and if the chunk is too large, cut a new one.
	if !chunkCreated && len(c.chunk.Bytes()) > maxBytesPerXORChunk {
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	if c.chunk.Encoding() != e {
		// The chunk encoding expected by this append is different than the head chunk's
		// encoding. So we cut a new chunk with the expected encoding.
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	numSamples := c.chunk.NumSamples()
	if numSamples == 0 {
		// It could be the new chunk created after reading the chunk snapshot,
		// hence we fix the minTime of the chunk here.
		c.minTime = t
		s.nextAt = rangeForTimestamp(c.minTime, o.chunkRange)
	}

	// If we reach 25% of a chunk's desired sample count, predict an end time
	// for this chunk that will try to make samples equally distributed within
	// the remaining chunks in the current chunk range.
	// At latest it must happen at the timestamp set when the chunk was cut.
	if numSamples == o.samplesPerChunk/4 {
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, s.nextAt, 4)
	}
	// If numSamples > samplesPerChunk*2 then our previous prediction was invalid,
	// most likely because samples rate has changed and now they are arriving more frequently.
	// Since we assume that the rate is higher, we're being conservative and cutting at 2*samplesPerChunk
	// as we expect more chunks to come.
	// Note that next chunk will have its nextAt recalculated for the new rate.
	if t >= s.nextAt || numSamples >= o.samplesPerChunk*2 {
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	return c, true, chunkCreated
}

// histogramsAppendPreprocessor takes care of cutting new histogram chunks and m-mapping old ones. Histogram chunks are
// cut based on their size in bytes.
// It is unsafe to call this concurrently with s.iterator(...) without holding the series lock.
// This should be called only when appending data.
func (s *memSeries) histogramsAppendPreprocessor(t int64, e chunkenc.Encoding, o chunkOpts) (c *memChunk, sampleInOrder, chunkCreated bool) {
	c = s.headChunks

	if c == nil {
		if len(s.mmappedChunks) > 0 && s.mmappedChunks[len(s.mmappedChunks)-1].maxTime >= t {
			// Out of order sample. Sample timestamp is already in the mmapped chunks, so ignore it.
			return c, false, false
		}
		// There is no head chunk in this series yet, create the first chunk for the sample.
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	// Out of order sample.
	if c.maxTime >= t {
		return c, false, chunkCreated
	}

	if c.chunk.Encoding() != e {
		// The chunk encoding expected by this append is different than the head chunk's
		// encoding. So we cut a new chunk with the expected encoding.
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	numSamples := c.chunk.NumSamples()
	targetBytes := chunkenc.TargetBytesPerHistogramChunk
	numBytes := len(c.chunk.Bytes())

	if numSamples == 0 {
		// It could be the new chunk created after reading the chunk snapshot,
		// hence we fix the minTime of the chunk here.
		c.minTime = t
		s.nextAt = rangeForTimestamp(c.minTime, o.chunkRange)
	}

	// Below, we will enforce chunkenc.MinSamplesPerHistogramChunk. There are, however, two cases that supersede it:
	//  - The current chunk range is ending before chunkenc.MinSamplesPerHistogramChunk will be satisfied.
	//  - s.nextAt was set while loading a chunk snapshot with the intent that a new chunk be cut on the next append.
	var nextChunkRangeStart int64
	if s.histogramChunkHasComputedEndTime {
		nextChunkRangeStart = rangeForTimestamp(c.minTime, o.chunkRange)
	} else {
		// If we haven't yet computed an end time yet, s.nextAt is either set to
		// rangeForTimestamp(c.minTime, o.chunkRange) or was set while loading a chunk snapshot. Either way, we want to
		// skip enforcing chunkenc.MinSamplesPerHistogramChunk.
		nextChunkRangeStart = s.nextAt
	}

	// If we reach 25% of a chunk's desired maximum size, predict an end time
	// for this chunk that will try to make samples equally distributed within
	// the remaining chunks in the current chunk range.
	// At the latest it must happen at the timestamp set when the chunk was cut.
	if !s.histogramChunkHasComputedEndTime && numBytes >= targetBytes/4 {
		ratioToFull := float64(targetBytes) / float64(numBytes)
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, s.nextAt, ratioToFull)
		s.histogramChunkHasComputedEndTime = true
	}
	// If numBytes > targetBytes*2 then our previous prediction was invalid. This could happen if the sample rate has
	// increased or if the bucket/span count has increased.
	// Note that next chunk will have its nextAt recalculated for the new rate.
	if (t >= s.nextAt || numBytes >= targetBytes*2) && (numSamples >= chunkenc.MinSamplesPerHistogramChunk || t >= nextChunkRangeStart) {
		c = s.cutNewHeadChunk(t, e, o.chunkRange)
		chunkCreated = true
	}

	// The new chunk will also need a new computed end time.
	if chunkCreated {
		s.histogramChunkHasComputedEndTime = false
	}

	return c, true, chunkCreated
}

// computeChunkEndTime estimates the end timestamp based the beginning of a
// chunk, its current timestamp and the upper bound up to which we insert data.
// It assumes that the time range is 1/ratioToFull full.
// Assuming that the samples will keep arriving at the same rate, it will make the
// remaining n chunks within this chunk range (before max) equally sized.
func computeChunkEndTime(start, cur, maxT int64, ratioToFull float64) int64 {
	n := float64(maxT-start) / (float64(cur-start+1) * ratioToFull)
	if n <= 1 {
		return maxT
	}
	return int64(float64(start) + float64(maxT-start)/math.Floor(n))
}

func (s *memSeries) cutNewHeadChunk(mint int64, e chunkenc.Encoding, chunkRange int64) *memChunk {
	// When cutting a new head chunk we create a new memChunk instance with .prev
	// pointing at the current .headChunks, so it forms a linked list.
	// All but first headChunks list elements will be m-mapped as soon as possible
	// so this is a single element list most of the time.
	s.headChunks = &memChunk{
		minTime: mint,
		maxTime: math.MinInt64,
		prev:    s.headChunks,
	}

	if chunkenc.IsValidEncoding(e) {
		var err error
		s.headChunks.chunk, err = chunkenc.NewEmptyChunk(e)
		if err != nil {
			panic(err) // This should never happen.
		}
	} else {
		s.headChunks.chunk = chunkenc.NewXORChunk()
	}

	// Set upper bound on when the next chunk must be started. An earlier timestamp
	// may be chosen dynamically at a later point.
	s.nextAt = rangeForTimestamp(mint, chunkRange)

	app, err := s.headChunks.chunk.Appender()
	if err != nil {
		panic(err)
	}
	s.app = app
	return s.headChunks
}

// cutNewOOOHeadChunk cuts a new OOO chunk and m-maps the old chunk.
// The caller must ensure that s is locked and s.ooo is not nil.
func (s *memSeries) cutNewOOOHeadChunk(mint int64, chunkDiskMapper *chunks.ChunkDiskMapper, logger *slog.Logger) (*oooHeadChunk, []chunks.ChunkDiskMapperRef) {
	ref := s.mmapCurrentOOOHeadChunk(chunkDiskMapper, logger)

	s.ooo.oooHeadChunk = &oooHeadChunk{
		chunk:   NewOOOChunk(),
		minTime: mint,
		maxTime: math.MinInt64,
	}

	return s.ooo.oooHeadChunk, ref
}

// s must be locked when calling.
func (s *memSeries) mmapCurrentOOOHeadChunk(chunkDiskMapper *chunks.ChunkDiskMapper, logger *slog.Logger) []chunks.ChunkDiskMapperRef {
	if s.ooo == nil || s.ooo.oooHeadChunk == nil {
		// OOO is not enabled or there is no head chunk, so nothing to m-map here.
		return nil
	}
	chks, err := s.ooo.oooHeadChunk.chunk.ToEncodedChunks(math.MinInt64, math.MaxInt64)
	if err != nil {
		handleChunkWriteError(err)
		return nil
	}
	chunkRefs := make([]chunks.ChunkDiskMapperRef, 0, len(chks))
	for _, memchunk := range chks {
		if len(s.ooo.oooMmappedChunks) >= (oooChunkIDMask - 1) {
			logger.Error("Too many OOO chunks, dropping data", "series", s.lset.String())
			break
		}
		chunkRef := chunkDiskMapper.WriteChunk(s.ref, memchunk.minTime, memchunk.maxTime, memchunk.chunk, true, handleChunkWriteError)
		chunkRefs = append(chunkRefs, chunkRef)
		s.ooo.oooMmappedChunks = append(s.ooo.oooMmappedChunks, &mmappedChunk{
			ref:        chunkRef,
			numSamples: uint16(memchunk.chunk.NumSamples()),
			minTime:    memchunk.minTime,
			maxTime:    memchunk.maxTime,
		})
	}
	s.ooo.oooHeadChunk = nil
	return chunkRefs
}

// mmapChunks will m-map all but first chunk on s.headChunks list.
func (s *memSeries) mmapChunks(chunkDiskMapper *chunks.ChunkDiskMapper) (count int) {
	if s.headChunks == nil || s.headChunks.prev == nil {
		// There is none or only one head chunk, so nothing to m-map here.
		return count
	}

	// Write chunks starting from the oldest one and stop before we get to current s.headChunks.
	// If we have this chain: s.headChunks{t4} -> t3 -> t2 -> t1 -> t0
	// then we need to write chunks t0 to t3, but skip s.headChunks.
	for i := s.headChunks.len() - 1; i > 0; i-- {
		chk := s.headChunks.atOffset(i)
		chunkRef := chunkDiskMapper.WriteChunk(s.ref, chk.minTime, chk.maxTime, chk.chunk, false, handleChunkWriteError)
		s.mmappedChunks = append(s.mmappedChunks, &mmappedChunk{
			ref:        chunkRef,
			numSamples: uint16(chk.chunk.NumSamples()),
			minTime:    chk.minTime,
			maxTime:    chk.maxTime,
		})
		count++
	}

	// Once we've written out all chunks except s.headChunks we need to unlink these from s.headChunk.
	s.headChunks.prev = nil

	return count
}

func handleChunkWriteError(err error) {
	if err != nil && !errors.Is(err, chunks.ErrChunkDiskMapperClosed) {
		panic(err)
	}
}

// Rollback removes the samples and exemplars from headAppender and writes any series to WAL.
func (a *headAppender) Rollback() (err error) {
	if a.closed {
		return ErrAppenderClosed
	}
	h := a.head
	defer func() {
		a.unmarkCreatedSeriesAsPendingCommit()
		h.iso.closeAppend(a.appendID)
		h.metrics.activeAppenders.Dec()
		a.closed = true
		h.putRefSeriesBuffer(a.seriesRefs)
		h.putSeriesBuffer(a.series)
		h.putTypeMap(a.typesInBatch)
	}()

	var series *memSeries
	for _, b := range a.batches {
		for i := range b.floats {
			series = b.floatSeries[i]
			series.Lock()
			series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
			series.pendingCommit = false
			series.Unlock()
		}
		for i := range b.histograms {
			series = b.histogramSeries[i]
			series.Lock()
			series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
			series.pendingCommit = false
			series.Unlock()
		}
		for i := range b.floatHistograms {
			series = b.floatHistogramSeries[i]
			series.Lock()
			series.cleanupAppendIDsBelow(a.cleanupAppendIDsBelow)
			series.pendingCommit = false
			series.Unlock()
		}
		b.close(h)
	}
	a.batches = a.batches[:0]
	// Series are created in the head memory regardless of rollback. Thus we have
	// to log them to the WAL in any case.
	return a.log()
}
