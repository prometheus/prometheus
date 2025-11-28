// Copyright The Prometheus Authors
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
	"math"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

// initAppenderV2 is a helper to initialize the time bounds of the head
// upon the first sample it receives.
type initAppenderV2 struct {
	app  storage.AppenderV2
	head *Head
}

var _ storage.GetRef = &initAppenderV2{}

func (a *initAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	if a.app == nil {
		a.head.initTime(t)
		a.app = a.head.appenderV2()
	}
	return a.app.Append(ref, ls, st, t, v, h, fh, opts)
}

func (a *initAppenderV2) GetRef(lset labels.Labels, hash uint64) (storage.SeriesRef, labels.Labels) {
	if g, ok := a.app.(storage.GetRef); ok {
		return g.GetRef(lset, hash)
	}
	return 0, labels.EmptyLabels()
}

func (a *initAppenderV2) Commit() error {
	if a.app == nil {
		a.head.metrics.activeAppenders.Dec()
		return nil
	}
	return a.app.Commit()
}

func (a *initAppenderV2) Rollback() error {
	if a.app == nil {
		a.head.metrics.activeAppenders.Dec()
		return nil
	}
	return a.app.Rollback()
}

// AppenderV2 returns a new AppenderV2 on the database.
func (h *Head) AppenderV2(context.Context) storage.AppenderV2 {
	h.metrics.activeAppenders.Inc()

	// The head cache might not have a starting point yet. The init appender
	// picks up the first appended timestamp as the base.
	if !h.initialized() {
		return &initAppenderV2{
			head: h,
		}
	}
	return h.appenderV2()
}

func (h *Head) appenderV2() *headAppenderV2 {
	minValidTime := h.appendableMinValidTime()
	appendID, cleanupAppendIDsBelow := h.iso.newAppendID(minValidTime) // Every appender gets an ID that is cleared upon commit/rollback.
	return &headAppenderV2{
		headAppenderBase: headAppenderBase{
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
		},
	}
}

type headAppenderV2 struct {
	headAppenderBase
}

