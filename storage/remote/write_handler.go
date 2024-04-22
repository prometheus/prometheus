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
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/write/v2"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type writeHandler struct {
	logger     log.Logger
	appendable storage.Appendable

	samplesWithInvalidLabelsTotal prometheus.Counter

	// PRW 2.0 definies backward compatible content negotiation across prototypes
	// and compressions.
	protoTypes                []config.RemoteWriteProtoType
	acceptHeaderValue         string
	compressions              []config.RemoteWriteCompression
	acceptEncodingHeaderValue string
}

// NewWriteHandler creates a http.Handler that accepts remote write requests and
// writes them to the provided appendable.
// Compatible with both 1.x and 2.0 spec.
func NewWriteHandler(logger log.Logger, reg prometheus.Registerer, appendable storage.Appendable, protoTypes []config.RemoteWriteProtoType, compressions []config.RemoteWriteCompression) http.Handler {
	h := &writeHandler{
		logger:                    logger,
		appendable:                appendable,
		protoTypes:                protoTypes,
		acceptHeaderValue:         config.RemoteWriteProtoTypes(protoTypes).ServerAcceptHeaderValue(),
		compressions:              compressions,
		acceptEncodingHeaderValue: config.RemoteWriteCompressions(compressions).ServerAcceptEncodingHeaderValue(),
		samplesWithInvalidLabelsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "remote_write_invalid_labels_samples_total",
			Help:      "The total number of remote write samples which contains invalid labels.",
		}),
	}
	if reg != nil {
		reg.MustRegister(h.samplesWithInvalidLabelsTotal)
	}
	return h
}

func (h *writeHandler) parseEncoding(contentEncoding string) (config.RemoteWriteCompression, error) {
	// TODO(bwplotka): TBD
	got := config.RemoteWriteCompression(contentEncoding)
	if err := got.Validate(); err != nil {
		return "", err // TODO(bwplotka): Wrap properly
	}

	// TODO(bwplotka): Build utils for this.
	for _, c := range h.compressions {
		if c == got {
			return got, nil
		}
	}
	return "", fmt.Errorf("unsupported compression, got %v, supported %v", got, h.acceptEncodingHeaderValue)
}

func (h *writeHandler) parseType(contentType string) (config.RemoteWriteProtoType, error) {
	// TODO(bwplotka): TBD
	got := config.RemoteWriteProtoType(contentType)
	if err := got.Validate(); err != nil {
		return "", err // TODO(bwplotka): Wrap properly
	}

	// TODO(bwplotka): Build utils for this.
	for _, c := range h.protoTypes {
		if c == got {
			return got, nil
		}
	}
	return "", fmt.Errorf("unsupported proto type, got %v, supported %v", got, h.acceptHeaderValue)
}

