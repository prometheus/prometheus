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
	"github.com/prometheus/prometheus/appstate"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage/metric"
	"net/http"
)

type PrometheusStatus struct {
	Config      string
	Rules       string
	Status      string
	TargetPools map[string]*retrieval.TargetPool
	BuildInfo   map[string]string
	Flags       map[string]string
	Curation    metric.CurationState
}

type StatusHandler struct {
	appState         *appstate.ApplicationState
	PrometheusStatus *PrometheusStatus
}

func (h *StatusHandler) Run() {
	flags := map[string]string{}

	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	h.PrometheusStatus = &PrometheusStatus{
		Config:      h.appState.Config.String(),
		Rules:       "TODO: list rules here",
		Status:      "TODO: add status information here",
		TargetPools: h.appState.TargetManager.Pools(),
		BuildInfo:   h.appState.BuildInfo,
		Flags:       flags,
	}

	// Law of Demeter :-(
	for state := range h.appState.CurationState {
		h.PrometheusStatus.Curation = state
	}
}

func (h StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	executeTemplate(w, "status", h.PrometheusStatus)
}
