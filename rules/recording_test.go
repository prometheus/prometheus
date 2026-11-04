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
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

type recordingAppenderV2 struct {
	gotLabels []labels.Labels
	gotMetas  []metadata.Metadata
}

func (a *recordingAppenderV2) AppenderV2(context.Context) storage.AppenderV2 { return a }

func (a *recordingAppenderV2) Append(_ storage.SeriesRef, ls labels.Labels, _, _ int64, _ float64, _ *histogram.Histogram, _ *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	a.gotLabels = append(a.gotLabels, ls.Copy())
	a.gotMetas = append(a.gotMetas, opts.Metadata)
	return 1, nil
}

func (*recordingAppenderV2) Commit() error   { return nil }
func (*recordingAppenderV2) Rollback() error { return nil }

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
		name                    string
		ruleLabels              labels.Labels
		ruleMeta                metadata.Metadata
		enableTypeAndUnitLabels bool
		samples                 promql.Vector
		wantLabels              []labels.Labels
		wantMetas               []metadata.Metadata
		wantErr                 error
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
			name:                    "float/preserved_schema_with_type_and_unit_labels",
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
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", "job", "api"),
				labels.FromStrings(model.MetricNameLabel, "job:req:rate5m", "version", "1.0"),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeCounter},
				{Type: model.MetricTypeInfo},
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
			name: "mismatch/float_sample_with_histogram_yaml_falls_back_to_gauge",
			ruleMeta: metadata.Metadata{
				Type: model.MetricTypeHistogram,
				Unit: "seconds",
				Help: "Mismatched float sample.",
			},
			enableTypeAndUnitLabels: true,
			samples: promql.Vector{
				{Metric: labels.FromStrings("job", "api"), T: evalMs, F: 0.42},
			},
			wantLabels: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "job:req:rate5m",
					model.MetricTypeLabel, "gauge",
					model.MetricUnitLabel, "seconds",
					"job", "api",
				),
			},
			wantMetas: []metadata.Metadata{
				{Type: model.MetricTypeGauge, Unit: "seconds", Help: "Mismatched float sample."},
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := testParser.ParseExpr("vector(1)")
			require.NoError(t, err)
			rule := NewRecordingRuleWithMetadata("job:req:rate5m", expr, tc.ruleLabels, tc.ruleMeta)
			require.Equal(t, tc.ruleMeta, rule.Metadata())

			app := &recordingAppenderV2{}
			qf := func(context.Context, string, time.Time) (promql.Vector, error) {
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
					AppendMetadata:          true,
					EnableTypeAndUnitLabels: tc.enableTypeAndUnitLabels,
				},
			})

			group.Eval(t.Context(), ts)
			if tc.wantErr != nil {
				require.ErrorIs(t, rule.LastError(), tc.wantErr)
				return
			}
			require.NoError(t, rule.LastError())
			require.Equal(t, tc.wantMetas, app.gotMetas)
			require.Len(t, app.gotLabels, len(tc.wantLabels))
			for i := range app.gotLabels {
				testutil.RequireEqual(t, tc.wantLabels[i], app.gotLabels[i])
			}
		})
	}
}
