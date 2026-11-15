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

package searchexpr

import (
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestParsePrecedence(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  Expr
	}{
		{
			name:  "term",
			input: "cortex",
			want:  Term{Value: "cortex"},
		},
		{
			name:  "AND binds tighter than OR",
			input: "cortex AND rule_evaluation_failures OR loki",
			want: Or{
				Left:  And{Left: Term{Value: "cortex"}, Right: Term{Value: "rule_evaluation_failures"}},
				Right: Term{Value: "loki"},
			},
		},
		{
			name:  "AND binds tighter than a preceding OR",
			input: "cortex OR loki AND envoy",
			want: Or{
				Left:  Term{Value: "cortex"},
				Right: And{Left: Term{Value: "loki"}, Right: Term{Value: "envoy"}},
			},
		},
		{
			name:  "parentheses override precedence",
			input: "(cortex OR loki) AND rule_evaluation_failures",
			want: And{
				Left:  Or{Left: Term{Value: "cortex"}, Right: Term{Value: "loki"}},
				Right: Term{Value: "rule_evaluation_failures"},
			},
		},
		{
			name:  "NOT binds tighter than AND",
			input: "NOT cortex AND loki",
			want: And{
				Left:  Not{Expr: Term{Value: "cortex"}},
				Right: Term{Value: "loki"},
			},
		},
		{
			name:  "parenthesized NOT applies to the whole group",
			input: "NOT (cortex OR loki)",
			want: Not{Expr: Or{
				Left:  Term{Value: "cortex"},
				Right: Term{Value: "loki"},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertParse(t, test.input, test.want)
		})
	}
}

func TestParseOperatorsAndQuotedTerms(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  Expr
	}{
		{
			name:  "operators are case insensitive",
			input: "cortex and loki Or envoy",
			want: Or{
				Left:  And{Left: Term{Value: "cortex"}, Right: Term{Value: "loki"}},
				Right: Term{Value: "envoy"},
			},
		},
		{
			name:  "chains AND left to right",
			input: "cortex AND loki AND envoy",
			want: And{
				Left:  And{Left: Term{Value: "cortex"}, Right: Term{Value: "loki"}},
				Right: Term{Value: "envoy"},
			},
		},
		{
			name:  "chains OR left to right",
			input: "cortex OR loki OR envoy",
			want: Or{
				Left:  Or{Left: Term{Value: "cortex"}, Right: Term{Value: "loki"}},
				Right: Term{Value: "envoy"},
			},
		},
		{
			name:  "quoted keywords are terms",
			input: `"AND" AND "OR" OR "NOT"`,
			want: Or{
				Left:  And{Left: Term{Value: "AND"}, Right: Term{Value: "OR"}},
				Right: Term{Value: "NOT"},
			},
		},
		{
			name:  "quoted term supports escapes",
			input: `"cortex \"ruler\""`,
			want:  Term{Value: `cortex "ruler"`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertParse(t, test.input, test.want)
		})
	}
}

func TestParseResultDoesNotAliasInput(t *testing.T) {
	// Term values must own their memory. A term that points into the parser
	// input keeps the whole expression alive, and if the input ever references a
	// pooled request buffer it could outlive that buffer. Cover every path that
	// produces a Term value: a bare term, a quoted term, and a quoted term
	// containing an escape.
	input := strings.Join([]string{
		"cortex", "AND", `"quoted term"`, "OR", "NOT", `"esc\"aped"`, "OR", "deprecated",
	}, " ")
	expr, err := Parse(input)
	require.NoError(t, err)

	terms := collectTermValues(expr)
	require.Equal(t, []string{"cortex", "quoted term", `esc"aped`, "deprecated"}, terms)

	inputStart := uintptr(unsafe.Pointer(unsafe.StringData(input)))
	inputEnd := inputStart + uintptr(len(input))
	for _, term := range terms {
		termStart := uintptr(unsafe.Pointer(unsafe.StringData(term)))
		require.False(t, termStart >= inputStart && termStart < inputEnd, "term %q aliases the parser input", term)
	}
}

