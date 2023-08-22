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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/teststorage"
)

var (
	ruleEvaluationTime       = time.Unix(0, 0).UTC()
	exprWithMetricName, _    = parser.ParseExpr(`sort(metric)`)
	exprWithoutMetricName, _ = parser.ParseExpr(`sort(metric + metric)`)
)

var ruleEvalTestScenarios = []struct {
	name       string
	ruleLabels labels.Labels
	expr       parser.Expr
	expected   promql.Vector
}{
	{
		name:       "no labels in recording rule, metric name in query result",
		ruleLabels: labels.EmptyLabels(),
		expr:       exprWithMetricName,
		expected: promql.Vector{
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "1", "label_b", "3"),
				F:      1,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4"),
				F:      10,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
		},
	},
	{
		name:       "only new labels in recording rule, metric name in query result",
		ruleLabels: labels.FromStrings("extra_from_rule", "foo"),
		expr:       exprWithMetricName,
		expected: promql.Vector{
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "1", "label_b", "3", "extra_from_rule", "foo"),
				F:      1,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4", "extra_from_rule", "foo"),
				F:      10,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
		},
	},
	{
		name:       "some replacement labels in recording rule, metric name in query result",
		ruleLabels: labels.FromStrings("label_a", "from_rule"),
		expr:       exprWithMetricName,
		expected: promql.Vector{
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "from_rule", "label_b", "3"),
				F:      1,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "from_rule", "label_b", "4"),
				F:      10,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
		},
	},
	{
		name:       "no labels in recording rule, no metric name in query result",
		ruleLabels: labels.EmptyLabels(),
		expr:       exprWithoutMetricName,
		expected: promql.Vector{
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "1", "label_b", "3"),
				F:      2,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
			promql.Sample{
				Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4"),
				F:      20,
				T:      timestamp.FromTime(ruleEvaluationTime),
			},
		},
	},
}

func setUpRuleEvalTest(t require.TestingT) *teststorage.TestStorage {
	return promql.LoadedStorage(t, `
		load 1m
			metric{label_a="1",label_b="3"} 1
			metric{label_a="2",label_b="4"} 10
	`)
}

func TestRuleEval(t *testing.T) {
	storage := setUpRuleEvalTest(t)
	t.Cleanup(func() { storage.Close() })

	for _, scenario := range ruleEvalTestScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			rule := NewRecordingRule("test_rule", scenario.expr, scenario.ruleLabels)
			result, err := rule.Eval(context.TODO(), ruleEvaluationTime, EngineQueryFunc(testEngine, storage), nil, 0)
			require.NoError(t, err)
			require.Equal(t, scenario.expected, result)
		})
	}
}

func BenchmarkRuleEval(b *testing.B) {
	storage := setUpRuleEvalTest(b)
	b.Cleanup(func() { storage.Close() })

	for _, scenario := range ruleEvalTestScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			rule := NewRecordingRule("test_rule", scenario.expr, scenario.ruleLabels)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := rule.Eval(context.TODO(), ruleEvaluationTime, EngineQueryFunc(testEngine, storage), nil, 0)
				if err != nil {
					require.NoError(b, err)
				}
			}
		})
	}
}

// TestRuleEvalDuplicate tests for duplicate labels in recorded metrics, see #5529.
func TestRuleEvalDuplicate(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}

	engine := promql.NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	now := time.Now()

	expr, _ := parser.ParseExpr(`vector(0) or label_replace(vector(0),"test","x","","")`)
	rule := NewRecordingRule("foo", expr, labels.FromStrings("test", "test"))
	_, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil, 0)
	require.Error(t, err)
	require.EqualError(t, err, "vector contains metrics with the same labelset after applying rule labels")
}

func TestRecordingRuleLimit(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			metric{label="1"} 1
			metric{label="2"} 1
	`)
	t.Cleanup(func() { storage.Close() })

	tests := []struct {
		limit int
		err   string
	}{
		{
			limit: 0,
		},
		{
			limit: -1,
		},
		{
			limit: 2,
		},
		{
			limit: 1,
			err:   "exceeded limit of 1 with 2 series",
		},
	}

	expr, _ := parser.ParseExpr(`metric > 0`)
	rule := NewRecordingRule(
		"foo",
		expr,
		labels.FromStrings("test", "test"),
	)

	evalTime := time.Unix(0, 0)

	for _, test := range tests {
		switch _, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, test.limit); {
		case err != nil:
			require.EqualError(t, err, test.err)
		case test.err != "":
			t.Errorf("Expected error %s, got none", test.err)
		}
	}
}

// TestRecordingEvalWithOrigin checks that the recording rule details are passed through the context.
func TestRecordingEvalWithOrigin(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	const (
		name  = "my-recording-rule"
		query = `count(metric{foo="bar"})`
	)

	var (
		detail RuleDetail
		lbs    = labels.FromStrings("foo", "bar")
	)

	expr, err := parser.ParseExpr(query)
	require.NoError(t, err)

	rule := NewRecordingRule(name, expr, lbs)
	_, err = rule.Eval(ctx, now, func(ctx context.Context, qs string, _ time.Time) (promql.Vector, error) {
		detail = FromOriginContext(ctx)
		return nil, nil
	}, nil, 0)

	require.NoError(t, err)
	require.Equal(t, detail, NewRuleDetail(rule))
}
