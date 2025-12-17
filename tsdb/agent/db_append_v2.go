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

package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

// AppenderV2 implements storage.AppenderV2.
func (db *DB) AppenderV2(context.Context) storage.AppenderV2 {
	return db.appenderV2Pool.Get().(storage.AppenderV2)
}

type appenderV2 struct {
	appenderBase
}

// Append appends pending sample to agent's DB.
// TODO: Wire metadata in the Agent's appender.
func (a *appenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	var (
		// Avoid shadowing err variables for reliability.
		valErr, partialErr error
		sampleMetricType   = sampleMetricTypeFloat
		isStale            bool
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

	// series references and chunk references are identical for agent mode.
	s := a.series.GetByID(chunks.HeadSeriesRef(ref))
	if s == nil {
		var err error
		s, err = a.getOrCreate(ls)
		if err != nil {
			return 0, err
		}
	}

	s.Lock()
	lastTS := s.lastTs
	s.Unlock()

	// TODO(bwplotka): Handle ST natively (as per PROM-60).
	if a.opts.EnableSTAsZeroSample && st != 0 {
		a.bestEffortAppendSTZeroSample(s, ls, lastTS, st, t, h, fh)
	}

	if t <= a.minValidTime(lastTS) {
		a.metrics.totalOutOfOrderSamples.Inc()
		return 0, storage.ErrOutOfOrderSample
	}

	switch {
	case fh != nil:
		isStale = value.IsStaleNaN(fh.Sum)
		// NOTE: always modify pendingFloatHistograms and floatHistogramSeries together
		a.pendingFloatHistograms = append(a.pendingFloatHistograms, record.RefFloatHistogramSample{
			Ref: s.ref,
			T:   t,
			FH:  fh,
		})
		a.floatHistogramSeries = append(a.floatHistogramSeries, s)
	case h != nil:
		isStale = value.IsStaleNaN(h.Sum)
		// NOTE: always modify pendingHistograms and histogramSeries together
		a.pendingHistograms = append(a.pendingHistograms, record.RefHistogramSample{
			Ref: s.ref,
			T:   t,
			H:   h,
		})
		a.histogramSeries = append(a.histogramSeries, s)
	default:
		isStale = value.IsStaleNaN(v)

		// NOTE: always modify pendingSamples and sampleSeries together.
		a.pendingSamples = append(a.pendingSamples, record.RefSample{
			Ref: s.ref,
			T:   t,
			V:   v,
		})
		a.sampleSeries = append(a.sampleSeries, s)
	}
	a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricType).Inc()
	if isStale {
		// For stale values we never attempt to process metadata/exemplars, claim the success.
		return storage.SeriesRef(s.ref), nil
	}

	// Append exemplars if any and if storage was configured for it.
	// TODO(bwplotka): Agent does not have equivalent of a.head.opts.EnableExemplarStorage && a.head.opts.MaxExemplars.Load() > 0 ?
	if len(opts.Exemplars) > 0 {
		// Currently only exemplars can return partial errors.
		partialErr = a.appendExemplars(s, opts.Exemplars)
	}
	return storage.SeriesRef(s.ref), partialErr
}

func (a *appenderV2) appendExemplars(s *memSeries, exemplar []exemplar.Exemplar) error {
	var errs []error
	for _, e := range exemplar {
		// Ensure no empty labels have gotten through.
		e.Labels = e.Labels.WithoutEmpty()

		if err := a.validateExemplar(s.ref, e); err != nil {
			if !errors.Is(err, storage.ErrDuplicateExemplar) {
				// Except duplicates, return partial errors.
				errs = append(errs, err)
				continue
			}
			if !errors.Is(err, storage.ErrOutOfOrderExemplar) {
				a.logger.Debug("Error while adding an exemplar on AppendSample", "exemplars", fmt.Sprintf("%+v", e), "err", e)
			}
			continue
		}

		a.series.SetLatestExemplar(s.ref, &e)
		a.pendingExamplars = append(a.pendingExamplars, record.RefExemplar{
			Ref:    s.ref,
			T:      e.Ts,
			V:      e.Value,
			Labels: e.Labels,
		})
		a.metrics.totalAppendedExemplars.Inc()
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
func (a *appenderV2) bestEffortAppendSTZeroSample(s *memSeries, ls labels.Labels, lastTS, st, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) {
	// NOTE: Use lset instead of s.lset to avoid locking memSeries. Using s.ref is acceptable without locking.
	if st >= t {
		a.logger.Debug("Error when appending ST", "series", ls.String(), "st", st, "t", t, "err", storage.ErrSTNewerThanSample)
		return
	}
	if st <= lastTS {
		a.logger.Debug("Error when appending ST", "series", ls.String(), "st", st, "t", t, "err", storage.ErrOutOfOrderST)
		return
	}

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
		a.pendingFloatHistograms = append(a.pendingFloatHistograms, record.RefFloatHistogramSample{Ref: s.ref, T: st, FH: zeroFloatHistogram})
		a.floatHistogramSeries = append(a.floatHistogramSeries, s)
		a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
	case h != nil:
		zeroHistogram := &histogram.Histogram{
			// The STZeroSample represents a counter reset by definition.
			CounterResetHint: histogram.CounterReset,
			// Replicate other fields to avoid needless chunk creation.
			Schema:        h.Schema,
			ZeroThreshold: h.ZeroThreshold,
			CustomValues:  h.CustomValues,
		}
		a.pendingHistograms = append(a.pendingHistograms, record.RefHistogramSample{Ref: s.ref, T: st, H: zeroHistogram})
		a.histogramSeries = append(a.histogramSeries, s)
		a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
	default:
		a.pendingSamples = append(a.pendingSamples, record.RefSample{Ref: s.ref, T: st, V: 0})
		a.sampleSeries = append(a.sampleSeries, s)
		a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
	}
}
