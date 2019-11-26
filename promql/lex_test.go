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

	"github.com/prometheus/prometheus/util/testutil"
)

type testCase struct {
	input      string
	expected   []item
	fail       bool
	seriesDesc bool // Whether to lex a series description.
}

var tests = []struct {
	name  string
	tests []testCase
}{
	{
		name: "common",
		tests: []testCase{
			{
				input:    ",",
				expected: []item{{COMMA, 0, ","}},
			}, {
				input:    "()",
				expected: []item{{LEFT_PAREN, 0, `(`}, {RIGHT_PAREN, 1, `)`}},
			}, {
				input:    "{}",
				expected: []item{{LEFT_BRACE, 0, `{`}, {RIGHT_BRACE, 1, `}`}},
			}, {
				input: "[5m]",
				expected: []item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 1, `5m`},
					{RIGHT_BRACKET, 3, `]`},
				},
			}, {
				input: "[ 5m]",
				expected: []item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 2, `5m`},
					{RIGHT_BRACKET, 4, `]`},
				},
			}, {
				input: "[  5m]",
				expected: []item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 3, `5m`},
					{RIGHT_BRACKET, 5, `]`},
				},
			}, {
				input: "[  5m ]",
				expected: []item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 3, `5m`},
					{RIGHT_BRACKET, 6, `]`},
				},
			}, {
				input:    "\r\n\r",
				expected: []item{},
			},
		},
	},
	{
		name: "numbers",
		tests: []testCase{
			{
				input:    "1",
				expected: []item{{NUMBER, 0, "1"}},
			}, {
				input:    "4.23",
				expected: []item{{NUMBER, 0, "4.23"}},
			}, {
				input:    ".3",
				expected: []item{{NUMBER, 0, ".3"}},
			}, {
				input:    "5.",
				expected: []item{{NUMBER, 0, "5."}},
			}, {
				input:    "NaN",
				expected: []item{{NUMBER, 0, "NaN"}},
			}, {
				input:    "nAN",
				expected: []item{{NUMBER, 0, "nAN"}},
			}, {
				input:    "NaN 123",
				expected: []item{{NUMBER, 0, "NaN"}, {NUMBER, 4, "123"}},
			}, {
				input:    "NaN123",
				expected: []item{{IDENTIFIER, 0, "NaN123"}},
			}, {
				input:    "iNf",
				expected: []item{{NUMBER, 0, "iNf"}},
			}, {
				input:    "Inf",
				expected: []item{{NUMBER, 0, "Inf"}},
			}, {
				input:    "+Inf",
				expected: []item{{ADD, 0, "+"}, {NUMBER, 1, "Inf"}},
			}, {
				input:    "+Inf 123",
				expected: []item{{ADD, 0, "+"}, {NUMBER, 1, "Inf"}, {NUMBER, 5, "123"}},
			}, {
				input:    "-Inf",
				expected: []item{{SUB, 0, "-"}, {NUMBER, 1, "Inf"}},
			}, {
				input:    "Infoo",
				expected: []item{{IDENTIFIER, 0, "Infoo"}},
			}, {
				input:    "-Infoo",
				expected: []item{{SUB, 0, "-"}, {IDENTIFIER, 1, "Infoo"}},
			}, {
				input:    "-Inf 123",
				expected: []item{{SUB, 0, "-"}, {NUMBER, 1, "Inf"}, {NUMBER, 5, "123"}},
			}, {
				input:    "0x123",
				expected: []item{{NUMBER, 0, "0x123"}},
			},
		},
	},
	{
		name: "strings",
		tests: []testCase{
			{
				input:    "\"test\\tsequence\"",
				expected: []item{{STRING, 0, `"test\tsequence"`}},
			},
			{
				input:    "\"test\\\\.expression\"",
				expected: []item{{STRING, 0, `"test\\.expression"`}},
			},
			{
				input: "\"test\\.expression\"",
				expected: []item{
					{ERROR, 0, "unknown escape sequence U+002E '.'"},
					{STRING, 0, `"test\.expression"`},
				},
			},
			{
				input:    "`test\\.expression`",
				expected: []item{{STRING, 0, "`test\\.expression`"}},
			},
			{
				// See https://github.com/prometheus/prometheus/issues/939.
				input: ".٩",
				fail:  true,
			},
		},
	},
	{
		name: "durations",
		tests: []testCase{
			{
				input:    "5s",
				expected: []item{{DURATION, 0, "5s"}},
			}, {
				input:    "123m",
				expected: []item{{DURATION, 0, "123m"}},
			}, {
				input:    "1h",
				expected: []item{{DURATION, 0, "1h"}},
			}, {
				input:    "3w",
				expected: []item{{DURATION, 0, "3w"}},
			}, {
				input:    "1y",
				expected: []item{{DURATION, 0, "1y"}},
			},
		},
	},
	{
		name: "identifiers",
		tests: []testCase{
			{
				input:    "abc",
				expected: []item{{IDENTIFIER, 0, "abc"}},
			}, {
				input:    "a:bc",
				expected: []item{{METRIC_IDENTIFIER, 0, "a:bc"}},
			}, {
				input:    "abc d",
				expected: []item{{IDENTIFIER, 0, "abc"}, {IDENTIFIER, 4, "d"}},
			}, {
				input:    ":bc",
				expected: []item{{METRIC_IDENTIFIER, 0, ":bc"}},
			}, {
				input: "0a:bc",
				fail:  true,
			},
		},
	},
	{
		name: "comments",
		tests: []testCase{
			{
				input:    "# some comment",
				expected: []item{{COMMENT, 0, "# some comment"}},
			}, {
				input: "5 # 1+1\n5",
				expected: []item{
					{NUMBER, 0, "5"},
					{COMMENT, 2, "# 1+1"},
					{NUMBER, 8, "5"},
				},
			},
		},
	},
	{
		name: "operators",
		tests: []testCase{
			{
				input:    `=`,
				expected: []item{{ASSIGN, 0, `=`}},
			}, {
				// Inside braces equality is a single '=' character.
				input:    `{=}`,
				expected: []item{{LEFT_BRACE, 0, `{`}, {EQL, 1, `=`}, {RIGHT_BRACE, 2, `}`}},
			}, {
				input:    `==`,
				expected: []item{{EQL, 0, `==`}},
			}, {
				input:    `!=`,
				expected: []item{{NEQ, 0, `!=`}},
			}, {
				input:    `<`,
				expected: []item{{LSS, 0, `<`}},
			}, {
				input:    `>`,
				expected: []item{{GTR, 0, `>`}},
			}, {
				input:    `>=`,
				expected: []item{{GTE, 0, `>=`}},
			}, {
				input:    `<=`,
				expected: []item{{LTE, 0, `<=`}},
			}, {
				input:    `+`,
				expected: []item{{ADD, 0, `+`}},
			}, {
				input:    `-`,
				expected: []item{{SUB, 0, `-`}},
			}, {
				input:    `*`,
				expected: []item{{MUL, 0, `*`}},
			}, {
				input:    `/`,
				expected: []item{{DIV, 0, `/`}},
			}, {
				input:    `^`,
				expected: []item{{POW, 0, `^`}},
			}, {
				input:    `%`,
				expected: []item{{MOD, 0, `%`}},
			}, {
				input:    `AND`,
				expected: []item{{LAND, 0, `AND`}},
			}, {
				input:    `or`,
				expected: []item{{LOR, 0, `or`}},
			}, {
				input:    `unless`,
				expected: []item{{LUNLESS, 0, `unless`}},
			},
		},
	},
	{
		name: "aggregators",
		tests: []testCase{
			{
				input:    `sum`,
				expected: []item{{SUM, 0, `sum`}},
			}, {
				input:    `AVG`,
				expected: []item{{AVG, 0, `AVG`}},
			}, {
				input:    `MAX`,
				expected: []item{{MAX, 0, `MAX`}},
			}, {
				input:    `min`,
				expected: []item{{MIN, 0, `min`}},
			}, {
				input:    `count`,
				expected: []item{{COUNT, 0, `count`}},
			}, {
				input:    `stdvar`,
				expected: []item{{STDVAR, 0, `stdvar`}},
			}, {
				input:    `stddev`,
				expected: []item{{STDDEV, 0, `stddev`}},
			},
		},
	},
	{
		name: "keywords",
		tests: []testCase{
			{
				input:    "offset",
				expected: []item{{OFFSET, 0, "offset"}},
			}, {
				input:    "by",
				expected: []item{{BY, 0, "by"}},
			}, {
				input:    "without",
				expected: []item{{WITHOUT, 0, "without"}},
			}, {
				input:    "on",
				expected: []item{{ON, 0, "on"}},
			}, {
				input:    "ignoring",
				expected: []item{{IGNORING, 0, "ignoring"}},
			}, {
				input:    "group_left",
				expected: []item{{GROUP_LEFT, 0, "group_left"}},
			}, {
				input:    "group_right",
				expected: []item{{GROUP_RIGHT, 0, "group_right"}},
			}, {
				input:    "bool",
				expected: []item{{BOOL, 0, "bool"}},
			},
		},
	},
	{
		name: "selectors",
		tests: []testCase{
			{
				input: `台北`,
				fail:  true,
			}, {
				input: `{台北='a'}`,
				fail:  true,
			}, {
				input: `{0a='a'}`,
				fail:  true,
			}, {
				input: `{foo='bar'}`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `'bar'`},
					{RIGHT_BRACE, 10, `}`},
				},
			}, {
				input: `{foo="bar"}`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `"bar"`},
					{RIGHT_BRACE, 10, `}`},
				},
			}, {
				input: `{foo="bar\"bar"}`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `"bar\"bar"`},
					{RIGHT_BRACE, 15, `}`},
				},
			}, {
				input: `{NaN	!= "bar" }`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `NaN`},
					{NEQ, 5, `!=`},
					{STRING, 8, `"bar"`},
					{RIGHT_BRACE, 14, `}`},
				},
			}, {
				input: `{alert=~"bar" }`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `alert`},
					{EQL_REGEX, 6, `=~`},
					{STRING, 8, `"bar"`},
					{RIGHT_BRACE, 14, `}`},
				},
			}, {
				input: `{on!~"bar"}`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `on`},
					{NEQ_REGEX, 3, `!~`},
					{STRING, 5, `"bar"`},
					{RIGHT_BRACE, 10, `}`},
				},
			}, {
				input: `{alert!#"bar"}`, fail: true,
			}, {
				input: `{foo:a="bar"}`, fail: true,
			},
		},
	},
	{
		name: "common errors",
		tests: []testCase{
			{
				input: `=~`, fail: true,
			}, {
				input: `!~`, fail: true,
			}, {
				input: `!(`, fail: true,
			}, {
				input: "1a", fail: true,
			},
		},
	},
	{
		name: "mismatched parentheses",
		tests: []testCase{
			{
				input: `(`, fail: true,
			}, {
				input: `())`, fail: true,
			}, {
				input: `(()`, fail: true,
			}, {
				input: `{`, fail: true,
			}, {
				input: `}`, fail: true,
			}, {
				input: "{{", fail: true,
			}, {
				input: "{{}}", fail: true,
			}, {
				input: `[`, fail: true,
			}, {
				input: `[[`, fail: true,
			}, {
				input: `[]]`, fail: true,
			}, {
				input: `[[]]`, fail: true,
			}, {
				input: `]`, fail: true,
			},
		},
	},
	{
		name: "encoding issues",
		tests: []testCase{
			{
				input: "\"\xff\"", fail: true,
			},
			{
				input: "`\xff`", fail: true,
			},
		},
	},
	{
		name: "series descriptions",
		tests: []testCase{
			{
				input: `{} _ 1 x .3`,
				expected: []item{
					{LEFT_BRACE, 0, `{`},
					{RIGHT_BRACE, 1, `}`},
					{SPACE, 2, ` `},
					{BLANK, 3, `_`},
					{SPACE, 4, ` `},
					{NUMBER, 5, `1`},
					{SPACE, 6, ` `},
					{TIMES, 7, `x`},
					{SPACE, 8, ` `},
					{NUMBER, 9, `.3`},
				},
				seriesDesc: true,
			},
			{
				input: `metric +Inf Inf NaN`,
				expected: []item{
					{IDENTIFIER, 0, `metric`},
					{SPACE, 6, ` `},
					{ADD, 7, `+`},
					{NUMBER, 8, `Inf`},
					{SPACE, 11, ` `},
					{NUMBER, 12, `Inf`},
					{SPACE, 15, ` `},
					{NUMBER, 16, `NaN`},
				},
				seriesDesc: true,
			},
			{
				input: `metric 1+1x4`,
				expected: []item{
					{IDENTIFIER, 0, `metric`},
					{SPACE, 6, ` `},
					{NUMBER, 7, `1`},
					{ADD, 8, `+`},
					{NUMBER, 9, `1`},
					{TIMES, 10, `x`},
					{NUMBER, 11, `4`},
				},
				seriesDesc: true,
			},
		},
	},
	{
		name: "subqueries",
		tests: []testCase{
			{
				input: `test_name{on!~"bar"}[4m:4s]`,
				expected: []item{
					{IDENTIFIER, 0, `test_name`},
					{LEFT_BRACE, 9, `{`},
					{IDENTIFIER, 10, `on`},
					{NEQ_REGEX, 12, `!~`},
					{STRING, 14, `"bar"`},
					{RIGHT_BRACE, 19, `}`},
					{LEFT_BRACKET, 20, `[`},
					{DURATION, 21, `4m`},
					{COLON, 23, `:`},
					{DURATION, 24, `4s`},
					{RIGHT_BRACKET, 26, `]`},
				},
			},
			{
				input: `test:name{on!~"bar"}[4m:4s]`,
				expected: []item{
					{METRIC_IDENTIFIER, 0, `test:name`},
					{LEFT_BRACE, 9, `{`},
					{IDENTIFIER, 10, `on`},
					{NEQ_REGEX, 12, `!~`},
					{STRING, 14, `"bar"`},
					{RIGHT_BRACE, 19, `}`},
					{LEFT_BRACKET, 20, `[`},
					{DURATION, 21, `4m`},
					{COLON, 23, `:`},
					{DURATION, 24, `4s`},
					{RIGHT_BRACKET, 26, `]`},
				},
			}, {
				input: `test:name{on!~"b:ar"}[4m:4s]`,
				expected: []item{
					{METRIC_IDENTIFIER, 0, `test:name`},
					{LEFT_BRACE, 9, `{`},
					{IDENTIFIER, 10, `on`},
					{NEQ_REGEX, 12, `!~`},
					{STRING, 14, `"b:ar"`},
					{RIGHT_BRACE, 20, `}`},
					{LEFT_BRACKET, 21, `[`},
					{DURATION, 22, `4m`},
					{COLON, 24, `:`},
					{DURATION, 25, `4s`},
					{RIGHT_BRACKET, 27, `]`},
				},
			}, {
				input: `test:name{on!~"b:ar"}[4m:]`,
				expected: []item{
					{METRIC_IDENTIFIER, 0, `test:name`},
					{LEFT_BRACE, 9, `{`},
					{IDENTIFIER, 10, `on`},
					{NEQ_REGEX, 12, `!~`},
					{STRING, 14, `"b:ar"`},
					{RIGHT_BRACE, 20, `}`},
					{LEFT_BRACKET, 21, `[`},
					{DURATION, 22, `4m`},
					{COLON, 24, `:`},
					{RIGHT_BRACKET, 25, `]`},
				},
			}, { // Nested Subquery.
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
				expected: []item{

					{IDENTIFIER, 0, `min_over_time`},
					{LEFT_PAREN, 13, `(`},
					{IDENTIFIER, 14, `rate`},
					{LEFT_PAREN, 18, `(`},
					{IDENTIFIER, 19, `foo`},
					{LEFT_BRACE, 22, `{`},
					{IDENTIFIER, 23, `bar`},
					{EQL, 26, `=`},
					{STRING, 27, `"baz"`},
					{RIGHT_BRACE, 32, `}`},
					{LEFT_BRACKET, 33, `[`},
					{DURATION, 34, `2s`},
					{RIGHT_BRACKET, 36, `]`},
					{RIGHT_PAREN, 37, `)`},
					{LEFT_BRACKET, 38, `[`},
					{DURATION, 39, `5m`},
					{COLON, 41, `:`},
					{RIGHT_BRACKET, 42, `]`},
					{RIGHT_PAREN, 43, `)`},
					{LEFT_BRACKET, 44, `[`},
					{DURATION, 45, `4m`},
					{COLON, 47, `:`},
					{DURATION, 48, `3s`},
					{RIGHT_BRACKET, 50, `]`},
				},
			},
			// Subquery with offset.
			{
				input: `test:name{on!~"b:ar"}[4m:4s] offset 10m`,
				expected: []item{
					{METRIC_IDENTIFIER, 0, `test:name`},
					{LEFT_BRACE, 9, `{`},
					{IDENTIFIER, 10, `on`},
					{NEQ_REGEX, 12, `!~`},
					{STRING, 14, `"b:ar"`},
					{RIGHT_BRACE, 20, `}`},
					{LEFT_BRACKET, 21, `[`},
					{DURATION, 22, `4m`},
					{COLON, 24, `:`},
					{DURATION, 25, `4s`},
					{RIGHT_BRACKET, 27, `]`},
					{OFFSET, 29, "offset"},
					{DURATION, 36, "10m"},
				},
			}, {
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 6m)[4m:3s]`,
				expected: []item{

					{IDENTIFIER, 0, `min_over_time`},
					{LEFT_PAREN, 13, `(`},
					{IDENTIFIER, 14, `rate`},
					{LEFT_PAREN, 18, `(`},
					{IDENTIFIER, 19, `foo`},
					{LEFT_BRACE, 22, `{`},
					{IDENTIFIER, 23, `bar`},
					{EQL, 26, `=`},
					{STRING, 27, `"baz"`},
					{RIGHT_BRACE, 32, `}`},
					{LEFT_BRACKET, 33, `[`},
					{DURATION, 34, `2s`},
					{RIGHT_BRACKET, 36, `]`},
					{RIGHT_PAREN, 37, `)`},
					{LEFT_BRACKET, 38, `[`},
					{DURATION, 39, `5m`},
					{COLON, 41, `:`},
					{RIGHT_BRACKET, 42, `]`},
					{OFFSET, 44, `offset`},
					{DURATION, 51, `6m`},
					{RIGHT_PAREN, 53, `)`},
					{LEFT_BRACKET, 54, `[`},
					{DURATION, 55, `4m`},
					{COLON, 57, `:`},
					{DURATION, 58, `3s`},
					{RIGHT_BRACKET, 60, `]`},
				},
			},
			{
				input: `test:name[ 5m]`,
				expected: []item{
					{METRIC_IDENTIFIER, 0, `test:name`},
					{LEFT_BRACKET, 9, `[`},
					{DURATION, 11, `5m`},
					{RIGHT_BRACKET, 13, `]`},
				},
			},
			{
				input: `test:name{o:n!~"bar"}[4m:4s]`,
				fail:  true,
			},
			{
				input: `test:name{on!~"bar"}[4m:4s:4h]`,
				fail:  true,
			},
			{
				input: `test:name{on!~"bar"}[4m:4s:]`,
				fail:  true,
			},
			{
				input: `test:name{on!~"bar"}[4m::]`,
				fail:  true,
			},
			{
				input: `test:name{on!~"bar"}[:4s]`,
				fail:  true,
			},
		},
	},
}

// TestLexer tests basic functionality of the lexer. More elaborate tests are implemented
// for the parser to avoid duplicated effort.
func TestLexer(t *testing.T) {
	for _, typ := range tests {
		t.Run(typ.name, func(t *testing.T) {
			for i, test := range typ.tests {
				l := &lexer{
					input:      test.input,
					seriesDesc: test.seriesDesc,
				}
				l.run()

				out := l.items

				lastItem := out[len(out)-1]
				if test.fail {
					if lastItem.typ != ERROR {
						t.Logf("%d: input %q", i, test.input)
						t.Fatalf("expected lexing error but did not fail")
					}
					continue
				}
				if lastItem.typ == ERROR {
					t.Logf("%d: input %q", i, test.input)
					t.Fatalf("unexpected lexing error at position %d: %s", lastItem.pos, lastItem)
				}

				eofItem := item{EOF, Pos(len(test.input)), ""}
				testutil.Equals(t, lastItem, eofItem, "%d: input %q", i, test.input)

				out = out[:len(out)-1]
				testutil.Equals(t, out, test.expected, "%d: input %q", i, test.input)
			}
		})
	}
}
