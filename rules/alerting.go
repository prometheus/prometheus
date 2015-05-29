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

package rules

import (
	"fmt"
	"html/template"
	"reflect"
	"strings"
	"sync"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// AlertMetricName is the metric name for synthetic alert timeseries.
	AlertMetricName clientmodel.LabelValue = "ALERTS"

	// AlertNameLabel is the label name indicating the name of an alert.
	AlertNameLabel clientmodel.LabelName = "alertname"
	// AlertStateLabel is the label name indicating the state of an alert.
	AlertStateLabel clientmodel.LabelName = "alertstate"
)

// AlertState denotes the state of an active alert.
type AlertState int

func (s AlertState) String() string {
	switch s {
	case Inactive:
		return "inactive"
	case Pending:
		return "pending"
	case Firing:
		return "firing"
	default:
		panic("undefined")
	}
}

const (
	// Inactive alerts are neither firing nor pending.
	Inactive AlertState = iota
	// Pending alerts have been active for less than the configured
	// threshold duration.
	Pending
	// Firing alerts have been active for longer than the configured
	// threshold duration.
	Firing
)

// Alert is used to track active (pending/firing) alerts over time.
type Alert struct {
	// The name of the alert.
	Name string
	// The vector element labelset triggering this alert.
	Labels clientmodel.LabelSet
	// The state of the alert (Pending or Firing).
	State AlertState
	// The time when the alert first transitioned into Pending state.
	ActiveSince clientmodel.Timestamp
	// The value of the alert expression for this vector element.
	Value clientmodel.SampleValue
}

// sample returns a Sample suitable for recording the alert.
func (a Alert) sample(timestamp clientmodel.Timestamp, value clientmodel.SampleValue) *promql.Sample {
	recordedMetric := clientmodel.Metric{}
	for label, value := range a.Labels {
		recordedMetric[label] = value
	}

	recordedMetric[clientmodel.MetricNameLabel] = AlertMetricName
	recordedMetric[AlertNameLabel] = clientmodel.LabelValue(a.Name)
	recordedMetric[AlertStateLabel] = clientmodel.LabelValue(a.State.String())

	return &promql.Sample{
		Metric: clientmodel.COWMetric{
			Metric: recordedMetric,
			Copied: true,
		},
		Value:     value,
		Timestamp: timestamp,
	}
}

// An AlertingRule generates alerts from its vector expression.
type AlertingRule struct {
	// The name of the alert.
	name string
	// The vector expression from which to generate alerts.
	Vector promql.Expr
	// The duration for which a labelset needs to persist in the expression
	// output vector before an alert transitions from Pending to Firing state.
	holdDuration time.Duration
	// Extra labels to attach to the resulting alert sample vectors.
	Labels clientmodel.LabelSet
	// Short alert summary, suitable for email subjects.
	Summary string
	// More detailed alert description.
	Description string

	// Protects the below.
	mutex sync.Mutex
	// A map of alerts which are currently active (Pending or Firing), keyed by
	// the fingerprint of the labelset they correspond to.
	activeAlerts map[clientmodel.Fingerprint]*Alert
}

// Name returns the name of the alert.
func (rule *AlertingRule) Name() string {
	return rule.name
}

// EvalRaw returns the raw value of the rule expression, without creating alerts.
func (rule *AlertingRule) EvalRaw(timestamp clientmodel.Timestamp, engine *promql.Engine) (promql.Vector, error) {
	query, err := engine.NewInstantQuery(rule.Vector.String(), timestamp)
	if err != nil {
		return nil, err
	}
	return query.Exec().Vector()
}

