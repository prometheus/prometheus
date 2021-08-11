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

package parser

import (
	"fmt"
	"math"
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
		input: "1",
		expected: &NumberLiteral{
			Val:      1,
			PosRange: PositionRange{Start: 0, End: 1},
		},
	}, {
		input: "+Inf",
		expected: &NumberLiteral{
			Val:      math.Inf(1),
			PosRange: PositionRange{Start: 0, End: 4},
		},
	}, {
		input: "-Inf",
		expected: &NumberLiteral{
			Val:      math.Inf(-1),
			PosRange: PositionRange{Start: 0, End: 4},
		},
	}, {
		input: ".5",
		expected: &NumberLiteral{
			Val:      0.5,
			PosRange: PositionRange{Start: 0, End: 2},
		},
	}, {
		input: "5.",
		expected: &NumberLiteral{
			Val:      5,
			PosRange: PositionRange{Start: 0, End: 2},
		},
	}, {
		input: "123.4567",
		expected: &NumberLiteral{
			Val:      123.4567,
			PosRange: PositionRange{Start: 0, End: 8},
		},
	}, {
		input: "5e-3",
		expected: &NumberLiteral{
			Val:      0.005,
			PosRange: PositionRange{Start: 0, End: 4},
		},
	}, {
		input: "5e3",
		expected: &NumberLiteral{
			Val:      5000,
			PosRange: PositionRange{Start: 0, End: 3},
		},
	}, {
		input: "0xc",
		expected: &NumberLiteral{
			Val:      12,
			PosRange: PositionRange{Start: 0, End: 3},
		},
	}, {
		input: "0755",
		expected: &NumberLiteral{
			Val:      493,
			PosRange: PositionRange{Start: 0, End: 4},
		},
	}, {
		input: "+5.5e-3",
		expected: &NumberLiteral{
			Val:      0.0055,
			PosRange: PositionRange{Start: 0, End: 7},
		},
	}, {
		input: "-0755",
		expected: &NumberLiteral{
			Val:      -493,
			PosRange: PositionRange{Start: 0, End: 5},
		},
	}, {
		input: "1 + 1",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 4, End: 5},
			},
		},
	}, {
		input: "1 - 1",
		expected: &BinaryExpr{
			Op: SUB,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 4, End: 5},
			},
		},
	}, {
		input: "1 * 1",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 4, End: 5},
			},
		},
	}, {
		input: "1 % 1",
		expected: &BinaryExpr{
			Op: MOD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 4, End: 5},
			},
		},
	}, {
		input: "1 / 1",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 4, End: 5},
			},
		},
	}, {
		input: "1 == bool 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	}, {
		input: "1 != bool 1",
		expected: &BinaryExpr{
			Op: NEQ,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	}, {
		input: "1 > bool 1",
		expected: &BinaryExpr{
			Op: GTR,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 9, End: 10},
			},
			ReturnBool: true,
		},
	}, {
		input: "1 >= bool 1",
		expected: &BinaryExpr{
			Op: GTE,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	}, {
		input: "1 < bool 1",
		expected: &BinaryExpr{
			Op: LSS,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 9, End: 10},
			},
			ReturnBool: true,
		},
	}, {
		input: "1 <= bool 1",
		expected: &BinaryExpr{
			Op: LTE,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	}, {
		input: "-1^2",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &BinaryExpr{
				Op: POW,
				LHS: &NumberLiteral{
					Val:      1,
					PosRange: PositionRange{Start: 1, End: 2},
				},
				RHS: &NumberLiteral{
					Val:      2,
					PosRange: PositionRange{Start: 3, End: 4},
				},
			},
		},
	}, {
		input: "-1*2",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &NumberLiteral{
				Val:      -1,
				PosRange: PositionRange{Start: 0, End: 2},
			},
			RHS: &NumberLiteral{
				Val:      2,
				PosRange: PositionRange{Start: 3, End: 4},
			},
		},
	}, {
		input: "-1+2",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      -1,
				PosRange: PositionRange{Start: 0, End: 2},
			},
			RHS: &NumberLiteral{
				Val:      2,
				PosRange: PositionRange{Start: 3, End: 4},
			},
		},
	}, {
		input: "-1^-2",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &BinaryExpr{
				Op: POW,
				LHS: &NumberLiteral{
					Val:      1,
					PosRange: PositionRange{Start: 1, End: 2},
				},
				RHS: &NumberLiteral{
					Val:      -2,
					PosRange: PositionRange{Start: 3, End: 5},
				},
			},
		},
	}, {
		input: "+1 + -2 * 1",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 2},
			},
			RHS: &BinaryExpr{
				Op: MUL,
				LHS: &NumberLiteral{
					Val:      -2,
					PosRange: PositionRange{Start: 5, End: 7},
				},
				RHS: &NumberLiteral{
					Val:      1,
					PosRange: PositionRange{Start: 10, End: 11},
				},
			},
		},
	}, {
		input: "1 + 2/(3*1)",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &BinaryExpr{
				Op: DIV,
				LHS: &NumberLiteral{
					Val:      2,
					PosRange: PositionRange{Start: 4, End: 5},
				},
				RHS: &ParenExpr{
					Expr: &BinaryExpr{
						Op: MUL,
						LHS: &NumberLiteral{
							Val:      3,
							PosRange: PositionRange{Start: 7, End: 8},
						},
						RHS: &NumberLiteral{
							Val:      1,
							PosRange: PositionRange{Start: 9, End: 10},
						},
					},
					PosRange: PositionRange{Start: 6, End: 11},
				},
			},
		},
	}, {
		input: "1 < bool 2 - 1 * 2",
		expected: &BinaryExpr{
			Op:         LSS,
			ReturnBool: true,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 0, End: 1},
			},
			RHS: &BinaryExpr{
				Op: SUB,
				LHS: &NumberLiteral{
					Val:      2,
					PosRange: PositionRange{Start: 9, End: 10},
				},
				RHS: &BinaryExpr{
					Op: MUL,
					LHS: &NumberLiteral{
						Val:      1,
						PosRange: PositionRange{Start: 13, End: 14},
					},
					RHS: &NumberLiteral{
						Val:      2,
						PosRange: PositionRange{Start: 17, End: 18},
					},
				},
			},
		},
	}, {
		input: "-some_metric",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 1,
					End:   12,
				},
			},
		},
	}, {
		input: "+some_metric",
		expected: &UnaryExpr{
			Op: ADD,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 1,
					End:   12,
				},
			},
		},
	}, {
		input: " +some_metric",
		expected: &UnaryExpr{
			Op: ADD,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 2,
					End:   13,
				},
			},
			StartPos: 1,
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
		errMsg: "unexpected end of input",
	}, {
		input:  ".",
		fail:   true,
		errMsg: "unexpected character: '.'",
	}, {
		input:  "2.5.",
		fail:   true,
		errMsg: "unexpected character: '.'",
	}, {
		input:  "100..4",
		fail:   true,
		errMsg: `unexpected number ".4"`,
	}, {
		input:  "0deadbeef",
		fail:   true,
		errMsg: "bad number or duration syntax: \"0de\"",
	}, {
		input:  "1 /",
		fail:   true,
		errMsg: "unexpected end of input",
	}, {
		input:  "*1",
		fail:   true,
		errMsg: "unexpected <op:*>",
	}, {
		input:  "(1))",
		fail:   true,
		errMsg: "unexpected right parenthesis ')'",
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
		errMsg: "1:3: parse error: comparisons between scalars must use BOOL modifier",
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
		errMsg: `unexpected character after '!': '~'`,
	}, {
		input:  "1 =~ 1",
		fail:   true,
		errMsg: `unexpected character after '=': '~'`,
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
		errMsg: "unexpected <op:*>",
	}, {
		input:  "1 offset 1d",
		fail:   true,
		errMsg: "1:1: parse error: offset modifier must be preceded by an instant vector selector or range vector selector or a subquery",
	}, {
		input:  "foo offset 1s offset 2s",
		fail:   true,
		errMsg: "offset may not be set multiple times",
	}, {
		input:  "a - on(b) ignoring(c) d",
		fail:   true,
		errMsg: "1:11: parse error: unexpected <ignoring>",
	},
	// Vector binary operations.
	{
		input: "foo * bar",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 6,
					End:   9,
				},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	}, {
		input: "foo * sum",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: PositionRange{
					Start: 6,
					End:   9,
				},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	}, {
		input: "foo == 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 7, End: 8},
			},
		},
	}, {
		input: "foo == bool 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: PositionRange{Start: 12, End: 13},
			},
			ReturnBool: true,
		},
	}, {
		input: "2.5 / bar",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &NumberLiteral{
				Val:      2.5,
				PosRange: PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 6,
					End:   9,
				},
			},
		},
	}, {
		input: "foo and bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 8,
					End:   11,
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		input: "foo or bar",
		expected: &BinaryExpr{
			Op: LOR,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 7,
					End:   10,
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		input: "foo unless bar",
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 11,
					End:   14,
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test and/or precedence and reassigning of operands.
		input: "foo + bar or bla and blub",
		expected: &BinaryExpr{
			Op: LOR,
			LHS: &BinaryExpr{
				Op: ADD,
				LHS: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
					},
					PosRange: PositionRange{
						Start: 0,
						End:   3,
					},
				},
				RHS: &VectorSelector{
					Name: "bar",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
					},
					PosRange: PositionRange{
						Start: 6,
						End:   9,
					},
				},
				VectorMatching: &VectorMatching{Card: CardOneToOne},
			},
			RHS: &BinaryExpr{
				Op: LAND,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bla"),
					},
					PosRange: PositionRange{
						Start: 13,
						End:   16,
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "blub"),
					},
					PosRange: PositionRange{
						Start: 21,
						End:   25,
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
			Op: LOR,
			LHS: &BinaryExpr{
				Op: LUNLESS,
				LHS: &BinaryExpr{
					Op: LAND,
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
						},
						PosRange: PositionRange{
							Start: 0,
							End:   3,
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: PositionRange{
							Start: 8,
							End:   11,
						},
					},
					VectorMatching: &VectorMatching{Card: CardManyToMany},
				},
				RHS: &VectorSelector{
					Name: "baz",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
					},
					PosRange: PositionRange{
						Start: 19,
						End:   22,
					},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			RHS: &VectorSelector{
				Name: "qux",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "qux"),
				},
				PosRange: PositionRange{
					Start: 26,
					End:   29,
				},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	}, {
		// Test precedence and reassigning of operands.
		input: "bar + on(foo) bla / on(baz, buz) group_right(test) blub",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &BinaryExpr{
				Op: DIV,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bla"),
					},
					PosRange: PositionRange{
						Start: 14,
						End:   17,
					},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "blub"),
					},
					PosRange: PositionRange{
						Start: 51,
						End:   55,
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
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 20,
					End:   23,
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
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 31,
					End:   34,
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
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 22,
					End:   25,
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
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 13,
					End:   16,
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
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 28,
					End:   31,
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
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 19,
					End:   22,
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
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "baz",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
				},
				PosRange: PositionRange{
					Start: 19,
					End:   22,
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
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 36,
					End:   39,
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
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 43,
					End:   46,
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
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 42,
					End:   45,
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
			Op: SUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 41,
					End:   44,
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
			Op: SUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 47,
					End:   50,
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
		input:  "foo + group_left(baz) bar",
		fail:   true,
		errMsg: "unexpected <group_left>",
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
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   3,
			},
		},
	}, {
		input: "min",
		expected: &VectorSelector{
			Name: "min",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "min"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   3,
			},
		},
	}, {
		input: "foo offset 5m",
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 5 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   13,
			},
		},
	}, {
		input: "foo offset -7m",
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: -7 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   14,
			},
		},
	}, {
		input: `foo OFFSET 1h30m`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 90 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   16,
			},
		},
	}, {
		input: `foo OFFSET 1m30ms`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: time.Minute + 30*time.Millisecond,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   17,
			},
		},
	}, {
		input: `foo @ 1603774568`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(1603774568000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   16,
			},
		},
	}, {
		input: `foo @ -100`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(-100000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   10,
			},
		},
	}, {
		input: `foo @ .3`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   8,
			},
		},
	}, {
		input: `foo @ 3.`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   8,
			},
		},
	}, {
		input: `foo @ 3.33`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3330),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   10,
			},
		},
	}, { // Rounding off.
		input: `foo @ 3.3333`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3333),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   12,
			},
		},
	}, { // Rounding off.
		input: `foo @ 3.3335`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3334),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   12,
			},
		},
	}, {
		input: `foo @ 3e2`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   9,
			},
		},
	}, {
		input: `foo @ 3e-1`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   10,
			},
		},
	}, {
		input: `foo @ 0xA`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(10000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   9,
			},
		},
	}, {
		input: `foo @ -3.3e1`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(-33000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   12,
			},
		},
	}, {
		input:  `foo @ +Inf`,
		fail:   true,
		errMsg: "1:1: parse error: timestamp out of bounds for @ modifier: +Inf",
	}, {
		input:  `foo @ -Inf`,
		fail:   true,
		errMsg: "1:1: parse error: timestamp out of bounds for @ modifier: -Inf",
	}, {
		input:  `foo @ NaN`,
		fail:   true,
		errMsg: "1:1: parse error: timestamp out of bounds for @ modifier: NaN",
	}, {
		input:  fmt.Sprintf(`foo @ %f`, float64(math.MaxInt64)+1),
		fail:   true,
		errMsg: fmt.Sprintf("1:1: parse error: timestamp out of bounds for @ modifier: %f", float64(math.MaxInt64)+1),
	}, {
		input:  fmt.Sprintf(`foo @ %f`, float64(math.MinInt64)-1),
		fail:   true,
		errMsg: fmt.Sprintf("1:1: parse error: timestamp out of bounds for @ modifier: %f", float64(math.MinInt64)-1),
	}, {
		input: `foo:bar{a="bc"}`,
		expected: &VectorSelector{
			Name: "foo:bar",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "a", "bc"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo:bar"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   15,
			},
		},
	}, {
		input: `foo{NaN='bc'}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "NaN", "bc"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   13,
			},
		},
	}, {
		input: `foo{bar='}'}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "bar", "}"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   12,
			},
		},
	}, {
		input: `foo{a="b", foo!="bar", test=~"test", bar!~"baz"}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "a", "b"),
				MustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
				MustLabelMatcher(labels.MatchRegexp, "test", "test"),
				MustLabelMatcher(labels.MatchNotRegexp, "bar", "baz"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   48,
			},
		},
	}, {
		input: `foo{a="b", foo!="bar", test=~"test", bar!~"baz",}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "a", "b"),
				MustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
				MustLabelMatcher(labels.MatchRegexp, "test", "test"),
				MustLabelMatcher(labels.MatchNotRegexp, "bar", "baz"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   49,
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
		errMsg: "unexpected character: '}'",
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
		errMsg: "1:15: parse error: invalid UTF-8 rune",
	}, {
		input:  `foo{gibberish}`,
		fail:   true,
		errMsg: `unexpected "}" in label matching, expected label matching operator`,
	}, {
		input:  `foo{1}`,
		fail:   true,
		errMsg: "unexpected character inside braces: '1'",
	}, {
		input:  `{}`,
		fail:   true,
		errMsg: "vector selector must contain at least one non-empty matcher",
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
		errMsg: `metric name must not be set twice: "foo" or "bar"`,
	}, {
		input:  `foo{__name__= =}`,
		fail:   true,
		errMsg: `1:15: parse error: unexpected "=" in label matching, expected string`,
	}, {
		input:  `foo{,}`,
		fail:   true,
		errMsg: `unexpected "," in label matching, expected identifier or "}"`,
	}, {
		input:  `foo{__name__ == "bar"}`,
		fail:   true,
		errMsg: `1:15: parse error: unexpected "=" in label matching, expected string`,
	}, {
		input:  `foo{__name__="bar" lol}`,
		fail:   true,
		errMsg: `unexpected identifier "lol" in label matching, expected "," or "}"`,
	},
	// Test matrix selector.
	{
		input: "test[5s]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * time.Second,
			EndPos: 8,
		},
	}, {
		input: "test[5m]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * time.Minute,
			EndPos: 8,
		},
	}, {
		input: `foo[5m30s]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			Range:  5*time.Minute + 30*time.Second,
			EndPos: 10,
		},
	}, {
		input: "test[5h] OFFSET 5m",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 5 * time.Minute,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * time.Hour,
			EndPos: 18,
		},
	}, {
		input: "test[5d] OFFSET 10s",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 10 * time.Second,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * 24 * time.Hour,
			EndPos: 19,
		},
	}, {
		input: "test[5w] offset 2w",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 14 * 24 * time.Hour,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * 7 * 24 * time.Hour,
			EndPos: 18,
		},
	}, {
		input: `test{a="b"}[5y] OFFSET 3d`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 3 * 24 * time.Hour,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "a", "b"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   11,
				},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 25,
		},
	}, {
		input: `test{a="b"}[5y] @ 1603774699`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "test",
				Timestamp: makeInt64Pointer(1603774699000),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "a", "b"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   11,
				},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 28,
		},
	}, {
		input:  `foo[5mm]`,
		fail:   true,
		errMsg: "bad duration syntax: \"5mm\"",
	}, {
		input:  `foo[5m1]`,
		fail:   true,
		errMsg: "bad duration syntax: \"5m1\"",
	}, {
		input:  `foo[5m:1m1]`,
		fail:   true,
		errMsg: "bad number or duration syntax: \"1m1\"",
	}, {
		input:  `foo[5y1hs]`,
		fail:   true,
		errMsg: "not a valid duration string: \"5y1hs\"",
	}, {
		input:  `foo[5m1h]`,
		fail:   true,
		errMsg: "not a valid duration string: \"5m1h\"",
	}, {
		input:  `foo[5m1m]`,
		fail:   true,
		errMsg: "not a valid duration string: \"5m1m\"",
	}, {
		input:  `foo[0m]`,
		fail:   true,
		errMsg: "duration must be greater than 0",
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
		errMsg: "1:22: parse error: no offset modifiers allowed before range",
	}, {
		input:  `some_metric[5m] @ 1m`,
		fail:   true,
		errMsg: "1:19: parse error: unexpected duration \"1m\" in @, expected timestamp",
	}, {
		input:  `some_metric[5m] @`,
		fail:   true,
		errMsg: "1:18: parse error: unexpected end of input in @, expected timestamp",
	}, {
		input:  `some_metric @ 1234 [5m]`,
		fail:   true,
		errMsg: "1:20: parse error: no @ modifiers allowed before range",
	},
	{
		input:  `(foo + bar)[5m]`,
		fail:   true,
		errMsg: "1:12: parse error: ranges only allowed for vector selectors",
	},
	// Test aggregation.
	{
		input: "sum by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 13,
					End:   24,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   25,
			},
		},
	}, {
		input: "avg by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: AVG,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 13,
					End:   24,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   25,
			},
		},
	}, {
		input: "max by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: MAX,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 13,
					End:   24,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   25,
			},
		},
	}, {
		input: "sum without (foo) (some_metric)",
		expected: &AggregateExpr{
			Op:      SUM,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 19,
					End:   30,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   31,
			},
		},
	}, {
		input: "sum (some_metric) without (foo)",
		expected: &AggregateExpr{
			Op:      SUM,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 5,
					End:   16,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   31,
			},
		},
	}, {
		input: "stddev(some_metric)",
		expected: &AggregateExpr{
			Op: STDDEV,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 7,
					End:   18,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   19,
			},
		},
	}, {
		input: "stdvar by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: STDVAR,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 16,
					End:   27,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   28,
			},
		},
	}, {
		input: "sum by ()(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 10,
					End:   21,
				},
			},
			Grouping: []string{},
			PosRange: PositionRange{
				Start: 0,
				End:   22,
			},
		},
	}, {
		input: "sum by (foo,bar,)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 18,
					End:   29,
				},
			},
			Grouping: []string{"foo", "bar"},
			PosRange: PositionRange{
				Start: 0,
				End:   30,
			},
		},
	}, {
		input: "sum by (foo,)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 14,
					End:   25,
				},
			},
			Grouping: []string{"foo"},
			PosRange: PositionRange{
				Start: 0,
				End:   26,
			},
		},
	}, {
		input: "topk(5, some_metric)",
		expected: &AggregateExpr{
			Op: TOPK,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 8,
					End:   19,
				},
			},
			Param: &NumberLiteral{
				Val: 5,
				PosRange: PositionRange{
					Start: 5,
					End:   6,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   20,
			},
		},
	}, {
		input: `count_values("value", some_metric)`,
		expected: &AggregateExpr{
			Op: COUNT_VALUES,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 22,
					End:   33,
				},
			},
			Param: &StringLiteral{
				Val: "value",
				PosRange: PositionRange{
					Start: 13,
					End:   20,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   34,
			},
		},
	}, {
		// Test usage of keywords as label names.
		input: "sum without(and, by, avg, count, alert, annotations)(some_metric)",
		expected: &AggregateExpr{
			Op:      SUM,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 53,
					End:   64,
				},
			},
			Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
			PosRange: PositionRange{
				Start: 0,
				End:   65,
			},
		},
	}, {
		input:  "sum without(==)(some_metric)",
		fail:   true,
		errMsg: "unexpected <op:==> in grouping opts, expected label",
	}, {
		input:  "sum without(,)(some_metric)",
		fail:   true,
		errMsg: `unexpected "," in grouping opts, expected label`,
	}, {
		input:  "sum without(foo,,)(some_metric)",
		fail:   true,
		errMsg: `unexpected "," in grouping opts, expected label`,
	}, {
		input:  `sum some_metric by (test)`,
		fail:   true,
		errMsg: "unexpected identifier \"some_metric\"",
	}, {
		input:  `sum (some_metric) by test`,
		fail:   true,
		errMsg: "unexpected identifier \"test\" in grouping opts",
	}, {
		input:  `sum (some_metric) by test`,
		fail:   true,
		errMsg: "unexpected identifier \"test\" in grouping opts",
	}, {
		input:  `sum () by (test)`,
		fail:   true,
		errMsg: "no arguments for aggregate expression provided",
	}, {
		input:  "MIN keep_common (some_metric)",
		fail:   true,
		errMsg: "1:5: parse error: unexpected identifier \"keep_common\"",
	}, {
		input:  "MIN (some_metric) keep_common",
		fail:   true,
		errMsg: `unexpected identifier "keep_common"`,
	}, {
		input:  `sum (some_metric) without (test) by (test)`,
		fail:   true,
		errMsg: "unexpected <by>",
	}, {
		input:  `sum without (test) (some_metric) by (test)`,
		fail:   true,
		errMsg: "unexpected <by>",
	}, {
		input:  `topk(some_metric)`,
		fail:   true,
		errMsg: "wrong number of arguments for aggregate expression provided, expected 2, got 1",
	}, {
		input:  `topk(some_metric,)`,
		fail:   true,
		errMsg: "trailing commas not allowed in function call args",
	}, {
		input:  `topk(some_metric, other_metric)`,
		fail:   true,
		errMsg: "1:6: parse error: expected type scalar in aggregation parameter, got instant vector",
	}, {
		input:  `count_values(5, other_metric)`,
		fail:   true,
		errMsg: "1:14: parse error: expected type string in aggregation parameter, got scalar",
	}, {
		input:  `rate(some_metric[5m]) @ 1234`,
		fail:   true,
		errMsg: "1:1: parse error: @ modifier must be preceded by an instant vector selector or range vector selector or a subquery",
	},
	// Test function calls.
	{
		input: "time()",
		expected: &Call{
			Func: MustGetFunction("time"),
			Args: Expressions{},
			PosRange: PositionRange{
				Start: 0,
				End:   6,
			},
		},
	}, {
		input: `floor(some_metric{foo!="bar"})`,
		expected: &Call{
			Func: MustGetFunction("floor"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: PositionRange{
						Start: 6,
						End:   29,
					},
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   30,
			},
		},
	}, {
		input: "rate(some_metric[5m])",
		expected: &Call{
			Func: MustGetFunction("rate"),
			Args: Expressions{
				&MatrixSelector{
					VectorSelector: &VectorSelector{
						Name: "some_metric",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
						},
						PosRange: PositionRange{
							Start: 5,
							End:   16,
						},
					},
					Range:  5 * time.Minute,
					EndPos: 20,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   21,
			},
		},
	}, {
		input: "round(some_metric)",
		expected: &Call{
			Func: MustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: PositionRange{
						Start: 6,
						End:   17,
					},
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   18,
			},
		},
	}, {
		input: "round(some_metric, 5)",
		expected: &Call{
			Func: MustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: PositionRange{
						Start: 6,
						End:   17,
					},
				},
				&NumberLiteral{
					Val: 5,
					PosRange: PositionRange{
						Start: 19,
						End:   20,
					},
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   21,
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
		input:  "floor(some_metric, 1)",
		fail:   true,
		errMsg: "expected 1 argument(s) in call to \"floor\", got 2",
	}, {
		input:  "floor(1)",
		fail:   true,
		errMsg: "expected type instant vector in call to function \"floor\", got scalar",
	}, {
		input:  "hour(some_metric, some_metric, some_metric)",
		fail:   true,
		errMsg: "expected at most 1 argument(s) in call to \"hour\", got 3",
	}, {
		input:  "time(some_metric)",
		fail:   true,
		errMsg: "expected 0 argument(s) in call to \"time\", got 1",
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
		errMsg: "1:23: parse error: invalid UTF-8 rune",
	},
	// Fuzzing regression tests.
	{
		input:  "-=",
		fail:   true,
		errMsg: `unexpected "="`,
	}, {
		input:  "++-++-+-+-<",
		fail:   true,
		errMsg: `unexpected <op:<>`,
	}, {
		input:  "e-+=/(0)",
		fail:   true,
		errMsg: `unexpected "="`,
	}, {
		input:  "a>b()",
		fail:   true,
		errMsg: `unknown function`,
	}, {
		input:  "rate(avg)",
		fail:   true,
		errMsg: `expected type range vector`,
	}, {
		// This is testing that we are not re-rendering the expression string for each error, which would timeout.
		input:  "(" + strings.Repeat("-{}-1", 10000) + ")" + strings.Repeat("[1m:]", 1000),
		fail:   true,
		errMsg: `1:3: parse error: vector selector must contain at least one non-empty matcher`,
	}, {
		input: "sum(sum)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: PositionRange{
					Start: 4,
					End:   7,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   8,
			},
		},
	}, {
		input: "a + sum",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &VectorSelector{
				Name: "a",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "a"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   1,
				},
			},
			RHS: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: PositionRange{
					Start: 4,
					End:   7,
				},
			},
			VectorMatching: &VectorMatching{},
		},
	},
	// String quoting and escape sequence interpretation tests.
	{
		input: `"double-quoted string \" with escaped quote"`,
		expected: &StringLiteral{
			Val:      "double-quoted string \" with escaped quote",
			PosRange: PositionRange{Start: 0, End: 44},
		},
	}, {
		input: `'single-quoted string \' with escaped quote'`,
		expected: &StringLiteral{
			Val:      "single-quoted string ' with escaped quote",
			PosRange: PositionRange{Start: 0, End: 44},
		},
	}, {
		input: "`backtick-quoted string`",
		expected: &StringLiteral{
			Val:      "backtick-quoted string",
			PosRange: PositionRange{Start: 0, End: 24},
		},
	}, {
		input: `"\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺"`,
		expected: &StringLiteral{
			Val:      "\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺",
			PosRange: PositionRange{Start: 0, End: 62},
		},
	}, {
		input: `'\a\b\f\n\r\t\v\\\' - \xFF\377\u1234\U00010111\U0001011111☺'`,
		expected: &StringLiteral{
			Val:      "\a\b\f\n\r\t\v\\' - \xFF\377\u1234\U00010111\U0001011111☺",
			PosRange: PositionRange{Start: 0, End: 62},
		},
	}, {
		input: "`" + `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺` + "`",
		expected: &StringLiteral{
			Val:      `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺`,
			PosRange: PositionRange{Start: 0, End: 64},
		},
	}, {
		input:  "`\\``",
		fail:   true,
		errMsg: "unterminated raw string",
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
					MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   14,
				},
			},
			Range:  10 * time.Minute,
			Step:   6 * time.Second,
			EndPos: 22,
		},
	},
	{
		input: `foo{bar="baz"}[10m5s:1h6ms]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   14,
				},
			},
			Range:  10*time.Minute + 5*time.Second,
			Step:   time.Hour + 6*time.Millisecond,
			EndPos: 27,
		},
	}, {
		input: `foo[10m:]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			Range:  10 * time.Minute,
			EndPos: 9,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:5s])`,
		expected: &Call{
			Func: MustGetFunction("min_over_time"),
			Args: Expressions{
				&SubqueryExpr{
					Expr: &Call{
						Func: MustGetFunction("rate"),
						Args: Expressions{
							&MatrixSelector{
								VectorSelector: &VectorSelector{
									Name: "foo",
									LabelMatchers: []*labels.Matcher{
										MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
										MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
									},
									PosRange: PositionRange{
										Start: 19,
										End:   33,
									},
								},
								Range:  2 * time.Second,
								EndPos: 37,
							},
						},
						PosRange: PositionRange{
							Start: 14,
							End:   38,
						},
					},
					Range: 5 * time.Minute,
					Step:  5 * time.Second,

					EndPos: 45,
				},
			},
			PosRange: PositionRange{
				Start: 0,
				End:   46,
			},
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: MustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: MustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									VectorSelector: &VectorSelector{
										Name: "foo",
										LabelMatchers: []*labels.Matcher{
											MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
											MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
										},
										PosRange: PositionRange{
											Start: 19,
											End:   33,
										},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: PositionRange{
								Start: 14,
								End:   38,
							},
						},
						Range:  5 * time.Minute,
						EndPos: 43,
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   44,
				},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 51,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 4m)[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: MustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: MustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									VectorSelector: &VectorSelector{
										Name: "foo",
										LabelMatchers: []*labels.Matcher{
											MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
											MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
										},
										PosRange: PositionRange{
											Start: 19,
											End:   33,
										},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: PositionRange{
								Start: 14,
								End:   38,
							},
						},
						Range:          5 * time.Minute,
						OriginalOffset: 4 * time.Minute,
						EndPos:         53,
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   54,
				},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 61,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] @ 1603775091)[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: MustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: MustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									VectorSelector: &VectorSelector{
										Name: "foo",
										LabelMatchers: []*labels.Matcher{
											MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
											MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
										},
										PosRange: PositionRange{
											Start: 19,
											End:   33,
										},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: PositionRange{
								Start: 14,
								End:   38,
							},
						},
						Range:     5 * time.Minute,
						Timestamp: makeInt64Pointer(1603775091000),
						EndPos:    56,
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   57,
				},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 64,
		},
	}, {
		input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] @ -160377509)[4m:3s]`,
		expected: &SubqueryExpr{
			Expr: &Call{
				Func: MustGetFunction("min_over_time"),
				Args: Expressions{
					&SubqueryExpr{
						Expr: &Call{
							Func: MustGetFunction("rate"),
							Args: Expressions{
								&MatrixSelector{
									VectorSelector: &VectorSelector{
										Name: "foo",
										LabelMatchers: []*labels.Matcher{
											MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
											MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
										},
										PosRange: PositionRange{
											Start: 19,
											End:   33,
										},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: PositionRange{
								Start: 14,
								End:   38,
							},
						},
						Range:     5 * time.Minute,
						Timestamp: makeInt64Pointer(-160377509000),
						EndPos:    56,
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   57,
				},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 64,
		},
	}, {
		input: "sum without(and, by, avg, count, alert, annotations)(some_metric) [30m:10s]",
		expected: &SubqueryExpr{
			Expr: &AggregateExpr{
				Op:      SUM,
				Without: true,
				Expr: &VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: PositionRange{
						Start: 53,
						End:   64,
					},
				},
				Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
				PosRange: PositionRange{
					Start: 0,
					End:   65,
				},
			},
			Range:  30 * time.Minute,
			Step:   10 * time.Second,
			EndPos: 75,
		},
	}, {
		input: `some_metric OFFSET 1m [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   21,
				},
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 30,
		},
	}, {
		input: `some_metric @ 123 [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   17,
				},
				Timestamp: makeInt64Pointer(123000),
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 26,
		},
	}, {
		input: `some_metric @ 123 offset 1m [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   27,
				},
				Timestamp:      makeInt64Pointer(123000),
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 36,
		},
	}, {
		input: `some_metric offset 1m @ 123 [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   27,
				},
				Timestamp:      makeInt64Pointer(123000),
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 36,
		},
	}, {
		input: `some_metric[10m:5s] offset 1m @ 123`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   11,
				},
			},
			Timestamp:      makeInt64Pointer(123000),
			OriginalOffset: 1 * time.Minute,
			Range:          10 * time.Minute,
			Step:           5 * time.Second,
			EndPos:         35,
		},
	}, {
		input: `(foo + bar{nm="val"})[5m:]`,
		expected: &SubqueryExpr{
			Expr: &ParenExpr{
				Expr: &BinaryExpr{
					Op: ADD,
					VectorMatching: &VectorMatching{
						Card: CardOneToOne,
					},
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
						},
						PosRange: PositionRange{
							Start: 1,
							End:   4,
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: PositionRange{
							Start: 7,
							End:   20,
						},
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   21,
				},
			},
			Range:  5 * time.Minute,
			EndPos: 26,
		},
	}, {
		input: `(foo + bar{nm="val"})[5m:] offset 10m`,
		expected: &SubqueryExpr{
			Expr: &ParenExpr{
				Expr: &BinaryExpr{
					Op: ADD,
					VectorMatching: &VectorMatching{
						Card: CardOneToOne,
					},
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
						},
						PosRange: PositionRange{
							Start: 1,
							End:   4,
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: PositionRange{
							Start: 7,
							End:   20,
						},
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   21,
				},
			},
			Range:          5 * time.Minute,
			OriginalOffset: 10 * time.Minute,
			EndPos:         37,
		},
	}, {
		input: `(foo + bar{nm="val"} @ 1234)[5m:] @ 1603775019`,
		expected: &SubqueryExpr{
			Expr: &ParenExpr{
				Expr: &BinaryExpr{
					Op: ADD,
					VectorMatching: &VectorMatching{
						Card: CardOneToOne,
					},
					LHS: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
						},
						PosRange: PositionRange{
							Start: 1,
							End:   4,
						},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						Timestamp: makeInt64Pointer(1234000),
						PosRange: PositionRange{
							Start: 7,
							End:   27,
						},
					},
				},
				PosRange: PositionRange{
					Start: 0,
					End:   28,
				},
			},
			Range:     5 * time.Minute,
			Timestamp: makeInt64Pointer(1603775019000),
			EndPos:    46,
		},
	}, {
		input:  "test[5d] OFFSET 10s [10m:5s]",
		fail:   true,
		errMsg: "1:1: parse error: subquery is only allowed on instant vector, got matrix",
	}, {
		input:  `(foo + bar{nm="val"})[5m:][10m:5s]`,
		fail:   true,
		errMsg: `1:1: parse error: subquery is only allowed on instant vector, got matrix`,
	}, {
		input:  "rate(food[1m])[1h] offset 1h",
		fail:   true,
		errMsg: `1:15: parse error: ranges only allowed for vector selectors`,
	}, {
		input:  "rate(food[1m])[1h] @ 100",
		fail:   true,
		errMsg: `1:15: parse error: ranges only allowed for vector selectors`,
	},
	// Preprocessors.
	{
		input: `foo @ start()`,
		expected: &VectorSelector{
			Name:       "foo",
			StartOrEnd: START,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   13,
			},
		},
	}, {
		input: `foo @ end()`,
		expected: &VectorSelector{
			Name:       "foo",
			StartOrEnd: END,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   11,
			},
		},
	}, {
		input: `test[5y] @ start()`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:       "test",
				StartOrEnd: START,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 18,
		},
	}, {
		input: `test[5y] @ end()`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:       "test",
				StartOrEnd: END,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   4,
				},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 16,
		},
	}, {
		input: `foo[10m:6s] @ start()`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			Range:      10 * time.Minute,
			Step:       6 * time.Second,
			StartOrEnd: START,
			EndPos:     21,
		},
	}, {
		input: `foo[10m:6s] @ end()`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			Range:      10 * time.Minute,
			Step:       6 * time.Second,
			StartOrEnd: END,
			EndPos:     19,
		},
	}, {
		input:  `start()`,
		fail:   true,
		errMsg: `1:6: parse error: unexpected "("`,
	}, {
		input:  `end()`,
		fail:   true,
		errMsg: `1:4: parse error: unexpected "("`,
	},
	// Check that start and end functions do not mask metrics.
	{
		input: `start`,
		expected: &VectorSelector{
			Name: "start",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "start"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   5,
			},
		},
	}, {
		input: `end`,
		expected: &VectorSelector{
			Name: "end",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "end"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   3,
			},
		},
	}, {
		input: `start{end="foo"}`,
		expected: &VectorSelector{
			Name: "start",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "end", "foo"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "start"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   16,
			},
		},
	}, {
		input: `end{start="foo"}`,
		expected: &VectorSelector{
			Name: "end",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "start", "foo"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "end"),
			},
			PosRange: PositionRange{
				Start: 0,
				End:   16,
			},
		},
	}, {
		input: `foo unless on(start) bar`,
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 21,
					End:   24,
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"start"},
				On:             true,
			},
		},
	}, {
		input: `foo unless on(end) bar`,
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: PositionRange{
					Start: 0,
					End:   3,
				},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: PositionRange{
					Start: 19,
					End:   22,
				},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"end"},
				On:             true,
			},
		},
	},
}

