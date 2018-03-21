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
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"sync"
	"time"

	html_template "html/template"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

// Constants for instrumentation.
const namespace = "prometheus"

var (
	evalDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_duration_seconds",
			Help:      "The duration for a rule to execute.",
		},
	)
	evalFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_failures_total",
			Help:      "The total number of rule evaluation failures.",
		},
	)
	evalTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_evaluations_total",
			Help:      "The total number of rule evaluations.",
		},
	)
	iterationDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:  namespace,
		Name:       "rule_group_duration_seconds",
		Help:       "The duration of rule group evaluations.",
		Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
	})
	iterationsMissed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rule_group_iterations_missed_total",
		Help:      "The total number of rule group evaluations missed due to slow rule group evaluation.",
	})
	iterationsScheduled = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rule_group_iterations_total",
		Help:      "The total number of scheduled rule group evaluations, whether executed or missed.",
	})
	lastDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "rule_group_last_duration_seconds"),
		"The duration of the last rule group evaulation.",
		[]string{"rule_group"},
		nil,
	)
	groupInterval = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "rule_group_interval_seconds"),
		"The interval of a rule group.",
		[]string{"rule_group"},
		nil,
	)
)

func init() {
	prometheus.MustRegister(iterationDuration)
	prometheus.MustRegister(iterationsScheduled)
	prometheus.MustRegister(iterationsMissed)
	prometheus.MustRegister(evalFailures)
	prometheus.MustRegister(evalDuration)
}

// QueryFunc processes PromQL queries.
type QueryFunc func(ctx context.Context, q string, t time.Time) (promql.Vector, error)

// EngineQueryFunc returns a new query function that executes instant queries against
// the given engine.
// It converts scaler into vector results.
func EngineQueryFunc(engine *promql.Engine, q storage.Queryable) QueryFunc {
	return func(ctx context.Context, qs string, t time.Time) (promql.Vector, error) {
		q, err := engine.NewInstantQuery(q, qs, t)
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
			return nil, fmt.Errorf("rule result is not a vector or scalar")
		}
	}
}

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	Name() string
	// eval evaluates the rule, including any associated recording or alerting actions.
	Eval(context.Context, time.Time, QueryFunc, *url.URL) (promql.Vector, error)
	// String returns a human-readable string representation of the rule.
	String() string

	SetEvaluationTime(time.Duration)
	GetEvaluationTime() time.Duration
	// HTMLSnippet returns a human-readable string representation of the rule,
	// decorated with HTML elements for use the web frontend.
	HTMLSnippet(pathPrefix string) html_template.HTML
}

// Group is a set of rules that have a logical relation.
type Group struct {
	name                 string
	file                 string
	interval             time.Duration
	rules                []Rule
	seriesInPreviousEval []map[string]labels.Labels // One per Rule.
	opts                 *ManagerOptions
	evaluationTime       time.Duration
	mtx                  sync.Mutex

	done       chan struct{}
	terminated chan struct{}

	logger log.Logger
}

// NewGroup makes a new Group with the given name, options, and rules.
func NewGroup(name, file string, interval time.Duration, rules []Rule, opts *ManagerOptions) *Group {
	return &Group{
		name:                 name,
		file:                 file,
		interval:             interval,
		rules:                rules,
		opts:                 opts,
		seriesInPreviousEval: make([]map[string]labels.Labels, len(rules)),
		done:                 make(chan struct{}),
		terminated:           make(chan struct{}),
		logger:               log.With(opts.Logger, "group", name),
	}
}

// Name returns the group name.
func (g *Group) Name() string { return g.name }

// File returns the group's file.
func (g *Group) File() string { return g.file }

// Rules returns the group's rules.
func (g *Group) Rules() []Rule { return g.rules }

func (g *Group) run(ctx context.Context) {
	defer close(g.terminated)

	// Wait an initial amount to have consistently slotted intervals.
	select {
	case <-time.After(g.offset()):
	case <-g.done:
		return
	}

	iter := func() {
		iterationsScheduled.Inc()

		start := time.Now()
		g.Eval(ctx, start)

		iterationDuration.Observe(time.Since(start).Seconds())
		g.SetEvaluationTime(time.Since(start))
	}
	lastTriggered := time.Now()
	iter()

	tick := time.NewTicker(g.interval)
	defer tick.Stop()

	for {
		select {
		case <-g.done:
			return
		default:
			select {
			case <-g.done:
				return
			case <-tick.C:
				missed := (time.Since(lastTriggered).Nanoseconds() / g.interval.Nanoseconds()) - 1
				if missed > 0 {
					iterationsMissed.Add(float64(missed))
					iterationsScheduled.Add(float64(missed))
				}
				lastTriggered = time.Now()
				iter()
			}
		}
	}
}

func (g *Group) stop() {
	close(g.done)
	<-g.terminated
}

