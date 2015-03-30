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
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/metric"
)

var testExpr = []struct {
	input    string
	expected Expr
	fail     bool
}{
	// Scalars and scalar-to-scalar operations.
	{
		input:    "1",
		expected: &NumberLiteral{1},
	}, {
		input:    "+Inf",
		expected: &NumberLiteral{clientmodel.SampleValue(math.Inf(1))},
	}, {
		input:    "-Inf",
		expected: &NumberLiteral{clientmodel.SampleValue(math.Inf(-1))},
	}, {
		input:    ".5",
		expected: &NumberLiteral{0.5},
	}, {
		input:    "5.",
		expected: &NumberLiteral{5},
	}, {
		input:    "123.4567",
		expected: &NumberLiteral{123.4567},
	}, {
		input:    "5e-3",
		expected: &NumberLiteral{0.005},
	}, {
		input:    "5e3",
		expected: &NumberLiteral{5000},
	}, {
		input:    "0xc",
		expected: &NumberLiteral{12},
	}, {
		input:    "0755",
		expected: &NumberLiteral{493},
	}, {
		input:    "+5.5e-3",
		expected: &NumberLiteral{0.0055},
	}, {
		input:    "-0755",
		expected: &NumberLiteral{-493},
	}, {
		input:    "1 + 1",
		expected: &BinaryExpr{itemADD, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 - 1",
		expected: &BinaryExpr{itemSUB, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 * 1",
		expected: &BinaryExpr{itemMUL, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 % 1",
		expected: &BinaryExpr{itemMOD, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 / 1",
		expected: &BinaryExpr{itemDIV, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 == 1",
		expected: &BinaryExpr{itemEQL, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 != 1",
		expected: &BinaryExpr{itemNEQ, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 > 1",
		expected: &BinaryExpr{itemGTR, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 >= 1",
		expected: &BinaryExpr{itemGTE, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 < 1",
		expected: &BinaryExpr{itemLSS, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input:    "1 <= 1",
		expected: &BinaryExpr{itemLTE, &NumberLiteral{1}, &NumberLiteral{1}, nil},
	}, {
		input: "+1 + -2 * 1",
		expected: &BinaryExpr{
			Op:  itemADD,
			LHS: &NumberLiteral{1},
			RHS: &BinaryExpr{
				Op: itemMUL, LHS: &NumberLiteral{-2}, RHS: &NumberLiteral{1},
			},
		},
	}, {
		input: "1 + 2/(3*1)",
		expected: &BinaryExpr{
			Op:  itemADD,
			LHS: &NumberLiteral{1},
			RHS: &BinaryExpr{
				Op:  itemDIV,
				LHS: &NumberLiteral{2},
				RHS: &ParenExpr{&BinaryExpr{
					Op: itemMUL, LHS: &NumberLiteral{3}, RHS: &NumberLiteral{1},
				}},
			},
		},
	}, {
		input: "", fail: true,
	}, {
		input: "# just a comment\n\n", fail: true,
	}, {
		input: "1+", fail: true,
	}, {
		input: "2.5.", fail: true,
	}, {
		input: "100..4", fail: true,
	}, {
		input: "0deadbeef", fail: true,
	}, {
		input: "1 /", fail: true,
	}, {
		input: "*1", fail: true,
	}, {
		input: "(1))", fail: true,
	}, {
		input: "((1)", fail: true,
	}, {
		input: "(", fail: true,
	}, {
		input: "1 and 1", fail: true,
	}, {
		input: "1 or 1", fail: true,
	}, {
		input: "1 !~ 1", fail: true,
	}, {
		input: "1 =~ 1", fail: true,
	}, {
		input: "-some_metric", fail: true,
	}, {
		input: `-"string"`, fail: true,
	},
	// Vector binary operations.
	{
		input: "foo * bar",
		expected: &BinaryExpr{
			Op: itemMUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	}, {
		input: "foo == 1",
		expected: &BinaryExpr{
			Op: itemEQL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &NumberLiteral{1},
		},
	}, {
		input: "2.5 / bar",
		expected: &BinaryExpr{
			Op:  itemDIV,
			LHS: &NumberLiteral{2.5},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
		},
	}, {
		input: "foo and bar",
		expected: &BinaryExpr{
			Op: itemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		input: "foo or bar",
		expected: &BinaryExpr{
			Op: itemLOR,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test and/or precedence and reassigning of operands.
		input: "foo + bar or bla and blub",
		expected: &BinaryExpr{
			Op: itemLOR,
			LHS: &BinaryExpr{
				Op: itemADD,
				LHS: &VectorSelector{
					Name: "foo",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
					},
				},
				RHS: &VectorSelector{
					Name: "bar",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
					},
				},
				VectorMatching: &VectorMatching{Card: CardOneToOne},
			},
			RHS: &BinaryExpr{
				Op: itemLAND,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bla"},
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "blub"},
					},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test precedence and reassigning of operands.
		input: "bar + on(foo) bla / on(baz, buz) group_right(test) blub",
		expected: &BinaryExpr{
			Op: itemADD,
			LHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			RHS: &BinaryExpr{
				Op: itemDIV,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bla"},
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "blub"},
					},
				},
				VectorMatching: &VectorMatching{
					Card:    CardOneToMany,
					On:      clientmodel.LabelNames{"baz", "buz"},
					Include: clientmodel.LabelNames{"test"},
				},
			},
			VectorMatching: &VectorMatching{
				Card: CardOneToOne,
				On:   clientmodel.LabelNames{"foo"},
			},
		},
	}, {
		input: "foo * on(test,blub) bar",
		expected: &BinaryExpr{
			Op: itemMUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{
				Card: CardOneToOne,
				On:   clientmodel.LabelNames{"test", "blub"},
			},
		},
	}, {
		input: "foo and on(test,blub) bar",
		expected: &BinaryExpr{
			Op: itemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{
				Card: CardManyToMany,
				On:   clientmodel.LabelNames{"test", "blub"},
			},
		},
	}, {
		input: "foo / on(test,blub) group_left(bar) bar",
		expected: &BinaryExpr{
			Op: itemDIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{
				Card:    CardManyToOne,
				On:      clientmodel.LabelNames{"test", "blub"},
				Include: clientmodel.LabelNames{"bar"},
			},
		},
	}, {
		input: "foo - on(test,blub) group_right(bar,foo) bar",
		expected: &BinaryExpr{
			Op: itemSUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
				},
			},
			VectorMatching: &VectorMatching{
				Card:    CardOneToMany,
				On:      clientmodel.LabelNames{"test", "blub"},
				Include: clientmodel.LabelNames{"bar", "foo"},
			},
		},
	}, {
		input: "foo and 1", fail: true,
	}, {
		input: "1 and foo", fail: true,
	}, {
		input: "foo or 1", fail: true,
	}, {
		input: "1 or foo", fail: true,
	}, {
		input: "1 or on(bar) foo", fail: true,
	}, {
		input: "foo == on(bar) 10", fail: true,
	}, {
		input: "foo and on(bar) group_left(baz) bar", fail: true,
	}, {
		input: "foo and on(bar) group_right(baz) bar", fail: true,
	}, {
		input: "foo or on(bar) group_left(baz) bar", fail: true,
	}, {
		input: "foo or on(bar) group_right(baz) bar", fail: true,
	},
	// Test vector selector.
	{
		input: "foo",
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
			},
		},
	}, {
		input: "foo offset 5m",
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 5 * time.Minute,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
			},
		},
	}, {
		input: `foo:bar{a="b"}`,
		expected: &VectorSelector{
			Name:   "foo:bar",
			Offset: 0,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: "a", Value: "b"},
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo:bar"},
			},
		},
	}, {
		input: `foo{NaN='b'}`,
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: "NaN", Value: "b"},
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
			},
		},
	}, {
		input: `foo{a="b", foo!="bar", test=~"test", bar!~"baz"}`,
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: "a", Value: "b"},
				{Type: metric.NotEqual, Name: "foo", Value: "bar"},
				mustLabelMatcher(metric.RegexMatch, "test", "test"),
				mustLabelMatcher(metric.RegexNoMatch, "bar", "baz"),
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
			},
		},
	}, {
		input: `{`, fail: true,
	}, {
		input: `}`, fail: true,
	}, {
		input: `some{`, fail: true,
	}, {
		input: `some}`, fail: true,
	}, {
		input: `some_metric{a=b}`, fail: true,
	}, {
		input: `some_metric{a:b="b"}`, fail: true,
	}, {
		input: `foo{a*"b"}`, fail: true,
	}, {
		input: `foo{a>="b"}`, fail: true,
	}, {
		input: `foo{gibberish}`, fail: true,
	}, {
		input: `foo{1}`, fail: true,
	}, {
		input: `{}`, fail: true,
	}, {
		input: `foo{__name__="bar"}`, fail: true,
	}, {
		input: `:foo`, fail: true,
	},
	// Test matrix selector.
	{
		input: "test[5s]",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 0,
			Range:  5 * time.Second,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: "test[5m]",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 0,
			Range:  5 * time.Minute,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: "test[5h] OFFSET 5m",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 5 * time.Minute,
			Range:  5 * time.Hour,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: "test[5d] OFFSET 10s",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 10 * time.Second,
			Range:  5 * 24 * time.Hour,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: "test[5w] offset 2w",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 14 * 24 * time.Hour,
			Range:  5 * 7 * 24 * time.Hour,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: `test{a="b"}[5y] OFFSET 3d`,
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 3 * 24 * time.Hour,
			Range:  5 * 365 * 24 * time.Hour,
			LabelMatchers: metric.LabelMatchers{
				{Type: metric.Equal, Name: "a", Value: "b"},
				{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "test"},
			},
		},
	}, {
		input: `foo[5mm]`, fail: true,
	}, {
		input: `foo[0m]`, fail: true,
	}, {
		input: `foo[5m30s]`, fail: true,
	}, {
		input: `foo[5m] OFFSET 1h30m`, fail: true,
	}, {
		input: `foo[]`, fail: true,
	}, {
		input: `foo[1]`, fail: true,
	}, {
		input: `some_metric[5m] OFFSET 1`, fail: true,
	}, {
		input: `some_metric[5m] OFFSET 1mm`, fail: true,
	}, {
		input: `some_metric[5m] OFFSET`, fail: true,
	}, {
		input: `(foo + bar)[5m]`, fail: true,
	},
	// Test aggregation.
	{
		input: "sum by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: itemSum,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo"},
		},
	}, {
		input: "sum by (foo) keeping_extra (some_metric)",
		expected: &AggregateExpr{
			Op:              itemSum,
			KeepExtraLabels: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo"},
		},
	}, {
		input: "sum (some_metric) by (foo,bar) keeping_extra",
		expected: &AggregateExpr{
			Op:              itemSum,
			KeepExtraLabels: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo", "bar"},
		},
	}, {
		input: "avg by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: itemAvg,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo"},
		},
	}, {
		input: "COUNT by (foo) keeping_extra (some_metric)",
		expected: &AggregateExpr{
			Op: itemCount,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping:        clientmodel.LabelNames{"foo"},
			KeepExtraLabels: true,
		},
	}, {
		input: "MIN (some_metric) by (foo) keeping_extra",
		expected: &AggregateExpr{
			Op: itemMin,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping:        clientmodel.LabelNames{"foo"},
			KeepExtraLabels: true,
		},
	}, {
		input: "max by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: itemMax,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo"},
		},
	}, {
		input: "stddev(some_metric)",
		expected: &AggregateExpr{
			Op: itemStddev,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
		},
	}, {
		input: "stdvar by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: itemStdvar,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: metric.LabelMatchers{
					{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
				},
			},
			Grouping: clientmodel.LabelNames{"foo"},
		},
	}, {
		input: `sum some_metric by (test)`, fail: true,
	}, {
		input: `sum (some_metric) by test`, fail: true,
	}, {
		input: `sum (some_metric) by ()`, fail: true,
	}, {
		input: `sum (some_metric) by test`, fail: true,
	}, {
		input: `some_metric[5m] OFFSET`, fail: true,
	}, {
		input: `sum () by (test)`, fail: true,
	}, {
		input: "MIN keeping_extra (some_metric) by (foo)", fail: true,
	}, {
		input: "MIN by(test) (some_metric) keeping_extra", fail: true,
	},
	// Test function calls.
	{
		input: "time()",
		expected: &Call{
			Func: mustGetFunction("time"),
		},
	}, {
		input: `floor(some_metric{foo!="bar"})`,
		expected: &Call{
			Func: mustGetFunction("floor"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.NotEqual, Name: "foo", Value: "bar"},
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
					},
				},
			},
		},
	}, {
		input: "rate(some_metric[5m])",
		expected: &Call{
			Func: mustGetFunction("rate"),
			Args: Expressions{
				&MatrixSelector{
					Name: "some_metric",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
					},
					Range: 5 * time.Minute,
				},
			},
		},
	}, {
		input: "round(some_metric)",
		expected: &Call{
			Func: mustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
					},
				},
			},
		},
	}, {
		input: "round(some_metric, 5)",
		expected: &Call{
			Func: mustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
					},
				},
				&NumberLiteral{5},
			},
		},
	}, {
		input: "floor()", fail: true,
	}, {
		input: "floor(some_metric, other_metric)", fail: true,
	}, {
		input: "floor(1)", fail: true,
	}, {
		input: "non_existant_function_far_bar()", fail: true,
	}, {
		input: "rate(some_metric)", fail: true,
	},
}

