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

package legacy

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/util/httputil"
)

// API manages the /api HTTP endpoint.
type API struct {
	Now         func() model.Time
	Storage     local.Storage
	QueryEngine *promql.Engine
}

// Register registers the handler for the various endpoints below /api.
func (api *API) Register(router *route.Router) {
	// List all the endpoints here instead of using a wildcard route because we
	// would otherwise handle /api/v1 as well.
	router.Options("/query", handle("options", api.Options))
	router.Options("/query_range", handle("options", api.Options))
	router.Options("/metrics", handle("options", api.Options))

	router.Get("/query", handle("query", api.Query))
	router.Get("/query_range", handle("query_range", api.QueryRange))
	router.Get("/metrics", handle("metrics", api.Metrics))
}

func handle(name string, f http.HandlerFunc) http.HandlerFunc {
	h := httputil.CompressionHandler{
		Handler: f,
	}
	return prometheus.InstrumentHandler(name, h)
}
