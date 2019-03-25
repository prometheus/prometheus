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
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	html_template "html/template"

	yaml "gopkg.in/yaml.v2"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	// AlertMetricName is the metric name for synthetic alert timeseries.
	alertMetricName = "ALERTS"
	// AlertForStateMetricName is the metric name for 'for' state of alert.
	alertForStateMetricName = "ALERTS_FOR_STATE"

	// AlertNameLabel is the label name indicating the name of an alert.
	alertNameLabel = "alertname"
	// AlertStateLabel is the label name indicating the state of an alert.
	alertStateLabel = "alertstate"
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
	panic(errors.Errorf("unknown alert state: %s", s.String()))
}

// Alert is the user-level representation of a single instance of an alerting rule.
type Alert struct {
	State AlertState

	Labels      labels.Labels
	Annotations labels.Labels

	// The value at the last evaluation of the alerting expression.
	Value float64
	// The interval during which the condition of this alert held true.
	// ResolvedAt will be 0 to indicate a still active alert.
	ActiveAt   time.Time
	FiredAt    time.Time
	ResolvedAt time.Time
	LastSentAt time.Time
	ValidUntil time.Time
}

func (a *Alert) needsSending(ts time.Time, resendDelay time.Duration) bool {
	if a.State == StatePending {
		return false
	}

	// if an alert has been resolved since the last send, resend it
	if a.ResolvedAt.After(a.LastSentAt) {
		return true
	}

	return a.LastSentAt.Add(resendDelay).Before(ts)
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
	labels labels.Labels
	// Non-identifying key/value pairs.
	annotations labels.Labels
	// true if old state has been restored. We start persisting samples for ALERT_FOR_STATE
	// only after the restoration.
	restored bool
	// Protects the below.
	mtx sync.Mutex
	// Time in seconds taken to evaluate rule.
	evaluationDuration time.Duration
	// Timestamp of last evaluation of rule.
	evaluationTimestamp time.Time
	// The health of the alerting rule.
	health RuleHealth
	// The last error seen by the alerting rule.
	lastError error
	// A map of alerts which are currently active (Pending or Firing), keyed by
	// the fingerprint of the labelset they correspond to.
	active map[uint64]*Alert

	logger log.Logger
}

// NewAlertingRule constructs a new AlertingRule.
func NewAlertingRule(name string, vec promql.Expr, hold time.Duration, lbls, anns labels.Labels, restored bool, logger log.Logger) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vec,
		holdDuration: hold,
		labels:       lbls,
		annotations:  anns,
		health:       HealthUnknown,
		active:       map[uint64]*Alert{},
		logger:       logger,
		restored:     restored,
	}
}

// Name returns the name of the alerting rule.
func (r *AlertingRule) Name() string {
	return r.name
}

// SetLastError sets the current error seen by the alerting rule.
func (r *AlertingRule) SetLastError(err error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.lastError = err
}

// LastError returns the last error seen by the alerting rule.
func (r *AlertingRule) LastError() error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.lastError
}

// SetHealth sets the current health of the alerting rule.
func (r *AlertingRule) SetHealth(health RuleHealth) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.health = health
}

// Health returns the current health of the alerting rule.
func (r *AlertingRule) Health() RuleHealth {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.health
}

// Query returns the query expression of the alerting rule.
func (r *AlertingRule) Query() promql.Expr {
	return r.vector
}

// Duration returns the hold duration of the alerting rule.
func (r *AlertingRule) Duration() time.Duration {
	return r.holdDuration
}

// Labels returns the labels of the alerting rule.
func (r *AlertingRule) Labels() labels.Labels {
	return r.labels
}

// Annotations returns the annotations of the alerting rule.
func (r *AlertingRule) Annotations() labels.Labels {
	return r.annotations
}

func (r *AlertingRule) sample(alert *Alert, ts time.Time) promql.Sample {
	lb := labels.NewBuilder(r.labels)

	for _, l := range alert.Labels {
		lb.Set(l.Name, l.Value)
	}

	lb.Set(labels.MetricName, alertMetricName)
	lb.Set(labels.AlertName, r.name)
	lb.Set(alertStateLabel, alert.State.String())

	s := promql.Sample{
		Metric: lb.Labels(),
		Point:  promql.Point{T: timestamp.FromTime(ts), V: 1},
	}
	return s
}