func (h *writeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Start with ensuring receiver response headers indicate PRW 2.0 and accepted
	// content type and compressions.
	w.Header().Set("Accept", h.acceptHeaderValue)
	w.Header().Set("Accept-Encoding", h.acceptEncodingHeaderValue)

	// Initial content type and encoding negotiation.
	// NOTE: PRW 2.0 deprecated X-Prometheus-Remote-Write-Version, ignore it, we
	// rely on content-type header only.
	enc, err := h.parseEncoding(r.Header.Get("Content-Encoding"))
	if err != nil {

		w.WriteHeader(http.StatusUnsupportedMediaType)
		w.Write([]byte(err.Error())) // TODO(bwplotka) Format? Log?
		return
	}
	typ, err := h.parseType(r.Header.Get("Content-Type"))
	if err != nil {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		w.Write([]byte(err.Error())) // TODO(bwplotka) Format? Log? use http.Error
		return
	}

	// Read the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var decompressed []byte
	switch enc {
	case config.RemoteWriteCompressionSnappy:
		decompressed, err = snappy.Decode(nil, body)
		if err != nil {
			level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		panic("should not happen")
	}

	switch typ {
	case config.RemoteWriteProtoTypeV1:
		var req prompb.WriteRequest
		if err := proto.Unmarshal(decompressed, &req); err != nil {
			level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.write(r.Context(), &req)
	case config.RemoteWriteProtoTypeV2:
		// 2.0 request.
		var reqMinStr writev2.WriteRequest
		if err := proto.Unmarshal(decompressed, &reqMinStr); err != nil {
			level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.writeMinStr(r.Context(), &reqMinStr)
	default:
		panic("should not happen")
	}

	switch {
	case err == nil:
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp), errors.Is(err, storage.ErrTooOldSample):
		// Indicated an out of order sample is a bad request to prevent retries.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		level.Error(h.logger).Log("msg", "Error appending remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// checkAppendExemplarError modifies the AppendExemplar's returned error based on the error cause.
func (h *writeHandler) checkAppendExemplarError(err error, e exemplar.Exemplar, outOfOrderErrs *int) error {
	unwrappedErr := errors.Unwrap(err)
	if unwrappedErr == nil {
		unwrappedErr = err
	}
	switch {
	case errors.Is(unwrappedErr, storage.ErrNotFound):
		return storage.ErrNotFound
	case errors.Is(unwrappedErr, storage.ErrOutOfOrderExemplar):
		*outOfOrderErrs++
		level.Debug(h.logger).Log("msg", "Out of order exemplar", "exemplar", fmt.Sprintf("%+v", e))
		return nil
	default:
		return err
	}
}

func (h *writeHandler) write(ctx context.Context, req *prompb.WriteRequest) (err error) {
	outOfOrderExemplarErrs := 0
	samplesWithInvalidLabels := 0

	app := h.appendable.Appender(ctx)
	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
	}()

	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		ls := labelProtosToLabels(&b, ts.Labels)
		if !ls.IsValid() {
			level.Warn(h.logger).Log("msg", "Invalid metric names or labels", "got", ls.String())
			samplesWithInvalidLabels++
			continue
		}

		err := h.appendSamples(app, ts.Samples, ls)
		if err != nil {
			return err
		}

		for _, ep := range ts.Exemplars {
			e := exemplarProtoToExemplar(&b, ep)
			h.appendExemplar(app, e, ls, &outOfOrderExemplarErrs)
		}

		err = h.appendHistograms(app, ts.Histograms, ls)
		if err != nil {
			return err
		}
	}

	if outOfOrderExemplarErrs > 0 {
		_ = level.Warn(h.logger).Log("msg", "Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}
	if samplesWithInvalidLabels > 0 {
		h.samplesWithInvalidLabelsTotal.Add(float64(samplesWithInvalidLabels))
	}

	return nil
}

func (h *writeHandler) appendExemplar(app storage.Appender, e exemplar.Exemplar, labels labels.Labels, outOfOrderExemplarErrs *int) {
	_, err := app.AppendExemplar(0, labels, e)
	err = h.checkAppendExemplarError(err, e, outOfOrderExemplarErrs)
	if err != nil {
		// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
		level.Debug(h.logger).Log("msg", "Error while adding exemplar in AddExemplar", "exemplar", fmt.Sprintf("%+v", e), "err", err)
	}
}

func (h *writeHandler) appendSamples(app storage.Appender, ss []prompb.Sample, labels labels.Labels) error {
	var ref storage.SeriesRef
	var err error
	for _, s := range ss {
		ref, err = app.Append(ref, labels, s.GetTimestamp(), s.GetValue())
		if err != nil {
			unwrappedErr := errors.Unwrap(err)
			if unwrappedErr == nil {
				unwrappedErr = err
			}
			if errors.Is(err, storage.ErrOutOfOrderSample) || errors.Is(unwrappedErr, storage.ErrOutOfBounds) || errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp) {
				level.Error(h.logger).Log("msg", "Out of order sample from remote write", "err", err.Error(), "series", labels.String(), "timestamp", s.Timestamp)
			}
			return err
		}
	}
	return nil
}

func (h *writeHandler) appendMinSamples(app storage.Appender, ss []writev2.Sample, labels labels.Labels) error {
	var ref storage.SeriesRef
	var err error
	for _, s := range ss {
		ref, err = app.Append(ref, labels, s.GetTimestamp(), s.GetValue())
		if err != nil {
			unwrappedErr := errors.Unwrap(err)
			if unwrappedErr == nil {
				unwrappedErr = err
			}
			if errors.Is(err, storage.ErrOutOfOrderSample) || errors.Is(unwrappedErr, storage.ErrOutOfBounds) || errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp) {
				level.Error(h.logger).Log("msg", "Out of order sample from remote write", "err", err.Error(), "series", labels.String(), "timestamp", s.Timestamp)
			}
			return err
		}
	}
	return nil
}

func (h *writeHandler) appendHistograms(app storage.Appender, hh []prompb.Histogram, labels labels.Labels) error {
	var err error
	for _, hp := range hh {
		if hp.IsFloatHistogram() {
			fhs := FloatHistogramProtoToFloatHistogram(hp)
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, nil, fhs)
		} else {
			hs := HistogramProtoToHistogram(hp)
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hs, nil)
		}
		if err != nil {
			unwrappedErr := errors.Unwrap(err)
			if unwrappedErr == nil {
				unwrappedErr = err
			}
			// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(unwrappedErr, storage.ErrOutOfOrderSample) || errors.Is(unwrappedErr, storage.ErrOutOfBounds) || errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp) {
				level.Error(h.logger).Log("msg", "Out of order histogram from remote write", "err", err.Error(), "series", labels.String(), "timestamp", hp.Timestamp)
			}
			return err
		}
	}
	return nil
}