func (a *headAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	var (
		// Avoid shadowing err variables for reliability.
		valErr, appErr, partialErr error
		sampleMetricType           = sampleMetricTypeFloat
		isStale                    bool
	)
	// Fail fast on incorrect histograms.

	switch {
	case fh != nil:
		sampleMetricType = sampleMetricTypeHistogram
		valErr = fh.Validate()
	case h != nil:
		sampleMetricType = sampleMetricTypeHistogram
		valErr = h.Validate()
	}
	if valErr != nil {
		return 0, valErr
	}

	// Fail fast if OOO is disabled and the sample is out of bounds.
	// Otherwise, a full check will be done later to decide if the sample is in-order or out-of-order.
	if a.oooTimeWindow == 0 && t < a.minValidTime {
		a.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricType).Inc()
		return 0, storage.ErrOutOfBounds
	}

	s := a.head.series.getByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, _, err = a.getOrCreate(ls)
		if err != nil {
			return 0, err
		}
	}

	// TODO(bwplotka): Handle ST natively (as per PROM-60).
	if a.head.opts.EnableSTAsZeroSample && st != 0 {
		a.bestEffortAppendSTZeroSample(s, st, t, h, fh)
	}

	switch {
	case fh != nil:
		isStale = value.IsStaleNaN(fh.Sum)
		appErr = a.appendFloatHistogram(s, t, fh, opts.RejectOutOfOrder)
	case h != nil:
		isStale = value.IsStaleNaN(h.Sum)
		appErr = a.appendHistogram(s, t, h, opts.RejectOutOfOrder)
	default:
		isStale = value.IsStaleNaN(v)
		if isStale {
			// If we have added a sample before with this same appender, we
			// can check the previously used type and turn a stale float
			// sample into a stale histogram sample or stale float histogram
			// sample as appropriate. This prevents an unnecessary creation
			// of a new batch. However, since other appenders might append
			// to the same series concurrently, this is not perfect but just
			// an optimization for the more likely case.
			switch a.typesInBatch[s.ref] {
			case stHistogram, stCustomBucketHistogram:
				return a.Append(ref, ls, st, t, 0, &histogram.Histogram{Sum: v}, nil, storage.AOptions{
					RejectOutOfOrder: opts.RejectOutOfOrder,
				})
			case stFloatHistogram, stCustomBucketFloatHistogram:
				return a.Append(ref, ls, st, t, 0, nil, &histogram.FloatHistogram{Sum: v}, storage.AOptions{
					RejectOutOfOrder: opts.RejectOutOfOrder,
				})
			}
			// Note that a series reference not yet in the map will come out
			// as stNone, but since we do not handle that case separately,
			// we do not need to check for the difference between "unknown
			// series" and "known series with stNone".
		}
		appErr = a.appendFloat(s, t, v, opts.RejectOutOfOrder)
	}
	// Handle append error, if any.
	if appErr != nil {
		switch {
		case errors.Is(appErr, storage.ErrOutOfOrderSample):
			a.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricType).Inc()
		case errors.Is(appErr, storage.ErrTooOldSample):
			a.head.metrics.tooOldSamples.WithLabelValues(sampleMetricType).Inc()
		}
		return 0, appErr
	}

	if t < a.mint {
		a.mint = t
	}
	if t > a.maxt {
		a.maxt = t
	}

	if isStale {
		// For stale values we never attempt to process metadata/exemplars, claim the success.
		return ref, nil
	}

	// Append exemplars if any and if storage was configured for it.
	if len(opts.Exemplars) > 0 && a.head.opts.EnableExemplarStorage && a.head.opts.MaxExemplars.Load() > 0 {
		// Currently only exemplars can return partial errors.
		partialErr = a.appendExemplars(s, opts.Exemplars)
	}

	// TODO(bwplotka): Move/reuse metadata tests from scrape, once scrape adopts AppenderV2.
	// Currently tsdb package does not test metadata.
	if a.head.opts.EnableMetadataWALRecords && !opts.Metadata.IsEmpty() {
		s.Lock()
		metaChanged := s.meta == nil || !s.meta.Equals(opts.Metadata)
		s.Unlock()
		if metaChanged {
			b := a.getCurrentBatch(stNone, s.ref)
			b.metadata = append(b.metadata, record.RefMetadata{
				Ref:  s.ref,
				Type: record.GetMetricType(opts.Metadata.Type),
				Unit: opts.Metadata.Unit,
				Help: opts.Metadata.Help,
			})
			b.metadataSeries = append(b.metadataSeries, s)
		}
	}
	return storage.SeriesRef(s.ref), partialErr
}

