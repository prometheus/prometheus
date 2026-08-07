// Copyright The Prometheus Authors
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

package textparse

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// lexStep is one observation of a lexer's externally visible state.
type lexStep struct {
	tok   token
	i     int
	start int
	state int
}

// lexInputs are inputs that exercise the fast paths in lex.go and, more
// importantly, the cases where they must hand over to the generated lexers.
var lexInputs = []struct {
	name  string
	input string
}{
	{"plain label value", `m{a="b"} 1`},
	{"empty label value", `m{a=""} 1`},
	{"long label value", `m{a="` + strings.Repeat("0123456789", 30) + `"} 1`},
	{"several labels", `m{a="b",c="d", e="f"} 1`},
	{"escaped quote", `m{a="\"b\""} 1`},
	{"escaped backslash", `m{a="b\\"} 1`},
	{"trailing backslash", `m{a="b\`},
	{"unterminated value", `m{a="b} 1`},
	{"unterminated at eof", `m{a="`},
	{"null in label value", "m{a=\"b\x00c\"} 1"},
	{"only null in label value", "m{a=\"\x00\"} 1"},
	// The generated lexers swallow a null byte that directly follows the
	// closing quote, because the rule action has not switched the start
	// condition yet. Found by FuzzLexFastPathEquivalence.
	{"null after label value", "m{a=\"\"\x00} 1"},
	{"null after space before label value", "m{a= \x00\"b\"} 1"},
	{"null in help text", "# HELP m so\x00me text\n"},
	{"whitespace only help text", "# HELP m \n"},
	{"empty help text", "# HELP m\n"},
	{"tabs around value", "m\t1\t\t123\n"},
	{"no trailing newline", "m 1"},
	{"timestamp", "m 1 1395066363000\n"},
	{"newline in label value", "m{a=\"b\nc\"} 1"},
	{"quoted label name", `{"a.b"="c"} 1`},
	{"quoted metric name", `{"m.n",a="b"} 1`},
	{"quoted name in help", `# HELP "m.n" some text` + "\n"},
	{"quoted name in type", `# TYPE "m.n" counter` + "\n"},
	{"escaped quoted name", `{"a\"b"="c"} 1`},
	{"value looking like a string", `m "1"`},
	{"help text with quote", `# HELP m a "quoted" word` + "\n"},
	{"utf8 label value", `m{a="ünïcödé"} 1`},
	{"empty input", ``},
	{"lone quote", `"`},
}

// omLexInputs additionally covers the OpenMetrics-only exemplar states.
var omLexInputs = []struct {
	name  string
	input string
}{
	{"exemplar", `m_bucket{le="1"} 1 # {a="b"} 0.5 123` + "\n"},
	{"exemplar escaped", `m_bucket{le="1"} 1 # {a="\"b\""} 0.5` + "\n"},
	{"exemplar unterminated", `m_bucket{le="1"} 1 # {a="b} 0.5` + "\n"},
	{"om eof", "m 1\n# EOF\n"},
}

// TestLexFastPathEquivalence asserts that the fast paths in lex.go emit exactly
// the same tokens, and leave the lexer at exactly the same position, as the
// generated state machines they stand in for.
func TestLexFastPathEquivalence(t *testing.T) {
	inputs := append([]struct {
		name  string
		input string
	}{}, lexInputs...)
	inputs = append(inputs, omLexInputs...)
	for _, f := range []string{
		"alltypes.237mfs.prom.txt",
		"alltypes.237mfs.nometa.prom.txt",
		"alltypes.5mfs.om.txt",
		"1histogram.om.txt",
	} {
		inputs = append(inputs, struct {
			name  string
			input string
		}{f, string(readTestdataFile(t, f))})
	}

	for _, c := range inputs {
		t.Run(c.name, func(t *testing.T) {
			b := append([]byte(c.input), '\n')

			fast := &promlexer{b: b}
			slow := &promlexer{b: b}
			requireSameTokens(t, func() lexStep {
				tok := fast.Lex()
				return lexStep{tok, fast.i, fast.start, fast.state}
			}, func() lexStep {
				tok := slow.lexDFA()
				return lexStep{tok, slow.i, slow.start, slow.state}
			})

			omFast := &openMetricsLexer{b: b}
			omSlow := &openMetricsLexer{b: b}
			requireSameTokens(t, func() lexStep {
				tok := omFast.Lex()
				return lexStep{tok, omFast.i, omFast.start, omFast.state}
			}, func() lexStep {
				tok := omSlow.lexDFA()
				return lexStep{tok, omSlow.i, omSlow.start, omSlow.state}
			})
		})
	}
}

func requireSameTokens(t *testing.T, fast, slow func() lexStep) {
	t.Helper()
	for n := 0; ; n++ {
		got, want := fast(), slow()
		require.Equal(t, want, got, "token %d", n)
		if got.tok == tEOF || got.tok == tInvalid {
			return
		}
	}
}

// FuzzLexFastPathEquivalence is the property-based counterpart of
// TestLexFastPathEquivalence: on any input, both lexers must agree.
func FuzzLexFastPathEquivalence(f *testing.F) {
	for _, c := range lexInputs {
		f.Add(c.input)
	}
	for _, c := range omLexInputs {
		f.Add(c.input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		b := append([]byte(input), '\n')

		fast := &promlexer{b: b}
		slow := &promlexer{b: b}
		requireSameTokens(t, func() lexStep {
			tok := fast.Lex()
			return lexStep{tok, fast.i, fast.start, fast.state}
		}, func() lexStep {
			tok := slow.lexDFA()
			return lexStep{tok, slow.i, slow.start, slow.state}
		})

		omFast := &openMetricsLexer{b: b}
		omSlow := &openMetricsLexer{b: b}
		requireSameTokens(t, func() lexStep {
			tok := omFast.Lex()
			return lexStep{tok, omFast.i, omFast.start, omFast.state}
		}, func() lexStep {
			tok := omSlow.lexDFA()
			return lexStep{tok, omSlow.i, omSlow.start, omSlow.state}
		})
	})
}

func TestScanQuoted(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want int
	}{
		{"empty string", `""`, 2},
		{"simple", `"abc"`, 5},
		{"stops at closing quote", `"abc"def`, 5},
		{"utf8", `"ünï"`, len(`"ünï"`)},
		{"unterminated", `"abc`, -1},
		{"escape", `"a\"b"`, -1},
		{"trailing escape", `"a\`, -1},
		{"null byte", "\"a\x00b\"", -1},
		{"null byte after closing quote", "\"ab\"\x00", -1},
		{"newline", "\"a\nb\"", -1},
		{"lone quote", `"`, -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, scanQuoted([]byte(c.in)))
		})
	}
}
