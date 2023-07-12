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
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/gate"
)

type readHandler struct {
	logger                    log.Logger
	queryable                 storage.SampleAndChunkQueryable
	config                    func() config.Config
	remoteReadSampleLimit     int
	remoteReadMaxBytesInFrame int
	remoteReadGate            *gate.Gate
	queries                   prometheus.Gauge
	marshalPool               *sync.Pool
}

// NewReadHandler creates a http.Handler that accepts remote read requests and
// writes them to the provided queryable.
func NewReadHandler(logger log.Logger, r prometheus.Registerer, queryable storage.SampleAndChunkQueryable, config func() config.Config, remoteReadSampleLimit, remoteReadConcurrencyLimit, remoteReadMaxBytesInFrame int) http.Handler {
	h := &readHandler{
		logger:                    logger,
		queryable:                 queryable,
		config:                    config,
		remoteReadSampleLimit:     remoteReadSampleLimit,
		remoteReadGate:            gate.New(remoteReadConcurrencyLimit),
		remoteReadMaxBytesInFrame: remoteReadMaxBytesInFrame,
		marshalPool:               &sync.Pool{},

		queries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "prometheus",
			Subsystem: "api", // TODO: changes to storage in Prometheus 3.0.
			Name:      "remote_read_queries",
			Help:      "The current number of remote read queries being executed or waiting.",
		}),
	}
	if r != nil {
		r.MustRegister(h.queries)
	}
	return h
}

func (h *readHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.remoteReadGate.Start(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.queries.Inc()

	defer h.remoteReadGate.Done()
	defer h.queries.Dec()

	req, err := DecodeReadRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	externalLabels := h.config().GlobalConfig.ExternalLabels.Map()

	sortedExternalLabels := make([]prompb.Label, 0, len(externalLabels))
	for name, value := range externalLabels {
		sortedExternalLabels = append(sortedExternalLabels, prompb.Label{
			Name:  name,
			Value: value,
		})
	}
	slices.SortFunc(sortedExternalLabels, func(a, b prompb.Label) bool {
		return a.Name < b.Name
	})

	responseType, err := NegotiateResponseType(req.AcceptedResponseTypes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch responseType {
	case prompb.ReadRequest_STREAMED_XOR_CHUNKS:
		h.remoteReadStreamedXORChunks(ctx, w, req, externalLabels, sortedExternalLabels)
	default:
		// On empty or unknown types in req.AcceptedResponseTypes we default to non streamed, raw samples response.
		h.remoteReadSamples(ctx, w, req, externalLabels, sortedExternalLabels)
	}
}

func (h *readHandler) remoteReadSamples(
	ctx context.Context,
	w http.ResponseWriter,
	req *prompb.ReadRequest,
	externalLabels map[string]string,
	sortedExternalLabels []prompb.Label,
) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")

	resp := prompb.ReadResponse{
		Results: make([]*prompb.QueryResult, len(req.Queries)),
	}
	for i, query := range req.Queries {
		if err := func() error {
			filteredMatchers, err := filterExtLabelsFromMatchers(query.Matchers, externalLabels)
			if err != nil {
				return err
			}

			querier, err := h.queryable.Querier(ctx, query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				return err
			}
			defer func() {
				if err := querier.Close(); err != nil {
					level.Warn(h.logger).Log("msg", "Error on querier close", "err", err.Error())
				}
			}()

			var hints *storage.SelectHints
			if query.Hints != nil {
				hints = &storage.SelectHints{
					Start:    query.Hints.StartMs,
					End:      query.Hints.EndMs,
					Step:     query.Hints.StepMs,
					Func:     query.Hints.Func,
					Grouping: query.Hints.Grouping,
					Range:    query.Hints.RangeMs,
					By:       query.Hints.By,
				}
			}

			var ws storage.Warnings
			resp.Results[i], ws, err = ToQueryResult(querier.Select(false, hints, filteredMatchers...), h.remoteReadSampleLimit)
			if err != nil {
				return err
			}
			for _, w := range ws {
				level.Warn(h.logger).Log("msg", "Warnings on remote read query", "err", w.Error())
			}
			for _, ts := range resp.Results[i].Timeseries {
				ts.Labels = MergeLabels(ts.Labels, sortedExternalLabels)
			}
			return nil
		}(); err != nil {
			if httpErr, ok := err.(HTTPError); ok {
				http.Error(w, httpErr.Error(), httpErr.Status())
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := EncodeReadResponse(&resp, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *readHandler) remoteReadStreamedXORChunks(ctx context.Context, w http.ResponseWriter, req *prompb.ReadRequest, externalLabels map[string]string, sortedExternalLabels []prompb.Label) {
	w.Header().Set("Content-Type", "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse")

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "internal http.ResponseWriter does not implement http.Flusher interface", http.StatusInternalServerError)
		return
	}

	for i, query := range req.Queries {
		if err := func() error {
			filteredMatchers, err := filterExtLabelsFromMatchers(query.Matchers, externalLabels)
			if err != nil {
				return err
			}

			querier, err := h.queryable.ChunkQuerier(ctx, query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				return err
			}
			defer func() {
				if err := querier.Close(); err != nil {
					level.Warn(h.logger).Log("msg", "Error on chunk querier close", "err", err.Error())
				}
			}()

			var hints *storage.SelectHints
			if query.Hints != nil {
				hints = &storage.SelectHints{
					Start:    query.Hints.StartMs,
					End:      query.Hints.EndMs,
					Step:     query.Hints.StepMs,
					Func:     query.Hints.Func,
					Grouping: query.Hints.Grouping,
					Range:    query.Hints.RangeMs,
					By:       query.Hints.By,
				}
			}

			ws, err := StreamChunkedReadResponses(
				NewChunkedWriter(w, f),
				int64(i),
				// The streaming API has to provide the series sorted.
				querier.Select(true, hints, filteredMatchers...),
				sortedExternalLabels,
				h.remoteReadMaxBytesInFrame,
				h.marshalPool,
			)
			if err != nil {
				return err
			}

			for _, w := range ws {
				level.Warn(h.logger).Log("msg", "Warnings on chunked remote read query", "warnings", w.Error())
			}
			return nil
		}(); err != nil {
			if httpErr, ok := err.(HTTPError); ok {
				http.Error(w, httpErr.Error(), httpErr.Status())
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// filterExtLabelsFromMatchers change equality matchers which match external labels
// to a matcher that looks for an empty label,
// as that label should not be present in the storage.
func filterExtLabelsFromMatchers(pbMatchers []*prompb.LabelMatcher, externalLabels map[string]string) ([]*labels.Matcher, error) {
	matchers, err := FromLabelMatchers(pbMatchers)
	if err != nil {
		return nil, err
	}

	filteredMatchers := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		value := externalLabels[m.Name]
		if m.Type == labels.MatchEqual && value == m.Value {
			matcher, err := labels.NewMatcher(labels.MatchEqual, m.Name, "")
			if err != nil {
				return nil, err
			}
			filteredMatchers = append(filteredMatchers, matcher)
		} else {
			filteredMatchers = append(filteredMatchers, m)
		}
	}

	return filteredMatchers, nil
}
