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
	"net/http"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	writev2 "github.com/prometheus/prometheus/prompb/write/v2"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

const (
	RemoteWriteVersionHeader        = "X-Prometheus-Remote-Write-Version"
	RemoteWriteVersion1HeaderValue  = "0.1.0"
	RemoteWriteVersion20HeaderValue = "2.0"
)

func RemoteWriteHeaderNameValues(rwFormat RemoteWriteFormat) map[string]string {
	// Return the correct remote write header name/values based on provided rwFormat
	ret := make(map[string]string, 1)

	switch rwFormat {
	case Version1:
		ret[RemoteWriteVersionHeader] = RemoteWriteVersion1HeaderValue
	case Version2:
		// We need to add the supported protocol definitions in order:
		tuples := make([]string, 0, 2)
		// Add 2.0;snappy;
		tuples = append(tuples, RemoteWriteVersion20HeaderValue+";snappy;")
		// Add default 0.1.0
		tuples = append(tuples, RemoteWriteVersion1HeaderValue)
		ret[RemoteWriteVersionHeader] = strings.Join(tuples, ",")
	}
	return ret
}

type writeHeadHandler struct {
	logger log.Logger

	remoteWrite20HeadRequests prometheus.Counter

	// Experimental feature, new remote write proto format
	// The handler will accept the new format, but it can still accept the old one
	rwFormat RemoteWriteFormat
}

func NewWriteHeadHandler(logger log.Logger, reg prometheus.Registerer, rwFormat RemoteWriteFormat) http.Handler {
	h := &writeHeadHandler{
		logger:   logger,
		rwFormat: rwFormat,
		remoteWrite20HeadRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "remote_write_20_head_requests",
			Help:      "The number of remote write 2.0 head requests.",
		}),
	}
	if reg != nil {
		reg.MustRegister(h.remoteWrite20HeadRequests)
	}
	return h
}

func (h *writeHeadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Send a response to the HEAD request based on the format supported

	// Add appropriate header values for the specific rwFormat
	for hName, hValue := range RemoteWriteHeaderNameValues(h.rwFormat) {
		w.Header().Set(hName, hValue)
	}

	w.WriteHeader(http.StatusOK)
}

type writeHandler struct {
	logger     log.Logger
	appendable storage.Appendable

	samplesWithInvalidLabelsTotal prometheus.Counter

	// Experimental feature, new remote write proto format
	// The handler will accept the new format, but it can still accept the old one
	rwFormat RemoteWriteFormat
}

// NewWriteHandler creates a http.Handler that accepts remote write requests and
// writes them to the provided appendable.
func NewWriteHandler(logger log.Logger, reg prometheus.Registerer, appendable storage.Appendable, rwFormat RemoteWriteFormat) http.Handler {
	h := &writeHandler{
		logger:     logger,
		appendable: appendable,
		rwFormat:   rwFormat,
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

func (h *writeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	var req *prompb.WriteRequest
	var reqMinStr *writev2.WriteRequest

	// Set the header(s) in the response based on the rwFormat the server supports
	for hName, hValue := range RemoteWriteHeaderNameValues(h.rwFormat) {
		w.Header().Set(hName, hValue)
	}

	// Parse the headers to work out how to handle this
	contentEncoding := r.Header.Get("Content-Encoding")
	protoVer := r.Header.Get(RemoteWriteVersionHeader)

	if protoVer == "" {
		// No header provided, assume 0.1.0 as everything that relies on later
		// features MUST supply the correct headers
		protoVer = RemoteWriteVersion1HeaderValue
	} else if protoVer == RemoteWriteVersion20HeaderValue {
		// This is a 2.0 request, woo
	} else {
		// We have a version in the header but it is not one we recognise
		// TODO - make a proper error for this
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", "Unknown remote write version in headers", "ver", protoVer)
		http.Error(w, "Unknown remote write version in headers", http.StatusBadRequest)
		return
	}

	// At this point we are happy with the version but need to check the encoding
	if protoVer == RemoteWriteVersion1HeaderValue {
		// If the version is 0.1.0 then we automatically assume Snappy encoding
		// so we check that it is "snappy" if specified or unspecified
		if contentEncoding != "" && contentEncoding != "snappy" {
			level.Error(h.logger).Log("msg", "Error determining remote write request encoding", "contentEncoding", contentEncoding)
			http.Error(w, "Error determining remote write encoding", http.StatusBadRequest)
			return
		}
		req, err = DecodeWriteRequest(r.Body)
		if err != nil {
			level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.write(r.Context(), req)
	} else {
		// 2.0 request
		// MUST be snappy encoded
		if contentEncoding != "snappy" {
			level.Error(h.logger).Log("msg", "Error determining remote write request encoding", "contentEncoding", contentEncoding)
			http.Error(w, "Error determining remote write encoding", http.StatusNotAcceptable)
			return
		}
		reqMinStr, err = DecodeMinimizedWriteRequestStr(r.Body)
		if err != nil {
			level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.writeMinStr(r.Context(), reqMinStr)
	}

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

	w.WriteHeader(http.StatusNoContent)
}

// checkAppendExemplarError modifies the AppendExamplar's returned error based on the error cause.
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

	for _, ts := range req.Timeseries {
		ls := labelProtosToLabels(ts.Labels)
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
			e := exemplarProtoToExemplar(ep)
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
