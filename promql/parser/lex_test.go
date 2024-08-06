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
				expected: []Item{{Typ: COMMA, Pos: 0, Val: ","}},
			}, {
				input:    "()",
				expected: []Item{{Typ: LEFT_PAREN, Pos: 0, Val: `(`}, {Typ: RIGHT_PAREN, Pos: 1, Val: `)`}},
			}, {
				input:    "{}",
				expected: []Item{{Typ: LEFT_BRACE, Pos: 0, Val: `{`}, {Typ: RIGHT_BRACE, Pos: 1, Val: `}`}},
			}, {
				input: "[5m]",
				expected: []Item{
					{Typ: LEFT_BRACKET, Pos: 0, Val: `[`},
					{Typ: DURATION, Pos: 1, Val: `5m`},
					{Typ: RIGHT_BRACKET, Pos: 3, Val: `]`},
				},
			}, {
				input: "[ 5m]",
				expected: []Item{
					{Typ: LEFT_BRACKET, Pos: 0, Val: `[`},
					{Typ: DURATION, Pos: 2, Val: `5m`},
					{Typ: RIGHT_BRACKET, Pos: 4, Val: `]`},
				},
			}, {
				input: "[  5m]",
				expected: []Item{
					{Typ: LEFT_BRACKET, Pos: 0, Val: `[`},
					{Typ: DURATION, Pos: 3, Val: `5m`},
					{Typ: RIGHT_BRACKET, Pos: 5, Val: `]`},
				},
			}, {
				input: "[  5m ]",
				expected: []Item{
					{Typ: LEFT_BRACKET, Pos: 0, Val: `[`},
					{Typ: DURATION, Pos: 3, Val: `5m`},
					{Typ: RIGHT_BRACKET, Pos: 6, Val: `]`},
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
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "1"}},
			}, {
				input:    "4.23",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "4.23"}},
			}, {
				input:    ".3",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: ".3"}},
			}, {
				input:    "5.",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "5."}},
			}, {
				input:    "NaN",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "NaN"}},
			}, {
				input:    "nAN",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "nAN"}},
			}, {
				input:    "NaN 123",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "NaN"}, {Typ: NUMBER, Pos: 4, Val: "123"}},
			}, {
				input:    "NaN123",
				expected: []Item{{Typ: IDENTIFIER, Pos: 0, Val: "NaN123"}},
			}, {
				input:    "iNf",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "iNf"}},
			}, {
				input:    "Inf",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "Inf"}},
			}, {
				input:    "+Inf",
				expected: []Item{{Typ: ADD, Pos: 0, Val: "+"}, {Typ: NUMBER, Pos: 1, Val: "Inf"}},
			}, {
				input:    "+Inf 123",
				expected: []Item{{Typ: ADD, Pos: 0, Val: "+"}, {Typ: NUMBER, Pos: 1, Val: "Inf"}, {Typ: NUMBER, Pos: 5, Val: "123"}},
			}, {
				input:    "-Inf",
				expected: []Item{{Typ: SUB, Pos: 0, Val: "-"}, {Typ: NUMBER, Pos: 1, Val: "Inf"}},
			}, {
				input:    "Infoo",
				expected: []Item{{Typ: IDENTIFIER, Pos: 0, Val: "Infoo"}},
			}, {
				input:    "-Infoo",
				expected: []Item{{Typ: SUB, Pos: 0, Val: "-"}, {Typ: IDENTIFIER, Pos: 1, Val: "Infoo"}},
			}, {
				input:    "-Inf 123",
				expected: []Item{{Typ: SUB, Pos: 0, Val: "-"}, {Typ: NUMBER, Pos: 1, Val: "Inf"}, {Typ: NUMBER, Pos: 5, Val: "123"}},
			}, {
				input:    "0x123",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "0x123"}},
			}, {
				input: "1..2",
				fail:  true,
			}, {
				input: "1.2.",
				fail:  true,
			}, {
				input:    "00_1_23_4.56_7_8",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "00_1_23_4.56_7_8"}},
			}, {
				input: "00_1_23__4.56_7_8",
				fail:  true,
			}, {
				input: "00_1_23_4._56_7_8",
				fail:  true,
			}, {
				input: "00_1_23_4_.56_7_8",
				fail:  true,
			}, {
				input:    "0x1_2_34",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "0x1_2_34"}},
			}, {
				input: "0x1_2__34",
				fail:  true,
			}, {
				input: "0x1_2__34.5_6p1", // "0x1.1p1"-based formats are not supported yet.
				fail:  true,
			}, {
				input: "0x1_2__34.5_6",
				fail:  true,
			}, {
				input: "0x1_2__34.56",
				fail:  true,
			}, {
				input: "1_e2",
				fail:  true,
			}, {
				input:    "1.e2",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "1.e2"}},
			}, {
				input: "1e.2",
				fail:  true,
			}, {
				input: "1e+.2",
				fail:  true,
			}, {
				input: "1ee2",
				fail:  true,
			}, {
				input: "1e+e2",
				fail:  true,
			}, {
				input: "1e",
				fail:  true,
			}, {
				input: "1e+",
				fail:  true,
			}, {
				input:    "1e1_2_34",
				expected: []Item{{Typ: NUMBER, Pos: 0, Val: "1e1_2_34"}},
			}, {
				input: "1e_1_2_34",
				fail:  true,
			}, {
				input: "1e1_2__34",
				fail:  true,
			}, {
				input: "1e+_1_2_34",
				fail:  true,
			}, {
				input: "1e-_1_2_34",
				fail:  true,
			}, {
				input: "12_",
				fail:  true,
			}, {
				input:    "_1_2",
				expected: []Item{{Typ: IDENTIFIER, Pos: 0, Val: "_1_2"}},
			},
		},
	},
	{
		name: "strings",
		tests: []testCase{
			{
				input:    "\"test\\tsequence\"",
				expected: []Item{{Typ: STRING, Pos: 0, Val: `"test\tsequence"`}},
			},
			{
				input:    "\"test\\\\.expression\"",
				expected: []Item{{Typ: STRING, Pos: 0, Val: `"test\\.expression"`}},
			},
			{
				input: "\"test\\.expression\"",
				expected: []Item{
					{Typ: ERROR, Pos: 0, Val: "unknown escape sequence U+002E '.'"},
					{Typ: STRING, Pos: 0, Val: `"test\.expression"`},
				},
			},
			{
				input:    "`test\\.expression`",
				expected: []Item{{Typ: STRING, Pos: 0, Val: "`test\\.expression`"}},
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
				expected: []Item{{Typ: DURATION, Pos: 0, Val: "5s"}},
			}, {
				input:    "123m",
				expected: []Item{{Typ: DURATION, Pos: 0, Val: "123m"}},
			}, {
				input:    "1h",
				expected: []Item{{Typ: DURATION, Pos: 0, Val: "1h"}},
			}, {
				input:    "3w",
				expected: []Item{{Typ: DURATION, Pos: 0, Val: "3w"}},
			}, {
				input:    "1y",
				expected: []Item{{Typ: DURATION, Pos: 0, Val: "1y"}},
			},
		},
	},
	{
		name: "identifiers",
		tests: []testCase{
			{
				input:    "abc",
				expected: []Item{{Typ: IDENTIFIER, Pos: 0, Val: "abc"}},
			}, {
				input:    "a:bc",
				expected: []Item{{Typ: METRIC_IDENTIFIER, Pos: 0, Val: "a:bc"}},
			}, {
				input:    "abc d",
				expected: []Item{{Typ: IDENTIFIER, Pos: 0, Val: "abc"}, {Typ: IDENTIFIER, Pos: 4, Val: "d"}},
			}, {
				input:    ":bc",
				expected: []Item{{Typ: METRIC_IDENTIFIER, Pos: 0, Val: ":bc"}},
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
				expected: []Item{{Typ: COMMENT, Pos: 0, Val: "# some comment"}},
			}, {
				input: "5 # 1+1\n5",
				expected: []Item{
					{Typ: NUMBER, Pos: 0, Val: "5"},
					{Typ: COMMENT, Pos: 2, Val: "# 1+1"},
					{Typ: NUMBER, Pos: 8, Val: "5"},
				},
			},
		},
	},
	{
		name: "operators",
		tests: []testCase{
			{
				input:    `=`,
				expected: []Item{{Typ: EQL, Pos: 0, Val: `=`}},
			}, {
				// Inside braces equality is a single '=' character but in terms of a token
				// it should be treated as ASSIGN.
				input:    `{=}`,
				expected: []Item{{Typ: LEFT_BRACE, Pos: 0, Val: `{`}, {Typ: EQL, Pos: 1, Val: `=`}, {Typ: RIGHT_BRACE, Pos: 2, Val: `}`}},
			}, {
				input:    `==`,
				expected: []Item{{Typ: EQLC, Pos: 0, Val: `==`}},
			}, {
				input:    `!=`,
				expected: []Item{{Typ: NEQ, Pos: 0, Val: `!=`}},
			}, {
				input:    `<`,
				expected: []Item{{Typ: LSS, Pos: 0, Val: `<`}},
			}, {
				input:    `>`,
				expected: []Item{{Typ: GTR, Pos: 0, Val: `>`}},
			}, {
				input:    `>=`,
				expected: []Item{{Typ: GTE, Pos: 0, Val: `>=`}},
			}, {
				input:    `<=`,
				expected: []Item{{Typ: LTE, Pos: 0, Val: `<=`}},
			}, {
				input:    `+`,
				expected: []Item{{Typ: ADD, Pos: 0, Val: `+`}},
			}, {
				input:    `-`,
				expected: []Item{{Typ: SUB, Pos: 0, Val: `-`}},
			}, {
				input:    `*`,
				expected: []Item{{Typ: MUL, Pos: 0, Val: `*`}},
			}, {
				input:    `/`,
				expected: []Item{{Typ: DIV, Pos: 0, Val: `/`}},
			}, {
				input:    `^`,
				expected: []Item{{Typ: POW, Pos: 0, Val: `^`}},
			}, {
				input:    `%`,
				expected: []Item{{Typ: MOD, Pos: 0, Val: `%`}},
			}, {
				input:    `AND`,
				expected: []Item{{Typ: LAND, Pos: 0, Val: `AND`}},
			}, {
				input:    `or`,
				expected: []Item{{Typ: LOR, Pos: 0, Val: `or`}},
			}, {
				input:    `unless`,
				expected: []Item{{Typ: LUNLESS, Pos: 0, Val: `unless`}},
			}, {
				input:    `@`,
				expected: []Item{{Typ: AT, Pos: 0, Val: `@`}},
			},
		},
	},
	{
		name: "aggregators",
		tests: []testCase{
			{
				input:    `sum`,
				expected: []Item{{Typ: SUM, Pos: 0, Val: `sum`}},
			}, {
				input:    `AVG`,
				expected: []Item{{Typ: AVG, Pos: 0, Val: `AVG`}},
			}, {
				input:    `GROUP`,
				expected: []Item{{Typ: GROUP, Pos: 0, Val: `GROUP`}},
			}, {
				input:    `MAX`,
				expected: []Item{{Typ: MAX, Pos: 0, Val: `MAX`}},
			}, {
				input:    `min`,
				expected: []Item{{Typ: MIN, Pos: 0, Val: `min`}},
			}, {
				input:    `count`,
				expected: []Item{{Typ: COUNT, Pos: 0, Val: `count`}},
			}, {
				input:    `stdvar`,
				expected: []Item{{Typ: STDVAR, Pos: 0, Val: `stdvar`}},
			}, {
				input:    `stddev`,
				expected: []Item{{Typ: STDDEV, Pos: 0, Val: `stddev`}},
			},
		},
	},
	{
		name: "keywords",
		tests: []testCase{
			{
				input:    "offset",
				expected: []Item{{Typ: OFFSET, Pos: 0, Val: "offset"}},
			},
			{
				input:    "by",
				expected: []Item{{Typ: BY, Pos: 0, Val: "by"}},
			},
			{
				input:    "without",
				expected: []Item{{Typ: WITHOUT, Pos: 0, Val: "without"}},
			},
			{
				input:    "on",
				expected: []Item{{Typ: ON, Pos: 0, Val: "on"}},
			},
			{
				input:    "ignoring",
				expected: []Item{{Typ: IGNORING, Pos: 0, Val: "ignoring"}},
			},
			{
				input:    "group_left",
				expected: []Item{{Typ: GROUP_LEFT, Pos: 0, Val: "group_left"}},
			},
			{
				input:    "group_right",
				expected: []Item{{Typ: GROUP_RIGHT, Pos: 0, Val: "group_right"}},
			},
			{
				input:    "bool",
				expected: []Item{{Typ: BOOL, Pos: 0, Val: "bool"}},
			},
			{
				input:    "atan2",
				expected: []Item{{Typ: ATAN2, Pos: 0, Val: "atan2"}},
			},
		},
	},
	{
		name: "preprocessors",
		tests: []testCase{
			{
				input:    `start`,
				expected: []Item{{Typ: START, Pos: 0, Val: `start`}},
			},
			{
				input:    `end`,
				expected: []Item{{Typ: END, Pos: 0, Val: `end`}},
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
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `foo`},
					{Typ: EQL, Pos: 4, Val: `=`},
					{Typ: STRING, Pos: 5, Val: `'bar'`},
					{Typ: RIGHT_BRACE, Pos: 10, Val: `}`},
				},
			}, {
				input: `{foo="bar"}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `foo`},
					{Typ: EQL, Pos: 4, Val: `=`},
					{Typ: STRING, Pos: 5, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 10, Val: `}`},
				},
			}, {
				input: `{foo="bar\"bar"}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `foo`},
					{Typ: EQL, Pos: 4, Val: `=`},
					{Typ: STRING, Pos: 5, Val: `"bar\"bar"`},
					{Typ: RIGHT_BRACE, Pos: 15, Val: `}`},
				},
			}, {
				input: `{NaN	!= "bar" }`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `NaN`},
					{Typ: NEQ, Pos: 5, Val: `!=`},
					{Typ: STRING, Pos: 8, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 14, Val: `}`},
				},
			}, {
				input: `{alert=~"bar" }`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `alert`},
					{Typ: EQL_REGEX, Pos: 6, Val: `=~`},
					{Typ: STRING, Pos: 8, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 14, Val: `}`},
				},
			}, {
				input: `{on!~"bar"}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: IDENTIFIER, Pos: 1, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 3, Val: `!~`},
					{Typ: STRING, Pos: 5, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 10, Val: `}`},
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
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: RIGHT_BRACE, Pos: 1, Val: `}`},
					{Typ: SPACE, Pos: 2, Val: ` `},
					{Typ: OPEN_HIST, Pos: 3, Val: `{{`},
					{Typ: BUCKETS_DESC, Pos: 5, Val: `buckets`},
					{Typ: COLON, Pos: 12, Val: `:`},
					{Typ: LEFT_BRACKET, Pos: 13, Val: `[`},
					{Typ: NUMBER, Pos: 14, Val: `5`},
					{Typ: RIGHT_BRACKET, Pos: 15, Val: `]`},
					{Typ: CLOSE_HIST, Pos: 16, Val: `}}`},
				},
				seriesDesc: true,
			},
			{
				input: `{} {{buckets: [5 10 7]}}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: RIGHT_BRACE, Pos: 1, Val: `}`},
					{Typ: SPACE, Pos: 2, Val: ` `},
					{Typ: OPEN_HIST, Pos: 3, Val: `{{`},
					{Typ: BUCKETS_DESC, Pos: 5, Val: `buckets`},
					{Typ: COLON, Pos: 12, Val: `:`},
					{Typ: SPACE, Pos: 13, Val: ` `},
					{Typ: LEFT_BRACKET, Pos: 14, Val: `[`},
					{Typ: NUMBER, Pos: 15, Val: `5`},
					{Typ: SPACE, Pos: 16, Val: ` `},
					{Typ: NUMBER, Pos: 17, Val: `10`},
					{Typ: SPACE, Pos: 19, Val: ` `},
					{Typ: NUMBER, Pos: 20, Val: `7`},
					{Typ: RIGHT_BRACKET, Pos: 21, Val: `]`},
					{Typ: CLOSE_HIST, Pos: 22, Val: `}}`},
				},
				seriesDesc: true,
			},
			{
				input: `{} {{buckets: [5 10 7] schema:1}}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: RIGHT_BRACE, Pos: 1, Val: `}`},
					{Typ: SPACE, Pos: 2, Val: ` `},
					{Typ: OPEN_HIST, Pos: 3, Val: `{{`},
					{Typ: BUCKETS_DESC, Pos: 5, Val: `buckets`},
					{Typ: COLON, Pos: 12, Val: `:`},
					{Typ: SPACE, Pos: 13, Val: ` `},
					{Typ: LEFT_BRACKET, Pos: 14, Val: `[`},
					{Typ: NUMBER, Pos: 15, Val: `5`},
					{Typ: SPACE, Pos: 16, Val: ` `},
					{Typ: NUMBER, Pos: 17, Val: `10`},
					{Typ: SPACE, Pos: 19, Val: ` `},
					{Typ: NUMBER, Pos: 20, Val: `7`},
					{Typ: RIGHT_BRACKET, Pos: 21, Val: `]`},
					{Typ: SPACE, Pos: 22, Val: ` `},
					{Typ: SCHEMA_DESC, Pos: 23, Val: `schema`},
					{Typ: COLON, Pos: 29, Val: `:`},
					{Typ: NUMBER, Pos: 30, Val: `1`},
					{Typ: CLOSE_HIST, Pos: 31, Val: `}}`},
				},
				seriesDesc: true,
			},
			{ // Series with sum as -Inf and count as NaN.
				input: `{} {{buckets: [5 10 7] sum:Inf count:NaN}}`,
				expected: []Item{
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: RIGHT_BRACE, Pos: 1, Val: `}`},
					{Typ: SPACE, Pos: 2, Val: ` `},
					{Typ: OPEN_HIST, Pos: 3, Val: `{{`},
					{Typ: BUCKETS_DESC, Pos: 5, Val: `buckets`},
					{Typ: COLON, Pos: 12, Val: `:`},
					{Typ: SPACE, Pos: 13, Val: ` `},
					{Typ: LEFT_BRACKET, Pos: 14, Val: `[`},
					{Typ: NUMBER, Pos: 15, Val: `5`},
					{Typ: SPACE, Pos: 16, Val: ` `},
					{Typ: NUMBER, Pos: 17, Val: `10`},
					{Typ: SPACE, Pos: 19, Val: ` `},
					{Typ: NUMBER, Pos: 20, Val: `7`},
					{Typ: RIGHT_BRACKET, Pos: 21, Val: `]`},
					{Typ: SPACE, Pos: 22, Val: ` `},
					{Typ: SUM_DESC, Pos: 23, Val: `sum`},
					{Typ: COLON, Pos: 26, Val: `:`},
					{Typ: NUMBER, Pos: 27, Val: `Inf`},
					{Typ: SPACE, Pos: 30, Val: ` `},
					{Typ: COUNT_DESC, Pos: 31, Val: `count`},
					{Typ: COLON, Pos: 36, Val: `:`},
					{Typ: NUMBER, Pos: 37, Val: `NaN`},
					{Typ: CLOSE_HIST, Pos: 40, Val: `}}`},
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
					{Typ: LEFT_BRACE, Pos: 0, Val: `{`},
					{Typ: RIGHT_BRACE, Pos: 1, Val: `}`},
					{Typ: SPACE, Pos: 2, Val: ` `},
					{Typ: BLANK, Pos: 3, Val: `_`},
					{Typ: SPACE, Pos: 4, Val: ` `},
					{Typ: NUMBER, Pos: 5, Val: `1`},
					{Typ: SPACE, Pos: 6, Val: ` `},
					{Typ: TIMES, Pos: 7, Val: `x`},
					{Typ: SPACE, Pos: 8, Val: ` `},
					{Typ: NUMBER, Pos: 9, Val: `.3`},
				},
				seriesDesc: true,
			},
			{
				input: `metric +Inf Inf NaN`,
				expected: []Item{
					{Typ: IDENTIFIER, Pos: 0, Val: `metric`},
					{Typ: SPACE, Pos: 6, Val: ` `},
					{Typ: ADD, Pos: 7, Val: `+`},
					{Typ: NUMBER, Pos: 8, Val: `Inf`},
					{Typ: SPACE, Pos: 11, Val: ` `},
					{Typ: NUMBER, Pos: 12, Val: `Inf`},
					{Typ: SPACE, Pos: 15, Val: ` `},
					{Typ: NUMBER, Pos: 16, Val: `NaN`},
				},
				seriesDesc: true,
			},
			{
				input: `metric 1+1x4`,
				expected: []Item{
					{Typ: IDENTIFIER, Pos: 0, Val: `metric`},
					{Typ: SPACE, Pos: 6, Val: ` `},
					{Typ: NUMBER, Pos: 7, Val: `1`},
					{Typ: ADD, Pos: 8, Val: `+`},
					{Typ: NUMBER, Pos: 9, Val: `1`},
					{Typ: TIMES, Pos: 10, Val: `x`},
					{Typ: NUMBER, Pos: 11, Val: `4`},
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
					{Typ: IDENTIFIER, Pos: 0, Val: `test_name`},
					{Typ: LEFT_BRACE, Pos: 9, Val: `{`},
					{Typ: IDENTIFIER, Pos: 10, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 12, Val: `!~`},
					{Typ: STRING, Pos: 14, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 19, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 20, Val: `[`},
					{Typ: DURATION, Pos: 21, Val: `4m`},
					{Typ: COLON, Pos: 23, Val: `:`},
					{Typ: DURATION, Pos: 24, Val: `4s`},
					{Typ: RIGHT_BRACKET, Pos: 26, Val: `]`},
				},
			},
			{
				input: `test:name{on!~"bar"}[4m:4s]`,
				expected: []Item{
					{Typ: METRIC_IDENTIFIER, Pos: 0, Val: `test:name`},
					{Typ: LEFT_BRACE, Pos: 9, Val: `{`},
					{Typ: IDENTIFIER, Pos: 10, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 12, Val: `!~`},
					{Typ: STRING, Pos: 14, Val: `"bar"`},
					{Typ: RIGHT_BRACE, Pos: 19, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 20, Val: `[`},
					{Typ: DURATION, Pos: 21, Val: `4m`},
					{Typ: COLON, Pos: 23, Val: `:`},
					{Typ: DURATION, Pos: 24, Val: `4s`},
					{Typ: RIGHT_BRACKET, Pos: 26, Val: `]`},
				},
			},
			{
				input: `test:name{on!~"b:ar"}[4m:4s]`,
				expected: []Item{
					{Typ: METRIC_IDENTIFIER, Pos: 0, Val: `test:name`},
					{Typ: LEFT_BRACE, Pos: 9, Val: `{`},
					{Typ: IDENTIFIER, Pos: 10, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 12, Val: `!~`},
					{Typ: STRING, Pos: 14, Val: `"b:ar"`},
					{Typ: RIGHT_BRACE, Pos: 20, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 21, Val: `[`},
					{Typ: DURATION, Pos: 22, Val: `4m`},
					{Typ: COLON, Pos: 24, Val: `:`},
					{Typ: DURATION, Pos: 25, Val: `4s`},
					{Typ: RIGHT_BRACKET, Pos: 27, Val: `]`},
				},
			},
			{
				input: `test:name{on!~"b:ar"}[4m:]`,
				expected: []Item{
					{Typ: METRIC_IDENTIFIER, Pos: 0, Val: `test:name`},
					{Typ: LEFT_BRACE, Pos: 9, Val: `{`},
					{Typ: IDENTIFIER, Pos: 10, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 12, Val: `!~`},
					{Typ: STRING, Pos: 14, Val: `"b:ar"`},
					{Typ: RIGHT_BRACE, Pos: 20, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 21, Val: `[`},
					{Typ: DURATION, Pos: 22, Val: `4m`},
					{Typ: COLON, Pos: 24, Val: `:`},
					{Typ: RIGHT_BRACKET, Pos: 25, Val: `]`},
				},
			},
			{ // Nested Subquery.
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
				expected: []Item{
					{Typ: IDENTIFIER, Pos: 0, Val: `min_over_time`},
					{Typ: LEFT_PAREN, Pos: 13, Val: `(`},
					{Typ: IDENTIFIER, Pos: 14, Val: `rate`},
					{Typ: LEFT_PAREN, Pos: 18, Val: `(`},
					{Typ: IDENTIFIER, Pos: 19, Val: `foo`},
					{Typ: LEFT_BRACE, Pos: 22, Val: `{`},
					{Typ: IDENTIFIER, Pos: 23, Val: `bar`},
					{Typ: EQL, Pos: 26, Val: `=`},
					{Typ: STRING, Pos: 27, Val: `"baz"`},
					{Typ: RIGHT_BRACE, Pos: 32, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 33, Val: `[`},
					{Typ: DURATION, Pos: 34, Val: `2s`},
					{Typ: RIGHT_BRACKET, Pos: 36, Val: `]`},
					{Typ: RIGHT_PAREN, Pos: 37, Val: `)`},
					{Typ: LEFT_BRACKET, Pos: 38, Val: `[`},
					{Typ: DURATION, Pos: 39, Val: `5m`},
					{Typ: COLON, Pos: 41, Val: `:`},
					{Typ: RIGHT_BRACKET, Pos: 42, Val: `]`},
					{Typ: RIGHT_PAREN, Pos: 43, Val: `)`},
					{Typ: LEFT_BRACKET, Pos: 44, Val: `[`},
					{Typ: DURATION, Pos: 45, Val: `4m`},
					{Typ: COLON, Pos: 47, Val: `:`},
					{Typ: DURATION, Pos: 48, Val: `3s`},
					{Typ: RIGHT_BRACKET, Pos: 50, Val: `]`},
				},
			},
			// Subquery with offset.
			{
				input: `test:name{on!~"b:ar"}[4m:4s] offset 10m`,
				expected: []Item{
					{Typ: METRIC_IDENTIFIER, Pos: 0, Val: `test:name`},
					{Typ: LEFT_BRACE, Pos: 9, Val: `{`},
					{Typ: IDENTIFIER, Pos: 10, Val: `on`},
					{Typ: NEQ_REGEX, Pos: 12, Val: `!~`},
					{Typ: STRING, Pos: 14, Val: `"b:ar"`},
					{Typ: RIGHT_BRACE, Pos: 20, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 21, Val: `[`},
					{Typ: DURATION, Pos: 22, Val: `4m`},
					{Typ: COLON, Pos: 24, Val: `:`},
					{Typ: DURATION, Pos: 25, Val: `4s`},
					{Typ: RIGHT_BRACKET, Pos: 27, Val: `]`},
					{Typ: OFFSET, Pos: 29, Val: "offset"},
					{Typ: DURATION, Pos: 36, Val: "10m"},
				},
			},
			{
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 6m)[4m:3s]`,
				expected: []Item{
					{Typ: IDENTIFIER, Pos: 0, Val: `min_over_time`},
					{Typ: LEFT_PAREN, Pos: 13, Val: `(`},
					{Typ: IDENTIFIER, Pos: 14, Val: `rate`},
					{Typ: LEFT_PAREN, Pos: 18, Val: `(`},
					{Typ: IDENTIFIER, Pos: 19, Val: `foo`},
					{Typ: LEFT_BRACE, Pos: 22, Val: `{`},
					{Typ: IDENTIFIER, Pos: 23, Val: `bar`},
					{Typ: EQL, Pos: 26, Val: `=`},
					{Typ: STRING, Pos: 27, Val: `"baz"`},
					{Typ: RIGHT_BRACE, Pos: 32, Val: `}`},
					{Typ: LEFT_BRACKET, Pos: 33, Val: `[`},
					{Typ: DURATION, Pos: 34, Val: `2s`},
					{Typ: RIGHT_BRACKET, Pos: 36, Val: `]`},
					{Typ: RIGHT_PAREN, Pos: 37, Val: `)`},
					{Typ: LEFT_BRACKET, Pos: 38, Val: `[`},
					{Typ: DURATION, Pos: 39, Val: `5m`},
					{Typ: COLON, Pos: 41, Val: `:`},
					{Typ: RIGHT_BRACKET, Pos: 42, Val: `]`},
					{Typ: OFFSET, Pos: 44, Val: `offset`},
					{Typ: DURATION, Pos: 51, Val: `6m`},
					{Typ: RIGHT_PAREN, Pos: 53, Val: `)`},
					{Typ: LEFT_BRACKET, Pos: 54, Val: `[`},
					{Typ: DURATION, Pos: 55, Val: `4m`},
					{Typ: COLON, Pos: 57, Val: `:`},
					{Typ: DURATION, Pos: 58, Val: `3s`},
					{Typ: RIGHT_BRACKET, Pos: 60, Val: `]`},
				},
			},
			{
				input: `test:name[ 5m]`,
				expected: []Item{
					{Typ: METRIC_IDENTIFIER, Pos: 0, Val: `test:name`},
					{Typ: LEFT_BRACKET, Pos: 9, Val: `[`},
					{Typ: DURATION, Pos: 11, Val: `5m`},
					{Typ: RIGHT_BRACKET, Pos: 13, Val: `]`},
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

				eofItem := Item{Typ: EOF, Pos: posrange.Pos(len(test.input)), Val: ""}
				require.Equal(t, lastItem, eofItem, "%d: input %q", i, test.input)

				out = out[:len(out)-1]
				require.Equal(t, test.expected, out, "%d: input %q", i, test.input)
			}
		})
	}
}
