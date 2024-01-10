// Copyright 2016 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

var testEngine = promql.NewEngine(promql.EngineOpts{
	Logger:                   nil,
	Reg:                      nil,
	MaxSamples:               10000,
	Timeout:                  100 * time.Second,
	NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
	EnableAtModifier:         true,
	EnableNegativeOffset:     true,
	EnablePerStepStats:       true,
})

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
		got := rule.State()
		require.Equal(t, test.want, got, "test case %d unexpected AlertState, want:%d got:%d", i, test.want, got)
	}
}

func TestAlertingRuleLabelsUpdate(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70 stale
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests < 100`)
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

	baseTime := time.Unix(0, 0)
	for i, result := range results {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(time.Duration(i) * time.Minute)
		result[0].T = timestamp.FromTime(evalTime)
		res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
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

		require.Equal(t, result, filteredRes)
	}
	evalTime := baseTime.Add(time.Duration(len(results)) * time.Minute)
	res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestAlertingRuleExternalLabelsInTemplate(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests < 100`)
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
		true, log.NewNopLogger(),
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
		true, log.NewNopLogger(),
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

	evalTime := time.Unix(0, 0)
	result[0].T = timestamp.FromTime(evalTime)
	result[1].T = timestamp.FromTime(evalTime)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := ruleWithoutExternalLabels.Eval(
		context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0,
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
		context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0,
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

	require.Equal(t, result, filteredRes)
}

func TestAlertingRuleExternalURLInTemplate(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests < 100`)
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
		true, log.NewNopLogger(),
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
		true, log.NewNopLogger(),
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

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := ruleWithoutExternalURL.Eval(
		context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0,
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
		context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0,
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

	require.Equal(t, result, filteredRes)
}

func TestAlertingRuleEmptyLabelFromTemplate(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests < 100`)
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
		true, log.NewNopLogger(),
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

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := rule.Eval(
		context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0,
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
	require.Equal(t, result, filteredRes)
}

func TestAlertingRuleQueryInTemplate(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	70 85 70 70
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`sum(http_requests) < 100`)
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
		true, log.NewNopLogger(),
	)
	evalTime := time.Unix(0, 0)

	startQueryCh := make(chan struct{})
	getDoneCh := make(chan struct{})
	slowQueryFunc := func(ctx context.Context, q string, ts time.Time) (promql.Vector, error) {
		if q == "sort(sum(http_requests) by (instance))" {
			// This is a minimum reproduction of issue 10703, expand template with query.
			close(startQueryCh)
			select {
			case <-getDoneCh:
			case <-time.After(time.Millisecond * 10):
				// Assert no blocking when template expanding.
				require.Fail(t, "unexpected blocking when template expanding.")
			}
		}
		return EngineQueryFunc(testEngine, storage)(ctx, q, ts)
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
		context.TODO(), evalTime, slowQueryFunc, nil, 0,
	)
	require.NoError(t, err)
}

func BenchmarkAlertingRuleAtomicField(b *testing.B) {
	b.ReportAllocs()
	rule := NewAlertingRule("bench", nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil)
	done := make(chan struct{})
	go func() {
		for i := 0; i < b.N; i++ {
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
	rule := NewAlertingRule(
		"foo",
		expr,
		time.Minute,
		0,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, log.NewNopLogger(),
	)
	_, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil, 0)
	require.Error(t, err)
	require.EqualError(t, err, "vector contains metrics with the same labelset after applying alert labels")
}

func TestAlertingRuleLimit(t *testing.T) {
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
			err:   "exceeded limit of 1 with 2 alerts",
		},
	}

	expr, _ := parser.ParseExpr(`metric > 0`)
	rule := NewAlertingRule(
		"foo",
		expr,
		time.Minute,
		0,
		labels.FromStrings("test", "test"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"",
		true, log.NewNopLogger(),
	)

	evalTime := time.Unix(0, 0)

	for _, test := range tests {
		switch _, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, test.limit); {
		case err != nil:
			require.EqualError(t, err, test.err)
		case test.err != "":
			t.Errorf("Expected errror %s, got none", test.err)
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
			selectMockFunction: func(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
				return storage.EmptySeriesSet()
			},
			expectedSeries: nil,
			expectedError:  nil,
		},
		// Test for error series.
		{
			selectMockFunction: func(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
				return storage.ErrSeriesSet(testError)
			},
			expectedSeries: nil,
			expectedError:  testError,
		},
		// Test for mock series.
		{
			selectMockFunction: func(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
				return storage.TestSeriesSet(storage.MockSeries(
					[]int64{1, 2, 3},
					[]float64{1, 2, 3},
					[]string{"__name__", "ALERTS_FOR_STATE", "alertname", "TestRule", "severity", "critical"},
				))
			},
			expectedSeries: storage.MockSeries(
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

		alert := &Alert{
			State:       0,
			Labels:      labels.EmptyLabels(),
			Annotations: labels.EmptyLabels(),
			Value:       0,
			ActiveAt:    time.Time{},
			FiredAt:     time.Time{},
			ResolvedAt:  time.Time{},
			LastSentAt:  time.Time{},
			ValidUntil:  time.Time{},
		}

		series, err := rule.QueryforStateSeries(context.Background(), alert, querier)

		require.Equal(t, tst.expectedSeries, series)
		require.Equal(t, tst.expectedError, err)
	}

	for _, tst := range tests {
		testFunc(tst)
	}
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

	expr, err := parser.ParseExpr("foo")
	require.NoError(t, err)
	rule.vector = expr

	// The relabel rule reproduced the bug here.
	opts := notifier.Options{
		QueueCapacity: 1,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{"a1"},
				Regex:        relabel.MustNewRegexp("(.+)"),
				TargetLabel:  "a1",
				Replacement:  "bug",
				Action:       "replace",
			},
		},
	}
	nm := notifier.NewManager(&opts, log.NewNopLogger())

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
	require.Equal(t, labels.FromStrings("a1", "1"), rule.active[h].Labels)
}

func TestKeepFiringFor(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70 10x5
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests > 50`)
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

	baseTime := time.Unix(0, 0)
	for i, result := range results {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(time.Duration(i) * time.Minute)
		result[0].T = timestamp.FromTime(evalTime)
		res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
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

		require.Equal(t, result, filteredRes)
	}
	evalTime := baseTime.Add(time.Duration(len(results)) * time.Minute)
	res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestPendingAndKeepFiringFor(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 10x10
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests > 50`)
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

	baseTime := time.Unix(0, 0)
	result.T = timestamp.FromTime(baseTime)
	res, err := rule.Eval(context.TODO(), baseTime, EngineQueryFunc(testEngine, storage), nil, 0)
	require.NoError(t, err)

	require.Len(t, res, 2)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			require.Equal(t, result, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			require.Equal(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	evalTime := baseTime.Add(time.Minute)
	res, err = rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
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

	expr, err := parser.ParseExpr(query)
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
		true, log.NewNopLogger(),
	)

	_, err = rule.Eval(ctx, now, func(ctx context.Context, qs string, _ time.Time) (promql.Vector, error) {
		detail = FromOriginContext(ctx)
		return nil, nil
	}, nil, 0)

	require.NoError(t, err)
	require.Equal(t, detail, NewRuleDetail(rule))
}
