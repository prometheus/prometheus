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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/util/strutil"
)

// Constants for instrumentation.
const (
	namespace = "prometheus"

	ruleTypeLabel     = "rule_type"
	alertingRuleType  = "alerting"
	recordingRuleType = "recording"
)

var (
	evalDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_duration_milliseconds",
			Help:      "The duration for a rule to execute.",
		},
		[]string{ruleTypeLabel},
	)
	evalFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_evaluation_failures_total",
			Help:      "The total number of rule evaluation failures.",
		},
	)
	iterationDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:  namespace,
		Name:       "evaluator_duration_milliseconds",
		Help:       "The duration for all evaluations to execute.",
		Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
	})
)

func init() {
	prometheus.MustRegister(iterationDuration)
	prometheus.MustRegister(evalFailures)
	prometheus.MustRegister(evalDuration)
}

// The Manager manages recording and alerting rules.
type Manager struct {
	// Protects the rules list.
	sync.Mutex
	rules []Rule

	done chan bool

	interval    time.Duration
	queryEngine *promql.Engine

	sampleAppender      storage.SampleAppender
	notificationHandler *notification.NotificationHandler

	prometheusURL string
	pathPrefix    string
}

// ManagerOptions bundles options for the Manager.
type ManagerOptions struct {
	EvaluationInterval time.Duration
	QueryEngine        *promql.Engine

	NotificationHandler *notification.NotificationHandler
	SampleAppender      storage.SampleAppender

	PrometheusURL string
	PathPrefix    string
}

// NewManager returns an implementation of Manager, ready to be started
// by calling the Run method.
func NewManager(o *ManagerOptions) *Manager {
	manager := &Manager{
		rules: []Rule{},
		done:  make(chan bool),

		interval:            o.EvaluationInterval,
		sampleAppender:      o.SampleAppender,
		queryEngine:         o.QueryEngine,
		notificationHandler: o.NotificationHandler,
		prometheusURL:       o.PrometheusURL,
	}
	return manager
}

// Run the rule manager's periodic rule evaluation.
func (m *Manager) Run() {
	defer log.Info("Rule manager stopped.")

	m.Lock()
	lastInterval := m.interval
	m.Unlock()

	ticker := time.NewTicker(lastInterval)
	defer ticker.Stop()

	for {
		// The outer select clause makes sure that m.done is looked at
		// first. Otherwise, if m.runIteration takes longer than
		// m.interval, there is only a 50% chance that m.done will be
		// looked at before the next m.runIteration call happens.
		select {
		case <-m.done:
			return
		default:
			select {
			case <-ticker.C:
				start := time.Now()
				m.runIteration()
				iterationDuration.Observe(float64(time.Since(start) / time.Millisecond))

				m.Lock()
				if lastInterval != m.interval {
					ticker.Stop()
					ticker = time.NewTicker(m.interval)
					lastInterval = m.interval
				}
				m.Unlock()
			case <-m.done:
				return
			}
		}
	}
}

// Stop the rule manager's rule evaluation cycles.
func (m *Manager) Stop() {
	log.Info("Stopping rule manager...")
	m.done <- true
}

func (m *Manager) queueAlertNotifications(rule *AlertingRule, timestamp clientmodel.Timestamp) {
	activeAlerts := rule.ActiveAlerts()
	if len(activeAlerts) == 0 {
		return
	}

	notifications := make(notification.NotificationReqs, 0, len(activeAlerts))
	for _, aa := range activeAlerts {
		if aa.State != Firing {
			// BUG: In the future, make AlertManager support pending alerts?
			continue
		}

		// Provide the alert information to the template.
		l := map[string]string{}
		for k, v := range aa.Labels {
			l[string(k)] = string(v)
		}
		tmplData := struct {
			Labels map[string]string
			Value  clientmodel.SampleValue
		}{
			Labels: l,
			Value:  aa.Value,
		}
		// Inject some convenience variables that are easier to remember for users
		// who are not used to Go's templating system.
		defs := "{{$labels := .Labels}}{{$value := .Value}}"

		expand := func(text string) string {
			tmpl := template.NewTemplateExpander(defs+text, "__alert_"+rule.Name(), tmplData, timestamp, m.queryEngine, m.pathPrefix)
			result, err := tmpl.Expand()
			if err != nil {
				result = err.Error()
				log.Warnf("Error expanding alert template %v with data '%v': %v", rule.Name(), tmplData, err)
			}
			return result
		}

		notifications = append(notifications, &notification.NotificationReq{
			Summary:     expand(rule.Summary),
			Description: expand(rule.Description),
			Labels: aa.Labels.Merge(clientmodel.LabelSet{
				AlertNameLabel: clientmodel.LabelValue(rule.Name()),
			}),
			Value:        aa.Value,
			ActiveSince:  aa.ActiveSince.Time(),
			RuleString:   rule.String(),
			GeneratorURL: m.prometheusURL + strings.TrimLeft(strutil.GraphLinkForExpression(rule.Vector.String()), "/"),
		})
	}
	m.notificationHandler.SubmitReqs(notifications)
}