func makeInt64Pointer(val int64) *int64 {
	valp := new(int64)
	*valp = val
	return valp
}

func TestParseExpressions(t *testing.T) {
	for _, test := range testExpr {
		t.Run(test.input, func(t *testing.T) {
			expr, err := ParseExpr(test.input)

			// Unexpected errors are always caused by a bug.
			require.NotEqual(t, err, errUnexpected, "unexpected error occurred")

			if !test.fail {
				require.NoError(t, err)
				require.Equal(t, test.expected, expr, "error on input '%s'", test.input)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.errMsg, "unexpected error on input '%s', expected '%s', got '%s'", test.input, test.errMsg, err.Error())

				errorList, ok := err.(ParseErrors)

				require.True(t, ok, "unexpected error type")

				for _, e := range errorList {
					require.True(t, 0 <= e.PositionRange.Start, "parse error has negative position\nExpression '%s'\nError: %v", test.input, e)
					require.True(t, e.PositionRange.Start <= e.PositionRange.End, "parse error has negative length\nExpression '%s'\nError: %v", test.input, e)
					require.True(t, e.PositionRange.End <= Pos(len(test.input)), "parse error is not contained in input\nExpression '%s'\nError: %v", test.input, e)
				}
			}
		})
	}
}

// NaN has no equality. Thus, we need a separate test for it.
func TestNaNExpression(t *testing.T) {
	expr, err := ParseExpr("NaN")
	require.NoError(t, err)

	nl, ok := expr.(*NumberLiteral)
	require.True(t, ok, "expected number literal but got %T", expr)
	require.True(t, math.IsNaN(float64(nl.Val)), "expected 'NaN' in number literal but got %v", nl.Val)
}

