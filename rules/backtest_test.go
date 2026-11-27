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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
)

// alertStateTimeline renders the ALERTS series of a backtest as one state per
// evaluation step, using "-" where no alert was active.
func alertStateTimeline(t *testing.T, m promql.Matrix, opts BacktestOptions) []string {
	t.Helper()

	out := make([]string, opts.steps())
	for i := range out {
		out[i] = "-"
	}
	for _, s := range m {
		if s.Metric.Get(labels.MetricName) != alertMetricName {
			continue
		}
		state := s.Metric.Get(alertStateLabel)
		for _, p := range s.Floats {
			i := int(timestamp.Time(p.T).Sub(opts.Start) / opts.Interval)
			require.Equalf(t, "-", out[i], "more than one alert state at step %d", i)
			out[i] = state
		}
	}
	return out
}

func TestBacktest(t *testing.T) {
	st := promqltest.LoadedStorage(t, `
		load 1m
			http_requests{job="app-server", instance="0"} 120 90 90 90 90 120 90 90 120
	`)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	engine := testEngine(t)
	baseTime := time.Unix(0, 0).UTC()

	for _, tc := range []struct {
		name          string
		holdDuration  time.Duration
		keepFiringFor time.Duration
		want          []string
	}{
		{
			name: "no for",
			want: []string{"-", "firing", "firing", "firing", "firing", "-", "firing", "firing", "-"},
		},
		{
			name:         "for shorter than the outage",
			holdDuration: 2 * time.Minute,
			want:         []string{"-", "pending", "pending", "firing", "firing", "-", "pending", "pending", "-"},
		},
		{
			name:         "for longer than the outage never fires",
			holdDuration: 5 * time.Minute,
			want:         []string{"-", "pending", "pending", "pending", "pending", "-", "pending", "pending", "-"},
		},
		{
			name:          "keep_firing_for bridges the gap",
			keepFiringFor: 2 * time.Minute,
			want:          []string{"-", "firing", "firing", "firing", "firing", "firing", "firing", "firing", "firing"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := NewAlertingRule(
				"HTTPRequestRateLow", expr, tc.holdDuration, tc.keepFiringFor,
				labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", false, nil,
			)
			opts := BacktestOptions{
				Start:    baseTime,
				End:      baseTime.Add(8 * time.Minute),
				Interval: time.Minute,
			}

			m, _, err := Backtest(context.Background(), rule, engine, st, opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, alertStateTimeline(t, m, opts))

			// Backtesting must not disturb the state of the rule it was given.
			require.Empty(t, rule.ActiveAlerts())
			require.False(t, rule.Restored())
		})
	}
}

// TestBacktestMatchesEval guards the range query shortcut: replaying a matrix
// must produce exactly what stepping the rule with instant queries produces.
func TestBacktestMatchesEval(t *testing.T) {
	st := promqltest.LoadedStorage(t, `
		load 30s
			http_requests{job="app-server", instance="0"} 120 90 90 _ 90 120 90 90 120 90 90 90
			http_requests{job="app-server", instance="1"} 90 90 120 120 90 90 90 120 90 90 120 90
	`)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	expr, err := testParser.ParseExpr(`http_requests < 100`)
	require.NoError(t, err)

	engine := testEngine(t)
	baseTime := time.Unix(0, 0).UTC()
	opts := BacktestOptions{
		Start:    baseTime,
		End:      baseTime.Add(6 * time.Minute),
		Interval: 30 * time.Second,
	}

	newRule := func(restored bool) *AlertingRule {
		return NewAlertingRule(
			"HTTPRequestRateLow", expr, time.Minute, 30*time.Second,
			labels.FromStrings("severity", "page"),
			labels.FromStrings("summary", "{{$labels.instance}} is at {{$value}}"),
			labels.EmptyLabels(), "", restored, nil,
		)
	}

	got, _, err := Backtest(context.Background(), newRule(false), engine, st, opts)
	require.NoError(t, err)

	query := EngineQueryFunc(engine, st)
	reference := seriesBuilder{}
	stepped := newRule(true)
	for i := range opts.steps() {
		ts := opts.Start.Add(time.Duration(i) * opts.Interval)
		vec, err := stepped.Eval(context.Background(), 0, ts, query, nil, 0)
		require.NoError(t, err)
		reference.add(vec)
	}

	require.Equal(t, reference.matrix(), got)
}

func TestBacktestOptionsValidation(t *testing.T) {
	baseTime := time.Unix(0, 0).UTC()

	for _, tc := range []struct {
		name string
		opts BacktestOptions
		err  string
	}{
		{
			name: "zero interval",
			opts: BacktestOptions{Start: baseTime, End: baseTime.Add(time.Hour)},
			err:  "interval must be positive",
		},
		{
			name: "end before start",
			opts: BacktestOptions{Start: baseTime.Add(time.Hour), End: baseTime, Interval: time.Minute},
			err:  "end timestamp must not be before start timestamp",
		},
		{
			name: "too many steps",
			opts: BacktestOptions{Start: baseTime, End: baseTime.Add(MaxBacktestSteps * time.Minute), Interval: time.Minute},
			err:  "exceeded maximum of 11000 evaluations",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BacktestMatrix(context.Background(), nil, nil, nil, tc.opts)
			require.ErrorContains(t, err, tc.err)
		})
	}
}
