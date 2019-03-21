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
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
)

var testExpr = []struct {
	input    string // The input to be parsed.
	expected Expr   // The expected expression AST.
	fail     bool   // Whether parsing is supposed to fail.
	errMsg   string // If not empty the parsing error has to contain this string.
}{
	// Scalars and scalar-to-scalar operations.
	{
		input:    "1",
		expected: &NumberLiteral{1},
	}, {
		input:    "+Inf",
		expected: &NumberLiteral{math.Inf(1)},
	}, {
		input:    "-Inf",
		expected: &NumberLiteral{math.Inf(-1)},
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
		expected: &BinaryExpr{ItemADD, &NumberLiteral{1}, &NumberLiteral{1}, nil, false},
	}, {
		input:    "1 - 1",
		expected: &BinaryExpr{ItemSUB, &NumberLiteral{1}, &NumberLiteral{1}, nil, false},
	}, {
		input:    "1 * 1",
		expected: &BinaryExpr{ItemMUL, &NumberLiteral{1}, &NumberLiteral{1}, nil, false},
	}, {
		input:    "1 % 1",
		expected: &BinaryExpr{ItemMOD, &NumberLiteral{1}, &NumberLiteral{1}, nil, false},
	}, {
		input:    "1 / 1",
		expected: &BinaryExpr{ItemDIV, &NumberLiteral{1}, &NumberLiteral{1}, nil, false},
	}, {
		input:    "1 == bool 1",
		expected: &BinaryExpr{ItemEQL, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input:    "1 != bool 1",
		expected: &BinaryExpr{ItemNEQ, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input:    "1 > bool 1",
		expected: &BinaryExpr{ItemGTR, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input:    "1 >= bool 1",
		expected: &BinaryExpr{ItemGTE, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input:    "1 < bool 1",
		expected: &BinaryExpr{ItemLSS, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input:    "1 <= bool 1",
		expected: &BinaryExpr{ItemLTE, &NumberLiteral{1}, &NumberLiteral{1}, nil, true},
	}, {
		input: "+1 + -2 * 1",
		expected: &BinaryExpr{
			Op:  ItemADD,
			LHS: &NumberLiteral{1},
			RHS: &BinaryExpr{
				Op: ItemMUL, LHS: &NumberLiteral{-2}, RHS: &NumberLiteral{1},
			},
		},
	}, {
		input: "1 + 2/(3*1)",
		expected: &BinaryExpr{
			Op:  ItemADD,
			LHS: &NumberLiteral{1},
			RHS: &BinaryExpr{
				Op:  ItemDIV,
				LHS: &NumberLiteral{2},
				RHS: &ParenExpr{&BinaryExpr{
					Op: ItemMUL, LHS: &NumberLiteral{3}, RHS: &NumberLiteral{1},
				}},
			},
		},
	}, {
		input: "1 < bool 2 - 1 * 2",
		expected: &BinaryExpr{
			Op:         ItemLSS,
			ReturnBool: true,
			LHS:        &NumberLiteral{1},
			RHS: &BinaryExpr{
				Op:  ItemSUB,
				LHS: &NumberLiteral{2},
				RHS: &BinaryExpr{
					Op: ItemMUL, LHS: &NumberLiteral{1}, RHS: &NumberLiteral{2},
				},
			},
		},
	}, {
		input: "-some_metric",
		expected: &UnaryExpr{
			Op: ItemSUB,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
		},
	}, {
		input: "+some_metric",
		expected: &UnaryExpr{
			Op: ItemADD,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
		},
	}, {
		input:  "",
		fail:   true,
		errMsg: "no expression found in input",
	}, {
		input:  "# just a comment\n\n",
		fail:   true,
		errMsg: "no expression found in input",
	}, {
		input:  "1+",
		fail:   true,
		errMsg: "no valid expression found",
	}, {
		input:  ".",
		fail:   true,
		errMsg: "unexpected character: '.'",
	}, {
		input:  "2.5.",
		fail:   true,
		errMsg: "could not parse remaining input \".\"...",
	}, {
		input:  "100..4",
		fail:   true,
		errMsg: "could not parse remaining input \".4\"...",
	}, {
		input:  "0deadbeef",
		fail:   true,
		errMsg: "bad number or duration syntax: \"0de\"",
	}, {
		input:  "1 /",
		fail:   true,
		errMsg: "no valid expression found",
	}, {
		input:  "*1",
		fail:   true,
		errMsg: "no valid expression found",
	}, {
		input:  "(1))",
		fail:   true,
		errMsg: "could not parse remaining input \")\"...",
	}, {
		input:  "((1)",
		fail:   true,
		errMsg: "unclosed left parenthesis",
	}, {
		input:  "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999",
		fail:   true,
		errMsg: "out of range",
	}, {
		input:  "(",
		fail:   true,
		errMsg: "unclosed left parenthesis",
	}, {
		input:  "1 and 1",
		fail:   true,
		errMsg: "set operator \"and\" not allowed in binary scalar expression",
	}, {
		input:  "1 == 1",
		fail:   true,
		errMsg: "parse error at char 7: comparisons between scalars must use BOOL modifier",
	}, {
		input:  "1 or 1",
		fail:   true,
		errMsg: "set operator \"or\" not allowed in binary scalar expression",
	}, {
		input:  "1 unless 1",
		fail:   true,
		errMsg: "set operator \"unless\" not allowed in binary scalar expression",
	}, {
		input:  "1 !~ 1",
		fail:   true,
		errMsg: "could not parse remaining input \"!~ 1\"...",
	}, {
		input:  "1 =~ 1",
		fail:   true,
		errMsg: "could not parse remaining input \"=~ 1\"...",
	}, {
		input:  `-"string"`,
		fail:   true,
		errMsg: `unary expression only allowed on expressions of type scalar or instant vector, got "string"`,
	}, {
		input:  `-test[5m]`,
		fail:   true,
		errMsg: `unary expression only allowed on expressions of type scalar or instant vector, got "range vector"`,
	}, {
		input:  `*test`,
		fail:   true,
		errMsg: "no valid expression found",
	}, {
		input:  "1 offset 1d",
		fail:   true,
		errMsg: "offset modifier must be preceded by an instant or range selector",
	}, {
		input:  "a - on(b) ignoring(c) d",
		fail:   true,
		errMsg: "parse error at char 11: no valid expression found",
	},
	// Vector binary operations.
	{
		input: "foo * bar",
		expected: &BinaryExpr{
			Op: ItemMUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	}, {
		input: "foo == 1",
		expected: &BinaryExpr{
			Op: ItemEQL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &NumberLiteral{1},
		},
	}, {
		input: "foo == bool 1",
		expected: &BinaryExpr{
			Op: ItemEQL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS:        &NumberLiteral{1},
			ReturnBool: true,
		},
	}, {
		input: "2.5 / bar",
		expected: &BinaryExpr{
			Op:  ItemDIV,
			LHS: &NumberLiteral{2.5},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
		},
	}, {
		input: "foo and bar",
		expected: &BinaryExpr{
			Op: ItemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		input: "foo or bar",
		expected: &BinaryExpr{
			Op: ItemLOR,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		input: "foo unless bar",
		expected: &BinaryExpr{
			Op: ItemLUnless,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test and/or precedence and reassigning of operands.
		input: "foo + bar or bla and blub",
		expected: &BinaryExpr{
			Op: ItemLOR,
			LHS: &BinaryExpr{
				Op: ItemADD,
				LHS: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
					},
				},
				RHS: &VectorSelector{
					Name: "bar",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
					},
				},
				VectorMatching: &VectorMatching{Card: CardOneToOne},
			},
			RHS: &BinaryExpr{
				Op: ItemLAND,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bla"),
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "blub"),
					},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test and/or/unless precedence.
		input: "foo and bar unless baz or qux",
		expected: &BinaryExpr{
			Op: ItemLOR,
			LHS: &BinaryExpr{
				Op: ItemLUnless,
				LHS: &BinaryExpr{
					Op: ItemLAND,
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
						},
					},
					VectorMatching: &VectorMatching{Card: CardManyToMany},
				},
				RHS: &VectorSelector{
					Name: "baz",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "baz"),
					},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			RHS: &VectorSelector{
				Name: "qux",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "qux"),
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test precedence and reassigning of operands.
		input: "bar + on(foo) bla / on(baz, buz) group_right(test) blub",
		expected: &BinaryExpr{
			Op: ItemADD,
			LHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			RHS: &BinaryExpr{
				Op: ItemDIV,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bla"),
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "blub"),
					},
				},
				VectorMatching: &VectorMatching{
					Card:           CardOneToMany,
					MatchingLabels: []string{"baz", "buz"},
					On:             true,
					Include:        []string{"test"},
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToOne,
				MatchingLabels: []string{"foo"},
				On:             true,
			},
		},
	}, {
		input: "foo * on(test,blub) bar",
		expected: &BinaryExpr{
			Op: ItemMUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	}, {
		input: "foo * on(test,blub) group_left bar",
		expected: &BinaryExpr{
			Op: ItemMUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	}, {
		input: "foo and on(test,blub) bar",
		expected: &BinaryExpr{
			Op: ItemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	}, {
		input: "foo and on() bar",
		expected: &BinaryExpr{
			Op: ItemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{},
				On:             true,
			},
		},
	}, {
		input: "foo and ignoring(test,blub) bar",
		expected: &BinaryExpr{
			Op: ItemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"test", "blub"},
			},
		},
	}, {
		input: "foo and ignoring() bar",
		expected: &BinaryExpr{
			Op: ItemLAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{},
			},
		},
	}, {
		input: "foo unless on(bar) baz",
		expected: &BinaryExpr{
			Op: ItemLUnless,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "baz",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "baz"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"bar"},
				On:             true,
			},
		},
	}, {
		input: "foo / on(test,blub) group_left(bar) bar",
		expected: &BinaryExpr{
			Op: ItemDIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
				Include:        []string{"bar"},
			},
		},
	}, {
		input: "foo / ignoring(test,blub) group_left(blub) bar",
		expected: &BinaryExpr{
			Op: ItemDIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"blub"},
			},
		},
	}, {
		input: "foo / ignoring(test,blub) group_left(bar) bar",
		expected: &BinaryExpr{
			Op: ItemDIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar"},
			},
		},
	}, {
		input: "foo - on(test,blub) group_right(bar,foo) bar",
		expected: &BinaryExpr{
			Op: ItemSUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToMany,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar", "foo"},
				On:             true,
			},
		},
	}, {
		input: "foo - ignoring(test,blub) group_right(bar,foo) bar",
		expected: &BinaryExpr{
			Op: ItemSUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToMany,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar", "foo"},
			},
		},
	}, {
		input:  "foo and 1",
		fail:   true,
		errMsg: "set operator \"and\" not allowed in binary scalar expression",
	}, {
		input:  "1 and foo",
		fail:   true,
		errMsg: "set operator \"and\" not allowed in binary scalar expression",
	}, {
		input:  "foo or 1",
		fail:   true,
		errMsg: "set operator \"or\" not allowed in binary scalar expression",
	}, {
		input:  "1 or foo",
		fail:   true,
		errMsg: "set operator \"or\" not allowed in binary scalar expression",
	}, {
		input:  "foo unless 1",
		fail:   true,
		errMsg: "set operator \"unless\" not allowed in binary scalar expression",
	}, {
		input:  "1 unless foo",
		fail:   true,
		errMsg: "set operator \"unless\" not allowed in binary scalar expression",
	}, {
		input:  "1 or on(bar) foo",
		fail:   true,
		errMsg: "vector matching only allowed between instant vectors",
	}, {
		input:  "foo == on(bar) 10",
		fail:   true,
		errMsg: "vector matching only allowed between instant vectors",
	}, {
		input:  "foo and on(bar) group_left(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"and\" operation",
	}, {
		input:  "foo and on(bar) group_right(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"and\" operation",
	}, {
		input:  "foo or on(bar) group_left(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"or\" operation",
	}, {
		input:  "foo or on(bar) group_right(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"or\" operation",
	}, {
		input:  "foo unless on(bar) group_left(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"unless\" operation",
	}, {
		input:  "foo unless on(bar) group_right(baz) bar",
		fail:   true,
		errMsg: "no grouping allowed for \"unless\" operation",
	}, {
		input:  `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
		fail:   true,
		errMsg: "label \"instance\" must not occur in ON and GROUP clause at once",
	}, {
		input:  "foo + bool bar",
		fail:   true,
		errMsg: "bool modifier can only be used on comparison operators",
	}, {
		input:  "foo + bool 10",
		fail:   true,
		errMsg: "bool modifier can only be used on comparison operators",
	}, {
		input:  "foo and bool 10",
		fail:   true,
		errMsg: "bool modifier can only be used on comparison operators",
	},
	// Test Vector selector.
	{
		input: "foo",
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
			},
		},
	}, {
		input: "foo offset 5m",
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 5 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
			},
		},
	}, {
		input: `foo:bar{a="bc"}`,
		expected: &VectorSelector{
			Name:   "foo:bar",
			Offset: 0,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, "a", "bc"),
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo:bar"),
			},
		},
	}, {
		input: `foo{NaN='bc'}`,
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, "NaN", "bc"),
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
			},
		},
	}, {
		input: `foo{a="b", foo!="bar", test=~"test", bar!~"baz"}`,
		expected: &VectorSelector{
			Name:   "foo",
			Offset: 0,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, "a", "b"),
				mustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
				mustLabelMatcher(labels.MatchRegexp, "test", "test"),
				mustLabelMatcher(labels.MatchNotRegexp, "bar", "baz"),
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
			},
		},
	}, {
		input:  `{`,
		fail:   true,
		errMsg: "unexpected end of input inside braces",
	}, {
		input:  `}`,
		fail:   true,
		errMsg: "unexpected character: '}'",
	}, {
		input:  `some{`,
		fail:   true,
		errMsg: "unexpected end of input inside braces",
	}, {
		input:  `some}`,
		fail:   true,
		errMsg: "could not parse remaining input \"}\"...",
	}, {
		input:  `some_metric{a=b}`,
		fail:   true,
		errMsg: "unexpected identifier \"b\" in label matching, expected string",
	}, {
		input:  `some_metric{a:b="b"}`,
		fail:   true,
		errMsg: "unexpected character inside braces: ':'",
	}, {
		input:  `foo{a*"b"}`,
		fail:   true,
		errMsg: "unexpected character inside braces: '*'",
	}, {
		input: `foo{a>="b"}`,
		fail:  true,
		// TODO(fabxc): willingly lexing wrong tokens allows for more precise error
		// messages from the parser - consider if this is an option.
		errMsg: "unexpected character inside braces: '>'",
	}, {
		input:  "some_metric{a=\"\xff\"}",
		fail:   true,
		errMsg: "parse error at char 15: invalid UTF-8 rune",
	}, {
		input:  `foo{gibberish}`,
		fail:   true,
		errMsg: "expected label matching operator but got }",
	}, {
		input:  `foo{1}`,
		fail:   true,
		errMsg: "unexpected character inside braces: '1'",
	}, {
		input:  `{}`,
		fail:   true,
		errMsg: "vector selector must contain label matchers or metric name",
	}, {
		input:  `{x=""}`,
		fail:   true,
		errMsg: "vector selector must contain at least one non-empty matcher",
	}, {
		input:  `{x=~".*"}`,
		fail:   true,
		errMsg: "vector selector must contain at least one non-empty matcher",
	}, {
		input:  `{x!~".+"}`,
		fail:   true,
		errMsg: "vector selector must contain at least one non-empty matcher",
	}, {
		input:  `{x!="a"}`,
		fail:   true,
		errMsg: "vector selector must contain at least one non-empty matcher",
	}, {
		input:  `foo{__name__="bar"}`,
		fail:   true,
		errMsg: "metric name must not be set twice: \"foo\" or \"bar\"",
	},
	// Test matrix selector.
	{
		input: "test[5s]",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 0,
			Range:  5 * time.Second,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input: "test[5m]",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 0,
			Range:  5 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input: "test[5h] OFFSET 5m",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 5 * time.Minute,
			Range:  5 * time.Hour,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input: "test[5d] OFFSET 10s",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 10 * time.Second,
			Range:  5 * 24 * time.Hour,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input: "test[5w] offset 2w",
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 14 * 24 * time.Hour,
			Range:  5 * 7 * 24 * time.Hour,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input: `test{a="b"}[5y] OFFSET 3d`,
		expected: &MatrixSelector{
			Name:   "test",
			Offset: 3 * 24 * time.Hour,
			Range:  5 * 365 * 24 * time.Hour,
			LabelMatchers: []*labels.Matcher{
				mustLabelMatcher(labels.MatchEqual, "a", "b"),
				mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "test"),
			},
		},
	}, {
		input:  `foo[5mm]`,
		fail:   true,
		errMsg: "bad duration syntax: \"5mm\"",
	}, {
		input:  `foo[0m]`,
		fail:   true,
		errMsg: "duration must be greater than 0",
	}, {
		input:  `foo[5m30s]`,
		fail:   true,
		errMsg: "bad duration syntax: \"5m3\"",
	}, {
		input:  `foo[5m] OFFSET 1h30m`,
		fail:   true,
		errMsg: "bad number or duration syntax: \"1h3\"",
	}, {
		input: `foo["5m"]`,
		fail:  true,
	}, {
		input:  `foo[]`,
		fail:   true,
		errMsg: "missing unit character in duration",
	}, {
		input:  `foo[1]`,
		fail:   true,
		errMsg: "missing unit character in duration",
	}, {
		input:  `some_metric[5m] OFFSET 1`,
		fail:   true,
		errMsg: "unexpected number \"1\" in offset, expected duration",
	}, {
		input:  `some_metric[5m] OFFSET 1mm`,
		fail:   true,
		errMsg: "bad number or duration syntax: \"1mm\"",
	}, {
		input:  `some_metric[5m] OFFSET`,
		fail:   true,
		errMsg: "unexpected end of input in offset, expected duration",
	}, {
		input:  `some_metric OFFSET 1m[5m]`,
		fail:   true,
		errMsg: "parse error at char 25: unexpected \"]\" in subquery selector, expected \":\"",
	}, {
		input:  `(foo + bar)[5m]`,
		fail:   true,
		errMsg: "parse error at char 15: unexpected \"]\" in subquery selector, expected \":\"",
	},
	// Test aggregation.
	{
		input: "sum by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: ItemSum,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "avg by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: ItemAvg,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "max by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: ItemMax,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "sum without (foo) (some_metric)",
		expected: &AggregateExpr{
			Op:      ItemSum,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "sum (some_metric) without (foo)",
		expected: &AggregateExpr{
			Op:      ItemSum,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "stddev(some_metric)",
		expected: &AggregateExpr{
			Op: ItemStddev,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
		},
	}, {
		input: "stdvar by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: ItemStdvar,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"foo"},
		},
	}, {
		input: "sum by ()(some_metric)",
		expected: &AggregateExpr{
			Op: ItemSum,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{},
		},
	}, {
		input: "topk(5, some_metric)",
		expected: &AggregateExpr{
			Op: ItemTopK,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Param: &NumberLiteral{5},
		},
	}, {
		input: "count_values(\"value\", some_metric)",
		expected: &AggregateExpr{
			Op: ItemCountValues,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Param: &StringLiteral{"value"},
		},
	}, {
		// Test usage of keywords as label names.
		input: "sum without(and, by, avg, count, alert, annotations)(some_metric)",
		expected: &AggregateExpr{
			Op:      ItemSum,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
			},
			Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
		},
	}, {
		input:  "sum without(==)(some_metric)",
		fail:   true,
		errMsg: "unexpected <op:==> in grouping opts, expected label",
	}, {
		input:  `sum some_metric by (test)`,
		fail:   true,
		errMsg: "unexpected identifier \"some_metric\" in aggregation, expected \"(\"",
	}, {
		input:  `sum (some_metric) by test`,
		fail:   true,
		errMsg: "unexpected identifier \"test\" in grouping opts, expected \"(\"",
	}, {
		input:  `sum (some_metric) by test`,
		fail:   true,
		errMsg: "unexpected identifier \"test\" in grouping opts, expected \"(\"",
	}, {
		input:  `sum () by (test)`,
		fail:   true,
		errMsg: "no valid expression found",
	}, {
		input:  "MIN keep_common (some_metric)",
		fail:   true,
		errMsg: "parse error at char 5: unexpected identifier \"keep_common\" in aggregation, expected \"(\"",
	}, {
		input:  "MIN (some_metric) keep_common",
		fail:   true,
		errMsg: "could not parse remaining input \"keep_common\"...",
	}, {
		input:  `sum (some_metric) without (test) by (test)`,
		fail:   true,
		errMsg: "could not parse remaining input \"by (test)\"...",
	}, {
		input:  `sum without (test) (some_metric) by (test)`,
		fail:   true,
		errMsg: "could not parse remaining input \"by (test)\"...",
	}, {
		input:  `topk(some_metric)`,
		fail:   true,
		errMsg: "parse error at char 17: unexpected \")\" in aggregation, expected \",\"",
	}, {
		input:  `topk(some_metric, other_metric)`,
		fail:   true,
		errMsg: "parse error at char 32: expected type scalar in aggregation parameter, got instant vector",
	}, {
		input:  `count_values(5, other_metric)`,
		fail:   true,
		errMsg: "parse error at char 30: expected type string in aggregation parameter, got scalar",
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
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
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
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
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
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
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
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
					},
				},
				&NumberLiteral{5},
			},
		},
	}, {
		input:  "floor()",
		fail:   true,
		errMsg: "expected 1 argument(s) in call to \"floor\", got 0",
	}, {
		input:  "floor(some_metric, other_metric)",
		fail:   true,
		errMsg: "expected 1 argument(s) in call to \"floor\", got 2",
	}, {
		input:  "floor(1)",
		fail:   true,
		errMsg: "expected type instant vector in call to function \"floor\", got scalar",
	}, {
		input:  "non_existent_function_far_bar()",
		fail:   true,
		errMsg: "unknown function with name \"non_existent_function_far_bar\"",
	}, {
		input:  "rate(some_metric)",
		fail:   true,
		errMsg: "expected type range vector in call to function \"rate\", got instant vector",
	}, {
		input:  "label_replace(a, `b`, `c\xff`, `d`, `.*`)",
		fail:   true,
		errMsg: "parse error at char 23: invalid UTF-8 rune",
	},
	// Fuzzing regression tests.
	{
		input:  "-=",
		fail:   true,
		errMsg: `no valid expression found`,
	}, {
		input:  "++-++-+-+-<",
		fail:   true,
		errMsg: `no valid expression found`,
	}, {
		input:  "e-+=/(0)",
		fail:   true,
		errMsg: `no valid expression found`,
	},
	// String quoting and escape sequence interpretation tests.
	{
		input: `"double-quoted string \" with escaped quote"`,
		expected: &StringLiteral{
			Val: "double-quoted string \" with escaped quote",
		},
	}, {
		input: `'single-quoted string \' with escaped quote'`,
		expected: &StringLiteral{
			Val: "single-quoted string ' with escaped quote",
		},
	}, {
		input: "`backtick-quoted string`",
		expected: &StringLiteral{
			Val: "backtick-quoted string",
		},
	}, {
		input: `"\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺"`,
		expected: &StringLiteral{
			Val: "\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺",
		},
	}, {
		input: `'\a\b\f\n\r\t\v\\\' - \xFF\377\u1234\U00010111\U0001011111☺'`,
		expected: &StringLiteral{
			Val: "\a\b\f\n\r\t\v\\' - \xFF\377\u1234\U00010111\U0001011111☺",
		},
	}, {
		input: "`" + `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺` + "`",
		expected: &StringLiteral{
			Val: `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺`,
		},
	}, {
		input:  "`\\``",
		fail:   true,
		errMsg: "could not parse remaining input",
	}, {
		input:  `"\`,
		fail:   true,
		errMsg: "escape sequence not terminated",
	}, {
		input:  `"\c"`,
		fail:   true,
		errMsg: "unknown escape sequence U+0063 'c'",
	}, {
		input:  `"\x."`,
		fail:   true,
		errMsg: "illegal character U+002E '.' in escape sequence",
	},
	// Subquery.
	{
		input: `foo{bar="baz"}[10m:6s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, "bar", "baz"),
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			Range: 10 * time.Minute,
			Step:  6 * time.Second,
		},
	}, {
		input: `foo[10m:]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
				},
			},
			Range: 10 * time.Minute,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:5s])`,
		expected: &Call{
			Func: mustGetFunction("min_over_time"),
			Args: Expressions{
				&SubqueryExpr{
					Expr: &Call{
						Func: mustGetFunction("rate"),
						Args: Expressions{
							&MatrixSelector{
								Name:  "foo",
								Range: 2 * time.Second,
								LabelMatchers: []*labels.Matcher{
									mustLabelMatcher(labels.MatchEqual, "bar", "baz"),
									mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
								},
							},
						},
					},
					Range: 5 * time.Minute,
					Step:  5 * time.Second,
				},
			},
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: mustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: mustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									Name:  "foo",
									Range: 2 * time.Second,
									LabelMatchers: []*labels.Matcher{
										mustLabelMatcher(labels.MatchEqual, "bar", "baz"),
										mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
									},
								},
							},
						},
						Range: 5 * time.Minute,
					},
				},
			},
			Range: 4 * time.Minute,
			Step:  3 * time.Second,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 4m)[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: mustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: mustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									Name:  "foo",
									Range: 2 * time.Second,
									LabelMatchers: []*labels.Matcher{
										mustLabelMatcher(labels.MatchEqual, "bar", "baz"),
										mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
									},
								},
							},
						},
						Range:  5 * time.Minute,
						Offset: 4 * time.Minute,
					},
				},
			},
			Range: 4 * time.Minute,
			Step:  3 * time.Second,
		},
	}, {
		input: "sum without(and, by, avg, count, alert, annotations)(some_metric) [30m:10s]",
		expected: &SubqueryExpr{
			Expr: &AggregateExpr{
				Op:      ItemSum,
				Without: true,
				Expr: &VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
					},
				},
				Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
			},
			Range: 30 * time.Minute,
			Step:  10 * time.Second,
		},
	}, {
		input: `some_metric OFFSET 1m [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "some_metric"),
				},
				Offset: 1 * time.Minute,
			},
			Range: 10 * time.Minute,
			Step:  5 * time.Second,
		},
	}, {
		input: `(foo + bar{nm="val"})[5m:]`,
		expected: &SubqueryExpr{
			Expr: &ParenExpr{
				Expr: &BinaryExpr{
					Op: ItemADD,
					VectorMatching: &VectorMatching{
						Card: CardOneToOne,
					},
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, "nm", "val"),
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
						},
					},
				},
			},
			Range: 5 * time.Minute,
		},
	}, {
		input: `(foo + bar{nm="val"})[5m:] offset 10m`,
		expected: &SubqueryExpr{
			Expr: &ParenExpr{
				Expr: &BinaryExpr{
					Op: ItemADD,
					VectorMatching: &VectorMatching{
						Card: CardOneToOne,
					},
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "foo"),
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							mustLabelMatcher(labels.MatchEqual, "nm", "val"),
							mustLabelMatcher(labels.MatchEqual, string(model.MetricNameLabel), "bar"),
						},
					},
				},
			},
			Range:  5 * time.Minute,
			Offset: 10 * time.Minute,
		},
	}, {
		input:  "test[5d] OFFSET 10s [10m:5s]",
		fail:   true,
		errMsg: "parse error at char 29: subquery is only allowed on instant vector, got matrix in \"test[5d] offset 10s[10m:5s]\"",
	}, {
		input:  `(foo + bar{nm="val"})[5m:][10m:5s]`,
		fail:   true,
		errMsg: "parse error at char 27: could not parse remaining input \"[10m:5s]\"...",
	},
}

func TestParseExpressions(t *testing.T) {
	for _, test := range testExpr {
		expr, err := ParseExpr(test.input)

		// Unexpected errors are always caused by a bug.
		if err == errUnexpected {
			t.Fatalf("unexpected error occurred")
		}

		if !test.fail && err != nil {
			t.Errorf("error in input '%s'", test.input)
			t.Fatalf("could not parse: %s", err)
		}

		if test.fail && err != nil {
			if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("unexpected error on input '%s'", test.input)
				t.Fatalf("expected error to contain %q but got %q", test.errMsg, err)
			}
			continue
		}

		if !reflect.DeepEqual(expr, test.expected) {
			t.Errorf("error on input '%s'", test.input)
			t.Fatalf("no match\n\nexpected:\n%s\ngot: \n%s\n", Tree(test.expected), Tree(expr))
		}
	}
}

// NaN has no equality. Thus, we need a separate test for it.
func TestNaNExpression(t *testing.T) {
	expr, err := ParseExpr("NaN")
	if err != nil {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("could not parse: %s", err)
	}

	nl, ok := expr.(*NumberLiteral)
	if !ok {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("expected number literal but got %T", expr)
	}

	if !math.IsNaN(float64(nl.Val)) {
		t.Errorf("error on input 'NaN'")
		t.Fatalf("expected 'NaN' in number literal but got %v", nl.Val)
	}
}

func mustLabelMatcher(mt labels.MatchType, name, val string) *labels.Matcher {
	m, err := labels.NewMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}

func mustGetFunction(name string) *Function {
	f, ok := getFunction(name)
	if !ok {
		panic(errors.Errorf("function %q does not exist", name))
	}
	return f
}

var testSeries = []struct {
	input          string
	expectedMetric labels.Labels
	expectedValues []sequenceValue
	fail           bool
}{
	{
		input:          `{} 1 2 3`,
		expectedMetric: labels.Labels{},
		expectedValues: newSeq(1, 2, 3),
	}, {
		input:          `{a="b"} -1 2 3`,
		expectedMetric: labels.FromStrings("a", "b"),
		expectedValues: newSeq(-1, 2, 3),
	}, {
		input:          `my_metric 1 2 3`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric"),
		expectedValues: newSeq(1, 2, 3),
	}, {
		input:          `my_metric{} 1 2 3`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric"),
		expectedValues: newSeq(1, 2, 3),
	}, {
		input:          `my_metric{a="b"} 1 2 3`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 2, 3),
	}, {
		input:          `my_metric{a="b"} 1 2 3-10x4`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 2, 3, -7, -17, -27, -37),
	}, {
		input:          `my_metric{a="b"} 1 2 3-0x4`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 2, 3, 3, 3, 3, 3),
	}, {
		input:          `my_metric{a="b"} 1 3 _ 5 _x4`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 3, none, 5, none, none, none, none),
	}, {
		input: `my_metric{a="b"} 1 3 _ 5 _a4`,
		fail:  true,
	}, {
		input:          `my_metric{a="b"} 1 -1`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, -1),
	}, {
		input:          `my_metric{a="b"} 1 +1`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 1),
	}, {
		input:          `my_metric{a="b"} 1 -1 -3-10x4 7 9 +5`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, -1, -3, -13, -23, -33, -43, 7, 9, 5),
	}, {
		input:          `my_metric{a="b"} 1 +1 +4 -6 -2 8`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 1, 4, -6, -2, 8),
	}, {
		// Trailing spaces should be correctly handles.
		input:          `my_metric{a="b"} 1 2 3    `,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", "b"),
		expectedValues: newSeq(1, 2, 3),
	}, {
		input: `my_metric{a="b"} -3-3 -3`,
		fail:  true,
	}, {
		input: `my_metric{a="b"} -3 -3-3`,
		fail:  true,
	}, {
		input: `my_metric{a="b"} -3 _-2`,
		fail:  true,
	}, {
		input: `my_metric{a="b"} -3 3+3x4-4`,
		fail:  true,
	},
}

// For these tests only, we use the smallest float64 to signal an omitted value.
const none = math.SmallestNonzeroFloat64

func newSeq(vals ...float64) (res []sequenceValue) {
	for _, v := range vals {
		if v == none {
			res = append(res, sequenceValue{omitted: true})
		} else {
			res = append(res, sequenceValue{value: v})
		}
	}
	return res
}

func TestParseSeries(t *testing.T) {
	for _, test := range testSeries {
		metric, vals, err := parseSeriesDesc(test.input)

		// Unexpected errors are always caused by a bug.
		if err == errUnexpected {
			t.Fatalf("unexpected error occurred")
		}

		if test.fail {
			if err != nil {
				continue
			}
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("failure expected, but passed")
		} else {
			if err != nil {
				t.Errorf("error in input: \n\n%s\n", test.input)
				t.Fatalf("could not parse: %s", err)
			}
		}

		require.Equal(t, test.expectedMetric, metric)
		require.Equal(t, test.expectedValues, vals)

		if !reflect.DeepEqual(vals, test.expectedValues) || !reflect.DeepEqual(metric, test.expectedMetric) {
			t.Errorf("error in input: \n\n%s\n", test.input)
			t.Fatalf("no match\n\nexpected:\n%s %s\ngot: \n%s %s\n", test.expectedMetric, test.expectedValues, metric, vals)
		}
	}
}

func TestRecoverParserRuntime(t *testing.T) {
	p := newParser("foo bar")
	var err error

	defer func() {
		if err != errUnexpected {
			t.Fatalf("wrong error message: %q, expected %q", err, errUnexpected)
		}

		if _, ok := <-p.lex.items; ok {
			t.Fatalf("lex.items was not closed")
		}
	}()
	defer p.recover(&err)
	// Cause a runtime panic.
	var a []int
	a[123] = 1
}

func TestRecoverParserError(t *testing.T) {
	p := newParser("foo bar")
	var err error

	e := errors.New("custom error")

	defer func() {
		if err.Error() != e.Error() {
			t.Fatalf("wrong error message: %q, expected %q", err, e)
		}
	}()
	defer p.recover(&err)

	panic(e)
}