// Eval evaluates the rule expression and then creates pending alerts and fires
// or removes previously pending alerts accordingly.
func (rule *AlertingRule) Eval(timestamp clientmodel.Timestamp, engine *promql.Engine) (promql.Vector, error) {
	exprResult, err := rule.EvalRaw(timestamp, engine)
	if err != nil {
		return nil, err
	}

	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFPs := map[clientmodel.Fingerprint]struct{}{}
	for _, sample := range exprResult {
		fp := sample.Metric.Metric.Fingerprint()
		resultFPs[fp] = struct{}{}

		if alert, ok := rule.activeAlerts[fp]; !ok {
			labels := clientmodel.LabelSet{}
			labels.MergeFromMetric(sample.Metric.Metric)
			labels = labels.Merge(rule.Labels)
			if _, ok := labels[clientmodel.MetricNameLabel]; ok {
				delete(labels, clientmodel.MetricNameLabel)
			}
			rule.activeAlerts[fp] = &Alert{
				Name:        rule.name,
				Labels:      labels,
				State:       Pending,
				ActiveSince: timestamp,
				Value:       sample.Value,
			}
		} else {
			alert.Value = sample.Value
		}
	}

	vector := promql.Vector{}

	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, activeAlert := range rule.activeAlerts {
		if _, ok := resultFPs[fp]; !ok {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			delete(rule.activeAlerts, fp)
			continue
		}

		if activeAlert.State == Pending && timestamp.Sub(activeAlert.ActiveSince) >= rule.holdDuration {
			vector = append(vector, activeAlert.sample(timestamp, 0))
			activeAlert.State = Firing
		}

		vector = append(vector, activeAlert.sample(timestamp, 1))
	}

	return vector, nil
}

// DotGraph returns the text representation of a dot graph.
func (rule *AlertingRule) DotGraph() string {
	graph := fmt.Sprintf(
		`digraph "Rules" {
	  %#p[shape="box",label="ALERT %s IF FOR %s"];
		%#p -> %x;
		%s
	}`,
		&rule, rule.name, strutil.DurationToString(rule.holdDuration),
		&rule, reflect.ValueOf(rule.Vector).Pointer(),
		rule.Vector.DotGraph(),
	)
	return graph
}

func (rule *AlertingRule) String() string {
	return fmt.Sprintf("ALERT %s IF %s FOR %s WITH %s", rule.name, rule.Vector, strutil.DurationToString(rule.holdDuration), rule.Labels)
}

// HTMLSnippet returns an HTML snippet representing this alerting rule.
func (rule *AlertingRule) HTMLSnippet(pathPrefix string) template.HTML {
	alertMetric := clientmodel.Metric{
		clientmodel.MetricNameLabel: AlertMetricName,
		AlertNameLabel:              clientmodel.LabelValue(rule.name),
	}
	return template.HTML(fmt.Sprintf(
		`ALERT <a href="%s">%s</a> IF <a href="%s">%s</a> FOR %s WITH %s`,
		pathPrefix+strings.TrimLeft(strutil.GraphLinkForExpression(alertMetric.String()), "/"),
		rule.name,
		pathPrefix+strings.TrimLeft(strutil.GraphLinkForExpression(rule.Vector.String()), "/"),
		rule.Vector,
		strutil.DurationToString(rule.holdDuration),
		rule.Labels))
}

// State returns the "maximum" state: firing > pending > inactive.
func (rule *AlertingRule) State() AlertState {
	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	maxState := Inactive
	for _, activeAlert := range rule.activeAlerts {
		if activeAlert.State > maxState {
			maxState = activeAlert.State
		}
	}
	return maxState
}

// ActiveAlerts returns a slice of active alerts.
func (rule *AlertingRule) ActiveAlerts() []Alert {
	rule.mutex.Lock()
	defer rule.mutex.Unlock()

	alerts := make([]Alert, 0, len(rule.activeAlerts))
	for _, alert := range rule.activeAlerts {
		alerts = append(alerts, *alert)
	}
	return alerts
}

// NewAlertingRule constructs a new AlertingRule.
func NewAlertingRule(name string, vector promql.Expr, holdDuration time.Duration, labels clientmodel.LabelSet, summary string, description string) *AlertingRule {
	return &AlertingRule{
		name:         name,
		Vector:       vector,
		holdDuration: holdDuration,
		Labels:       labels,
		Summary:      summary,
		Description:  description,

		activeAlerts: map[clientmodel.Fingerprint]*Alert{},
	}
}