func (g *Group) hash() uint64 {
	l := labels.New(
		labels.Label{"name", g.name},
		labels.Label{"file", g.file},
	)
	return l.Hash()
}

// GetEvaluationTime returns the time in seconds it took to evaluate the rule group.
func (g *Group) GetEvaluationTime() time.Duration {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return g.evaluationTime
}

// SetEvaluationTime sets the time in seconds the last evaluation took.
func (g *Group) SetEvaluationTime(dur time.Duration) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.evaluationTime = dur
}

// offset returns until the next consistently slotted evaluation interval.
func (g *Group) offset() time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now - (now % int64(g.interval))
		offset = g.hash() % uint64(g.interval)
		next   = base + int64(offset)
	)

	if next < now {
		next += int64(g.interval)
	}
	return time.Duration(next - now)
}

// copyState copies the alerting rule and staleness related state from the given group.
//
// Rules are matched based on their name. If there are duplicates, the
// first is matched with the first, second with the second etc.
func (g *Group) copyState(from *Group) {
	g.evaluationTime = from.evaluationTime

	ruleMap := make(map[string][]int, len(from.rules))

	for fi, fromRule := range from.rules {
		l := ruleMap[fromRule.Name()]
		ruleMap[fromRule.Name()] = append(l, fi)
	}

	for i, rule := range g.rules {
		indexes := ruleMap[rule.Name()]
		if len(indexes) == 0 {
			continue
		}
		fi := indexes[0]
		g.seriesInPreviousEval[i] = from.seriesInPreviousEval[fi]
		ruleMap[rule.Name()] = indexes[1:]

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
}

// Eval runs a single evaluation cycle in which all rules are evaluated sequentially.
func (g *Group) Eval(ctx context.Context, ts time.Time) {
	for i, rule := range g.rules {
		select {
		case <-g.done:
			return
		default:
		}

		func(i int, rule Rule) {
			defer func(t time.Time) {
				evalDuration.Observe(time.Since(t).Seconds())
				rule.SetEvaluationTime(time.Since(t))
			}(time.Now())

			evalTotal.Inc()

			vector, err := rule.Eval(ctx, ts, g.opts.QueryFunc, g.opts.ExternalURL)
			if err != nil {
				// Canceled queries are intentional termination of queries. This normally
				// happens on shutdown and thus we skip logging of any errors here.
				if _, ok := err.(promql.ErrQueryCanceled); !ok {
					level.Warn(g.logger).Log("msg", "Evaluating rule failed", "rule", rule, "err", err)
				}
				evalFailures.Inc()
				return
			}

			if ar, ok := rule.(*AlertingRule); ok {
				g.opts.NotifyFunc(ctx, ar.vector.String(), ar.currentAlerts()...)
			}
			var (
				numOutOfOrder = 0
				numDuplicates = 0
			)

			app, err := g.opts.Appendable.Appender()
			if err != nil {
				level.Warn(g.logger).Log("msg", "creating appender failed", "err", err)
				return
			}

			seriesReturned := make(map[string]labels.Labels, len(g.seriesInPreviousEval[i]))
			for _, s := range vector {
				if _, err := app.Add(s.Metric, s.T, s.V); err != nil {
					switch err {
					case storage.ErrOutOfOrderSample:
						numOutOfOrder++
						level.Debug(g.logger).Log("msg", "Rule evaluation result discarded", "err", err, "sample", s)
					case storage.ErrDuplicateSampleForTimestamp:
						numDuplicates++
						level.Debug(g.logger).Log("msg", "Rule evaluation result discarded", "err", err, "sample", s)
					default:
						level.Warn(g.logger).Log("msg", "Rule evaluation result discarded", "err", err, "sample", s)
					}
				} else {
					seriesReturned[s.Metric.String()] = s.Metric
				}
			}
			if numOutOfOrder > 0 {
				level.Warn(g.logger).Log("msg", "Error on ingesting out-of-order result from rule evaluation", "numDropped", numOutOfOrder)
			}
			if numDuplicates > 0 {
				level.Warn(g.logger).Log("msg", "Error on ingesting results from rule evaluation with different value but same timestamp", "numDropped", numDuplicates)
			}

			for metric, lset := range g.seriesInPreviousEval[i] {
				if _, ok := seriesReturned[metric]; !ok {
					// Series no longer exposed, mark it stale.
					_, err = app.Add(lset, timestamp.FromTime(ts), math.Float64frombits(value.StaleNaN))
					switch err {
					case nil:
					case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
						// Do not count these in logging, as this is expected if series
						// is exposed from a different rule.
					default:
						level.Warn(g.logger).Log("msg", "adding stale sample failed", "sample", metric, "err", err)
					}
				}
			}
			if err := app.Commit(); err != nil {
				level.Warn(g.logger).Log("msg", "rule sample appending failed", "err", err)
			} else {
				g.seriesInPreviousEval[i] = seriesReturned
			}
		}(i, rule)
	}
}

// The Manager manages recording and alerting rules.
type Manager struct {
	opts   *ManagerOptions
	groups map[string]*Group
	mtx    sync.RWMutex
	block  chan struct{}

	logger log.Logger
}

// Appendable returns an Appender.
type Appendable interface {
	Appender() (storage.Appender, error)
}

// NotifyFunc sends notifications about a set of alerts generated by the given expression.
type NotifyFunc func(ctx context.Context, expr string, alerts ...*Alert) error

// ManagerOptions bundles options for the Manager.
type ManagerOptions struct {
	ExternalURL *url.URL
	QueryFunc   QueryFunc
	NotifyFunc  NotifyFunc
	Context     context.Context
	Appendable  Appendable
	Logger      log.Logger
	Registerer  prometheus.Registerer
}

// NewManager returns an implementation of Manager, ready to be started
// by calling the Run method.
func NewManager(o *ManagerOptions) *Manager {
	m := &Manager{
		groups: map[string]*Group{},
		opts:   o,
		block:  make(chan struct{}),
		logger: o.Logger,
	}
	if o.Registerer != nil {
		o.Registerer.MustRegister(m)
	}
	return m
}

// Run starts processing of the rule manager.
func (m *Manager) Run() {
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

	level.Info(m.logger).Log("msg", "Rule manager stopped")
}

// Update the rule manager's state as the config requires. If
// loading the new rules failed the old rule set is restored.
func (m *Manager) Update(interval time.Duration, files []string) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// To be replaced with a configurable per-group interval.
	groups, errs := m.loadGroups(interval, files...)
	if errs != nil {
		for _, e := range errs {
			level.Error(m.logger).Log("msg", "loading groups failed", "err", e)
		}
		return errors.New("error loading rules, previous rule set restored")
	}

	var wg sync.WaitGroup

	for _, newg := range groups {
		wg.Add(1)

		// If there is an old group with the same identifier, stop it and wait for
		// it to finish the current iteration. Then copy it into the new group.
		gn := groupKey(newg.name, newg.file)
		oldg, ok := m.groups[gn]
		delete(m.groups, gn)

		go func(newg *Group) {
			if ok {
				oldg.stop()
				newg.copyState(oldg)
			}
			go func() {
				// Wait with starting evaluation until the rule manager
				// is told to run. This is necessary to avoid running
				// queries against a bootstrapping storage.
				<-m.block
				newg.run(m.opts.Context)
			}()
			wg.Done()
		}(newg)
	}

	// Stop remaining old groups.
	for _, oldg := range m.groups {
		oldg.stop()
	}

	wg.Wait()
	m.groups = groups

	return nil
}

