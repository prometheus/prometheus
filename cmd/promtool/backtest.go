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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
)

// backtestConfig holds the flags of the "query backtest" command.
type backtestConfig struct {
	name          string
	expr          string
	forDuration   time.Duration
	keepFiringFor time.Duration
	ruleLabels    map[string]string
	start, end    string
	interval      time.Duration
	headers       map[string]string
}

// alertPeriod is a contiguous run of evaluations during which one alert
// instance stayed in the same state.
type alertPeriod struct {
	Labels map[string]string `json:"labels"`
	State  string            `json:"state"`
	Start  time.Time         `json:"start"`
	End    time.Time         `json:"end"`
}

// BacktestAlert replays an alerting rule over historical data on a Prometheus
// server and reports when it would have been pending and firing.
func BacktestAlert(serverURL *url.URL, roundTripper http.RoundTripper, cfg backtestConfig, exprParser parser.Parser, p printer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	periods, err := backtestAlert(ctx, serverURL, roundTripper, cfg, exprParser)
	if err != nil {
		return handleAPIError(err)
	}

	printAlertPeriods(periods, p)
	return successExitCode
}

func backtestAlert(ctx context.Context, serverURL *url.URL, roundTripper http.RoundTripper, cfg backtestConfig, exprParser parser.Parser) ([]alertPeriod, error) {
	opts, err := cfg.backtestOptions()
	if err != nil {
		return nil, err
	}

	rule, err := cfg.rule(exprParser)
	if err != nil {
		return nil, err
	}

	api, err := newAPI(serverURL, roundTripper, cfg.headers)
	if err != nil {
		return nil, fmt.Errorf("creating API client: %w", err)
	}

	// A single range query at the evaluation interval yields the same samples as
	// one instant query per step, at a fraction of the cost.
	val, _, err := api.QueryRange(ctx, cfg.expr, v1.Range{Start: opts.Start, End: opts.End, Step: opts.Interval})
	if err != nil {
		return nil, err
	}
	m, ok := val.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("expected a range vector result, got %s", val.Type())
	}
	samples, err := convertMatrix(m)
	if err != nil {
		return nil, err
	}

	alerts, err := rules.BacktestMatrix(ctx, rule, samples, apiQueryFunc(api), opts)
	if err != nil {
		return nil, err
	}
	return alertPeriods(alerts, opts.Interval), nil
}

func (cfg backtestConfig) rule(exprParser parser.Parser) (*rules.AlertingRule, error) {
	expr, err := exprParser.ParseExpr(cfg.expr)
	if err != nil {
		return nil, err
	}
	return rules.NewAlertingRule(
		cfg.name, expr, cfg.forDuration, cfg.keepFiringFor,
		labels.FromMap(cfg.ruleLabels), labels.EmptyLabels(), labels.EmptyLabels(),
		"", false, nil,
	), nil
}

func (cfg backtestConfig) backtestOptions() (rules.BacktestOptions, error) {
	opts := rules.BacktestOptions{Interval: cfg.interval}

	var err error
	opts.End = time.Now()
	if cfg.end != "" {
		if opts.End, err = parseTime(cfg.end); err != nil {
			return opts, fmt.Errorf("end time: %w", err)
		}
	}

	opts.Start = opts.End.Add(-time.Hour)
	if cfg.start != "" {
		if opts.Start, err = parseTime(cfg.start); err != nil {
			return opts, fmt.Errorf("start time: %w", err)
		}
	}
	return opts, nil
}

// apiQueryFunc adapts a Prometheus API client to the query function used for
// expanding alert templates.
func apiQueryFunc(api v1.API) rules.QueryFunc {
	return func(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
		val, _, err := api.Query(ctx, q, t)
		if err != nil {
			return nil, err
		}
		vec, ok := val.(model.Vector)
		if !ok {
			return nil, fmt.Errorf("expected an instant vector result, got %s", val.Type())
		}
		out := make(promql.Vector, 0, len(vec))
		for _, s := range vec {
			out = append(out, promql.Sample{
				Metric: convertMetric(s.Metric),
				T:      int64(s.Timestamp),
				F:      float64(s.Value),
			})
		}
		return out, nil
	}
}

func convertMatrix(m model.Matrix) (promql.Matrix, error) {
	out := make(promql.Matrix, 0, len(m))
	for _, ss := range m {
		if len(ss.Histograms) > 0 {
			return nil, errors.New("backtesting expressions that return native histograms is not supported")
		}
		s := promql.Series{
			Metric: convertMetric(ss.Metric),
			Floats: make([]promql.FPoint, 0, len(ss.Values)),
		}
		for _, p := range ss.Values {
			s.Floats = append(s.Floats, promql.FPoint{T: int64(p.Timestamp), F: float64(p.Value)})
		}
		out = append(out, s)
	}
	sort.Sort(out)
	return out, nil
}

func convertMetric(m model.Metric) labels.Labels {
	b := labels.NewScratchBuilder(len(m))
	for n, v := range m {
		b.Add(string(n), string(v))
	}
	b.Sort()
	return b.Labels()
}

// alertPeriods collapses the ALERTS series of a backtest into one entry per
// uninterrupted run of the same state. A gap wider than the evaluation interval
// ends the current period.
func alertPeriods(m promql.Matrix, interval time.Duration) []alertPeriod {
	var out []alertPeriod
	for _, s := range m {
		if s.Metric.Get(labels.MetricName) != "ALERTS" {
			continue
		}

		lset := s.Metric.Map()
		delete(lset, labels.MetricName)
		state := lset["alertstate"]
		delete(lset, "alertstate")

		var cur *alertPeriod
		for _, p := range s.Floats {
			ts := timestamp.Time(p.T)
			if cur != nil && ts.Sub(cur.End) <= interval {
				cur.End = ts
				continue
			}
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &alertPeriod{Labels: lset, State: state, Start: ts, End: ts}
		}
		if cur != nil {
			out = append(out, *cur)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].State < out[j].State
	})
	return out
}

func printAlertPeriods(periods []alertPeriod, p printer) {
	if _, ok := p.(*jsonPrinter); ok {
		//nolint:errcheck
		json.NewEncoder(os.Stdout).Encode(periods)
		return
	}

	if len(periods) == 0 {
		fmt.Println("The alert would never have been active over this range.")
		return
	}
	for _, period := range periods {
		fmt.Printf("%-7s %s -> %s (%s) %s\n",
			period.State,
			period.Start.UTC().Format(time.RFC3339),
			period.End.UTC().Format(time.RFC3339),
			period.End.Sub(period.Start),
			labels.FromMap(period.Labels).String(),
		)
	}
}
