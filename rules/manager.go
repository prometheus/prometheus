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
	"math"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// RuleHealth describes the health state of a rule.
type RuleHealth string

// The possible health states of a rule based on the last execution.
const (
	HealthUnknown RuleHealth = "unknown"
	HealthGood    RuleHealth = "ok"
	HealthBad     RuleHealth = "err"
)

// Constants for instrumentation.
const namespace = "prometheus"

// Metrics for rule evaluation.
type Metrics struct {
	EvalDuration        prometheus.Summary
	IterationDuration   prometheus.Summary
	IterationsMissed    *prometheus.CounterVec
	IterationsScheduled *prometheus.CounterVec
	EvalTotal           *prometheus.CounterVec
	EvalFailures        *prometheus.CounterVec
	GroupInterval       *prometheus.GaugeVec
	GroupLastEvalTime   *prometheus.GaugeVec
	GroupLastDuration   *prometheus.GaugeVec
	GroupRules          *prometheus.GaugeVec
	GroupSamples        *prometheus.GaugeVec
}

// NewGroupMetrics creates a new instance of Metrics and registers it with the provided registerer,
// if not nil.
func NewGroupMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		EvalDuration: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace:  namespace,
				Name:       "rule_evaluation_duration_seconds",
				Help:       "The duration for a rule to execute.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			}),
		IterationDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "rule_group_duration_seconds",
			Help:       "The duration of rule group evaluations.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}),
		IterationsMissed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_group_iterations_missed_total",
				Help:      "The total number of rule group evaluations missed due to slow rule group evaluation.",
			},
			[]string{"rule_group"},
		),
		IterationsScheduled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_group_iterations_total",
				Help:      "The total number of scheduled rule group evaluations, whether executed or missed.",
			},
			[]string{"rule_group"},
		),
		EvalTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_evaluations_total",
				Help:      "The total number of rule evaluations.",
			},
			[]string{"rule_group"},
		),
		EvalFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rule_evaluation_failures_total",
				Help:      "The total number of rule evaluation failures.",
			},
			[]string{"rule_group"},
		),
		GroupInterval: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_interval_seconds",
				Help:      "The interval of a rule group.",
			},
			[]string{"rule_group"},
		),
		GroupLastEvalTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_evaluation_timestamp_seconds",
				Help:      "The timestamp of the last rule group evaluation in seconds.",
			},
			[]string{"rule_group"},
		),
		GroupLastDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_duration_seconds",
				Help:      "The duration of the last rule group evaluation.",
			},
			[]string{"rule_group"},
		),
		GroupRules: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_rules",
				Help:      "The number of rules.",
			},
			[]string{"rule_group"},
		),
		GroupSamples: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rule_group_last_evaluation_samples",
				Help:      "The number of samples returned during the last rule group evaluation.",
			},
			[]string{"rule_group"},
		),
	}

	if reg != nil {
		reg.MustRegister(
			m.EvalDuration,
			m.IterationDuration,
			m.IterationsMissed,
			m.IterationsScheduled,
			m.EvalTotal,
			m.EvalFailures,
			m.GroupInterval,
			m.GroupLastEvalTime,
			m.GroupLastDuration,
			m.GroupRules,
			m.GroupSamples,
		)
	}

	return m
}

// QueryFunc processes PromQL queries.
type QueryFunc func(ctx context.Context, q string, t time.Time) (promql.Vector, error)

// EngineQueryFunc returns a new query function that executes instant queries against
// the given engine.
// It converts scalar into vector results.
func EngineQueryFunc(engine *promql.Engine, q storage.Queryable) QueryFunc {
	return func(ctx context.Context, qs string, t time.Time) (promql.Vector, error) {
		q, err := engine.NewInstantQuery(q, nil, qs, t)
		if err != nil {
			return nil, err
		}
		res := q.Exec(ctx)
		if res.Err != nil {
			return nil, res.Err
		}
		switch v := res.Value.(type) {
		case promql.Vector:
			return v, nil
		case promql.Scalar:
			return promql.Vector{promql.Sample{
				Point:  promql.Point(v),
				Metric: labels.Labels{},
			}}, nil
		default:
			return nil, errors.New("rule result is not a vector or scalar")
		}
	}
}

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	Name() string
	// Labels of the rule.
	Labels() labels.Labels
	// Eval evaluates the rule, including any associated recording or alerting actions.
	// The duration passed is the evaluation delay.
	Eval(context.Context, time.Duration, time.Time, QueryFunc, *url.URL, int) (promql.Vector, error)
	// String returns a human-readable string representation of the rule.
	String() string
	// Query returns the rule query expression.
	Query() parser.Expr
	// SetLastErr sets the current error experienced by the rule.
	SetLastError(error)
	// LastErr returns the last error experienced by the rule.
	LastError() error
	// SetHealth sets the current health of the rule.
	SetHealth(RuleHealth)
	// Health returns the current health of the rule.
	Health() RuleHealth
	SetEvaluationDuration(time.Duration)
	// GetEvaluationDuration returns last evaluation duration.
	// NOTE: Used dynamically by rules.html template.
	GetEvaluationDuration() time.Duration
	SetEvaluationTimestamp(time.Time)
	// GetEvaluationTimestamp returns last evaluation timestamp.
	// NOTE: Used dynamically by rules.html template.
	GetEvaluationTimestamp() time.Time
}