// forStateSample returns the sample for ALERTS_FOR_STATE.
func (r *AlertingRule) forStateSample(alert *Alert, ts time.Time, v float64) promql.Sample {
	lb := labels.NewBuilder(r.labels)

	for _, l := range alert.Labels {
		lb.Set(l.Name, l.Value)
	}

	lb.Set(labels.MetricName, alertForStateMetricName)
	lb.Set(labels.AlertName, r.name)

	s := promql.Sample{
		Metric: lb.Labels(),
		Point:  promql.Point{T: timestamp.FromTime(ts), V: v},
	}
	return s
}

// SetEvaluationDuration updates evaluationDuration to the duration it took to evaluate the rule on its last evaluation.
func (r *AlertingRule) SetEvaluationDuration(dur time.Duration) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.evaluationDuration = dur
}

// GetEvaluationDuration returns the time in seconds it took to evaluate the alerting rule.
func (r *AlertingRule) GetEvaluationDuration() time.Duration {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.evaluationDuration
}

// SetEvaluationTimestamp updates evaluationTimestamp to the timestamp of when the rule was last evaluated.
func (r *AlertingRule) SetEvaluationTimestamp(ts time.Time) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.evaluationTimestamp = ts
}

// GetEvaluationTimestamp returns the time the evaluation took place.
func (r *AlertingRule) GetEvaluationTimestamp() time.Time {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.evaluationTimestamp
}

// SetRestored updates the restoration state of the alerting rule.
func (r *AlertingRule) SetRestored(restored bool) {
	r.restored = restored
}

// resolvedRetention is the duration for which a resolved alert instance
// is kept in memory state and consequentally repeatedly sent to the AlertManager.
const resolvedRetention = 15 * time.Minute

