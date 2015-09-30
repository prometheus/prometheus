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

package strutil

import (
	"testing"
)

type quoteTest struct {
	in    string
	out   string
	ascii string
}

var quotetests = []quoteTest{
	{"\a\b\f\r\n\t\v", `"\a\b\f\r\n\t\v"`, `"\a\b\f\r\n\t\v"`},
	{"\\", `"\\"`, `"\\"`},
	{"abc\xffdef", `"abc\xffdef"`, `"abc\xffdef"`},
	{"\u263a", `"☺"`, `"\u263a"`},
	{"\U0010ffff", `"\U0010ffff"`, `"\U0010ffff"`},
	{"\x04", `"\x04"`, `"\x04"`},
}

type unQuoteTest struct {
	in  string
	out string
}

var unquotetests = []unQuoteTest{
	{`""`, ""},
	{`"a"`, "a"},
	{`"abc"`, "abc"},
	{`"☺"`, "☺"},
	{`"hello world"`, "hello world"},
	{`"\xFF"`, "\xFF"},
	{`"\377"`, "\377"},
	{`"\u1234"`, "\u1234"},
	{`"\U00010111"`, "\U00010111"},
	{`"\U0001011111"`, "\U0001011111"},
	{`"\a\b\f\n\r\t\v\\\""`, "\a\b\f\n\r\t\v\\\""},
	{`"'"`, "'"},

	{`''`, ""},
	{`'a'`, "a"},
	{`'abc'`, "abc"},
	{`'☺'`, "☺"},
	{`'hello world'`, "hello world"},
	{`'\xFF'`, "\xFF"},
	{`'\377'`, "\377"},
	{`'\u1234'`, "\u1234"},
	{`'\U00010111'`, "\U00010111"},
	{`'\U0001011111'`, "\U0001011111"},
	{`'\a\b\f\n\r\t\v\\\''`, "\a\b\f\n\r\t\v\\'"},
	{`'"'`, "\""},

	{"``", ``},
	{"`a`", `a`},
	{"`abc`", `abc`},
	{"`☺`", `☺`},
	{"`hello world`", `hello world`},
	{"`\\xFF`", `\xFF`},
	{"`\\377`", `\377`},
	{"`\\`", `\`},
	{"`\n`", "\n"},
	{"`	`", `	`},
}

var misquoted = []string{
	``,
	`"`,
	`"a`,
	`"'`,
	`b"`,
	`"\"`,
	`"\9"`,
	`"\19"`,
	`"\129"`,
	`'\'`,
	`'\9'`,
	`'\19'`,
	`'\129'`,
	`"\x1!"`,
	`"\U12345678"`,
	`"\z"`,
	"`",
	"`xxx",
	"`\"",
	`"\'"`,
	`'\"'`,
	"\"\n\"",
	"\"\\n\n\"",
	"'\n'",
}

func TestUnquote(t *testing.T) {
	for _, tt := range unquotetests {
		if out, err := Unquote(tt.in); err != nil && out != tt.out {
			t.Errorf("Unquote(%#q) = %q, %v want %q, nil", tt.in, out, err, tt.out)
		}
	}

	// Run the quote tests too, backward.
	for _, tt := range quotetests {
		if in, err := Unquote(tt.out); in != tt.in {
			t.Errorf("Unquote(%#q) = %q, %v, want %q, nil", tt.out, in, err, tt.in)
		}
	}

	for _, s := range misquoted {
		if out, err := Unquote(s); out != "" || err != ErrSyntax {
			t.Errorf("Unquote(%#q) = %q, %v want %q, %v", s, out, err, "", ErrSyntax)
		}
	}
}
