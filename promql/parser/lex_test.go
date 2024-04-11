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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser/posrange"
)

type testCase struct {
	input      string
	expected   []Item
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
				expected: []Item{{COMMA, 0, ","}},
			}, {
				input:    "()",
				expected: []Item{{LEFT_PAREN, 0, `(`}, {RIGHT_PAREN, 1, `)`}},
			}, {
				input:    "{}",
				expected: []Item{{LEFT_BRACE, 0, `{`}, {RIGHT_BRACE, 1, `}`}},
			}, {
				input: "[5m]",
				expected: []Item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 1, `5m`},
					{RIGHT_BRACKET, 3, `]`},
				},
			}, {
				input: "[ 5m]",
				expected: []Item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 2, `5m`},
					{RIGHT_BRACKET, 4, `]`},
				},
			}, {
				input: "[  5m]",
				expected: []Item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 3, `5m`},
					{RIGHT_BRACKET, 5, `]`},
				},
			}, {
				input: "[  5m ]",
				expected: []Item{
					{LEFT_BRACKET, 0, `[`},
					{DURATION, 3, `5m`},
					{RIGHT_BRACKET, 6, `]`},
				},
			}, {
				input:    "\r\n\r",
				expected: []Item{},
			},
		},
	},
	{
		name: "numbers",
		tests: []testCase{
			{
				input:    "1",
				expected: []Item{{NUMBER, 0, "1"}},
			}, {
				input:    "4.23",
				expected: []Item{{NUMBER, 0, "4.23"}},
			}, {
				input:    ".3",
				expected: []Item{{NUMBER, 0, ".3"}},
			}, {
				input:    "5.",
				expected: []Item{{NUMBER, 0, "5."}},
			}, {
				input:    "NaN",
				expected: []Item{{NUMBER, 0, "NaN"}},
			}, {
				input:    "nAN",
				expected: []Item{{NUMBER, 0, "nAN"}},
			}, {
				input:    "NaN 123",
				expected: []Item{{NUMBER, 0, "NaN"}, {NUMBER, 4, "123"}},
			}, {
				input:    "NaN123",
				expected: []Item{{IDENTIFIER, 0, "NaN123"}},
			}, {
				input:    "iNf",
				expected: []Item{{NUMBER, 0, "iNf"}},
			}, {
				input:    "Inf",
				expected: []Item{{NUMBER, 0, "Inf"}},
			}, {
				input:    "+Inf",
				expected: []Item{{ADD, 0, "+"}, {NUMBER, 1, "Inf"}},
			}, {
				input:    "+Inf 123",
				expected: []Item{{ADD, 0, "+"}, {NUMBER, 1, "Inf"}, {NUMBER, 5, "123"}},
			}, {
				input:    "-Inf",
				expected: []Item{{SUB, 0, "-"}, {NUMBER, 1, "Inf"}},
			}, {
				input:    "Infoo",
				expected: []Item{{IDENTIFIER, 0, "Infoo"}},
			}, {
				input:    "-Infoo",
				expected: []Item{{SUB, 0, "-"}, {IDENTIFIER, 1, "Infoo"}},
			}, {
				input:    "-Inf 123",
				expected: []Item{{SUB, 0, "-"}, {NUMBER, 1, "Inf"}, {NUMBER, 5, "123"}},
			}, {
				input:    "0x123",
				expected: []Item{{NUMBER, 0, "0x123"}},
			},
		},
	},
	{
		name: "strings",
		tests: []testCase{
			{
				input:    "\"test\\tsequence\"",
				expected: []Item{{STRING, 0, `"test\tsequence"`}},
			},
			{
				input:    "\"test\\\\.expression\"",
				expected: []Item{{STRING, 0, `"test\\.expression"`}},
			},
			{
				input: "\"test\\.expression\"",
				expected: []Item{
					{ERROR, 0, "unknown escape sequence U+002E '.'"},
					{STRING, 0, `"test\.expression"`},
				},
			},
			{
				input:    "`test\\.expression`",
				expected: []Item{{STRING, 0, "`test\\.expression`"}},
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
				expected: []Item{{DURATION, 0, "5s"}},
			}, {
				input:    "123m",
				expected: []Item{{DURATION, 0, "123m"}},
			}, {
				input:    "1h",
				expected: []Item{{DURATION, 0, "1h"}},
			}, {
				input:    "3w",
				expected: []Item{{DURATION, 0, "3w"}},
			}, {
				input:    "1y",
				expected: []Item{{DURATION, 0, "1y"}},
			},
		},
	},
	{
		name: "identifiers",
		tests: []testCase{
			{
				input:    "abc",
				expected: []Item{{IDENTIFIER, 0, "abc"}},
			}, {
				input:    "a:bc",
				expected: []Item{{METRIC_IDENTIFIER, 0, "a:bc"}},
			}, {
				input:    "abc d",
				expected: []Item{{IDENTIFIER, 0, "abc"}, {IDENTIFIER, 4, "d"}},
			}, {
				input:    ":bc",
				expected: []Item{{METRIC_IDENTIFIER, 0, ":bc"}},
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
				expected: []Item{{COMMENT, 0, "# some comment"}},
			}, {
				input: "5 # 1+1\n5",
				expected: []Item{
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
				expected: []Item{{EQL, 0, `=`}},
			}, {
				// Inside braces equality is a single '=' character but in terms of a token
				// it should be treated as ASSIGN.
				input:    `{=}`,
				expected: []Item{{LEFT_BRACE, 0, `{`}, {EQL, 1, `=`}, {RIGHT_BRACE, 2, `}`}},
			}, {
				input:    `==`,
				expected: []Item{{EQLC, 0, `==`}},
			}, {
				input:    `!=`,
				expected: []Item{{NEQ, 0, `!=`}},
			}, {
				input:    `<`,
				expected: []Item{{LSS, 0, `<`}},
			}, {
				input:    `>`,
				expected: []Item{{GTR, 0, `>`}},
			}, {
				input:    `>=`,
				expected: []Item{{GTE, 0, `>=`}},
			}, {
				input:    `<=`,
				expected: []Item{{LTE, 0, `<=`}},
			}, {
				input:    `+`,
				expected: []Item{{ADD, 0, `+`}},
			}, {
				input:    `-`,
				expected: []Item{{SUB, 0, `-`}},
			}, {
				input:    `*`,
				expected: []Item{{MUL, 0, `*`}},
			}, {
				input:    `/`,
				expected: []Item{{DIV, 0, `/`}},
			}, {
				input:    `^`,
				expected: []Item{{POW, 0, `^`}},
			}, {
				input:    `%`,
				expected: []Item{{MOD, 0, `%`}},
			}, {
				input:    `AND`,
				expected: []Item{{LAND, 0, `AND`}},
			}, {
				input:    `or`,
				expected: []Item{{LOR, 0, `or`}},
			}, {
				input:    `unless`,
				expected: []Item{{LUNLESS, 0, `unless`}},
			}, {
				input:    `@`,
				expected: []Item{{AT, 0, `@`}},
			},
		},
	},
	{
		name: "aggregators",
		tests: []testCase{
			{
				input:    `sum`,
				expected: []Item{{SUM, 0, `sum`}},
			}, {
				input:    `AVG`,
				expected: []Item{{AVG, 0, `AVG`}},
			}, {
				input:    `GROUP`,
				expected: []Item{{GROUP, 0, `GROUP`}},
			}, {
				input:    `MAX`,
				expected: []Item{{MAX, 0, `MAX`}},
			}, {
				input:    `min`,
				expected: []Item{{MIN, 0, `min`}},
			}, {
				input:    `count`,
				expected: []Item{{COUNT, 0, `count`}},
			}, {
				input:    `stdvar`,
				expected: []Item{{STDVAR, 0, `stdvar`}},
			}, {
				input:    `stddev`,
				expected: []Item{{STDDEV, 0, `stddev`}},
			},
		},
	},
	{
		name: "keywords",
		tests: []testCase{
			{
				input:    "offset",
				expected: []Item{{OFFSET, 0, "offset"}},
			},
			{
				input:    "by",
				expected: []Item{{BY, 0, "by"}},
			},
			{
				input:    "without",
				expected: []Item{{WITHOUT, 0, "without"}},
			},
			{
				input:    "on",
				expected: []Item{{ON, 0, "on"}},
			},
			{
				input:    "ignoring",
				expected: []Item{{IGNORING, 0, "ignoring"}},
			},
			{
				input:    "group_left",
				expected: []Item{{GROUP_LEFT, 0, "group_left"}},
			},
			{
				input:    "group_right",
				expected: []Item{{GROUP_RIGHT, 0, "group_right"}},
			},
			{
				input:    "bool",
				expected: []Item{{BOOL, 0, "bool"}},
			},
			{
				input:    "atan2",
				expected: []Item{{ATAN2, 0, "atan2"}},
			},
		},
	},
	{
		name: "preprocessors",
		tests: []testCase{
			{
				input:    `start`,
				expected: []Item{{START, 0, `start`}},
			},
			{
				input:    `end`,
				expected: []Item{{END, 0, `end`}},
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
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `'bar'`},
					{RIGHT_BRACE, 10, `}`},
				},
			}, {
				input: `{foo="bar"}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `"bar"`},
					{RIGHT_BRACE, 10, `}`},
				},
			}, {
				input: `{foo="bar\"bar"}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `foo`},
					{EQL, 4, `=`},
					{STRING, 5, `"bar\"bar"`},
					{RIGHT_BRACE, 15, `}`},
				},
			}, {
				input: `{NaN	!= "bar" }`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `NaN`},
					{NEQ, 5, `!=`},
					{STRING, 8, `"bar"`},
					{RIGHT_BRACE, 14, `}`},
				},
			}, {
				input: `{alert=~"bar" }`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{IDENTIFIER, 1, `alert`},
					{EQL_REGEX, 6, `=~`},
					{STRING, 8, `"bar"`},
					{RIGHT_BRACE, 14, `}`},
				},
			}, {
				input: `{on!~"bar"}`,
				expected: []Item{
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
		name: "histogram series descriptions",
		tests: []testCase{
			{
				input: `{} {{buckets:[5]}}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{RIGHT_BRACE, 1, `}`},
					{SPACE, 2, ` `},
					{OPEN_HIST, 3, `{{`},
					{BUCKETS_DESC, 5, `buckets`},
					{COLON, 12, `:`},
					{LEFT_BRACKET, 13, `[`},
					{NUMBER, 14, `5`},
					{RIGHT_BRACKET, 15, `]`},
					{CLOSE_HIST, 16, `}}`},
				},
				seriesDesc: true,
			},
			{
				input: `{} {{buckets: [5 10 7]}}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{RIGHT_BRACE, 1, `}`},
					{SPACE, 2, ` `},
					{OPEN_HIST, 3, `{{`},
					{BUCKETS_DESC, 5, `buckets`},
					{COLON, 12, `:`},
					{SPACE, 13, ` `},
					{LEFT_BRACKET, 14, `[`},
					{NUMBER, 15, `5`},
					{SPACE, 16, ` `},
					{NUMBER, 17, `10`},
					{SPACE, 19, ` `},
					{NUMBER, 20, `7`},
					{RIGHT_BRACKET, 21, `]`},
					{CLOSE_HIST, 22, `}}`},
				},
				seriesDesc: true,
			},
			{
				input: `{} {{buckets: [5 10 7] schema:1}}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{RIGHT_BRACE, 1, `}`},
					{SPACE, 2, ` `},
					{OPEN_HIST, 3, `{{`},
					{BUCKETS_DESC, 5, `buckets`},
					{COLON, 12, `:`},
					{SPACE, 13, ` `},
					{LEFT_BRACKET, 14, `[`},
					{NUMBER, 15, `5`},
					{SPACE, 16, ` `},
					{NUMBER, 17, `10`},
					{SPACE, 19, ` `},
					{NUMBER, 20, `7`},
					{RIGHT_BRACKET, 21, `]`},
					{SPACE, 22, ` `},
					{SCHEMA_DESC, 23, `schema`},
					{COLON, 29, `:`},
					{NUMBER, 30, `1`},
					{CLOSE_HIST, 31, `}}`},
				},
				seriesDesc: true,
			},
			{ // Series with sum as -Inf and count as NaN.
				input: `{} {{buckets: [5 10 7] sum:Inf count:NaN}}`,
				expected: []Item{
					{LEFT_BRACE, 0, `{`},
					{RIGHT_BRACE, 1, `}`},
					{SPACE, 2, ` `},
					{OPEN_HIST, 3, `{{`},
					{BUCKETS_DESC, 5, `buckets`},
					{COLON, 12, `:`},
					{SPACE, 13, ` `},
					{LEFT_BRACKET, 14, `[`},
					{NUMBER, 15, `5`},
					{SPACE, 16, ` `},
					{NUMBER, 17, `10`},
					{SPACE, 19, ` `},
					{NUMBER, 20, `7`},
					{RIGHT_BRACKET, 21, `]`},
					{SPACE, 22, ` `},
					{SUM_DESC, 23, `sum`},
					{COLON, 26, `:`},
					{NUMBER, 27, `Inf`},
					{SPACE, 30, ` `},
					{COUNT_DESC, 31, `count`},
					{COLON, 36, `:`},
					{NUMBER, 37, `NaN`},
					{CLOSE_HIST, 40, `}}`},
				},
				seriesDesc: true,
			},
		},
	},
	{
		name: "series descriptions",
		tests: []testCase{
			{
				input: `{} _ 1 x .3`,
				expected: []Item{
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
				expected: []Item{
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
				expected: []Item{
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
				expected: []Item{
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
				expected: []Item{
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
			},
			{
				input: `test:name{on!~"b:ar"}[4m:4s]`,
				expected: []Item{
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
			},
			{
				input: `test:name{on!~"b:ar"}[4m:]`,
				expected: []Item{
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
			},
			{ // Nested Subquery.
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
				expected: []Item{
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
				expected: []Item{
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
			},
			{
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 6m)[4m:3s]`,
				expected: []Item{
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
				expected: []Item{
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
				l := &Lexer{
					input:      test.input,
					seriesDesc: test.seriesDesc,
				}

				var out []Item

				for l.state = lexStatements; l.state != nil; {
					out = append(out, Item{})
					l.NextItem(&out[len(out)-1])
				}

				lastItem := out[len(out)-1]
				if test.fail {
					hasError := false
					for _, item := range out {
						if item.Typ == ERROR {
							hasError = true
						}
					}
					require.True(t, hasError, "%d: input %q, expected lexing error but did not fail", i, test.input)
					continue
				}
				require.NotEqual(t, ERROR, lastItem.Typ, "%d: input %q, unexpected lexing error at position %d: %s", i, test.input, lastItem.Pos, lastItem)

				eofItem := Item{EOF, posrange.Pos(len(test.input)), ""}
				require.Equal(t, lastItem, eofItem, "%d: input %q", i, test.input)

				out = out[:len(out)-1]
				require.Equal(t, test.expected, out, "%d: input %q", i, test.input)
			}
		})
	}
}
