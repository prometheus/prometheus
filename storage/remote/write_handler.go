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

package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gogo/protobuf/proto"
	deltatocumulative "github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/exemplar"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type writeHandler struct {
	logger     *slog.Logger
	appendable storage.AppendableV2

	samplesWithInvalidLabelsTotal  prometheus.Counter
	samplesAppendedWithoutMetadata prometheus.Counter

	enableTypeAndUnitLabels bool
}

const maxAheadTime = 10 * time.Minute

// NewWriteHandler creates a http.Handler that accepts remote write requests with
// the given message in acceptedMsgs and writes them to the provided appendable.
//
// NOTE(bwplotka): When accepting v2 proto and spec, partial writes are possible
// as per https://prometheus.io/docs/specs/remote_write_spec_2_0/#partial-write.
func NewWriteHandler(logger *slog.Logger, reg prometheus.Registerer, appendable storage.AppendableV2, acceptedMsgs remoteapi.MessageTypes, enableTypeAndUnitLabels bool) http.Handler {
	h := &writeHandler{
		logger:     logger,
		appendable: appendable,
		samplesWithInvalidLabelsTotal: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "remote_write_invalid_labels_samples_total",
			Help:      "The total number of received remote write samples and histogram samples which were rejected due to invalid labels.",
		}),
		samplesAppendedWithoutMetadata: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "remote_write_without_metadata_appended_samples_total",
			Help:      "The total number of received remote write samples (and histogram samples) which were ingested without corresponding metadata.",
		}),

		enableTypeAndUnitLabels: enableTypeAndUnitLabels,
	}
	return remoteapi.NewWriteHandler(h, acceptedMsgs, remoteapi.WithWriteHandlerLogger(logger))
}

// isHistogramValidationError checks if the error is a native histogram validation error.
func isHistogramValidationError(err error) bool {
	var e histogram.Error
	return errors.As(err, &e)
}

// Store implements remoteapi.writeStorage interface.
func (h *writeHandler) Store(r *http.Request, msgType remoteapi.WriteMessageType) (*remoteapi.WriteResponse, error) {
	// Store receives request with decompressed content in body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Error reading remote write request body", "err", err.Error())
		return nil, err
	}

	wr := remoteapi.NewWriteResponse()
	if msgType == remoteapi.WriteV1MessageType {
		// PRW 1.0 flow has different proto message and no partial write handling.
		var req prompb.WriteRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			// TODO(bwplotka): Add more context to responded error?
			h.logger.Error("Error decoding v1 remote write request", "protobuf_message", msgType, "err", err.Error())
			wr.SetStatusCode(http.StatusBadRequest)
			return wr, err
		}
		if err = h.write(r.Context(), &req); err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp), errors.Is(err, storage.ErrTooOldSample):
				// Indicated an out-of-order sample is a bad request to prevent retries.
				wr.SetStatusCode(http.StatusBadRequest)
				return wr, err
			case isHistogramValidationError(err):
				wr.SetStatusCode(http.StatusBadRequest)
				return wr, err
			default:
				wr.SetStatusCode(http.StatusInternalServerError)
				return wr, err
			}
		}
		return wr, nil
	}

	// Remote Write 2.x proto message handling.
	var req writev2.Request
	if err := proto.Unmarshal(body, &req); err != nil {
		// TODO(bwplotka): Add more context to responded error?
		h.logger.Error("Error decoding v2 remote write request", "protobuf_message", msgType, "err", err.Error())
		wr.SetStatusCode(http.StatusBadRequest)
		return wr, err
	}

	respStats, errHTTPCode, err := h.writeV2(r.Context(), &req)
	// Add stats required X-Prometheus-Remote-Write-Written-* response headers.
	wr.Add(respStats)
	if err != nil {
		wr.SetStatusCode(errHTTPCode)
		return wr, err
	}
	return wr, nil
}

