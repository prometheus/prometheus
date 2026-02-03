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
	"fmt"
	"log/slog"
	"net/http"
	"time"

	deltatocumulative "github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type OTLPOptions struct {
	// Convert delta samples to their cumulative equivalent by aggregating in-memory
	ConvertDelta bool
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
	if opts.NativeDelta && opts.ConvertDelta {
		// This should be validated when iterating through feature flags, so not expected to fail here.
		panic("cannot enable native delta ingestion and delta2cumulative conversion at the same time")
	}

	ex := &rwExporter{
		logger:                  logger,
		appendable:              newOTLPInstrumentedAppendable(reg, appendable),
		config:                  configFunc,
		allowDeltaTemporality:   opts.NativeDelta,
		lookbackDelta:           opts.LookbackDelta,
		enableTypeAndUnitLabels: opts.EnableTypeAndUnitLabels,
	}

	wh := &otlpWriteHandler{logger: logger, defaultConsumer: ex}

	if opts.ConvertDelta {
		fac := deltatocumulative.NewFactory()
		set := processor.Settings{
			ID:                component.NewID(fac.Type()),
			TelemetrySettings: component.TelemetrySettings{MeterProvider: noop.NewMeterProvider()},
		}
		d2c, err := fac.CreateMetrics(context.Background(), set, fac.CreateDefaultConfig(), wh.defaultConsumer)
		if err != nil {
			// fac.CreateMetrics directly calls [deltatocumulativeprocessor.createMetricsProcessor],
			// which only errors if:
			//   - cfg.(type) != *Config
			//   - telemetry.New fails due to bad set.TelemetrySettings
			//
			// both cannot be the case, as we pass a valid *Config and valid TelemetrySettings.
			// as such, we assume this error to never occur.
			// if it is, our assumptions are broken in which case a panic seems acceptable.
			panic(fmt.Errorf("failed to create metrics processor: %w", err))
		}
		if err := d2c.Start(context.Background(), nil); err != nil {
			// deltatocumulative does not error on start. see above for panic reasoning
			panic(err)
		}
		wh.d2cConsumer = d2c
	}

	return wh
}

type rwExporter struct {
	logger                  *slog.Logger
	appendable              storage.AppendableV2
	config                  func() config.Config
	allowDeltaTemporality   bool
	lookbackDelta           time.Duration
	enableTypeAndUnitLabels bool
}

func (rw *rwExporter) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	otlpCfg := rw.config().OTLPConfig
	app := &remoteWriteAppenderV2{
		AppenderV2: rw.appendable.AppenderV2(ctx),
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}
	converter := otlptranslator.NewPrometheusConverter(app)
	annots, err := converter.FromMetrics(ctx, md, otlptranslator.Settings{
		AddMetricSuffixes:                    otlpCfg.TranslationStrategy.ShouldAddSuffixes(),
		AllowUTF8:                            !otlpCfg.TranslationStrategy.ShouldEscape(),
		PromoteResourceAttributes:            otlptranslator.NewPromoteResourceAttributes(otlpCfg),
		KeepIdentifyingResourceAttributes:    otlpCfg.KeepIdentifyingResourceAttributes,
		ConvertHistogramsToNHCB:              otlpCfg.ConvertHistogramsToNHCB,
		PromoteScopeMetadata:                 otlpCfg.PromoteScopeMetadata,
		AllowDeltaTemporality:                rw.allowDeltaTemporality,
		LookbackDelta:                        rw.lookbackDelta,
		EnableTypeAndUnitLabels:              rw.enableTypeAndUnitLabels,
		LabelNameUnderscoreSanitization:      otlpCfg.LabelNameUnderscoreSanitization,
		LabelNamePreserveMultipleUnderscores: otlpCfg.LabelNamePreserveMultipleUnderscores,
	})

	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
	}()
	ws, _ := annots.AsStrings("", 0, 0)
	if len(ws) > 0 {
		rw.logger.Warn("Warnings translating OTLP metrics to Prometheus write request", "warnings", ws)
	}
	return err
}

func (*rwExporter) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

type otlpWriteHandler struct {
	logger *slog.Logger

	defaultConsumer consumer.Metrics // stores deltas as-is
	d2cConsumer     consumer.Metrics // converts deltas to cumulative
}

func (h *otlpWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeOTLPWriteRequest(r)
	if err != nil {
		h.logger.Error("Error decoding OTLP write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	md := req.Metrics()
	// If deltatocumulative conversion enabled AND delta samples exist, use slower conversion path.
	// While deltatocumulative can also accept cumulative metrics (and then just forwards them as-is), it currently
	// holds a sync.Mutex when entering ConsumeMetrics. This is slow and not necessary when ingesting cumulative metrics.
	if h.d2cConsumer != nil && hasDelta(md) {
		err = h.d2cConsumer.ConsumeMetrics(r.Context(), md)
	} else {
		// Otherwise use default consumer (alongside cumulative samples, this will accept delta samples and write as-is
		// if native-delta-support is enabled).
		err = h.defaultConsumer.ConsumeMetrics(r.Context(), md)
	}

	switch {
	case err == nil:
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
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

func hasDelta(md pmetric.Metrics) bool {
	for i := range md.ResourceMetrics().Len() {
		sms := md.ResourceMetrics().At(i).ScopeMetrics()
		for i := range sms.Len() {
			ms := sms.At(i).Metrics()
			for i := range ms.Len() {
				temporality := pmetric.AggregationTemporalityUnspecified
				m := ms.At(i)
				switch ms.At(i).Type() {
				case pmetric.MetricTypeSum:
					temporality = m.Sum().AggregationTemporality()
				case pmetric.MetricTypeExponentialHistogram:
					temporality = m.ExponentialHistogram().AggregationTemporality()
				case pmetric.MetricTypeHistogram:
					temporality = m.Histogram().AggregationTemporality()
				}
				if temporality == pmetric.AggregationTemporalityDelta {
					return true
				}
			}
		}
	}
	return false
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
