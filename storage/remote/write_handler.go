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
	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"

	deltatocumulative "github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/metric/noop"
)

type writeStorage struct {
	logger     *slog.Logger
	appendable storage.Appendable

	samplesWithInvalidLabelsTotal  prometheus.Counter
	samplesAppendedWithoutMetadata prometheus.Counter

	ingestCTZeroSample bool
}

const maxAheadTime = 10 * time.Minute

// NewWriteHandler creates a http.Handler that accepts remote write requests with
// the given message in acceptedMessages and writes them to the provided appendable.
//
// NOTE(bwplotka): When accepting v2 proto and spec, partial writes are possible
// as per https://prometheus.io/docs/specs/remote_write_spec_2_0/#partial-write.
func NewWriteHandler(logger *slog.Logger, reg prometheus.Registerer, appendable storage.Appendable, acceptedMessages remote.MessageTypes, ingestCTZeroSample bool) http.Handler {
	s := &writeStorage{
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

		ingestCTZeroSample: ingestCTZeroSample,
	}

	return remote.NewHandler(s, acceptedMessages, remote.WithHandlerLogger(logger))
}

// Store implements remote.writeStorage interface.
func (s *writeStorage) Store(ctx context.Context, msgType remote.WriteMessageType, r *http.Request) (*remote.WriteResponse, error) {
	// Store receives request with snappy-decompressed content in body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("Error reading remote write request body", "err", err.Error())
		return nil, err
	}

	wr := remote.NewWriteResponse()
	if msgType == remote.WriteV1MessageType {
		// PRW 1.0 flow has different proto message and no partial write handling.
		var req prompb.WriteRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			// TODO(bwplotka): Add more context to responded error?
			s.logger.Error("Error decoding v1 remote write request", "protobuf_message", msgType, "err", err.Error())
			wr.SetStatusCode(http.StatusBadRequest)
			return wr, err
		}
		if err = s.write(ctx, &req); err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp), errors.Is(err, storage.ErrTooOldSample):
				// Indicated an out-of-order sample is a bad request to prevent retries.
				wr.SetStatusCode(http.StatusBadRequest)
				return wr, err
			default:
				s.logger.Error("Error while remote writing the v1 request", "err", err.Error())
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
		s.logger.Error("Error decoding v2 remote write request", "protobuf_message", msgType, "err", err.Error())
		wr.SetStatusCode(http.StatusBadRequest)
		return wr, err
	}

	respStats, errHTTPCode, err := s.writeV2(ctx, &req)
	if err != nil {
		if errHTTPCode/5 == 100 { // 5xx
			s.logger.Error("Error while remote writing the v2 request", "err", err.Error())
		}
		// Add stats required X-Prometheus-Remote-Write-Written-* response headers even for partial writes.
		wr.Add(respStats)
		wr.SetStatusCode(errHTTPCode)
		return wr, err
	}

	// Add stats required X-Prometheus-Remote-Write-Written-* response headers.
	wr.Add(respStats)
	wr.SetStatusCode(http.StatusNoContent)
	return wr, nil
}

func (s *writeStorage) write(ctx context.Context, req *prompb.WriteRequest) (err error) {
	outOfOrderExemplarErrs := 0
	samplesWithInvalidLabels := 0
	samplesAppended := 0

	app := &timeLimitAppender{
		Appender: s.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
		if err != nil {
			s.samplesAppendedWithoutMetadata.Add(float64(samplesAppended))
		}
	}()

	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, nil)

		// TODO(bwplotka): Even as per 1.0 spec, this should be a 400 error, while other samples are
		// potentially written. Perhaps unify with fixed writeV2 implementation a bit.
		if !ls.Has(labels.MetricName) || !ls.IsValid(model.NameValidationScheme) {
			s.logger.Warn("Invalid metric names or labels", "got", ls.String())
			samplesWithInvalidLabels++
			continue
		} else if duplicateLabel, hasDuplicate := ls.HasDuplicateLabelNames(); hasDuplicate {
			s.logger.Warn("Invalid labels for series.", "labels", ls.String(), "duplicated_label", duplicateLabel)
			samplesWithInvalidLabels++
			continue
		}

		if err := s.appendV1Samples(app, ts.Samples, ls); err != nil {
			return err
		}
		samplesAppended += len(ts.Samples)

		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, nil)
			if _, err := app.AppendExemplar(0, ls, e); err != nil {
				switch {
				case errors.Is(err, storage.ErrOutOfOrderExemplar):
					outOfOrderExemplarErrs++
					s.logger.Debug("Out of order exemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
				default:
					// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
					s.logger.Debug("Error while adding exemplar in AppendExemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e), "err", err)
				}
			}
		}

		if err = s.appendV1Histograms(app, ts.Histograms, ls); err != nil {
			return err
		}
		samplesAppended += len(ts.Histograms)
	}

	if outOfOrderExemplarErrs > 0 {
		s.logger.Warn("Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}
	if samplesWithInvalidLabels > 0 {
		s.samplesWithInvalidLabelsTotal.Add(float64(samplesWithInvalidLabels))
	}
	return nil
}

func (s *writeStorage) appendV1Samples(app storage.Appender, ss []prompb.Sample, labels labels.Labels) error {
	var ref storage.SeriesRef
	var err error
	for _, sample := range ss {
		ref, err = app.Append(ref, labels, sample.Timestamp, sample.Value)
		if err != nil {
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				s.logger.Error("Out of order sample from remote write", "err", err.Error(), "series", labels.String(), "timestamp", sample.Timestamp)
			}
			return err
		}
	}
	return nil
}