// collectTermValues returns every Term value in expr, left to right.
func collectTermValues(expr Expr) []string {
	switch expr := expr.(type) {
	case Term:
		return []string{expr.Value}
	case And:
		return append(collectTermValues(expr.Left), collectTermValues(expr.Right)...)
	case Or:
		return append(collectTermValues(expr.Left), collectTermValues(expr.Right)...)
	case Not:
		return collectTermValues(expr.Expr)
	default:
		return nil
	}
}

func assertParse(t *testing.T, input string, want Expr) {
	t.Helper()
	got, err := Parse(input)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// Label names and values may hold any valid UTF-8, so terms must keep every
// byte that is not ASCII whitespace, a parenthesis, or a quote.
func TestParseUTF8Terms(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  Expr
	}{
		{name: "accented", input: "métrique", want: Term{Value: "métrique"}},
		{name: "CJK", input: "指标 AND 名前", want: And{Left: Term{Value: "指标"}, Right: Term{Value: "名前"}}},
		{name: "emoji", input: "🔥 OR 🚀", want: Or{Left: Term{Value: "🔥"}, Right: Term{Value: "🚀"}}},
		{name: "right-to-left", input: "NOT مقياس", want: Not{Expr: Term{Value: "مقياس"}}},
		{name: "combining mark", input: "é", want: Term{Value: "é"}},
		{name: "dotted UTF-8 label name", input: "http.server.request.duration", want: Term{Value: "http.server.request.duration"}},
		{name: "punctuation in bare term", input: `k8s-pod:name/a=b,c\d`, want: Term{Value: `k8s-pod:name/a=b,c\d`}},
		{name: "non-ASCII whitespace does not separate terms", input: "a b　c d", want: Term{Value: "a b　c d"}},
		{name: "fullwidth keywords are terms", input: "ＮＯＴ AND ｏｒ", want: And{Left: Term{Value: "ＮＯＴ"}, Right: Term{Value: "ｏｒ"}}},
		{name: "quoted delimiters and keywords", input: `"foo (bar) AND \"baz\" OR qux"`, want: Term{Value: `foo (bar) AND "baz" OR qux`}},
		{name: "quoted escapes around multi-byte runes", input: `"\"é\" \\🔥"`, want: Term{Value: `"é" \🔥`}},
		{name: "quoted control bytes", input: "\"a\x00b\x7fc\td\"", want: Term{Value: "a\x00b\x7fc\td"}},
		{name: "invalid UTF-8 bytes pass through", input: "\xff\xfe AND \"\xc3\"", want: And{Left: Term{Value: "\xff\xfe"}, Right: Term{Value: "\xc3"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertParse(t, test.input, test.want)
		})
	}
}

func TestParseUTF8PositionsAndLimitsAreBytes(t *testing.T) {
	_, err := Parse("é AND")
	require.EqualError(t, err, "search expression: expected term or (, got end of expression at position 6")

	atLimit := strings.Repeat("é", maxExpressionBytes/len("é"))
	_, err = Parse(atLimit)
	require.NoError(t, err)

	_, err = Parse(atLimit + "a")
	require.EqualError(t, err, "search expression: length 4097 exceeds maximum of 4096 bytes")
}

func TestParseRejectsInvalidExpressions(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty expression", input: "", want: "search expression: expected term or (, got end of expression at position 0"},
		{name: "leading AND", input: "AND cortex", want: "search expression: expected term or (, got AND at position 0"},
		{name: "leading OR", input: "OR cortex", want: "search expression: expected term or (, got OR at position 0"},
		{name: "bare NOT", input: "NOT", want: "search expression: expected term or (, got end of expression at position 3"},
		{name: "trailing AND", input: "cortex AND", want: "search expression: expected term or (, got end of expression at position 10"},
		{name: "trailing OR", input: "cortex OR", want: "search expression: expected term or (, got end of expression at position 9"},
		{name: "trailing NOT", input: "cortex AND NOT", want: "search expression: expected term or (, got end of expression at position 14"},
		{name: "repeated AND", input: "cortex AND AND loki", want: "search expression: expected term or (, got AND at position 11"},
		{name: "repeated OR", input: "cortex OR OR loki", want: "search expression: expected term or (, got OR at position 10"},
		{name: "NOT before close parenthesis", input: "NOT )", want: "search expression: expected term or (, got ) at position 4"},
		{name: "implicit adjacent terms", input: "cortex rule_evaluation_failures", want: `search expression: unexpected term "rule_evaluation_failures" at position 7`},
		{name: "implicit term before group", input: "cortex (loki OR envoy)", want: "search expression: unexpected ( at position 7"},
		{name: "empty group", input: "()", want: "search expression: expected term or (, got ) at position 1"},
		{name: "whitespace-only group", input: "(  )", want: "search expression: expected term or (, got ) at position 3"},
		{name: "missing closing parenthesis", input: "(cortex OR loki", want: "search expression: expected ), got end of expression at position 15"},
		{name: "missing group operand", input: "(", want: "search expression: expected term or (, got end of expression at position 1"},
		{name: "operator before close parenthesis", input: "cortex AND )", want: "search expression: expected term or (, got ) at position 11"},
		{name: "extra closing parenthesis", input: "cortex)", want: "search expression: unexpected ) at position 6"},
		{name: "operator in nested group", input: "cortex AND (loki OR)", want: "search expression: expected term or (, got ) at position 19"},
		{name: "adjacent quoted terms", input: `"cortex""loki"`, want: `search expression: unexpected term "loki" at position 8`},
		{name: "empty quoted term", input: `""`, want: "search expression: empty quoted term at position 0"},
		{name: "unterminated quoted term", input: `"unterminated`, want: "search expression: unterminated quoted term at position 0"},
		{name: "trailing quote escape", input: "\"trailing escape\\", want: "search expression: unterminated quoted term at position 0"},
		{name: "unterminated quoted term with escape", input: `"esc\"aped`, want: "search expression: unterminated quoted term at position 0"},
		{name: "escaped letter", input: `"a\nb"`, want: "search expression: invalid escape at position 2"},
		{name: "escaped tab byte", input: "\"\\\t\"", want: "search expression: invalid escape at position 1"},
		{name: "hex escape", input: `"\x41"`, want: "search expression: invalid escape at position 1"},
		{name: "escaped multi-byte rune", input: `"\é"`, want: "search expression: invalid escape at position 1"},
		{name: "invalid escape after valid escape", input: `"a\\\"\qb"`, want: "search expression: invalid escape at position 6"},
		{name: "invalid escape in unterminated quoted term", input: `"ok" AND "a\qb`, want: "search expression: invalid escape at position 11"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestParseReportsOffendingTokenPosition(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "AND foo", want: "search expression: expected term or (, got AND at position 0"},
		{input: "OR foo", want: "search expression: expected term or (, got OR at position 0"},
		{input: ") foo", want: "search expression: expected term or (, got ) at position 0"},
	} {
		t.Run(test.input, func(t *testing.T) {
			_, err := Parse(test.input)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestParseRejectsQuoteInsideBareTerm(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: `NOT"foo"`, want: "search expression: unexpected quote at position 3"},
		{input: `a"`, want: "search expression: unexpected quote at position 1"},
		{input: `foo"bar baz"`, want: "search expression: unexpected quote at position 3"},
	} {
		t.Run(test.input, func(t *testing.T) {
			_, err := Parse(test.input)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestParseSeparators(t *testing.T) {
	want := And{
		Left:  Term{Value: "a"},
		Right: Or{Left: Term{Value: "b"}, Right: Not{Expr: Term{Value: "c"}}},
	}
	for _, test := range []struct {
		name      string
		separator string
	}{
		{name: "space", separator: " "},
		{name: "tab", separator: "\t"},
		{name: "line feed", separator: "\n"},
		{name: "carriage return", separator: "\r"},
		{name: "CRLF", separator: "\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Join([]string{"", "a", "AND", "(b", "OR", "NOT", "c)", ""}, test.separator)
			assertParse(t, input, want)
		})
	}
}

// The parser lexes on demand, so a parse error that comes before a lex error
// in the input wins.
func TestParseReportsFirstError(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "operand error before unterminated quote", input: `AND "unterminated`, want: "search expression: expected term or (, got AND at position 0"},
		{name: "adjacent terms before unterminated quote", input: `a b "unterminated`, want: `search expression: unexpected term "b" at position 2`},
		{name: "extra parenthesis before bare quote", input: `a ) b"`, want: "search expression: unexpected ) at position 2"},
		{name: "adjacent terms before term count", input: strings.Repeat("t ", maxExpressionTerms+1), want: `search expression: unexpected term "t" at position 2`},
		{name: "nesting depth before unterminated quote", input: strings.Repeat("(", maxExpressionNestingDepth+1) + `"x`, want: "search expression: nesting depth exceeds maximum of 16 at position 16"},
		{name: "lex error after operator", input: `"x" AND "`, want: "search expression: unterminated quoted term at position 8"},
		{name: "lex error after term", input: `a "x`, want: "search expression: unterminated quoted term at position 2"},
		{name: "lex error after NOT", input: `NOT "x`, want: "search expression: unterminated quoted term at position 4"},
		{name: "lex error after open parenthesis", input: `("x`, want: "search expression: unterminated quoted term at position 1"},
		{name: "lex error after close parenthesis", input: `(a) "x`, want: "search expression: unterminated quoted term at position 4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestParseResourceLimits(t *testing.T) {
	t.Run("expression bytes", func(t *testing.T) {
		_, err := Parse("a" + strings.Repeat(" ", maxExpressionBytes))
		require.EqualError(t, err, "search expression: length 4097 exceeds maximum of 4096 bytes")
	})

	t.Run("term leaves", func(t *testing.T) {
		terms := make([]string, maxExpressionTerms+1)
		for i := range terms {
			terms[i] = "term"
		}
		_, err := Parse(strings.Join(terms, " AND "))
		require.EqualError(t, err, "search expression: term count exceeds maximum of 32 at position 288")
	})

	t.Run("quoted term leaves", func(t *testing.T) {
		terms := make([]string, maxExpressionTerms+1)
		for i := range terms {
			terms[i] = `"term"`
		}
		_, err := Parse(strings.Join(terms, " AND "))
		require.EqualError(t, err, "search expression: term count exceeds maximum of 32 at position 352")
	})

	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "parentheses", input: strings.Repeat("(", maxExpressionNestingDepth+1) + "foo" + strings.Repeat(")", maxExpressionNestingDepth+1)},
		{name: "NOT", input: strings.Repeat("NOT ", maxExpressionNestingDepth+1) + "foo"},
		{name: "mixed", input: strings.Repeat("NOT (", maxExpressionNestingDepth/2+1) + "foo" + strings.Repeat(")", maxExpressionNestingDepth/2+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), "nesting depth exceeds maximum of 16")
		})
	}
}

func TestParseAcceptsResourceLimitBoundaries(t *testing.T) {
	_, err := Parse("foo" + strings.Repeat(" ", maxExpressionBytes-len("foo")))
	require.NoError(t, err)

	terms := make([]string, maxExpressionTerms)
	for i := range terms {
		terms[i] = "term"
	}
	_, err = Parse(strings.Join(terms, " AND "))
	require.NoError(t, err)

	_, err = Parse(strings.Repeat("(", maxExpressionNestingDepth) + "foo" + strings.Repeat(")", maxExpressionNestingDepth))
	require.NoError(t, err)

	_, err = Parse(strings.Repeat("NOT ", maxExpressionNestingDepth) + "foo")
	require.NoError(t, err)

	_, err = Parse(strings.Repeat("NOT ", maxExpressionNestingDepth) + strings.Join(terms, " AND "))
	require.NoError(t, err)
}

func TestTokenDescribe(t *testing.T) {
	for _, test := range []struct {
		token token
		want  string
	}{
		{token: token{kind: tokenEOF}, want: "end of expression"},
		{token: token{kind: tokenTerm, value: "cortex"}, want: `term "cortex"`},
		{token: token{kind: tokenNot}, want: "NOT"},
		{token: token{kind: tokenAnd}, want: "AND"},
		{token: token{kind: tokenOr}, want: "OR"},
		{token: token{kind: tokenLeftParen}, want: "("},
		{token: token{kind: tokenRightParen}, want: ")"},
		{token: token{kind: tokenKind(99)}, want: "token"},
	} {
		require.Equal(t, test.want, test.token.describe())
	}
}

func TestExprNodesImplementExpr(*testing.T) {
	Term{}.expr()
	And{}.expr()
	Or{}.expr()
	Not{}.expr()
}

func FuzzParse(f *testing.F) {
	for _, input := range []string{
		"",
		"cortex",
		"cortex AND rule_evaluation_failures OR loki",
		"loki AND NOT cortex",
		"(cortex OR loki) AND rule_evaluation_failures",
		`"AND" AND "OR" OR "NOT"`,
		`"unterminated`,
		"cortex AND )",
		"\x00",
		"\xff",
		"((((((cortex))))))",
		`"指标 \"é\"" AND NOT 🔥`,
		"a b OR ＮＯＴ",
		strings.Repeat("NOT ", maxExpressionNestingDepth+1) + "cortex",
	} {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, input string) {
		expr, err := Parse(input)
		if err != nil {
			return
		}
		require.NotNil(t, expr)
		assertWellFormedExpr(t, expr)

		require.LessOrEqual(t, len(input), maxExpressionBytes)
		terms := collectTermValues(expr)
		require.LessOrEqual(t, len(terms), maxExpressionTerms)
		if utf8.ValidString(input) {
			for _, term := range terms {
				require.True(t, utf8.ValidString(term), "term %q splits a UTF-8 sequence", term)
			}
		}
		require.LessOrEqual(t, maxNestingDepth(lexTokens(t, input)), maxExpressionNestingDepth)
	})
}

func lexTokens(t *testing.T, input string) []token {
	t.Helper()
	p := parser{input: input}
	var tokens []token
	for {
		require.NoError(t, p.advance())
		tokens = append(tokens, p.cur)
		if p.cur.kind == tokenEOF {
			return tokens
		}
	}
}

// maxNestingDepth measures nesting from tokens because groups leave no node in
// the AST. Each NOT and each group adds one level. A NOT run ends with the
// primary that follows it: a term, or a group at its closing parenthesis.
func maxNestingDepth(tokens []token) int {
	var depth, deepest, pendingNots int
	var notsBeforeGroup []int
	for _, tok := range tokens {
		switch tok.kind {
		case tokenNot:
			pendingNots++
			depth++
		case tokenTerm:
			depth -= pendingNots
			pendingNots = 0
		case tokenLeftParen:
			notsBeforeGroup = append(notsBeforeGroup, pendingNots)
			pendingNots = 0
			depth++
		case tokenRightParen:
			depth -= 1 + notsBeforeGroup[len(notsBeforeGroup)-1]
			notsBeforeGroup = notsBeforeGroup[:len(notsBeforeGroup)-1]
		}
		deepest = max(deepest, depth)
	}
	return deepest
}

func TestMaxNestingDepth(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int
	}{
		{input: "foo", want: 0},
		{input: "NOT foo", want: 1},
		{input: "NOT (foo)", want: 2},
		{input: "NOT NOT foo AND NOT bar", want: 2},
		{input: "(((foo))) AND NOT NOT bar", want: 3},
		{input: "(NOT foo) AND (bar)", want: 2},
		{input: "NOT (NOT foo OR (bar)) AND NOT baz", want: 3},
		{input: strings.Repeat("NOT (", maxExpressionNestingDepth/2) + "foo" + strings.Repeat(")", maxExpressionNestingDepth/2), want: maxExpressionNestingDepth},
	} {
		t.Run(test.input, func(t *testing.T) {
			_, err := Parse(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, maxNestingDepth(lexTokens(t, test.input)))
		})
	}
}

func assertWellFormedExpr(t *testing.T, expr Expr) {
	t.Helper()
	switch expr := expr.(type) {
	case Term:
		require.NotEmpty(t, expr.Value)
	case And:
		require.NotNil(t, expr.Left)
		require.NotNil(t, expr.Right)
		assertWellFormedExpr(t, expr.Left)
		assertWellFormedExpr(t, expr.Right)
	case Or:
		require.NotNil(t, expr.Left)
		require.NotNil(t, expr.Right)
		assertWellFormedExpr(t, expr.Left)
		assertWellFormedExpr(t, expr.Right)
	case Not:
		require.NotNil(t, expr.Expr)
		assertWellFormedExpr(t, expr.Expr)
	default:
		t.Fatalf("unexpected expression type %T", expr)
	}
}
