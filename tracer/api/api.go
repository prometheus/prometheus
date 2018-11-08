// Copyright 2015 The Prometheus Authors
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

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	opentracing "github.com/opentracing/opentracing-go"
	route "github.com/prometheus/common/route"
	tracer "github.com/prometheus/prometheus/tracer/client"
	"github.com/prometheus/prometheus/tracer/model"
)

const (
	// All traces are stored under the same service-name
	serviceName     = "prometheus"
	defaultLookback = 60 * 60 * 1000 // 1 hour
	queryLimit      = 10
)

func parseInt64(values url.Values, key string, def int64) (int64, error) {
	value := values.Get(key)
	if value == "" {
		return def, nil
	}

	intVal, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}

	return intVal, nil
}

// Register exposes the tracer API used by zipkin-ui
func Register(logger log.Logger, router *route.Router, opentracingTracer opentracing.Tracer) {

	modelTracer := opentracingTracer.(*model.Tracer)
	collector := modelTracer.Collector.(*tracer.Collector)

	router.Get("/traces/api/v2/dependencies", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(struct{}{}); err != nil {
			level.Error(logger).Log("msg", "Error marshalling zipkin dependencies", "err", err)
		}
	})

	router.Get("/traces/config.json", func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(struct {
			DefaultLookback int `json:"defaultLookback"`
			QueryLimit      int `json:"queryLimit"`
		}{
			DefaultLookback: defaultLookback,
			QueryLimit:      queryLimit,
		}); err != nil {
			level.Error(logger).Log("msg", "Error marshalling zipkin config.json", "err", err)
		}
	})

	router.Get("/traces/api/v2/services", func(w http.ResponseWriter, r *http.Request) {
		services := []string{serviceName}
		if err := json.NewEncoder(w).Encode(services); err != nil {
			level.Error(logger).Log("msg", "Error marshalling zipkin service names", "err", err)
		}
	})

	router.Get("/traces/api/v2/spans", func(w http.ResponseWriter, r *http.Request) {
		spanNames := collector.GetSpanNames()
		if err := json.NewEncoder(w).Encode(spanNames); err != nil {
			level.Error(logger).Log("msg", "Error marshalling zipkin span names", "err", err)
		}
	})

	router.Get("/traces/api/v2/trace", func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()

		value := values.Get("id")
		if value == "" {
			http.Error(w, "Missing Trace Id", http.StatusBadRequest)
			return
		}

		id, err := fromIDStr(value)
		if err != nil {
			http.Error(w, "Invalid Trace Id", http.StatusBadRequest)
			return
		}
		trace, err := collector.GetTrace(id)
		if err != nil {
			http.Error(w, "Trace not found in buffer", http.StatusOK)
		} else {
			if err := json.NewEncoder(w).Encode(SpansToWire(trace.Spans)); err != nil {
				level.Error(logger).Log("msg", "Error marshalling trace spans", "err", err)
			}
		}
	})

	router.Get("/traces/api/v2/traces", func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()

		serviceName := values.Get("serviceName")
		if serviceName == "" {
			serviceName = "all"
		}

		spanName := values.Get("spanName")
		if spanName == "" {
			spanName = "all"
		}

		minDurationUS, err := parseInt64(values, "minDuration", 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		maxDurationUS, err := parseInt64(values, "maxDuration", 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		limit, err := parseInt64(values, "limit", queryLimit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		nowMS := time.Now().UnixNano() / int64(time.Millisecond)
		endMS, err := parseInt64(values, "endTs", nowMS)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		end := time.Unix(endMS/1e3, (endMS%1e3)*1e6)

		lookback, err := parseInt64(values, "lookback", defaultLookback)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		start := end.Add(time.Duration(-lookback) * time.Millisecond)

		searchQuery := strings.Trim(values.Get("annotationQuery"), " ")
		tagMap, err := tracer.ParseTagsQuery(searchQuery)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		query := &tracer.Query{
			End:         end,
			Start:       start,
			Limit:       int(limit),
			ServiceName: serviceName,
			SpanName:    spanName,
			SearchQuery: tagMap,
			MinDuration: time.Duration(minDurationUS) * time.Microsecond,
			MaxDuration: time.Duration(maxDurationUS) * time.Microsecond,
		}
		traces := collector.Gather(query)

		if err := json.NewEncoder(w).Encode(TracesToWire(traces)); err != nil {
			level.Error(logger).Log("msg", "Error marshalling traces", "err", err)
		}
	})
}