func (s *writeStorage) appendV1Histograms(app storage.Appender, hh []prompb.Histogram, labels labels.Labels) error {
	var err error
	for _, hp := range hh {
		if hp.IsFloatHistogram() {
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, nil, hp.ToFloatHistogram())
		} else {
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hp.ToIntHistogram(), nil)
		}
		if err != nil {
			// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				s.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", labels.String(), "timestamp", hp.Timestamp)
			}
			return err
		}
	}
	return nil
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
func (s *writeStorage) writeV2(ctx context.Context, req *writev2.Request) (_ remote.WriteResponseStats, errHTTPCode int, _ error) {
	app := &timeLimitAppender{
		Appender: s.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	stats := remote.WriteResponseStats{}
	samplesWithoutMetadata, errHTTPCode, err := s.appendV2(app, req, &stats)
	if err != nil {
		if errHTTPCode/5 == 100 {
			// On 5xx, we always rollback, because we expect
			// sender to retry and TSDB is not idempotent.
			if rerr := app.Rollback(); rerr != nil {
				s.logger.Error("writev2 rollback failed on retry-able error", "err", rerr)
			}
			return remote.WriteResponseStats{}, errHTTPCode, err
		}

		// Non-retriable (e.g. bad request error case). Can be partially written.
		commitErr := app.Commit()
		if commitErr != nil {
			// Bad requests does not matter as we have internal error (retryable).
			return remote.WriteResponseStats{}, http.StatusInternalServerError, commitErr
		}
		// Bad request error happened, but rest of data (if any) was written.
		s.samplesAppendedWithoutMetadata.Add(float64(samplesWithoutMetadata))
		return stats, errHTTPCode, err
	}

	// All good just commit.
	if err := app.Commit(); err != nil {
		return remote.WriteResponseStats{}, http.StatusInternalServerError, err
	}
	s.samplesAppendedWithoutMetadata.Add(float64(samplesWithoutMetadata))
	return stats, 0, nil
}

func (s *writeStorage) appendV2(app storage.Appender, req *writev2.Request, rs *remote.WriteResponseStats) (samplesWithoutMetadata, errHTTPCode int, err error) {
	var (
		badRequestErrs                                   []error
		outOfOrderExemplarErrs, samplesWithInvalidLabels int

		b = labels.NewScratchBuilder(0)
	)
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, req.Symbols)
		// Validate series labels early.
		// NOTE(bwplotka): While spec allows UTF-8, Prometheus Receiver may impose
		// specific limits and follow https://prometheus.io/docs/specs/remote_write_spec_2_0/#invalid-samples case.
		if !ls.Has(labels.MetricName) || !ls.IsValid(model.NameValidationScheme) {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("invalid metric name or labels, got %v", ls.String()))
			samplesWithInvalidLabels += len(ts.Samples) + len(ts.Histograms)
			continue
		} else if duplicateLabel, hasDuplicate := ls.HasDuplicateLabelNames(); hasDuplicate {
			badRequestErrs = append(badRequestErrs, fmt.Errorf("invalid labels for series, labels %v, duplicated label %s", ls.String(), duplicateLabel))
			samplesWithInvalidLabels += len(ts.Samples) + len(ts.Histograms)
			continue
		}

		allSamplesSoFar := rs.AllSamples()
		var ref storage.SeriesRef

		// Samples.
		if s.ingestCTZeroSample && len(ts.Samples) > 0 && ts.Samples[0].Timestamp != 0 && ts.CreatedTimestamp != 0 {
			// CT only needs to be ingested for the first sample, it will be considered
			// out of order for the rest.
			ref, err = app.AppendCTZeroSample(ref, ls, ts.Samples[0].Timestamp, ts.CreatedTimestamp)
			if err != nil && !errors.Is(err, storage.ErrOutOfOrderCT) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a CT was already ingested in a previous request.
				// We ignore the error.
				s.logger.Debug("Error when appending CT in remote write request", "err", err, "series", ls.String(), "created_timestamp", ts.CreatedTimestamp, "timestamp", ts.Samples[0].Timestamp)
			}
		}
		for _, sample := range ts.Samples {
			ref, err = app.Append(ref, ls, sample.GetTimestamp(), sample.GetValue())
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
				s.logger.Error("Out of order sample from remote write", "err", err.Error(), "series", ls.String(), "timestamp", sample.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			return 0, http.StatusInternalServerError, err
		}

		// Native Histograms.
		for _, hp := range ts.Histograms {
			if s.ingestCTZeroSample && hp.Timestamp != 0 && ts.CreatedTimestamp != 0 {
				// Differently from samples, we need to handle CT for each histogram instead of just the first one.
				// This is because histograms and float histograms are stored separately, even if they have the same labels.
				ref, err = s.handleHistogramZeroSample(app, ref, ls, hp, ts.CreatedTimestamp)
				if err != nil && !errors.Is(err, storage.ErrOutOfOrderCT) {
					// Even for the first sample OOO is a common scenario because
					// we can't tell if a CT was already ingested in a previous request.
					// We ignore the error.
					s.logger.Debug("Error when appending CT in remote write request", "err", err, "series", ls.String(), "created_timestamp", ts.CreatedTimestamp, "timestamp", hp.Timestamp)
				}
			}
			if hp.IsFloatHistogram() {
				ref, err = app.AppendHistogram(ref, ls, hp.Timestamp, nil, hp.ToFloatHistogram())
			} else {
				ref, err = app.AppendHistogram(ref, ls, hp.Timestamp, hp.ToIntHistogram(), nil)
			}
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
				s.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			return 0, http.StatusInternalServerError, err
		}

		// Exemplars.
		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, req.Symbols)
			ref, err = app.AppendExemplar(ref, ls, e)
			if err == nil {
				rs.Exemplars++
				continue
			}
			// Handle append error.
			if errors.Is(err, storage.ErrOutOfOrderExemplar) {
				outOfOrderExemplarErrs++ // Maintain old metrics, but technically not needed, given we fail here.
				s.logger.Error("Out of order exemplar", "err", err.Error(), "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			// TODO(bwplotka): Add strict mode which would trigger rollback of everything if needed.
			// For now we keep the previously released flow (just error not debug leve) of dropping them without rollback and 5xx.
			s.logger.Error("failed to ingest exemplar, emitting error log, but no error for PRW caller", "err", err.Error(), "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
		}

		m := ts.ToMetadata(req.Symbols)
		if _, err = app.UpdateMetadata(ref, ls, m); err != nil {
			s.logger.Debug("error while updating metadata from remote write", "err", err)
			// Metadata is attached to each series, so since Prometheus does not reject sample without metadata information,
			// we don't report remote write error either. We increment metric instead.
			samplesWithoutMetadata += rs.AllSamples() - allSamplesSoFar
		}
	}

	if outOfOrderExemplarErrs > 0 {
		s.logger.Warn("Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}
	s.samplesWithInvalidLabelsTotal.Add(float64(samplesWithInvalidLabels))

	if len(badRequestErrs) == 0 {
		return samplesWithoutMetadata, 0, nil
	}
	// TODO(bwplotka): Better concat formatting? Perhaps add size limit?
	return samplesWithoutMetadata, http.StatusBadRequest, errors.Join(badRequestErrs...)
}

// handleHistogramZeroSample appends CT as a zero-value sample with CT value as the sample timestamp.
// It doesn't return errors in case of out of order CT.
func (s *writeStorage) handleHistogramZeroSample(app storage.Appender, ref storage.SeriesRef, l labels.Labels, hist writev2.Histogram, ct int64) (storage.SeriesRef, error) {
	var err error
	if hist.IsFloatHistogram() {
		ref, err = app.AppendHistogramCTZeroSample(ref, l, hist.Timestamp, ct, nil, hist.ToFloatHistogram())
	} else {
		ref, err = app.AppendHistogramCTZeroSample(ref, l, hist.Timestamp, ct, hist.ToIntHistogram(), nil)
	}
	return ref, err
}

type OTLPOptions struct {
	// Convert delta samples to their cumulative equivalent by aggregating in-memory
	ConvertDelta bool
}

// NewOTLPWriteHandler creates a http.Handler that accepts OTLP write requests and
// writes them to the provided appendable.
func NewOTLPWriteHandler(logger *slog.Logger, _ prometheus.Registerer, appendable storage.Appendable, configFunc func() config.Config, opts OTLPOptions) http.Handler {
	ex := &rwExporter{
		writeStorage: &writeStorage{
			logger:     logger,
			appendable: appendable,
		},
		config: configFunc,
	}

	wh := &otlpWriteHandler{logger: logger, cumul: ex}

	if opts.ConvertDelta {
		fac := deltatocumulative.NewFactory()
		set := processor.Settings{TelemetrySettings: component.TelemetrySettings{MeterProvider: noop.NewMeterProvider()}}
		d2c, err := fac.CreateMetrics(context.Background(), set, fac.CreateDefaultConfig(), wh.cumul)
		if err != nil {
			// fac.CreateMetrics directly calls [deltatocumulativeprocessor.createMetricsProcessor],
			// which only errors if:
			//   - cfg.(type) != *Config
			//   - telemetry.New fails due to bad set.TelemetrySettings
			//
			// both cannot be the case, as we pass a valid *Config and valid TelemetrySettings.
			// as such, we assume this error to never occur.
			// if it is, our assumptions are broken in which case a panic seems acceptable.
			panic(err)
		}
		if err := d2c.Start(context.Background(), nil); err != nil {
			// deltatocumulative does not error on start. see above for panic reasoning
			panic(err)
		}
		wh.delta = d2c
	}

	return wh
}

type rwExporter struct {
	*writeStorage
	config func() config.Config
}

func (rw *rwExporter) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	otlpCfg := rw.config().OTLPConfig

	converter := otlptranslator.NewPrometheusConverter()
	annots, err := converter.FromMetrics(ctx, md, otlptranslator.Settings{
		AddMetricSuffixes:                 true,
		AllowUTF8:                         otlpCfg.TranslationStrategy == config.NoUTF8EscapingWithSuffixes,
		PromoteResourceAttributes:         otlpCfg.PromoteResourceAttributes,
		KeepIdentifyingResourceAttributes: otlpCfg.KeepIdentifyingResourceAttributes,
	})
	if err != nil {
		rw.logger.Warn("Error translating OTLP metrics to Prometheus write request", "err", err)
	}
	ws, _ := annots.AsStrings("", 0, 0)
	if len(ws) > 0 {
		rw.logger.Warn("Warnings translating OTLP metrics to Prometheus write request", "warnings", ws)
	}

	err = rw.write(ctx, &prompb.WriteRequest{
		Timeseries: converter.TimeSeries(),
		Metadata:   converter.Metadata(),
	})
	return err
}

func (rw *rwExporter) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

type otlpWriteHandler struct {
	logger *slog.Logger

	cumul consumer.Metrics // only cumulative
	delta consumer.Metrics // delta capable
}

func (h *otlpWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeOTLPWriteRequest(r)
	if err != nil {
		h.logger.Error("Error decoding OTLP write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	md := req.Metrics()
	// if delta conversion enabled AND delta samples exist, use slower delta capable path
	if h.delta != nil && hasDelta(md) {
		err = h.delta.ConsumeMetrics(r.Context(), md)
	} else {
		// deltatocumulative currently holds a sync.Mutex when entering ConsumeMetrics.
		// This is slow and not necessary when no delta samples exist anyways
		err = h.cumul.ConsumeMetrics(r.Context(), md)
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

type timeLimitAppender struct {
	storage.Appender

	maxTime int64
}

func (app *timeLimitAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

func (app *timeLimitAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	ref, err := app.Appender.AppendHistogram(ref, l, t, h, fh)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

func (app *timeLimitAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if e.Ts > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	ref, err := app.Appender.AppendExemplar(ref, l, e)
	if err != nil {
		return 0, err
	}
	return ref, nil
}
