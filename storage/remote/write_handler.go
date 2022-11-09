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

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

type writeHandler struct {
	logger     log.Logger
	appendable storage.Appendable
}

// NewWriteHandler creates a http.Handler that accepts remote write requests and
// writes them to the provided appendable.
func NewWriteHandler(logger log.Logger, appendable storage.Appendable) http.Handler {
	return &writeHandler{
		logger:     logger,
		appendable: appendable,
	}
}

func (h *writeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeWriteRequest(r.Body)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.write(r.Context(), req)
	switch err {
	case nil:
	case storage.ErrOutOfOrderSample, storage.ErrOutOfBounds, storage.ErrDuplicateSampleForTimestamp:
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
	unwrapedErr := errors.Unwrap(err)
	switch {
	case errors.Is(unwrapedErr, storage.ErrNotFound):
		return storage.ErrNotFound
	case errors.Is(unwrapedErr, storage.ErrOutOfOrderExemplar):
		*outOfOrderErrs++
		level.Debug(h.logger).Log("msg", "Out of order exemplar", "exemplar", fmt.Sprintf("%+v", e))
		return nil
	default:
		return err
	}
}

func (h *writeHandler) write(ctx context.Context, req *prompb.WriteRequest) (err error) {
	outOfOrderExemplarErrs := 0

	app := h.appendable.Appender(ctx)
	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
	}()

	var exemplarErr error
	for _, ts := range req.Timeseries {
		labels := labelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			_, err = app.Append(0, labels, s.Timestamp, s.Value)
			if err != nil {
				unwrapedErr := errors.Unwrap(err)
				if errors.Is(unwrapedErr, storage.ErrOutOfOrderSample) || errors.Is(unwrapedErr, storage.ErrOutOfBounds) || errors.Is(unwrapedErr, storage.ErrDuplicateSampleForTimestamp) {
					level.Error(h.logger).Log("msg", "Out of order sample from remote write", "err", err.Error(), "series", labels.String(), "timestamp", s.Timestamp)
				}
				return err
			}

		}

		for _, ep := range ts.Exemplars {
			e := exemplarProtoToExemplar(ep)

			_, exemplarErr = app.AppendExemplar(0, labels, e)
			exemplarErr = h.checkAppendExemplarError(exemplarErr, e, &outOfOrderExemplarErrs)
			if exemplarErr != nil {
				// Since exemplar storage is still experimental, we don't fail the request on ingestion errors.
				level.Debug(h.logger).Log("msg", "Error while adding exemplar in AddExemplar", "exemplar", fmt.Sprintf("%+v", e), "err", exemplarErr)
			}
		}

		for _, hp := range ts.Histograms {
			hs := HistogramProtoToHistogram(hp)
			_, err = app.AppendHistogram(0, labels, hp.Timestamp, hs)
			if err != nil {
				unwrappedErr := errors.Unwrap(err)
				// Althogh AppendHistogram does not currently return ErrDuplicateSampleForTimestamp there is
				// a note indicating its inclusion in the future.
				if errors.Is(unwrappedErr, storage.ErrOutOfOrderSample) || errors.Is(unwrappedErr, storage.ErrOutOfBounds) || errors.Is(unwrappedErr, storage.ErrDuplicateSampleForTimestamp) {
					level.Error(h.logger).Log("msg", "Out of order histogram from remote write", "err", err.Error(), "series", labels.String(), "timestamp", hp.Timestamp)
				}
				return err
			}
		}
	}

	if outOfOrderExemplarErrs > 0 {
		_ = level.Warn(h.logger).Log("msg", "Error on ingesting out-of-order exemplars", "num_dropped", outOfOrderExemplarErrs)
	}

	return nil
}
