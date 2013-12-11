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

package rules

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	intervalKey       = "interval"
	ruleTypeLabel     = "rule_type"
	alertingRuleType  = "alerting"
	recordingRuleType = "recording"
)

var (
	evalDuration  = prometheus.NewDefaultHistogram()
	ruleCount     = prometheus.NewCounter()
	evalDurations = prometheus.NewHistogram(&prometheus.HistogramSpecification{
		Starts:                prometheus.LogarithmicSizedBucketsFor(0, 10000),
		BucketBuilder:         prometheus.AccumulatingBucketBuilder(prometheus.EvictAndReplaceWith(10, prometheus.AverageReducer), 100),
		ReportablePercentiles: []float64{0.01, 0.05, 0.5, 0.90, 0.99}})
)

func init() {
	prometheus.Register("prometheus_evaluator_duration_ms", "The duration for each evaluation pool to execute.", prometheus.NilLabels, evalDurations)
	prometheus.Register("prometheus_rule_evaluation_duration_ms", "The duration for a rule to execute.", prometheus.NilLabels, evalDuration)
	prometheus.Register("prometheus_rule_evaluation_count", "The number of rules evaluated.", prometheus.NilLabels, ruleCount)
}
