// Copyright 2025 The Prometheus Authors
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

package prometheusremotewrite

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	modelLabels "github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite/labels"
)

// CombinedAppender is similar to storage.Appender, but combines updates to
// metadata, created timestamps, exemplars and samples into a single call.
type CombinedAppender interface {
	// AppendSample appends a sample and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendSample(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, es []exemplar.Exemplar) error
	// AppendHistogram appends a histogram and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendHistogram(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, h *histogram.Histogram, es []exemplar.Exemplar) error
	// Commit finalizes the ongoing transaction in storage.
	Commit() error
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
			Name:      "otlp_without_metadata_appended_samples_total",
			Help:      "The total number of received OTLP data points which were ingested without corresponding metadata.",
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
		refs:                           make(map[uint64]labelsRef),
		samplesAppendedWithoutMetadata: metrics.samplesAppendedWithoutMetadata,
		outOfOrderExemplars:            metrics.outOfOrderExemplars,
	}
}

type labelsRef struct {
	ref storage.SeriesRef
	ls  modelLabels.Labels
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
	refs map[uint64]labelsRef
}

func (b *combinedAppender) AppendSample(_ string, rawls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, es []exemplar.Exemplar) (err error) {
	return b.appendFloatOrHistogram(rawls, meta, t, ct, v, nil, es)
}

func (b *combinedAppender) AppendHistogram(_ string, rawls labels.Labels, meta metadata.Metadata, t, ct int64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	if h == nil {
		// Sanity check, we should never get here with a nil histogram.
		ls := modelLabels.NewFromSorted(rawls)
		b.logger.Error("Received nil histogram in CombinedAppender.AppendHistogram", "series", ls.String())
		return nil
	}
	return b.appendFloatOrHistogram(rawls, meta, t, ct, 0, h, es)
}

func (b *combinedAppender) appendFloatOrHistogram(rawls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	ls := modelLabels.NewFromSorted(rawls)
	hash := ls.Hash()
	lref, exists := b.refs[hash]
	ref := lref.ref
	if exists && !modelLabels.Equal(lref.ls, ls) {
		// Hash collision, this is a new series.
		exists = false
	}
	if !exists {
		if ct != 0 && b.ingestCTZeroSample {
			if h != nil {
				ref, err = b.app.AppendHistogramCTZeroSample(ref, ls, t, ct, h, nil)
			} else {
				ref, err = b.app.AppendCTZeroSample(ref, ls, t, ct)
			}
			if err != nil && !errors.Is(err, storage.ErrOutOfOrderCT) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a CT was already ingested in a previous request.
				// We ignore the error.
				b.logger.Warn("Error when appending CT from OTLP", "err", err, "series", ls.String(), "created_timestamp", ct, "timestamp", t, "sample_type", sampleType(h))
			}
		}
	}
	if h != nil {
		ref, err = b.app.AppendHistogram(ref, ls, t, h, nil)
	} else {
		ref, err = b.app.Append(ref, ls, t, v)
	}
	if err != nil {
		// Although Append does not currently return ErrDuplicateSampleForTimestamp there is
		// a note indicating its inclusion in the future.
		if errors.Is(err, storage.ErrOutOfOrderSample) ||
			errors.Is(err, storage.ErrOutOfBounds) ||
			errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
			b.logger.Error("Error when appending sample from OTLP", "err", err.Error(), "series", ls.String(), "timestamp", t, "sample_type", sampleType(h))
		}
	}

	if ref == 0 {
		// We cannot update metadata or add exemplars on non existent series.
		return
	}

	if !exists {
		b.refs[hash] = labelsRef{
			ref: ref,
			ls:  ls,
		}
		// If this is the first time we see this series, set the metadata.
		_, err = b.app.UpdateMetadata(ref, ls, meta)
		if err != nil {
			b.samplesAppendedWithoutMetadata.Add(1)
			b.logger.Warn("Error while updating metadata from OTLP", "err", err)
		}
	}

	b.appendExemplars(ref, ls, es)

	return
}

func sampleType(h *histogram.Histogram) string {
	if h == nil {
		return "float"
	}
	return "histogram"
}

func (b *combinedAppender) Commit() error {
	return b.app.Commit()
}

func (b *combinedAppender) appendExemplars(ref storage.SeriesRef, ls modelLabels.Labels, es []exemplar.Exemplar) storage.SeriesRef {
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
