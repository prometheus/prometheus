// Copyright 2013 The Prometheus Authors
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
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/util/httputil"
)

// MetricsService manages the /api HTTP endpoint.
type MetricsService struct {
	Now         func() clientmodel.Timestamp
	Storage     local.Storage
	QueryEngine *promql.Engine
}

// RegisterHandler registers the handler for the various endpoints below /api.
func (msrv *MetricsService) RegisterHandler(pathPrefix string) {
	handler := func(h func(http.ResponseWriter, *http.Request)) http.Handler {
		return httputil.CompressionHandler{
			Handler: http.HandlerFunc(h),
		}
	}
	http.Handle(pathPrefix+"api/query", prometheus.InstrumentHandler(
		pathPrefix+"api/query", handler(msrv.Query),
	))
	http.Handle(pathPrefix+"api/query_range", prometheus.InstrumentHandler(
		pathPrefix+"api/query_range", handler(msrv.QueryRange),
	))
	http.Handle(pathPrefix+"api/metrics", prometheus.InstrumentHandler(
		pathPrefix+"api/metrics", handler(msrv.Metrics),
	))
}
