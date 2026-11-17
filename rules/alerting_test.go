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
	"errors"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

func testEngine(tb testing.TB) *promql.Engine {
	tb.Helper()
	return promqltest.NewTestEngineWithOpts(tb, promql.EngineOpts{
		Logger:                   nil,
		Reg:                      nil,
		MaxSamples:               10000,
		Timeout:                  100 * time.Second,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
		EnablePerStepStats:       true,
	})
}

func TestAlertingRuleState(t *testing.T) {
	tests := []struct {
		name   string
		active map[uint64]*Alert
		want   AlertState
	}{
		{
			name: "MaxStateFiring",
			active: map[uint64]*Alert{
				0: {State: StatePending},
				1: {State: StateFiring},
			},
			want: StateFiring,
		},
		{
			name: "MaxStatePending",
			active: map[uint64]*Alert{
				0: {State: StateInactive},
				1: {State: StatePending},
			},
			want: StatePending,
		},
		{
			name: "MaxStateInactive",
			active: map[uint64]*Alert{
				0: {State: StateInactive},
				1: {State: StateInactive},
			},
			want: StateInactive,
		},
	}

	for i, test := range tests {
		rule := NewAlertingRule(test.name, nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil)
		rule.active = test.active
		// Set evaluation timestamp to simulate that the rule has been evaluated
		rule.SetEvaluationTimestamp(time.Now())
		got := rule.State()
		require.Equal(t, test.want, got, "test case %d unexpected AlertState, want:%d got:%d", i, test.want, got)
	}
}

func TestAlertingRuleTemplateWithHistogram(t *testing.T) {
	h := histogram.FloatHistogram{
		Schema:        0,
		Count:         30,
		Sum:           1111.1,
		ZeroThreshold: 0.001,
		ZeroCount:     2,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 1},
			{Offset: 1, Length: 5},
		},
		PositiveBuckets: []float64{1, 1, 2, 1, 1, 1},
		NegativeSpans: []histogram.Span{
			{Offset: 1, Length: 4},
			{Offset: 4, Length: 3},
		},
		NegativeBuckets: []float64{-2, 2, 2, 7, 5, 5, 2},
	}

	q := func(context.Context, string, time.Time) (promql.Vector, error) {
		return []promql.Sample{{H: &h}}, nil
	}

	expr, err := testParser.ParseExpr("foo")
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HistogramAsValue",
		expr,
		time.Minute,
		0,
		labels.FromStrings("histogram", "{{ $value }}"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	evalTime := time.Now()
	res, err := rule.Eval(context.TODO(), 0, evalTime, q, nil, 0)
	require.NoError(t, err)

	require.Len(t, res, 2)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			result := promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HistogramAsValue",
					"alertstate", "pending",
					"histogram", h.String(),
				),
				T: timestamp.FromTime(evalTime),
				F: 1,
			}
			testutil.RequireEqual(t, result, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}
}