// loadGroups reads groups from a list of files.
// As there's currently no group syntax a single group named "default" containing
// all rules will be returned.
func (m *Manager) loadGroups(interval time.Duration, filenames ...string) (map[string]*Group, []error) {
	groups := make(map[string]*Group)

	for _, fn := range filenames {
		rgs, errs := rulefmt.ParseFile(fn)
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
				expr, err := promql.ParseExpr(r.Expr)
				if err != nil {
					return nil, []error{err}
				}

				if r.Alert != "" {
					rules = append(rules, NewAlertingRule(
						r.Alert,
						expr,
						time.Duration(r.For),
						labels.FromMap(r.Labels),
						labels.FromMap(r.Annotations),
						log.With(m.logger, "alert", r.Alert),
					))
					continue
				}
				rules = append(rules, NewRecordingRule(
					r.Record,
					expr,
					labels.FromMap(r.Labels),
				))
			}

			groups[groupKey(rg.Name, fn)] = NewGroup(rg.Name, fn, itv, rules, m.opts)
		}
	}

	return groups, nil
}

// Group names need not be unique across filenames.
func groupKey(name, file string) string {
	return name + ";" + file
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
		return rgs[i].file < rgs[j].file && rgs[i].name < rgs[j].name
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
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	alerts := []*AlertingRule{}
	for _, rule := range m.Rules() {
		if alertingRule, ok := rule.(*AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}
	return alerts
}

// Describe implements prometheus.Collector.
func (m *Manager) Describe(ch chan<- *prometheus.Desc) {
	ch <- lastDuration
	ch <- groupInterval
}

// Collect implements prometheus.Collector.
func (m *Manager) Collect(ch chan<- prometheus.Metric) {
	for _, g := range m.RuleGroups() {
		ch <- prometheus.MustNewConstMetric(lastDuration,
			prometheus.GaugeValue,
			g.GetEvaluationTime().Seconds(),
			groupKey(g.file, g.name))
	}
	for _, g := range m.RuleGroups() {
		ch <- prometheus.MustNewConstMetric(groupInterval,
			prometheus.GaugeValue,
			g.interval.Seconds(),
			groupKey(g.file, g.name))
	}
}
