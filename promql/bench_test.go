// Copyright 2015 The Prometheus Authors
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

package promql_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/teststorage"
)

func setupRangeQueryTestData(stor *teststorage.TestStorage, _ *promql.Engine, interval, numIntervals int) error {
	ctx := context.Background()

	metrics := []labels.Labels{}
	// Generating test series: a_X, b_X, and h_X, where X can take values of one, ten, or hundred,
	// representing the number of series each metric name contains.
	// Metric a_X and b_X are simple metrics where h_X is a histogram.
	// These metrics will have data for all test time range
	metrics = append(metrics, labels.FromStrings("__name__", "a_one"))
	metrics = append(metrics, labels.FromStrings("__name__", "b_one"))
	for j := 0; j < 10; j++ {
		metrics = append(metrics, labels.FromStrings("__name__", "h_one", "le", strconv.Itoa(j)))
	}
	metrics = append(metrics, labels.FromStrings("__name__", "h_one", "le", "+Inf"))

	for i := 0; i < 10; i++ {
		metrics = append(metrics, labels.FromStrings("__name__", "a_ten", "l", strconv.Itoa(i)))
		metrics = append(metrics, labels.FromStrings("__name__", "b_ten", "l", strconv.Itoa(i)))
		for j := 0; j < 10; j++ {
			metrics = append(metrics, labels.FromStrings("__name__", "h_ten", "l", strconv.Itoa(i), "le", strconv.Itoa(j)))
		}
		metrics = append(metrics, labels.FromStrings("__name__", "h_ten", "l", strconv.Itoa(i), "le", "+Inf"))
	}

	for i := 0; i < 100; i++ {
		metrics = append(metrics, labels.FromStrings("__name__", "a_hundred", "l", strconv.Itoa(i)))
		metrics = append(metrics, labels.FromStrings("__name__", "b_hundred", "l", strconv.Itoa(i)))
		for j := 0; j < 10; j++ {
			metrics = append(metrics, labels.FromStrings("__name__", "h_hundred", "l", strconv.Itoa(i), "le", strconv.Itoa(j)))
		}
		metrics = append(metrics, labels.FromStrings("__name__", "h_hundred", "l", strconv.Itoa(i), "le", "+Inf"))
	}
	refs := make([]storage.SeriesRef, len(metrics))

	// Number points for each different label value of "l" for the sparse series
	pointsPerSparseSeries := numIntervals / 50

	for s := 0; s < numIntervals; s++ {
		a := stor.Appender(context.Background())
		ts := int64(s * interval)
		for i, metric := range metrics {
			ref, _ := a.Append(refs[i], metric, ts, float64(s)+float64(i)/float64(len(metrics)))
			refs[i] = ref
		}
		// Generating a sparse time series: each label value of "l" will contain data only for
		// pointsPerSparseSeries points
		metric := labels.FromStrings("__name__", "sparse", "l", strconv.Itoa(s/pointsPerSparseSeries))
		_, err := a.Append(0, metric, ts, float64(s)/float64(len(metrics)))
		if err != nil {
			return err
		}
		if err := a.Commit(); err != nil {
			return err
		}
	}

	stor.DB.ForceHeadMMap() // Ensure we have at most one head chunk for every series.
	stor.DB.Compact(ctx)
	return nil
}

type benchCase struct {
	expr  string
	steps int
}