// Eval evaluates the rule expression and then creates pending alerts and fires
// or removes previously pending alerts accordingly.
func (r *AlertingRule) Eval(ctx context.Context, ts time.Time, query QueryFunc, externalURL *url.URL) (promql.Vector, error) {
	res, err := query(ctx, r.vector.String(), ts)
	if err != nil {
		r.SetHealth(HealthBad)
		r.SetLastError(err)
		return nil, err
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFPs := map[uint64]struct{}{}

	var vec promql.Vector
	for _, smpl := range res {
		// Provide the alert information to the template.
		l := make(map[string]string, len(smpl.Metric))
		for _, lbl := range smpl.Metric {
			l[lbl.Name] = lbl.Value
		}

		tmplData := template.AlertTemplateData(l, smpl.V)
		// Inject some convenience variables that are easier to remember for users
		// who are not used to Go's templating system.
		defs := "{{$labels := .Labels}}{{$value := .Value}}"

		expand := func(text string) string {
			tmpl := template.NewTemplateExpander(
				ctx,
				defs+text,
				"__alert_"+r.Name(),
				tmplData,
				model.Time(timestamp.FromTime(ts)),
				template.QueryFunc(query),
				externalURL,
			)
			result, err := tmpl.Expand()
			if err != nil {
				result = fmt.Sprintf("<error expanding template: %s>", err)
				level.Warn(r.logger).Log("msg", "Expanding alert template failed", "err", err, "data", tmplData)
			}
			return result
		}

		lb := labels.NewBuilder(smpl.Metric).Del(labels.MetricName)

		for _, l := range r.labels {
			lb.Set(l.Name, expand(l.Value))
		}
		lb.Set(labels.AlertName, r.Name())

		annotations := make(labels.Labels, 0, len(r.annotations))
		for _, a := range r.annotations {
			annotations = append(annotations, labels.Label{Name: a.Name, Value: expand(a.Value)})
		}

		lbs := lb.Labels()
		h := lbs.Hash()
		resultFPs[h] = struct{}{}

		// Check whether we already have alerting state for the identifying label set.
		// Update the last value and annotations if so, create a new alert entry otherwise.
		if alert, ok := r.active[h]; ok && alert.State != StateInactive {
			alert.Value = smpl.V
			alert.Annotations = annotations
			continue
		}

		r.active[h] = &Alert{
			Labels:      lbs,
			Annotations: annotations,
			ActiveAt:    ts,
			State:       StatePending,
			Value:       smpl.V,
		}
	}

	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, a := range r.active {
		if _, ok := resultFPs[fp]; !ok {
			// If the alert was previously firing, keep it around for a given
			// retention time so it is reported as resolved to the AlertManager.
			if a.State == StatePending || (!a.ResolvedAt.IsZero() && ts.Sub(a.ResolvedAt) > resolvedRetention) {
				delete(r.active, fp)
			}
			if a.State != StateInactive {
				a.State = StateInactive
				a.ResolvedAt = ts
			}
			continue
		}

		if a.State == StatePending && ts.Sub(a.ActiveAt) >= r.holdDuration {
			a.State = StateFiring
			a.FiredAt = ts
		}

		if r.restored {
			vec = append(vec, r.sample(a, ts))
			vec = append(vec, r.forStateSample(a, ts, float64(a.ActiveAt.Unix())))
		}
	}

	// We have already acquired the lock above hence using SetHealth and
	// SetLastError will deadlock.
	r.health = HealthGood
	r.lastError = err
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
		if a.ResolvedAt.IsZero() {
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

// ForEachActiveAlert runs the given function on each alert.
// This should be used when you want to use the actual alerts from the AlertingRule
// and not on its copy.
// If you want to run on a copy of alerts then don't use this, get the alerts from 'ActiveAlerts()'.
func (r *AlertingRule) ForEachActiveAlert(f func(*Alert)) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, a := range r.active {
		f(a)
	}
}

func (r *AlertingRule) sendAlerts(ctx context.Context, ts time.Time, resendDelay time.Duration, interval time.Duration, notifyFunc NotifyFunc) {
	alerts := make([]*Alert, 0)
	r.ForEachActiveAlert(func(alert *Alert) {
		if alert.needsSending(ts, resendDelay) {
			alert.LastSentAt = ts
			// Allow for a couple Eval or Alertmanager send failures
			delta := resendDelay
			if interval > resendDelay {
				delta = interval
			}
			alert.ValidUntil = ts.Add(3 * delta)
			anew := *alert
			alerts = append(alerts, &anew)
		}
	})
	notifyFunc(ctx, r.vector.String(), alerts...)
}

func (r *AlertingRule) String() string {
	ar := rulefmt.Rule{
		Alert:       r.name,
		Expr:        r.vector.String(),
		For:         model.Duration(r.holdDuration),
		Labels:      r.labels.Map(),
		Annotations: r.annotations.Map(),
	}

	byt, err := yaml.Marshal(ar)
	if err != nil {
		return fmt.Sprintf("error marshaling alerting rule: %s", err.Error())
	}

	return string(byt)
}

// HTMLSnippet returns an HTML snippet representing this alerting rule. The
// resulting snippet is expected to be presented in a <pre> element, so that
// line breaks and other returned whitespace is respected.
func (r *AlertingRule) HTMLSnippet(pathPrefix string) html_template.HTML {
	alertMetric := model.Metric{
		model.MetricNameLabel: alertMetricName,
		alertNameLabel:        model.LabelValue(r.name),
	}

	labelsMap := make(map[string]string, len(r.labels))
	for _, l := range r.labels {
		labelsMap[l.Name] = html_template.HTMLEscapeString(l.Value)
	}

	annotationsMap := make(map[string]string, len(r.annotations))
	for _, l := range r.annotations {
		annotationsMap[l.Name] = html_template.HTMLEscapeString(l.Value)
	}

	ar := rulefmt.Rule{
		Alert:       fmt.Sprintf("<a href=%q>%s</a>", pathPrefix+strutil.TableLinkForExpression(alertMetric.String()), r.name),
		Expr:        fmt.Sprintf("<a href=%q>%s</a>", pathPrefix+strutil.TableLinkForExpression(r.vector.String()), html_template.HTMLEscapeString(r.vector.String())),
		For:         model.Duration(r.holdDuration),
		Labels:      labelsMap,
		Annotations: annotationsMap,
	}

	byt, err := yaml.Marshal(ar)
	if err != nil {
		return html_template.HTML(fmt.Sprintf("error marshaling alerting rule: %q", html_template.HTMLEscapeString(err.Error())))
	}
	return html_template.HTML(byt)
}

// HoldDuration returns the holdDuration of the alerting rule.
func (r *AlertingRule) HoldDuration() time.Duration {
	return r.holdDuration
}
