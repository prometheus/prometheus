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
				expected: []item{{ItemComma, 0, ","}},
			}, {
				input:    "()",
				expected: []item{{ItemLeftParen, 0, `(`}, {ItemRightParen, 1, `)`}},
			}, {
				input:    "{}",
				expected: []item{{ItemLeftBrace, 0, `{`}, {ItemRightBrace, 1, `}`}},
			}, {
				input: "[5m]",
				expected: []item{
					{ItemLeftBracket, 0, `[`},
					{ItemDuration, 1, `5m`},
					{ItemRightBracket, 3, `]`},
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
				expected: []item{{ItemNumber, 0, "1"}},
			}, {
				input:    "4.23",
				expected: []item{{ItemNumber, 0, "4.23"}},
			}, {
				input:    ".3",
				expected: []item{{ItemNumber, 0, ".3"}},
			}, {
				input:    "5.",
				expected: []item{{ItemNumber, 0, "5."}},
			}, {
				input:    "NaN",
				expected: []item{{ItemNumber, 0, "NaN"}},
			}, {
				input:    "nAN",
				expected: []item{{ItemNumber, 0, "nAN"}},
			}, {
				input:    "NaN 123",
				expected: []item{{ItemNumber, 0, "NaN"}, {ItemNumber, 4, "123"}},
			}, {
				input:    "NaN123",
				expected: []item{{ItemIdentifier, 0, "NaN123"}},
			}, {
				input:    "iNf",
				expected: []item{{ItemNumber, 0, "iNf"}},
			}, {
				input:    "Inf",
				expected: []item{{ItemNumber, 0, "Inf"}},
			}, {
				input:    "+Inf",
				expected: []item{{ItemADD, 0, "+"}, {ItemNumber, 1, "Inf"}},
			}, {
				input:    "+Inf 123",
				expected: []item{{ItemADD, 0, "+"}, {ItemNumber, 1, "Inf"}, {ItemNumber, 5, "123"}},
			}, {
				input:    "-Inf",
				expected: []item{{ItemSUB, 0, "-"}, {ItemNumber, 1, "Inf"}},
			}, {
				input:    "Infoo",
				expected: []item{{ItemIdentifier, 0, "Infoo"}},
			}, {
				input:    "-Infoo",
				expected: []item{{ItemSUB, 0, "-"}, {ItemIdentifier, 1, "Infoo"}},
			}, {
				input:    "-Inf 123",
				expected: []item{{ItemSUB, 0, "-"}, {ItemNumber, 1, "Inf"}, {ItemNumber, 5, "123"}},
			}, {
				input:    "0x123",
				expected: []item{{ItemNumber, 0, "0x123"}},
			},
		},
	},
	{
		name: "strings",
		tests: []testCase{
			{
				input:    "\"test\\tsequence\"",
				expected: []item{{ItemString, 0, `"test\tsequence"`}},
			},
			{
				input:    "\"test\\\\.expression\"",
				expected: []item{{ItemString, 0, `"test\\.expression"`}},
			},
			{
				input: "\"test\\.expression\"",
				expected: []item{
					{ItemError, 0, "unknown escape sequence U+002E '.'"},
					{ItemString, 0, `"test\.expression"`},
				},
			},
			{
				input:    "`test\\.expression`",
				expected: []item{{ItemString, 0, "`test\\.expression`"}},
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
				expected: []item{{ItemDuration, 0, "5s"}},
			}, {
				input:    "123m",
				expected: []item{{ItemDuration, 0, "123m"}},
			}, {
				input:    "1h",
				expected: []item{{ItemDuration, 0, "1h"}},
			}, {
				input:    "3w",
				expected: []item{{ItemDuration, 0, "3w"}},
			}, {
				input:    "1y",
				expected: []item{{ItemDuration, 0, "1y"}},
			},
		},
	},
	{
		name: "identifiers",
		tests: []testCase{
			{
				input:    "abc",
				expected: []item{{ItemIdentifier, 0, "abc"}},
			}, {
				input:    "a:bc",
				expected: []item{{ItemMetricIdentifier, 0, "a:bc"}},
			}, {
				input:    "abc d",
				expected: []item{{ItemIdentifier, 0, "abc"}, {ItemIdentifier, 4, "d"}},
			}, {
				input:    ":bc",
				expected: []item{{ItemMetricIdentifier, 0, ":bc"}},
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
				expected: []item{{ItemComment, 0, "# some comment"}},
			}, {
				input: "5 # 1+1\n5",
				expected: []item{
					{ItemNumber, 0, "5"},
					{ItemComment, 2, "# 1+1"},
					{ItemNumber, 8, "5"},
				},
			},
		},
	},
	{
		name: "operators",
		tests: []testCase{
			{
				input:    `=`,
				expected: []item{{ItemAssign, 0, `=`}},
			}, {
				// Inside braces equality is a single '=' character.
				input:    `{=}`,
				expected: []item{{ItemLeftBrace, 0, `{`}, {ItemEQL, 1, `=`}, {ItemRightBrace, 2, `}`}},
			}, {
				input:    `==`,
				expected: []item{{ItemEQL, 0, `==`}},
			}, {
				input:    `!=`,
				expected: []item{{ItemNEQ, 0, `!=`}},
			}, {
				input:    `<`,
				expected: []item{{ItemLSS, 0, `<`}},
			}, {
				input:    `>`,
				expected: []item{{ItemGTR, 0, `>`}},
			}, {
				input:    `>=`,
				expected: []item{{ItemGTE, 0, `>=`}},
			}, {
				input:    `<=`,
				expected: []item{{ItemLTE, 0, `<=`}},
			}, {
				input:    `+`,
				expected: []item{{ItemADD, 0, `+`}},
			}, {
				input:    `-`,
				expected: []item{{ItemSUB, 0, `-`}},
			}, {
				input:    `*`,
				expected: []item{{ItemMUL, 0, `*`}},
			}, {
				input:    `/`,
				expected: []item{{ItemDIV, 0, `/`}},
			}, {
				input:    `^`,
				expected: []item{{ItemPOW, 0, `^`}},
			}, {
				input:    `%`,
				expected: []item{{ItemMOD, 0, `%`}},
			}, {
				input:    `AND`,
				expected: []item{{ItemLAND, 0, `AND`}},
			}, {
				input:    `or`,
				expected: []item{{ItemLOR, 0, `or`}},
			}, {
				input:    `unless`,
				expected: []item{{ItemLUnless, 0, `unless`}},
			},
		},
	},
	{
		name: "aggregators",
		tests: []testCase{
			{
				input:    `sum`,
				expected: []item{{ItemSum, 0, `sum`}},
			}, {
				input:    `AVG`,
				expected: []item{{ItemAvg, 0, `AVG`}},
			}, {
				input:    `MAX`,
				expected: []item{{ItemMax, 0, `MAX`}},
			}, {
				input:    `min`,
				expected: []item{{ItemMin, 0, `min`}},
			}, {
				input:    `count`,
				expected: []item{{ItemCount, 0, `count`}},
			}, {
				input:    `stdvar`,
				expected: []item{{ItemStdvar, 0, `stdvar`}},
			}, {
				input:    `stddev`,
				expected: []item{{ItemStddev, 0, `stddev`}},
			},
		},
	},
	{
		name: "keywords",
		tests: []testCase{
			{
				input:    "offset",
				expected: []item{{ItemOffset, 0, "offset"}},
			}, {
				input:    "by",
				expected: []item{{ItemBy, 0, "by"}},
			}, {
				input:    "without",
				expected: []item{{ItemWithout, 0, "without"}},
			}, {
				input:    "on",
				expected: []item{{ItemOn, 0, "on"}},
			}, {
				input:    "ignoring",
				expected: []item{{ItemIgnoring, 0, "ignoring"}},
			}, {
				input:    "group_left",
				expected: []item{{ItemGroupLeft, 0, "group_left"}},
			}, {
				input:    "group_right",
				expected: []item{{ItemGroupRight, 0, "group_right"}},
			}, {
				input:    "bool",
				expected: []item{{ItemBool, 0, "bool"}},
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
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `foo`},
					{ItemEQL, 4, `=`},
					{ItemString, 5, `'bar'`},
					{ItemRightBrace, 10, `}`},
				},
			}, {
				input: `{foo="bar"}`,
				expected: []item{
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `foo`},
					{ItemEQL, 4, `=`},
					{ItemString, 5, `"bar"`},
					{ItemRightBrace, 10, `}`},
				},
			}, {
				input: `{foo="bar\"bar"}`,
				expected: []item{
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `foo`},
					{ItemEQL, 4, `=`},
					{ItemString, 5, `"bar\"bar"`},
					{ItemRightBrace, 15, `}`},
				},
			}, {
				input: `{NaN	!= "bar" }`,
				expected: []item{
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `NaN`},
					{ItemNEQ, 5, `!=`},
					{ItemString, 8, `"bar"`},
					{ItemRightBrace, 14, `}`},
				},
			}, {
				input: `{alert=~"bar" }`,
				expected: []item{
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `alert`},
					{ItemEQLRegex, 6, `=~`},
					{ItemString, 8, `"bar"`},
					{ItemRightBrace, 14, `}`},
				},
			}, {
				input: `{on!~"bar"}`,
				expected: []item{
					{ItemLeftBrace, 0, `{`},
					{ItemIdentifier, 1, `on`},
					{ItemNEQRegex, 3, `!~`},
					{ItemString, 5, `"bar"`},
					{ItemRightBrace, 10, `}`},
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
					{ItemLeftBrace, 0, `{`},
					{ItemRightBrace, 1, `}`},
					{ItemSpace, 2, ` `},
					{ItemBlank, 3, `_`},
					{ItemSpace, 4, ` `},
					{ItemNumber, 5, `1`},
					{ItemSpace, 6, ` `},
					{ItemTimes, 7, `x`},
					{ItemSpace, 8, ` `},
					{ItemNumber, 9, `.3`},
				},
				seriesDesc: true,
			},
			{
				input: `metric +Inf Inf NaN`,
				expected: []item{
					{ItemIdentifier, 0, `metric`},
					{ItemSpace, 6, ` `},
					{ItemADD, 7, `+`},
					{ItemNumber, 8, `Inf`},
					{ItemSpace, 11, ` `},
					{ItemNumber, 12, `Inf`},
					{ItemSpace, 15, ` `},
					{ItemNumber, 16, `NaN`},
				},
				seriesDesc: true,
			},
			{
				input: `metric 1+1x4`,
				expected: []item{
					{ItemIdentifier, 0, `metric`},
					{ItemSpace, 6, ` `},
					{ItemNumber, 7, `1`},
					{ItemADD, 8, `+`},
					{ItemNumber, 9, `1`},
					{ItemTimes, 10, `x`},
					{ItemNumber, 11, `4`},
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
					{ItemIdentifier, 0, `test_name`},
					{ItemLeftBrace, 9, `{`},
					{ItemIdentifier, 10, `on`},
					{ItemNEQRegex, 12, `!~`},
					{ItemString, 14, `"bar"`},
					{ItemRightBrace, 19, `}`},
					{ItemLeftBracket, 20, `[`},
					{ItemDuration, 21, `4m`},
					{ItemColon, 23, `:`},
					{ItemDuration, 24, `4s`},
					{ItemRightBracket, 26, `]`},
				},
			},
			{
				input: `test:name{on!~"bar"}[4m:4s]`,
				expected: []item{
					{ItemMetricIdentifier, 0, `test:name`},
					{ItemLeftBrace, 9, `{`},
					{ItemIdentifier, 10, `on`},
					{ItemNEQRegex, 12, `!~`},
					{ItemString, 14, `"bar"`},
					{ItemRightBrace, 19, `}`},
					{ItemLeftBracket, 20, `[`},
					{ItemDuration, 21, `4m`},
					{ItemColon, 23, `:`},
					{ItemDuration, 24, `4s`},
					{ItemRightBracket, 26, `]`},
				},
			}, {
				input: `test:name{on!~"b:ar"}[4m:4s]`,
				expected: []item{
					{ItemMetricIdentifier, 0, `test:name`},
					{ItemLeftBrace, 9, `{`},
					{ItemIdentifier, 10, `on`},
					{ItemNEQRegex, 12, `!~`},
					{ItemString, 14, `"b:ar"`},
					{ItemRightBrace, 20, `}`},
					{ItemLeftBracket, 21, `[`},
					{ItemDuration, 22, `4m`},
					{ItemColon, 24, `:`},
					{ItemDuration, 25, `4s`},
					{ItemRightBracket, 27, `]`},
				},
			}, {
				input: `test:name{on!~"b:ar"}[4m:]`,
				expected: []item{
					{ItemMetricIdentifier, 0, `test:name`},
					{ItemLeftBrace, 9, `{`},
					{ItemIdentifier, 10, `on`},
					{ItemNEQRegex, 12, `!~`},
					{ItemString, 14, `"b:ar"`},
					{ItemRightBrace, 20, `}`},
					{ItemLeftBracket, 21, `[`},
					{ItemDuration, 22, `4m`},
					{ItemColon, 24, `:`},
					{ItemRightBracket, 25, `]`},
				},
			}, { // Nested Subquery.
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
				expected: []item{

					{ItemIdentifier, 0, `min_over_time`},
					{ItemLeftParen, 13, `(`},
					{ItemIdentifier, 14, `rate`},
					{ItemLeftParen, 18, `(`},
					{ItemIdentifier, 19, `foo`},
					{ItemLeftBrace, 22, `{`},
					{ItemIdentifier, 23, `bar`},
					{ItemEQL, 26, `=`},
					{ItemString, 27, `"baz"`},
					{ItemRightBrace, 32, `}`},
					{ItemLeftBracket, 33, `[`},
					{ItemDuration, 34, `2s`},
					{ItemRightBracket, 36, `]`},
					{ItemRightParen, 37, `)`},
					{ItemLeftBracket, 38, `[`},
					{ItemDuration, 39, `5m`},
					{ItemColon, 41, `:`},
					{ItemRightBracket, 42, `]`},
					{ItemRightParen, 43, `)`},
					{ItemLeftBracket, 44, `[`},
					{ItemDuration, 45, `4m`},
					{ItemColon, 47, `:`},
					{ItemDuration, 48, `3s`},
					{ItemRightBracket, 50, `]`},
				},
			},
			// Subquery with offset.
			{
				input: `test:name{on!~"b:ar"}[4m:4s] offset 10m`,
				expected: []item{
					{ItemMetricIdentifier, 0, `test:name`},
					{ItemLeftBrace, 9, `{`},
					{ItemIdentifier, 10, `on`},
					{ItemNEQRegex, 12, `!~`},
					{ItemString, 14, `"b:ar"`},
					{ItemRightBrace, 20, `}`},
					{ItemLeftBracket, 21, `[`},
					{ItemDuration, 22, `4m`},
					{ItemColon, 24, `:`},
					{ItemDuration, 25, `4s`},
					{ItemRightBracket, 27, `]`},
					{ItemOffset, 29, "offset"},
					{ItemDuration, 36, "10m"},
				},
			}, {
				input: `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 6m)[4m:3s]`,
				expected: []item{

					{ItemIdentifier, 0, `min_over_time`},
					{ItemLeftParen, 13, `(`},
					{ItemIdentifier, 14, `rate`},
					{ItemLeftParen, 18, `(`},
					{ItemIdentifier, 19, `foo`},
					{ItemLeftBrace, 22, `{`},
					{ItemIdentifier, 23, `bar`},
					{ItemEQL, 26, `=`},
					{ItemString, 27, `"baz"`},
					{ItemRightBrace, 32, `}`},
					{ItemLeftBracket, 33, `[`},
					{ItemDuration, 34, `2s`},
					{ItemRightBracket, 36, `]`},
					{ItemRightParen, 37, `)`},
					{ItemLeftBracket, 38, `[`},
					{ItemDuration, 39, `5m`},
					{ItemColon, 41, `:`},
					{ItemRightBracket, 42, `]`},
					{ItemOffset, 44, `offset`},
					{ItemDuration, 51, `6m`},
					{ItemRightParen, 53, `)`},
					{ItemLeftBracket, 54, `[`},
					{ItemDuration, 55, `4m`},
					{ItemColon, 57, `:`},
					{ItemDuration, 58, `3s`},
					{ItemRightBracket, 60, `]`},
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
					items:      make(chan item),
					seriesDesc: test.seriesDesc,
				}
				go l.run()

				out := []item{}
				for it := range l.items {
					out = append(out, it)
				}

				lastItem := out[len(out)-1]
				if test.fail {
					if lastItem.typ != ItemError {
						t.Logf("%d: input %q", i, test.input)
						t.Fatalf("expected lexing error but did not fail")
					}
					continue
				}
				if lastItem.typ == ItemError {
					t.Logf("%d: input %q", i, test.input)
					t.Fatalf("unexpected lexing error at position %d: %s", lastItem.pos, lastItem)
				}

				eofItem := item{ItemEOF, Pos(len(test.input)), ""}
				testutil.Equals(t, lastItem, eofItem, "%d: input %q", i, test.input)

				out = out[:len(out)-1]
				testutil.Equals(t, out, test.expected, "%d: input %q", i, test.input)
			}
		})
	}
}