func TestAlertingRuleLabelsUpdate(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70 stale
	`)

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		0,
		// Basing alerting rule labels off of a value that can change is a very bad idea.
		// If an alert is going back and forth between two label values it will never fire.
		// Instead, you should write two alerts with constant labels.
		labels.FromStrings("severity", "{{ if lt $value 80.0 }}critical{{ else }}warning{{ end }}"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	results := []promql.Vector{
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "warning",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				F: 1,
			},
		},
	}

	ng := testEngine(t)

	baseTime := time.Unix(0, 0)
	for i, result := range results {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(time.Duration(i) * time.Minute)
		result[0].T = timestamp.FromTime(evalTime)
		res, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0)
		require.NoError(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
				require.Equal(t, "ALERTS_FOR_STATE", smplName)
			}
		}

		testutil.RequireEqual(t, result, filteredRes)
	}
	evalTime := baseTime.Add(time.Duration(len(results)) * time.Minute)
	res, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestAlertingRuleExternalLabelsInTemplate(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	ruleWithoutExternalLabels := NewAlertingRule(
		"ExternalLabelDoesNotExist",
		expr,
		time.Minute,
		0,
		labels.FromStrings("templated_label", "There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}."),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	ruleWithExternalLabels := NewAlertingRule(
		"ExternalLabelExists",
		expr,
		time.Minute,
		0,
		labels.FromStrings("templated_label", "There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}."),
		labels.EmptyLabels(),
		labels.FromStrings("foo", "bar", "dings", "bums"),
		"",
		true, promslog.NewNopLogger(),
	)
	result := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalLabelDoesNotExist",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 0 external Labels, of which foo is .",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalLabelExists",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 2 external Labels, of which foo is bar.",
			),
			F: 1,
		},
	}

	ng := testEngine(t)

	evalTime := time.Unix(0, 0)
	result[0].T = timestamp.FromTime(evalTime)
	result[1].T = timestamp.FromTime(evalTime)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := ruleWithoutExternalLabels.Eval(
		context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0,
	)
	require.NoError(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	res, err = ruleWithExternalLabels.Eval(
		context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0,
	)
	require.NoError(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	testutil.RequireEqual(t, result, filteredRes)
}

func TestAlertingRuleExternalURLInTemplate(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	ruleWithoutExternalURL := NewAlertingRule(
		"ExternalURLDoesNotExist",
		expr,
		time.Minute,
		0,
		labels.FromStrings("templated_label", "The external URL is {{ $externalURL }}."),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	ruleWithExternalURL := NewAlertingRule(
		"ExternalURLExists",
		expr,
		time.Minute,
		0,
		labels.FromStrings("templated_label", "The external URL is {{ $externalURL }}."),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"http://localhost:1234",
		true, promslog.NewNopLogger(),
	)
	result := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalURLDoesNotExist",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "The external URL is .",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalURLExists",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "The external URL is http://localhost:1234.",
			),
			F: 1,
		},
	}

	evalTime := time.Unix(0, 0)
	result[0].T = timestamp.FromTime(evalTime)
	result[1].T = timestamp.FromTime(evalTime)

	ng := testEngine(t)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := ruleWithoutExternalURL.Eval(
		context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0,
	)
	require.NoError(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	res, err = ruleWithExternalURL.Eval(
		context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0,
	)
	require.NoError(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	testutil.RequireEqual(t, result, filteredRes)
}

func TestAlertingRuleEmptyLabelFromTemplate(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"EmptyLabel",
		expr,
		time.Minute,
		0,
		labels.FromStrings("empty_label", ""),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	result := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "EmptyLabel",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
			),
			F: 1,
		},
	}

	evalTime := time.Unix(0, 0)
	result[0].T = timestamp.FromTime(evalTime)

	ng := testEngine(t)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := rule.Eval(
		context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0,
	)
	require.NoError(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}
	testutil.RequireEqual(t, result, filteredRes)
}

func TestAlertingRuleQueryInTemplate(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	70 85 70 70
	`)

	expr, err := testParser.ParseExpr(`sum(http_requests) < 100`)
	require.NoError(t, err)

	ruleWithQueryInTemplate := NewAlertingRule(
		"ruleWithQueryInTemplate",
		expr,
		time.Minute,
		0,
		labels.FromStrings("label", "value"),
		labels.FromStrings("templated_label", `{{- with "sort(sum(http_requests) by (instance))" | query -}}
{{- range $i,$v := . -}}
instance: {{ $v.Labels.instance }}, value: {{ printf "%.0f" $v.Value }};
{{- end -}}
{{- end -}}
`),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	evalTime := time.Unix(0, 0)

	ng := testEngine(t)

	startQueryCh := make(chan struct{})
	getDoneCh := make(chan struct{})
	slowQueryFunc := func(ctx context.Context, q string, ts time.Time) (promql.Vector, error) {
		if q == "sort(sum(http_requests) by (instance))" {
			// This is a minimum reproduction of issue 10703, expand template with query.
			close(startQueryCh)
			select {
			case <-getDoneCh:
			case <-time.After(20 * time.Millisecond):
				// Assert no blocking when template expanding.
				buf := make([]byte, 1<<20)
				n := runtime.Stack(buf, true)
				t.Logf("goroutine dump:\n%s", buf[:n])
				require.Fail(t, "unexpected blocking when template expanding.")
			}
		}
		return EngineQueryFunc(ng, storage)(ctx, q, ts)
	}
	go func() {
		<-startQueryCh
		_ = ruleWithQueryInTemplate.Health()
		_ = ruleWithQueryInTemplate.LastError()
		_ = ruleWithQueryInTemplate.GetEvaluationDuration()
		_ = ruleWithQueryInTemplate.GetEvaluationTimestamp()
		close(getDoneCh)
	}()
	_, err = ruleWithQueryInTemplate.Eval(
		context.TODO(), 0, evalTime, slowQueryFunc, nil, 0,
	)
	require.NoError(t, err)
}

func BenchmarkAlertingRuleAtomicField(b *testing.B) {
	b.ReportAllocs()
	rule := NewAlertingRule("bench", nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil)
	done := make(chan struct{})
	go func() {
		for b.Loop() {
			rule.GetEvaluationTimestamp()
		}
		close(done)
	}()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rule.SetEvaluationTimestamp(time.Now())
		}
	})
	<-done
}

