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
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage/metric"
)

type PrometheusStatusHandler struct {
	mu sync.RWMutex

	BuildInfo   map[string]string
	Config      string
	Curation    metric.CurationState
	Flags       map[string]string
	RuleManager rules.RuleManager
	TargetPools map[string]*retrieval.TargetPool

	Birth time.Time
}

func (s *PrometheusStatus) UpdateCurationState(c *metric.CurationState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Curation = *c
}

func (h *PrometheusStatusHandler) UpdateCurationState(c *metric.CurationState) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.Curation = *c
}

func (h *PrometheusStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	executeTemplate(w, "status", h)
}