func rangeQueryCases() []benchCase {
	cases := []benchCase{
		// Plain retrieval.
		{
			expr: "a_X",
		},
		// Simple rate.
		{
			expr: "rate(a_X[1m])",
		},
		{
			expr:  "rate(a_X[1m])",
			steps: 10000,
		},
		{
			expr:  "rate(sparse[1m])",
			steps: 10000,
		},
		// Holt-Winters and long ranges.
		{
			expr: "holt_winters(a_X[1d], 0.3, 0.3)",
		},
		{
			expr: "changes(a_X[1d])",
		},
		{
			expr: "rate(a_X[1d])",
		},
		{
			expr: "absent_over_time(a_X[1d])",
		},
		// Unary operators.
		{
			expr: "-a_X",
		},
		// Binary operators.
		{
			expr: "a_X - b_X",
		},
		{
			expr:  "a_X - b_X",
			steps: 10000,
		},
		{
			expr: "a_X and b_X{l=~'.*[0-4]$'}",
		},
		{
			expr: "a_X or b_X{l=~'.*[0-4]$'}",
		},
		{
			expr: "a_X unless b_X{l=~'.*[0-4]$'}",
		},
		{
			expr: "a_X and b_X{l='notfound'}",
		},
		// Simple functions.
		{
			expr: "abs(a_X)",
		},
		{
			expr: "label_replace(a_X, 'l2', '$1', 'l', '(.*)')",
		},
		{
			expr: "label_join(a_X, 'l2', '-', 'l', 'l')",
		},
		// Simple aggregations.
		{
			expr: "sum(a_X)",
		},
		{
			expr: "sum without (l)(h_X)",
		},
		{
			expr: "sum without (le)(h_X)",
		},
		{
			expr: "sum by (l)(h_X)",
		},
		{
			expr: "sum by (le)(h_X)",
		},
		{
			expr:  "count_values('value', h_X)",
			steps: 100,
		},
		{
			expr: "topk(1, a_X)",
		},
		{
			expr: "topk(5, a_X)",
		},
		// Combinations.
		{
			expr: "rate(a_X[1m]) + rate(b_X[1m])",
		},
		{
			expr: "sum without (l)(rate(a_X[1m]))",
		},
		{
			expr: "sum without (l)(rate(a_X[1m])) / sum without (l)(rate(b_X[1m]))",
		},
		{
			expr: "histogram_quantile(0.9, rate(h_X[5m]))",
		},
		// Many-to-one join.
		{
			expr: "a_X + on(l) group_right a_one",
		},
		// Label compared to blank string.
		{
			expr:  "count({__name__!=\"\"})",
			steps: 1,
		},
		{
			expr:  "count({__name__!=\"\",l=\"\"})",
			steps: 1,
		},
		// Functions which have special handling inside eval()
		{
			expr: "timestamp(a_X)",
		},
	}

	// X in an expr will be replaced by different metric sizes.
	tmp := []benchCase{}
	for _, c := range cases {
		if !strings.Contains(c.expr, "X") {
			tmp = append(tmp, c)
		} else {
			tmp = append(tmp, benchCase{expr: strings.ReplaceAll(c.expr, "X", "one"), steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.ReplaceAll(c.expr, "X", "ten"), steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.ReplaceAll(c.expr, "X", "hundred"), steps: c.steps})
		}
	}
	cases = tmp

	// No step will be replaced by cases with the standard step.
	tmp = []benchCase{}
	for _, c := range cases {
		if c.steps != 0 {
			tmp = append(tmp, c)
		} else {
			tmp = append(tmp, benchCase{expr: c.expr, steps: 1})
			tmp = append(tmp, benchCase{expr: c.expr, steps: 100})
			tmp = append(tmp, benchCase{expr: c.expr, steps: 1000})
		}
	}
	return tmp
}

func BenchmarkRangeQuery(b *testing.B) {
	stor := teststorage.New(b)
	stor.DB.DisableCompactions() // Don't want auto-compaction disrupting timings.
	defer stor.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 50000000,
		Timeout:    100 * time.Second,
	}
	engine := promql.NewEngine(opts)

	const interval = 10000 // 10s interval.
	// A day of data plus 10k steps.
	numIntervals := 8640 + 10000

	err := setupRangeQueryTestData(stor, engine, interval, numIntervals)
	if err != nil {
		b.Fatal(err)
	}
	cases := rangeQueryCases()

	for _, c := range cases {
		name := fmt.Sprintf("expr=%s,steps=%d", c.expr, c.steps)
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				qry, err := engine.NewRangeQuery(
					ctx, stor, nil, c.expr,
					time.Unix(int64((numIntervals-c.steps)*10), 0),
					time.Unix(int64(numIntervals*10), 0), time.Second*10)
				if err != nil {
					b.Fatal(err)
				}
				res := qry.Exec(ctx)
				if res.Err != nil {
					b.Fatal(res.Err)
				}
				qry.Close()
			}
		})
	}
}

