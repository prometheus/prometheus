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

package web

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
)

// PrometheusStatusHandler implements http.Handler.
type PrometheusStatusHandler struct {
	mu sync.RWMutex

	BuildInfo   map[string]string
	Config      string
	Flags       map[string]string
	RuleManager rules.RuleManager
	TargetPools map[string]*retrieval.TargetPool

	Birth time.Time
	PathPrefix string
}

// TargetStateToClass returns a map of TargetState to the name of a Bootstrap CSS class.
func (h *PrometheusStatusHandler) TargetStateToClass() map[retrieval.TargetState]string {
	return map[retrieval.TargetState]string{
		retrieval.Unknown:   "warning",
		retrieval.Healthy:   "success",
		retrieval.Unhealthy: "danger",
	}
}

func (h *PrometheusStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	executeTemplate(w, "status", h, h.PathPrefix)
}
