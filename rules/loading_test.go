// Copyright 2013 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestFileLoader(t *testing.T) {
	out, errs := FileLoader{}.Load("fixtures/loading_rules.yaml")
	testutil.Assert(t, len(errs) == 0, "should parse successfully")

	exprA, err := parser.ParseExpr("sum by (job)(rate(http_requests_total[5m]))")
	testutil.Ok(t, err)

	exprB, err := parser.ParseExpr(`job:request_latency_seconds:mean5m{job="myjob"} > 0.5`)
	testutil.Ok(t, err)

	expected := []*ParsedGroup{
		&ParsedGroup{
			File: "fixtures/loading_rules.yaml",
			Name: "recording",
			Rules: []*ParsedRule{
				{
					Name: "job:http_requests:rate5m",
					Expr: exprA,
				},
			},
		},
		&ParsedGroup{
			File:     "fixtures/loading_rules.yaml",
			Name:     "alerts",
			Interval: 30 * time.Second,
			Rules: []*ParsedRule{
				{
					Name:   "HighRequestLatency",
					Expr:   exprB,
					Alert:  true,
					Period: 10 * time.Minute,
					Labels: labels.FromMap(map[string]string{
						"severity": "page",
					}),
					Annotations: labels.FromMap(map[string]string{
						"summary": "High request latency",
					}),
				},
			},
		},
	}
	testutil.Equals(t, expected, out, "must parse successfully")
}
