// Copyright 2013 Prometheus Team
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

	"github.com/prometheus/client_golang/prometheus/exp"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
	"github.com/prometheus/prometheus/web/http_utils"
)

type MetricsService struct {
	time          utility.Time
	Config        *config.Config
	TargetManager retrieval.TargetManager
	Storage       *metric.TieredStorage
}

func (msrv *MetricsService) RegisterHandler() {
	handler := func(h func(http.ResponseWriter, *http.Request)) http.Handler {
		return http_utils.CompressionHandler{
			Handler: http.HandlerFunc(h),
		}
	}
	exp.Handle("/api/query", handler(msrv.Query))
	exp.Handle("/api/query_range", handler(msrv.QueryRange))
	exp.Handle("/api/metrics", handler(msrv.Metrics))
	exp.Handle("/api/targets", handler(msrv.SetTargets))
}
