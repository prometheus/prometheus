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

package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/templates"
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
	iterationDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:  namespace,
		Name:       "evaluator_duration_milliseconds",
		Help:       "The duration for all evaluations to execute.",
		Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
	})
)

func init() {
	prometheus.MustRegister(iterationDuration)
	prometheus.MustRegister(evalDuration)
}

type RuleManager interface {
	// Load and add rules from rule files specified in the configuration.
	AddRulesFromConfig(config config.Config) error
	// Start the rule manager's periodic rule evaluation.
	Run()
	// Stop the rule manager's rule evaluation cycles.
	Stop()
	// Return all rules.
	Rules() []rules.Rule
	// Return all alerting rules.
	AlertingRules() []*rules.AlertingRule
}

type ruleManager struct {
	// Protects the rules list.
	sync.Mutex
	rules []rules.Rule

	done chan bool

	interval time.Duration
	storage  local.Storage

	results             chan<- *extraction.Result
	notificationHandler *notification.NotificationHandler

	prometheusUrl string
}

type RuleManagerOptions struct {
	EvaluationInterval time.Duration
	Storage            local.Storage

	NotificationHandler *notification.NotificationHandler
	Results             chan<- *extraction.Result

	PrometheusUrl string
}

func NewRuleManager(o *RuleManagerOptions) RuleManager {
	manager := &ruleManager{
		rules: []rules.Rule{},
		done:  make(chan bool),

		interval:            o.EvaluationInterval,
		storage:             o.Storage,
		results:             o.Results,
		notificationHandler: o.NotificationHandler,
		prometheusUrl:       o.PrometheusUrl,
	}
	return manager
}

func (m *ruleManager) Run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			m.runIteration(m.results)
			iterationDuration.Observe(float64(time.Since(start) / time.Millisecond))
		case <-m.done:
			glog.Info("Rule manager stopped.")
			return
		}
	}
}

func (m *ruleManager) Stop() {
	glog.Info("Stopping rule manager...")
	m.done <- true
}

func (m *ruleManager) queueAlertNotifications(rule *rules.AlertingRule, timestamp clientmodel.Timestamp) {
	activeAlerts := rule.ActiveAlerts()
	if len(activeAlerts) == 0 {
		return
	}

	notifications := make(notification.NotificationReqs, 0, len(activeAlerts))
	for _, aa := range activeAlerts {
		if aa.State != rules.FIRING {
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
			template := templates.NewTemplateExpander(defs+text, "__alert_"+rule.Name(), tmplData, timestamp, m.storage)
			result, err := template.Expand()
			if err != nil {
				result = err.Error()
				glog.Warningf("Error expanding alert template %v with data '%v': %v", rule.Name(), tmplData, err)
			}
			return result
		}

		notifications = append(notifications, &notification.NotificationReq{
			Summary:     expand(rule.Summary),
			Description: expand(rule.Description),
			Labels: aa.Labels.Merge(clientmodel.LabelSet{
				rules.AlertNameLabel: clientmodel.LabelValue(rule.Name()),
			}),
			Value:        aa.Value,
			ActiveSince:  aa.ActiveSince.Time(),
			RuleString:   rule.String(),
			GeneratorUrl: m.prometheusUrl + rules.GraphLinkForExpression(rule.Vector.String()),
		})
	}
	m.notificationHandler.SubmitReqs(notifications)
}

func (m *ruleManager) runIteration(results chan<- *extraction.Result) {
	now := clientmodel.Now()
	wg := sync.WaitGroup{}

	m.Lock()
	rulesSnapshot := make([]rules.Rule, len(m.rules))
	copy(rulesSnapshot, m.rules)
	m.Unlock()

	for _, rule := range rulesSnapshot {
		wg.Add(1)
		// BUG(julius): Look at fixing thundering herd.
		go func(rule rules.Rule) {
			defer wg.Done()

			start := time.Now()
			vector, err := rule.Eval(now, m.storage)
			duration := time.Since(start)

			samples := make(clientmodel.Samples, len(vector))
			copy(samples, vector)
			m.results <- &extraction.Result{
				Samples: samples,
				Err:     err,
			}

			switch r := rule.(type) {
			case *rules.AlertingRule:
				m.queueAlertNotifications(r, now)
				evalDuration.WithLabelValues(alertingRuleType).Observe(
					float64(duration / time.Millisecond),
				)
			case *rules.RecordingRule:
				evalDuration.WithLabelValues(recordingRuleType).Observe(
					float64(duration / time.Millisecond),
				)
			default:
				panic(fmt.Sprintf("Unknown rule type: %T", rule))
			}
		}(rule)
	}

	wg.Wait()
}

func (m *ruleManager) AddRulesFromConfig(config config.Config) error {
	for _, ruleFile := range config.Global.RuleFile {
		newRules, err := rules.LoadRulesFromFile(ruleFile)
		if err != nil {
			return fmt.Errorf("%s: %s", ruleFile, err)
		}
		m.Lock()
		m.rules = append(m.rules, newRules...)
		m.Unlock()
	}
	return nil
}

func (m *ruleManager) Rules() []rules.Rule {
	m.Lock()
	defer m.Unlock()

	rules := make([]rules.Rule, len(m.rules))
	copy(rules, m.rules)
	return rules
}

func (m *ruleManager) AlertingRules() []*rules.AlertingRule {
	m.Lock()
	defer m.Unlock()

	alerts := []*rules.AlertingRule{}
	for _, rule := range m.rules {
		if alertingRule, ok := rule.(*rules.AlertingRule); ok {
			alerts = append(alerts, alertingRule)
		}
	}
	return alerts
}
