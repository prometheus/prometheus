// Copyright The Prometheus Authors
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
	"math"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

var testParser = parser.NewParser(parser.Options{})

var (
	ruleEvaluationTime       = time.Unix(0, 0).UTC()
	exprWithMetricName, _    = testParser.ParseExpr(`sort(metric)`)
	exprWithoutMetricName, _ = testParser.ParseExpr(`sort(metric + metric)`)
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

func setUpRuleEvalTest(t testing.TB) *teststorage.TestStorage {
	return promqltest.LoadedStorage(t, `
		load 1m
			metric{label_a="1",label_b="3"} 1
			metric{label_a="2",label_b="4"} 10
	`)
}

func TestRuleEval(t *testing.T) {
	storage := setUpRuleEvalTest(t)

	ng := testEngine(t)
	for _, scenario := range ruleEvalTestScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			rule := NewRecordingRule("test_rule", scenario.expr, scenario.ruleLabels)
			result, err := rule.Eval(context.TODO(), 0, ruleEvaluationTime, EngineQueryFunc(ng, storage), nil, 0)
			require.NoError(t, err)
			testutil.RequireEqual(t, scenario.expected, result)
		})
	}
}

func BenchmarkRuleEval(b *testing.B) {
	storage := setUpRuleEvalTest(b)
	b.Cleanup(func() { storage.Close() })

	ng := testEngine(b)
	for _, scenario := range ruleEvalTestScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			rule := NewRecordingRule("test_rule", scenario.expr, scenario.ruleLabels)

			b.ResetTimer()

			for b.Loop() {
				_, err := rule.Eval(context.TODO(), 0, ruleEvaluationTime, EngineQueryFunc(ng, storage), nil, 0)
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

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}

	engine := promqltest.NewTestEngineWithOpts(t, opts)
	ctx := t.Context()

	now := time.Now()

	expr, _ := testParser.ParseExpr(`vector(0) or label_replace(vector(0),"test","x","","")`)
	rule := NewRecordingRule("foo", expr, labels.FromStrings("test", "test"))
	_, err := rule.Eval(ctx, 0, now, EngineQueryFunc(engine, storage), nil, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDuplicateRecordingLabelSet)
}