func TestParseExpressions(t *testing.T) {
	for _, test := range testExpr {

		parser := newParser("test", test.input)

		expr, err := parser.parseExpr()
		if !test.fail && err != nil {
			t.Errorf("error in input '%s'", test.input)
			t.Fatalf("could not parse: %s", err)
		}
		if test.fail && err != nil {
			continue
		}

		err = parser.typecheck(expr)
		if !test.fail && err != nil {
			t.Errorf("error on input '%s'", test.input)
			t.Fatalf("typecheck failed: %s", err)
		}

		if test.fail {
			if err != nil {
				continue
			}
			t.Errorf("error on input '%s'", test.input)
			t.Fatalf("failure expected, but passed with result: %q", expr)
		}

		if !reflect.DeepEqual(expr, test.expected) {
			t.Errorf("error on input '%s'", test.input)
			t.Fatalf("no match\n\nexpected:\n%s\ngot: \n%s\n", Tree(test.expected), Tree(expr))
		}
	}
}

// NaN has no equality. Thus, we need a separate test for it.
func TestNaNExpression(t *testing.T) {
	parser := newParser("test", "NaN")

	expr, err := parser.parseExpr()
	if err != nil {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("coud not parse: %s", err)
	}

	nl, ok := expr.(*NumberLiteral)
	if !ok {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("expected number literal but got %T", expr)
	}

	if !math.IsNaN(float64(nl.Val)) {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("expected 'NaN' in number literal but got %d", nl.Val)
	}
}