// Group is a set of rules that have a logical relation.
type Group struct {
	name                 string
	file                 string
	interval             time.Duration
	evaluationDelay      *time.Duration
	limit                int
	rules                []Rule
	sourceTenants        []string
	seriesInPreviousEval []map[string]labels.Labels // One per Rule.
	staleSeries          []labels.Labels
	opts                 *ManagerOptions
	mtx                  sync.Mutex
	evaluationTime       time.Duration
	lastEvaluation       time.Time

	shouldRestore bool

	markStale   bool
	done        chan struct{}
	terminated  chan struct{}
	managerDone chan struct{}

	logger log.Logger

	metrics *Metrics

	ruleGroupPostProcessFunc RuleGroupPostProcessFunc
}

// This function will be used before each rule group evaluation if not nil.
// Use this function type if the rule group post processing is needed.
type RuleGroupPostProcessFunc func(g *Group, lastEvalTimestamp time.Time, log log.Logger) error

type GroupOptions struct {
	Name, File               string
	Interval                 time.Duration
	Limit                    int
	Rules                    []Rule
	SourceTenants            []string
	ShouldRestore            bool
	Opts                     *ManagerOptions
	EvaluationDelay          *time.Duration
	done                     chan struct{}
	RuleGroupPostProcessFunc RuleGroupPostProcessFunc
}

// NewGroup makes a new Group with the given name, options, and rules.
func NewGroup(o GroupOptions) *Group {
	metrics := o.Opts.Metrics
	if metrics == nil {
		metrics = NewGroupMetrics(o.Opts.Registerer)
	}

	key := GroupKey(o.File, o.Name)
	metrics.IterationsMissed.WithLabelValues(key)
	metrics.IterationsScheduled.WithLabelValues(key)
	metrics.EvalTotal.WithLabelValues(key)
	metrics.EvalFailures.WithLabelValues(key)
	metrics.GroupLastEvalTime.WithLabelValues(key)
	metrics.GroupLastDuration.WithLabelValues(key)
	metrics.GroupRules.WithLabelValues(key).Set(float64(len(o.Rules)))
	metrics.GroupSamples.WithLabelValues(key)
	metrics.GroupInterval.WithLabelValues(key).Set(o.Interval.Seconds())

	return &Group{
		name:                     o.Name,
		file:                     o.File,
		interval:                 o.Interval,
		evaluationDelay:          o.EvaluationDelay,
		limit:                    o.Limit,
		rules:                    o.Rules,
		shouldRestore:            o.ShouldRestore,
		opts:                     o.Opts,
		sourceTenants:            o.SourceTenants,
		seriesInPreviousEval:     make([]map[string]labels.Labels, len(o.Rules)),
		done:                     make(chan struct{}),
		managerDone:              o.done,
		terminated:               make(chan struct{}),
		logger:                   log.With(o.Opts.Logger, "file", o.File, "group", o.Name),
		metrics:                  metrics,
		ruleGroupPostProcessFunc: o.RuleGroupPostProcessFunc,
	}
}

// Name returns the group name.
func (g *Group) Name() string { return g.name }

// File returns the group's file.
func (g *Group) File() string { return g.file }

// Rules returns the group's rules.
func (g *Group) Rules() []Rule { return g.rules }

// Queryable returns the group's querable.
func (g *Group) Queryable() storage.Queryable { return g.opts.Queryable }

// Context returns the group's context.
func (g *Group) Context() context.Context { return g.opts.Context }

// Interval returns the group's interval.
func (g *Group) Interval() time.Duration { return g.interval }

// Limit returns the group's limit.
func (g *Group) Limit() int { return g.limit }

// SourceTenants returns the source tenants for the group.
// If it's empty or nil, then the owning user/tenant is considered to be the source tenant.
func (g *Group) SourceTenants() []string { return g.sourceTenants }

