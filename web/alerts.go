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
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/manager"
	"net/http"
	"sort"
	"sync"
)

type AlertStatus struct {
	AlertingRules        []*rules.AlertingRule
	AlertStateToRowClass map[rules.AlertState]string
}

type AlertsHandler struct {
	RuleManager manager.RuleManager

	mutex sync.Mutex
}

type byAlertStateSorter struct {
	alerts []*rules.AlertingRule
}

func (s byAlertStateSorter) Len() int {
	return len(s.alerts)
}

func (s byAlertStateSorter) Less(i, j int) bool {
	return s.alerts[i].State() > s.alerts[j].State()
}

func (s byAlertStateSorter) Swap(i, j int) {
	s.alerts[i], s.alerts[j] = s.alerts[j], s.alerts[i]
}

func (h *AlertsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	alerts := h.RuleManager.AlertingRules()
	alertsSorter := byAlertStateSorter{alerts: alerts}
	sort.Sort(alertsSorter)

	alertStatus := AlertStatus{
		AlertingRules: alertsSorter.alerts,
		AlertStateToRowClass: map[rules.AlertState]string{
			rules.INACTIVE: "success",
			rules.PENDING:  "warning",
			rules.FIRING:   "error",
		},
	}
	executeTemplate(w, "alerts", alertStatus)
}
