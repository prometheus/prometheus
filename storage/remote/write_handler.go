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
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gogo/protobuf/proto"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/storage"
)

type writeHandler struct {
	logger     *slog.Logger
	appendable storage.Appendable

	samplesWithInvalidLabelsTotal  prometheus.Counter
	samplesAppendedWithoutMetadata prometheus.Counter

	ingestSTZeroSample      bool
	enableTypeAndUnitLabels bool
	appendMetadata          bool
}

const maxAheadTime = 10 * time.Minute

// NewWriteHandler creates a http.Handler that accepts remote write requests with
// the given message in acceptedMsgs and writes them to the provided appendable.
//
// NOTE(bwplotka): When accepting v2 proto and spec, partial writes are possible
// as per https://prometheus.io/docs/specs/remote_write_spec_2_0/#partial-write.
func NewWriteHandler(logger *slog.Logger, reg prometheus.Registerer, appendable storage.Appendable, acceptedMsgs remoteapi.MessageTypes, ingestSTZeroSample, enableTypeAndUnitLabels, appendMetadata bool) http.Handler {
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

		ingestSTZeroSample:      ingestSTZeroSample,
		enableTypeAndUnitLabels: enableTypeAndUnitLabels,
		appendMetadata:          appendMetadata,
	}
	return remoteapi.NewWriteHandler(h, acceptedMsgs, remoteapi.WithWriteHandlerLogger(logger))
}

// isHistogramValidationError checks if the error is a native histogram validation error.
func isHistogramValidationError(err error) bool {
	var e histogram.Error
	return errors.As(err, &e)
}

// Store implements remoteapi.writeStorage interface.
// TODO(bwplotka): Improve remoteapi.Store API. Right now it's confusing if PRWv1 flows should use WriteResponse or not.
// If it's not filled, it will be "confirmed zero" which caused partial error reporting on client side in the past.
// Temporary fix was done to only care about WriteResponse stats for PRW2 (see https://github.com/prometheus/client_golang/pull/1927
// but better approach would be to only confirm if explicit stats were injected.
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

	app := &remoteWriteAppender{
		Appender: h.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
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
	for _, ts := range req.Timeseries {
		ls := ts.ToLabels(&b, nil)

		// TODO(bwplotka): Even as per 1.0 spec, this should be a 400 error, while other samples are
		// potentially written. Perhaps unify with fixed writeV2 implementation a bit.
		if !ls.Has(labels.MetricName) || !ls.IsValid(model.UTF8Validation) {
			h.logger.Warn("Invalid metric names or labels", "got", ls.String())
			samplesWithInvalidLabels++
			continue
		} else if duplicateLabel, hasDuplicate := ls.HasDuplicateLabelNames(); hasDuplicate {
			h.logger.Warn("Invalid labels for series.", "labels", ls.String(), "duplicated_label", duplicateLabel)
			samplesWithInvalidLabels++
			continue
		}

		if err := h.appendV1Samples(app, ts.Samples, ls); err != nil {
			return err
		}
		samplesAppended += len(ts.Samples)

		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, nil)
			if _, err := app.AppendExemplar(0, ls, e); err != nil {
				switch {
				case errors.Is(err, storage.ErrOutOfOrderExemplar):
					outOfOrderExemplarErrs++
					h.logger.Debug("Out of order exemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
				default:
					// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
					h.logger.Debug("Error while adding exemplar in AppendExemplar", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e), "err", err)
				}
			}
		}

		if err = h.appendV1Histograms(app, ts.Histograms, ls); err != nil {
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

func (h *writeHandler) appendV1Samples(app storage.Appender, ss []prompb.Sample, labels labels.Labels) error {
	var ref storage.SeriesRef
	var err error
	for _, s := range ss {
		ref, err = app.Append(ref, labels, s.GetTimestamp(), s.GetValue())
		if err != nil {
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				h.logger.Error("Out of order sample from remote write", "err", err.Error(), "series", labels.String(), "timestamp", s.Timestamp)
			}
			return err
		}
	}
	return nil
}

