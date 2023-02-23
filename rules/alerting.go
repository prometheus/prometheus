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
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/template"
)

const (
	// AlertMetricName is the metric name for synthetic alert timeseries.
	alertMetricName = "ALERTS"
	// AlertForStateMetricName is the metric name for 'for' state of alert.
	alertForStateMetricName = "ALERTS_FOR_STATE"

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
	panic(fmt.Errorf("unknown alert state: %d", s))
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
	ActiveAt        time.Time
	FiredAt         time.Time
	ResolvedAt      time.Time
	LastSentAt      time.Time
	ValidUntil      time.Time
	KeepFiringSince time.Time
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
	vector parser.Expr
	// The duration for which a labelset needs to persist in the expression
	// output vector before an alert transitions from Pending to Firing state.
	holdDuration time.Duration
	// The amount of time that the alert should remain firing after the
	// resolution.
	keepFiringFor time.Duration
	// Extra labels to attach to the resulting alert sample vectors.
	labels labels.Labels
	// Non-identifying key/value pairs.
	annotations labels.Labels
	// External labels from the global config.
	externalLabels map[string]string
	// The external URL from the --web.external-url flag.
	externalURL string
	// true if old state has been restored. We start persisting samples for ALERT_FOR_STATE
	// only after the restoration.
	restored *atomic.Bool
	// Time in seconds taken to evaluate rule.
	evaluationDuration *atomic.Duration
	// Timestamp of last evaluation of rule.
	evaluationTimestamp *atomic.Time
	// The health of the alerting rule.
	health *atomic.String
	// The last error seen by the alerting rule.
	lastError *atomic.Error
	// activeMtx Protects the `active` map.
	activeMtx sync.Mutex
	// A map of alerts which are currently active (Pending or Firing), keyed by
	// the fingerprint of the labelset they correspond to.
	active map[uint64]*Alert

	logger log.Logger
}

// NewAlertingRule constructs a new AlertingRule.
func NewAlertingRule(
	name string, vec parser.Expr, hold, keepFiringFor time.Duration,
	labels, annotations, externalLabels labels.Labels, externalURL string,
	restored bool, logger log.Logger,
) *AlertingRule {
	el := externalLabels.Map()

	return &AlertingRule{
		name:                name,
		vector:              vec,
		holdDuration:        hold,
		keepFiringFor:       keepFiringFor,
		labels:              labels,
		annotations:         annotations,
		externalLabels:      el,
		externalURL:         externalURL,
		active:              map[uint64]*Alert{},
		logger:              logger,
		restored:            atomic.NewBool(restored),
		health:              atomic.NewString(string(HealthUnknown)),
		evaluationTimestamp: atomic.NewTime(time.Time{}),
		evaluationDuration:  atomic.NewDuration(0),
		lastError:           atomic.NewError(nil),
	}
}

// Name returns the name of the alerting rule.
func (r *AlertingRule) Name() string {
	return r.name
}

// SetLastError sets the current error seen by the alerting rule.
func (r *AlertingRule) SetLastError(err error) {
	r.lastError.Store(err)
}

// LastError returns the last error seen by the alerting rule.
func (r *AlertingRule) LastError() error {
	return r.lastError.Load()
}

// SetHealth sets the current health of the alerting rule.
func (r *AlertingRule) SetHealth(health RuleHealth) {
	r.health.Store(string(health))
}

// Health returns the current health of the alerting rule.
func (r *AlertingRule) Health() RuleHealth {
	return RuleHealth(r.health.String())
}

// Query returns the query expression of the alerting rule.
func (r *AlertingRule) Query() parser.Expr {
	return r.vector
}

// HoldDuration returns the hold duration of the alerting rule.
func (r *AlertingRule) HoldDuration() time.Duration {
	return r.holdDuration
}

// KeepFiringFor returns the duration an alerting rule should keep firing for
// after resolution.
func (r *AlertingRule) KeepFiringFor() time.Duration {
	return r.keepFiringFor
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

	alert.Labels.Range(func(l labels.Label) {
		lb.Set(l.Name, l.Value)
	})

	lb.Set(labels.MetricName, alertMetricName)
	lb.Set(labels.AlertName, r.name)
	lb.Set(alertStateLabel, alert.State.String())

	s := promql.Sample{
		Metric: lb.Labels(labels.EmptyLabels()),
		Point:  promql.Point{T: timestamp.FromTime(ts), V: 1},
	}
	return s
}

