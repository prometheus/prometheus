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
	"html/template"
	"reflect"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name   string
	vector promql.Expr
	labels clientmodel.LabelSet
}

// Name returns the rule name.
func (rule RecordingRule) Name() string { return rule.name }

// EvalRaw returns the raw value of the rule expression.
func (rule RecordingRule) EvalRaw(timestamp clientmodel.Timestamp, engine *promql.Engine) (promql.Vector, error) {
	query, err := engine.NewInstantQuery(rule.vector.String(), timestamp)
	if err != nil {
		return nil, err
	}
	return query.Exec().Vector()
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule RecordingRule) Eval(timestamp clientmodel.Timestamp, engine *promql.Engine) (promql.Vector, error) {
	vector, err := rule.EvalRaw(timestamp, engine)
	if err != nil {
		return nil, err
	}

	// Override the metric name and labels.
	for _, sample := range vector {
		sample.Metric.Set(clientmodel.MetricNameLabel, clientmodel.LabelValue(rule.name))
		for label, value := range rule.labels {
			if value == "" {
				sample.Metric.Delete(label)
			} else {
				sample.Metric.Set(label, value)
			}
		}
	}

	return vector, nil
}

// DotGraph returns the text representation of a dot graph.
func (rule RecordingRule) DotGraph() string {
	graph := fmt.Sprintf(
		`digraph "Rules" {
	  %#p[shape="box",label="%s = "];
		%#p -> %x;
		%s
	}`,
		&rule, rule.name,
		&rule, reflect.ValueOf(rule.vector).Pointer(),
		rule.vector.DotGraph(),
	)
	return graph
}

func (rule RecordingRule) String() string {
	return fmt.Sprintf("%s%s = %s\n", rule.name, rule.labels, rule.vector)
}

// HTMLSnippet returns an HTML snippet representing this rule.
func (rule RecordingRule) HTMLSnippet(pathPrefix string) template.HTML {
	ruleExpr := rule.vector.String()
	return template.HTML(fmt.Sprintf(
		`<a href="%s">%s</a>%s = <a href="%s">%s</a>`,
		pathPrefix+strings.TrimLeft(strutil.GraphLinkForExpression(rule.name), "/"),
		rule.name,
		rule.labels,
		pathPrefix+strings.TrimLeft(strutil.GraphLinkForExpression(ruleExpr), "/"),
		ruleExpr))
}
