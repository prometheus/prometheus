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
	"io/ioutil"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	html_template "html/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/util/strutil"
)

// Constants for instrumentation.
const namespace = "prometheus"

var (
	evalDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_duration_seconds",
			Help:      "The duration for a rule to execute.",
		},
		[]string{"rule_type"},
	)
	evalFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_failures_total",
			Help:      "The total number of rule evaluation failures.",
		},
		[]string{"rule_type"},
	)
	evalTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_evaluations_total",
			Help:      "The total number of rule evaluations.",
		},
		[]string{"rule_type"},
	)
	iterationDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:  namespace,
		Name:       "evaluator_duration_seconds",
		Help:       "The duration of rule group evaluations.",
		Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
	})
	iterationsSkipped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "evaluator_iterations_skipped_total",
		Help:      "The total number of rule group evaluations skipped due to throttled metric storage.",
	})
	iterationsScheduled = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "evaluator_iterations_total",
		Help:      "The total number of scheduled rule group evaluations, whether skipped or executed.",
	})
)

func init() {
	evalTotal.WithLabelValues(string(ruleTypeAlert))
	evalTotal.WithLabelValues(string(ruleTypeRecording))
	evalFailures.WithLabelValues(string(ruleTypeAlert))
	evalFailures.WithLabelValues(string(ruleTypeRecording))

	prometheus.MustRegister(iterationDuration)
	prometheus.MustRegister(iterationsSkipped)
	prometheus.MustRegister(evalFailures)
	prometheus.MustRegister(evalDuration)
}

type ruleType string

const (
	ruleTypeAlert     = "alerting"
	ruleTypeRecording = "recording"
)

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	Name() string
	// eval evaluates the rule, including any associated recording or alerting actions.
	eval(model.Time, *promql.Engine) (model.Vector, error)
	// String returns a human-readable string representation of the rule.
	String() string
	// HTMLSnippet returns a human-readable string representation of the rule,
	// decorated with HTML elements for use the web frontend.
	HTMLSnippet(pathPrefix string) html_template.HTML
}

// Group is a set of rules that have a logical relation.
type Group struct {
	name     string
	interval time.Duration
	rules    []Rule
	opts     *ManagerOptions

	done       chan struct{}
	terminated chan struct{}
}

func newGroup(name string, opts *ManagerOptions) *Group {
	return &Group{
		name:       name,
		opts:       opts,
		done:       make(chan struct{}),
		terminated: make(chan struct{}),
	}
}

func (g *Group) run() {
	defer close(g.terminated)

	// Wait an initial amount to have consistently slotted intervals.
	select {
	case <-time.After(g.offset()):
	case <-g.done:
		return
	}

	iter := func() {
		iterationsScheduled.Inc()
		if g.opts.SampleAppender.NeedsThrottling() {
			iterationsSkipped.Inc()
			return
		}
		start := time.Now()
		g.eval()

		iterationDuration.Observe(float64(time.Since(start)) / float64(time.Second))
	}
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
				iter()
			}
		}
	}
}

func (g *Group) stop() {
	close(g.done)
	<-g.terminated
}

func (g *Group) fingerprint() model.Fingerprint {
	l := model.LabelSet{"name": model.LabelValue(g.name)}
	return l.Fingerprint()
}

// offset returns until the next consistently slotted evaluation interval.
func (g *Group) offset() time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now - (now % int64(g.interval))
		offset = uint64(g.fingerprint()) % uint64(g.interval)
		next   = base + int64(offset)
	)

	if next < now {
		next += int64(g.interval)
	}
	return time.Duration(next - now)
}

// copyState copies the alerting rule state from the given group.
func (g *Group) copyState(from *Group) {
	for _, fromRule := range from.rules {
		far, ok := fromRule.(*AlertingRule)
		if !ok {
			continue
		}
		for _, rule := range g.rules {
			ar, ok := rule.(*AlertingRule)
			if !ok {
				continue
			}
			// TODO(fabxc): forbid same alert definitions that are not unique by
			// at least on static label or alertname?
			if far.equal(ar) {
				for fp, a := range far.active {
					ar.active[fp] = a
				}
			}
		}
	}
}