func (h *writeHandler) appendV1Histograms(app storage.Appender, hh []prompb.Histogram, labels labels.Labels) error {
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
				h.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", labels.String(), "timestamp", hp.Timestamp)
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
func (h *writeHandler) writeV2(ctx context.Context, req *writev2.Request) (_ remoteapi.WriteResponseStats, errHTTPCode int, _ error) {
	app := &remoteWriteAppender{
		Appender: h.appendable.Appender(ctx),
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
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

func (h *writeHandler) appendV2(app storage.Appender, req *writev2.Request, rs *remoteapi.WriteResponseStats) (samplesWithoutMetadata, errHTTPCode int, err error) {
	var (
		badRequestErrs                                   []error
		outOfOrderExemplarErrs, samplesWithInvalidLabels int

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
		if !ls.Has(labels.MetricName) || !ls.IsValid(model.UTF8Validation) {
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

		allSamplesSoFar := rs.AllSamples()
		var ref storage.SeriesRef
		for _, s := range ts.Samples {
			if h.ingestSTZeroSample && s.StartTimestamp != 0 && s.Timestamp != 0 {
				ref, err = app.AppendSTZeroSample(ref, ls, s.Timestamp, s.StartTimestamp)
				// We treat OOO errors specially as it's a common scenario given:
				// * We can't tell if ST was already ingested in a previous request.
				// * We don't check if ST changed for stream of samples (we typically have one though),
				// as it's checked in the AppendSTZeroSample reliably.
				if err != nil && !errors.Is(err, storage.ErrOutOfOrderST) {
					h.logger.Debug("Error when appending ST from remote write request", "err", err, "series", ls.String(), "start_timestamp", s.StartTimestamp, "timestamp", s.Timestamp)
				}
			}

			ref, err = app.Append(ref, ls, s.GetTimestamp(), s.GetValue())
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
			return 0, http.StatusInternalServerError, err
		}

		// Native Histograms.
		for _, hp := range ts.Histograms {
			if h.ingestSTZeroSample && hp.StartTimestamp != 0 && hp.Timestamp != 0 {
				ref, err = h.handleHistogramZeroSample(app, ref, ls, hp, hp.StartTimestamp)
				// We treat OOO errors specially as it's a common scenario given:
				// * We can't tell if ST was already ingested in a previous request.
				// * We don't check if ST changed for stream of samples (we typically have one though),
				// as it's checked in the ingestSTZeroSample reliably.
				if err != nil && !errors.Is(err, storage.ErrOutOfOrderST) {
					h.logger.Debug("Error when appending ST from remote write request", "err", err, "series", ls.String(), "start_timestamp", hp.StartTimestamp, "timestamp", hp.Timestamp)
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
				h.logger.Error("Out of order histogram from remote write", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			if isHistogramValidationError(err) {
				h.logger.Error("Invalid histogram received", "err", err.Error(), "series", ls.String(), "timestamp", hp.Timestamp)
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			return 0, http.StatusInternalServerError, err
		}

		// Exemplars.
		for _, ep := range ts.Exemplars {
			e, err := ep.ToExemplar(&b, req.Symbols)
			if err != nil {
				badRequestErrs = append(badRequestErrs, fmt.Errorf("parsing exemplar for series %v: %w", ls.String(), err))
				continue
			}
			ref, err = app.AppendExemplar(ref, ls, e)
			if err == nil {
				rs.Exemplars++
				continue
			}
			// Handle append error.
			if errors.Is(err, storage.ErrOutOfOrderExemplar) {
				outOfOrderExemplarErrs++ // Maintain old metrics, but technically not needed, given we fail here.
				h.logger.Error("Out of order exemplar", "err", err.Error(), "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
				badRequestErrs = append(badRequestErrs, fmt.Errorf("%w for series %v", err, ls.String()))
				continue
			}
			// TODO(bwplotka): Add strict mode which would trigger rollback of everything if needed.
			// For now we keep the previously released flow (just error not debug leve) of dropping them without rollback and 5xx.
			h.logger.Error("failed to ingest exemplar, emitting error log, but no error for PRW caller", "err", err.Error(), "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
		}

		// Only update metadata in WAL if the metadata-wal-records feature is enabled.
		// Without this feature, metadata is not persisted to WAL.
		if h.appendMetadata {
			if _, err = app.UpdateMetadata(ref, ls, m); err != nil {
				h.logger.Debug("error while updating metadata from remote write", "err", err)
				// Metadata is attached to each series, so since Prometheus does not reject sample without metadata information,
				// we don't report remote write error either. We increment metric instead.
				samplesWithoutMetadata += rs.AllSamples() - allSamplesSoFar
			}
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

// TODO(bwplotka): Consider exposing timeLimitAppender and bucketLimitAppender appenders from scrape/target.go
// to DRY, they do the same.
type remoteWriteAppender struct {
	storage.Appender

	maxTime int64
}

func (app *remoteWriteAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

func (app *remoteWriteAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	var err error
	if t > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	if h != nil && histogram.IsExponentialSchemaReserved(h.Schema) && h.Schema > histogram.ExponentialSchemaMax {
		if err = h.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
			return 0, err
		}
	}
	if fh != nil && histogram.IsExponentialSchemaReserved(fh.Schema) && fh.Schema > histogram.ExponentialSchemaMax {
		if err = fh.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
			return 0, err
		}
	}

	if ref, err = app.Appender.AppendHistogram(ref, l, t, h, fh); err != nil {
		return 0, err
	}
	return ref, nil
}

func (app *remoteWriteAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if e.Ts > app.maxTime {
		return 0, fmt.Errorf("%w: timestamp is too far in the future", storage.ErrOutOfBounds)
	}

	ref, err := app.Appender.AppendExemplar(ref, l, e)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

type remoteWriteAppenderV2 struct {
	storage.AppenderV2

	maxTime int64
}

func (app *remoteWriteAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
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