func (h *writeHandler) appendMinHistograms(app storage.Appender, hh []writev2.Histogram, labels labels.Labels) error {
	var err error
	for _, hp := range hh {
		if hp.IsFloatHistogram() {
			fhs := FloatMinHistogramProtoToFloatHistogram(hp)
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, nil, fhs)
		} else {
			hs := MinHistogramProtoToHistogram(hp)
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hs, nil)
		}
		if err != nil {
			unwrappedErr := errors.Unwrap(err)
			if unwrappedErr == nil {
				unwrappedErr = err
			}
			// Although AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(unwrappedErr, storage.ErrOutOfOrderSample) || errors.Is(unwrappedErr, storage.ErrOutOfBounds) || errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp) {
				level.Error(h.logger).Log("msg", "Out of order histogram from remote write", "err", err.Error(), "series", labels.String(), "timestamp", hp.Timestamp)
			}
			return err
		}
	}
	return nil
}

// NewOTLPWriteHandler creates a http.Handler that accepts OTLP write requests and
// writes them to the provided appendable.
func NewOTLPWriteHandler(logger log.Logger, appendable storage.Appendable) http.Handler {
	rwHandler := &writeHandler{
		logger:     logger,
		appendable: appendable,
	}

	return &otlpWriteHandler{
		logger:    logger,
		rwHandler: rwHandler,
	}
}

type otlpWriteHandler struct {
	logger    log.Logger
	rwHandler *writeHandler
}

func (h *otlpWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeOTLPWriteRequest(r)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prwMetricsMap, errs := otlptranslator.FromMetrics(req.Metrics(), otlptranslator.Settings{
		AddMetricSuffixes: true,
	})
	if errs != nil {
		level.Warn(h.logger).Log("msg", "Error translating OTLP metrics to Prometheus write request", "err", errs)
	}

	prwMetrics := make([]prompb.TimeSeries, 0, len(prwMetricsMap))

	for _, ts := range prwMetricsMap {
		prwMetrics = append(prwMetrics, *ts)
	}

	err = h.rwHandler.write(r.Context(), &prompb.WriteRequest{
		Timeseries: prwMetrics,
	})

	switch {
	case err == nil:
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrOutOfBounds), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
		// Indicated an out of order sample is a bad request to prevent retries.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		level.Error(h.logger).Log("msg", "Error appending remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *writeHandler) writeMinStr(ctx context.Context, req *writev2.WriteRequest) (err error) {
	outOfOrderExemplarErrs := 0

	app := h.appendable.Appender(ctx)
	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
	}()

	for _, ts := range req.Timeseries {
		ls := labelProtosV2ToLabels(ts.LabelsRefs, req.Symbols)

		err := h.appendMinSamples(app, ts.Samples, ls)
		if err != nil {
			return err
		}

		for _, ep := range ts.Exemplars {
			e := exemplarProtoV2ToExemplar(ep, req.Symbols)
			h.appendExemplar(app, e, ls, &outOfOrderExemplarErrs)
		}

		err = h.appendMinHistograms(app, ts.Histograms, ls)
		if err != nil {
			return err
		}

		m := metadataProtoV2ToMetadata(ts.Metadata, req.Symbols)
		if _, err = app.UpdateMetadata(0, ls, m); err != nil {
			level.Debug(h.logger).Log("msg", "error while updating metadata from remote write", "err", err)
		}
	}

	if outOfOrderExemplarErrs > 0 {
		_ = level.Warn(h.logger).Log("msg", "Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}

	return nil
}