func (g *Group) run(ctx context.Context) {
	defer close(g.terminated)

	// Wait an initial amount to have consistently slotted intervals.
	evalTimestamp := g.EvalTimestamp(time.Now().UnixNano()).Add(g.interval)
	select {
	case <-time.After(time.Until(evalTimestamp)):
	case <-g.done:
		return
	}

	ctx = promql.NewOriginContext(ctx, map[string]interface{}{
		"ruleGroup": map[string]string{
			"file": g.File(),
			"name": g.Name(),
		},
	})

	iter := func() {
		g.metrics.IterationsScheduled.WithLabelValues(GroupKey(g.file, g.name)).Inc()

		start := time.Now()
		g.Eval(ctx, evalTimestamp)
		timeSinceStart := time.Since(start)

		g.metrics.IterationDuration.Observe(timeSinceStart.Seconds())
		g.setEvaluationTime(timeSinceStart)
		g.setLastEvaluation(start)
	}

	// The assumption here is that since the ticker was started after having
	// waited for `evalTimestamp` to pass, the ticks will trigger soon
	// after each `evalTimestamp + N * g.interval` occurrence.
	tick := time.NewTicker(g.interval)
	defer tick.Stop()

	defer func() {
		if !g.markStale {
			return
		}
		go func(now time.Time) {
			for _, rule := range g.seriesInPreviousEval {
				for _, r := range rule {
					g.staleSeries = append(g.staleSeries, r)
				}
			}
			// That can be garbage collected at this point.
			g.seriesInPreviousEval = nil
			// Wait for 2 intervals to give the opportunity to renamed rules
			// to insert new series in the tsdb. At this point if there is a
			// renamed rule, it should already be started.
			select {
			case <-g.managerDone:
			case <-time.After(2 * g.interval):
				g.cleanupStaleSeries(ctx, now)
			}
		}(time.Now())
	}()

	iter()
	if g.shouldRestore {
		// If we have to restore, we wait for another Eval to finish.
		// The reason behind this is, during first eval (or before it)
		// we might not have enough data scraped, and recording rules would not
		// have updated the latest values, on which some alerts might depend.
		select {
		case <-g.done:
			return
		case <-tick.C:
			missed := (time.Since(evalTimestamp) / g.interval) - 1
			if missed > 0 {
				g.metrics.IterationsMissed.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
				g.metrics.IterationsScheduled.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
			}
			evalTimestamp = evalTimestamp.Add((missed + 1) * g.interval)
			iter()
		}

		g.RestoreForState(time.Now())
		g.shouldRestore = false
	}

	for {
		select {
		case <-g.done:
			return
		default:
			select {
			case <-g.done:
				return
			case <-tick.C:
				missed := (time.Since(evalTimestamp) / g.interval) - 1
				if missed > 0 {
					g.metrics.IterationsMissed.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
					g.metrics.IterationsScheduled.WithLabelValues(GroupKey(g.file, g.name)).Add(float64(missed))
				}
				evalTimestamp = evalTimestamp.Add((missed + 1) * g.interval)

				useRuleGroupPostProcessFunc(g, evalTimestamp.Add(-(missed+1)*g.interval))

				iter()
			}
		}
	}
}

func useRuleGroupPostProcessFunc(g *Group, lastEvalTimestamp time.Time) {
	if g.ruleGroupPostProcessFunc != nil {
		err := g.ruleGroupPostProcessFunc(g, lastEvalTimestamp, g.logger)
		if err != nil {
			level.Warn(g.logger).Log("msg", "ruleGroupPostProcessFunc failed", "err", err)
		}
	}
}

func (g *Group) stop() {
	close(g.done)
	<-g.terminated
}

func (g *Group) hash() uint64 {
	l := labels.New(
		labels.Label{Name: "name", Value: g.name},
		labels.Label{Name: "file", Value: g.file},
	)
	return l.Hash()
}

// AlertingRules returns the list of the group's alerting rules.
func (g *Group) AlertingRules() []*AlertingRule {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	var alerts []*AlertingRule
	for _, rule := range g.rules {
		if alertingRule, ok := rule.(*AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].State() > alerts[j].State() ||
			(alerts[i].State() == alerts[j].State() &&
				alerts[i].Name() < alerts[j].Name())
	})
	return alerts
}

// HasAlertingRules returns true if the group contains at least one AlertingRule.
func (g *Group) HasAlertingRules() bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	for _, rule := range g.rules {
		if _, ok := rule.(*AlertingRule); ok {
			return true
		}
	}
	return false
}

