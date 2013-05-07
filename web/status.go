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

package web

import (
	"flag"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage/metric"
	"net/http"
	"sync"
)

type PrometheusStatus struct {
	BuildInfo   map[string]string
	Config      string
	Curation    metric.CurationState
	Flags       map[string]string
	Rules       string
	TargetPools map[string]*retrieval.TargetPool
}

type StatusHandler struct {
	sync.Mutex
	BuildInfo        map[string]string
	Config           *config.Config
	CurationState    chan metric.CurationState
	PrometheusStatus *PrometheusStatus
	TargetManager    retrieval.TargetManager
}

func (h *StatusHandler) ServeRequestsForever() {
	flags := map[string]string{}

	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	h.PrometheusStatus = &PrometheusStatus{
		BuildInfo: h.BuildInfo,
		Config:    h.Config.String(),
		Flags:     flags,
		Rules:     "TODO: list rules here",
		// BUG: race condition, concurrent map access
		TargetPools: h.TargetManager.Pools(),
	}

	for state := range h.CurationState {
		h.Lock()
		h.PrometheusStatus.Curation = state
		h.Unlock()
	}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	executeTemplate(w, "status", h.PrometheusStatus)
}
