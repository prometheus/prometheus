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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

// NewCombinedAppender creates a combined appender that sets start times and
// updates metadata for each series only once, and appends samples and
// exemplars for each call.
func NewCombinedAppender(app storage.Appender, logger *slog.Logger, reg prometheus.Registerer, ingestCTZeroSample bool) CombinedAppender {
	return &combinedAppender{
		app:                app,
		logger:             logger,
		ingestCTZeroSample: ingestCTZeroSample,
		refs:               make(map[uint64]storage.SeriesRef),
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

// CombinedAppender is similar to storage.Appender, but combines updates to
// metadata, created timestamps, exemplars and samples into a single call.
type CombinedAppender interface {
	// AppendSample appends a sample and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendSample(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, es []exemplar.Exemplar) error
	// AppendSample appends a histogram and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendHistogram(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, h *histogram.Histogram, es []exemplar.Exemplar) error
}

type combinedAppender struct {
	app                            storage.Appender
	logger                         *slog.Logger
	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
	ingestCTZeroSample             bool
	// Used to ensure we only update metadata and created timestamps once, and to share storage.SeriesRefs.
	refs map[uint64]storage.SeriesRef
}

func (b *combinedAppender) AppendSample(_ string, ls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, es []exemplar.Exemplar) (err error) {
	hash := ls.Hash()
	ref, exists := b.refs[hash]
	if !exists {
		ref, err = b.app.UpdateMetadata(0, ls, meta)
		if err != nil {
			b.samplesAppendedWithoutMetadata.Add(1)
			b.logger.Debug("error while updating metadata from OTLP", "err", err)
		}
		if ct != 0 && b.ingestCTZeroSample {
			ref, err = b.app.AppendCTZeroSample(ref, ls, t, ct)
			if err != nil && !errors.Is(err, storage.ErrOutOfOrderCT) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a CT was already ingested in a previous request.
				// We ignore the error.
				b.logger.Debug("Error when appending CT in OTLP request", "err", err, "series", ls.String(), "created_timestamp", ct, "timestamp", t)
			}
		}
	}
	ref, err = b.app.Append(ref, ls, t, v)
	if err != nil {
		// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
		// a note indicating its inclusion in the future.
		if errors.Is(err, storage.ErrOutOfOrderSample) ||
			errors.Is(err, storage.ErrOutOfBounds) ||
			errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
			b.logger.Error("Out of order sample from OTLP", "err", err.Error(), "series", ls.String(), "timestamp", t)
		}
	}
	ref = b.appendExemplars(ref, ls, es)
	b.refs[hash] = ref
	return
}

func (b *combinedAppender) AppendHistogram(_ string, ls labels.Labels, meta metadata.Metadata, t, ct int64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	hash := ls.Hash()
	ref, exists := b.refs[hash]
	if !exists {
		ref, err = b.app.UpdateMetadata(0, ls, meta)
		if err != nil {
			b.samplesAppendedWithoutMetadata.Add(1)
			b.logger.Debug("error while updating metadata from OTLP", "err", err)
		}
		if b.ingestCTZeroSample {
			ref, err = b.app.AppendHistogramCTZeroSample(ref, ls, t, ct, h, nil)
			if err != nil && !errors.Is(err, storage.ErrOutOfOrderCT) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a CT was already ingested in a previous request.
				// We ignore the error.
				b.logger.Debug("Error when appending Histogram CT in remote write request", "err", err, "series", ls.String(), "created_timestamp", ct, "timestamp", t)
			}
		}
	}
	ref, err = b.app.AppendHistogram(ref, ls, t, h, nil)
	if err != nil {
		// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
		// a note indicating its inclusion in the future.
		if errors.Is(err, storage.ErrOutOfOrderSample) ||
			errors.Is(err, storage.ErrOutOfBounds) ||
			errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
			b.logger.Error("Out of order histogram from OTLP", "err", err.Error(), "series", ls.String(), "timestamp", t)
		}
	}
	ref = b.appendExemplars(ref, ls, es)
	b.refs[hash] = ref
	return
}

func (b *combinedAppender) appendExemplars(ref storage.SeriesRef, ls labels.Labels, es []exemplar.Exemplar) storage.SeriesRef {
	var err error
	for _, e := range es {
		if ref, err = b.app.AppendExemplar(ref, ls, e); err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderExemplar):
				b.outOfOrderExemplars.Add(1)
				b.logger.Debug("Out of order exemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
			default:
				// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
				b.logger.Debug("Error while adding exemplar in AppendExemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e), "err", err)
			}
		}
	}
	return ref
}