// GetEvaluationTime returns the time in seconds it took to evaluate the rule group.
func (g *Group) GetEvaluationTime() time.Duration {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.evaluationTime
}

// setEvaluationTime sets the time in seconds the last evaluation took.
func (g *Group) setEvaluationTime(dur time.Duration) {
	g.metrics.GroupLastDuration.WithLabelValues(GroupKey(g.file, g.name)).Set(dur.Seconds())

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.evaluationTime = dur
}

// GetLastEvaluation returns the time the last evaluation of the rule group took place.
func (g *Group) GetLastEvaluation() time.Time {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.lastEvaluation
}

// setLastEvaluation updates evaluationTimestamp to the timestamp of when the rule group was last evaluated.
func (g *Group) setLastEvaluation(ts time.Time) {
	g.metrics.GroupLastEvalTime.WithLabelValues(GroupKey(g.file, g.name)).Set(float64(ts.UnixNano()) / 1e9)

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.lastEvaluation = ts
}

// EvalTimestamp returns the immediately preceding consistently slotted evaluation time.
func (g *Group) EvalTimestamp(startTime int64) time.Time {
	var (
		offset = int64(g.hash() % uint64(g.interval))
		adjNow = startTime - offset
		base   = adjNow - (adjNow % int64(g.interval))
	)

	return time.Unix(0, base+offset).UTC()
}

func nameAndLabels(rule Rule) string {
	return rule.Name() + rule.Labels().String()
}

// CopyState copies the alerting rule and staleness related state from the given group.
//
// Rules are matched based on their name and labels. If there are duplicates, the
// first is matched with the first, second with the second etc.
func (g *Group) CopyState(from *Group) {
	g.evaluationTime = from.evaluationTime
	g.lastEvaluation = from.lastEvaluation

	ruleMap := make(map[string][]int, len(from.rules))

	for fi, fromRule := range from.rules {
		nameAndLabels := nameAndLabels(fromRule)
		l := ruleMap[nameAndLabels]
		ruleMap[nameAndLabels] = append(l, fi)
	}

	for i, rule := range g.rules {
		nameAndLabels := nameAndLabels(rule)
		indexes := ruleMap[nameAndLabels]
		if len(indexes) == 0 {
			continue
		}
		fi := indexes[0]
		g.seriesInPreviousEval[i] = from.seriesInPreviousEval[fi]
		ruleMap[nameAndLabels] = indexes[1:]

		ar, ok := rule.(*AlertingRule)
		if !ok {
			continue
		}
		far, ok := from.rules[fi].(*AlertingRule)
		if !ok {
			continue
		}

		for fp, a := range far.active {
			ar.active[fp] = a
		}
	}

	// Handle deleted and unmatched duplicate rules.
	g.staleSeries = from.staleSeries
	for fi, fromRule := range from.rules {
		nameAndLabels := nameAndLabels(fromRule)
		l := ruleMap[nameAndLabels]
		if len(l) != 0 {
			for _, series := range from.seriesInPreviousEval[fi] {
				g.staleSeries = append(g.staleSeries, series)
			}
		}
	}
}