func (h *writeHandler) write(ctx context.Context, req *prompb.WriteRequest) (err error) {
	outOfOrderExemplarErrs := 0
	samplesWithInvalidLabels := 0
	samplesAppended := 0

	app := &validationAppender{
		AppenderV2: h.appendable.AppenderV2(ctx),
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
		if err != nil {
			h.samplesAppendedWithoutMetadata.Add(float64(samplesAppended))
		}
	}()

	b := labels.NewScratchBuilder(0)
	var es []exemplar.Exemplar
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, nil)

		// TODO(bwplotka): Even as per 1.0 spec, this should be a 400 error, while other samples are
		// potentially written. Perhaps unify with fixed writeV2 implementation a bit.
		if !ls.Has(model.MetricNameLabel) || !ls.IsValid(model.UTF8Validation) {
			h.logger.Warn("Invalid metric names or labels", "got", ls.String())
			samplesWithInvalidLabels++
			continue
		} else if duplicateLabel, hasDuplicate := ls.HasDuplicateLabelNames(); hasDuplicate {
			h.logger.Warn("Invalid labels for series.", "labels", ls.String(), "duplicated_label", duplicateLabel)
			samplesWithInvalidLabels++
			continue
		}

		es = es[:0]
		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, nil)
			es = append(es, e)
		}

		outOfOrderExemplarErrs, err = h.appendV1Samples(app, ts.Samples, ls, es)
		if err != nil {
			return err
		}
		samplesAppended += len(ts.Samples)

		outOfOrderExemplarErrs, err = h.appendV1Histograms(app, ts.Histograms, ls, es)
		if err != nil {
			return err
		}
		samplesAppended += len(ts.Histograms)
	}

	if outOfOrderExemplarErrs > 0 {
		h.logger.Warn("Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}
	if samplesWithInvalidLabels > 0 {
		h.samplesWithInvalidLabelsTotal.Add(float64(samplesWithInvalidLabels))
	}
	return nil
}

func (h *writeHandler) appendV1Samples(app storage.AppenderV2, ss []prompb.Sample, ls labels.Labels, es []exemplar.Exemplar) (outOfOrderExemplarErrs int, err error) {
	var ref storage.SeriesRef
	for _, s := range ss {
		ref, err = app.Append(ref, ls, 0, s.GetTimestamp(), s.GetValue(), nil, nil, storage.AOptions{Exemplars: es})
		if err != nil {
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				h.logger.Error("Out of order sample from remote write", "err", err.Error(), "series", ls.String(), "timestamp", s.Timestamp)
			}
			var pErr *storage.AppendPartialError
			if errors.As(err, &pErr) {
				for _, e := range pErr.ExemplarErrors {
					if errors.Is(e, storage.ErrOutOfOrderExemplar) {
						outOfOrderExemplarErrs++
						h.logger.Debug("Out of order exemplar", "series", ls.String())
						continue
					}
					// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
					h.logger.Debug("Error while adding exemplar in AppendExemplar", "series", ls.String(), "err", err)
				}
				// Still claim success and continue.
				continue
			}
			return outOfOrderExemplarErrs, err
		}
	}
	return outOfOrderExemplarErrs, nil
}

func (h *writeHandler) appendV1Histograms(app storage.AppenderV2, hh []prompb.Histogram, ls labels.Labels, es []exemplar.Exemplar) (outOfOrderExemplarErrs int, err error) {
	var ref storage.SeriesRef
	for _, hp := range hh {
		ref, err = app.Append(ref, ls, 0, hp.GetTimestamp(), 0, hp.ToIntHistogram(), hp.ToFloatHistogram(), storage.AOptions{Exemplars: es})
		if err != nil {
			// Although Append does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				h.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
			}
			var pErr *storage.AppendPartialError
			if errors.As(err, &pErr) {
				for _, e := range pErr.ExemplarErrors {
					if errors.Is(e, storage.ErrOutOfOrderExemplar) {
						outOfOrderExemplarErrs++
						h.logger.Debug("Out of order exemplar", "series", ls.String())
						continue
					}
					// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
					h.logger.Debug("Error while adding exemplar in AppendExemplar", "series", ls.String(), "err", err)
				}
				// Still claim success and continue.
				continue
			}
			return outOfOrderExemplarErrs, err
		}
	}
	return outOfOrderExemplarErrs, nil
}

