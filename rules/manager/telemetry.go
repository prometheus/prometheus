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

package manager

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	intervalLabel     = "interval"
	ruleTypeLabel     = "rule_type"
	alertingRuleType  = "alerting"
	recordingRuleType = "recording"
)

var (
	evalDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "prometheus_rule_evaluation_duration_ms",
			Help: "The duration for a rule to execute.",
		},
		[]string{ruleTypeLabel},
	)
	iterationDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_evaluator_duration_ms",
			Help:       "The duration for each evaluation pool to execute.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{intervalLabel},
	)
)

func recordOutcome(ruleType string, duration time.Duration) {
	millisecondDuration := float64(duration / time.Millisecond)
	evalDuration.WithLabelValues(ruleType).Observe(millisecondDuration)
}

func init() {
	prometheus.MustRegister(iterationDuration)
	prometheus.MustRegister(evalDuration)
}
