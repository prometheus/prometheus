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
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/prometheus/prometheus/config"
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
	// IngestSTZeroSample enables writing zero samples based on the start time
	// of metrics.
	IngestSTZeroSample bool
	// AppendMetadata enables writing metadata to WAL when metadata-wal-records feature is enabled.
	AppendMetadata bool
}

// NewOTLPWriteHandler creates a http.Handler that accepts OTLP write requests and
// writes them to the provided appendable.
func NewOTLPWriteHandler(logger *slog.Logger, reg prometheus.Registerer, appendable storage.Appendable, configFunc func() config.Config, opts OTLPOptions) http.Handler {
	if opts.NativeDelta && opts.ConvertDelta {
		// This should be validated when iterating through feature flags, so not expected to fail here.
		panic("cannot enable native delta ingestion and delta2cumulative conversion at the same time")
	}

	ex := &rwExporter{
		logger:                  logger,
		appendable:              appendable,
		config:                  configFunc,
		allowDeltaTemporality:   opts.NativeDelta,
		lookbackDelta:           opts.LookbackDelta,
		ingestSTZeroSample:      opts.IngestSTZeroSample,
		enableTypeAndUnitLabels: opts.EnableTypeAndUnitLabels,
		appendMetadata:          opts.AppendMetadata,
		// Register metrics.
		metrics: otlptranslator.NewCombinedAppenderMetrics(reg),
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
	appendable              storage.Appendable
	config                  func() config.Config
	allowDeltaTemporality   bool
	lookbackDelta           time.Duration
	ingestSTZeroSample      bool
	enableTypeAndUnitLabels bool
	appendMetadata          bool

	// Metrics.
	metrics otlptranslator.CombinedAppenderMetrics
}

func (rw *rwExporter) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	otlpCfg := rw.config().OTLPConfig
	app := &remoteWriteAppender{
		Appender: rw.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}
	combinedAppender := otlptranslator.NewCombinedAppender(app, rw.logger, rw.ingestSTZeroSample, rw.appendMetadata, rw.metrics)
	converter := otlptranslator.NewPrometheusConverter(combinedAppender)
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