// writeV2 is similar to write, but it works with v2 proto message,
// allows partial 4xx writes and gathers statistics.
//
// writeV2 returns the statistics.
// In error cases, writeV2, also returns statistics, but also the error that
// should be propagated to the remote write sender and httpCode to use for status.
//
// NOTE(bwplotka): TSDB storage is NOT idempotent, so we don't allow "partial retry-able" errors.
// Once we have 5xx type of error, we immediately stop and rollback all appends.
func (h *writeHandler) writeV2(ctx context.Context, req *writev2.Request) (_ remoteapi.WriteResponseStats, errHTTPCode int, _ error) {
	app := &validationAppender{
		AppenderV2: h.appendable.AppenderV2(ctx),
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	s := remoteapi.WriteResponseStats{}
	samplesWithoutMetadata, errHTTPCode, err := h.appendV2(app, req, &s)
	if err != nil {
		if errHTTPCode/5 == 100 {
			// On 5xx, we always rollback, because we expect
			// sender to retry and TSDB is not idempotent.
			if rerr := app.Rollback(); rerr != nil {
				h.logger.Error("writev2 rollback failed on retry-able error", "err", rerr)
			}
			return remoteapi.WriteResponseStats{}, errHTTPCode, err
		}

		// Non-retriable (e.g. bad request error case). Can be partially written.
		commitErr := app.Commit()
		if commitErr != nil {
			// Bad requests does not matter as we have internal error (retryable).
			return remoteapi.WriteResponseStats{}, http.StatusInternalServerError, commitErr
		}
		// Bad request error happened, but rest of data (if any) was written.
		h.samplesAppendedWithoutMetadata.Add(float64(samplesWithoutMetadata))
		return s, errHTTPCode, err
	}

	// All good just commit.
	if err := app.Commit(); err != nil {
		return remoteapi.WriteResponseStats{}, http.StatusInternalServerError, err
	}
	h.samplesAppendedWithoutMetadata.Add(float64(samplesWithoutMetadata))
	return s, 0, nil
}

func (h *writeHandler) appendV2(app storage.AppenderV2, req *writev2.Request, rs *remoteapi.WriteResponseStats) (samplesWithoutMetadata, errHTTPCode int, err error) {
	var (
		badRequestErrs                                   []error
		outOfOrderExemplarErrs, samplesWithInvalidLabels int
		es                                               []exemplar.Exemplar

		b = labels.NewScratchBuilder(0)
	)
	for _, ts := range req.Timeseries {
		ls, err := ts.ToLabels(&b, req.Symbols)
		if err != nil {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("parsing labels for series %v: %w", ts.LabelsRefs, err))
			samplesWithInvalidLabels += len(ts.Samples) + len(ts.Histograms)
			continue
		}

		m := ts.ToMetadata(req.Symbols)
		if m.IsEmpty() {
			// Account for missing metadata this TimeSeries.
			samplesWithoutMetadata += rs.AllSamples()
		}

		if h.enableTypeAndUnitLabels && (m.Type != model.MetricTypeUnknown || m.Unit != "") {
			slb := labels.NewScratchBuilder(ls.Len() + 2) // +2 for __type__ and __unit__
			ls.Range(func(l labels.Label) {
				// Skip __type__ and __unit__ labels if they exist in the incoming labels.
				// They will be added from metadata to avoid duplicates.
				if l.Name != model.MetricTypeLabel && l.Name != model.MetricUnitLabel {
					slb.Add(l.Name, l.Value)
				}
			})
			schema.Metadata{Type: m.Type, Unit: m.Unit}.AddToLabels(&slb)
			slb.Sort()
			ls = slb.Labels()
		}

		// Validate series labels early.
		// NOTE(bwplotka): While spec allows UTF-8, Prometheus Receiver may impose
		// specific limits and follow https://prometheus.io/docs/specs/remote_write_spec_2_0/#invalid-samples case.
		if !ls.Has(model.MetricNameLabel) || !ls.IsValid(model.UTF8Validation) {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("invalid metric name or labels, got %v", ls.String()))
			samplesWithInvalidLabels += len(ts.Samples) + len(ts.Histograms)
			continue
		} else if duplicateLabel, hasDuplicate := ls.HasDuplicateLabelNames(); hasDuplicate {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("invalid labels for series, labels %v, duplicated label %s", ls.String(), duplicateLabel))
			samplesWithInvalidLabels += len(ts.Samples) + len(ts.Histograms)
			continue
		}

		// Validate that the TimeSeries has at least one sample or histogram.
		if len(ts.Samples) == 0 && len(ts.Histograms) == 0 {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("TimeSeries must contain at least one sample or histogram for series %v", ls.String()))
			continue
		}
		// Validate that TimeSeries does not have both; it's against the spec e.g. where to attach exemplars to?
		if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("TimeSeries must contain either samples or histograms for series %v not both", ls.String()))
			continue
		}

		var ref storage.SeriesRef

		// Attach potential exemplars to append.
		es = es[:0]
		for _, ep := range ts.Exemplars {
			e, err := ep.ToExemplar(&b, req.Symbols)
			if err != nil {
				badRequestErrs = append(badRequestErrs, fmt.Errorf("parsing exemplar for series %v: %w", ls.String(), err))
				continue
			}
			es = append(es, e)
		}
		appOpts := storage.AppendV2Options{
			Metadata:  m,
			Exemplars: es,
		}
		rs.Exemplars += len(appOpts.Exemplars) // Rejection is accounted later on.

		for _, s := range ts.Samples {
			ref, err = app.Append(ref, ls, s.GetStartTimestamp(), s.GetTimestamp(), s.GetValue(), nil, nil, appOpts)
			if err == nil {
				rs.Samples++
				continue
			}
			// Handle append error.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) ||
				errors.Is(err, storage.ErrTooOldSample) {
				// TODO(bwplotka): Not too spammy log?
				h.logger.Error("Out of order sample from remote write", "err", err.Error(), "series", ls.String(), "timestamp", s.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			var pErr *storage.AppendPartialError
			if errors.As(err, &pErr) {
				for _, e := range pErr.ExemplarErrors {
					rs.Exemplars--
					if errors.Is(e, storage.ErrOutOfOrderExemplar) {
						outOfOrderExemplarErrs++ // Maintain old metrics, but technically not needed, given we fail here.
						h.logger.Error("Out of order exemplar", "err", err.Error(), "series", ls.String())
						badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
						continue
					}
					// Since exemplar storage is still experimental, we don't fail or check other errors.
					// Debug log is emitted in TSDB already.
				}
				// Still claim success and continue.
				rs.Samples++
				continue
			}
			return 0, http.StatusInternalServerError, err
		}

		// Native Histograms.
		for _, hp := range ts.Histograms {
			ref, err = app.Append(ref, ls, hp.GetStartTimestamp(), hp.GetTimestamp(), 0, hp.ToIntHistogram(), hp.ToFloatHistogram(), appOpts)
			if err == nil {
				rs.Histograms++
				continue
			}
			// Handle append error.
			// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				// TODO(bwplotka): Not too spammy log?
				h.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			if isHistogramValidationError(err) {
				h.logger.Error("Invalid histogram received", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			var pErr *storage.AppendPartialError
			if errors.As(err, &pErr) {
				for _, e := range pErr.ExemplarErrors {
					rs.Exemplars--
					if errors.Is(e, storage.ErrOutOfOrderExemplar) {
						outOfOrderExemplarErrs++ // Maintain old metrics, but technically not needed, given we fail here.
						h.logger.Error("Out of order exemplar", "err", err.Error(), "series", ls.String())
						badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
						continue
					}
					// Since exemplar storage is still experimental, we don't fail or check other errors.
					// Debug log is emitted in TSDB already.
				}
				// Still claim success and continue.
				rs.Histograms++
				continue
			}
			return 0, http.StatusInternalServerError, err
		}
	}

	if outOfOrderExemplarErrs > 0 {
		h.logger.Warn("Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}
	h.samplesWithInvalidLabelsTotal.Add(float64(samplesWithInvalidLabels))

	if len(badRequestErrs) == 0 {
		return samplesWithoutMetadata, 0, nil
	}
	// TODO(bwplotka): Better concat formatting? Perhaps add size limit?
	return samplesWithoutMetadata, http.StatusBadRequest, errors.Join(badRequestErrs...)
}

// handleHistogramZeroSample appends ST as a zero-value sample with st value as the sample timestamp.
// It doesn't return errors in case of out of order ST.
func (*writeHandler) handleHistogramZeroSample(app storage.Appender, ref storage.SeriesRef, l labels.Labels, hist writev2.Histogram, st int64) (storage.SeriesRef, error) {
	var err error
	if hist.IsFloatHistogram() {
		ref, err = app.AppendHistogramSTZeroSample(ref, l, hist.Timestamp, st, nil, hist.ToFloatHistogram())
	} else {
		ref, err = app.AppendHistogramSTZeroSample(ref, l, hist.Timestamp, st, hist.ToIntHistogram(), nil)
	}
	return ref, err
}

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
		appendable:              appendable,
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
	app := &validationAppender{
		AppenderV2: rw.appendable.AppenderV2(ctx),
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	// NOTE(bwplotka): When switching to AppenderV2 I skipped 2 things:
	// * Metrics
	//    // TODO: Add, likely in a single place in metrics_to_prw.go
	//		samplesAppendedWithoutMetadata: promauto.With(reg).NewCounter(prometheus.CounterOpts{
	//			Namespace: "prometheus",
	//			Subsystem: "api",
	//			Name:      "otlp_appended_samples_without_metadata_total",
	//			Help:      "The total number of samples ingested from OTLP without corresponding metadata.",
	//		}),
	//    // TODO: Add using storage.AppenderPartialError
	//		outOfOrderExemplars: promauto.With(reg).NewCounter(prometheus.CounterOpts{
	//			Namespace: "prometheus",
	//			Subsystem: "api",
	//			Name:      "otlp_out_of_order_exemplars_total",
	//			Help:      "The total number of received OTLP exemplars which were rejected because they were out of order.",
	//		}),
	//	}
	// * this odd ref cache. This one I propose to skip until we know we need it for efficiency reasons.
	// As a part of a single OTLP message, do we even envision ANY ref to be shared? (it's only one sample per series, no?
	//
	//  // Used to ensure we only update metadata and created timestamps once, and to share storage.SeriesRefs.
	//	// To detect hash collision it also stores the labels.
	//	// There is no overflow/conflict list, the TSDB will handle that part.
	//	refs map[uint64]seriesRef
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

type validationAppender struct {
	storage.AppenderV2

	maxTime int64
}

func (app *validationAppender) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	if h != nil && histogram.IsExponentialSchemaReserved(h.Schema) && h.Schema > histogram.ExponentialSchemaMax {
		if err := h.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
			return 0, err
		}
	}
	if fh != nil && histogram.IsExponentialSchemaReserved(fh.Schema) && fh.Schema > histogram.ExponentialSchemaMax {
		if err := fh.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
			return 0, err
		}
	}
	return app.AppenderV2.Append(ref, ls, st, t, v, h, fh, opts)
}
