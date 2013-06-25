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
	"fmt"
	"html/template"
	"sync"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

const (
	// The metric name for synthetic alert timeseries.
	AlertMetricName clientmodel.LabelValue = "ALERTS"

	// The label name indicating the name of an alert.
	AlertNameLabel clientmodel.LabelName = "alertname"
	// The label name indicating the state of an alert.
	AlertStateLabel clientmodel.LabelName = "alertstate"
)

// States that active alerts can be in.
type AlertState int

func (s AlertState) String() string {
	switch s {
	case INACTIVE:
		return "inactive"
	case PENDING:
		return "pending"
	case FIRING:
		return "firing"
	default:
		panic("undefined")
	}
}

const (
	INACTIVE AlertState = iota
	PENDING
	FIRING
)

// Alert is used to track active (pending/firing) alerts over time.
type Alert struct {
	// The name of the alert.
	Name string
	// The vector element labelset triggering this alert.
	Labels clientmodel.LabelSet
	// The state of the alert (PENDING or FIRING).
	State AlertState
	// The time when the alert first transitioned into PENDING state.
	ActiveSince time.Time
	// The value of the alert expression for this vector element.
	Value clientmodel.SampleValue
}

// sample returns a Sample suitable for recording the alert.
func (a Alert) sample(timestamp time.Time, value clientmodel.SampleValue) *clientmodel.Sample {
	recordedMetric := clientmodel.Metric{}
	for label, value := range a.Labels {
		recordedMetric[label] = value
	}

	recordedMetric[clientmodel.MetricNameLabel] = AlertMetricName
	recordedMetric[AlertNameLabel] = clientmodel.LabelValue(a.Name)
	recordedMetric[AlertStateLabel] = clientmodel.LabelValue(a.State.String())

	return &clientmodel.Sample{
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
	labels clientmodel.LabelSet

	// Protects the below.
	mutex sync.Mutex
	// A map of alerts which are currently active (PENDING or FIRING), keyed by
	// the fingerprint of the labelset they correspond to.
	activeAlerts map[clientmodel.Fingerprint]*Alert
}

func (rule *AlertingRule) Name() string { return rule.name }

func (rule *AlertingRule) EvalRaw(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error) {
	return ast.EvalVectorInstant(rule.vector, timestamp, storage, stats.NewTimerGroup())
}

func (rule *AlertingRule) Eval(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error) {
	// Get the raw value of the rule expression.
	exprResult, err := rule.EvalRaw(timestamp, storage)
	if err != nil {
		return nil, err
	}

	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFingerprints := utility.Set{}
	for _, sample := range exprResult {
		fp := new(clientmodel.Fingerprint)
		fp.LoadFromMetric(sample.Metric)
		resultFingerprints.Add(*fp)

		if alert, ok := rule.activeAlerts[*fp]; !ok {
			labels := clientmodel.LabelSet{}
			labels.MergeFromMetric(sample.Metric)
			if _, ok := labels[clientmodel.MetricNameLabel]; ok {
				delete(labels, clientmodel.MetricNameLabel)
			}
			rule.activeAlerts[*fp] = &Alert{
				Name:        rule.name,
				Labels:      labels,
				State:       PENDING,
				ActiveSince: timestamp,
				Value:       sample.Value,
			}
		} else {
			alert.Value = sample.Value
		}
	}

	vector := ast.Vector{}

	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, activeAlert := range rule.activeAlerts {
		if !resultFingerprints.Has(fp) {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			delete(rule.activeAlerts, fp)
			continue
		}

		if activeAlert.State == PENDING && timestamp.Sub(activeAlert.ActiveSince) >= rule.holdDuration {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			activeAlert.State = FIRING
		}

		vector = append(vector, activeAlert.sample(timestamp, 1))
	}

	return vector, nil
}

func (rule *AlertingRule) ToDotGraph() string {
	graph := fmt.Sprintf(`digraph "Rules" {
	  %#p[shape="box",label="ALERT %s IF FOR %s"];
		%#p -> %#p;
		%s
	}`, &rule, rule.name, utility.DurationToString(rule.holdDuration), &rule, rule.vector, rule.vector.NodeTreeToDotGraph())
	return graph
}

func (rule *AlertingRule) String() string {
	return fmt.Sprintf("ALERT %s IF %s FOR %s WITH %s", rule.name, rule.vector, utility.DurationToString(rule.holdDuration), rule.labels)
}

func (rule *AlertingRule) HTMLSnippet() template.HTML {
	alertMetric := clientmodel.Metric{
		clientmodel.MetricNameLabel: AlertMetricName,
		AlertNameLabel:              clientmodel.LabelValue(rule.name),
	}
	return template.HTML(fmt.Sprintf(
		`ALERT <a href="%s">%s</a> IF <a href="%s">%s</a> FOR %s WITH %s`,
		ConsoleLinkForExpression(alertMetric.String()),
		rule.name,
		ConsoleLinkForExpression(rule.vector.String()),
		rule.vector,
		utility.DurationToString(rule.holdDuration),
		rule.labels))
}

func (rule *AlertingRule) State() AlertState {
	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	maxState := INACTIVE
	for _, activeAlert := range rule.activeAlerts {
		if activeAlert.State > maxState {
			maxState = activeAlert.State
		}
	}
	return maxState
}

func (rule *AlertingRule) ActiveAlerts() []Alert {
	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	alerts := make([]Alert, 0, len(rule.activeAlerts))
	for _, alert := range rule.activeAlerts {
		alerts = append(alerts, *alert)
	}
	return alerts
}

// Construct a new AlertingRule.
func NewAlertingRule(name string, vector ast.VectorNode, holdDuration time.Duration, labels clientmodel.LabelSet) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vector,
		holdDuration: holdDuration,
		labels:       labels,
		activeAlerts: map[clientmodel.Fingerprint]*Alert{},
	}
}