func typeForRule(r Rule) ruleType {
	switch r.(type) {
	case *AlertingRule:
		return ruleTypeAlert
	case *RecordingRule:
		return ruleTypeRecording
	}
	panic(fmt.Errorf("unknown rule type: %T", r))
}

// eval runs a single evaluation cycle in which all rules are evaluated in parallel.
// In the future a single group will be evaluated sequentially to properly handle
// rule dependency.
func (g *Group) eval() {
	var (
		now = model.Now()
		wg  sync.WaitGroup
	)

	for _, rule := range g.rules {
		rtyp := string(typeForRule(rule))

		wg.Add(1)
		// BUG(julius): Look at fixing thundering herd.
		go func(rule Rule) {
			defer wg.Done()

			defer func(t time.Time) {
				evalDuration.WithLabelValues(rtyp).Observe(float64(time.Since(t)) / float64(time.Second))
			}(time.Now())

			evalTotal.WithLabelValues(rtyp).Inc()

			vector, err := rule.eval(now, g.opts.QueryEngine)
			if err != nil {
				// Canceled queries are intentional termination of queries. This normally
				// happens on shutdown and thus we skip logging of any errors here.
				if _, ok := err.(promql.ErrQueryCanceled); !ok {
					log.Warnf("Error while evaluating rule %q: %s", rule, err)
				}
				evalFailures.WithLabelValues(rtyp).Inc()
				return
			}

			if ar, ok := rule.(*AlertingRule); ok {
				g.sendAlerts(ar, now)
			}
			var (
				numOutOfOrder = 0
				numDuplicates = 0
			)
			for _, s := range vector {
				if err := g.opts.SampleAppender.Append(s); err != nil {
					switch err {
					case local.ErrOutOfOrderSample:
						numOutOfOrder++
						log.With("sample", s).With("error", err).Debug("Rule evaluation result discarded")
					case local.ErrDuplicateSampleForTimestamp:
						numDuplicates++
						log.With("sample", s).With("error", err).Debug("Rule evaluation result discarded")
					default:
						log.With("sample", s).With("error", err).Warn("Rule evaluation result discarded")
					}
				}
			}
			if numOutOfOrder > 0 {
				log.With("numDropped", numOutOfOrder).Warn("Error on ingesting out-of-order result from rule evaluation")
			}
			if numDuplicates > 0 {
				log.With("numDropped", numDuplicates).Warn("Error on ingesting results from rule evaluation with different value but same timestamp")
			}
		}(rule)
	}
	wg.Wait()
}

// sendAlerts sends alert notifications for the given rule.
func (g *Group) sendAlerts(rule *AlertingRule, timestamp model.Time) error {
	var alerts model.Alerts

	for _, alert := range rule.currentAlerts() {
		// Only send actually firing alerts.
		if alert.State == StatePending {
			continue
		}

		// Provide the alert information to the template.
		l := make(map[string]string, len(alert.Labels))
		for k, v := range alert.Labels {
			l[string(k)] = string(v)
		}

		tmplData := struct {
			Labels map[string]string
			Value  float64
		}{
			Labels: l,
			Value:  float64(alert.Value),
		}
		// Inject some convenience variables that are easier to remember for users
		// who are not used to Go's templating system.
		defs := "{{$labels := .Labels}}{{$value := .Value}}"

		expand := func(text model.LabelValue) model.LabelValue {
			tmpl := template.NewTemplateExpander(
				defs+string(text),
				"__alert_"+rule.Name(),
				tmplData,
				timestamp,
				g.opts.QueryEngine,
				g.opts.ExternalURL.Path,
			)
			result, err := tmpl.Expand()
			if err != nil {
				result = fmt.Sprintf("<error expanding template: %s>", err)
				log.Warnf("Error expanding alert template %v with data '%v': %s", rule.Name(), tmplData, err)
			}
			return model.LabelValue(result)
		}

		labels := make(model.LabelSet, len(alert.Labels)+1)
		for ln, lv := range alert.Labels {
			labels[ln] = expand(lv)
		}
		labels[model.AlertNameLabel] = model.LabelValue(rule.Name())

		annotations := make(model.LabelSet, len(rule.annotations))
		for an, av := range rule.annotations {
			annotations[an] = expand(av)
		}

		a := &model.Alert{
			StartsAt:     alert.ActiveAt.Add(rule.holdDuration).Time(),
			Labels:       labels,
			Annotations:  annotations,
			GeneratorURL: g.opts.ExternalURL.String() + strutil.GraphLinkForExpression(rule.vector.String()),
		}
		if alert.ResolvedAt != 0 {
			a.EndsAt = alert.ResolvedAt.Time()
		}

		alerts = append(alerts, a)
	}

	if len(alerts) > 0 {
		g.opts.Notifier.Send(alerts...)
	}

	return nil
}