// Eval runs a single evaluation cycle in which all rules are evaluated sequentially.
func (g *Group) Eval(ctx context.Context, ts time.Time) {
	var samplesTotal float64
	evaluationDelay := g.EvaluationDelay()
	for i, rule := range g.rules {
		select {
		case <-g.done:
			return
		default:
		}

		func(i int, rule Rule) {
			ctx, sp := otel.Tracer("").Start(ctx, "rule")
			sp.SetAttributes(attribute.String("name", rule.Name()))
			defer func(t time.Time) {
				sp.End()

				since := time.Since(t)
				g.metrics.EvalDuration.Observe(since.Seconds())
				rule.SetEvaluationDuration(since)
				rule.SetEvaluationTimestamp(t)
			}(time.Now())

			g.metrics.EvalTotal.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

			vector, err := rule.Eval(ctx, evaluationDelay, ts, g.opts.QueryFunc, g.opts.ExternalURL, g.Limit())
			if err != nil {
				rule.SetHealth(HealthBad)
				rule.SetLastError(err)
				sp.SetStatus(codes.Error, err.Error())
				g.metrics.EvalFailures.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

				// Canceled queries are intentional termination of queries. This normally
				// happens on shutdown and thus we skip logging of any errors here.
				if _, ok := err.(promql.ErrQueryCanceled); !ok {
					level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Evaluating rule failed", "rule", rule, "err", err)
				}
				return
			}
			rule.SetHealth(HealthGood)
			rule.SetLastError(nil)
			samplesTotal += float64(len(vector))

			if ar, ok := rule.(*AlertingRule); ok {
				ar.sendAlerts(ctx, ts, g.opts.ResendDelay, g.interval, g.opts.NotifyFunc)
			}
			var (
				numOutOfOrder = 0
				numDuplicates = 0
			)

			app := g.opts.Appendable.Appender(ctx)
			seriesReturned := make(map[string]labels.Labels, len(g.seriesInPreviousEval[i]))
			defer func() {
				if err := app.Commit(); err != nil {
					rule.SetHealth(HealthBad)
					rule.SetLastError(err)
					sp.SetStatus(codes.Error, err.Error())
					g.metrics.EvalFailures.WithLabelValues(GroupKey(g.File(), g.Name())).Inc()

					level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Rule sample appending failed", "err", err)
					return
				}
				g.seriesInPreviousEval[i] = seriesReturned
			}()

			for _, s := range vector {
				if _, err := app.Append(0, s.Metric, s.T, s.V); err != nil {
					rule.SetHealth(HealthBad)
					rule.SetLastError(err)
					sp.SetStatus(codes.Error, err.Error())

					switch errors.Cause(err) {
					case storage.ErrOutOfOrderSample:
						numOutOfOrder++
						level.Debug(g.logger).Log("name", rule.Name(), "index", i, "msg", "Rule evaluation result discarded", "err", err, "sample", s)
					case storage.ErrDuplicateSampleForTimestamp:
						numDuplicates++
						level.Debug(g.logger).Log("name", rule.Name(), "index", i, "msg", "Rule evaluation result discarded", "err", err, "sample", s)
					default:
						level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Rule evaluation result discarded", "err", err, "sample", s)
					}
				} else {
					buf := [1024]byte{}
					seriesReturned[string(s.Metric.Bytes(buf[:]))] = s.Metric
				}
			}
			if numOutOfOrder > 0 {
				level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Error on ingesting out-of-order result from rule evaluation", "numDropped", numOutOfOrder)
			}
			if numDuplicates > 0 {
				level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Error on ingesting results from rule evaluation with different value but same timestamp", "numDropped", numDuplicates)
			}

			for metric, lset := range g.seriesInPreviousEval[i] {
				if _, ok := seriesReturned[metric]; !ok {
					// Series no longer exposed, mark it stale.
					_, err = app.Append(0, lset, timestamp.FromTime(ts.Add(-evaluationDelay)), math.Float64frombits(value.StaleNaN))
					switch errors.Cause(err) {
					case nil:
					case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
						// Do not count these in logging, as this is expected if series
						// is exposed from a different rule.
					default:
						level.Warn(g.logger).Log("name", rule.Name(), "index", i, "msg", "Adding stale sample failed", "sample", lset.String(), "err", err)
					}
				}
			}
		}(i, rule)
	}
	if g.metrics != nil {
		g.metrics.GroupSamples.WithLabelValues(GroupKey(g.File(), g.Name())).Set(samplesTotal)
	}
	g.cleanupStaleSeries(ctx, ts)
}

func (g *Group) EvaluationDelay() time.Duration {
	if g.evaluationDelay != nil {
		return *g.evaluationDelay
	}
	if g.opts.DefaultEvaluationDelay != nil {
		return g.opts.DefaultEvaluationDelay()
	}
	return time.Duration(0)
}

func (g *Group) cleanupStaleSeries(ctx context.Context, ts time.Time) {
	if len(g.staleSeries) == 0 {
		return
	}
	app := g.opts.Appendable.Appender(ctx)
	evaluationDelay := g.EvaluationDelay()
	for _, s := range g.staleSeries {
		// Rule that produced series no longer configured, mark it stale.
		_, err := app.Append(0, s, timestamp.FromTime(ts.Add(-evaluationDelay)), math.Float64frombits(value.StaleNaN))
		switch errors.Cause(err) {
		case nil:
		case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
			// Do not count these in logging, as this is expected if series
			// is exposed from a different rule.
		default:
			level.Warn(g.logger).Log("msg", "Adding stale sample for previous configuration failed", "sample", s, "err", err)
		}
	}
	if err := app.Commit(); err != nil {
		level.Warn(g.logger).Log("msg", "Stale sample appending for previous configuration failed", "err", err)
	} else {
		g.staleSeries = nil
	}
}

