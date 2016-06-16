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
			in: `sum(task:errors:rate10s{job="s"}) BY (code)`,
		},
		{
			in: `sum(task:errors:rate10s{job="s"}) BY (code) KEEP_COMMON`,
		},
		{
			in: `sum(task:errors:rate10s{job="s"}) KEEP_COMMON`,
		},
		{
			in: `sum(task:errors:rate10s{job="s"}) WITHOUT (instance)`,
		},
		{
			in: `a - ON(b) c`,
		},
		{
			in: `a - ON(b) GROUP_LEFT(x) c`,
		},
		{
			in: `a - ON(b) GROUP_LEFT(x, y) c`,
		},
		{
			in: `a - ON(b) GROUP_LEFT c`,
		},
		{
			in: `a - IGNORING(b) c`,
		},
		{
			in: `up > BOOL 0`,
		},
		{
			in: `a OFFSET 1m`,
		},
		{
			in: `a{c="d"}[5m] OFFSET 1m`,
		},
		{
			in: `a[5m] OFFSET 1m`,
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

func TestStmtsString(t *testing.T) {
	// A list of valid statements that are expected to be returned as out when
	// calling String(). If out is empty the output is expected to equal the
	// input.
	inputs := []struct {
		in, out string
	}{
		{
			in:  `ALERT foo IF up == 0 FOR 1m`,
			out: "ALERT foo\n\tIF up == 0\n\tFOR 1m",
		},
		{
			in:  `ALERT foo IF up == 0 FOR 1m ANNOTATIONS {summary="foo"}`,
			out: "ALERT foo\n\tIF up == 0\n\tFOR 1m\n\tANNOTATIONS {summary=\"foo\"}",
		},
	}

	for _, test := range inputs {
		expr, err := ParseStmts(test.in)
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