func (m *Manager) runIteration() {
	now := clientmodel.Now()
	wg := sync.WaitGroup{}

	m.Lock()
	rulesSnapshot := make([]Rule, len(m.rules))
	copy(rulesSnapshot, m.rules)
	m.Unlock()

	for _, rule := range rulesSnapshot {
		wg.Add(1)
		// BUG(julius): Look at fixing thundering herd.
		go func(rule Rule) {
			defer wg.Done()

			start := time.Now()
			vector, err := rule.Eval(now, m.queryEngine)
			duration := time.Since(start)

			if err != nil {
				evalFailures.Inc()
				log.Warnf("Error while evaluating rule %q: %s", rule, err)
				return
			}

			switch r := rule.(type) {
			case *AlertingRule:
				m.queueAlertNotifications(r, now)
				evalDuration.WithLabelValues(alertingRuleType).Observe(
					float64(duration / time.Millisecond),
				)
			case *RecordingRule:
				evalDuration.WithLabelValues(recordingRuleType).Observe(
					float64(duration / time.Millisecond),
				)
			default:
				panic(fmt.Errorf("Unknown rule type: %T", rule))
			}

			for _, s := range vector {
				m.sampleAppender.Append(&clientmodel.Sample{
					Metric:    s.Metric.Metric,
					Value:     s.Value,
					Timestamp: s.Timestamp,
				})
			}
		}(rule)
	}
	wg.Wait()
}

// ApplyConfig updates the rule manager's state as the config requires. If
// loading the new rules failed the old rule set is restored.
func (m *Manager) ApplyConfig(conf *config.Config) {
	m.Lock()
	defer m.Unlock()

	m.interval = time.Duration(conf.GlobalConfig.EvaluationInterval)

	rulesSnapshot := make([]Rule, len(m.rules))
	copy(rulesSnapshot, m.rules)
	m.rules = m.rules[:0]

	var files []string
	for _, pat := range conf.RuleFiles {
		fs, err := filepath.Glob(pat)
		if err != nil {
			// The only error can be a bad pattern.
			log.Errorf("Error retrieving rule files for %s: %s", pat, err)
		}
		files = append(files, fs...)
	}
	if err := m.loadRuleFiles(files...); err != nil {
		// If loading the new rules failed, restore the old rule set.
		m.rules = rulesSnapshot
		log.Errorf("Error loading rules, previous rule set restored: %s", err)
	}
}

// loadRuleFiles loads alerting and recording rules from the given files.
func (m *Manager) loadRuleFiles(filenames ...string) error {
	for _, fn := range filenames {
		content, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		stmts, err := promql.ParseStmts(string(content))
		if err != nil {
			return fmt.Errorf("error parsing %s: %s", fn, err)
		}
		for _, stmt := range stmts {
			switch r := stmt.(type) {
			case *promql.AlertStmt:
				rule := NewAlertingRule(r.Name, r.Expr, r.Duration, r.Labels, r.Summary, r.Description)
				m.rules = append(m.rules, rule)
			case *promql.RecordStmt:
				rule := &RecordingRule{r.Name, r.Expr, r.Labels}
				m.rules = append(m.rules, rule)
			default:
				panic("retrieval.Manager.LoadRuleFiles: unknown statement type")
			}
		}
	}
	return nil
}

// Rules returns the list of the manager's rules.
func (m *Manager) Rules() []Rule {
	m.Lock()
	defer m.Unlock()

	rules := make([]Rule, len(m.rules))
	copy(rules, m.rules)
	return rules
}

// AlertingRules returns the list of the manager's alerting rules.
func (m *Manager) AlertingRules() []*AlertingRule {
	m.Lock()
	defer m.Unlock()

	alerts := []*AlertingRule{}
	for _, rule := range m.rules {
		if alertingRule, ok := rule.(*AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}
	return alerts
}
