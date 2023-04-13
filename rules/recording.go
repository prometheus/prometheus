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
	"github.com/prometheus/prometheus/template"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name   string
	vector parser.Expr
	labels labels.Labels
	// The health of the recording rule.
	health *atomic.String
	// Timestamp of last evaluation of the recording rule.
	evaluationTimestamp *atomic.Time
	// The last error seen by the recording rule.
	lastError *atomic.Error
	// Duration of how long it took to evaluate the recording rule.
	evaluationDuration *atomic.Duration
	// External labels from the global config.
	externalLabels map[string]string
	// The external URL from the --web.external-url flag.
	externalURL string
	logger      log.Logger
}

// NewRecordingRule returns a new recording rule.
func NewRecordingRule(
	name string, vector parser.Expr, lset labels.Labels,
	externalLabels labels.Labels, externalURL string,
	logger log.Logger,
) *RecordingRule {
	el := externalLabels.Map()
	return &RecordingRule{
		name:                name,
		vector:              vector,
		labels:              lset,
		health:              atomic.NewString(string(HealthUnknown)),
		evaluationTimestamp: atomic.NewTime(time.Time{}),
		evaluationDuration:  atomic.NewDuration(0),
		lastError:           atomic.NewError(nil),
		externalLabels:      el,
		externalURL:         externalURL,
		logger:              logger,
	}
}

// Name returns the rule name.
func (rule *RecordingRule) Name() string {
	return rule.name
}

// Query returns the rule query expression.
func (rule *RecordingRule) Query() parser.Expr {
	return rule.vector
}

// Labels returns the rule labels.
func (rule *RecordingRule) Labels() labels.Labels {
	return rule.labels
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule *RecordingRule) Eval(ctx context.Context, ts time.Time, query QueryFunc, externalURL *url.URL, limit int) (promql.Vector, error) {
	ctx = NewOriginContext(ctx, NewRuleDetail(rule))

	vector, err := query(ctx, rule.vector.String(), ts)
	if err != nil {
		return nil, err
	}

	// Override the metric name and labels.
	lb := labels.NewBuilder(labels.EmptyLabels())

	for i := range vector {
		sample := &vector[i]
		l := sample.Metric.Map()

		tmplData := template.AlertTemplateData(l, rule.externalLabels, rule.externalURL, sample.F)
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
				"__record_"+rule.Name(),
				tmplData,
				model.Time(timestamp.FromTime(ts)),
				template.QueryFunc(query),
				externalURL,
				nil,
			)
			result, err := tmpl.Expand()
			if err != nil {
				result = fmt.Sprintf("<error expanding template: %s>", err)
				level.Warn(rule.logger).Log(
					"msg",
					"Expanding record template failed", "err", err, "data", tmplData,
				)
			}
			return result
		}
		lb.Reset(sample.Metric)
		lb.Set(labels.MetricName, rule.name)

		rule.labels.Range(func(l labels.Label) {
			lb.Set(l.Name, expand(l.Value))
		})

		sample.Metric = lb.Labels()
	}

	// Check that the rule does not produce identical metrics after applying
	// labels.
	if vector.ContainsSameLabelset() {
		return nil, fmt.Errorf("vector contains metrics with the same labelset after applying rule labels")
	}

	numSeries := len(vector)
	if limit > 0 && numSeries > limit {
		return nil, fmt.Errorf("exceeded limit of %d with %d series", limit, numSeries)
	}

	rule.SetHealth(HealthGood)
	rule.SetLastError(err)
	return vector, nil
}

func (rule *RecordingRule) String() string {
	r := rulefmt.Rule{
		Record: rule.name,
		Expr:   rule.vector.String(),
		Labels: rule.labels.Map(),
	}

	byt, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Sprintf("error marshaling recording rule: %q", err.Error())
	}

	return string(byt)
}

// SetEvaluationDuration updates evaluationDuration to the time in seconds it took to evaluate the rule on its last evaluation.
func (rule *RecordingRule) SetEvaluationDuration(dur time.Duration) {
	rule.evaluationDuration.Store(dur)
}

// SetLastError sets the current error seen by the recording rule.
func (rule *RecordingRule) SetLastError(err error) {
	rule.lastError.Store(err)
}

// LastError returns the last error seen by the recording rule.
func (rule *RecordingRule) LastError() error {
	return rule.lastError.Load()
}

// SetHealth sets the current health of the recording rule.
func (rule *RecordingRule) SetHealth(health RuleHealth) {
	rule.health.Store(string(health))
}

// Health returns the current health of the recording rule.
func (rule *RecordingRule) Health() RuleHealth {
	return RuleHealth(rule.health.Load())
}

// GetEvaluationDuration returns the time in seconds it took to evaluate the recording rule.
func (rule *RecordingRule) GetEvaluationDuration() time.Duration {
	return rule.evaluationDuration.Load()
}

// SetEvaluationTimestamp updates evaluationTimestamp to the timestamp of when the rule was last evaluated.
func (rule *RecordingRule) SetEvaluationTimestamp(ts time.Time) {
	rule.evaluationTimestamp.Store(ts)
}

// GetEvaluationTimestamp returns the time the evaluation took place.
func (rule *RecordingRule) GetEvaluationTimestamp() time.Time {
	return rule.evaluationTimestamp.Load()
}
