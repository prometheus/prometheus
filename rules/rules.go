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

// A recorded rule.
type Rule struct {
	name      string
	vector    ast.VectorNode
	labels    model.LabelSet
	permanent bool
}

func (rule *Rule) Name() string { return rule.name }

func (rule *Rule) EvalRaw(timestamp *time.Time) (vector ast.Vector) {
	return ast.EvalVectorInstant(rule.vector, *timestamp)
}

func (rule *Rule) Eval(timestamp *time.Time) ast.Vector {
	// Get the raw value of the rule expression.
	vector := rule.EvalRaw(timestamp)

	// Override the metric name and labels.
	for _, sample := range vector {
		sample.Metric["name"] = model.LabelValue(rule.name)
		for label, value := range rule.labels {
			if value == "" {
				delete(sample.Metric, label)
			} else {
				sample.Metric[label] = value
			}
		}
	}
	return vector
}

func (rule *Rule) RuleToDotGraph() string {
	graph := "digraph \"Rules\" {\n"
	graph += fmt.Sprintf("%#p[shape=\"box\",label=\"%v = \"];\n", rule, rule.name)
	graph += fmt.Sprintf("%#p -> %#p;\n", rule, rule.vector)
	graph += rule.vector.NodeTreeToDotGraph()
	graph += "}\n"
	return graph
}

func NewRule(name string, labels model.LabelSet, vector ast.VectorNode, permanent bool) *Rule {
	return &Rule{
		name:      name,
		labels:    labels,
		vector:    vector,
		permanent: permanent,
	}
}
