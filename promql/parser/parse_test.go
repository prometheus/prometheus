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
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/testutil"
)

func repeatError(query string, err error, start, startStep, end, endStep, count int) (errs ParseErrors) {
	for i := range count {
		errs = append(errs, ParseErr{
			PositionRange: posrange.PositionRange{
				Start: posrange.Pos(start + (i * startStep)),
				End:   posrange.Pos(end + (i * endStep)),
			},
			Err:   err,
			Query: query,
		})
	}
	return errs
}

var testExpr = []struct {
	input    string      // The input to be parsed.
	expected Expr        // The expected expression AST.
	fail     bool        // Whether parsing is supposed to fail.
	errors   ParseErrors // The errors that should be returned.
}{
	// Scalars and scalar-to-scalar operations.
	{
		input: "1",
		expected: &NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 1},
		},
	},
	{
		input: "+Inf",
		expected: &NumberLiteral{
			Val:      math.Inf(1),
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "-Inf",
		expected: &NumberLiteral{
			Val:      math.Inf(-1),
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: ".5",
		expected: &NumberLiteral{
			Val:      0.5,
			PosRange: posrange.PositionRange{Start: 0, End: 2},
		},
	},
	{
		input: "5.",
		expected: &NumberLiteral{
			Val:      5,
			PosRange: posrange.PositionRange{Start: 0, End: 2},
		},
	},
	{
		input: "123.4567",
		expected: &NumberLiteral{
			Val:      123.4567,
			PosRange: posrange.PositionRange{Start: 0, End: 8},
		},
	},
	{
		input: "5e-3",
		expected: &NumberLiteral{
			Val:      0.005,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "5e3",
		expected: &NumberLiteral{
			Val:      5000,
			PosRange: posrange.PositionRange{Start: 0, End: 3},
		},
	},
	{
		input: "0xc",
		expected: &NumberLiteral{
			Val:      12,
			PosRange: posrange.PositionRange{Start: 0, End: 3},
		},
	},
	{
		input: "0755",
		expected: &NumberLiteral{
			Val:      493,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "+5.5e-3",
		expected: &NumberLiteral{
			Val:      0.0055,
			PosRange: posrange.PositionRange{Start: 0, End: 7},
		},
	},
	{
		input: "-0755",
		expected: &NumberLiteral{
			Val:      -493,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "1 + 1",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 4, End: 5},
			},
		},
	},
	{
		input: "1 - 1",
		expected: &BinaryExpr{
			Op: SUB,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 4, End: 5},
			},
		},
	},
	{
		input: "1 * 1",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 4, End: 5},
			},
		},
	},
	{
		input: "1 % 1",
		expected: &BinaryExpr{
			Op: MOD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 4, End: 5},
			},
		},
	},
	{
		input: "1 / 1",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 4, End: 5},
			},
		},
	},
	{
		input: "1 == bool 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	},
	{
		input: "1 != bool 1",
		expected: &BinaryExpr{
			Op: NEQ,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	},
	{
		input: "1 > bool 1",
		expected: &BinaryExpr{
			Op: GTR,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 9, End: 10},
			},
			ReturnBool: true,
		},
	},
	{
		input: "1 >= bool 1",
		expected: &BinaryExpr{
			Op: GTE,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	},
	{
		input: "1 < bool 1",
		expected: &BinaryExpr{
			Op: LSS,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 9, End: 10},
			},
			ReturnBool: true,
		},
	},
	{
		input: "1 <= bool 1",
		expected: &BinaryExpr{
			Op: LTE,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 10, End: 11},
			},
			ReturnBool: true,
		},
	},
	{
		input: "-1^2",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &BinaryExpr{
				Op: POW,
				LHS: &NumberLiteral{
					Val:      1,
					PosRange: posrange.PositionRange{Start: 1, End: 2},
				},
				RHS: &NumberLiteral{
					Val:      2,
					PosRange: posrange.PositionRange{Start: 3, End: 4},
				},
			},
		},
	},
	{
		input: "-1*2",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &NumberLiteral{
				Val:      -1,
				PosRange: posrange.PositionRange{Start: 0, End: 2},
			},
			RHS: &NumberLiteral{
				Val:      2,
				PosRange: posrange.PositionRange{Start: 3, End: 4},
			},
		},
	},
	{
		input: "-1+2",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      -1,
				PosRange: posrange.PositionRange{Start: 0, End: 2},
			},
			RHS: &NumberLiteral{
				Val:      2,
				PosRange: posrange.PositionRange{Start: 3, End: 4},
			},
		},
	},
	{
		input: "-1^-2",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &BinaryExpr{
				Op: POW,
				LHS: &NumberLiteral{
					Val:      1,
					PosRange: posrange.PositionRange{Start: 1, End: 2},
				},
				RHS: &NumberLiteral{
					Val:      -2,
					PosRange: posrange.PositionRange{Start: 3, End: 5},
				},
			},
		},
	},
	{
		input: "+1 + -2 * 1",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 2},
			},
			RHS: &BinaryExpr{
				Op: MUL,
				LHS: &NumberLiteral{
					Val:      -2,
					PosRange: posrange.PositionRange{Start: 5, End: 7},
				},
				RHS: &NumberLiteral{
					Val:      1,
					PosRange: posrange.PositionRange{Start: 10, End: 11},
				},
			},
		},
	},
	{
		input: "1 + 2/(3*1)",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &BinaryExpr{
				Op: DIV,
				LHS: &NumberLiteral{
					Val:      2,
					PosRange: posrange.PositionRange{Start: 4, End: 5},
				},
				RHS: &ParenExpr{
					Expr: &BinaryExpr{
						Op: MUL,
						LHS: &NumberLiteral{
							Val:      3,
							PosRange: posrange.PositionRange{Start: 7, End: 8},
						},
						RHS: &NumberLiteral{
							Val:      1,
							PosRange: posrange.PositionRange{Start: 9, End: 10},
						},
					},
					PosRange: posrange.PositionRange{Start: 6, End: 11},
				},
			},
		},
	},
	{
		input: "1 < bool 2 - 1 * 2",
		expected: &BinaryExpr{
			Op:         LSS,
			ReturnBool: true,
			LHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &BinaryExpr{
				Op: SUB,
				LHS: &NumberLiteral{
					Val:      2,
					PosRange: posrange.PositionRange{Start: 9, End: 10},
				},
				RHS: &BinaryExpr{
					Op: MUL,
					LHS: &NumberLiteral{
						Val:      1,
						PosRange: posrange.PositionRange{Start: 13, End: 14},
					},
					RHS: &NumberLiteral{
						Val:      2,
						PosRange: posrange.PositionRange{Start: 17, End: 18},
					},
				},
			},
		},
	},
	{
		input: "-some_metric",
		expected: &UnaryExpr{
			Op: SUB,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 1, End: 12},
			},
		},
	},
	{
		input: "+some_metric",
		expected: &UnaryExpr{
			Op: ADD,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 1, End: 12},
			},
		},
	},
	{
		input: " +some_metric",
		expected: &UnaryExpr{
			Op: ADD,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 2, End: 13},
			},
			StartPos: 1,
		},
	},
	{
		input: ` +{"some_metric"}`,
		expected: &UnaryExpr{
			Op: ADD,
			Expr: &VectorSelector{
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 2, End: 17},
			},
			StartPos: 1,
		},
	},
	{
		input: "",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0},
				Err:           errors.New("no expression found in input"),
				Query:         "",
			},
		},
	},
	{
		input: "# just a comment\n\n",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0},
				Err:           errors.New("no expression found in input"),
				Query:         "# just a comment\n\n",
			},
		},
	},
	{
		input: "1+",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 2, End: 2},
				Err:           errors.New("unexpected end of input"),
				Query:         "1+",
			},
		},
	},
	{
		input: ".",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("unexpected character: '.'"),
				Query:         ".",
			},
		},
	},
	{
		input: "2.5.",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 4},
				Err:           errors.New(`bad number or duration syntax: "2.5."`),
				Query:         "2.5.",
			},
		},
	},
	{
		input: "100..4",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 6},
				Err:           errors.New(`bad number or duration syntax: "100.."`),
				Query:         "100..4",
			},
		},
	},
	{
		input: "0deadbeef",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New("bad number or duration syntax: \"0de\""),
				Query:         "0deadbeef",
			},
		},
	},
	{
		input: "1 /",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 3, End: 3},
				Err:           errors.New("unexpected end of input"),
				Query:         "1 /",
			},
		},
	},
	{
		input: "*1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("unexpected <op:*>"),
				Query:         "*1",
			},
		},
	},
	{
		input: "(1))",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 4},
				Err:           errors.New("unexpected right parenthesis ')'"),
				Query:         "(1))",
			},
		},
	},
	{
		input: "((1)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 4},
				Err:           errors.New("unclosed left parenthesis"),
				Query:         "((1)",
			},
		},
	},
	{
		input: "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 309},
				Err:           errors.New(`error parsing number: strconv.ParseFloat: parsing "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999": value out of range`),
				Query:         "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999",
			},
		},
	},
	{
		input: "(",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 1, End: 1},
				Err:           errors.New("unclosed left parenthesis"),
				Query:         "(",
			},
		},
	},
	{
		input: "1 and 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 7},
				Err:           errors.New("set operator \"and\" not allowed in binary scalar expression"),
				Query:         "1 and 1",
			},
		},
	},
	{
		input: "1 == 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 2, End: 3},
				Err:           errors.New("comparisons between scalars must use BOOL modifier"),
				Query:         "1 == 1",
			},
		},
	},
	{
		input: "1 or 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 6},
				Err:           errors.New("set operator \"or\" not allowed in binary scalar expression"),
				Query:         "1 or 1",
			},
		},
	},
	{
		input: "1 unless 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 10},
				Err:           errors.New("set operator \"unless\" not allowed in binary scalar expression"),
				Query:         "1 unless 1",
			},
		},
	},
	{
		input: "1 !~ 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 2, End: 6},
				Err:           errors.New("unexpected character after '!': '~'"),
				Query:         "1 !~ 1",
			},
		},
	},
	{
		input: "1 =~ 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 2, End: 6},
				Err:           errors.New("unexpected character after '=': '~'"),
				Query:         "1 =~ 1",
			},
		},
	},
	{
		input: `-"string"`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New(`unary expression only allowed on expressions of type scalar or instant vector, got "string"`),
				Query:         `-"string"`,
			},
		},
	},
	{
		input: `-test[5m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New(`unary expression only allowed on expressions of type scalar or instant vector, got "range vector"`),
				Query:         `-test[5m]`,
			},
		},
	},
	{
		input: `*test`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("unexpected <op:*>"),
				Query:         `*test`,
			},
		},
	},
	{
		input: `@@`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New(`unexpected <op:@>`),
				Query:         `@@`,
			},
		},
	},
	{
		input: "1 offset 1d",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("offset modifier must be preceded by an instant vector selector or range vector selector or a subquery"),
				Query:         "1 offset 1d",
			},
		},
	},
	{
		input: "foo offset 1s offset 2s",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 13},
				Err:           errors.New("offset may not be set multiple times"),
				Query:         "foo offset 1s offset 2s",
			},
		},
	},
	{
		input: "a - on(b) ignoring(c) d",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 10, End: 18},
				Err:           errors.New("unexpected <ignoring>"),
				Query:         "a - on(b) ignoring(c) d",
			},
		},
	},
	// Vector selectors.
	{
		input: `offset{step="1s"}[5m]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "offset",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "step", "1s"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "offset"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 17},
			},
			Range:  5 * time.Minute,
			EndPos: 21,
		},
	},
	{
		input: `step{offset="1s"}[5m]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "step",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "offset", "1s"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "step"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 17},
			},
			Range:  5 * time.Minute,
			EndPos: 21,
		},
	},
	{
		input: `anchored{job="test"}`,
		expected: &VectorSelector{
			Name: "anchored",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "job", "test"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "anchored"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 20},
		},
	},
	{
		input: `smoothed{job="test"}`,
		expected: &VectorSelector{
			Name: "smoothed",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "job", "test"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "smoothed"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 20},
		},
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
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 6, End: 9},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	},
	{
		input: "foo * sum",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: posrange.PositionRange{Start: 6, End: 9},
			},
			VectorMatching: &VectorMatching{Card: CardOneToOne},
		},
	},
	{
		input: "foo == 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 7, End: 8},
			},
		},
	},
	{
		input: "foo == bool 1",
		expected: &BinaryExpr{
			Op: EQLC,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 12, End: 13},
			},
			ReturnBool: true,
		},
	},
	{
		input: "2.5 / bar",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &NumberLiteral{
				Val:      2.5,
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 6, End: 9},
			},
		},
	},
	{
		input: "foo and bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 8, End: 11},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	},
	{
		input: "foo or bar",
		expected: &BinaryExpr{
			Op: LOR,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 7, End: 10},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	},
	{
		input: "foo unless bar",
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 11, End: 14},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	},
	{
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
					PosRange: posrange.PositionRange{Start: 0, End: 3},
				},
				RHS: &VectorSelector{
					Name: "bar",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
					},
					PosRange: posrange.PositionRange{Start: 6, End: 9},
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
					PosRange: posrange.PositionRange{Start: 13, End: 16},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "blub"),
					},
					PosRange: posrange.PositionRange{Start: 21, End: 25},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	},
	{
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
						PosRange: posrange.PositionRange{Start: 0, End: 3},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: posrange.PositionRange{Start: 8, End: 11},
					},
					VectorMatching: &VectorMatching{Card: CardManyToMany},
				},
				RHS: &VectorSelector{
					Name: "baz",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
					},
					PosRange: posrange.PositionRange{Start: 19, End: 22},
				},
				VectorMatching: &VectorMatching{Card: CardManyToMany},
			},
			RHS: &VectorSelector{
				Name: "qux",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "qux"),
				},
				PosRange: posrange.PositionRange{Start: 26, End: 29},
			},
			VectorMatching: &VectorMatching{Card: CardManyToMany},
		},
	},
	{
		// Test precedence and reassigning of operands.
		input: "bar + on(foo) bla / on(baz, buz) group_right(test) blub",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &BinaryExpr{
				Op: DIV,
				LHS: &VectorSelector{
					Name: "bla",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bla"),
					},
					PosRange: posrange.PositionRange{Start: 14, End: 17},
				},
				RHS: &VectorSelector{
					Name: "blub",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "blub"),
					},
					PosRange: posrange.PositionRange{Start: 51, End: 55},
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
	},
	{
		input: "foo * on(test,blub) bar",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 20, End: 23},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	},
	{
		input: "foo * on(test,blub) group_left bar",
		expected: &BinaryExpr{
			Op: MUL,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 31, End: 34},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	},
	{
		input: "foo and on(test,blub) bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 22, End: 25},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
			},
		},
	},
	{
		input: "foo and on() bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 13, End: 16},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{},
				On:             true,
			},
		},
	},
	{
		input: "foo and ignoring(test,blub) bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 28, End: 31},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"test", "blub"},
			},
		},
	},
	{
		input: "foo and ignoring() bar",
		expected: &BinaryExpr{
			Op: LAND,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 19, End: 22},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{},
			},
		},
	},
	{
		input: "foo unless on(bar) baz",
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "baz",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
				},
				PosRange: posrange.PositionRange{Start: 19, End: 22},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"bar"},
				On:             true,
			},
		},
	},
	{
		input: "foo / on(test,blub) group_left(bar) bar",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 36, End: 39},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				On:             true,
				Include:        []string{"bar"},
			},
		},
	},
	{
		input: "foo / ignoring(test,blub) group_left(blub) bar",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 43, End: 46},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"blub"},
			},
		},
	},
	{
		input: "foo / ignoring(test,blub) group_left(bar) bar",
		expected: &BinaryExpr{
			Op: DIV,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 42, End: 45},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToOne,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar"},
			},
		},
	},
	{
		input: "foo - on(test,blub) group_right(bar,foo) bar",
		expected: &BinaryExpr{
			Op: SUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 41, End: 44},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToMany,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar", "foo"},
				On:             true,
			},
		},
	},
	{
		input: "foo - ignoring(test,blub) group_right(bar,foo) bar",
		expected: &BinaryExpr{
			Op: SUB,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 47, End: 50},
			},
			VectorMatching: &VectorMatching{
				Card:           CardOneToMany,
				MatchingLabels: []string{"test", "blub"},
				Include:        []string{"bar", "foo"},
			},
		},
	},
	{
		input: "foo and 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New("set operator \"and\" not allowed in binary scalar expression"),
				Query:         "foo and 1",
			},
		},
	},
	{
		input: "1 and foo",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New("set operator \"and\" not allowed in binary scalar expression"),
				Query:         "1 and foo",
			},
		},
	},
	{
		input: "foo or 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 8},
				Err:           errors.New("set operator \"or\" not allowed in binary scalar expression"),
				Query:         "foo or 1",
			},
		},
	},
	{
		input: "1 or foo",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 8},
				Err:           errors.New("set operator \"or\" not allowed in binary scalar expression"),
				Query:         "1 or foo",
			},
		},
	},
	{
		input: "foo unless 1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 12},
				Err:           errors.New("set operator \"unless\" not allowed in binary scalar expression"),
				Query:         "foo unless 1",
			},
		},
	},
	{
		input: "1 unless foo",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 12},
				Err:           errors.New("set operator \"unless\" not allowed in binary scalar expression"),
				Query:         "1 unless foo",
			},
		},
	},
	{
		input: "1 or on(bar) foo",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 16},
				Err:           errors.New("vector matching only allowed between instant vectors"),
				Query:         "1 or on(bar) foo",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 16},
				Err:           errors.New(`set operator "or" not allowed in binary scalar expression`),
				Query:         "1 or on(bar) foo",
			},
		},
	},
	{
		input: "foo == on(bar) 10",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 17},
				Err:           errors.New("vector matching only allowed between instant vectors"),
				Query:         "foo == on(bar) 10",
			},
		},
	},
	{
		input: "foo + group_left(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 6, End: 16},
				Err:           errors.New("unexpected <group_left>"),
				Query:         "foo + group_left(baz) bar",
			},
		},
	},
	{
		input: "foo and on(bar) group_left(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 35},
				Err:           errors.New("no grouping allowed for \"and\" operation"),
				Query:         "foo and on(bar) group_left(baz) bar",
			},
			{
				PositionRange: posrange.PositionRange{Start: 0, End: 35},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo and on(bar) group_left(baz) bar",
			},
		},
	},
	{
		input: "foo and on(bar) group_right(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 36},
				Err:           errors.New("no grouping allowed for \"and\" operation"),
				Query:         "foo and on(bar) group_right(baz) bar",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 36},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo and on(bar) group_right(baz) bar",
			},
		},
	},
	{
		input: "foo or on(bar) group_left(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 34},
				Err:           errors.New("no grouping allowed for \"or\" operation"),
				Query:         "foo or on(bar) group_left(baz) bar",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 34},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo or on(bar) group_left(baz) bar",
			},
		},
	},
	{
		input: "foo or on(bar) group_right(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 35},
				Err:           errors.New("no grouping allowed for \"or\" operation"),
				Query:         "foo or on(bar) group_right(baz) bar",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 35},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo or on(bar) group_right(baz) bar",
			},
		},
	},
	{
		input: "foo unless on(bar) group_left(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 38},
				Err:           errors.New("no grouping allowed for \"unless\" operation"),
				Query:         "foo unless on(bar) group_left(baz) bar",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 38},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo unless on(bar) group_left(baz) bar",
			},
		},
	},
	{
		input: "foo unless on(bar) group_right(baz) bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 39},
				Err:           errors.New("no grouping allowed for \"unless\" operation"),
				Query:         "foo unless on(bar) group_right(baz) bar",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 39},
				Err:           errors.New("set operations must always be many-to-many"),
				Query:         "foo unless on(bar) group_right(baz) bar",
			},
		},
	},
	{
		input: `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 34, End: 72},
				Err:           errors.New("label \"instance\" must not occur in ON and GROUP clause at once"),
				Query:         `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
			},
		},
	},
	{
		input: "foo + bool bar",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 9},
				Err:           errors.New("bool modifier can only be used on comparison operators"),
				Query:         "foo + bool bar",
			},
		},
	},
	{
		input: "foo + bool 10",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 9},
				Err:           errors.New("bool modifier can only be used on comparison operators"),
				Query:         "foo + bool 10",
			},
		},
	},
	{
		input: "foo and bool 10",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 11},
				Err:           errors.New("bool modifier can only be used on comparison operators"),
				Query:         "foo and bool 10",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{End: 15},
				Err:           errors.New(`set operator "and" not allowed in binary scalar expression`),
				Query:         "foo and bool 10",
			},
		},
	},
	// Test Vector selector.
	{
		input: "foo",
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 3},
		},
	},
	{
		input: "min",
		expected: &VectorSelector{
			Name: "min",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "min"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 3},
		},
	},
	{
		input: "foo offset 5m",
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 5 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 13},
		},
	},
	{
		input: "foo offset -7m",
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: -7 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 14},
		},
	},
	{
		input: `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 34, End: 72},
				Err:           errors.New("label \"instance\" must not occur in ON and GROUP clause at once"),
				Query:         `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
			},
		},
	},
	{
		input: `foo OFFSET 1h30m`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 90 * time.Minute,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 16},
		},
	},
	{
		input: `foo OFFSET 1m30ms`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: time.Minute + 30*time.Millisecond,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 17},
		},
	},
	{
		input: `foo @ 1603774568`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(1603774568000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 16},
		},
	},
	{
		input: `foo @ -100`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(-100000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{
		input: `foo @ .3`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 8},
		},
	},
	{
		input: `foo @ 3.`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 8},
		},
	},
	{
		input: `foo @ 3.33`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3330),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{ // Rounding off.
		input: `foo @ 3.3333`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3333),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 12},
		},
	},
	{ // Rounding off.
		input: `foo @ 3.3335`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(3334),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 12},
		},
	},
	{
		input: `foo @ 3e2`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 9},
		},
	},
	{
		input: `foo @ 3e-1`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(300),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{
		input: `foo @ 0xA`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(10000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 9},
		},
	},
	{
		input: `foo @ -3.3e1`,
		expected: &VectorSelector{
			Name:      "foo",
			Timestamp: makeInt64Pointer(-33000),
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 12},
		},
	},
	{
		input: `foo @ +Inf`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 3},
				Err:           errors.New("timestamp out of bounds for @ modifier: +Inf"),
				Query:         `foo @ +Inf`,
			},
		},
	},
	{
		input: `foo @ -Inf`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 3},
				Err:           errors.New("timestamp out of bounds for @ modifier: -Inf"),
				Query:         `foo @ -Inf`,
			},
		},
	},
	{
		input: `foo @ NaN`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 3},
				Err:           errors.New("timestamp out of bounds for @ modifier: NaN"),
				Query:         `foo @ NaN`,
			},
		},
	},
	{
		input: fmt.Sprintf(`foo @ %f`, float64(math.MaxInt64)+1),
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 3},
				Err:           fmt.Errorf("timestamp out of bounds for @ modifier: %f", float64(math.MaxInt64)+1),
				Query:         fmt.Sprintf(`foo @ %f`, float64(math.MaxInt64)+1),
			},
		},
	},
	{
		input: fmt.Sprintf(`foo @ %f`, float64(math.MinInt64)-1),
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 3},
				Err:           fmt.Errorf("timestamp out of bounds for @ modifier: %f", float64(math.MinInt64)-1),
				Query:         fmt.Sprintf(`foo @ %f`, float64(math.MinInt64)-1),
			},
		},
	},
	{
		input: `foo:bar{a="bc"}`,
		expected: &VectorSelector{
			Name: "foo:bar",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "a", "bc"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo:bar"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 15},
		},
	},
	{
		input: `{"foo"}`,
		expected: &VectorSelector{
			// When a metric is named inside the braces, the Name field is not set.
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 7},
		},
	},
	{
		input: `{'foo\'bar', 'a\\dos\\path'='boo\\urns'}`,
		expected: &VectorSelector{
			// When a metric is named inside the braces, the Name field is not set.
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, `foo'bar`),
				MustLabelMatcher(labels.MatchEqual, `a\dos\path`, `boo\urns`),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 40},
		},
	},
	{
		input: `{'foo\'bar', ` + "`" + `a\dos\path` + "`" + `="boo"}`,
		expected: &VectorSelector{
			// When a metric is named inside the braces, the Name field is not set.
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, `foo'bar`),
				MustLabelMatcher(labels.MatchEqual, `a\dos\path`, "boo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 32},
		},
	},
	{
		input: `{"foo", a="bc"}`,
		expected: &VectorSelector{
			// When a metric is named inside the braces, the Name field is not set.
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				MustLabelMatcher(labels.MatchEqual, "a", "bc"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 15},
		},
	},
	{
		input: `foo{NaN='bc'}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "NaN", "bc"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 13},
		},
	},
	{
		input: `foo{bar='}'}`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "bar", "}"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 12},
		},
	},
	{
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
			PosRange: posrange.PositionRange{Start: 0, End: 48},
		},
	},
	{
		// Metric name in the middle of selector list is fine.
		input: `{a="b", foo!="bar", "foo", test=~"test", bar!~"baz"}`,
		expected: &VectorSelector{
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "a", "b"),
				MustLabelMatcher(labels.MatchNotEqual, "foo", "bar"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				MustLabelMatcher(labels.MatchRegexp, "test", "test"),
				MustLabelMatcher(labels.MatchNotRegexp, "bar", "baz"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 52},
		},
	},
	{
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
			PosRange: posrange.PositionRange{Start: 0, End: 49},
		},
	},
	{
		// Specifying __name__ twice inside the braces is ok.
		input: `{__name__=~"bar", __name__!~"baz"}`,
		expected: &VectorSelector{
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchRegexp, model.MetricNameLabel, "bar"),
				MustLabelMatcher(labels.MatchNotRegexp, model.MetricNameLabel, "baz"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 34},
		},
	},
	{
		// Specifying __name__ with equality twice inside the braces is even allowed.
		input: `{__name__="bar", __name__="baz"}`,
		expected: &VectorSelector{
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 32},
		},
	},
	{
		// Because the above are allowed, this is also allowed.
		input: `{"bar", __name__="baz"}`,
		expected: &VectorSelector{
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "baz"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 23},
		},
	},
	{
		input: `{`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 1, End: 1},
				Err:           errors.New("unexpected end of input inside braces"),
				Query:         `{`,
			},
		},
	},
	{
		input: `}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("unexpected character: '}'"),
				Query:         `}`,
			},
		},
	},
	{
		input: `some{`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 5},
				Err:           errors.New("unexpected end of input inside braces"),
				Query:         `some{`,
			},
		},
	},
	{
		input: `some}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 5},
				Err:           errors.New("unexpected character: '}'"),
				Query:         `some}`,
			},
		},
	},
	{
		input: `some_metric{a=b}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 15},
				Err:           errors.New("unexpected identifier \"b\" in label matching, expected string"),
				Query:         `some_metric{a=b}`,
			},
		},
	},
	{
		input: `some_metric{a:b="b"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 13, End: 20},
				Err:           errors.New("unexpected character inside braces: ':'"),
				Query:         `some_metric{a:b="b"}`,
			},
		},
	},
	{
		input: `foo{a*"b"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 10},
				Err:           errors.New("unexpected character inside braces: '*'"),
				Query:         `foo{a*"b"}`,
			},
		},
	},
	{
		input: `foo{a>="b"}`,
		fail:  true,
		// TODO(fabxc): willingly lexing wrong tokens allows for more precise error
		// messages from the parser - consider if this is an option.
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 11},
				Err:           errors.New("unexpected character inside braces: '>'"),
				Query:         `foo{a>="b"}`,
			},
		},
	},
	{
		input: "some_metric{a=\"\xff\"}",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 18},
				Err:           errors.New("invalid UTF-8 rune"),
				Query:         "some_metric{a=\"\xff\"}",
			},
		},
	},
	{
		input: `foo{gibberish}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 13, End: 14},
				Err:           errors.New(`unexpected "}" in label matching, expected label matching operator`),
				Query:         `foo{gibberish}`,
			},
		},
	},
	{
		input: `foo{1}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 6},
				Err:           errors.New("unexpected character inside braces: '1'"),
				Query:         `foo{1}`,
			},
		},
	},
	{
		input: `{}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 2},
				Err:           errors.New("vector selector must contain at least one non-empty matcher"),
				Query:         `{}`,
			},
		},
	},
	{
		input: `{x=""}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 6},
				Err:           errors.New("vector selector must contain at least one non-empty matcher"),
				Query:         `{x=""}`,
			},
		},
	},
	{
		input: `{x=~".*"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New("vector selector must contain at least one non-empty matcher"),
				Query:         `{x=~".*"}`,
			},
		},
	},
	{
		input: `{x!~".+"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 9},
				Err:           errors.New("vector selector must contain at least one non-empty matcher"),
				Query:         `{x!~".+"}`,
			},
		},
	},
	{
		input: `{x!="a"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 8},
				Err:           errors.New("vector selector must contain at least one non-empty matcher"),
				Query:         `{x!="a"}`,
			},
		},
	},
	// Although {"bar", __name__="baz"} is allowed (see above), specifying a
	// metric name inside and outside the braces is not.
	{
		input: `foo{__name__="bar"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 19},
				Err:           errors.New(`metric name must not be set twice: "foo" or "bar"`),
				Query:         `foo{__name__="bar"}`,
			},
		},
	},
	{
		input: `foo{__name__= =}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 15},
				Err:           errors.New("unexpected \"=\" in label matching, expected string"),
				Query:         "foo{__name__= =}",
			},
		},
	},
	{
		input: `foo{,}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 5},
				Err:           errors.New("unexpected \",\" in label matching, expected identifier or \"}\""),
				Query:         "foo{,}",
			},
		},
	},
	{
		input: `foo{__name__ == "bar"}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 15},
				Err:           errors.New("unexpected \"=\" in label matching, expected string"),
				Query:         "foo{__name__ == \"bar\"}",
			},
		},
	},
	{
		input: `foo{__name__="bar" lol}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 19, End: 22},
				Err:           errors.New(`unexpected identifier "lol" in label matching, expected "," or "}"`),
				Query:         `foo{__name__="bar" lol}`,
			},
		},
	},
	{
		input: `foo{"a"=}`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 8, End: 9},
				Err:           errors.New(`unexpected "}" in label matching, expected string`),
				Query:         `foo{"a"=}`,
			},
		},
	},
	// Test matrix selector.
	{
		input: "test[1000ms]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  1000 * time.Millisecond,
			EndPos: 12,
		},
	},
	{
		input: "test[1001ms]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  1001 * time.Millisecond,
			EndPos: 12,
		},
	},
	{
		input: "test[1002ms]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  1002 * time.Millisecond,
			EndPos: 12,
		},
	},
	{
		input: "test[5s]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * time.Second,
			EndPos: 8,
		},
	},
	{
		input: "test[5m]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * time.Minute,
			EndPos: 8,
		},
	},
	{
		input: `foo[5m30s]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  5*time.Minute + 30*time.Second,
			EndPos: 10,
		},
	},
	{
		input: "test[5h] OFFSET 5m",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 5 * time.Minute,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * time.Hour,
			EndPos: 18,
		},
	},
	{
		input: "test[5d] OFFSET 10s",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 10 * time.Second,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * 24 * time.Hour,
			EndPos: 19,
		},
	},
	{
		input: "test[5w] offset 2w",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 14 * 24 * time.Hour,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * 7 * 24 * time.Hour,
			EndPos: 18,
		},
	},
	{
		input: `test{a="b"}[5y] OFFSET 3d`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 3 * 24 * time.Hour,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "a", "b"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 11},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 25,
		},
	},
	{
		input: `test{a="b"}[5m] OFFSET 3600`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:           "test",
				OriginalOffset: 1 * time.Hour,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "a", "b"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 11},
			},
			Range:  5 * time.Minute,
			EndPos: 27,
		},
	},
	{
		input: `foo[3ms] @ 2.345`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "foo",
				Timestamp: makeInt64Pointer(2345),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  3 * time.Millisecond,
			EndPos: 16,
		},
	},
	{
		input: `foo[4s180ms] @ 2.345`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "foo",
				Timestamp: makeInt64Pointer(2345),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  4*time.Second + 180*time.Millisecond,
			EndPos: 20,
		},
	},
	{
		input: `foo[4.18] @ 2.345`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "foo",
				Timestamp: makeInt64Pointer(2345),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  4*time.Second + 180*time.Millisecond,
			EndPos: 17,
		},
	},
	{
		input: `foo[4s18ms] @ 2.345`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "foo",
				Timestamp: makeInt64Pointer(2345),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  4*time.Second + 18*time.Millisecond,
			EndPos: 19,
		},
	},
	{
		input: `foo[4.018] @ 2.345`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "foo",
				Timestamp: makeInt64Pointer(2345),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  4*time.Second + 18*time.Millisecond,
			EndPos: 18,
		},
	},
	{
		input: `test{a="b"}[5y] @ 1603774699`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "test",
				Timestamp: makeInt64Pointer(1603774699000),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "a", "b"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 11},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 28,
		},
	},
	{
		input: "test[5]",
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "test",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * time.Second,
			EndPos: 7,
		},
	},
	{
		input: `some_metric[5m] @ 1m`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:      "some_metric",
				Timestamp: makeInt64Pointer(60000),
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 11},
			},
			Range:  5 * time.Minute,
			EndPos: 20,
		},
	},
	{
		input: `foo[5mm]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New("bad number or duration syntax: \"5mm\""),
				Query:         `foo[5mm]`,
			},
		},
	},
	{
		input: `foo[5m1]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New("bad number or duration syntax: \"5m1\""),
				Query:         `foo[5m1]`,
			},
		},
	},
	{
		input: `foo[5m:1m1]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 7, End: 11},
				Err:           errors.New("bad number or duration syntax: \"1m1\""),
				Query:         `foo[5m:1m1]`,
			},
		},
	},
	{
		input: `foo[5y1hs]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 9},
				Err:           errors.New("unknown unit \"hs\" in duration \"5y1hs\""),
				Query:         `foo[5y1hs]`,
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 9},
				Err:           errors.New("duration must be greater than 0"),
				Query:         "foo[5y1hs]",
			},
		},
	},
	{
		input: `foo[5m1h]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New("not a valid duration string: \"5m1h\""),
				Query:         `foo[5m1h]`,
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New("duration must be greater than 0"),
				Query:         "foo[5m1h]",
			},
		},
	},
	{
		input: `foo[5m1m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New("not a valid duration string: \"5m1m\""),
				Query:         `foo[5m1m]`,
			},
			{
				PositionRange: posrange.PositionRange{Start: 4, End: 8},
				Err:           errors.New(`duration must be greater than 0`),
				Query:         `foo[5m1m]`,
			},
		},
	},
	{
		input: `foo[0m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 6},
				Err:           errors.New("duration must be greater than 0"),
				Query:         `foo[0m]`,
			},
		},
	},
	{
		input: `foo["5m"]`,
		fail:  true,
		errors: ParseErrors{
			{
				PositionRange: posrange.PositionRange{Start: 4, End: 9},
				Err:           errors.New(`unexpected character in duration expression: '"'`),
				Query:         `foo["5m"]`,
			},
		},
	},
	{
		input: `foo[]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 5},
				Err:           errors.New("unexpected \"]\" in subquery or range selector, expected number, duration, step(), or range()"),
				Query:         `foo[]`,
			},
		},
	},
	{
		input: `foo[-1]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 6},
				Err:           errors.New("duration must be greater than 0"),
				Query:         `foo[-1]`,
			},
		},
	},
	{
		input: `some_metric[5m] OFFSET 1mm`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 23, End: 26},
				Err:           errors.New("bad number or duration syntax: \"1mm\""),
				Query:         `some_metric[5m] OFFSET 1mm`,
			},
		},
	},
	{
		input: `some_metric[5m] OFFSET`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 22, End: 22},
				Err:           errors.New("unexpected end of input in offset, expected number, duration, step(), or range()"),
				Query:         `some_metric[5m] OFFSET`,
			},
		},
	},
	{
		input: `some_metric OFFSET 1m[5m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 21, End: 25},
				Err:           errors.New("no offset modifiers allowed before range"),
				Query:         `some_metric OFFSET 1m[5m]`,
			},
		},
	},
	{
		input: `some_metric[5m] @`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 17, End: 17},
				Err:           errors.New("unexpected end of input in @, expected timestamp"),
				Query:         `some_metric[5m] @`,
			},
		},
	},
	{
		input: `some_metric @ 1234 [5m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 19, End: 23},
				Err:           errors.New("no @ modifiers allowed before range"),
				Query:         `some_metric @ 1234 [5m]`,
			},
		},
	},
	{
		input: `(foo + bar)[5m]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 11, End: 15},
				Err:           errors.New("ranges only allowed for vector selectors"),
				Query:         `(foo + bar)[5m]`,
			},
		},
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
				PosRange: posrange.PositionRange{Start: 13, End: 24},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 25},
		},
	},
	{
		input: "sum by (anchored)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 18, End: 29},
			},
			Grouping: []string{"anchored"},
			PosRange: posrange.PositionRange{Start: 0, End: 30},
		},
	},
	{
		input: "sum by (smoothed)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 18, End: 29},
			},
			Grouping: []string{"smoothed"},
			PosRange: posrange.PositionRange{Start: 0, End: 30},
		},
	},
	{
		input: `sum by ("foo bar")({"some.metric"})`,
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some.metric"),
				},
				PosRange: posrange.PositionRange{Start: 19, End: 34},
			},
			Grouping: []string{"foo bar"},
			PosRange: posrange.PositionRange{Start: 0, End: 35},
		},
	},
	{
		input: `sum by ("foo)(some_metric{})`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 8, End: 28},
				Err:           errors.New("unterminated quoted string"),
				Query:         `sum by ("foo)(some_metric{})`,
			},
		},
	},
	{
		input: `sum by ("foo", bar, 'baz')({"some.metric"})`,
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some.metric"),
				},
				PosRange: posrange.PositionRange{Start: 27, End: 42},
			},
			Grouping: []string{"foo", "bar", "baz"},
			PosRange: posrange.PositionRange{Start: 0, End: 43},
		},
	},
	{
		input: "avg by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: AVG,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 13, End: 24},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 25},
		},
	},
	{
		input: "max by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: MAX,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 13, End: 24},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 25},
		},
	},
	{
		input: "sum without (foo) (some_metric)",
		expected: &AggregateExpr{
			Op:      SUM,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 19, End: 30},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 31},
		},
	},
	{
		input: "sum (some_metric) without (foo)",
		expected: &AggregateExpr{
			Op:      SUM,
			Without: true,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 5, End: 16},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 31},
		},
	},
	{
		input: "stddev(some_metric)",
		expected: &AggregateExpr{
			Op: STDDEV,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 7, End: 18},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 19},
		},
	},
	{
		input: "stdvar by (foo)(some_metric)",
		expected: &AggregateExpr{
			Op: STDVAR,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 16, End: 27},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 28},
		},
	},
	{
		input: "sum by ()(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 10, End: 21},
			},
			Grouping: []string{},
			PosRange: posrange.PositionRange{Start: 0, End: 22},
		},
	},
	{
		input: "sum by (foo,bar,)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 18, End: 29},
			},
			Grouping: []string{"foo", "bar"},
			PosRange: posrange.PositionRange{Start: 0, End: 30},
		},
	},
	{
		input: "sum by (foo,)(some_metric)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 14, End: 25},
			},
			Grouping: []string{"foo"},
			PosRange: posrange.PositionRange{Start: 0, End: 26},
		},
	},
	{
		input: "topk(5, some_metric)",
		expected: &AggregateExpr{
			Op: TOPK,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 8, End: 19},
			},
			Param: &NumberLiteral{
				Val:      5,
				PosRange: posrange.PositionRange{Start: 5, End: 6},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 20},
		},
	},
	{
		input: `count_values("value", some_metric)`,
		expected: &AggregateExpr{
			Op: COUNT_VALUES,
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 22, End: 33},
			},
			Param: &StringLiteral{
				Val:      "value",
				PosRange: posrange.PositionRange{Start: 13, End: 20},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 34},
		},
	},
	{
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
				PosRange: posrange.PositionRange{Start: 53, End: 64},
			},
			Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
			PosRange: posrange.PositionRange{Start: 0, End: 65},
		},
	},
	{
		input: "sum without(==)(some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 12, End: 14},
				Err:           errors.New("unexpected <op:==> in grouping opts, expected label"),
				Query:         "sum without(==)(some_metric)",
			},
		},
	},
	{
		input: "sum without(,)(some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 12, End: 13},
				Err:           errors.New(`unexpected "," in grouping opts, expected label`),
				Query:         "sum without(,)(some_metric)",
			},
		},
	},
	{
		input: "sum without(foo,,)(some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 16, End: 17},
				Err:           errors.New(`unexpected "," in grouping opts, expected label`),
				Query:         "sum without(foo,,)(some_metric)",
			},
		},
	},
	{
		input: `sum some_metric by (test)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 15},
				Err:           errors.New(`unexpected identifier "some_metric"`),
				Query:         `sum some_metric by (test)`,
			},
		},
	},
	{
		input: `sum (some_metric) by test`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 21, End: 25},
				Err:           errors.New(`unexpected identifier "test" in grouping opts, expected "("`),
				Query:         `sum (some_metric) by test`,
			},
		},
	},
	{
		input: `sum () by (test)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 16},
				Err:           errors.New("no arguments for aggregate expression provided"),
				Query:         `sum () by (test)`,
			},
		},
	},
	{
		input: "MIN keep_common (some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 15},
				Err:           errors.New("unexpected identifier \"keep_common\""),
				Query:         "MIN keep_common (some_metric)",
			},
		},
	},
	{
		input: "MIN (some_metric) keep_common",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 18, End: 29},
				Err:           errors.New(`unexpected identifier "keep_common"`),
				Query:         "MIN (some_metric) keep_common",
			},
		},
	},
	{
		input: `sum (some_metric) without (test) by (test)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 33, End: 35},
				Err:           errors.New("unexpected <by>"),
				Query:         `sum (some_metric) without (test) by (test)`,
			},
		},
	},
	{
		input: `sum without (test) (some_metric) by (test)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 33, End: 35},
				Err:           errors.New("unexpected <by>"),
				Query:         `sum without (test) (some_metric) by (test)`,
			},
		},
	},
	{
		input: `topk(some_metric)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 17},
				Err:           errors.New("wrong number of arguments for aggregate expression provided, expected 2, got 1"),
				Query:         `topk(some_metric)`,
			},
		},
	},
	{
		input: `topk(some_metric,)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 16, End: 17},
				Err:           errors.New("trailing commas not allowed in function call args"),
				Query:         `topk(some_metric,)`,
			},
			{
				PositionRange: posrange.PositionRange{Start: 0, End: 18},
				Err:           errors.New("wrong number of arguments for aggregate expression provided, expected 2, got 1"),
				Query:         "topk(some_metric,)",
			},
		},
	},
	{
		input: `topk(some_metric, other_metric)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 16},
				Err:           errors.New("expected type scalar in aggregation parameter, got instant vector"),
				Query:         `topk(some_metric, other_metric)`,
			},
		},
	},
	{
		input: `count_values(5, other_metric)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 13, End: 14},
				Err:           errors.New("expected type string in aggregation parameter, got scalar"),
				Query:         `count_values(5, other_metric)`,
			},
		},
	},
	{
		input: `rate(some_metric[5m]) @ 1234`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 21},
				Err:           errors.New("@ modifier must be preceded by an instant vector selector or range vector selector or a subquery"),
				Query:         `rate(some_metric[5m]) @ 1234`,
			},
		},
	},
	// Test function calls.
	{
		input: "time()",
		expected: &Call{
			Func:     MustGetFunction("time"),
			Args:     Expressions{},
			PosRange: posrange.PositionRange{Start: 0, End: 6},
		},
	},
	{
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
					PosRange: posrange.PositionRange{Start: 6, End: 29},
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 30},
		},
	},
	{
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
						PosRange: posrange.PositionRange{Start: 5, End: 16},
					},
					Range:  5 * time.Minute,
					EndPos: 20,
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 21},
		},
	},
	{
		input: "round(some_metric)",
		expected: &Call{
			Func: MustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: posrange.PositionRange{Start: 6, End: 17},
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 18},
		},
	},
	{
		input: "round(some_metric, 5)",
		expected: &Call{
			Func: MustGetFunction("round"),
			Args: Expressions{
				&VectorSelector{
					Name: "some_metric",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
					},
					PosRange: posrange.PositionRange{Start: 6, End: 17},
				},
				&NumberLiteral{
					Val:      5,
					PosRange: posrange.PositionRange{Start: 19, End: 20},
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 21},
		},
	},
	{
		input: "floor()",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 7},
				Err:           errors.New("expected 1 argument(s) in call to \"floor\", got 0"),
				Query:         "floor()",
			},
		},
	},
	{
		input: "floor(some_metric, other_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 32},
				Err:           errors.New("expected 1 argument(s) in call to \"floor\", got 2"),
				Query:         "floor(some_metric, other_metric)",
			},
		},
	},
	{
		input: "floor(some_metric, 1)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 21},
				Err:           errors.New("expected 1 argument(s) in call to \"floor\", got 2"),
				Query:         "floor(some_metric, 1)",
			},
		},
	},
	{
		input: "floor(1)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 6, End: 7},
				Err:           errors.New("expected type instant vector in call to function \"floor\", got scalar"),
				Query:         "floor(1)",
			},
		},
	},
	{
		input: "hour(some_metric, some_metric, some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 43},
				Err:           errors.New("expected at most 1 argument(s) in call to \"hour\", got 3"),
				Query:         "hour(some_metric, some_metric, some_metric)",
			},
		},
	},
	{
		input: "time(some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 17},
				Err:           errors.New("expected 0 argument(s) in call to \"time\", got 1"),
				Query:         "time(some_metric)",
			},
		},
	},
	{
		input: "non_existent_function_far_bar()",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 29},
				Err:           errors.New("unknown function with name \"non_existent_function_far_bar\""),
				Query:         "non_existent_function_far_bar()",
			},
		},
	},
	{
		input: "rate(some_metric)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 16},
				Err:           errors.New("expected type range vector in call to function \"rate\", got instant vector"),
				Query:         "rate(some_metric)",
			},
		},
	},
	{
		input: "label_replace(a, `b`, `c\xff`, `d`, `.*`)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 22, End: 38},
				Err:           errors.New("invalid UTF-8 rune"),
				Query:         "label_replace(a, `b`, `c\xff`, `d`, `.*`)",
			},
			{
				PositionRange: posrange.PositionRange{Start: 20, End: 21},
				Err:           errors.New("trailing commas not allowed in function call args"),
				Query:         "label_replace(a, `b`, `c\xff`, `d`, `.*`)",
			},
		},
	},
	// Fuzzing regression tests.
	{
		input: "*1",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 1},
				Err:           errors.New("unexpected <op:*>"),
				Query:         "*1",
			},
		},
	},
	{
		input: "-=",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 1, End: 2},
				Err:           errors.New(`unexpected "="`),
				Query:         "-=",
			},
		},
	},
	{
		input: "++-++-+-+-<",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 10, End: 11},
				Err:           errors.New(`unexpected <op:<>`),
				Query:         "++-++-+-+-<",
			},
		},
	},
	{
		input: "e-+=/(0)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 3, End: 4},
				Err:           errors.New(`unexpected "="`),
				Query:         "e-+=/(0)",
			},
		},
	},
	{
		input: "a>b()",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 2, End: 3},
				Err:           errors.New(`unknown function with name "b"`),
				Query:         "a>b()",
			},
		},
	},
	{
		input: "rate(avg)",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 8},
				Err:           errors.New(`expected type range vector in call to function "rate", got instant vector`),
				Query:         "rate(avg)",
			},
		},
	},
	{
		// This is testing that we are not re-rendering the expression string for each error, which would timeout.
		input: "(" + strings.Repeat("-{}-1", 10000) + ")" + strings.Repeat("[1m:]", 1000),
		fail:  true,
		// This test generates a lot of errors, so we need a helper function to generate it for us.
		errors: append(
			repeatError(
				"("+strings.Repeat("-{}-1", 10000)+")"+strings.Repeat("[1m:]", 1000),
				errors.New("vector selector must contain at least one non-empty matcher"),
				2, 5, // begin with start=2, increment by 5 each time
				4, 5, // begin with end=2, increment by 5 each time
				10000, // number of errors to generate
			),
			repeatError(
				"("+strings.Repeat("-{}-1", 10000)+")"+strings.Repeat("[1m:]", 1000),
				errors.New("subquery is only allowed on instant vector, got matrix instead"),
				0, 0, // begin with start=0, don't increment, it's always start=0
				50012, 5, // begin with end=50012, increment by 5 each time
				999, // number of errors to generate
			)...,
		),
	},
	{
		input: "sum(sum)",
		expected: &AggregateExpr{
			Op: SUM,
			Expr: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: posrange.PositionRange{Start: 4, End: 7},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 8},
		},
	},
	{
		input: "a + sum",
		expected: &BinaryExpr{
			Op: ADD,
			LHS: &VectorSelector{
				Name: "a",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "a"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 1},
			},
			RHS: &VectorSelector{
				Name: "sum",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "sum"),
				},
				PosRange: posrange.PositionRange{Start: 4, End: 7},
			},
			VectorMatching: &VectorMatching{},
		},
	},
	// String quoting and escape sequence interpretation tests.
	{
		input: `"double-quoted string \" with escaped quote"`,
		expected: &StringLiteral{
			Val:      "double-quoted string \" with escaped quote",
			PosRange: posrange.PositionRange{Start: 0, End: 44},
		},
	},
	{
		input: `'single-quoted string \' with escaped quote'`,
		expected: &StringLiteral{
			Val:      "single-quoted string ' with escaped quote",
			PosRange: posrange.PositionRange{Start: 0, End: 44},
		},
	},
	{
		input: "`backtick-quoted string`",
		expected: &StringLiteral{
			Val:      "backtick-quoted string",
			PosRange: posrange.PositionRange{Start: 0, End: 24},
		},
	},
	{
		input: `"\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺"`,
		expected: &StringLiteral{
			Val:      "\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺",
			PosRange: posrange.PositionRange{Start: 0, End: 62},
		},
	},
	{
		input: `'\a\b\f\n\r\t\v\\\' - \xFF\377\u1234\U00010111\U0001011111☺'`,
		expected: &StringLiteral{
			Val:      "\a\b\f\n\r\t\v\\' - \xFF\377\u1234\U00010111\U0001011111☺",
			PosRange: posrange.PositionRange{Start: 0, End: 62},
		},
	},
	{
		input: "`" + `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺` + "`",
		expected: &StringLiteral{
			Val:      `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺`,
			PosRange: posrange.PositionRange{Start: 0, End: 64},
		},
	},
	{
		input: "`\\``",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 3, End: 4},
				Err:           errors.New("unterminated raw string"),
				Query:         "`\\``",
			},
		},
	},
	{
		input: `"\`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 2},
				Err:           errors.New("escape sequence not terminated"),
				Query:         `"\`,
			},
		},
	},
	{
		input: `"\c"`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 4},
				Err:           errors.New("unknown escape sequence U+0063 'c'"),
				Query:         `"\c"`,
			},
		},
	},
	{
		input: `"\x."`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 5},
				Err:           errors.New("illegal character U+002E '.' in escape sequence"),
				Query:         `"\x."`,
			},
		},
	},
	// Subquery.
	{
		input: `foo{bar="baz"}[`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 15, End: 15},
				Err:           errors.New(`unexpected end of input in duration expression`),
				Query:         `foo{bar="baz"}[`,
			},
		},
	},
	{
		input: `foo{bar="baz"}[10m:6s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 14},
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
				PosRange: posrange.PositionRange{Start: 0, End: 14},
			},
			Range:  10*time.Minute + 5*time.Second,
			Step:   time.Hour + 6*time.Millisecond,
			EndPos: 27,
		},
	},
	{
		input: `foo[10m:]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:  10 * time.Minute,
			EndPos: 9,
		},
	},
	{
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
									PosRange: posrange.PositionRange{Start: 19, End: 33},
								},
								Range:  2 * time.Second,
								EndPos: 37,
							},
						},
						PosRange: posrange.PositionRange{Start: 14, End: 38},
					},
					Range: 5 * time.Minute,
					Step:  5 * time.Second,

					EndPos: 45,
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 46},
		},
	},
	{
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
										PosRange: posrange.PositionRange{Start: 19, End: 33},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: posrange.PositionRange{Start: 14, End: 38},
						},
						Range:  5 * time.Minute,
						EndPos: 43,
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 44},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 51,
		},
	},
	{
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
										PosRange: posrange.PositionRange{Start: 19, End: 33},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: posrange.PositionRange{Start: 14, End: 38},
						},
						Range:          5 * time.Minute,
						OriginalOffset: 4 * time.Minute,
						EndPos:         53,
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 54},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 61,
		},
	},
	{
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
										PosRange: posrange.PositionRange{Start: 19, End: 33},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: posrange.PositionRange{Start: 14, End: 38},
						},
						Range:     5 * time.Minute,
						Timestamp: makeInt64Pointer(1603775091000),
						EndPos:    56,
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 57},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 64,
		},
	},
	{
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
										PosRange: posrange.PositionRange{Start: 19, End: 33},
									},
									Range:  2 * time.Second,
									EndPos: 37,
								},
							},
							PosRange: posrange.PositionRange{Start: 14, End: 38},
						},
						Range:     5 * time.Minute,
						Timestamp: makeInt64Pointer(-160377509000),
						EndPos:    56,
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 57},
			},
			Range:  4 * time.Minute,
			Step:   3 * time.Second,
			EndPos: 64,
		},
	},
	{
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
					PosRange: posrange.PositionRange{Start: 53, End: 64},
				},
				Grouping: []string{"and", "by", "avg", "count", "alert", "annotations"},
				PosRange: posrange.PositionRange{Start: 0, End: 65},
			},
			Range:  30 * time.Minute,
			Step:   10 * time.Second,
			EndPos: 75,
		},
	},
	{
		input: `some_metric OFFSET 1m [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange:       posrange.PositionRange{Start: 0, End: 21},
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 30,
		},
	},
	{
		input: `some_metric @ 123 [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange:  posrange.PositionRange{Start: 0, End: 17},
				Timestamp: makeInt64Pointer(123000),
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 26,
		},
	},
	{
		input: `some_metric @ 123 offset 1m [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange:       posrange.PositionRange{Start: 0, End: 27},
				Timestamp:      makeInt64Pointer(123000),
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 36,
		},
	},
	{
		input: `some_metric offset 1m @ 123 [10m:5s]`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange:       posrange.PositionRange{Start: 0, End: 27},
				Timestamp:      makeInt64Pointer(123000),
				OriginalOffset: 1 * time.Minute,
			},
			Range:  10 * time.Minute,
			Step:   5 * time.Second,
			EndPos: 36,
		},
	},
	{
		input: `some_metric[10m:5s] offset 1m @ 123`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "some_metric",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "some_metric"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 11},
			},
			Timestamp:      makeInt64Pointer(123000),
			OriginalOffset: 1 * time.Minute,
			Range:          10 * time.Minute,
			Step:           5 * time.Second,
			EndPos:         35,
		},
	},
	{
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
						PosRange: posrange.PositionRange{Start: 1, End: 4},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: posrange.PositionRange{Start: 7, End: 20},
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 21},
			},
			Range:  5 * time.Minute,
			EndPos: 26,
		},
	},
	{
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
						PosRange: posrange.PositionRange{Start: 1, End: 4},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						PosRange: posrange.PositionRange{Start: 7, End: 20},
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 21},
			},
			Range:          5 * time.Minute,
			OriginalOffset: 10 * time.Minute,
			EndPos:         37,
		},
	},
	{
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
						PosRange: posrange.PositionRange{Start: 1, End: 4},
					},
					RHS: &VectorSelector{
						Name: "bar",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, "nm", "val"),
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
						},
						Timestamp: makeInt64Pointer(1234000),
						PosRange:  posrange.PositionRange{Start: 7, End: 27},
					},
				},
				PosRange: posrange.PositionRange{Start: 0, End: 28},
			},
			Range:     5 * time.Minute,
			Timestamp: makeInt64Pointer(1603775019000),
			EndPos:    46,
		},
	},
	{
		input: "test[5d] OFFSET 10s [10m:5s]",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 28},
				Err:           errors.New("subquery is only allowed on instant vector, got matrix instead"),
				Query:         "test[5d] OFFSET 10s [10m:5s]",
			},
		},
	},
	{
		input: `(foo + bar{nm="val"})[5m:][10m:5s]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 34},
				Err:           errors.New(`subquery is only allowed on instant vector, got matrix instead`),
				Query:         `(foo + bar{nm="val"})[5m:][10m:5s]`,
			},
		},
	},
	{
		input: "rate(food[1m])[1h] offset 1h",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 18},
				Err:           errors.New(`ranges only allowed for vector selectors`),
				Query:         "rate(food[1m])[1h] offset 1h",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 18},
				Err:           errors.New(`ranges only allowed for vector selectors`),
				Query:         "rate(food[1m])[1h] offset 1h",
			},
		},
	},
	{
		input: "rate(food[1m])[1h] @ 100",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 18},
				Err:           errors.New(`ranges only allowed for vector selectors`),
				Query:         "rate(food[1m])[1h] @ 100",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 18},
				Err:           errors.New(`ranges only allowed for vector selectors`),
				Query:         "rate(food[1m])[1h] @ 100",
			},
		},
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
			PosRange: posrange.PositionRange{Start: 0, End: 13},
		},
	},
	{
		input: `foo @ end()`,
		expected: &VectorSelector{
			Name:       "foo",
			StartOrEnd: END,
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: `test[5y] @ start()`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:       "test",
				StartOrEnd: START,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 18,
		},
	},
	{
		input: `test[5y] @ end()`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name:       "test",
				StartOrEnd: END,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 4},
			},
			Range:  5 * 365 * 24 * time.Hour,
			EndPos: 16,
		},
	},
	{
		input: `foo[10m:6s] @ start()`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:      10 * time.Minute,
			Step:       6 * time.Second,
			StartOrEnd: START,
			EndPos:     21,
		},
	},
	{
		input: `foo[10m:6s] @ end()`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			Range:      10 * time.Minute,
			Step:       6 * time.Second,
			StartOrEnd: END,
			EndPos:     19,
		},
	},
	{
		input: `start()`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 5, End: 6},
				Err:           errors.New(`unexpected "("`),
				Query:         `start()`,
			},
		},
	},
	{
		input: `end()`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 3, End: 4},
				Err:           errors.New(`unexpected "("`),
				Query:         `end()`,
			},
		},
	},
	// Check that start and end functions do not mask metrics.
	{
		input: `start`,
		expected: &VectorSelector{
			Name: "start",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "start"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: `end`,
		expected: &VectorSelector{
			Name: "end",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "end"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 3},
		},
	},
	{
		input: `start{end="foo"}`,
		expected: &VectorSelector{
			Name: "start",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "end", "foo"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "start"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 16},
		},
	},
	{
		input: `end{start="foo"}`,
		expected: &VectorSelector{
			Name: "end",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, "start", "foo"),
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "end"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 16},
		},
	},
	{
		input: `foo unless on(start) bar`,
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 21, End: 24},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"start"},
				On:             true,
			},
		},
	},
	{
		input: `foo unless on(end) bar`,
		expected: &BinaryExpr{
			Op: LUNLESS,
			LHS: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RHS: &VectorSelector{
				Name: "bar",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "bar"),
				},
				PosRange: posrange.PositionRange{Start: 19, End: 22},
			},
			VectorMatching: &VectorMatching{
				Card:           CardManyToMany,
				MatchingLabels: []string{"end"},
				On:             true,
			},
		},
	},
	{
		input: `info(rate(http_request_counter_total{}[5m]))`,
		expected: &Call{
			Func: MustGetFunction("info"),
			Args: Expressions{
				&Call{
					Func:     MustGetFunction("rate"),
					PosRange: posrange.PositionRange{Start: 5, End: 43},
					Args: Expressions{
						&MatrixSelector{
							VectorSelector: &VectorSelector{
								Name:           "http_request_counter_total",
								OriginalOffset: 0,
								LabelMatchers: []*labels.Matcher{
									MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "http_request_counter_total"),
								},
								PosRange: posrange.PositionRange{Start: 10, End: 38},
							},
							EndPos: 42,
							Range:  5 * time.Minute,
						},
					},
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 44},
		},
	},
	{
		input: `info(rate(http_request_counter_total{}[5m]), target_info{foo="bar"})`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 45, End: 67},
				Err:           errors.New(`expected label selectors only, got vector selector instead`),
				Query:         `info(rate(http_request_counter_total{}[5m]), target_info{foo="bar"})`,
			},
		},
	},
	{
		input: `info(http_request_counter_total{namespace="zzz"}, {foo="bar", bar="baz"})`,
		expected: &Call{
			Func: MustGetFunction("info"),
			Args: Expressions{
				&VectorSelector{
					Name: "http_request_counter_total",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, "namespace", "zzz"),
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "http_request_counter_total"),
					},
					PosRange: posrange.PositionRange{Start: 5, End: 48},
				},
				&VectorSelector{
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, "foo", "bar"),
						MustLabelMatcher(labels.MatchEqual, "bar", "baz"),
					},
					PosRange:                posrange.PositionRange{Start: 50, End: 72},
					BypassEmptyMatcherCheck: true,
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 73},
		},
	},
	{
		input: `info(http_request_counter_total{namespace="zzz"}, {foo="bar"} == 1)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 50, End: 66},
				Err:           errors.New("expected label selectors only"),
				Query:         `info(http_request_counter_total{namespace="zzz"}, {foo="bar"} == 1)`,
			},
		},
	},
	// Test that nested parentheses result in the correct position range.
	{
		input: `foo[11s+10s-5*2^2]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: SUB,
				LHS: &DurationExpr{
					Op: ADD,
					LHS: &NumberLiteral{
						Val:      11,
						PosRange: posrange.PositionRange{Start: 4, End: 7},
						Duration: true,
					},
					RHS: &NumberLiteral{
						Val:      10,
						PosRange: posrange.PositionRange{Start: 8, End: 11},
						Duration: true,
					},
				},
				RHS: &DurationExpr{
					Op:  MUL,
					LHS: &NumberLiteral{Val: 5, PosRange: posrange.PositionRange{Start: 12, End: 13}},
					RHS: &DurationExpr{
						Op:  POW,
						LHS: &NumberLiteral{Val: 2, PosRange: posrange.PositionRange{Start: 14, End: 15}},
						RHS: &NumberLiteral{Val: 2, PosRange: posrange.PositionRange{Start: 16, End: 17}},
					},
				},
			},
			EndPos: 18,
		},
	},
	{
		input: `foo[-(10s-5s)+20s]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: ADD,
				LHS: &DurationExpr{
					Op:       SUB,
					StartPos: 4,
					RHS: &DurationExpr{
						Op: SUB,
						LHS: &NumberLiteral{
							Val:      10,
							PosRange: posrange.PositionRange{Start: 6, End: 9},
							Duration: true,
						},
						RHS: &NumberLiteral{
							Val:      5,
							PosRange: posrange.PositionRange{Start: 10, End: 12},
							Duration: true,
						},
						Wrapped: true,
					},
				},
				RHS: &NumberLiteral{
					Val:      20,
					PosRange: posrange.PositionRange{Start: 14, End: 17},
					Duration: true,
				},
			},
			EndPos: 18,
		},
	},
	{
		input: `foo[-10s+15s]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: ADD,
				LHS: &NumberLiteral{
					Val:      -10,
					PosRange: posrange.PositionRange{Start: 4, End: 8},
					Duration: true,
				},
				RHS: &NumberLiteral{
					Val:      15,
					PosRange: posrange.PositionRange{Start: 9, End: 12},
					Duration: true,
				},
			},
			EndPos: 13,
		},
	},
	{
		input: `foo[step()]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       STEP,
				StartPos: 4,
				EndPos:   10,
			},
			EndPos: 11,
		},
	},
	{
		input: `foo[  -  step  (  )  ]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       SUB,
				StartPos: 6,
				RHS: &DurationExpr{
					Op:       STEP,
					StartPos: 9,
					EndPos:   19,
				},
			},
			EndPos: 22,
		},
	},
	{
		input: `foo[   step  (  )  ]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       STEP,
				StartPos: 7,
				EndPos:   17,
			},
			EndPos: 20,
		},
	},
	{
		input: `foo[-step()]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       SUB,
				StartPos: 4,
				RHS:      &DurationExpr{Op: STEP, StartPos: 5, EndPos: 11},
			},
			EndPos: 12,
		},
	},
	{
		input: `foo offset step()`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 17},
			OriginalOffsetExpr: &DurationExpr{
				Op:       STEP,
				StartPos: 11,
				EndPos:   17,
			},
		},
	},
	{
		input: `foo offset -step()`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 18},
			OriginalOffsetExpr: &DurationExpr{
				Op:       SUB,
				StartPos: 11,
				RHS:      &DurationExpr{Op: STEP, StartPos: 12, EndPos: 18},
			},
		},
	},
	{
		input: `foo[max(step(),5s)]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: MAX,
				LHS: &DurationExpr{
					Op:       STEP,
					StartPos: 8,
					EndPos:   14,
				},
				RHS: &NumberLiteral{
					Val:      5,
					Duration: true,
					PosRange: posrange.PositionRange{Start: 15, End: 17},
				},
				StartPos: 4,
				EndPos:   18,
			},
			EndPos: 19,
		},
	},
	{
		input: `foo offset max(step(),5s)`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 25},
			OriginalOffsetExpr: &DurationExpr{
				Op: MAX,
				LHS: &DurationExpr{
					Op:       STEP,
					StartPos: 15,
					EndPos:   21,
				},
				RHS: &NumberLiteral{
					Val:      5,
					Duration: true,
					PosRange: posrange.PositionRange{Start: 22, End: 24},
				},
				StartPos: 11,
				EndPos:   25,
			},
		},
	},
	{
		input: `foo offset -min(5s,step()+8s)`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 29},
			OriginalOffsetExpr: &DurationExpr{
				Op: SUB,
				RHS: &DurationExpr{
					Op: MIN,
					LHS: &NumberLiteral{
						Val:      5,
						Duration: true,
						PosRange: posrange.PositionRange{Start: 16, End: 18},
					},
					RHS: &DurationExpr{
						Op: ADD,
						LHS: &DurationExpr{
							Op:       STEP,
							StartPos: 19,
							EndPos:   25,
						},
						RHS: &NumberLiteral{
							Val:      8,
							Duration: true,
							PosRange: posrange.PositionRange{Start: 26, End: 28},
						},
					},
					StartPos: 12,
					EndPos:   28,
				},
				StartPos: 11,
				EndPos:   28,
			},
		},
	},
	{
		input: `foo[range()]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       RANGE,
				StartPos: 4,
				EndPos:   11,
			},
			EndPos: 12,
		},
	},
	{
		input: `foo[-range()]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:       SUB,
				StartPos: 4,
				RHS:      &DurationExpr{Op: RANGE, StartPos: 5, EndPos: 12},
			},
			EndPos: 13,
		},
	},
	{
		input: `foo offset range()`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 18},
			OriginalOffsetExpr: &DurationExpr{
				Op:       RANGE,
				StartPos: 11,
				EndPos:   18,
			},
		},
	},
	{
		input: `foo offset -range()`,
		expected: &VectorSelector{
			Name: "foo",
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 19},
			OriginalOffsetExpr: &DurationExpr{
				Op:       SUB,
				RHS:      &DurationExpr{Op: RANGE, StartPos: 12, EndPos: 19},
				StartPos: 11,
			},
		},
	},
	{
		input: `foo[max(range(),5s)]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: MAX,
				LHS: &DurationExpr{
					Op:       RANGE,
					StartPos: 8,
					EndPos:   15,
				},
				RHS: &NumberLiteral{
					Val:      5,
					Duration: true,
					PosRange: posrange.PositionRange{Start: 16, End: 18},
				},
				StartPos: 4,
				EndPos:   19,
			},
			EndPos: 20,
		},
	},
	{
		input: `foo[4s+4s:1s*2] offset (5s-8)`,
		expected: &SubqueryExpr{
			Expr: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: ADD,
				LHS: &NumberLiteral{
					Val:      4,
					PosRange: posrange.PositionRange{Start: 4, End: 6},
					Duration: true,
				},
				RHS: &NumberLiteral{
					Val:      4,
					PosRange: posrange.PositionRange{Start: 7, End: 9},
					Duration: true,
				},
			},
			StepExpr: &DurationExpr{
				Op: MUL,
				LHS: &NumberLiteral{
					Val:      1,
					PosRange: posrange.PositionRange{Start: 10, End: 12},
					Duration: true,
				},
				RHS: &NumberLiteral{
					Val:      2,
					PosRange: posrange.PositionRange{Start: 13, End: 14},
				},
			},
			OriginalOffsetExpr: &DurationExpr{
				Op: SUB,
				LHS: &NumberLiteral{
					Val:      5,
					PosRange: posrange.PositionRange{Start: 24, End: 26},
					Duration: true,
				},
				RHS: &NumberLiteral{
					Val:      8,
					PosRange: posrange.PositionRange{Start: 27, End: 28},
				},
				Wrapped: true,
			},
			EndPos: 29,
		},
	},
	{
		input: `foo offset 5s-8`,
		expected: &BinaryExpr{
			Op: SUB,
			LHS: &VectorSelector{
				Name:           "foo",
				OriginalOffset: 5 * time.Second,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 13},
			},
			RHS: &NumberLiteral{
				Val:      8,
				PosRange: posrange.PositionRange{Start: 14, End: 15},
			},
		},
	},
	{
		input: `rate(foo[2m+2m])`,
		expected: &Call{
			Func: MustGetFunction("rate"),
			Args: Expressions{
				&MatrixSelector{
					VectorSelector: &VectorSelector{
						Name: "foo",
						LabelMatchers: []*labels.Matcher{
							MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
						},
						PosRange: posrange.PositionRange{Start: 5, End: 8},
					},
					RangeExpr: &DurationExpr{
						Op: ADD,
						LHS: &NumberLiteral{
							Val:      120,
							PosRange: posrange.PositionRange{Start: 9, End: 11},
							Duration: true,
						},
						RHS: &NumberLiteral{
							Val:      120,
							PosRange: posrange.PositionRange{Start: 12, End: 14},
							Duration: true,
						},
					},
					EndPos: 15,
				},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 16},
		},
	},
	{
		input: `foo offset -1^1`,
		expected: &BinaryExpr{
			Op: POW,
			LHS: &VectorSelector{
				Name:           "foo",
				OriginalOffset: -time.Second,
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 13},
			},
			RHS: &NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 14, End: 15},
			},
		},
	},
	{
		input: `foo offset -(1^2)`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 0,
			OriginalOffsetExpr: &DurationExpr{
				Op: SUB,
				RHS: &DurationExpr{
					Op: POW,
					LHS: &NumberLiteral{
						Val:      1,
						PosRange: posrange.PositionRange{Start: 13, End: 14},
					},
					RHS: &NumberLiteral{
						Val:      2,
						PosRange: posrange.PositionRange{Start: 15, End: 16},
					},
					Wrapped: true,
				},
				StartPos: 11,
			},
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 17},
		},
	},
	{
		input: `foo offset (-1^2)`,
		expected: &VectorSelector{
			Name:           "foo",
			OriginalOffset: 0,
			OriginalOffsetExpr: &DurationExpr{
				Op: SUB,
				RHS: &DurationExpr{
					Op: POW,
					LHS: &NumberLiteral{
						Val:      1,
						PosRange: posrange.PositionRange{Start: 13, End: 14},
					},
					RHS: &NumberLiteral{
						Val:      2,
						PosRange: posrange.PositionRange{Start: 15, End: 16},
					},
				},
				Wrapped:  true,
				StartPos: 12,
			},
			LabelMatchers: []*labels.Matcher{
				MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			},
			PosRange: posrange.PositionRange{Start: 0, End: 17},
		},
	},
	{
		input: `foo[-2^2]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op:  SUB,
				LHS: nil,
				RHS: &DurationExpr{
					Op: POW,
					LHS: &NumberLiteral{
						Val:      2,
						PosRange: posrange.PositionRange{Start: 5, End: 6},
					},
					RHS: &NumberLiteral{
						Val:      2,
						PosRange: posrange.PositionRange{Start: 7, End: 8},
					},
				},
				StartPos: 4,
			},
			EndPos: 9,
		},
	},
	{
		input: `foo[0+-2^2]`,
		expected: &MatrixSelector{
			VectorSelector: &VectorSelector{
				Name: "foo",
				LabelMatchers: []*labels.Matcher{
					MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				},
				PosRange: posrange.PositionRange{Start: 0, End: 3},
			},
			RangeExpr: &DurationExpr{
				Op: ADD,
				LHS: &NumberLiteral{
					Val:      0,
					PosRange: posrange.PositionRange{Start: 4, End: 5},
				},
				RHS: &DurationExpr{
					Op:  SUB,
					LHS: nil,
					RHS: &DurationExpr{
						Op: POW,
						LHS: &NumberLiteral{
							Val:      2,
							PosRange: posrange.PositionRange{Start: 7, End: 8},
						},
						RHS: &NumberLiteral{
							Val:      2,
							PosRange: posrange.PositionRange{Start: 9, End: 10},
						},
					},
					StartPos: 6,
				},
			},
			EndPos: 11,
		},
	},
	{
		input: `foo[step]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 8, End: 9},
				Err:           errors.New(`unexpected "]" in subquery or range selector, expected number, duration, step(), or range()`),
				Query:         `foo[step]`,
			},
		},
	},
	{
		input: `foo[step()/0d]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 10, End: 11},
				Err:           errors.New(`division by zero`),
				Query:         `foo[step()/0d]`,
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0}, // FIXME: this position looks wrong.
				Err:           errors.New(`duration must be greater than 0`),
				Query:         `foo[step()/0d]`,
			},
		},
	},
	{
		input: `foo[5s/0d]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 6, End: 7},
				Err:           errors.New(`division by zero`),
				Query:         `foo[5s/0d]`,
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0}, // FIXME: this position looks wrong.
				Err:           errors.New(`duration must be greater than 0`),
				Query:         `foo[5s/0d]`,
			},
		},
	},
	{
		input: `foo offset (4d/0)`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 14, End: 15},
				Err:           errors.New(`division by zero`),
				Query:         `foo offset (4d/0)`,
			},
		},
	},
	{
		input: `foo[5s%0d]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 6, End: 7},
				Err:           errors.New(`modulo by zero`),
				Query:         `foo[5s%0d]`,
			},
			ParseErr{
				Err:   errors.New(`duration must be greater than 0`),
				Query: `foo[5s%0d]`,
			},
		},
	},
	{
		input: `foo offset 9.5e10`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 11, End: 17},
				Err:           errors.New(`duration out of range`),
				Query:         `foo offset 9.5e10`,
			},
		},
	},
	{
		input: `foo[9.5e10]`,
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 10},
				Err:           errors.New(`duration out of range`),
				Query:         `foo[9.5e10]`,
			},
			ParseErr{
				Err:   errors.New(`duration must be greater than 0`),
				Query: `foo[9.5e10]`,
			},
		},
	},
	{
		input: "(sum(foo))",
		expected: &ParenExpr{
			Expr: &AggregateExpr{
				Op: SUM,
				Expr: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
					},
					PosRange: posrange.PositionRange{Start: 5, End: 8},
				},
				PosRange: posrange.PositionRange{Start: 1, End: 9},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{
		input: "(sum(foo) )",
		expected: &ParenExpr{
			Expr: &AggregateExpr{
				Op: SUM,
				Expr: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
					},
					PosRange: posrange.PositionRange{
						Start: 5,
						End:   8,
					},
				},
				PosRange: posrange.PositionRange{Start: 1, End: 9},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "(sum(foo) by (bar))",
		expected: &ParenExpr{
			Expr: &AggregateExpr{
				Op:       SUM,
				Grouping: []string{"bar"},
				Expr: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
					},
					PosRange: posrange.PositionRange{Start: 5, End: 8},
				},
				PosRange: posrange.PositionRange{Start: 1, End: 18},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 19},
		},
	},
	{
		input: "(sum by (bar) (foo))",
		expected: &ParenExpr{
			Expr: &AggregateExpr{
				Op:       SUM,
				Grouping: []string{"bar"},
				Expr: &VectorSelector{
					Name: "foo",
					LabelMatchers: []*labels.Matcher{
						MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
					},
					PosRange: posrange.PositionRange{Start: 15, End: 18},
				},
				PosRange: posrange.PositionRange{Start: 1, End: 19},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 20},
		},
	},
	{
		input: "sum by (job)(rate(http_requests_total[30m]))",
		expected: &AggregateExpr{
			Op:       SUM,
			Grouping: []string{"job"},
			Expr: &Call{
				Func: MustGetFunction("rate"),
				Args: Expressions{
					&MatrixSelector{
						VectorSelector: &VectorSelector{
							Name: "http_requests_total",
							LabelMatchers: []*labels.Matcher{
								MustLabelMatcher(labels.MatchEqual, model.MetricNameLabel, "http_requests_total"),
							},
							PosRange: posrange.PositionRange{
								Start: 18,
								End:   37,
							},
						},
						Range:  30 * time.Minute,
						EndPos: 42,
					},
				},
				PosRange: posrange.PositionRange{Start: 13, End: 43},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 44},
		},
	},
	{
		input: "sum(",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 4, End: 4},
				Err:           errors.New("unclosed left parenthesis"),
				Query:         "sum(",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0}, // FIXME: this position looks wrong.
				Err:           errors.New("no arguments for aggregate expression provided"),
				Query:         "sum(",
			},
		},
	},
	{
		input: "sum(rate(",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 9, End: 9},
				Err:           errors.New("unclosed left parenthesis"),
				Query:         "sum(rate(",
			},
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 0, End: 0}, // FIXME: this position looks wrong.
				Err:           errors.New("no arguments for aggregate expression provided"),
				Query:         "sum(rate(",
			},
		},
	},
	{
		input: "foo[5s x 5s]",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 7, End: 12},
				Err:           errors.New("unexpected character: 'x', expected ':'"),
				Query:         "foo[5s x 5s]",
			},
		},
	},
	{
		input: "foo[5s s 5s]",
		fail:  true,
		errors: ParseErrors{
			ParseErr{
				PositionRange: posrange.PositionRange{Start: 7, End: 12},
				Err:           errors.New("unexpected character: 's', expected ':'"),
				Query:         "foo[5s s 5s]",
			},
		},
	},
}

func makeInt64Pointer(val int64) *int64 {
	valp := new(int64)
	*valp = val
	return valp
}

func readable(s string) string {
	const maxReadableStringLen = 40
	if len(s) < maxReadableStringLen {
		return s
	}
	return s[:maxReadableStringLen] + "..."
}

func TestParseExpressions(t *testing.T) {
	// Enable experimental functions testing.
	EnableExperimentalFunctions = true
	// Enable experimental duration expression parsing.
	ExperimentalDurationExpr = true
	t.Cleanup(func() {
		EnableExperimentalFunctions = false
		ExperimentalDurationExpr = false
	})

	for _, test := range testExpr {
		t.Run(readable(test.input), func(t *testing.T) {
			expr, err := ParseExpr(test.input)

			// Unexpected errors are always caused by a bug.
			require.NotEqual(t, err, errUnexpected, "unexpected error occurred")

			if !test.fail {
				require.NoError(t, err)
				expected := test.expected

				// The FastRegexMatcher is not comparable with a deep equal, so only compare its String() version.
				if actualVector, ok := expr.(*VectorSelector); ok {
					require.IsType(t, test.expected, actualVector, "error on input '%s'", test.input)
					expectedVector := test.expected.(*VectorSelector)

					require.Len(t, actualVector.LabelMatchers, len(expectedVector.LabelMatchers), "error on input '%s'", test.input)

					for i := 0; i < len(actualVector.LabelMatchers); i++ {
						expectedMatcher := expectedVector.LabelMatchers[i].String()
						actualMatcher := actualVector.LabelMatchers[i].String()

						require.Equal(t, expectedMatcher, actualMatcher, "unexpected label matcher '%s' on input '%s'", actualMatcher, test.input)
					}

					// Make a shallow copy of the expected expr (because the test cases are defined in a global variable)
					// and then reset the LabelMatcher to not compared them with the following deep equal.
					expectedCopy := *expectedVector
					expectedCopy.LabelMatchers = nil
					expected = &expectedCopy
					actualVector.LabelMatchers = nil
				}

				require.Equal(t, expected, expr, "error on input '%s'", test.input)
			} else {
				require.Error(t, err)

				var errorList ParseErrors
				ok := errors.As(err, &errorList)

				require.True(t, ok, "unexpected error type")

				if diff := cmp.Diff(test.errors, errorList, equalParseErr()); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s\nErrors: %+v", diff, errorList)
				}

				for _, e := range errorList {
					require.LessOrEqual(t, 0, e.PositionRange.Start, "parse error has negative position\nExpression '%s'\nError: %v", test.input, e)
					require.LessOrEqual(t, e.PositionRange.Start, e.PositionRange.End, "parse error has negative length\nExpression '%s'\nError: %v", test.input, e)
					require.LessOrEqual(t, e.PositionRange.End, posrange.Pos(len(test.input)), "parse error is not contained in input\nExpression '%s'\nError: %v", test.input, e)
				}
			}
		})
	}
}

// Two errors are identical if they return the exact same string from Error() call.
func equalParseErr() cmp.Option {
	return cmp.Comparer(func(a, b ParseErr) bool {
		if a.Query != b.Query {
			return false
		}
		if a.LineOffset != b.LineOffset {
			return false
		}
		if !cmp.Equal(a.PositionRange, b.PositionRange) {
			return false
		}
		return a.Err.Error() == b.Err.Error()
	})
}

func TestParseSeriesDesc(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedLabels labels.Labels
		expectedValues []SequenceValue
		expectError    string
	}{
		{
			name:           "empty string",
			expectedLabels: labels.EmptyLabels(),
			expectedValues: []SequenceValue{},
		},
		{
			name:  "simple line",
			input: `http_requests{job="api-server", instance="0", group="production"}`,
			expectedLabels: labels.FromStrings(
				"__name__", "http_requests",
				"group", "production",
				"instance", "0",
				"job", "api-server",
			),
			expectedValues: []SequenceValue{},
		},
		{
			name:  "label name characters that require quoting",
			input: `{"http.requests", "service.name"="api-server", instance="0", group="canary"}		0+50x2`,
			expectedLabels: labels.FromStrings(
				"__name__", "http.requests",
				"group", "canary",
				"instance", "0",
				"service.name", "api-server",
			),
			expectedValues: []SequenceValue{
				{Value: 0, Omitted: false, Histogram: (*histogram.FloatHistogram)(nil)},
				{Value: 50, Omitted: false, Histogram: (*histogram.FloatHistogram)(nil)},
				{Value: 100, Omitted: false, Histogram: (*histogram.FloatHistogram)(nil)},
			},
		},
		{
			name:        "confirm failure on junk after identifier",
			input:       `{"http.requests"xx}		0+50x2`,
			expectError: `parse error: unexpected identifier "xx" in label set, expected "," or "}"`,
		},
		{
			name:        "confirm failure on bare operator after identifier",
			input:       `{"http.requests"=, x="y"}		0+50x2`,
			expectError: `parse error: unexpected "," in label set, expected string`,
		},
		{
			name:        "confirm failure on unterminated string identifier",
			input:       `{"http.requests}		0+50x2`,
			expectError: `parse error: unterminated quoted string`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, v, err := ParseSeriesDesc(tc.input)
			if tc.expectError != "" {
				require.Contains(t, err.Error(), tc.expectError)
			} else {
				require.NoError(t, err)
				require.True(t, labels.Equal(tc.expectedLabels, l))
				require.Equal(t, tc.expectedValues, v)
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
	require.True(t, math.IsNaN(nl.Val), "expected 'NaN' in number literal but got %v", nl.Val)
}

var testSeries = []struct {
	input          string
	expectedMetric labels.Labels
	expectedValues []SequenceValue
	fail           bool
}{
	{
		input:          `{} 1 2 3`,
		expectedMetric: labels.EmptyLabels(),
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
		input: `{} 1+1`,
		fail:  true,
	}, {
		input:          `{} 1x0`,
		expectedMetric: labels.EmptyLabels(),
		expectedValues: newSeq(1),
	}, {
		input:          `{} 1+1x0`,
		expectedMetric: labels.EmptyLabels(),
		expectedValues: newSeq(1),
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

func TestParseHistogramSeries(t *testing.T) {
	for _, test := range []struct {
		name          string
		input         string
		expected      []histogram.FloatHistogram
		expectedError string
	}{
		{
			name:     "empty histogram",
			input:    "{} {{}}",
			expected: []histogram.FloatHistogram{{}},
		},
		{
			name:     "empty histogram with space",
			input:    "{} {{ }}",
			expected: []histogram.FloatHistogram{{}},
		},
		{
			name:  "all properties used",
			input: `{} {{schema:1 sum:0.3 count:3.1 z_bucket:7.1 z_bucket_w:0.05 buckets:[5.1 10 7] offset:3 n_buckets:[4.1 5] n_offset:5 counter_reset_hint:gauge}}`,
			expected: []histogram.FloatHistogram{{
				Schema:           1,
				Sum:              0.3,
				Count:            3.1,
				ZeroCount:        7.1,
				ZeroThreshold:    0.05,
				PositiveBuckets:  []float64{5.1, 10, 7},
				PositiveSpans:    []histogram.Span{{Offset: 3, Length: 3}},
				NegativeBuckets:  []float64{4.1, 5},
				NegativeSpans:    []histogram.Span{{Offset: 5, Length: 2}},
				CounterResetHint: histogram.GaugeType,
			}},
		},
		{
			name:  "all properties used - with spaces",
			input: `{} {{schema:1  sum:0.3  count:3  z_bucket:7 z_bucket_w:5 buckets:[5 10  7  ] offset:-3  n_buckets:[4 5]  n_offset:5  counter_reset_hint:gauge  }}`,
			expected: []histogram.FloatHistogram{{
				Schema:           1,
				Sum:              0.3,
				Count:            3,
				ZeroCount:        7,
				ZeroThreshold:    5,
				PositiveBuckets:  []float64{5, 10, 7},
				PositiveSpans:    []histogram.Span{{Offset: -3, Length: 3}},
				NegativeBuckets:  []float64{4, 5},
				NegativeSpans:    []histogram.Span{{Offset: 5, Length: 2}},
				CounterResetHint: histogram.GaugeType,
			}},
		},
		{
			name:  "all properties used, with negative values where supported",
			input: `{} {{schema:1 sum:-0.3 count:-3.1 z_bucket:-7.1 z_bucket_w:0.05 buckets:[-5.1 -10 -7] offset:-3 n_buckets:[-4.1 -5] n_offset:-5 counter_reset_hint:gauge}}`,
			expected: []histogram.FloatHistogram{{
				Schema:           1,
				Sum:              -0.3,
				Count:            -3.1,
				ZeroCount:        -7.1,
				ZeroThreshold:    0.05,
				PositiveBuckets:  []float64{-5.1, -10, -7},
				PositiveSpans:    []histogram.Span{{Offset: -3, Length: 3}},
				NegativeBuckets:  []float64{-4.1, -5},
				NegativeSpans:    []histogram.Span{{Offset: -5, Length: 2}},
				CounterResetHint: histogram.GaugeType,
			}},
		},
		{
			name:  "static series",
			input: `{} {{buckets:[5 10 7] schema:1}}x2`,
			expected: []histogram.FloatHistogram{
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
			},
		},
		{
			name:  "static series - x0",
			input: `{} {{buckets:[5 10 7] schema:1}}x0`,
			expected: []histogram.FloatHistogram{
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
			},
		},
		{
			name:  "2 histograms stated explicitly",
			input: `{} {{buckets:[5 10 7] schema:1}} {{buckets:[1 2 3] schema:1}}`,
			expected: []histogram.FloatHistogram{
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
				{
					Schema:          1,
					PositiveBuckets: []float64{1, 2, 3},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
			},
		},
		{
			name:  "series with increment - with different schemas",
			input: `{} {{buckets:[5] schema:0}}+{{buckets:[1 2] schema:1}}x2`,
			expected: []histogram.FloatHistogram{
				{
					PositiveBuckets: []float64{5},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 1,
					}},
				},
				{
					PositiveBuckets: []float64{6, 2},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 2,
					}},
				},
				{
					PositiveBuckets: []float64{7, 4},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 2,
					}},
				},
			},
		},
		{
			name:  "series with two different increments",
			input: `{} {{sum:1}}+{{sum:1}}x2 {{sum:2}}+{{sum:2}}x2`,
			expected: []histogram.FloatHistogram{
				{
					Sum: 1,
				},
				{
					Sum: 2,
				},
				{
					Sum: 3,
				},
				{
					Sum: 2,
				},
				{
					Sum: 4,
				},
				{
					Sum: 6,
				},
			},
		},
		{
			name:  "series with decrement",
			input: `{} {{buckets:[5 10 7] schema:1}}-{{buckets:[1 2 3] schema:1}}x2`,
			expected: []histogram.FloatHistogram{
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
				{
					Schema:          1,
					PositiveBuckets: []float64{4, 8, 4},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
				{
					Schema:          1,
					PositiveBuckets: []float64{3, 6, 1},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
			},
		},
		{
			name:  "series with increment - 0x",
			input: `{} {{buckets:[5 10 7] schema:1}}+{{buckets:[1 2 3] schema:1}}x0`,
			expected: []histogram.FloatHistogram{
				{
					Schema:          1,
					PositiveBuckets: []float64{5, 10, 7},
					PositiveSpans: []histogram.Span{{
						Offset: 0,
						Length: 3,
					}},
				},
			},
		},
		{
			name:          "series with different schemas - second one is smaller",
			input:         `{} {{buckets:[5 10 7] schema:1}}+{{buckets:[1 2 3] schema:0}}x2`,
			expectedError: `1:63: parse error: error combining histograms: cannot merge from schema 0 to 1`,
		},
		{
			name:  "different order",
			input: `{} {{buckets:[5 10 7] schema:1}}`,
			expected: []histogram.FloatHistogram{{
				Schema:          1,
				PositiveBuckets: []float64{5, 10, 7},
				PositiveSpans: []histogram.Span{{
					Offset: 0,
					Length: 3,
				}},
			}},
		},
		{
			name:          "double property",
			input:         `{} {{schema:1 schema:1}}`,
			expectedError: `1:1: parse error: duplicate key "schema" in histogram`,
		},
		{
			name:          "unknown property",
			input:         `{} {{foo:1}}`,
			expectedError: `1:6: parse error: bad histogram descriptor found: "foo"`,
		},
		{
			name:          "space before :",
			input:         `{} {{schema :1}}`,
			expectedError: "1:6: parse error: missing `:` for histogram descriptor",
		},
		{
			name:          "space after :",
			input:         `{} {{schema: 1}}`,
			expectedError: `1:13: parse error: unexpected " " in series values`,
		},
		{
			name:          "space after [",
			input:         `{} {{buckets:[ 1]}}`,
			expectedError: `1:15: parse error: unexpected " " in series values`,
		},
		{
			name:          "space after {{",
			input:         `{} {{ schema:1}}`,
			expectedError: `1:7: parse error: unexpected "<Item 57372>" "schema" in series values`,
		},
		{
			name:          "invalid counter reset hint value",
			input:         `{} {{counter_reset_hint:foo}}`,
			expectedError: `1:25: parse error: bad histogram descriptor found: "foo"`,
		},
		{
			name:  "'unknown' counter reset hint value",
			input: `{} {{counter_reset_hint:unknown}}`,
			expected: []histogram.FloatHistogram{{
				CounterResetHint: histogram.UnknownCounterReset,
			}},
		},
		{
			name:  "'reset' counter reset hint value",
			input: `{} {{counter_reset_hint:reset}}`,
			expected: []histogram.FloatHistogram{{
				CounterResetHint: histogram.CounterReset,
			}},
		},
		{
			name:  "'not_reset' counter reset hint value",
			input: `{} {{counter_reset_hint:not_reset}}`,
			expected: []histogram.FloatHistogram{{
				CounterResetHint: histogram.NotCounterReset,
			}},
		},
		{
			name:  "'gauge' counter reset hint value",
			input: `{} {{counter_reset_hint:gauge}}`,
			expected: []histogram.FloatHistogram{{
				CounterResetHint: histogram.GaugeType,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, vals, err := ParseSeriesDesc(test.input)
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
				return
			}
			require.NoError(t, err)
			var got []histogram.FloatHistogram
			for _, v := range vals {
				got = append(got, *v.Histogram)
			}
			require.Equal(t, test.expected, got)
		})
	}
}

func TestHistogramTestExpression(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    histogram.FloatHistogram
		expected string
	}{
		{
			name: "single positive and negative span",
			input: histogram.FloatHistogram{
				Schema:          1,
				Sum:             -0.3,
				Count:           3.1,
				ZeroCount:       7.1,
				ZeroThreshold:   0.05,
				PositiveBuckets: []float64{5.1, 10, 7},
				PositiveSpans:   []histogram.Span{{Offset: -3, Length: 3}},
				NegativeBuckets: []float64{4.1, 5},
				NegativeSpans:   []histogram.Span{{Offset: -5, Length: 2}},
			},
			expected: `{{schema:1 count:3.1 sum:-0.3 z_bucket:7.1 z_bucket_w:0.05 offset:-3 buckets:[5.1 10 7] n_offset:-5 n_buckets:[4.1 5]}}`,
		},
		{
			name: "multiple positive and negative spans",
			input: histogram.FloatHistogram{
				PositiveBuckets: []float64{5.1, 10, 7},
				PositiveSpans: []histogram.Span{
					{Offset: -3, Length: 1},
					{Offset: 4, Length: 2},
				},
				NegativeBuckets: []float64{4.1, 5, 7, 8, 9},
				NegativeSpans: []histogram.Span{
					{Offset: -1, Length: 2},
					{Offset: 2, Length: 3},
				},
			},
			expected: `{{offset:-3 buckets:[5.1 0 0 0 0 10 7] n_offset:-1 n_buckets:[4.1 5 0 0 7 8 9]}}`,
		},
		{
			name: "known counter reset hint",
			input: histogram.FloatHistogram{
				Schema:           1,
				Sum:              -0.3,
				Count:            3.1,
				ZeroCount:        7.1,
				ZeroThreshold:    0.05,
				PositiveBuckets:  []float64{5.1, 10, 7},
				PositiveSpans:    []histogram.Span{{Offset: -3, Length: 3}},
				NegativeBuckets:  []float64{4.1, 5},
				NegativeSpans:    []histogram.Span{{Offset: -5, Length: 2}},
				CounterResetHint: histogram.CounterReset,
			},
			expected: `{{schema:1 count:3.1 sum:-0.3 z_bucket:7.1 z_bucket_w:0.05 counter_reset_hint:reset offset:-3 buckets:[5.1 10 7] n_offset:-5 n_buckets:[4.1 5]}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			expression := test.input.TestExpression()
			require.Equal(t, test.expected, expression)
			_, vals, err := ParseSeriesDesc("{} " + expression)
			require.NoError(t, err)
			require.Len(t, vals, 1)
			canonical := vals[0].Histogram
			require.NotNil(t, canonical)
			require.Equal(t, test.expected, canonical.TestExpression())
		})
	}
}

func TestParseSeries(t *testing.T) {
	for _, test := range testSeries {
		metric, vals, err := ParseSeriesDesc(test.input)

		// Unexpected errors are always caused by a bug.
		require.NotEqual(t, err, errUnexpected, "unexpected error occurred")

		if !test.fail {
			require.NoError(t, err)
			testutil.RequireEqual(t, test.expectedMetric, metric, "error on input '%s'", test.input)
			require.Equal(t, test.expectedValues, vals, "error in input '%s'", test.input)
		} else {
			require.Error(t, err)
		}
	}
}

func TestRecoverParserRuntime(t *testing.T) {
	p := NewParser("foo bar")
	var err error

	defer func() {
		require.Equal(t, errUnexpected, err)
	}()
	defer p.recover(&err)
	// Cause a runtime panic.
	var a []int
	a[123] = 1 //nolint:govet // This is intended to cause a runtime panic.
}

func TestRecoverParserError(t *testing.T) {
	p := NewParser("foo bar")
	var err error

	e := errors.New("custom error")

	defer func() {
		require.EqualError(t, err, e.Error())
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

func TestParseCustomFunctions(t *testing.T) {
	funcs := Functions
	funcs["custom_func"] = &Function{
		Name:       "custom_func",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
	}
	input := "custom_func(metric[1m])"
	p := NewParser(input, WithFunctions(funcs))
	expr, err := p.ParseExpr()
	require.NoError(t, err)

	call, ok := expr.(*Call)
	require.True(t, ok)
	require.Equal(t, "custom_func", call.Func.Name)
}
