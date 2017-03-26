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

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name   string
	vector promql.Expr
	labels model.LabelSet
}

// NewRecordingRule returns a new recording rule.
func NewRecordingRule(name string, vector promql.Expr, labels model.LabelSet) *RecordingRule {
	return &RecordingRule{
		name:   name,
		vector: vector,
		labels: labels,
	}
}

// Name returns the rule name.
func (rule RecordingRule) Name() string {
	return rule.name
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule RecordingRule) Eval(ctx context.Context, timestamp model.Time, engine *promql.Engine, _ string) (model.Vector, error) {
	query, err := engine.NewInstantQuery(rule.vector.String(), timestamp)
	if err != nil {
		return nil, err
	}

	var (
		result = query.Exec(ctx)
		vector model.Vector
	)
	if result.Err != nil {
		return nil, err
	}

	switch result.Value.(type) {
	case model.Vector:
		vector, err = result.Vector()
		if err != nil {
			return nil, err
		}
	case *model.Scalar:
		scalar, err := result.Scalar()
		if err != nil {
			return nil, err
		}
		vector = model.Vector{&model.Sample{
			Value:     scalar.Value,
			Timestamp: scalar.Timestamp,
			Metric:    model.Metric{},
		}}
	default:
		return nil, fmt.Errorf("rule result is not a vector or scalar")
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

	return vector, nil
}

func (rule RecordingRule) String() string {
	return fmt.Sprintf("%s%s = %s\n", rule.name, rule.labels, rule.vector)
}

// HTMLSnippet returns an HTML snippet representing this rule.
func (rule RecordingRule) HTMLSnippet(pathPrefix string) template.HTML {
	ruleExpr := rule.vector.String()
	return template.HTML(fmt.Sprintf(
		`<a href="%s">%s</a>%s = <a href="%s">%s</a>`,
		pathPrefix+strutil.GraphLinkForExpression(rule.name),
		rule.name,
		template.HTMLEscapeString(rule.labels.String()),
		pathPrefix+strutil.GraphLinkForExpression(ruleExpr),
		template.HTMLEscapeString(ruleExpr)))
}