func TestAlertingRuleDuplicate(t *testing.T) {
	evalTime := time.Unix(600, 0).UTC()
	cases := []struct {
		name                    string
		series                  []labels.Labels
		expr                    string
		ruleLabels              labels.Labels
		enableTypeAndUnitLabels bool
	}{
		{
			name:       "rule_label_override",
			expr:       `vector(0) or label_replace(vector(0),"test","x","","")`,
			ruleLabels: labels.FromStrings("test", "test"),
		},
		{
			name: "differing_only_by_type",
			series: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "http_errors_total",
					model.MetricTypeLabel, string(model.MetricTypeCounter),
					"job", "api",
				),
				labels.FromStrings(
					model.MetricNameLabel, "http_errors_total",
					model.MetricTypeLabel, string(model.MetricTypeGauge),
					"job", "api",
				),
			},
			expr:                    `http_errors_total > 10`,
			ruleLabels:              labels.EmptyLabels(),
			enableTypeAndUnitLabels: true,
		},
		{
			name: "differing_only_by_unit",
			series: []labels.Labels{
				labels.FromStrings(
					model.MetricNameLabel, "http_errors_total",
					model.MetricTypeLabel, string(model.MetricTypeCounter),
					model.MetricUnitLabel, "errors",
					"job", "api",
				),
				labels.FromStrings(
					model.MetricNameLabel, "http_errors_total",
					model.MetricTypeLabel, string(model.MetricTypeCounter),
					model.MetricUnitLabel, "requests",
					"job", "api",
				),
			},
			expr:                    `http_errors_total > 10`,
			ruleLabels:              labels.EmptyLabels(),
			enableTypeAndUnitLabels: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := teststorage.New(t)
			if len(tc.series) > 0 {
				app := st.AppenderV2(t.Context())
				for _, lbls := range tc.series {
					_, err := app.Append(0, lbls, 0, timestamp.FromTime(evalTime), 42, nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			}

			engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
				MaxSamples:              10,
				Timeout:                 10 * time.Second,
				EnableTypeAndUnitLabels: tc.enableTypeAndUnitLabels,
			})

			expr, err := testParser.ParseExpr(tc.expr)
			require.NoError(t, err)
			rule := NewAlertingRule(
				"foo",
				expr,
				time.Minute,
				0,
				tc.ruleLabels,
				labels.EmptyLabels(),
				labels.EmptyLabels(),
				"",
				true,
				promslog.NewNopLogger(),
			)
			_, err = rule.Eval(t.Context(), 0, evalTime, EngineQueryFunc(engine, st), nil, 0)
			require.ErrorIs(t, err, ErrDuplicateAlertLabelSet)
		})
	}
}

func TestAlertingRuleLimit(t *testing.T) {
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
			err:   "exceeded limit of 1 with 2 alerts",
		},
	}

	expr, _ := testParser.ParseExpr(`metric > 0`)
	rule := NewAlertingRule(
		"foo",
		expr,
		time.Minute,
		0,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)

	evalTime := time.Unix(0, 0)
	ng := testEngine(t)
	for _, test := range tests {
		switch _, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, test.limit); {
		case err != nil:
			require.EqualError(t, err, test.err)
		case test.err != "":
			t.Errorf("Expected error %s, got none", test.err)
		}
	}
}

