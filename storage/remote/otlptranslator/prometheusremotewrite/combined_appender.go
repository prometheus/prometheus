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

// TODO(krajorama): rename this package to otlpappender or similar, as it is
// not specific to Prometheus remote write anymore.
// Note otlptranslator is already used by prometheus/otlptranslator repo.
package prometheusremotewrite

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

// Metadata extends metadata.Metadata with the metric family name.
// OTLP calculates the metric family name for all metrics and uses
// it for generating summary, histogram series by adding the magic
// suffixes. The metric family name is passed down to the appender
// in case the storage needs it for metadata updates.
// Known user is Mimir that implements /api/v1/metadata and uses
// Remote-Write 1.0 for this. Might be removed later if no longer
// needed by any downstream project.
type Metadata struct {
	metadata.Metadata
	MetricFamilyName string
}

// CombinedAppender is similar to storage.Appender, but combines updates to
// metadata, created timestamps, exemplars and samples into a single call.
type CombinedAppender interface {
	// AppendSample appends a sample and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendSample(ls labels.Labels, meta Metadata, ct, t int64, v float64, es []exemplar.Exemplar) error
	// AppendHistogram appends a histogram and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendHistogram(ls labels.Labels, meta Metadata, ct, t int64, h *histogram.Histogram, es []exemplar.Exemplar) error
}

// CombinedAppenderMetrics is for the metrics observed by the
// combinedAppender implementation.
type CombinedAppenderMetrics struct {
	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
}

func NewCombinedAppenderMetrics(reg prometheus.Registerer) CombinedAppenderMetrics {
	return CombinedAppenderMetrics{
		samplesAppendedWithoutMetadata: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "otlp_appended_samples_without_metadata_total",
			Help:      "The total number of samples ingested from OTLP without corresponding metadata.",
		}),
		outOfOrderExemplars: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "otlp_out_of_order_exemplars_total",
			Help:      "The total number of received OTLP exemplars which were rejected because they were out of order.",
		}),
	}
}

// NewCombinedAppender creates a combined appender that sets start times and
// updates metadata for each series only once, and appends samples and
// exemplars for each call.
func NewCombinedAppender(app storage.Appender, logger *slog.Logger, ingestCTZeroSample bool, metrics CombinedAppenderMetrics) CombinedAppender {
	return &combinedAppender{
		app:                            app,
		logger:                         logger,
		ingestCTZeroSample:             ingestCTZeroSample,
		refs:                           make(map[uint64]seriesRef),
		samplesAppendedWithoutMetadata: metrics.samplesAppendedWithoutMetadata,
		outOfOrderExemplars:            metrics.outOfOrderExemplars,
	}
}

type seriesRef struct {
	ref  storage.SeriesRef
	ct   int64
	ls   labels.Labels
	meta metadata.Metadata
}

type combinedAppender struct {
	app                            storage.Appender
	logger                         *slog.Logger
	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
	ingestCTZeroSample             bool
	// Used to ensure we only update metadata and created timestamps once, and to share storage.SeriesRefs.
	// To detect hash collision it also stores the labels.
	// There is no overflow/conflict list, the TSDB will handle that part.
	refs map[uint64]seriesRef
}

func (b *combinedAppender) AppendSample(ls labels.Labels, meta Metadata, ct, t int64, v float64, es []exemplar.Exemplar) (err error) {
	return b.appendFloatOrHistogram(ls, meta.Metadata, ct, t, v, nil, es)
}

func (b *combinedAppender) AppendHistogram(ls labels.Labels, meta Metadata, ct, t int64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	if h == nil {
		// Sanity check, we should never get here with a nil histogram.
		b.logger.Error("Received nil histogram in CombinedAppender.AppendHistogram", "series", ls.String())
		return errors.New("internal error, attempted to append nil histogram")
	}
	return b.appendFloatOrHistogram(ls, meta.Metadata, ct, t, 0, h, es)
}

func (b *combinedAppender) appendFloatOrHistogram(ls labels.Labels, meta metadata.Metadata, ct, t int64, v float64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	hash := ls.Hash()
	series, exists := b.refs[hash]
	ref := series.ref
	if exists && !labels.Equal(series.ls, ls) {
		// Hash collision. The series reference we stored is pointing to a
		// different series so we cannot use it, we need to reset the
		// reference and cache.
		// Note: we don't need to keep track of conflicts here,
		// the TSDB will handle that part when we pass 0 reference.
		exists = false
		ref = 0
	}
	updateRefs := !exists || series.ct != ct
	if updateRefs && ct != 0 && ct < t && b.ingestCTZeroSample {
		var newRef storage.SeriesRef
		if h != nil {
			newRef, err = b.app.AppendHistogramCTZeroSample(ref, ls, t, ct, h, nil)
		} else {
			newRef, err = b.app.AppendCTZeroSample(ref, ls, t, ct)
		}
		if err != nil {
			if !errors.Is(err, storage.ErrOutOfOrderCT) && !errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a CT was already ingested in a previous request.
				// We ignore the error.
				// ErrDuplicateSampleForTimestamp is also a common scenario because
				// unknown start times in Opentelemetry are indicated by setting
				// the start time to the same as the first sample time.
				// https://opentelemetry.io/docs/specs/otel/metrics/data-model/#cumulative-streams-handling-unknown-start-time
				b.logger.Warn("Error when appending CT from OTLP", "err", err, "series", ls.String(), "created_timestamp", ct, "timestamp", t, "sample_type", sampleType(h))
			}
		} else {
			// We only use the returned reference on success as otherwise an
			// error of CT append could invalidate the series reference.
			ref = newRef
		}
	}
	{
		var newRef storage.SeriesRef
		if h != nil {
			newRef, err = b.app.AppendHistogram(ref, ls, t, h, nil)
		} else {
			newRef, err = b.app.Append(ref, ls, t, v)
		}
		if err != nil {
			// Although Append does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				b.logger.Error("Error when appending sample from OTLP", "err", err.Error(), "series", ls.String(), "timestamp", t, "sample_type", sampleType(h))
			}
		} else {
			// If the append was successful, we can use the returned reference.
			ref = newRef
		}
	}

	if ref == 0 {
		// We cannot update metadata or add exemplars on non existent series.
		return err
	}

	if !exists || series.meta.Help != meta.Help || series.meta.Type != meta.Type || series.meta.Unit != meta.Unit {
		updateRefs = true
		// If this is the first time we see this series, set the metadata.
		_, err := b.app.UpdateMetadata(ref, ls, meta)
		if err != nil {
			b.samplesAppendedWithoutMetadata.Add(1)
			b.logger.Warn("Error while updating metadata from OTLP", "err", err)
		}
	}

	if updateRefs {
		b.refs[hash] = seriesRef{
			ref:  ref,
			ct:   ct,
			ls:   ls,
			meta: meta,
		}
	}

	b.appendExemplars(ref, ls, es)

	return err
}

func sampleType(h *histogram.Histogram) string {
	if h == nil {
		return "float"
	}
	return "histogram"
}

func (b *combinedAppender) appendExemplars(ref storage.SeriesRef, ls labels.Labels, es []exemplar.Exemplar) storage.SeriesRef {
	var err error
	for _, e := range es {
		if ref, err = b.app.AppendExemplar(ref, ls, e); err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderExemplar):
				b.outOfOrderExemplars.Add(1)
				b.logger.Debug("Out of order exemplar from OTLP", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
			default:
				// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
				b.logger.Debug("Error while adding exemplar from OTLP", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e), "err", err)
			}
		}
	}
	return ref
}
