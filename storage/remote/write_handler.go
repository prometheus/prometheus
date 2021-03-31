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
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		level.Error(h.logger).Log("msg", "Out of order sample from remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		level.Error(h.logger).Log("msg", "Error appending remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *writeHandler) write(ctx context.Context, req *prompb.WriteRequest) (err error) {
	app := h.appendable.Appender(ctx)
	defer func() {
		if err != nil {
			app.Rollback()
			return
		}
		err = app.Commit()
	}()

	for _, ts := range req.Timeseries {
		labels := labelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			_, err = app.Append(0, labels, s.Timestamp, s.Value)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