func (a *headAppenderV2) appendFloat(s *memSeries, t int64, v float64, fastRejectOOO bool) error {
	s.Lock()
	// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
	// to skip that sample from the WAL and write only in the WBL.
	isOOO, delta, err := s.appendable(t, v, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if isOOO && fastRejectOOO {
		s.Unlock()
		return storage.ErrOutOfOrderSample
	}
	if err == nil {
		s.pendingCommit = true
	}
	s.Unlock()
	if delta > 0 {
		a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
	}
	if err != nil {
		return err
	}

	b := a.getCurrentBatch(stFloat, s.ref)
	b.floats = append(b.floats, record.RefSample{Ref: s.ref, T: t, V: v})
	b.floatSeries = append(b.floatSeries, s)
	return nil
}

func (a *headAppenderV2) appendHistogram(s *memSeries, t int64, h *histogram.Histogram, fastRejectOOO bool) error {
	s.Lock()
	// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
	// to skip that sample from the WAL and write only in the WBL.
	isOOO, delta, err := s.appendableHistogram(t, h, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if isOOO && fastRejectOOO {
		s.Unlock()
		return storage.ErrOutOfOrderSample
	}
	if err == nil {
		s.pendingCommit = true
	}
	s.Unlock()
	if delta > 0 {
		a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
	}
	if err != nil {
		return err
	}
	st := stHistogram
	if h.UsesCustomBuckets() {
		st = stCustomBucketHistogram
	}
	b := a.getCurrentBatch(st, s.ref)
	b.histograms = append(b.histograms, record.RefHistogramSample{Ref: s.ref, T: t, H: h})
	b.histogramSeries = append(b.histogramSeries, s)
	return nil
}

func (a *headAppenderV2) appendFloatHistogram(s *memSeries, t int64, fh *histogram.FloatHistogram, fastRejectOOO bool) error {
	s.Lock()
	// TODO(codesome): If we definitely know at this point that the sample is ooo, then optimise
	// to skip that sample from the WAL and write only in the WBL.
	isOOO, delta, err := s.appendableFloatHistogram(t, fh, a.headMaxt, a.minValidTime, a.oooTimeWindow)
	if isOOO && fastRejectOOO {
		s.Unlock()
		return storage.ErrOutOfOrderSample
	}
	if err == nil {
		s.pendingCommit = true
	}
	s.Unlock()
	if delta > 0 {
		a.head.metrics.oooHistogram.Observe(float64(delta) / 1000)
	}
	if err != nil {
		return err
	}
	st := stFloatHistogram
	if fh.UsesCustomBuckets() {
		st = stCustomBucketFloatHistogram
	}
	b := a.getCurrentBatch(st, s.ref)
	b.floatHistograms = append(b.floatHistograms, record.RefFloatHistogramSample{Ref: s.ref, T: t, FH: fh})
	b.floatHistogramSeries = append(b.floatHistogramSeries, s)
	return nil
}

func (a *headAppenderV2) appendExemplars(s *memSeries, exemplar []exemplar.Exemplar) error {
	var errs []error
	for _, e := range exemplar {
		// Ensure no empty labels have gotten through.
		e.Labels = e.Labels.WithoutEmpty()
		if err := a.head.exemplars.ValidateExemplar(s.labels(), e); err != nil {
			if !errors.Is(err, storage.ErrDuplicateExemplar) && !errors.Is(err, storage.ErrExemplarsDisabled) {
				// Except duplicates, return partial errors.
				errs = append(errs, err)
			}
			if !errors.Is(err, storage.ErrOutOfOrderExemplar) {
				a.head.logger.Debug("Error while adding an exemplar on AppendSample", "exemplars", fmt.Sprintf("%+v", e), "err", e)
			}
			continue
		}
		b := a.getCurrentBatch(stNone, s.ref)
		b.exemplars = append(b.exemplars, exemplarWithSeriesRef{storage.SeriesRef(s.ref), e})
	}
	if len(errs) > 0 {
		return &storage.AppendPartialError{ExemplarErrors: errs}
	}
	return nil
}

// NOTE(bwplotka): This feature might be deprecated and removed once PROM-60
// is implemented.
//
// ST is an experimental feature, we don't fail the append on errors, just debug log.
func (a *headAppenderV2) bestEffortAppendSTZeroSample(s *memSeries, st, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) {
	if st >= t {
		a.head.logger.Debug("Error when appending ST", "series", s.lset.String(), "st", st, "t", t, "err", storage.ErrSTNewerThanSample)
		return
	}
	if st < a.minValidTime {
		a.head.logger.Debug("Error when appending ST", "series", s.lset.String(), "st", st, "t", t, "err", storage.ErrOutOfBounds)
		return
	}

	var err error
	switch {
	case fh != nil:
		zeroFloatHistogram := &histogram.FloatHistogram{
			// The STZeroSample represents a counter reset by definition.
			CounterResetHint: histogram.CounterReset,
			// Replicate other fields to avoid needless chunk creation.
			Schema:        fh.Schema,
			ZeroThreshold: fh.ZeroThreshold,
			CustomValues:  fh.CustomValues,
		}
		err = a.appendFloatHistogram(s, st, zeroFloatHistogram, true)
	case h != nil:
		zeroHistogram := &histogram.Histogram{
			// The STZeroSample represents a counter reset by definition.
			CounterResetHint: histogram.CounterReset,
			// Replicate other fields to avoid needless chunk creation.
			Schema:        h.Schema,
			ZeroThreshold: h.ZeroThreshold,
			CustomValues:  h.CustomValues,
		}
		err = a.appendHistogram(s, st, zeroHistogram, true)
	default:
		err = a.appendFloat(s, st, 0, true)
	}

	if err != nil {
		if errors.Is(err, storage.ErrOutOfOrderSample) {
			// OOO errors are common and expected (cumulative). Explicitly ignored.
			return
		}
		a.head.logger.Debug("Error when appending ST", "series", s.lset.String(), "st", st, "t", t, "err", err)
		return
	}

	if st > a.maxt {
		a.maxt = st
	}
}

var _ storage.GetRef = &headAppenderV2{}
