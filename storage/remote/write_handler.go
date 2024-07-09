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
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	otlptranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type writeHandler struct {
	logger     log.Logger
	appendable storage.Appendable

	samplesWithInvalidLabelsTotal prometheus.Counter

	acceptedProtoMsgs map[config.RemoteWriteProtoMsg]struct{}
}

const maxAheadTime = 10 * time.Minute

// NewWriteHandler creates a http.Handler that accepts remote write requests with
// the given message in acceptedProtoMsgs and writes them to the provided appendable.
func NewWriteHandler(logger log.Logger, reg prometheus.Registerer, appendable storage.Appendable, acceptedProtoMsgs []config.RemoteWriteProtoMsg) http.Handler {
	protoMsgs := map[config.RemoteWriteProtoMsg]struct{}{}
	for _, acc := range acceptedProtoMsgs {
		protoMsgs[acc] = struct{}{}
	}
	h := &writeHandler{
		logger:            logger,
		appendable:        appendable,
		acceptedProtoMsgs: protoMsgs,
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

func (h *writeHandler) parseProtoMsg(contentType string) (config.RemoteWriteProtoMsg, error) {
	contentType = strings.TrimSpace(contentType)

	parts := strings.Split(contentType, ";")
	if parts[0] != appProtoContentType {
		return "", fmt.Errorf("expected %v as the first (media) part, got %v content-type", appProtoContentType, contentType)
	}
	// Parse potential https://www.rfc-editor.org/rfc/rfc9110#parameter
	for _, p := range parts[1:] {
		pair := strings.Split(p, "=")
		if len(pair) != 2 {
			return "", fmt.Errorf("as per https://www.rfc-editor.org/rfc/rfc9110#parameter expected parameters to be key-values, got %v in %v content-type", p, contentType)
		}
		if pair[0] == "proto" {
			ret := config.RemoteWriteProtoMsg(pair[1])
			if err := ret.Validate(); err != nil {
				return "", fmt.Errorf("got %v content type; %w", contentType, err)
			}
			return ret, nil
		}
	}
	// No "proto=" parameter, assuming v1.
	return config.RemoteWriteProtoMsgV1, nil
}

func (h *writeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		// Don't break yolo 1.0 clients if not needed. This is similar to what we did
		// before 2.0: https://github.com/prometheus/prometheus/blob/d78253319daa62c8f28ed47e40bafcad2dd8b586/storage/remote/write_handler.go#L62
		// We could give http.StatusUnsupportedMediaType, but let's assume 1.0 message by default.
		contentType = appProtoContentType
	}

	msg, err := h.parseProtoMsg(contentType)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err)
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}

	if _, ok := h.acceptedProtoMsgs[msg]; !ok {
		err := fmt.Errorf("%v protobuf message is not accepted by this server; accepted %v", msg, func() (ret []string) {
			for k := range h.acceptedProtoMsgs {
				ret = append(ret, string(k))
			}
			return ret
		}())
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err)
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	}

	enc := r.Header.Get("Content-Encoding")
	if enc == "" {
		// Don't break yolo 1.0 clients if not needed. This is similar to what we did
		// before 2.0: https://github.com/prometheus/prometheus/blob/d78253319daa62c8f28ed47e40bafcad2dd8b586/storage/remote/write_handler.go#L62
		// We could give http.StatusUnsupportedMediaType, but let's assume snappy by default.
	} else if enc != string(SnappyBlockCompression) {
		err := fmt.Errorf("%v encoding (compression) is not accepted by this server; only %v is acceptable", enc, SnappyBlockCompression)
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err)
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	}

	// Read the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	decompressed, err := snappy.Decode(nil, body)
	if err != nil {
		// TODO(bwplotka): Add more context to responded error?
		level.Error(h.logger).Log("msg", "Error decompressing remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Now we have a decompressed buffer we can unmarshal it.
	switch msg {
	case config.RemoteWriteProtoMsgV1:
		var req prompb.WriteRequest
		if err := proto.Unmarshal(decompressed, &req); err != nil {
			// TODO(bwplotka): Add more context to responded error?
			level.Error(h.logger).Log("msg", "Error decoding v1 remote write request", "protobuf_message", msg, "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.write(r.Context(), &req)
	case config.RemoteWriteProtoMsgV2:
		var req writev2.Request
		if err := proto.Unmarshal(decompressed, &req); err != nil {
			// TODO(bwplotka): Add more context to responded error?
			level.Error(h.logger).Log("msg", "Error decoding v2 remote write request", "protobuf_message", msg, "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = h.writeV2(r.Context(), &req)
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

	timeLimitApp := &timeLimitAppender{
		Appender: h.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	defer func() {
		if err != nil {
			_ = timeLimitApp.Rollback()
			return
		}
		err = timeLimitApp.Commit()
	}()

	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, nil)
		if !ls.IsValid() {
			level.Warn(h.logger).Log("msg", "Invalid metric names or labels", "got", ls.String())
			samplesWithInvalidLabels++
			continue
		}

		err := h.appendSamples(timeLimitApp, ts.Samples, ls)
		if err != nil {
			return err
		}

		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, nil)
			h.appendExemplar(timeLimitApp, e, ls, &outOfOrderExemplarErrs)
		}

		err = h.appendHistograms(timeLimitApp, ts.Histograms, ls)
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

func (h *writeHandler) writeV2(ctx context.Context, req *writev2.Request) (err error) {
	outOfOrderExemplarErrs := 0

	timeLimitApp := &timeLimitAppender{
		Appender: h.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	defer func() {
		if err != nil {
			_ = timeLimitApp.Rollback()
			return
		}
		err = timeLimitApp.Commit()
	}()

	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, req.Symbols)

		err := h.appendSamplesV2(timeLimitApp, ts.Samples, ls)
		if err != nil {
			return err
		}

		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, req.Symbols)
			h.appendExemplar(timeLimitApp, e, ls, &outOfOrderExemplarErrs)
		}

		err = h.appendHistogramsV2(timeLimitApp, ts.Histograms, ls)
		if err != nil {
			return err
		}

		m := ts.ToMetadata(req.Symbols)
		if _, err = timeLimitApp.UpdateMetadata(0, ls, m); err != nil {
			level.Debug(h.logger).Log("msg", "error while updating metadata from remote write", "err", err)
		}
	}

	if outOfOrderExemplarErrs > 0 {
		_ = level.Warn(h.logger).Log("msg", "Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
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

func (h *writeHandler) appendSamplesV2(app storage.Appender, ss []writev2.Sample, labels labels.Labels) error {
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
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, nil, hp.ToFloatHistogram())
		} else {
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hp.ToIntHistogram(), nil)
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

func (h *writeHandler) appendHistogramsV2(app storage.Appender, hh []writev2.Histogram, labels labels.Labels) error {
	var err error
	for _, hp := range hh {
		if hp.IsFloatHistogram() {
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, nil, hp.ToFloatHistogram())
		} else {
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hp.ToIntHistogram(), nil)
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

	converter := otlptranslator.NewPrometheusConverter()
	if err := converter.FromMetrics(req.Metrics(), otlptranslator.Settings{
		AddMetricSuffixes: true,
	}); err != nil {
		level.Warn(h.logger).Log("msg", "Error translating OTLP metrics to Prometheus write request", "err", err)
	}

	err = h.rwHandler.write(r.Context(), &prompb.WriteRequest{
		Timeseries: converter.TimeSeries(),
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