// RestoreForState restores the 'for' state of the alerts
// by looking up last ActiveAt from storage.
func (g *Group) RestoreForState(ts time.Time) {
	maxtMS := int64(model.TimeFromUnixNano(ts.UnixNano()))
	// We allow restoration only if alerts were active before after certain time.
	mint := ts.Add(-g.opts.OutageTolerance)
	mintMS := int64(model.TimeFromUnixNano(mint.UnixNano()))
	q, err := g.opts.Queryable.Querier(g.opts.Context, mintMS, maxtMS)
	if err != nil {
		level.Error(g.logger).Log("msg", "Failed to get Querier", "err", err)
		return
	}
	defer func() {
		if err := q.Close(); err != nil {
			level.Error(g.logger).Log("msg", "Failed to close Querier", "err", err)
		}
	}()

	for _, rule := range g.Rules() {
		alertRule, ok := rule.(*AlertingRule)
		if !ok {
			continue
		}

		alertHoldDuration := alertRule.HoldDuration()
		if alertHoldDuration < g.opts.ForGracePeriod {
			// If alertHoldDuration is already less than grace period, we would not
			// like to make it wait for `g.opts.ForGracePeriod` time before firing.
			// Hence we skip restoration, which will make it wait for alertHoldDuration.
			alertRule.SetRestored(true)
			continue
		}

		alertRule.ForEachActiveAlert(func(a *Alert) {
			var s storage.Series

			s, err := alertRule.QueryforStateSeries(a, q)
			if err != nil {
				// Querier Warnings are ignored. We do not care unless we have an error.
				level.Error(g.logger).Log(
					"msg", "Failed to restore 'for' state",
					labels.AlertName, alertRule.Name(),
					"stage", "Select",
					"err", err,
				)
				return
			}

			if s == nil {
				return
			}

			// Series found for the 'for' state.
			var t int64
			var v float64
			it := s.Iterator()
			for it.Next() {
				t, v = it.At()
			}
			if it.Err() != nil {
				level.Error(g.logger).Log("msg", "Failed to restore 'for' state",
					labels.AlertName, alertRule.Name(), "stage", "Iterator", "err", it.Err())
				return
			}
			if value.IsStaleNaN(v) { // Alert was not active.
				return
			}

			downAt := time.Unix(t/1000, 0).UTC()
			restoredActiveAt := time.Unix(int64(v), 0).UTC()
			timeSpentPending := downAt.Sub(restoredActiveAt)
			timeRemainingPending := alertHoldDuration - timeSpentPending

			if timeRemainingPending <= 0 {
				// It means that alert was firing when prometheus went down.
				// In the next Eval, the state of this alert will be set back to
				// firing again if it's still firing in that Eval.
				// Nothing to be done in this case.
			} else if timeRemainingPending < g.opts.ForGracePeriod {
				// (new) restoredActiveAt = (ts + m.opts.ForGracePeriod) - alertHoldDuration
				//                            /* new firing time */      /* moving back by hold duration */
				//
				// Proof of correctness:
				// firingTime = restoredActiveAt.Add(alertHoldDuration)
				//            = ts + m.opts.ForGracePeriod - alertHoldDuration + alertHoldDuration
				//            = ts + m.opts.ForGracePeriod
				//
				// Time remaining to fire = firingTime.Sub(ts)
				//                        = (ts + m.opts.ForGracePeriod) - ts
				//                        = m.opts.ForGracePeriod
				restoredActiveAt = ts.Add(g.opts.ForGracePeriod).Add(-alertHoldDuration)
			} else {
				// By shifting ActiveAt to the future (ActiveAt + some_duration),
				// the total pending time from the original ActiveAt
				// would be `alertHoldDuration + some_duration`.
				// Here, some_duration = downDuration.
				downDuration := ts.Sub(downAt)
				restoredActiveAt = restoredActiveAt.Add(downDuration)
			}

			a.ActiveAt = restoredActiveAt
			level.Debug(g.logger).Log("msg", "'for' state restored",
				labels.AlertName, alertRule.Name(), "restored_time", a.ActiveAt.Format(time.RFC850),
				"labels", a.Labels.String())
		})

		alertRule.SetRestored(true)
	}
}

// Equals return if two groups are the same.
func (g *Group) Equals(ng *Group) bool {
	if g.name != ng.name {
		return false
	}

	if g.file != ng.file {
		return false
	}

	if g.interval != ng.interval {
		return false
	}

	if g.limit != ng.limit {
		return false
	}

	if len(g.rules) != len(ng.rules) {
		return false
	}

	for i, gr := range g.rules {
		if gr.String() != ng.rules[i].String() {
			return false
		}
	}
	{
		// compare source tenants
		if len(g.sourceTenants) != len(ng.sourceTenants) {
			return false
		}

		copyAndSort := func(x []string) []string {
			copied := make([]string, len(x))
			copy(copied, x)
			sort.Strings(copied)
			return copied
		}

		ngSourceTenantsCopy := copyAndSort(ng.sourceTenants)
		gSourceTenantsCopy := copyAndSort(g.sourceTenants)

		for i := range ngSourceTenantsCopy {
			if gSourceTenantsCopy[i] != ngSourceTenantsCopy[i] {
				return false
			}
		}
	}

	return true
}