// forStateSample returns the sample for ALERTS_FOR_STATE.
func (r *AlertingRule) forStateSample(alert *Alert, ts time.Time, v float64) promql.Sample {
	lb := labels.NewBuilder(r.labels)

	alert.Labels.Range(func(l labels.Label) {
		lb.Set(l.Name, l.Value)
	})

	lb.Set(labels.MetricName, alertForStateMetricName)
	lb.Set(labels.AlertName, r.name)

	s := promql.Sample{
		Metric: lb.Labels(labels.EmptyLabels()),
		Point:  promql.Point{T: timestamp.FromTime(ts), V: v},
	}
	return s
}

// QueryforStateSeries returns the series for ALERTS_FOR_STATE.
func (r *AlertingRule) QueryforStateSeries(alert *Alert, q storage.Querier) (storage.Series, error) {
	smpl := r.forStateSample(alert, time.Now(), 0)
	var matchers []*labels.Matcher
	smpl.Metric.Range(func(l labels.Label) {
		mt, err := labels.NewMatcher(labels.MatchEqual, l.Name, l.Value)
		if err != nil {
			panic(err)
		}
		matchers = append(matchers, mt)
	})
	sset := q.Select(false, nil, matchers...)

	var s storage.Series
	for sset.Next() {
		// Query assures that smpl.Metric is included in sset.At().Labels(),
		// hence just checking the length would act like equality.
		// (This is faster than calling labels.Compare again as we already have some info).
		if sset.At().Labels().Len() == len(matchers) {
			s = sset.At()
			break
		}
	}

	return s, sset.Err()
}

// SetEvaluationDuration updates evaluationDuration to the duration it took to evaluate the rule on its last evaluation.
func (r *AlertingRule) SetEvaluationDuration(dur time.Duration) {
	r.evaluationDuration.Store(dur)
}

// GetEvaluationDuration returns the time in seconds it took to evaluate the alerting rule.
func (r *AlertingRule) GetEvaluationDuration() time.Duration {
	return r.evaluationDuration.Load()
}

// SetEvaluationTimestamp updates evaluationTimestamp to the timestamp of when the rule was last evaluated.
func (r *AlertingRule) SetEvaluationTimestamp(ts time.Time) {
	r.evaluationTimestamp.Store(ts)
}

// GetEvaluationTimestamp returns the time the evaluation took place.
func (r *AlertingRule) GetEvaluationTimestamp() time.Time {
	return r.evaluationTimestamp.Load()
}

// SetRestored updates the restoration state of the alerting rule.
func (r *AlertingRule) SetRestored(restored bool) {
	r.restored.Store(restored)
}

// Restored returns the restoration state of the alerting rule.
func (r *AlertingRule) Restored() bool {
	return r.restored.Load()
}

// resolvedRetention is the duration for which a resolved alert instance
// is kept in memory state and consequently repeatedly sent to the AlertManager.
const resolvedRetention = 15 * time.Minute