var testSeries = []struct {
	input          string
	expectedMetric labels.Labels
	expectedValues []SequenceValue
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
		// Handle escaped unicode characters as whole label values.
		input:          `my_metric{a="\u70ac"} 1 2 3`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", `炬`),
		expectedValues: newSeq(1, 2, 3),
	}, {
		// Handle escaped unicode characters as partial label values.
		input:          `my_metric{a="\u70ac = torch"} 1 2 3`,
		expectedMetric: labels.FromStrings(labels.MetricName, "my_metric", "a", `炬 = torch`),
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

func newSeq(vals ...float64) (res []SequenceValue) {
	for _, v := range vals {
		if v == none {
			res = append(res, SequenceValue{Omitted: true})
		} else {
			res = append(res, SequenceValue{Value: v})
		}
	}
	return res
}

func TestParseSeries(t *testing.T) {
	for _, test := range testSeries {
		metric, vals, err := ParseSeriesDesc(test.input)

		// Unexpected errors are always caused by a bug.
		require.NotEqual(t, err, errUnexpected, "unexpected error occurred")

		if !test.fail {
			require.NoError(t, err)
			require.Equal(t, test.expectedMetric, metric, "error on input '%s'", test.input)
			require.Equal(t, test.expectedValues, vals, "error in input '%s'", test.input)
		} else {
			require.Error(t, err)
		}
	}
}

func TestRecoverParserRuntime(t *testing.T) {
	p := newParser("foo bar")
	var err error

	defer func() {
		require.Equal(t, errUnexpected, err)
	}()
	defer p.recover(&err)
	// Cause a runtime panic.
	var a []int
	//nolint:govet
	a[123] = 1
}

func TestRecoverParserError(t *testing.T) {
	p := newParser("foo bar")
	var err error

	e := errors.New("custom error")

	defer func() {
		require.Equal(t, e.Error(), err.Error())
	}()
	defer p.recover(&err)

	panic(e)
}

func TestExtractSelectors(t *testing.T) {
	for _, tc := range [...]struct {
		input    string
		expected []string
	}{
		{
			"foo",
			[]string{`{__name__="foo"}`},
		}, {
			`foo{bar="baz"}`,
			[]string{`{bar="baz", __name__="foo"}`},
		}, {
			`foo{bar="baz"} / flip{flop="flap"}`,
			[]string{`{bar="baz", __name__="foo"}`, `{flop="flap", __name__="flip"}`},
		}, {
			`rate(foo[5m])`,
			[]string{`{__name__="foo"}`},
		}, {
			`vector(1)`,
			[]string{},
		},
	} {
		expr, err := ParseExpr(tc.input)
		require.NoError(t, err)

		var expected [][]*labels.Matcher
		for _, s := range tc.expected {
			selector, err := ParseMetricSelector(s)
			require.NoError(t, err)
			expected = append(expected, selector)
		}

		require.Equal(t, expected, ExtractSelectors(expr))
	}
}