var testStatement = []struct {
	input    string
	expected Statements
	fail     bool
}{
	{
		// Test a file-like input.
		input: `
			# A simple test recording rule.
			dc:http_request:rate5m = sum(rate(http_request_count[5m])) by (dc)
	
			# A simple test alerting rule.
			ALERT GlobalRequestRateLow IF(dc:http_request:rate5m < 10000) FOR 5m WITH {
			    service = "testservice"
			    # ... more fields here ...
			  }
			  SUMMARY "Global request rate low"
			  DESCRIPTION "The global request rate is low"

			foo = bar{label1="value1"}

			ALERT BazAlert IF foo > 10 WITH {}
			  SUMMARY "Baz"
			  DESCRIPTION "BazAlert"
		`,
		expected: Statements{
			&RecordStmt{
				Name: "dc:http_request:rate5m",
				Expr: &AggregateExpr{
					Op:       itemSum,
					Grouping: clientmodel.LabelNames{"dc"},
					Expr: &Call{
						Func: mustGetFunction("rate"),
						Args: Expressions{
							&MatrixSelector{
								Name: "http_request_count",
								LabelMatchers: metric.LabelMatchers{
									{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "http_request_count"},
								},
								Range: 5 * time.Minute,
							},
						},
					},
				},
				Labels: nil,
			},
			&AlertStmt{
				Name: "GlobalRequestRateLow",
				Expr: &ParenExpr{&BinaryExpr{
					Op: itemLSS,
					LHS: &VectorSelector{
						Name: "dc:http_request:rate5m",
						LabelMatchers: metric.LabelMatchers{
							{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "dc:http_request:rate5m"},
						},
					},
					RHS: &NumberLiteral{10000},
				}},
				Labels:      clientmodel.LabelSet{"service": "testservice"},
				Duration:    5 * time.Minute,
				Summary:     "Global request rate low",
				Description: "The global request rate is low",
			},
			&RecordStmt{
				Name: "foo",
				Expr: &VectorSelector{
					Name: "bar",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: "label1", Value: "value1"},
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
					},
				},
				Labels: nil,
			},
			&AlertStmt{
				Name: "BazAlert",
				Expr: &BinaryExpr{
					Op: itemGTR,
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: metric.LabelMatchers{
							{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "foo"},
						},
					},
					RHS: &NumberLiteral{10},
				},
				Labels:      clientmodel.LabelSet{},
				Summary:     "Baz",
				Description: "BazAlert",
			},
		},
	}, {
		input: `foo{x="", a="z"} = bar{a="b", x=~"y"}`,
		expected: Statements{
			&RecordStmt{
				Name: "foo",
				Expr: &VectorSelector{
					Name: "bar",
					LabelMatchers: metric.LabelMatchers{
						{Type: metric.Equal, Name: "a", Value: "b"},
						mustLabelMatcher(metric.RegexMatch, "x", "y"),
						{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "bar"},
					},
				},
				Labels: clientmodel.LabelSet{"x": "", "a": "z"},
			},
		},
	}, {
		input: `ALERT SomeName IF some_metric > 1 
			SUMMARY "Global request rate low"
			DESCRIPTION "The global request rate is low"
		`,
		expected: Statements{
			&AlertStmt{
				Name: "SomeName",
				Expr: &BinaryExpr{
					Op: itemGTR,
					LHS: &VectorSelector{
						Name: "some_metric",
						LabelMatchers: metric.LabelMatchers{
							{Type: metric.Equal, Name: clientmodel.MetricNameLabel, Value: "some_metric"},
						},
					},
					RHS: &NumberLiteral{1},
				},
				Labels:      clientmodel.LabelSet{},
				Summary:     "Global request rate low",
				Description: "The global request rate is low",
			},
		},
	}, {
		input: `
			# A simple test alerting rule.
			ALERT GlobalRequestRateLow IF(dc:http_request:rate5m < 10000) FOR 5 WITH {
			    service = "testservice"
			    # ... more fields here ... 
			  }
			  SUMMARY "Global request rate low"
			  DESCRIPTION "The global request rate is low"
	  	`,
		fail: true,
	}, {
		input:    "",
		expected: Statements{},
	}, {
		input: "foo = time()",
		fail:  true,
	}, {
		input: "foo = 1",
		fail:  true,
	}, {
		input: "foo = bar[5m]",
		fail:  true,
	}, {
		input: `foo = "test"`,
		fail:  true,
	}, {
		input: `foo = `,
		fail:  true,
	}, {
		input: `foo{a!="b"} = bar`,
		fail:  true,
	}, {
		input: `foo{a=~"b"} = bar`,
		fail:  true,
	}, {
		input: `foo{a!~"b"} = bar`,
		fail:  true,
	}, {
		input: `ALERT SomeName IF time() WITH {} 
			SUMMARY "Global request rate low"
			DESCRIPTION "The global request rate is low"
		`,
		fail: true,
	}, {
		input: `ALERT SomeName IF some_metric > 1 WITH {} 
			SUMMARY "Global request rate low"
		`,
		fail: true,
	}, {
		input: `ALERT SomeName IF some_metric > 1 
			DESCRIPTION "The global request rate is low"
		`,
		fail: true,
	},
}

func TestParseStatements(t *testing.T) {
	for _, test := range testStatement {
		parser := newParser("test", test.input)

		stmts, err := parser.parseStmts()
		if !test.fail && err != nil {
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("could not parse: %s", err)
		}
		if test.fail && err != nil {
			continue
		}

		err = parser.typecheck(stmts)
		if !test.fail && err != nil {
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("typecheck failed: %s", err)
		}

		if test.fail {
			if err != nil {
				continue
			}
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("failure expected, but passed")
		}

		if !reflect.DeepEqual(stmts, test.expected) {
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("no match\n\nexpected:\n%s\ngot: \n%s\n", Tree(test.expected), Tree(stmts))
		}
	}
}

func mustLabelMatcher(mt metric.MatchType, name clientmodel.LabelName, val clientmodel.LabelValue) *metric.LabelMatcher {
	m, err := metric.NewLabelMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}

func mustGetFunction(name string) *Function {
	f, ok := GetFunction(name)
	if !ok {
		panic(fmt.Errorf("function %q does not exist", name))
	}
	return f
}
