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
	"github.com/prometheus/prometheus/util/testutil"
)

func BenchmarkRangeQuery(b *testing.B) {
	storage := testutil.NewStorage(b)
	defer storage.Close()
	engine := NewEngine(nil, nil, 10, 10*time.Second)

	metrics := make([]labels.Labels, 0, 2000)
	metrics = append(metrics, labels.FromStrings("__name__", "a_one"))
	metrics = append(metrics, labels.FromStrings("__name__", "b_one"))
	for i := 0; i < 10; i++ {
		metrics = append(metrics, labels.FromStrings("__name__", "a_ten", "l", strconv.Itoa(i)))
		metrics = append(metrics, labels.FromStrings("__name__", "b_ten", "l", strconv.Itoa(i)))
	}
	for i := 0; i < 100; i++ {
		metrics = append(metrics, labels.FromStrings("__name__", "a_hundred", "l", strconv.Itoa(i)))
		metrics = append(metrics, labels.FromStrings("__name__", "b_hundred", "l", strconv.Itoa(i)))
	}

	for s := 0; s < 10000; s += 1 {
		a, err := storage.Appender()
		if err != nil {
			b.Fatal(err)
		}
		ts := int64(s * 10000) // 10s interval.
		for _, metric := range metrics {
			a.Add(metric, ts, float64(s))
		}
		if err := a.Commit(); err != nil {
			b.Fatal(err)
		}
	}

	type benchCase struct {
		expr     string
		interval time.Duration
		steps    int64
	}
	cases := []benchCase{
		// Simple rate.
		{
			expr:     "rate(a_X[1m])",
			interval: time.Second * 10,
		},
		// Holt-Winters and long ranges.
		{
			expr:     "holt_winters(a_one[1h], 0.3, 0.3)",
			interval: time.Second * 10,
			steps:    1,
		},
		{
			expr:     "holt_winters(a_one[6h], 0.3, 0.3)",
			interval: time.Second * 10,
			steps:    1,
		},
		{
			expr:     "holt_winters(a_one[1d], 0.3, 0.3)",
			interval: time.Second * 10,
			steps:    1,
		},
		{
			expr:     "changes(a_one[1d])",
			interval: time.Second * 10,
			steps:    1,
		},
		{
			expr:     "rate(a_one[1d])",
			interval: time.Second * 10,
			steps:    1,
		},
		// Unary operators.
		{
			expr:     "-a_X",
			interval: time.Second * 10,
		},
		// Binary operators.
		{
			expr:     "a_X - b_X",
			interval: time.Second * 10,
		},
	}

	// X in an expr will be replaced by different metric sizes.
	tmp := []benchCase{}
	for _, c := range cases {
		if !strings.Contains(c.expr, "X") {
			tmp = append(tmp, c)
		} else {
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "one", -1), interval: c.interval, steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "ten", -1), interval: c.interval, steps: c.steps})
			tmp = append(tmp, benchCase{expr: strings.Replace(c.expr, "X", "hundred", -1), interval: c.interval, steps: c.steps})
		}
	}
	cases = tmp

	// No step will be replaced by cases with the standard step.
	tmp = []benchCase{}
	for _, c := range cases {
		if c.steps != 0 {
			tmp = append(tmp, c)
		} else {
			tmp = append(tmp, benchCase{expr: c.expr, interval: c.interval, steps: 1})
			tmp = append(tmp, benchCase{expr: c.expr, interval: c.interval, steps: 10})
			tmp = append(tmp, benchCase{expr: c.expr, interval: c.interval, steps: 100})
		}
	}
	cases = tmp
	for _, c := range cases {
		name := fmt.Sprintf("expr=%s,interval=%s,steps=%d", c.expr, c.interval, c.steps)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				end := c.steps*int64(c.interval.Seconds()) - 1
				qry, err := engine.NewRangeQuery(storage, c.expr, time.Unix(86400, 0), time.Unix(86400+end, 0), c.interval)
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