// Eval evaluates the rule expression and then creates pending alerts and fires
// or removes previously pending alerts accordingly.
func (r *AlertingRule) Eval(ctx context.Context, ts time.Time, query QueryFunc, externalURL *url.URL, limit int) (promql.Vector, error) {
	ctx = NewOriginContext(ctx, NewRuleDetail(r))

	res, err := query(ctx, r.vector.String(), ts)
	if err != nil {
		return nil, err
	}

	// Create pending alerts for any new vector elements in the alert expression
	// or update the expression value for existing elements.
	resultFPs := map[uint64]struct{}{}

	var vec promql.Vector
	alerts := make(map[uint64]*Alert, len(res))
	for _, smpl := range res {
		// Provide the alert information to the template.
		l := smpl.Metric.Map()

		tmplData := template.AlertTemplateData(l, r.externalLabels, r.externalURL, smpl.V)
		// Inject some convenience variables that are easier to remember for users
		// who are not used to Go's templating system.
		defs := []string{
			"{{$labels := .Labels}}",
			"{{$externalLabels := .ExternalLabels}}",
			"{{$externalURL := .ExternalURL}}",
			"{{$value := .Value}}",
		}

		expand := func(text string) string {
			tmpl := template.NewTemplateExpander(
				ctx,
				strings.Join(append(defs, text), ""),
				"__alert_"+r.Name(),
				tmplData,
				model.Time(timestamp.FromTime(ts)),
				template.QueryFunc(query),
				externalURL,
				nil,
			)
			result, err := tmpl.Expand()
			if err != nil {
				result = fmt.Sprintf("<error expanding template: %s>", err)
				level.Warn(r.logger).Log("msg", "Expanding alert template failed", "err", err, "data", tmplData)
			}
			return result
		}

		lb := labels.NewBuilder(smpl.Metric).Del(labels.MetricName)

		r.labels.Range(func(l labels.Label) {
			lb.Set(l.Name, expand(l.Value))
		})
		lb.Set(labels.AlertName, r.Name())

		sb := labels.ScratchBuilder{}
		r.annotations.Range(func(a labels.Label) {
			sb.Add(a.Name, expand(a.Value))
		})
		annotations := sb.Labels()

		lbs := lb.Labels(labels.EmptyLabels())
		h := lbs.Hash()
		resultFPs[h] = struct{}{}

		if _, ok := alerts[h]; ok {
			return nil, fmt.Errorf("vector contains metrics with the same labelset after applying alert labels")
		}

		alerts[h] = &Alert{
			Labels:      lbs,
			Annotations: annotations,
			ActiveAt:    ts,
			State:       StatePending,
			Value:       smpl.V,
		}
	}

	r.activeMtx.Lock()
	defer r.activeMtx.Unlock()

	for h, a := range alerts {
		// Check whether we already have alerting state for the identifying label set.
		// Update the last value and annotations if so, create a new alert entry otherwise.
		if alert, ok := r.active[h]; ok && alert.State != StateInactive {
			alert.Value = a.Value
			alert.Annotations = a.Annotations
			continue
		}

		r.active[h] = a
	}

	var numActivePending int
	// Check if any pending alerts should be removed or fire now. Write out alert timeseries.
	for fp, a := range r.active {
		if _, ok := resultFPs[fp]; !ok {
			// There is no firing alerts for this fingerprint. The alert is no
			// longer firing.

			// Use keepFiringFor value to determine if the alert should keep
			// firing.
			var keepFiring bool
			if a.State == StateFiring && r.keepFiringFor > 0 {
				if a.KeepFiringSince.IsZero() {
					a.KeepFiringSince = ts
				}
				if ts.Sub(a.KeepFiringSince) < r.keepFiringFor {
					keepFiring = true
				}
			}

			// If the alert was previously firing, keep it around for a given
			// retention time so it is reported as resolved to the AlertManager.
			if a.State == StatePending || (!a.ResolvedAt.IsZero() && ts.Sub(a.ResolvedAt) > resolvedRetention) {
				delete(r.active, fp)
			}
			if a.State != StateInactive && !keepFiring {
				a.State = StateInactive
				a.ResolvedAt = ts
			}
			if !keepFiring {
				continue
			}
		} else {
			// The alert is firing, reset keepFiringSince.
			a.KeepFiringSince = time.Time{}
		}
		numActivePending++

		if a.State == StatePending && ts.Sub(a.ActiveAt) >= r.holdDuration {
			a.State = StateFiring
			a.FiredAt = ts
		}

		if r.restored.Load() {
			vec = append(vec, r.sample(a, ts))
			vec = append(vec, r.forStateSample(a, ts, float64(a.ActiveAt.Unix())))
		}
	}

	if limit > 0 && numActivePending > limit {
		r.active = map[uint64]*Alert{}
		return nil, fmt.Errorf("exceeded limit of %d with %d alerts", limit, numActivePending)
	}

	return vec, nil
}

// State returns the maximum state of alert instances for this rule.
// StateFiring > StatePending > StateInactive
func (r *AlertingRule) State() AlertState {
	r.activeMtx.Lock()
	defer r.activeMtx.Unlock()

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
	r.activeMtx.Lock()
	defer r.activeMtx.Unlock()

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
	r.activeMtx.Lock()
	defer r.activeMtx.Unlock()

	for _, a := range r.active {
		f(a)
	}
}

func (r *AlertingRule) sendAlerts(ctx context.Context, ts time.Time, resendDelay, interval time.Duration, notifyFunc NotifyFunc) {
	alerts := []*Alert{}
	r.ForEachActiveAlert(func(alert *Alert) {
		if alert.needsSending(ts, resendDelay) {
			alert.LastSentAt = ts
			// Allow for two Eval or Alertmanager send failures.
			delta := resendDelay
			if interval > resendDelay {
				delta = interval
			}
			alert.ValidUntil = ts.Add(4 * delta)
			anew := *alert
			// The notifier re-uses the labels slice, hence make a copy.
			anew.Labels = alert.Labels.Copy()
			alerts = append(alerts, &anew)
		}
	})
	notifyFunc(ctx, r.vector.String(), alerts...)
}

func (r *AlertingRule) String() string {
	ar := rulefmt.Rule{
		Alert:         r.name,
		Expr:          r.vector.String(),
		For:           model.Duration(r.holdDuration),
		KeepFiringFor: model.Duration(r.keepFiringFor),
		Labels:        r.labels.Map(),
		Annotations:   r.annotations.Map(),
	}

	byt, err := yaml.Marshal(ar)
	if err != nil {
		return fmt.Sprintf("error marshaling alerting rule: %s", err.Error())
	}

	return string(byt)
}
