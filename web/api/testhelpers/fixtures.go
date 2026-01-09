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

// This file provides test fixture data for API tests.
package testhelpers

import (
	"time"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

// FixtureSeries creates a simple series with the "up" metric.
func FixtureSeries() []storage.Series {
	// Use timestamps relative to "now" so queries work.
	now := time.Now().UnixMilli()
	return []storage.Series{
		&FakeSeries{
			labels: labels.FromStrings("__name__", "up", "job", "prometheus", "instance", "localhost:9090"),
			samples: []promql.FPoint{
				{T: now - 120000, F: 1},
				{T: now - 60000, F: 1},
				{T: now, F: 1},
			},
		},
	}
}

// FixtureMultipleSeries creates multiple series for testing.
func FixtureMultipleSeries() []storage.Series {
	// Use timestamps relative to "now" so queries work.
	now := time.Now().UnixMilli()
	return []storage.Series{
		&FakeSeries{
			labels: labels.FromStrings("__name__", "up", "job", "prometheus", "instance", "localhost:9090"),
			samples: []promql.FPoint{
				{T: now - 60000, F: 1},
				{T: now, F: 1},
			},
		},
		&FakeSeries{
			labels: labels.FromStrings("__name__", "up", "job", "node", "instance", "localhost:9100"),
			samples: []promql.FPoint{
				{T: now - 60000, F: 1},
				{T: now, F: 0},
			},
		},
		&FakeSeries{
			labels: labels.FromStrings("__name__", "http_requests_total", "job", "api", "instance", "localhost:8080"),
			samples: []promql.FPoint{
				{T: now - 60000, F: 100},
				{T: now, F: 150},
			},
		},
	}
}

// FixtureRuleGroups creates a simple set of rule groups for testing.
func FixtureRuleGroups() []*rules.Group {
	// Create a simple recording rule.
	expr, _ := parser.ParseExpr("up == 1")
	recordingRule := rules.NewRecordingRule(
		"job:up:sum",
		expr,
		labels.EmptyLabels(),
	)

	// Create a simple alerting rule.
	alertExpr, _ := parser.ParseExpr("up == 0")
	alertingRule := rules.NewAlertingRule(
		"InstanceDown",
		alertExpr,
		time.Minute,
		0,
		labels.FromStrings("severity", "critical"),
		labels.EmptyLabels(),
		labels.EmptyLabels(),
		"Instance {{ $labels.instance }} is down",
		true,
		nil,
	)

	// Create a rule group.
	group := rules.NewGroup(rules.GroupOptions{
		Name:     "example",
		File:     "example.rules",
		Interval: time.Minute,
		Rules: []rules.Rule{
			recordingRule,
			alertingRule,
		},
	})

	return []*rules.Group{group}
}

// FixtureEmptyRuleGroups returns an empty set of rule groups.
func FixtureEmptyRuleGroups() []*rules.Group {
	return []*rules.Group{}
}

// FixtureSingleSeries creates a single series for simple tests.
func FixtureSingleSeries(metricName string, value float64) []storage.Series {
	return []storage.Series{
		&FakeSeries{
			labels: labels.FromStrings("__name__", metricName),
			samples: []promql.FPoint{
				{T: 0, F: value},
			},
		},
	}
}

// FixtureHistogramSeries creates a series with native histogram data.
func FixtureHistogramSeries() []storage.Series {
	// Use timestamps relative to "now" so queries work.
	now := time.Now().UnixMilli()
	return []storage.Series{
		&FakeHistogramSeries{
			labels: labels.FromStrings("__name__", "test_histogram", "job", "prometheus", "instance", "localhost:9090"),
			histograms: []promql.HPoint{
				{
					T: now - 60000,
					H: &histogram.FloatHistogram{
						Schema:        2,
						ZeroThreshold: 0.001,
						ZeroCount:     5,
						Count:         50,
						Sum:           100,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						NegativeSpans: []histogram.Span{
							{Offset: 0, Length: 1},
						},
						PositiveBuckets: []float64{5, 10, 8, 7},
						NegativeBuckets: []float64{3},
					},
				},
				{
					T: now,
					H: &histogram.FloatHistogram{
						Schema:        2,
						ZeroThreshold: 0.001,
						ZeroCount:     8,
						Count:         60,
						Sum:           120,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						NegativeSpans: []histogram.Span{
							{Offset: 0, Length: 1},
						},
						PositiveBuckets: []float64{6, 12, 10, 9},
						NegativeBuckets: []float64{4},
					},
				},
			},
		},
	}
}
