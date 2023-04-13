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

	"github.com/go-kit/log"
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

func setUpRuleEvalTest(t require.TestingT) *promql.Test {
	suite, err := promql.NewTest(t, `
		load 1m
			metric{label_a="1",label_b="3"} 1
			metric{label_a="2",label_b="4"} 10
	`)
	require.NoError(t, err)

	return suite
}

func TestRuleEval(t *testing.T) {
	suite := setUpRuleEvalTest(t)
	defer suite.Close()

	require.NoError(t, suite.Run())

	for _, scenario := range ruleEvalTestScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			rule := NewRecordingRule(
				"test_rule",
				scenario.expr,
				scenario.ruleLabels,
				labels.EmptyLabels(),
				"",
				nil,
			)
			result, err := rule.Eval(
				suite.Context(),
				ruleEvaluationTime,
				EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
				nil,
				0,
			)
			require.NoError(t, err)
			require.Equal(t, scenario.expected, result)
		})
	}
}

func TestRecordingRuleLabelsUpdate(t *testing.T) {
	suite := setUpRuleEvalTest(t)
	defer suite.Close()

	require.NoError(t, suite.Run())

	rule := NewRecordingRule(
		"test_rule",
		exprWithMetricName,
		labels.FromStrings("label_c", "{{ if lt $value 4.0 }}foo{{ else }}bar{{ end }}"),
		labels.EmptyLabels(),
		"",
		nil,
	)
	result, err := rule.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)
	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings("__name__", "test_rule", "label_a", "1", "label_b", "3", "label_c", "foo"),
			F:      1,
		},
		promql.Sample{
			Metric: labels.FromStrings("__name__", "test_rule", "label_a", "2", "label_b", "4", "label_c", "bar"),
			F:      10,
		},
	}
	require.Equal(t, expected, result)
}

func TestRecordingRuleExternalLabelsInTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	ruleWithoutExternalLabels := NewRecordingRule(
		"ExternalLabelDoesNotExist",
		expr,
		labels.FromStrings(
			"templated_label",
			"There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}.",
		),
		labels.EmptyLabels(),
		"",
		nil,
	)
	ruleWithExternalLabels := NewRecordingRule(
		"ExternalLabelExists",
		expr,
		labels.FromStrings(
			"templated_label",
			"There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}.",
		),
		labels.FromStrings("foo", "bar", "dings", "bums"),
		"",
		nil,
	)

	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ExternalLabelDoesNotExist",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 0 external Labels, of which foo is .",
			),
			F: 75,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ExternalLabelExists",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 2 external Labels, of which foo is bar.",
			),
			F: 75,
		},
	}
	var results promql.Vector
	result, err := ruleWithoutExternalLabels.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)
	results = append(results, result...)
	result, err = ruleWithExternalLabels.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)
	results = append(results, result...)

	require.Equal(t, expected, results)
}

func TestRecordingRuleExternalURLInTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	ruleWithoutExternalURL := NewRecordingRule(
		"ExternalURLDoesNotExist",
		expr,
		labels.FromStrings("templated_label", "The external URL is {{ $externalURL }}."),
		labels.EmptyLabels(),
		"",
		nil,
	)
	ruleWithExternalURL := NewRecordingRule(
		"ExternalURLExists",
		expr,
		labels.FromStrings("templated_label", "The external URL is {{ $externalURL }}."),
		labels.EmptyLabels(),
		"http://localhost:1234",
		nil,
	)

	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ExternalURLDoesNotExist",
				"instance", "0",
				"job", "app-server",
				"templated_label", "The external URL is .",
			),
			F: 75,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ExternalURLExists",
				"instance", "0",
				"job", "app-server",
				"templated_label", "The external URL is http://localhost:1234.",
			),
			F: 75,
		},
	}
	var results promql.Vector
	result, err := ruleWithoutExternalURL.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)
	results = append(results, result...)
	result, err = ruleWithExternalURL.Eval(
		suite.Context(), ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)
	results = append(results, result...)

	require.Equal(t, expected, results)
}

func TestRecordingRuleEmptyLabelFromTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	rule := NewRecordingRule(
		"EmptyLabel",
		expr,
		labels.FromStrings("empty_label", ""),
		labels.EmptyLabels(),
		"",
		nil,
	)

	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "EmptyLabel",
				"instance", "0",
				"job", "app-server",
			),
			F: 75,
		},
	}
	result, err := rule.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)

	require.Equal(t, expected, result)
}

func TestRecordingRuleQueryInTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

	expr, err := parser.ParseExpr(`sum(http_requests) < 100`)
	require.NoError(t, err)

	rule := NewRecordingRule(
		"ruleWithQueryInTemplate",
		expr,
		labels.FromStrings(
			"label", "value",
			"templated_label", `{{- with "sort(sum(http_requests) by (instance))" | query -}}
{{- range $i,$v := . -}}
instance: {{ $v.Labels.instance }}, value: {{ printf "%.0f" $v.Value }};
{{- end -}}
{{- end -}}
`,
		),
		labels.EmptyLabels(),
		"",
		nil,
	)

	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ruleWithQueryInTemplate",
				"label", "value",
				"templated_label", "instance: 0, value: 75;",
			),
			F: 75,
		},
	}
	result, err := rule.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)

	require.Equal(t, expected, result)
}

func TestRecordingRuleExpendError(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	require.NoError(t, err)
	defer suite.Close()

	require.NoError(t, suite.Run())

	expr, err := parser.ParseExpr(`sum(http_requests) < 100`)
	require.NoError(t, err)

	rule := NewRecordingRule(
		"test_rule",
		expr,
		labels.FromStrings(
			"label", "value",
			"templated_label", `{{ $notExist }}`,
		),
		labels.EmptyLabels(),
		"",
		log.NewNopLogger(),
	)

	expected := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "test_rule",
				"label", "value",
				"templated_label",
				"<error expanding template: error parsing template __record_test_rule: "+
					"template: __record_test_rule:1: undefined variable \"$notExist\">",
			),
			F: 75,
		},
	}
	result, err := rule.Eval(
		suite.Context(),
		ruleEvaluationTime,
		EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		nil,
		0,
	)
	require.NoError(t, err)

	require.Equal(t, expected, result)
}

func BenchmarkRuleEval(b *testing.B) {
	suite := setUpRuleEvalTest(b)
	defer suite.Close()

	require.NoError(b, suite.Run())

	for _, scenario := range ruleEvalTestScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			rule := NewRecordingRule(
				"test_rule",
				scenario.expr,
				scenario.ruleLabels,
				labels.EmptyLabels(),
				"",
				nil,
			)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := rule.Eval(suite.Context(), ruleEvaluationTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil, 0)
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
	rule := NewRecordingRule(
		"foo",
		expr,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		"",
		nil,
	)
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
		labels.EmptyLabels(),
		"",
		nil,
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

	rule := NewRecordingRule(name, expr, lbs, labels.EmptyLabels(), "", nil)
	_, err = rule.Eval(ctx, now, func(ctx context.Context, qs string, _ time.Time) (promql.Vector, error) {
		detail = FromOriginContext(ctx)
		return nil, nil
	}, nil, 0)

	require.NoError(t, err)
	require.Equal(t, detail, NewRuleDetail(rule))
}
