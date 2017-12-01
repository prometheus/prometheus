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
	"html/template"
	"net/url"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name           string
	vector         promql.Expr
	labels         labels.Labels
	mtx            sync.Mutex
	evaluationTime time.Duration
}

// NewRecordingRule returns a new recording rule.
func NewRecordingRule(name string, vector promql.Expr, lset labels.Labels) *RecordingRule {
	return &RecordingRule{
		name:   name,
		vector: vector,
		labels: lset,
	}
}

// Name returns the rule name.
func (rule *RecordingRule) Name() string {
	return rule.name
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule *RecordingRule) Eval(ctx context.Context, ts time.Time, query QueryFunc, _ *url.URL) (promql.Vector, error) {
	vector, err := query(ctx, rule.vector.String(), ts)
	if err != nil {
		return nil, err
	}
	// Override the metric name and labels.
	for i := range vector {
		sample := &vector[i]

		lb := labels.NewBuilder(sample.Metric)

		lb.Set(labels.MetricName, rule.name)

		for _, l := range rule.labels {
			if l.Value == "" {
				lb.Del(l.Name)
			} else {
				lb.Set(l.Name, l.Value)
			}
		}

		sample.Metric = lb.Labels()
	}

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
		return fmt.Sprintf("error marshalling recording rule: %q", err.Error())
	}

	return string(byt)
}

// SetEvaluationTime updates evaluationTimeSeconds to the time in seconds it took to evaluate the rule on its last evaluation.
func (rule *RecordingRule) SetEvaluationTime(dur time.Duration) {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	rule.evaluationTime = dur
}

// GetEvaluationTime returns the time in seconds it took to evaluate the recording rule.
func (rule *RecordingRule) GetEvaluationTime() time.Duration {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	return rule.evaluationTime
}

// HTMLSnippet returns an HTML snippet representing this rule.
func (rule *RecordingRule) HTMLSnippet(pathPrefix string) template.HTML {
	ruleExpr := rule.vector.String()
	labels := make(map[string]string, len(rule.labels))
	for _, l := range rule.labels {
		labels[l.Name] = template.HTMLEscapeString(l.Value)
	}

	r := rulefmt.Rule{
		Record: fmt.Sprintf(`<a href="%s">%s</a>`, pathPrefix+strutil.TableLinkForExpression(rule.name), rule.name),
		Expr:   fmt.Sprintf(`<a href="%s">%s</a>`, pathPrefix+strutil.TableLinkForExpression(ruleExpr), template.HTMLEscapeString(ruleExpr)),
		Labels: labels,
	}

	byt, err := yaml.Marshal(r)
	if err != nil {
		return template.HTML(fmt.Sprintf("error marshalling recording rule: %q", err.Error()))
	}

	return template.HTML(byt)
}