// The Manager manages recording and alerting rules.
type Manager struct {
	opts     *ManagerOptions
	groups   map[string]*Group
	mtx      sync.RWMutex
	block    chan struct{}
	done     chan struct{}
	restored bool

	logger log.Logger
}

// NotifyFunc sends notifications about a set of alerts generated by the given expression.
type NotifyFunc func(ctx context.Context, expr string, alerts ...*Alert)

type ContextWrapFunc func(ctx context.Context, g *Group) context.Context

// ManagerOptions bundles options for the Manager.
type ManagerOptions struct {
	ExternalURL *url.URL
	QueryFunc   QueryFunc
	NotifyFunc  NotifyFunc
	Context     context.Context
	// GroupEvaluationContextFunc will be called to wrap Context based on the group being evaluated.
	// Will be skipped if nil.
	GroupEvaluationContextFunc ContextWrapFunc
	Appendable                 storage.Appendable
	Queryable                  storage.Queryable
	Logger                     log.Logger
	Registerer                 prometheus.Registerer
	OutageTolerance            time.Duration
	ForGracePeriod             time.Duration
	ResendDelay                time.Duration
	GroupLoader                GroupLoader
	DefaultEvaluationDelay     func() time.Duration

	Metrics *Metrics
}

// NewManager returns an implementation of Manager, ready to be started
// by calling the Run method.
func NewManager(o *ManagerOptions) *Manager {
	if o.Metrics == nil {
		o.Metrics = NewGroupMetrics(o.Registerer)
	}

	if o.GroupLoader == nil {
		o.GroupLoader = FileLoader{}
	}

	m := &Manager{
		groups: map[string]*Group{},
		opts:   o,
		block:  make(chan struct{}),
		done:   make(chan struct{}),
		logger: o.Logger,
	}

	return m
}

// Run starts processing of the rule manager. It is blocking.
func (m *Manager) Run() {
	level.Info(m.logger).Log("msg", "Starting rule manager...")
	m.start()
	<-m.done
}

func (m *Manager) start() {
	close(m.block)
}

// Stop the rule manager's rule evaluation cycles.
func (m *Manager) Stop() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	level.Info(m.logger).Log("msg", "Stopping rule manager...")

	for _, eg := range m.groups {
		eg.stop()
	}

	// Shut down the groups waiting multiple evaluation intervals to write
	// staleness markers.
	close(m.done)

	level.Info(m.logger).Log("msg", "Rule manager stopped")
}

// Update the rule manager's state as the config requires. If
// loading the new rules failed the old rule set is restored.
func (m *Manager) Update(interval time.Duration, files []string, externalLabels labels.Labels, externalURL string, ruleGroupPostProcessFunc RuleGroupPostProcessFunc) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	groups, errs := m.LoadGroups(interval, externalLabels, externalURL, ruleGroupPostProcessFunc, files...)

	if errs != nil {
		for _, e := range errs {
			level.Error(m.logger).Log("msg", "loading groups failed", "err", e)
		}
		return errors.New("error loading rules, previous rule set restored")
	}
	m.restored = true

	var wg sync.WaitGroup
	for _, newg := range groups {
		// If there is an old group with the same identifier,
		// check if new group equals with the old group, if yes then skip it.
		// If not equals, stop it and wait for it to finish the current iteration.
		// Then copy it into the new group.
		gn := GroupKey(newg.file, newg.name)
		oldg, ok := m.groups[gn]
		delete(m.groups, gn)

		if ok && oldg.Equals(newg) {
			groups[gn] = oldg
			continue
		}

		wg.Add(1)
		go func(newg *Group) {
			if ok {
				oldg.stop()
				newg.CopyState(oldg)
			}
			wg.Done()

			ctx := m.opts.Context
			if m.opts.GroupEvaluationContextFunc != nil {
				ctx = m.opts.GroupEvaluationContextFunc(ctx, newg)
			}
			// Wait with starting evaluation until the rule manager
			// is told to run. This is necessary to avoid running
			// queries against a bootstrapping storage.
			<-m.block
			newg.run(ctx)
		}(newg)
	}

	// Stop remaining old groups.
	wg.Add(len(m.groups))
	for n, oldg := range m.groups {
		go func(n string, g *Group) {
			g.markStale = true
			g.stop()
			if m := g.metrics; m != nil {
				m.IterationsMissed.DeleteLabelValues(n)
				m.IterationsScheduled.DeleteLabelValues(n)
				m.EvalTotal.DeleteLabelValues(n)
				m.EvalFailures.DeleteLabelValues(n)
				m.GroupInterval.DeleteLabelValues(n)
				m.GroupLastEvalTime.DeleteLabelValues(n)
				m.GroupLastDuration.DeleteLabelValues(n)
				m.GroupRules.DeleteLabelValues(n)
				m.GroupSamples.DeleteLabelValues((n))
			}
			wg.Done()
		}(n, oldg)
	}

	wg.Wait()
	m.groups = groups

	return nil
}

