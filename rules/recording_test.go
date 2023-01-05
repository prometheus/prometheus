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

func TestRuleEval(t *testing.T) {
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

	suite := []struct {
		name   string
		expr   parser.Expr
		labels labels.Labels
		result promql.Vector
		err    string
	}{
		{
			name:   "nolabels",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.Labels{},
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "nolabels"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
		{
			name:   "labels",
			expr:   &parser.NumberLiteral{Val: 1},
			labels: labels.FromStrings("foo", "bar"),
			result: promql.Vector{promql.Sample{
				Metric: labels.FromStrings("__name__", "labels", "foo", "bar"),
				Point:  promql.Point{V: 1, T: timestamp.FromTime(now)},
			}},
		},
	}

	for _, test := range suite {
		rule := NewRecordingRule(test.name, test.expr, test.labels)
		result, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil, 0)
		if test.err == "" {
			require.NoError(t, err)
		} else {
			require.Equal(t, test.err, err.Error())
		}
		require.Equal(t, test.result, result)
	}
}

func BenchmarkRuleEval(b *testing.B) {
	suite, err := promql.NewTest(b, `
		load 1m
			metric{label_a="1",label_b="3"} 1
			metric{label_a="2",label_b="4"} 1
	`)
	require.NoError(b, err)
	defer suite.Close()

	require.NoError(b, suite.Run())
	ts := time.Unix(0, 0).UTC()
	exprWithMetricName, _ := parser.ParseExpr(`metric`)
	exprWithoutMetricName, _ := parser.ParseExpr(`metric + metric`)

	scenarios := []struct {
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
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
				},
				promql.Sample{
					Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4"),
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
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
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
				},
				promql.Sample{
					Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4", "extra_from_rule", "foo"),
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
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
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
				},
				promql.Sample{
					Metric: labels.FromStrings("__name__", "test_rule", "label_a", "from_rule", "label_b", "4"),
					Point:  promql.Point{V: 1, T: timestamp.FromTime(ts)},
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
					Point:  promql.Point{V: 2, T: timestamp.FromTime(ts)},
				},
				promql.Sample{
					Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4"),
					Point:  promql.Point{V: 2, T: timestamp.FromTime(ts)},
				},
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			rule := NewRecordingRule("test_rule", scenario.expr, scenario.ruleLabels)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				result, err := rule.Eval(suite.Context(), ts, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil, 0)

				require.NoError(b, err)
				require.ElementsMatch(b, scenario.expected, result)
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
	suite, err := promql.NewTest(t, `
		load 1m
			metric{label="1"} 1
			metric{label="2"} 1
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

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
		_, err := rule.Eval(suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil, test.limit)
		if err != nil {
			require.EqualError(t, err, test.err)
		} else if test.err != "" {
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
