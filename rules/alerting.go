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
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// AlertMetricName is the metric name for synthetic alert timeseries.
	alertMetricName model.LabelValue = "ALERTS"

	// AlertNameLabel is the label name indicating the name of an alert.
	alertNameLabel model.LabelName = "alertname"
	// AlertStateLabel is the label name indicating the state of an alert.
	alertStateLabel model.LabelName = "alertstate"
)

// AlertState denotes the state of an active alert.
type AlertState int

func (s AlertState) String() string {
	switch s {
	case StateInactive:
		return "inactive"
	case StatePending:
		return "pending"
	case StateFiring:
		return "firing"
	}
	panic(fmt.Errorf("unknown alert state: %v", s))
}

const (
	StateInactive AlertState = iota
	StatePending
	StateFiring
)

type alertInstance struct {
	metric      model.Metric
	value       model.SampleValue
	state       AlertState
	activeSince model.Time
}

// An AlertingRule generates alerts from its vector expression.
type AlertingRule struct {
	// The name of the alert.
	name string
	// The vector expression from which to generate alerts.
	vector promql.Expr
	// The duration for which a labelset needs to persist in the expression
	// output vector before an alert transitions from Pending to Firing state.
	holdDuration time.Duration
	// Extra labels to attach to the resulting alert sample vectors.
	labels model.LabelSet
	// Non-identifying key/value pairs.
	annotations model.LabelSet

	// Protects the below.
	mtx sync.Mutex
	// A map of alerts which are currently active (Pending or Firing), keyed by
	// the fingerprint of the labelset they correspond to.
	active map[model.Fingerprint]*alertInstance
}

// NewAlertingRule constructs a new AlertingRule.
func NewAlertingRule(name string, vec promql.Expr, hold time.Duration, lbls, anns model.LabelSet) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vec,
		holdDuration: hold,
		labels:       lbls,
		annotations:  anns,
		active:       map[model.Fingerprint]*alertInstance{},
	}
}

// Name returns the name of the alert.
func (rule *AlertingRule) Name() string {
	return rule.name
}

func (r *AlertingRule) sample(ai *alertInstance, ts model.Time, set bool) *model.Sample {
	// Build alert labels in order they can be overwritten.
	metric := model.Metric(r.labels.Clone())

	for ln, lv := range ai.metric {
		metric[ln] = lv
	}

	metric[model.MetricNameLabel] = alertMetricName
	metric[model.AlertNameLabel] = model.LabelValue(r.name)
	metric[alertStateLabel] = model.LabelValue(ai.state.String())

	s := &model.Sample{
		Metric:    metric,
		Timestamp: ts,
		Value:     0,
	}
	if set {
		s.Value = 1
	}
	return s
}

// eval evaluates the rule expression and then creates pending alerts and fires
// or removes previously pending alerts accordingly.
func (r *AlertingRule) eval(ts model.Time, engine *promql.Engine) (model.Vector, error) {
	query, err := engine.NewInstantQuery(r.vector.String(), ts)
	if err != nil {
		return nil, err
	}
	res, err := query.Exec().Vector()
	if err != nil {
		return nil, err
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFPs := map[model.Fingerprint]struct{}{}

	for _, smpl := range res {
		fp := smpl.Metric.Fingerprint()
		resultFPs[fp] = struct{}{}

		if ai, ok := r.active[fp]; ok {
			ai.value = smpl.Value
			continue
		}

		delete(smpl.Metric, model.MetricNameLabel)

		r.active[fp] = &alertInstance{
			metric:      smpl.Metric,
			activeSince: ts,
			state:       StatePending,
			value:       smpl.Value,
		}
	}

	var vec model.Vector
	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, ai := range r.active {
		if _, ok := resultFPs[fp]; !ok {
			delete(r.active, fp)
			vec = append(vec, r.sample(ai, ts, false))
			continue
		}

		if ai.state != StateFiring && ts.Sub(ai.activeSince) >= r.holdDuration {
			vec = append(vec, r.sample(ai, ts, false))
			ai.state = StateFiring
		}

		vec = append(vec, r.sample(ai, ts, true))
	}

	return vec, nil
}

// Alert is the user-level representation of a single instance of an alerting rule.
type Alert struct {
	State       AlertState
	Labels      model.LabelSet
	ActiveSince model.Time
	Value       model.SampleValue
}

func (r *AlertingRule) State() AlertState {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	maxState := StateInactive
	for _, ai := range r.active {
		if ai.state > maxState {
			maxState = ai.state
		}
	}
	return maxState
}

// ActiveAlerts returns a slice of active alerts.
func (r *AlertingRule) ActiveAlerts() []*Alert {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	alerts := make([]*Alert, 0, len(r.active))
	for _, ai := range r.active {
		labels := r.labels.Clone()
		for ln, lv := range ai.metric {
			labels[ln] = lv
		}
		alerts = append(alerts, &Alert{
			State:       ai.state,
			Labels:      labels,
			ActiveSince: ai.activeSince,
			Value:       ai.value,
		})
	}
	return alerts
}

func (rule *AlertingRule) String() string {
	s := fmt.Sprintf("ALERT %s", rule.name)
	s += fmt.Sprintf("\n\tIF %s", rule.vector)
	if rule.holdDuration > 0 {
		s += fmt.Sprintf("\n\tFOR %s", strutil.DurationToString(rule.holdDuration))
	}
	if len(rule.labels) > 0 {
		s += fmt.Sprintf("\n\tWITH %s", rule.labels)
	}
	if len(rule.annotations) > 0 {
		s += fmt.Sprintf("\n\tANNOTATIONS %s", rule.annotations)
	}
	return s
}

// HTMLSnippet returns an HTML snippet representing this alerting rule. The
// resulting snippet is expected to be presented in a <pre> element, so that
// line breaks and other returned whitespace is respected.
func (rule *AlertingRule) HTMLSnippet(pathPrefix string) template.HTML {
	alertMetric := model.Metric{
		model.MetricNameLabel: alertMetricName,
		alertNameLabel:        model.LabelValue(rule.name),
	}
	s := fmt.Sprintf("ALERT <a href=%q>%s</a>", pathPrefix+strutil.GraphLinkForExpression(alertMetric.String()), rule.name)
	s += fmt.Sprintf("\n  IF <a href=%q>%s</a>", pathPrefix+strutil.GraphLinkForExpression(rule.vector.String()), rule.vector)
	if rule.holdDuration > 0 {
		s += fmt.Sprintf("\n  FOR %s", strutil.DurationToString(rule.holdDuration))
	}
	if len(rule.labels) > 0 {
		s += fmt.Sprintf("\n  WITH %s", rule.labels)
	}
	if len(rule.annotations) > 0 {
		s += fmt.Sprintf("\n  ANNOTATIONS %s", rule.annotations)
	}
	return template.HTML(s)
}
