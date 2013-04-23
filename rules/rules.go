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

package rules

import (
	"fmt"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/rules/ast"
	"time"
)

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	// Name returns the name of the rule.
	Name() string
	// EvalRaw evaluates the rule's vector expression without triggering any
	// other actions, like recording or alerting.
	EvalRaw(timestamp *time.Time) (vector ast.Vector, err error)
	// Eval evaluates the rule, including any associated recording or alerting actions.
	Eval(timestamp *time.Time) (vector ast.Vector, err error)
}

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name      string
	vector    ast.VectorNode
	labels    model.LabelSet
	permanent bool
}

// An alerting rule generates alerts from its vector expression.
type AlertingRule struct {
	name         string
	vector       ast.VectorNode
	holdDuration time.Duration
	labels       model.LabelSet
}

func (rule RecordingRule) Name() string { return rule.name }

func (rule RecordingRule) EvalRaw(timestamp *time.Time) (vector ast.Vector, err error) {
	return ast.EvalVectorInstant(rule.vector, *timestamp)
}

func (rule RecordingRule) Eval(timestamp *time.Time) (vector ast.Vector, err error) {
	// Get the raw value of the rule expression.
	vector, err = rule.EvalRaw(timestamp)
	if err != nil {
		return
	}

	// Override the metric name and labels.
	for _, sample := range vector {
		sample.Metric[model.MetricNameLabel] = model.LabelValue(rule.name)
		for label, value := range rule.labels {
			if value == "" {
				delete(sample.Metric, label)
			} else {
				sample.Metric[label] = value
			}
		}
	}
	return
}

func (rule RecordingRule) RuleToDotGraph() string {
	graph := "digraph \"Rules\" {\n"
	graph += fmt.Sprintf("%#p[shape=\"box\",label=\"%v = \"];\n", rule, rule.name)
	graph += fmt.Sprintf("%#p -> %#p;\n", &rule, rule.vector)
	graph += rule.vector.NodeTreeToDotGraph()
	graph += "}\n"
	return graph
}

func (rule AlertingRule) Name() string { return rule.name }

func (rule AlertingRule) EvalRaw(timestamp *time.Time) (vector ast.Vector, err error) {
	return ast.EvalVectorInstant(rule.vector, *timestamp)
}

func (rule AlertingRule) Eval(timestamp *time.Time) (vector ast.Vector, err error) {
	// Get the raw value of the rule expression.
	vector, err = rule.EvalRaw(timestamp)
	if err != nil {
		return
	}

	// TODO(julius): handle alerting.
	return
}

func NewRecordingRule(name string, labels model.LabelSet, vector ast.VectorNode, permanent bool) *RecordingRule {
	return &RecordingRule{
		name:      name,
		labels:    labels,
		vector:    vector,
		permanent: permanent,
	}
}

func NewAlertingRule(name string, vector ast.VectorNode, holdDuration time.Duration, labels model.LabelSet) *AlertingRule {
	return &AlertingRule{
		name:         name,
		vector:       vector,
		holdDuration: holdDuration,
		labels:       labels,
	}
}
