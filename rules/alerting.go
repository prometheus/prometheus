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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/utility"
	"time"
)

// States that active alerts can be in.
type alertState int

func (s alertState) String() (state string) {
	switch s {
	case PENDING:
		state = "pending"
	case FIRING:
		state = "firing"
	}
	return
}

const (
	PENDING alertState = iota
	FIRING
)

// alert is used to track active (pending/firing) alerts over time.
type alert struct {
	// The name of the alert.
	name string
	// The vector element labelset triggering this alert.
	metric model.Metric
	// The state of the alert (PENDING or FIRING).
	state alertState
	// The time when the alert first transitioned into PENDING state.
	activeSince time.Time
}

// sample returns a Sample suitable for recording the alert.
func (a alert) sample(timestamp time.Time, value model.SampleValue) model.Sample {
	recordedMetric := model.Metric{}
	for label, value := range a.metric {
		recordedMetric[label] = value
	}

	recordedMetric[model.MetricNameLabel] = model.AlertMetricName
	recordedMetric[model.AlertNameLabel] = model.LabelValue(a.name)
	recordedMetric[model.AlertStateLabel] = model.LabelValue(a.state.String())

	return model.Sample{
		Metric:    recordedMetric,
		Value:     value,
		Timestamp: timestamp,
	}
}

// An alerting rule generates alerts from its vector expression.
type AlertingRule struct {
	// The name of the alert.
	name string
	// The vector expression from which to generate alerts.
	vector ast.VectorNode
	// The duration for which a labelset needs to persist in the expression
	// output vector before an alert transitions from PENDING to FIRING state.
	holdDuration time.Duration
	// Extra labels to attach to the resulting alert sample vectors.
	labels model.LabelSet
	// A map of alerts which are currently active (PENDING or FIRING), keyed by
	// the fingerprint of the labelset they correspond to.
	activeAlerts map[model.Fingerprint]*alert
}

func (rule AlertingRule) Name() string { return rule.name }

func (rule AlertingRule) EvalRaw(timestamp time.Time) (vector ast.Vector, err error) {
	return ast.EvalVectorInstant(rule.vector, timestamp)
}

func (rule AlertingRule) Eval(timestamp time.Time) (vector ast.Vector, err error) {
	// Get the raw value of the rule expression.
	exprResult, err := rule.EvalRaw(timestamp)
	if err != nil {
		return
	}

	// Create pending alerts for any new vector elements in the alert expression.
	resultFingerprints := utility.Set{}
	for _, sample := range exprResult {
		fp := model.NewFingerprintFromMetric(sample.Metric)
		resultFingerprints.Add(fp)

		if _, ok := rule.activeAlerts[fp]; !ok {
			rule.activeAlerts[fp] = &alert{
				name:        rule.name,
				metric:      sample.Metric,
				state:       PENDING,
				activeSince: timestamp,
			}
		}
	}

	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, activeAlert := range rule.activeAlerts {
		if !resultFingerprints.Has(fp) {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			delete(rule.activeAlerts, fp)
			continue
		}

		if activeAlert.state == PENDING && timestamp.Sub(activeAlert.activeSince) >= rule.holdDuration {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			activeAlert.state = FIRING
		}

		vector = append(vector, activeAlert.sample(timestamp, 1))
	}
	return
}

// Construct a new AlertingRule.
func NewAlertingRule(name string, vector ast.VectorNode, holdDuration time.Duration, labels model.LabelSet) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vector,
		holdDuration: holdDuration,
		labels:       labels,
		activeAlerts: map[model.Fingerprint]*alert{},
	}
}
