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
	"sync"
	"time"

	"golang.org/x/net/context"

	html_template "html/template"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/template"
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

const (
	// StateInactive is the state of an alert that is neither firing nor pending.
	StateInactive AlertState = iota
	// StatePending is the state of an alert that has been active for less than
	// the configured threshold duration.
	StatePending
	// StateFiring is the state of an alert that has been active for longer than
	// the configured threshold duration.
	StateFiring
)

func (s AlertState) String() string {
	switch s {
	case StateInactive:
		return "inactive"
	case StatePending:
		return "pending"
	case StateFiring:
		return "firing"
	}
	panic(fmt.Errorf("unknown alert state: %v", s.String()))
}

// Alert is the user-level representation of a single instance of an alerting rule.
type Alert struct {
	State       AlertState
	Labels      model.LabelSet
	Annotations model.LabelSet
	// The value at the last evaluation of the alerting expression.
	Value model.SampleValue
	// The interval during which the condition of this alert held true.
	// ResolvedAt will be 0 to indicate a still active alert.
	ActiveAt, ResolvedAt model.Time
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
	active map[model.Fingerprint]*Alert
}

// NewAlertingRule constructs a new AlertingRule.
func NewAlertingRule(name string, vec promql.Expr, hold time.Duration, lbls, anns model.LabelSet) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vec,
		holdDuration: hold,
		labels:       lbls,
		annotations:  anns,
		active:       map[model.Fingerprint]*Alert{},
	}
}

// Name returns the name of the alert.
func (r *AlertingRule) Name() string {
	return r.name
}

func (r *AlertingRule) equal(o *AlertingRule) bool {
	return r.name == o.name && r.labels.Equal(o.labels)
}