func TestQueryForStateSeries(t *testing.T) {
	testError := errors.New("test error")

	type testInput struct {
		selectMockFunction func(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet
		expectedSeries     storage.Series
		expectedError      error
	}

	tests := []testInput{
		// Test for empty series.
		{
			selectMockFunction: func(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
				return storage.EmptySeriesSet()
			},
			expectedSeries: nil,
			expectedError:  nil,
		},
		// Test for error series.
		{
			selectMockFunction: func(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
				return storage.ErrSeriesSet(testError)
			},
			expectedSeries: nil,
			expectedError:  testError,
		},
		// Test for mock series.
		{
			selectMockFunction: func(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
				return storage.TestSeriesSet(storage.MockSeries(
					nil,
					[]int64{1, 2, 3},
					[]float64{1, 2, 3},
					[]string{"__name__", "ALERTS_FOR_STATE", "alertname", "TestRule", "severity", "critical"},
				))
			},
			expectedSeries: storage.MockSeries(
				nil,
				[]int64{1, 2, 3},
				[]float64{1, 2, 3},
				[]string{"__name__", "ALERTS_FOR_STATE", "alertname", "TestRule", "severity", "critical"},
			),
			expectedError: nil,
		},
	}

	testFunc := func(tst testInput) {
		querier := &storage.MockQuerier{
			SelectMockFunction: tst.selectMockFunction,
		}

		rule := NewAlertingRule(
			"TestRule",
			nil,
			time.Minute,
			0,
			labels.FromStrings("severity", "critical"),
			labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
		)

		sample := rule.forStateSample(nil, time.Time{}, 0)

		seriesSet, err := rule.QueryForStateSeries(context.Background(), querier)

		var series storage.Series
		for seriesSet.Next() {
			if seriesSet.At().Labels().Len() == sample.Metric.Len() {
				series = seriesSet.At()
				break
			}
		}

		require.Equal(t, tst.expectedSeries, series)
		require.Equal(t, tst.expectedError, err)
	}

	for _, tst := range tests {
		testFunc(tst)
	}
}

// TestQueryForStateSeriesTemplateLabels verifies that labels containing Go
// template syntax are excluded from the matchers used to query ALERTS_FOR_STATE,
// so that per-instance expanded values stored in the series can still be found.
func TestQueryForStateSeriesTemplateLabels(t *testing.T) {
	var gotMatchers []*labels.Matcher
	querier := &storage.MockQuerier{
		SelectMockFunction: func(_ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
			gotMatchers = matchers
			return storage.EmptySeriesSet()
		},
	}

	rule := NewAlertingRule(
		"TestRule",
		nil,
		time.Minute,
		0,
		labels.FromStrings("severity", "critical", "instance_ext", "instance_{{ $labels.instance }}"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	_, err := rule.QueryForStateSeries(context.Background(), querier)
	require.NoError(t, err)

	for _, m := range gotMatchers {
		require.NotContains(t, m.Value, "{{", "template label %q must not appear in Select matchers", m.Name)
	}

	// The static label must still be present.
	found := false
	for _, m := range gotMatchers {
		if m.Name == "severity" && m.Value == "critical" {
			found = true
			break
		}
	}
	require.True(t, found, "static label severity=critical must be present in Select matchers")
}

// TestSendAlertsDontAffectActiveAlerts tests a fix for https://github.com/prometheus/prometheus/issues/11424.
func TestSendAlertsDontAffectActiveAlerts(t *testing.T) {
	rule := NewAlertingRule(
		"TestRule",
		nil,
		time.Minute,
		0,
		labels.FromStrings("severity", "critical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	// Set an active alert.
	lbls := labels.FromStrings("a1", "1")
	h := lbls.Hash()
	al := &Alert{State: StateFiring, Labels: lbls, ActiveAt: time.Now()}
	rule.active[h] = al

	expr, err := testParser.ParseExpr("foo")
	require.NoError(t, err)
	rule.vector = expr

	// The relabel rule reproduced the bug here.
	opts := notifier.Options{
		QueueCapacity: 1,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{"a1"},
				Regex:                relabel.MustNewRegexp("(.+)"),
				TargetLabel:          "a1",
				Replacement:          "bug",
				Action:               "replace",
				NameValidationScheme: model.UTF8Validation,
			},
		},
	}
	nm := notifier.NewManager(&opts, model.UTF8Validation, promslog.NewNopLogger())

	f := SendAlerts(nm, "")
	notifyFunc := func(ctx context.Context, expr string, alerts ...*Alert) {
		require.Len(t, alerts, 1)
		require.Equal(t, al, alerts[0])
		f(ctx, expr, alerts...)
	}

	rule.sendAlerts(context.Background(), time.Now(), 0, 0, notifyFunc)
	nm.Stop()

	// The relabel rule changes a1=1 to a1=bug.
	// But the labels with the AlertingRule should not be changed.
	testutil.RequireEqual(t, labels.FromStrings("a1", "1"), rule.active[h].Labels)
}

func TestKeepFiringFor(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70 10x5
	`)

	expr, err := testParser.ParseExpr(`http_requests > 50`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateHigh",
		expr,
		time.Minute,
		time.Minute,
		labels.EmptyLabels(),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	results := []promql.Vector{
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateHigh",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateHigh",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateHigh",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
				),
				F: 1,
			},
		},
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateHigh",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
				),
				F: 1,
			},
		},
		// From now on the alert should keep firing.
		{
			promql.Sample{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateHigh",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
				),
				F: 1,
			},
		},
	}

	ng := testEngine(t)
	baseTime := time.Unix(0, 0)
	for i, result := range results {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(time.Duration(i) * time.Minute)
		result[0].T = timestamp.FromTime(evalTime)
		res, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0)
		require.NoError(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
				require.Equal(t, "ALERTS_FOR_STATE", smplName)
			}
		}

		testutil.RequireEqual(t, result, filteredRes)
	}
	evalTime := baseTime.Add(time.Duration(len(results)) * time.Minute)
	res, err := rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestPendingAndKeepFiringFor(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 10x10
	`)

	expr, err := testParser.ParseExpr(`http_requests > 50`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateHigh",
		expr,
		time.Minute,
		time.Minute,
		labels.EmptyLabels(),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	result := promql.Sample{
		Metric: labels.FromStrings(
			"__name__", "ALERTS",
			"alertname", "HTTPRequestRateHigh",
			"alertstate", "pending",
			"instance", "0",
			"job", "app-server",
		),
		F: 1,
	}

	ng := testEngine(t)
	baseTime := time.Unix(0, 0)
	result.T = timestamp.FromTime(baseTime)
	res, err := rule.Eval(context.TODO(), 0, baseTime, EngineQueryFunc(ng, storage), nil, 0)
	require.NoError(t, err)

	require.Len(t, res, 2)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			testutil.RequireEqual(t, result, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	evalTime := baseTime.Add(time.Minute)
	res, err = rule.Eval(context.TODO(), 0, evalTime, EngineQueryFunc(ng, storage), nil, 0)
	require.NoError(t, err)
	require.Empty(t, res)
}

// TestAlertingEvalWithOrigin checks that the alerting rule details are passed through the context.
func TestAlertingEvalWithOrigin(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	const (
		name  = "my-recording-rule"
		query = `count(metric{foo="bar"}) > 0`
	)
	var (
		detail RuleDetail
		lbs    = labels.FromStrings("test", "test")
	)

	expr, err := testParser.ParseExpr(query)
	require.NoError(t, err)

	rule := NewAlertingRule(
		name,
		expr,
		time.Second,
		time.Minute,
		lbs,
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)

	_, err = rule.Eval(ctx, 0, now, func(ctx context.Context, _ string, _ time.Time) (promql.Vector, error) {
		detail = FromOriginContext(ctx)
		return nil, nil
	}, nil, 0)

	require.NoError(t, err)
	require.Equal(t, detail, NewRuleDetail(rule))
}

func TestAlertingRule_SetDependentRules(t *testing.T) {
	dependentRule := NewRecordingRule("test1", nil, labels.EmptyLabels())

	rule := NewAlertingRule(
		"test",
		&parser.NumberLiteral{Val: 1},
		time.Minute,
		0,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	require.False(t, rule.NoDependentRules())

	rule.SetDependentRules([]Rule{dependentRule})
	require.False(t, rule.NoDependentRules())
	require.Equal(t, []Rule{dependentRule}, rule.DependentRules())

	rule.SetDependentRules([]Rule{})
	require.True(t, rule.NoDependentRules())
	require.Empty(t, rule.DependentRules())
}

func TestAlertingRule_SetDependencyRules(t *testing.T) {
	dependencyRule := NewRecordingRule("test1", nil, labels.EmptyLabels())

	rule := NewAlertingRule(
		"test",
		&parser.NumberLiteral{Val: 1},
		time.Minute,
		0,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, promslog.NewNopLogger(),
	)
	require.False(t, rule.NoDependencyRules())

	rule.SetDependencyRules([]Rule{dependencyRule})
	require.False(t, rule.NoDependencyRules())
	require.Equal(t, []Rule{dependencyRule}, rule.DependencyRules())

	rule.SetDependencyRules([]Rule{})
	require.True(t, rule.NoDependencyRules())
	require.Empty(t, rule.DependencyRules())
}

func TestAlertingRule_ActiveAlertsCount(t *testing.T) {
	rule := NewAlertingRule(
		"TestRule",
		nil,
		time.Minute,
		0,
		labels.FromStrings("severity", "critical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	require.Equal(t, 0, rule.ActiveAlertsCount())

	// Set an active alert.
	lbls := labels.FromStrings("a1", "1")
	h := lbls.Hash()
	al := &Alert{State: StateFiring, Labels: lbls, ActiveAt: time.Now()}
	rule.active[h] = al

	require.Equal(t, 1, rule.ActiveAlertsCount())
}

// TestFiringAlertResetToPendingOnHoldDurationIncrease verifies that when the
// holdDuration ("for" duration) is increased on a rule that already has a
// firing alert, the alert is demoted back to StatePending because the elapsed
// time since ActiveAt no longer meets the new, larger holdDuration.
func TestFiringAlertResetToPendingOnHoldDurationIncrease(t *testing.T) {
	shortHold := 15 * time.Second
	longHold := 1 * time.Hour

	expr, err := testParser.ParseExpr("foo")
	require.NoError(t, err)

	rule := NewAlertingRule(
		"TestResetToPending",
		expr,
		shortHold,
		0,
		labels.EmptyLabels(),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	baseTime := time.Unix(0, 0)
	q := func(_ context.Context, _ string, ts time.Time) (promql.Vector, error) {
		return promql.Vector{
			promql.Sample{
				Metric: labels.EmptyLabels(),
				T:      timestamp.FromTime(ts),
				F:      1,
			},
		}, nil
	}

	// Eval at t=0: creates the alert in StatePending with ActiveAt = baseTime.
	_, err = rule.Eval(context.TODO(), 0, baseTime, q, nil, 0)
	require.NoError(t, err)

	require.Len(t, rule.active, 1)
	var alert *Alert
	for _, a := range rule.active {
		alert = a
	}
	require.Equal(t, StatePending, alert.State)

	// Eval at t=15s: the short holdDuration is met, alert transitions to firing.
	evalTime := baseTime.Add(shortHold)
	_, err = rule.Eval(context.TODO(), 0, evalTime, q, nil, 0)
	require.NoError(t, err)
	require.Equal(t, StateFiring, alert.State)
	require.False(t, alert.FiredAt.IsZero(), "FiredAt should be set after firing")

	// Now increase the holdDuration to 1 hour (simulating a rule config reload).
	rule.holdDuration = longHold

	// Eval at t=30s: only 30s have elapsed since ActiveAt, which is far less
	// than the new 1h holdDuration, so the alert must go back to pending.
	evalTime = baseTime.Add(30 * time.Second)
	res, err := rule.Eval(context.TODO(), 0, evalTime, q, nil, 0)
	require.NoError(t, err)

	require.Equal(t, StatePending, alert.State)
	require.True(t, alert.FiredAt.IsZero(), "FiredAt should be reset")
	require.True(t, alert.LastSentAt.IsZero(), "LastSentAt should be reset")
	require.True(t, alert.KeepFiringSince.IsZero(), "KeepFiringSince should be reset")

	for _, smpl := range res {
		if smpl.Metric.Get("__name__") == "ALERTS" {
			require.Equal(t, "pending", smpl.Metric.Get("alertstate"))
		}
	}

	// Eval at t=1h: now the new holdDuration is met, alert fires again.
	evalTime = baseTime.Add(longHold)
	res, err = rule.Eval(context.TODO(), 0, evalTime, q, nil, 0)
	require.NoError(t, err)
	require.Equal(t, StateFiring, alert.State)

	for _, smpl := range res {
		if smpl.Metric.Get("__name__") == "ALERTS" {
			require.Equal(t, "firing", smpl.Metric.Get("alertstate"))
		}
	}
}

func TestAlertingRuleStripsMetadataLabels(t *testing.T) {
	st := teststorage.New(t)
	app := st.AppenderV2(t.Context())
	evalTime := time.Unix(600, 0).UTC()
	_, err := app.Append(
		0,
		labels.FromStrings(
			model.MetricNameLabel, "http_errors_total",
			model.MetricTypeLabel, string(model.MetricTypeCounter),
			model.MetricUnitLabel, "errors",
			"job", "api",
		),
		0,
		timestamp.FromTime(evalTime),
		42,
		nil,
		nil,
		storage.AOptions{},
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	ng := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		MaxSamples:              10000,
		Timeout:                 100 * time.Second,
		EnableTypeAndUnitLabels: true,
	})

	expr, err := testParser.ParseExpr(`http_errors_total > 10`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HighErrors",
		expr,
		5*time.Minute,
		0,
		labels.FromStrings("severity", "page"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true,
		promslog.NewNopLogger(),
	)

	res, err := rule.Eval(t.Context(), 0, evalTime, EngineQueryFunc(ng, st), nil, 0)
	require.NoError(t, err)
	require.Len(t, res, 2)

	active := rule.ActiveAlerts()
	require.Len(t, active, 1)
	testutil.RequireEqual(t, labels.FromStrings(labels.AlertName, "HighErrors", "job", "api", "severity", "page"), active[0].Labels)

	expected := promql.Vector{
		{
			Metric: labels.FromStrings(
				model.MetricNameLabel, alertMetricName,
				labels.AlertName, "HighErrors",
				alertStateLabel, "pending",
				"job", "api",
				"severity", "page",
			),
			T: timestamp.FromTime(evalTime),
			F: 1,
		},
		{
			Metric: labels.FromStrings(
				model.MetricNameLabel, alertForStateMetricName,
				labels.AlertName, "HighErrors",
				"job", "api",
				"severity", "page",
			),
			T: timestamp.FromTime(evalTime),
			F: float64(evalTime.Unix()),
		},
	}
	testutil.RequireEqual(t, expected, res)

	// Verify RestoreForState matches ALERTS_FOR_STATE even when stored with __type__ and __unit__.
	app = st.AppenderV2(t.Context())
	_, err = app.Append(
		0,
		labels.FromStrings(
			model.MetricNameLabel, alertForStateMetricName,
			model.MetricTypeLabel, string(model.MetricTypeGauge),
			model.MetricUnitLabel, "errors",
			labels.AlertName, "HighErrors",
			"job", "api",
			"severity", "page",
		),
		0,
		timestamp.FromTime(evalTime),
		float64(evalTime.Add(-4*time.Minute).Unix()),
		nil,
		nil,
		storage.AOptions{},
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	restoredRule := NewAlertingRule(
		"HighErrors",
		expr,
		5*time.Minute,
		0,
		labels.FromStrings("severity", "page"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		false,
		promslog.NewNopLogger(),
	)
	var notified []*Alert
	g := NewGroup(GroupOptions{
		Name:          "default",
		Interval:      time.Minute,
		Rules:         []Rule{restoredRule},
		ShouldRestore: true,
		Opts: &ManagerOptions{
			Context:                 t.Context(),
			Queryable:               st,
			QueryFunc:               EngineQueryFunc(ng, st),
			AppendableV2:            st,
			NotifyFunc:              func(_ context.Context, _ string, alerts ...*Alert) { notified = append(notified, alerts...) },
			OutageTolerance:         time.Hour,
			ForGracePeriod:          30 * time.Second,
			Logger:                  promslog.NewNopLogger(),
			EnableTypeAndUnitLabels: true,
		},
	})
	g.Eval(t.Context(), evalTime)
	require.NoError(t, restoredRule.LastError())
	g.RestoreForState(evalTime.Add(30 * time.Second))
	require.True(t, restoredRule.Restored())

	restoredActive := restoredRule.ActiveAlerts()
	require.Len(t, restoredActive, 1)
	require.Equal(t, evalTime.Add(-3*time.Minute-30*time.Second), restoredActive[0].ActiveAt)

	// Evaluate past the hold duration so Group.Eval fires NotifyFunc and persists ALERTS.
	fireTime := evalTime.Add(2 * time.Minute)
	app = st.AppenderV2(t.Context())
	_, err = app.Append(
		0,
		labels.FromStrings(
			model.MetricNameLabel, "http_errors_total",
			model.MetricTypeLabel, string(model.MetricTypeCounter),
			model.MetricUnitLabel, "errors",
			"job", "api",
		),
		0,
		timestamp.FromTime(fireTime),
		42,
		nil,
		nil,
		storage.AOptions{},
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	g.Eval(t.Context(), fireTime)
	require.NoError(t, restoredRule.LastError())
	require.Len(t, notified, 1)
	require.Equal(t, StateFiring, notified[0].State)
	testutil.RequireEqual(t, labels.FromStrings(labels.AlertName, "HighErrors", "job", "api", "severity", "page"), notified[0].Labels)

	q, err := st.Querier(timestamp.FromTime(fireTime), timestamp.FromTime(fireTime))
	require.NoError(t, err)
	ss := q.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, alertMetricName))
	require.True(t, ss.Next())
	testutil.RequireEqual(t, labels.FromStrings(
		model.MetricNameLabel, alertMetricName,
		labels.AlertName, "HighErrors",
		alertStateLabel, "firing",
		"job", "api",
		"severity", "page",
	), ss.At().Labels())
	require.False(t, ss.Next())
	require.NoError(t, ss.Err())
	require.NoError(t, q.Close())

	t.Run("restore_colliding_for_state_series", func(t *testing.T) {
		type storedForState struct {
			lbls labels.Labels
			ts   time.Time
			val  float64
		}
		stale := math.Float64frombits(value.StaleNaN)
		activeVal := float64(evalTime.Add(-4 * time.Minute).Unix())
		wantRestored := evalTime.Add(-3*time.Minute - 30*time.Second)

		untypedLbls := labels.FromStrings(
			model.MetricNameLabel, alertForStateMetricName,
			labels.AlertName, "HighErrors",
			"job", "api",
			"severity", "page",
		)
		typedGaugeLbls := labels.FromStrings(
			model.MetricNameLabel, alertForStateMetricName,
			model.MetricTypeLabel, string(model.MetricTypeGauge),
			labels.AlertName, "HighErrors",
			"job", "api",
			"severity", "page",
		)
		leakedCounterLbls := labels.FromStrings(
			model.MetricNameLabel, alertForStateMetricName,
			model.MetricTypeLabel, string(model.MetricTypeCounter),
			model.MetricUnitLabel, "errors",
			labels.AlertName, "HighErrors",
			"job", "api",
			"severity", "page",
		)

		cases := []struct {
			name         string
			series       []storedForState
			wantActiveAt time.Time
		}{
			{
				name: "newer_typed_beats_older_stale_untyped",
				series: []storedForState{
					{lbls: typedGaugeLbls, ts: evalTime, val: activeVal},
					{lbls: untypedLbls, ts: evalTime.Add(-time.Minute), val: stale},
				},
				wantActiveAt: wantRestored,
			},
			{
				name: "newer_stale_untyped_beats_older_active_leaked",
				series: []storedForState{
					{lbls: untypedLbls, ts: evalTime, val: stale},
					{lbls: leakedCounterLbls, ts: evalTime.Add(-time.Minute), val: activeVal},
				},
				wantActiveAt: evalTime,
			},
			{
				name: "same_timestamp_active_first_beats_stale",
				series: []storedForState{
					{lbls: untypedLbls, ts: evalTime, val: activeVal},
					{lbls: leakedCounterLbls, ts: evalTime, val: stale},
				},
				wantActiveAt: wantRestored,
			},
			{
				name: "same_timestamp_stale_first_loses_to_active",
				series: []storedForState{
					{lbls: leakedCounterLbls, ts: evalTime, val: stale},
					{lbls: untypedLbls, ts: evalTime, val: activeVal},
				},
				wantActiveAt: wantRestored,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				caseStore := teststorage.New(t)
				app := caseStore.AppenderV2(t.Context())
				_, err := app.Append(
					0,
					labels.FromStrings(
						model.MetricNameLabel, "http_errors_total",
						model.MetricTypeLabel, string(model.MetricTypeCounter),
						model.MetricUnitLabel, "errors",
						"job", "api",
					),
					0,
					timestamp.FromTime(evalTime),
					42,
					nil,
					nil,
					storage.AOptions{},
				)
				require.NoError(t, err)
				for _, s := range tc.series {
					_, err := app.Append(0, s.lbls, 0, timestamp.FromTime(s.ts), s.val, nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				r := NewAlertingRule(
					"HighErrors",
					expr,
					5*time.Minute,
					0,
					labels.FromStrings("severity", "page"),
					labels.EmptyLabels(),
					labels.EmptyLabels(),
					"",
					false,
					promslog.NewNopLogger(),
				)
				grp := NewGroup(GroupOptions{
					Name:          "default",
					Interval:      time.Minute,
					Rules:         []Rule{r},
					ShouldRestore: true,
					Opts: &ManagerOptions{
						Context:                 t.Context(),
						Queryable:               caseStore,
						QueryFunc:               EngineQueryFunc(ng, caseStore),
						AppendableV2:            caseStore,
						NotifyFunc:              func(context.Context, string, ...*Alert) {},
						OutageTolerance:         time.Hour,
						ForGracePeriod:          30 * time.Second,
						Logger:                  promslog.NewNopLogger(),
						EnableTypeAndUnitLabels: true,
					},
				})
				grp.Eval(t.Context(), evalTime)
				require.NoError(t, r.LastError())
				grp.RestoreForState(evalTime.Add(30 * time.Second))
				require.True(t, r.Restored())

				got := r.ActiveAlerts()
				require.Len(t, got, 1)
				require.Equal(t, tc.wantActiveAt, got[0].ActiveAt)
			})
		}
	})

	t.Run("rule_labels_cannot_reinject_metadata_labels", func(t *testing.T) {
		ruleLbls := labels.FromStrings(
			model.MetricTypeLabel, string(model.MetricTypeGauge),
			model.MetricUnitLabel, "seconds",
			"job", "override",
			"severity", "page",
		)
		r := NewAlertingRule(
			"HighErrors",
			expr,
			5*time.Minute,
			0,
			ruleLbls,
			labels.EmptyLabels(),
			labels.EmptyLabels(),
			"",
			true,
			promslog.NewNopLogger(),
		)
		vec, err := r.Eval(t.Context(), 0, evalTime, EngineQueryFunc(ng, st), nil, 0)
		require.NoError(t, err)
		require.Len(t, vec, 2)

		wantAlertLbls := labels.FromStrings(labels.AlertName, "HighErrors", "job", "override", "severity", "page")
		gotActive := r.ActiveAlerts()
		require.Len(t, gotActive, 1)
		testutil.RequireEqual(t, wantAlertLbls, gotActive[0].Labels)

		testutil.RequireEqual(t, promql.Vector{
			{
				Metric: labels.FromStrings(
					model.MetricNameLabel, alertMetricName,
					labels.AlertName, "HighErrors",
					alertStateLabel, "pending",
					"job", "override",
					"severity", "page",
				),
				T: timestamp.FromTime(evalTime),
				F: 1,
			},
			{
				Metric: labels.FromStrings(
					model.MetricNameLabel, alertForStateMetricName,
					labels.AlertName, "HighErrors",
					"job", "override",
					"severity", "page",
				),
				T: timestamp.FromTime(evalTime),
				F: float64(evalTime.Unix()),
			},
		}, vec)

		// Verify RestoreForState still matches stored ALERTS_FOR_STATE when rule labels contain __type__/__unit__.
		restoreStore := teststorage.New(t)
		app := restoreStore.AppenderV2(t.Context())
		_, err = app.Append(
			0,
			labels.FromStrings(
				model.MetricNameLabel, "http_errors_total",
				model.MetricTypeLabel, string(model.MetricTypeCounter),
				model.MetricUnitLabel, "errors",
				"job", "api",
			),
			0,
			timestamp.FromTime(evalTime),
			42,
			nil,
			nil,
			storage.AOptions{},
		)
		require.NoError(t, err)
		_, err = app.Append(
			0,
			labels.FromStrings(
				model.MetricNameLabel, alertForStateMetricName,
				labels.AlertName, "HighErrors",
				"job", "override",
				"severity", "page",
			),
			0,
			timestamp.FromTime(evalTime),
			float64(evalTime.Add(-4*time.Minute).Unix()),
			nil,
			nil,
			storage.AOptions{},
		)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		restored := NewAlertingRule(
			"HighErrors",
			expr,
			5*time.Minute,
			0,
			ruleLbls,
			labels.EmptyLabels(),
			labels.EmptyLabels(),
			"",
			false,
			promslog.NewNopLogger(),
		)
		grp := NewGroup(GroupOptions{
			Name:          "default",
			Interval:      time.Minute,
			Rules:         []Rule{restored},
			ShouldRestore: true,
			Opts: &ManagerOptions{
				Context:                 t.Context(),
				Queryable:               restoreStore,
				QueryFunc:               EngineQueryFunc(ng, restoreStore),
				AppendableV2:            restoreStore,
				NotifyFunc:              func(context.Context, string, ...*Alert) {},
				OutageTolerance:         time.Hour,
				ForGracePeriod:          30 * time.Second,
				Logger:                  promslog.NewNopLogger(),
				EnableTypeAndUnitLabels: true,
			},
		})
		grp.Eval(t.Context(), evalTime)
		require.NoError(t, restored.LastError())
		grp.RestoreForState(evalTime.Add(30 * time.Second))
		require.True(t, restored.Restored())

		gotRestored := restored.ActiveAlerts()
		require.Len(t, gotRestored, 1)
		require.Equal(t, evalTime.Add(-3*time.Minute-30*time.Second), gotRestored[0].ActiveAt)
	})
}