func TestRecordingRuleLimit(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			metric{label="1"} 1
			metric{label="2"} 1
	`)

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

	expr, _ := testParser.ParseExpr(`metric > 0`)
	rule := NewRecordingRule(
		"foo",
		expr,
		labels.FromStrings("test", "test"),
	)

	ng := testEngine(t)
	evalTime := time.Unix(0, 0)

	for _, test := range tests {
		switch _, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, test.limit); {
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

	expr, err := testParser.ParseExpr(query)
	require.NoError(t, err)

	rule := NewRecordingRule(name, expr, lbs)
	_, err = rule.Eval(ctx, 0, now, func(ctx context.Context, _ string, _ time.Time) (promql.Vector, error) {
		detail = FromOriginContext(ctx)
		return nil, nil
	}, nil, 0)

	require.NoError(t, err)
	require.Equal(t, detail, NewRuleDetail(rule))
}

func TestRecordingRule_SetDependentRules(t *testing.T) {
	dependentRule := NewRecordingRule("test1", nil, labels.EmptyLabels())

	rule := NewRecordingRule("1", &parser.NumberLiteral{Val: 1}, labels.EmptyLabels())
	require.False(t, rule.NoDependentRules())

	rule.SetDependentRules([]Rule{dependentRule})
	require.False(t, rule.NoDependentRules())
	require.Equal(t, []Rule{dependentRule}, rule.DependentRules())

	rule.SetDependentRules([]Rule{})
	require.True(t, rule.NoDependentRules())
	require.Empty(t, rule.DependentRules())
}

func TestRecordingRule_SetDependencyRules(t *testing.T) {
	dependencyRule := NewRecordingRule("test1", nil, labels.EmptyLabels())

	rule := NewRecordingRule("1", &parser.NumberLiteral{Val: 1}, labels.EmptyLabels())
	require.False(t, rule.NoDependencyRules())

	rule.SetDependencyRules([]Rule{dependencyRule})
	require.False(t, rule.NoDependencyRules())
	require.Equal(t, []Rule{dependencyRule}, rule.DependencyRules())

	rule.SetDependencyRules([]Rule{})
	require.True(t, rule.NoDependencyRules())
	require.Empty(t, rule.DependencyRules())
}

func TestRecordingRuleMetadataDeduction(t *testing.T) {
	ts := time.Unix(60, 0).UTC()
	evalMs := timestamp.FromTime(ts)
	counterHist := &histogram.FloatHistogram{
		Schema:           0,
		Count:            10,
		Sum:              25,
		CounterResetHint: histogram.UnknownCounterReset,
	}
	gaugeHist := &histogram.FloatHistogram{
		Schema:           0,
		Count:            2,
		Sum:              5,
		CounterResetHint: histogram.GaugeType,
	}

	for _, tc := range []struct {
		name                         string
		ruleLabels                   labels.Labels
		ruleMeta                     metadata.Metadata
		enableTypeAndUnitLabels      bool
		disableAutoRecordingMetadata bool
		samples                      promql.Vector
		wantLabels                   []labels.Labels
		wantMetas                    []metadata.Metadata
		wantErr                      error
	}{
		{
			name:                    "float/schema_dropped_defaults_to_gauge",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("job", "api"), T: evalMs, F: 1.5},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", model.MetricTypeLabel, "gauge", "job", "api"),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGauge},
			},
		},
		{
			name:                    "float/schema_dropped_ignores_type_and_unit_labels",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 1.5,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", model.MetricTypeLabel, "gauge", "job", "api"),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGauge},
			},
		},
		{
			name:                    "float/preserved_schema_with_type_and_unit_labels",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests",
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 42,
				},
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "legacy_gauge_total",
						model.MetricTypeLabel, "gauge",
						model.MetricUnitLabel, "bytes",
						"job", "db",
					),
					T: evalMs,
					F: 7,
				},
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_request_duration_seconds_bucket",
						model.MetricTypeLabel, "histogram",
						model.MetricUnitLabel, "seconds",
						"job", "api",
						"le", "+Inf",
					),
					T: evalMs,
					F: 12,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "counter",
					model.MetricUnitLabel, "requests",
					"job", "api",
				),
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gauge",
					model.MetricUnitLabel, "bytes",
					"job", "db",
				),
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "histogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
					"le", "+Inf",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeCounter, Unit: "requests"},
				{Type: model.MetricTypeGauge, Unit: "bytes"},
				{Type: model.MetricTypeHistogram, Unit: "seconds"},
			},
		},
		{
			name:                    "float/preserved_schema_with_type_and_unit_labels_suffix_fallback_when_type_missing",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 42,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "counter",
					model.MetricUnitLabel, "requests",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeCounter, Unit: "requests"},
			},
		},
		{
			name:                    "float/preserved_schema_without_type_and_unit_labels_suffix_fallback",
			enableTypeAndUnitLabels: false,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "gauge",
						model.MetricUnitLabel, "ignored_when_flag_disabled",
						"job", "api",
					),
					T: evalMs,
					F: 42,
				},
				{
					Metric: labels.FromStrings(model.MetricNameLabel, "target_info", "version", "1.0"),
					T:      evalMs,
					F:      1,
				},
				{
					Metric: labels.FromStrings(model.MetricNameLabel, "http_request_duration_seconds_bucket", "le", "0.5"),
					T:      evalMs,
					F:      3,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", "job", "api"),
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", "version", "1.0"),
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", "le", "0.5"),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeCounter},
				{Type: model.MetricTypeInfo},
				{Type: model.MetricTypeHistogram},
			},
		},
		{
			name:                    "histogram/deduces_counter_vs_gauge_from_reset_hint",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("kind", "sum"), T: evalMs, H: counterHist},
				{Metric: labels.FromStrings("kind", "rate"), T: evalMs, H: gaugeHist},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", model.MetricTypeLabel, "histogram", "kind", "sum"),
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", model.MetricTypeLabel, "gaugehistogram", "kind", "rate"),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeHistogram},
				{Type: model.MetricTypeGaugeHistogram},
			},
		},
		{
			name:                    "histogram/preserved_schema_with_type_and_unit_labels",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "request_duration_seconds",
						model.MetricTypeLabel, "gaugehistogram",
						model.MetricUnitLabel, "seconds",
						"job", "api",
					),
					T: evalMs,
					H: counterHist,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gaugehistogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGaugeHistogram, Unit: "seconds"},
			},
		},
		{
			name: "histogram/yaml_override_compatible_type",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeGaugeHistogram,
				Unit: "seconds",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("job", "api"), T: evalMs, H: counterHist},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gaugehistogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGaugeHistogram, Unit: "seconds"},
			},
		},
		{
			name:       "yaml_overrides/explicit_help_type_unit_and_user_rule_labels",
			ruleLabels: labels.FromStrings(model.MetricTypeLabel, "gauge", model.MetricUnitLabel, "bytes"),
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeCounter,
				Unit: "requests_per_second",
				Help: "Recorded request rate.",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "gauge",
						model.MetricUnitLabel, "bytes",
						"job", "api",
					),
					T: evalMs,
					F: 5,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "counter",
					model.MetricUnitLabel, "requests_per_second",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{
					Type: model.MetricTypeCounter,
					Unit: "requests_per_second",
					Help: "Recorded request rate.",
				},
			},
		},
		{
			name:       "yaml_overrides/not_injected_as_labels_when_flag_disabled",
			ruleLabels: labels.FromStrings(model.MetricTypeLabel, "gauge", model.MetricUnitLabel, "bytes"),
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeCounter,
				Unit: "requests_per_second",
				Help: "Recorded request rate.",
			},
			enableTypeAndUnitLabels: false,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "gauge",
						model.MetricUnitLabel, "bytes",
						"job", "api",
					),
					T: evalMs,
					F: 5,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{
					Type: model.MetricTypeCounter,
					Unit: "requests_per_second",
					Help: "Recorded request rate.",
				},
			},
		},
		{
			name: "yaml_overrides/float_sample_with_histogram_yaml_uses_classic_histogram",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeHistogram,
				Unit: "seconds",
				Help: "Classic histogram bucket.",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("job", "api", "le", "+Inf"), T: evalMs, F: 42},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "histogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
					"le", "+Inf",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeHistogram, Unit: "seconds", Help: "Classic histogram bucket."},
			},
		},
		{
			name: "mismatch/incompatible_yaml_type_falls_back_to_preserved_type",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeGauge,
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "request_duration_seconds",
						model.MetricTypeLabel, "gaugehistogram",
						model.MetricUnitLabel, "seconds",
						"job", "api",
					),
					T: evalMs,
					H: counterHist,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gaugehistogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGaugeHistogram, Unit: "seconds"},
			},
		},
		{
			name:                    "mismatch/invalid_preserved_type_label_falls_back_to_gauge",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "custom_metric",
						model.MetricTypeLabel, "invalid_type",
						"job", "api",
					),
					T: evalMs,
					F: 1,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gauge",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGauge},
			},
		},
		{
			name: "mismatch/histogram_sample_with_gauge_yaml_falls_back_to_reset_hint",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeGauge,
				Unit: "seconds",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("job", "api"), T: evalMs, H: gaugeHist},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gaugehistogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGaugeHistogram, Unit: "seconds"},
			},
		},
		{
			name:                    "mismatch/histogram_sample_with_preserved_float_type_falls_back_to_reset_hint",
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "my_hist_total",
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "seconds",
						"job", "api",
					),
					T: evalMs,
					H: gaugeHist,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gaugehistogram",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGaugeHistogram, Unit: "seconds"},
			},
		},
		{
			name: "yaml_overrides/unknown_type_treated_as_unset",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeUnknown,
				Help: "Help with unknown type.",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 10,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "counter",
					model.MetricUnitLabel, "requests",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{
					Type: model.MetricTypeCounter,
					Unit: "requests",
					Help: "Help with unknown type.",
				},
			},
		},
		{
			name: "duplicate_detection/or_series_differing_only_by_input_type_caught",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeGauge,
				Unit: "seconds",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(model.MetricNameLabel, "m1", model.MetricTypeLabel, "counter", "job", "api"),
					T:      evalMs,
					F:      1,
				},
				{
					Metric: labels.FromStrings(model.MetricNameLabel, "m2", model.MetricTypeLabel, "gauge", "job", "api"),
					T:      evalMs,
					F:      2,
				},
			},
			wantErr: ErrDuplicateRecordingLabelSet,
		},
		{
			name:       "auto_recording_disabled/configured_metadata_injected",
			ruleLabels: labels.FromStrings(model.MetricTypeLabel, "counter", model.MetricUnitLabel, "bytes"),
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeGauge,
				Unit: "requests_per_second",
				Help: "Static configured metadata.",
			},
			enableTypeAndUnitLabels:      true,
			disableAutoRecordingMetadata: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 5,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gauge",
					model.MetricUnitLabel, "requests_per_second",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{
					Type: model.MetricTypeGauge,
					Unit: "requests_per_second",
					Help: "Static configured metadata.",
				},
			},
		},
		{
			name:                         "auto_recording_disabled/unannotated_strips_schema_labels_without_deduction",
			enableTypeAndUnitLabels:      true,
			disableAutoRecordingMetadata: true,
			samples: promql.Vector{
				{
					Metric: labels.FromStrings(
						model.MetricNameLabel, "http_requests_total",
						model.MetricTypeLabel, "counter",
						model.MetricUnitLabel, "requests",
						"job", "api",
					),
					T: evalMs,
					F: 5,
				},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, appendMetadata := range []bool{false, true} {
				expr, err := testParser.ParseExpr("input_metric")
				require.NoError(t, err)
				rule := NewRecordingRuleWithMetadata("job:req:rate5m", expr, tc.ruleLabels, tc.ruleMeta)
				require.Equal(t, tc.ruleMeta, rule.Metadata())

				app := teststorage.NewAppendable()
				returnEmpty := false
				qf := func(context.Context, string, time.Time) (promql.Vector, error) {
					if returnEmpty {
						return nil, nil
					}
					out := make(promql.Vector, len(tc.samples))
					copy(out, tc.samples)
					return out, nil
				}
				group := NewGroup(GroupOptions{
					Name:     "test-group",
					File:     "test.rules",
					Interval: time.Minute,
					Rules:    []Rule{rule},
					Opts: &ManagerOptions{
						AppendableV2:            app,
						QueryFunc:               qf,
						AppendMetadata:          appendMetadata,
						EnableTypeAndUnitLabels: tc.enableTypeAndUnitLabels,
						AutoRecordingMetadata:   !tc.disableAutoRecordingMetadata,
					},
				})

				group.Eval(t.Context(), ts)
				if tc.wantErr != nil {
					require.ErrorIs(t, rule.LastError(), tc.wantErr)
					require.Empty(t, app.ResultSamples())
					continue
				}
				require.NoError(t, rule.LastError())
				wantMetas := make([]metadata.Metadata, len(tc.wantLabels))
				if appendMetadata {
					wantMetas = tc.wantMetas
				}
				wantSamples := make([]teststorage.Sample, len(tc.wantLabels))
				for i := range tc.wantLabels {
					wantSamples[i] = teststorage.Sample{
						L:  tc.wantLabels[i],
						M:  wantMetas[i],
						T:  tc.samples[i].T,
						V:  tc.samples[i].F,
						FH: tc.samples[i].H,
					}
				}
				teststorage.RequireEqual(t, wantSamples, app.ResultSamples())

				// Verify staleness appends on the next evaluation preserve deduced labels and metadata per series.
				app.ResultReset()
				returnEmpty = true
				staleTs := ts.Add(time.Minute)
				group.Eval(t.Context(), staleTs)
				require.NoError(t, rule.LastError())
				wantStale := make([]teststorage.Sample, len(tc.wantLabels))
				for i := range tc.wantLabels {
					wantStale[i] = teststorage.Sample{
						L: tc.wantLabels[i],
						M: wantMetas[i],
						T: timestamp.FromTime(staleTs),
						V: math.Float64frombits(value.StaleNaN),
					}
				}
				teststorage.RequireEqual(t, wantStale, app.ResultSamples())
			}
		})
	}
}

func TestRecordingRuleMetadataProvenance(t *testing.T) {
	queryStore := promqltest.LoadedStorage(t, `
		load 1m
			http_requests_total{__type__="counter",__unit__="requests",job="api"} 0+60x5
	`)
	for _, delayed := range []bool{false, true} {
		engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
			MaxSamples:               10000,
			Timeout:                  time.Minute,
			EnableDelayedNameRemoval: delayed,
		})
		for _, schemaLabels := range []bool{false, true} {
			for _, tc := range []struct {
				expr            string
				preservesSchema bool
			}{
				{`http_requests_total`, true},
				{`(http_requests_total)`, true},
				{`http_requests_total > 0`, true},
				{`0 < http_requests_total`, true},
				{`sort(http_requests_total)`, true},
				{`topk(1, http_requests_total)`, true},
				{`label_replace(http_requests_total, "service", "$1", "job", "(.*)")`, true},
				{`label_join(http_requests_total, "service", "-", "job")`, true},
				{`http_requests_total and http_requests_total`, true},
				{`http_requests_total or http_requests_total`, true},
				{`count by (__name__) (http_requests_total)`, false},
				{`count by (__name__, __type__, __unit__) (http_requests_total)`, false},
				{`label_replace(rate(http_requests_total[5m]), "__name__", "derived_total", "", ".*")`, false},
				{`label_join(rate(http_requests_total[5m]), "__name__", "", "job")`, false},
				{`label_replace(http_requests_total, "__type__", "info", "", ".*")`, false},
				{`label_replace(http_requests_total, "__unit__", "seconds", "", ".*")`, false},
				{`sort(count by (__name__) (http_requests_total))`, false},
				{`topk(1, count by (__name__) (http_requests_total))`, false},
				{`count by (__name__) (http_requests_total) > 0`, false},
				{`http_requests_total > bool 0`, false},
				{`http_requests_total or label_replace(vector(1), "__name__", "derived_total", "", ".*")`, false},
			} {
				t.Run(fmt.Sprintf("delayed=%t/schema_labels=%t/%s", delayed, schemaLabels, tc.expr), func(t *testing.T) {
					expr, err := testParser.ParseExpr(tc.expr)
					require.NoError(t, err)
					rule := NewRecordingRule("recorded", expr, labels.EmptyLabels())
					app := teststorage.NewAppendable()
					group := NewGroup(GroupOptions{
						Name:     "test",
						File:     "test.rules",
						Interval: time.Minute,
						Rules:    []Rule{rule},
						Opts: &ManagerOptions{
							AppendableV2:            app,
							QueryFunc:               EngineQueryFunc(engine, queryStore),
							AppendMetadata:          true,
							EnableTypeAndUnitLabels: schemaLabels,
							AutoRecordingMetadata:   true,
						},
					})
					group.Eval(t.Context(), time.Unix(300, 0))
					require.NoError(t, rule.LastError())
					samples := app.ResultSamples()
					require.NotEmpty(t, samples)
					want := metadata.Metadata{Type: model.MetricTypeGauge}
					if tc.preservesSchema {
						want.Type = model.MetricTypeCounter
						if schemaLabels {
							want.Unit = "requests"
						}
					}
					for _, sample := range samples {
						require.Equal(t, want, sample.M)
						if schemaLabels {
							require.Equal(t, string(want.Type), sample.L.Get(model.MetricTypeLabel))
							require.Equal(t, want.Unit, sample.L.Get(model.MetricUnitLabel))
						} else {
							require.False(t, sample.L.Has(model.MetricTypeLabel))
							require.False(t, sample.L.Has(model.MetricUnitLabel))
						}
					}
				})
			}
		}
	}
}