func (r *AlertingRule) sample(alert *Alert, ts model.Time, set bool) *model.Sample {
	metric := model.Metric(r.labels.Clone())

	for ln, lv := range alert.Labels {
		metric[ln] = lv
	}

	metric[model.MetricNameLabel] = alertMetricName
	metric[model.AlertNameLabel] = model.LabelValue(r.name)
	metric[alertStateLabel] = model.LabelValue(alert.State.String())

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

// resolvedRetention is the duration for which a resolved alert instance
// is kept in memory state and consequentally repeatedly sent to the AlertManager.
const resolvedRetention = 15 * time.Minute

// Eval evaluates the rule expression and then creates pending alerts and fires
// or removes previously pending alerts accordingly.
func (r *AlertingRule) Eval(ctx context.Context, ts model.Time, engine *promql.Engine, externalURLPath string) (model.Vector, error) {
	query, err := engine.NewInstantQuery(r.vector.String(), ts)
	if err != nil {
		return nil, err
	}
	res, err := query.Exec(ctx).Vector()
	if err != nil {
		return nil, err
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFPs := map[model.Fingerprint]struct{}{}

	for _, smpl := range res {
		// Provide the alert information to the template.
		l := make(map[string]string, len(smpl.Metric))
		for k, v := range smpl.Metric {
			l[string(k)] = string(v)
		}

		tmplData := struct {
			Labels map[string]string
			Value  float64
		}{
			Labels: l,
			Value:  float64(smpl.Value),
		}
		// Inject some convenience variables that are easier to remember for users
		// who are not used to Go's templating system.
		defs := "{{$labels := .Labels}}{{$value := .Value}}"

		expand := func(text model.LabelValue) model.LabelValue {
			tmpl := template.NewTemplateExpander(
				ctx,
				defs+string(text),
				"__alert_"+r.Name(),
				tmplData,
				ts,
				engine,
				externalURLPath,
			)
			result, err := tmpl.Expand()
			if err != nil {
				result = fmt.Sprintf("<error expanding template: %s>", err)
				log.Warnf("Error expanding alert template %v with data '%v': %s", r.Name(), tmplData, err)
			}
			return model.LabelValue(result)
		}

		delete(smpl.Metric, model.MetricNameLabel)
		labels := make(model.LabelSet, len(smpl.Metric)+len(r.labels)+1)
		for ln, lv := range smpl.Metric {
			labels[ln] = lv
		}
		for ln, lv := range r.labels {
			labels[ln] = expand(lv)
		}
		labels[model.AlertNameLabel] = model.LabelValue(r.Name())

		annotations := make(model.LabelSet, len(r.annotations))
		for an, av := range r.annotations {
			annotations[an] = expand(av)
		}
		fp := smpl.Metric.Fingerprint()
		resultFPs[fp] = struct{}{}

		// Check whether we already have alerting state for the identifying label set.
		// Update the last value and annotations if so, create a new alert entry otherwise.
		if alert, ok := r.active[fp]; ok && alert.State != StateInactive {
			alert.Value = smpl.Value
			alert.Annotations = annotations
			continue
		}

		r.active[fp] = &Alert{
			Labels:      labels,
			Annotations: annotations,
			ActiveAt:    ts,
			State:       StatePending,
			Value:       smpl.Value,
		}
	}

	var vec model.Vector
	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, a := range r.active {
		if _, ok := resultFPs[fp]; !ok {
			if a.State != StateInactive {
				vec = append(vec, r.sample(a, ts, false))
			}
			// If the alert was previously firing, keep it around for a given
			// retention time so it is reported as resolved to the AlertManager.
			if a.State == StatePending || (a.ResolvedAt != 0 && ts.Sub(a.ResolvedAt) > resolvedRetention) {
				delete(r.active, fp)
			}
			if a.State != StateInactive {
				a.State = StateInactive
				a.ResolvedAt = ts
			}
			continue
		}

		if a.State == StatePending && ts.Sub(a.ActiveAt) >= r.holdDuration {
			vec = append(vec, r.sample(a, ts, false))
			a.State = StateFiring
		}

		vec = append(vec, r.sample(a, ts, true))
	}

	return vec, nil
}

// State returns the maximum state of alert instances for this rule.
// StateFiring > StatePending > StateInactive
func (r *AlertingRule) State() AlertState {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	maxState := StateInactive
	for _, a := range r.active {
		if a.State > maxState {
			maxState = a.State
		}
	}
	return maxState
}

// ActiveAlerts returns a slice of active alerts.
func (r *AlertingRule) ActiveAlerts() []*Alert {
	var res []*Alert
	for _, a := range r.currentAlerts() {
		if a.ResolvedAt == 0 {
			res = append(res, a)
		}
	}
	return res
}

// currentAlerts returns all instances of alerts for this rule. This may include
// inactive alerts that were previously firing.
func (r *AlertingRule) currentAlerts() []*Alert {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	alerts := make([]*Alert, 0, len(r.active))

	for _, a := range r.active {
		anew := *a
		alerts = append(alerts, &anew)
	}
	return alerts
}

func (r *AlertingRule) String() string {
	s := fmt.Sprintf("ALERT %s", r.name)
	s += fmt.Sprintf("\n\tIF %s", r.vector)
	if r.holdDuration > 0 {
		s += fmt.Sprintf("\n\tFOR %s", model.Duration(r.holdDuration))
	}
	if len(r.labels) > 0 {
		s += fmt.Sprintf("\n\tLABELS %s", r.labels)
	}
	if len(r.annotations) > 0 {
		s += fmt.Sprintf("\n\tANNOTATIONS %s", r.annotations)
	}
	return s
}

// HTMLSnippet returns an HTML snippet representing this alerting rule. The
// resulting snippet is expected to be presented in a <pre> element, so that
// line breaks and other returned whitespace is respected.
func (r *AlertingRule) HTMLSnippet(pathPrefix string) html_template.HTML {
	alertMetric := model.Metric{
		model.MetricNameLabel: alertMetricName,
		alertNameLabel:        model.LabelValue(r.name),
	}
	s := fmt.Sprintf("ALERT <a href=%q>%s</a>", pathPrefix+strutil.GraphLinkForExpression(alertMetric.String()), r.name)
	s += fmt.Sprintf("\n  IF <a href=%q>%s</a>", pathPrefix+strutil.GraphLinkForExpression(r.vector.String()), html_template.HTMLEscapeString(r.vector.String()))
	if r.holdDuration > 0 {
		s += fmt.Sprintf("\n  FOR %s", model.Duration(r.holdDuration))
	}
	if len(r.labels) > 0 {
		s += fmt.Sprintf("\n  LABELS %s", html_template.HTMLEscapeString(r.labels.String()))
	}
	if len(r.annotations) > 0 {
		s += fmt.Sprintf("\n  ANNOTATIONS %s", html_template.HTMLEscapeString(r.annotations.String()))
	}
	return html_template.HTML(s)
}
