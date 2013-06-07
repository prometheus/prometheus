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
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
	"time"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name      string
	vector    ast.VectorNode
	labels    model.LabelSet
	permanent bool
}

func (rule RecordingRule) Name() string { return rule.name }

func (rule RecordingRule) EvalRaw(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error) {
	return ast.EvalVectorInstant(rule.vector, timestamp, storage, stats.NewTimerGroup())
}

func (rule RecordingRule) Eval(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error) {
	// Get the raw value of the rule expression.
	vector, err := rule.EvalRaw(timestamp, storage)
	if err != nil {
		return nil, err
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

func (rule RecordingRule) ToDotGraph() string {
	graph := fmt.Sprintf(`digraph "Rules" {
	  %#p[shape="box",label="%s = "];
		%#p -> %#p;
		%s
	}`, &rule, rule.name, &rule, rule.vector, rule.vector.NodeTreeToDotGraph())
	return graph
}

func (rule RecordingRule) String() string {
	return fmt.Sprintf("%s%s = %s\n", rule.name, rule.labels, rule.vector)
}

// Construct a new RecordingRule.
func NewRecordingRule(name string, labels model.LabelSet, vector ast.VectorNode, permanent bool) *RecordingRule {
	return &RecordingRule{
		name:      name,
		labels:    labels,
		vector:    vector,
		permanent: permanent,
	}
}