// The Manager manages recording and alerting rules.
type Manager struct {
	opts   *ManagerOptions
	groups map[string]*Group
	mtx    sync.RWMutex
	block  chan struct{}
}

// ManagerOptions bundles options for the Manager.
type ManagerOptions struct {
	ExternalURL    *url.URL
	QueryEngine    *promql.Engine
	Notifier       *notifier.Notifier
	SampleAppender storage.SampleAppender
}

// NewManager returns an implementation of Manager, ready to be started
// by calling the Run method.
func NewManager(o *ManagerOptions) *Manager {
	manager := &Manager{
		groups: map[string]*Group{},
		opts:   o,
		block:  make(chan struct{}),
	}
	return manager
}

// Run starts processing of the rule manager.
func (m *Manager) Run() {
	close(m.block)
}

// Stop the rule manager's rule evaluation cycles.
func (m *Manager) Stop() {
	log.Info("Stopping rule manager...")

	for _, eg := range m.groups {
		eg.stop()
	}

	log.Info("Rule manager stopped.")
}

// ApplyConfig updates the rule manager's state as the config requires. If
// loading the new rules failed the old rule set is restored. Returns true on success.
func (m *Manager) ApplyConfig(conf *config.Config) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Get all rule files and load the groups they define.
	var files []string
	for _, pat := range conf.RuleFiles {
		fs, err := filepath.Glob(pat)
		if err != nil {
			// The only error can be a bad pattern.
			log.Errorf("Error retrieving rule files for %s: %s", pat, err)
			return false
		}
		files = append(files, fs...)
	}

	groups, err := m.loadGroups(files...)
	if err != nil {
		log.Errorf("Error loading rules, previous rule set restored: %s", err)
		return false
	}

	var wg sync.WaitGroup

	for _, newg := range groups {
		// To be replaced with a configurable per-group interval.
		newg.interval = time.Duration(conf.GlobalConfig.EvaluationInterval)

		wg.Add(1)

		// If there is an old group with the same identifier, stop it and wait for
		// it to finish the current iteration. Then copy its into the new group.
		oldg, ok := m.groups[newg.name]
		delete(m.groups, newg.name)

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
				newg.run()
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

	return true
}

// loadGroups reads groups from a list of files.
// As there's currently no group syntax a single group named "default" containing
// all rules will be returned.
func (m *Manager) loadGroups(filenames ...string) (map[string]*Group, error) {
	groups := map[string]*Group{}

	// Currently there is no group syntax implemented. Thus all rules
	// are read into a single default group.
	g := newGroup("default", m.opts)
	groups[g.name] = g

	for _, fn := range filenames {
		content, err := ioutil.ReadFile(fn)
		if err != nil {
			return nil, err
		}
		stmts, err := promql.ParseStmts(string(content))
		if err != nil {
			return nil, fmt.Errorf("error parsing %s: %s", fn, err)
		}

		for _, stmt := range stmts {
			var rule Rule

			switch r := stmt.(type) {
			case *promql.AlertStmt:
				rule = NewAlertingRule(r.Name, r.Expr, r.Duration, r.Labels, r.Annotations)

			case *promql.RecordStmt:
				rule = NewRecordingRule(r.Name, r.Expr, r.Labels)

			default:
				panic("retrieval.Manager.LoadRuleFiles: unknown statement type")
			}
			g.rules = append(g.rules, rule)
		}
	}

	return groups, nil
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
