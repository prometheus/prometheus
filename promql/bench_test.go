// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, softwar
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/teststorage"
)

func BenchmarkRangeQuery(b *testing.B) {
	storage := teststorage.New(b)
	defer storage.Close()
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    50000000,
		Timeout:       100 * time.Second,
	}
	engine := NewEngine(opts)

	metrics := []labels.Labels{}
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
	refs := make([]uint64, len(metrics))

	// A day of data plus 10k steps.
	numIntervals := 8640 + 10000

	for s := 0; s < numIntervals; s++ {
		a, err := storage.Appender()
		if err != nil {
			b.Fatal(err)
		}
		ts := int64(s * 10000) // 10s interval.
		for i, metric := range metrics {
			err := a.AddFast(metric, refs[i], ts, float64(s))
			if err != nil {
				refs[i], _ = a.Add(metric, ts, float64(s))
			}
		}
		if err := a.Commit(); err != nil {
			b.Fatal(err)
		}
	}

	type benchCase struct {
		expr  string
		steps int
	}
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
	}

	// X in an expr will be replaced by different metric sizes.
	tmp := []benchCase{}
	for _, c := range cases {
		if !strings.Contains(c.expr, "X") {
			tmp = append(tmp, c)
		} else {
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "one", -1), steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "ten", -1), steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "hundred", -1), steps: c.steps})
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
			tmp = append(tmp, benchCase{expr: c.expr, steps: 10})
			tmp = append(tmp, benchCase{expr: c.expr, steps: 100})
			tmp = append(tmp, benchCase{expr: c.expr, steps: 1000})
		}
	}
	cases = tmp
	for _, c := range cases {
		name := fmt.Sprintf("expr=%s,steps=%d", c.expr, c.steps)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				qry, err := engine.NewRangeQuery(
					storage, c.expr,
					time.Unix(int64((numIntervals-c.steps)*10), 0),
					time.Unix(int64(numIntervals*10), 0), time.Second*10)
				if err != nil {
					b.Fatal(err)
				}
				res := qry.Exec(context.Background())
				if res.Err != nil {
					b.Fatal(res.Err)
				}
				qry.Close()
			}
		})
	}
}
