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

package remote

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type OTLPOptions struct {
	// Store the raw delta samples as metrics with unknown type (we don't have a proper type for delta yet, therefore
	// marking the metric type as unknown for now).
	// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/)
	NativeDelta bool
	// LookbackDelta is the query lookback delta.
	// Used to calculate the target_info sample timestamp interval.
	LookbackDelta time.Duration
	// Add type and unit labels to the metrics.
	EnableTypeAndUnitLabels bool
}

// NewOTLPWriteHandler creates a http.Handler that accepts OTLP write requests and
// writes them to the provided appendable.
func NewOTLPWriteHandler(logger *slog.Logger, reg prometheus.Registerer, appendable storage.AppendableV2, configFunc func() config.Config, opts OTLPOptions) http.Handler {
	return &otlpWriteHandler{
		logger:                  logger,
		appendable:              newOTLPInstrumentedAppendable(reg, appendable),
		configFunc:              configFunc,
		allowDeltaTemporality:   opts.NativeDelta,
		lookbackDelta:           opts.LookbackDelta,
		enableTypeAndUnitLabels: opts.EnableTypeAndUnitLabels,
		converterPool: sync.Pool{
			New: func() any {
				return otlptranslator.NewPrometheusConverter(nil)
			},
		},
		translationWarnings: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "otlp_translation_warnings_total",
			Help:      "The total number of warnings produced while translating OTLP metrics to the Prometheus model, by category.",
		}, []string{"category"}),
	}
}

type otlpWriteHandler struct {
	logger                  *slog.Logger
	appendable              storage.AppendableV2
	configFunc              func() config.Config
	allowDeltaTemporality   bool
	lookbackDelta           time.Duration
	enableTypeAndUnitLabels bool
	translationWarnings     *prometheus.CounterVec
	converterPool           sync.Pool
}

func (h *otlpWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeOTLPWriteRequest(r)
	if err != nil {
		h.logger.Error("Error decoding OTLP write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.writeMetrics(r.Context(), req.Metrics())

	switch {
	case err == nil:
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp), errors.Is(err, storage.ErrTooOldSample):
		// Indicated an out of order sample is a bad request to prevent retries.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		h.logger.Error("Error appending remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *otlpWriteHandler) writeMetrics(ctx context.Context, md pmetric.Metrics) error {
	otlpCfg := h.configFunc().OTLPConfig
	app := &remoteWriteAppenderV2{
		AppenderV2: h.appendable.AppenderV2(ctx),
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}
	converter := h.converterPool.Get().(*otlptranslator.PrometheusConverter)
	converter.Reset(app)
	annots, err := converter.FromMetrics(ctx, md, otlptranslator.Settings{
		AddMetricSuffixes:                    otlpCfg.TranslationStrategy.ShouldAddSuffixes(),
		AllowUTF8:                            !otlpCfg.TranslationStrategy.ShouldEscape(),
		PromoteResourceAttributes:            otlptranslator.NewPromoteResourceAttributes(otlpCfg),
		KeepIdentifyingResourceAttributes:    otlpCfg.KeepIdentifyingResourceAttributes,
		ConvertHistogramsToNHCB:              otlpCfg.ConvertHistogramsToNHCB,
		PromoteScopeMetadata:                 otlpCfg.PromoteScopeMetadata,
		AllowDeltaTemporality:                h.allowDeltaTemporality,
		LookbackDelta:                        h.lookbackDelta,
		EnableTypeAndUnitLabels:              h.enableTypeAndUnitLabels,
		LabelNameUnderscoreSanitization:      otlpCfg.LabelNameUnderscoreSanitization,
		LabelNamePreserveMultipleUnderscores: otlpCfg.LabelNamePreserveMultipleUnderscores,
	})

	defer func() {
		if err != nil {
			_ = app.Rollback()
		} else {
			err = app.Commit()
		}
		converter.Reset(nil)
		h.converterPool.Put(converter)
	}()
	ws, _ := annots.AsStrings("", 0, 0)
	if len(ws) > 0 {
		for category, count := range otlptranslator.CountWarningsByCategory(annots) {
			h.translationWarnings.WithLabelValues(string(category)).Add(float64(count))
		}
		h.logger.Warn("Warnings translating OTLP metrics to Prometheus write request", "warnings", ws)
	}
	return err
}

type otlpInstrumentedAppendable struct {
	storage.AppendableV2

	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
}

// newOTLPInstrumentedAppendable instruments some OTLP metrics per append and
// handles partial errors, so the caller does not need to.
func newOTLPInstrumentedAppendable(reg prometheus.Registerer, app storage.AppendableV2) *otlpInstrumentedAppendable {
	return &otlpInstrumentedAppendable{
		AppendableV2: app,
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

func (a *otlpInstrumentedAppendable) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return &otlpInstrumentedAppender{
		AppenderV2: a.AppendableV2.AppenderV2(ctx),

		samplesAppendedWithoutMetadata: a.samplesAppendedWithoutMetadata,
		outOfOrderExemplars:            a.outOfOrderExemplars,
	}
}

type otlpInstrumentedAppender struct {
	storage.AppenderV2

	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
}

func (app *otlpInstrumentedAppender) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	ref, err := app.AppenderV2.Append(ref, ls, st, t, v, h, fh, opts)
	if err != nil {
		var partialErr *storage.AppendPartialError
		partialErr, hErr := partialErr.Handle(err)
		if hErr != nil {
			// Not a partial error, return err.
			return 0, err
		}
		app.outOfOrderExemplars.Add(float64(len(partialErr.ExemplarErrors)))
		// Hide the partial error as otlp converter does not handle it.
	}
	if opts.Metadata.IsEmpty() {
		app.samplesAppendedWithoutMetadata.Inc()
	}
	return ref, nil
}
