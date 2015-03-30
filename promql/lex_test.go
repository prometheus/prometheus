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
	"reflect"
	"testing"
)

var tests = []struct {
	input    string
	expected []item
	fail     bool
}{
	// Test common stuff.
	{
		input:    ",",
		expected: []item{{itemComma, 0, ","}},
	}, {
		input:    "()",
		expected: []item{{itemLeftParen, 0, `(`}, {itemRightParen, 1, `)`}},
	}, {
		input:    "{}",
		expected: []item{{itemLeftBrace, 0, `{`}, {itemRightBrace, 1, `}`}},
	}, {
		input: "[5m]",
		expected: []item{
			{itemLeftBracket, 0, `[`},
			{itemDuration, 1, `5m`},
			{itemRightBracket, 3, `]`},
		},
	},
	// Test numbers.
	{
		input:    "1",
		expected: []item{{itemNumber, 0, "1"}},
	}, {
		input:    "4.23",
		expected: []item{{itemNumber, 0, "4.23"}},
	}, {
		input:    ".3",
		expected: []item{{itemNumber, 0, ".3"}},
	}, {
		input:    "5.",
		expected: []item{{itemNumber, 0, "5."}},
	}, {
		input:    "NaN",
		expected: []item{{itemNumber, 0, "NaN"}},
	}, {
		input:    "nAN",
		expected: []item{{itemNumber, 0, "nAN"}},
	}, {
		input:    "NaN 123",
		expected: []item{{itemNumber, 0, "NaN"}, {itemNumber, 4, "123"}},
	}, {
		input:    "NaN123",
		expected: []item{{itemIdentifier, 0, "NaN123"}},
	}, {
		input:    "iNf",
		expected: []item{{itemNumber, 0, "iNf"}},
	}, {
		input:    "Inf",
		expected: []item{{itemNumber, 0, "Inf"}},
	}, {
		input:    "+Inf",
		expected: []item{{itemADD, 0, "+"}, {itemNumber, 1, "Inf"}},
	}, {
		input:    "+Inf 123",
		expected: []item{{itemADD, 0, "+"}, {itemNumber, 1, "Inf"}, {itemNumber, 5, "123"}},
	}, {
		input:    "-Inf",
		expected: []item{{itemSUB, 0, "-"}, {itemNumber, 1, "Inf"}},
	}, {
		input:    "Infoo",
		expected: []item{{itemIdentifier, 0, "Infoo"}},
	}, {
		input:    "-Infoo",
		expected: []item{{itemSUB, 0, "-"}, {itemIdentifier, 1, "Infoo"}},
	}, {
		input:    "-Inf 123",
		expected: []item{{itemSUB, 0, "-"}, {itemNumber, 1, "Inf"}, {itemNumber, 5, "123"}},
	}, {
		input:    "0x123",
		expected: []item{{itemNumber, 0, "0x123"}},
	},
	// Test duration.
	{
		input:    "5s",
		expected: []item{{itemDuration, 0, "5s"}},
	}, {
		input:    "123m",
		expected: []item{{itemDuration, 0, "123m"}},
	}, {
		input:    "1h",
		expected: []item{{itemDuration, 0, "1h"}},
	}, {
		input:    "3w",
		expected: []item{{itemDuration, 0, "3w"}},
	}, {
		input:    "1y",
		expected: []item{{itemDuration, 0, "1y"}},
	},
	// Test identifiers.
	{
		input:    "abc",
		expected: []item{{itemIdentifier, 0, "abc"}},
	}, {
		input:    "a:bc",
		expected: []item{{itemMetricIdentifier, 0, "a:bc"}},
	}, {
		input:    "abc d",
		expected: []item{{itemIdentifier, 0, "abc"}, {itemIdentifier, 4, "d"}},
	},
	// Test comments.
	{
		input:    "# some comment",
		expected: []item{{itemComment, 0, "# some comment"}},
	}, {
		input: "5 # 1+1\n5",
		expected: []item{
			{itemNumber, 0, "5"},
			{itemComment, 2, "# 1+1"},
			{itemNumber, 8, "5"},
		},
	},
	// Test operators.
	{
		input:    `=`,
		expected: []item{{itemAssign, 0, `=`}},
	}, {
		// Inside braces equality is a single '=' character.
		input:    `{=}`,
		expected: []item{{itemLeftBrace, 0, `{`}, {itemEQL, 1, `=`}, {itemRightBrace, 2, `}`}},
	}, {
		input:    `==`,
		expected: []item{{itemEQL, 0, `==`}},
	}, {
		input:    `!=`,
		expected: []item{{itemNEQ, 0, `!=`}},
	}, {
		input:    `<`,
		expected: []item{{itemLSS, 0, `<`}},
	}, {
		input:    `>`,
		expected: []item{{itemGTR, 0, `>`}},
	}, {
		input:    `>=`,
		expected: []item{{itemGTE, 0, `>=`}},
	}, {
		input:    `<=`,
		expected: []item{{itemLTE, 0, `<=`}},
	}, {
		input:    `+`,
		expected: []item{{itemADD, 0, `+`}},
	}, {
		input:    `-`,
		expected: []item{{itemSUB, 0, `-`}},
	}, {
		input:    `*`,
		expected: []item{{itemMUL, 0, `*`}},
	}, {
		input:    `/`,
		expected: []item{{itemDIV, 0, `/`}},
	}, {
		input:    `%`,
		expected: []item{{itemMOD, 0, `%`}},
	}, {
		input:    `AND`,
		expected: []item{{itemLAND, 0, `AND`}},
	}, {
		input:    `or`,
		expected: []item{{itemLOR, 0, `or`}},
	},
	// Test aggregators.
	{
		input:    `sum`,
		expected: []item{{itemSum, 0, `sum`}},
	}, {
		input:    `AVG`,
		expected: []item{{itemAvg, 0, `AVG`}},
	}, {
		input:    `MAX`,
		expected: []item{{itemMax, 0, `MAX`}},
	}, {
		input:    `min`,
		expected: []item{{itemMin, 0, `min`}},
	}, {
		input:    `count`,
		expected: []item{{itemCount, 0, `count`}},
	}, {
		input:    `stdvar`,
		expected: []item{{itemStdvar, 0, `stdvar`}},
	}, {
		input:    `stddev`,
		expected: []item{{itemStddev, 0, `stddev`}},
	},
	// Test keywords.
	{
		input:    "alert",
		expected: []item{{itemAlert, 0, "alert"}},
	}, {
		input:    "keeping_extra",
		expected: []item{{itemKeepingExtra, 0, "keeping_extra"}},
	}, {
		input:    "if",
		expected: []item{{itemIf, 0, "if"}},
	}, {
		input:    "for",
		expected: []item{{itemFor, 0, "for"}},
	}, {
		input:    "with",
		expected: []item{{itemWith, 0, "with"}},
	}, {
		input:    "description",
		expected: []item{{itemDescription, 0, "description"}},
	}, {
		input:    "summary",
		expected: []item{{itemSummary, 0, "summary"}},
	}, {
		input:    "offset",
		expected: []item{{itemOffset, 0, "offset"}},
	}, {
		input:    "by",
		expected: []item{{itemBy, 0, "by"}},
	}, {
		input:    "on",
		expected: []item{{itemOn, 0, "on"}},
	}, {
		input:    "group_left",
		expected: []item{{itemGroupLeft, 0, "group_left"}},
	}, {
		input:    "group_right",
		expected: []item{{itemGroupRight, 0, "group_right"}},
	},
	// Test Selector.
	{
		input: `{foo="bar"}`,
		expected: []item{
			{itemLeftBrace, 0, `{`},
			{itemIdentifier, 1, `foo`},
			{itemEQL, 4, `=`},
			{itemString, 5, `"bar"`},
			{itemRightBrace, 10, `}`},
		},
	}, {
		input: `{NaN	!= "bar" }`,
		expected: []item{
			{itemLeftBrace, 0, `{`},
			{itemIdentifier, 1, `NaN`},
			{itemNEQ, 5, `!=`},
			{itemString, 8, `"bar"`},
			{itemRightBrace, 14, `}`},
		},
	}, {
		input: `{alert=~"bar" }`,
		expected: []item{
			{itemLeftBrace, 0, `{`},
			{itemIdentifier, 1, `alert`},
			{itemEQLRegex, 6, `=~`},
			{itemString, 8, `"bar"`},
			{itemRightBrace, 14, `}`},
		},
	}, {
		input: `{on!~"bar"}`,
		expected: []item{
			{itemLeftBrace, 0, `{`},
			{itemIdentifier, 1, `on`},
			{itemNEQRegex, 3, `!~`},
			{itemString, 5, `"bar"`},
			{itemRightBrace, 10, `}`},
		},
	}, {
		input: `{alert!#"bar"}`, fail: true,
	}, {
		input: `{foo:a="bar"}`, fail: true,
	},
	// Test common errors.
	{
		input: `=~`, fail: true,
	}, {
		input: `!~`, fail: true,
	}, {
		input: `!(`, fail: true,
	}, {
		input: "1a", fail: true,
	},
	// Test mismatched parens.
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
}

// TestLexer tests basic functionality of the lexer. More elaborate tests are implemented
// for the parser to avoid duplicated effort.
func TestLexer(t *testing.T) {
	for i, test := range tests {
		tn := fmt.Sprintf("test.%d \"%s\"", i, test.input)
		l := lex(tn, test.input)

		out := []item{}
		for it := range l.items {
			out = append(out, it)
		}

		lastItem := out[len(out)-1]
		if test.fail {
			if lastItem.typ != itemError {
				t.Fatalf("%s: expected lexing error but did not fail", tn)
			}
			continue
		}
		if lastItem.typ == itemError {
			t.Fatalf("%s: unexpected lexing error: %s", tn, lastItem)
		}

		if !reflect.DeepEqual(lastItem, item{itemEOF, Pos(len(test.input)), ""}) {
			t.Fatalf("%s: lexing error: expected output to end with EOF item", tn)
		}
		out = out[:len(out)-1]
		if !reflect.DeepEqual(out, test.expected) {
			t.Errorf("%s: lexing mismatch:\nexpected: %#v\n-----\ngot: %#v", tn, test.expected, out)
		}
	}
}