func BenchmarkNativeHistograms(b *testing.B) {
	testStorage := teststorage.New(b)
	defer testStorage.Close()

	app := testStorage.Appender(context.TODO())
	if err := generateNativeHistogramSeries(app, 3000); err != nil {
		b.Fatal(err)
	}
	if err := app.Commit(); err != nil {
		b.Fatal(err)
	}

	start := time.Unix(0, 0)
	end := start.Add(2 * time.Hour)
	step := time.Second * 30

	cases := []struct {
		name  string
		query string
	}{
		{
			name:  "sum",
			query: "sum(native_histogram_series)",
		},
		{
			name:  "sum rate with short rate interval",
			query: "sum(rate(native_histogram_series[2m]))",
		},
		{
			name:  "sum rate with long rate interval",
			query: "sum(rate(native_histogram_series[20m]))",
		},
	}

	opts := promql.EngineOpts{
		Logger:               nil,
		Reg:                  nil,
		MaxSamples:           50000000,
		Timeout:              100 * time.Second,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			ng := promql.NewEngine(opts)
			for i := 0; i < b.N; i++ {
				qry, err := ng.NewRangeQuery(context.Background(), testStorage, nil, tc.query, start, end, step)
				if err != nil {
					b.Fatal(err)
				}
				if result := qry.Exec(context.Background()); result.Err != nil {
					b.Fatal(result.Err)
				}
			}
		})
	}
}

func generateNativeHistogramSeries(app storage.Appender, numSeries int) error {
	commonLabels := []string{labels.MetricName, "native_histogram_series", "foo", "bar"}
	series := make([][]*histogram.Histogram, numSeries)
	for i := range series {
		series[i] = tsdbutil.GenerateTestHistograms(2000)
	}
	higherSchemaHist := &histogram.Histogram{
		Schema: 3,
		PositiveSpans: []histogram.Span{
			{Offset: -5, Length: 2}, // -5 -4
			{Offset: 2, Length: 3},  // -1 0 1
			{Offset: 2, Length: 2},  // 4 5
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 3},
		Count:           13,
	}
	for sid, histograms := range series {
		seriesLabels := labels.FromStrings(append(commonLabels, "h", strconv.Itoa(sid))...)
		for i := range histograms {
			ts := time.Unix(int64(i*15), 0).UnixMilli()
			if i == 0 {
				// Inject a histogram with a higher schema.
				if _, err := app.AppendHistogram(0, seriesLabels, ts, higherSchemaHist, nil); err != nil {
					return err
				}
			}
			if _, err := app.AppendHistogram(0, seriesLabels, ts, histograms[i], nil); err != nil {
				return err
			}
		}
	}

	return nil
}

func BenchmarkParser(b *testing.B) {
	cases := []string{
		"a",
		"metric",
		"1",
		"1 >= bool 1",
		"1 + 2/(3*1)",
		"foo or bar",
		"foo and bar unless baz or qux",
		"bar + on(foo) bla / on(baz, buz) group_right(test) blub",
		"foo / ignoring(test,blub) group_left(blub) bar",
		"foo - ignoring(test,blub) group_right(bar,foo) bar",
		`foo{a="b", foo!="bar", test=~"test", bar!~"baz"}`,
		`min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
		"sum without(and, by, avg, count, alert, annotations)(some_metric) [30m:10s]",
	}
	errCases := []string{
		"(",
		"}",
		"1 or 1",
		"1 or on(bar) foo",
		"foo unless on(bar) group_left(baz) bar",
		"test[5d] OFFSET 10s [10m:5s]",
	}

	for _, c := range cases {
		b.Run(c, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parser.ParseExpr(c)
			}
		})
	}
	for _, c := range errCases {
		name := fmt.Sprintf("%s (should fail)", c)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parser.ParseExpr(c)
			}
		})
	}
}
