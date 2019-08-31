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

package promql

import (
	"testing"
)

func TestExprString(t *testing.T) {
	// A list of valid expressions that are expected to be
	// returned as out when calling String(). If out is empty the output
	// is expected to equal the input.
	inputs := []struct {
		in, out string
	}{
		{
			in:  `sum by() (task:errors:rate10s{job="s"})`,
			out: `sum(task:errors:rate10s{job="s"})`,
		},
		{
			in: `sum by(code) (task:errors:rate10s{job="s"})`,
		},
		{
			in: `sum without() (task:errors:rate10s{job="s"})`,
		},
		{
			in: `sum without(instance) (task:errors:rate10s{job="s"})`,
		},
		{
			in: `topk(5, task:errors:rate10s{job="s"})`,
		},
		{
			in: `count_values("value", task:errors:rate10s{job="s"})`,
		},
		{
			in: `a - on() c`,
		},
		{
			in: `a - on(b) c`,
		},
		{
			in: `a - on(b) group_left(x) c`,
		},
		{
			in: `a - on(b) group_left(x, y) c`,
		},
		{
			in:  `a - on(b) group_left c`,
			out: `a - on(b) group_left() c`,
		},
		{
			in: `a - on(b) group_left() (c)`,
		},
		{
			in: `a - ignoring(b) c`,
		},
		{
			in:  `a - ignoring() c`,
			out: `a - c`,
		},
		{
			in: `up > bool 0`,
		},
		{
			in: `a offset 1m`,
		},
		{
			in: `a{c="d"}[5m] offset 1m`,
		},
		{
			in: `a[5m] offset 1m`,
		},
		{
			in: `{__name__="a"}`,
		},
	}

	for _, test := range inputs {
		expr, err := ParseExpr(test.in)
		if err != nil {
			t.Fatalf("parsing error for %q: %s", test.in, err)
		}
		exp := test.in
		if test.out != "" {
			exp = test.out
		}
		if expr.String() != exp {
			t.Fatalf("expected %q to be returned as:\n%s\ngot:\n%s\n", test.in, exp, expr.String())
		}
	}
}