// GroupLoader is responsible for loading rule groups from arbitrary sources and parsing them.
type GroupLoader interface {
	Load(identifier string) (*rulefmt.RuleGroups, []error)
	Parse(query string) (parser.Expr, error)
}

// FileLoader is the default GroupLoader implementation. It defers to rulefmt.ParseFile
// and parser.ParseExpr
type FileLoader struct{}

func (FileLoader) Load(identifier string) (*rulefmt.RuleGroups, []error) {
	return rulefmt.ParseFile(identifier)
}

func (FileLoader) Parse(query string) (parser.Expr, error) { return parser.ParseExpr(query) }

// LoadGroups reads groups from a list of files.
func (m *Manager) LoadGroups(
	interval time.Duration, externalLabels labels.Labels, externalURL string, ruleGroupPostProcessFunc RuleGroupPostProcessFunc, filenames ...string,
) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	shouldRestore := !m.restored

	for _, fn := range filenames {
		rgs, errs := m.opts.GroupLoader.Load(fn)
		if errs != nil {
			return nil, errs
		}

		for _, rg := range rgs.Groups {
			itv := interval
			if rg.Interval != 0 {
				itv = time.Duration(rg.Interval)
			}

			rules := make([]Rule, 0, len(rg.Rules))
			for _, r := range rg.Rules {
				expr, err := m.opts.GroupLoader.Parse(r.Expr.Value)
				if err != nil {
					return nil, []error{errors.Wrap(err, fn)}
				}

				if r.Alert.Value != "" {
					rules = append(rules, NewAlertingRule(
						r.Alert.Value,
						expr,
						time.Duration(r.For),
						labels.FromMap(r.Labels),
						labels.FromMap(r.Annotations),
						externalLabels,
						externalURL,
						m.restored,
						log.With(m.logger, "alert", r.Alert),
					))
					continue
				}
				rules = append(rules, NewRecordingRule(
					r.Record.Value,
					expr,
					labels.FromMap(r.Labels),
				))
			}

			groups[GroupKey(fn, rg.Name)] = NewGroup(GroupOptions{
				Name:                     rg.Name,
				File:                     fn,
				Interval:                 itv,
				Limit:                    rg.Limit,
				Rules:                    rules,
				SourceTenants:            rg.SourceTenants,
				ShouldRestore:            shouldRestore,
				Opts:                     m.opts,
				EvaluationDelay:          (*time.Duration)(rg.EvaluationDelay),
				done:                     m.done,
				RuleGroupPostProcessFunc: ruleGroupPostProcessFunc,
			})
		}
	}

	return groups, nil
}

// GroupKey group names need not be unique across filenames.
func GroupKey(file, name string) string {
	return file + ";" + name
}

// RuleGroups returns the list of manager's rule groups.
func (m *Manager) RuleGroups() []*Group {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	rgs := make([]*Group, 0, len(m.groups))
	for _, g := range m.groups {
		rgs = append(rgs, g)
	}

	sort.Slice(rgs, func(i, j int) bool {
		if rgs[i].file != rgs[j].file {
			return rgs[i].file < rgs[j].file
		}
		return rgs[i].name < rgs[j].name
	})

	return rgs
}

// Rules returns the list of the manager's rules.
func (m *Manager) Rules() []Rule {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	var rules []Rule
	for _, g := range m.groups {
		rules = append(rules, g.rules...)
	}

	return rules
}

// AlertingRules returns the list of the manager's alerting rules.
func (m *Manager) AlertingRules() []*AlertingRule {
	alerts := []*AlertingRule{}
	for _, rule := range m.Rules() {
		if alertingRule, ok := rule.(*AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}

	return alerts
}
