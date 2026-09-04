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
	"bytes"
	"io"
)

// This file holds hand-written fast paths for the two golex-generated lexers in
// promlex.l.go and openmetricslex.l.go.
//
// The generated state machines walk the input one byte at a time through
// promlexer.next and openMetricsLexer.next. Both of those keep the read index
// in the lexer struct, so every byte costs a load-modify-store, and both
// re-test the null byte rule that only applies to three start conditions. The
// scanners below consume the same input with a local index and a byte class
// lookup instead.
//
// Every fast path is optional. When it does not recognise its input it leaves
// the lexer state untouched and reports failure, and the generated state
// machine lexes the token instead. The generated lexers therefore remain the
// single source of truth for the grammar: a fast path may only ever be more
// conservative than the rule it stands in for.

// charTable returns a lookup table marking every byte for which in reports
// true. The tables below mirror the character classes of the generated lexers.
func charTable(in func(c byte) bool) (t [256]bool) {
	for c := range 256 {
		t[c] = in(byte(c))
	}
	return t
}

var (
	// metricNameStart and metricNameChar mark the bytes that may start and
	// continue an unquoted metric family name: {M} and {M}|{D} in both
	// lexer grammars.
	metricNameStart = charTable(func(c byte) bool {
		return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == ':'
	})
	metricNameChar = charTable(func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == ':'
	})

	// labelNameStart and labelNameChar mark the bytes that may start and
	// continue an unquoted label name: {L} and {L}|{D}.
	labelNameStart = charTable(func(c byte) bool {
		return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
	})
	labelNameChar = charTable(func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
	})

	// digitChar marks the bytes of a timestamp: {D}.
	digitChar = charTable(func(c byte) bool {
		return c >= '0' && c <= '9'
	})

	// whitespaceChar marks the bytes matched by the [ \t]+ rule, which the
	// Prometheus text format lexer applies in every start condition.
	whitespaceChar = charTable(func(c byte) bool {
		return c == ' ' || c == '\t'
	})

	// promValueChar marks the bytes of a Prometheus text format sample
	// value: [^{ \t\n], minus the null byte, which no rule accepts here.
	promValueChar = charTable(func(c byte) bool {
		return c != 0 && c != '{' && c != ' ' && c != '\t' && c != '\n'
	})

	// omValueChar marks the bytes of an OpenMetrics sample value,
	// timestamp or exemplar value: [^ \n], minus the null byte, which no
	// rule accepts there.
	omValueChar = charTable(func(c byte) bool {
		return c != 0 && c != ' ' && c != '\n'
	})

	// quotedStop marks the bytes that terminate or complicate a quoted
	// string. A '"' closes the string, a '\\' starts an escape sequence,
	// and 0 and '\n' are accepted inside a string by some of the lexer
	// rules but not by others. The quoted string fast path bails out on all
	// but the closing quote.
	quotedStop = [256]bool{'"': true, '\\': true, 0: true, '\n': true}
)

// scanRun returns the length of the run of bytes at the start of b that are
// marked in tab.
func scanRun(b []byte, tab *[256]bool) int {
	i := 0
	for i < len(b) && tab[b[i]] {
		i++
	}
	return i
}

// scanPromText scans the {C}* help or type text at the start of b, which runs
// up to but not including the terminating newline. It returns the length of the
// text, or -1 if the generated lexer has to take over: when the line holds a
// null byte, which next swallows, when the line has no newline at all, or when
// the text is empty or all whitespace, in which case the [ \t]+ rule matches
// just as much and, being the earlier rule, wins.
func scanPromText(b []byte) int {
	n := bytes.IndexByte(b, '\n')
	if n <= 0 || bytes.IndexByte(b[:n], 0) >= 0 {
		return -1
	}
	if scanRun(b[:n], &whitespaceChar) == n {
		return -1
	}
	return n
}

// scanOMText scans the {S}{C}*\n help, type or unit text at the start of b. It
// returns the length of the text including the leading space and the trailing
// newline, both of which the rule matches, or -1 if the generated lexer has to
// take over: when the space or the newline is missing, or when a null byte sits
// inside the text or directly behind it, see scanQuoted.
func scanOMText(b []byte) int {
	if len(b) == 0 || b[0] != ' ' {
		return -1
	}
	n := bytes.IndexByte(b[1:], '\n')
	if n < 0 || bytes.IndexByte(b[1:n+1], 0) >= 0 {
		return -1
	}
	if n+2 < len(b) && b[n+2] == 0 {
		return -1
	}
	return n + 2
}

// scanQuoted scans the quoted string starting at b[0], which must be a '"'. It
// returns the offset just past the closing quote, or -1 if the string is
// unterminated or holds an escape sequence, a null byte or a newline. Callers
// must fall back to the generated lexer when it returns -1.
//
// A null byte directly after the closing quote is rejected as well. The next
// methods of both lexers swallow null bytes while the lexer is in one of the
// states that allow them, and the state is only advanced by the rule action, so
// the generated lexers pull such bytes into the token they are just finishing.
func scanQuoted(b []byte) int {
	for i := 1; i < len(b); i++ {
		if quotedStop[b[i]] {
			if b[i] == '"' && (i+1 >= len(b) || b[i+1] != 0) {
				return i + 1
			}
			return -1
		}
	}
	return -1
}

// emit records a token ending at end, switches to the next start condition and
// returns t, exactly as the generated lexer does when its rule for t matches up
// to end.
func (l *promlexer) emit(end, next int, t token) token {
	l.start = l.i
	l.i = end
	l.state = next
	if end >= len(l.b) {
		l.err = io.EOF
	}
	return t
}

// lexWhitespace matches the [ \t]+ rule, which leaves the start condition
// alone. The byte at the current position must be a space or a tab. It fails
// when the run ends on a null byte, for the reason given on scanQuoted: in the
// start conditions where next swallows null bytes the generated lexer would
// pull them into this token.
func (l *promlexer) lexWhitespace() (token, bool) {
	end := l.i + 1 + scanRun(l.b[l.i+1:], &whitespaceChar)
	if end < len(l.b) && l.b[end] == 0 {
		return 0, false
	}
	return l.emit(end, l.state, tWhitespace), true
}

// Lex returns the next token of the Prometheus text format input. It is the
// entry point used by PromParser; lexDFA in promlex.l.go holds the generated
// state machine that recognises the full grammar.
//
// The cases below cover the rules that a series line is made of. Comments,
// the HELP and TYPE keywords, escape sequences and every error are left to the
// generated lexer, which also takes over whenever a fast path finds a leading
// byte that its rules do not accept. See promlex.l for the rules themselves.
func (l *promlexer) Lex() token {
	if l.i >= len(l.b) {
		return tEOF
	}
	switch c := l.b[l.i]; l.state {
	case sInit:
		switch {
		case metricNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &metricNameChar), sValue, tMName)
		case c == '\n':
			return l.emit(l.i+1, sInit, tLinebreak)
		case c == '{':
			return l.emit(l.i+1, sLabels, tBraceOpen)
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	case sMeta1:
		switch {
		case metricNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &metricNameChar), sMeta2, tMName)
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sMeta2, tMName)
			}
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	case sMeta2:
		if n := scanPromText(l.b[l.i:]); n > 0 {
			return l.emit(l.i+n, sInit, tText)
		}
	case sLabels:
		switch {
		case labelNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &labelNameChar), sLabels, tLName)
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tQString)
			}
		case c == '=':
			return l.emit(l.i+1, sLValue, tEqual)
		case c == ',':
			return l.emit(l.i+1, sLabels, tComma)
		case c == '}':
			return l.emit(l.i+1, sValue, tBraceClose)
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	case sLValue:
		switch {
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tLValue)
			}
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	case sValue:
		switch {
		case promValueChar[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &promValueChar), sTimestamp, tValue)
		case c == '{':
			return l.emit(l.i+1, sLabels, tBraceOpen)
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	case sTimestamp:
		switch {
		case digitChar[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &digitChar), sTimestamp, tTimestamp)
		case c == '\n':
			return l.emit(l.i+1, sInit, tLinebreak)
		case whitespaceChar[c]:
			if t, ok := l.lexWhitespace(); ok {
				return t
			}
		}
	}
	return l.lexDFA()
}

// emit records a token ending at end, switches to the next start condition and
// returns t, exactly as the generated lexer does when its rule for t matches up
// to end.
func (l *openMetricsLexer) emit(end, next int, t token) token {
	l.start = l.i
	l.i = end
	l.state = next
	if end >= len(l.b) {
		l.err = io.EOF
	}
	return t
}

// lexSpaced matches the {S}[^ \n]+ rule that OpenMetrics uses for sample
// values and timestamps. The byte at the current position must be the space.
// It fails on an empty run, which no rule accepts.
func (l *openMetricsLexer) lexSpaced(next int, t token) (token, bool) {
	n := scanRun(l.b[l.i+1:], &omValueChar)
	if n == 0 {
		return 0, false
	}
	return l.emit(l.i+1+n, next, t), true
}

// Lex returns the next token of the OpenMetrics text format input. It is the
// entry point used by OpenMetricsParser; lexDFA in openmetricslex.l.go holds
// the generated state machine that recognises the full grammar.
//
// The cases below cover the rules that a series line and its exemplar are made
// of. Comments, the HELP, TYPE, UNIT and EOF keywords, escape sequences and
// every error are left to the generated lexer, which also takes over whenever a
// fast path finds a leading byte that its rules do not accept. See
// openmetricslex.l for the rules themselves.
func (l *openMetricsLexer) Lex() token {
	if l.i >= len(l.b) {
		return tEOF
	}
	switch c := l.b[l.i]; l.state {
	case sInit:
		switch {
		case metricNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &metricNameChar), sValue, tMName)
		case c == '{':
			return l.emit(l.i+1, sLabels, tBraceOpen)
		}
	case sMeta1:
		switch {
		case metricNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &metricNameChar), sMeta2, tMName)
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sMeta2, tMName)
			}
		}
	case sMeta2:
		if n := scanOMText(l.b[l.i:]); n > 0 {
			return l.emit(l.i+n, sInit, tText)
		}
	case sLabels:
		switch {
		case labelNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &labelNameChar), sLabels, tLName)
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tQString)
			}
		case c == '=':
			return l.emit(l.i+1, sLValue, tEqual)
		case c == ',':
			return l.emit(l.i+1, sLabels, tComma)
		case c == '}':
			return l.emit(l.i+1, sValue, tBraceClose)
		}
	case sLValue:
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tLValue)
			}
		}
	case sValue:
		switch c {
		case ' ':
			if t, ok := l.lexSpaced(sTimestamp, tValue); ok {
				return t
			}
		case '{':
			return l.emit(l.i+1, sLabels, tBraceOpen)
		}
	case sTimestamp:
		switch c {
		case '\n':
			return l.emit(l.i+1, sInit, tLinebreak)
		case ' ':
			// A '#' here may start the {S}#{S}\{ exemplar rule, which
			// overlaps with the timestamp rule for a whole token.
			if l.i+1 < len(l.b) && l.b[l.i+1] != '#' {
				if t, ok := l.lexSpaced(sTimestamp, tTimestamp); ok {
					return t
				}
			}
		}
	case sExemplar:
		switch {
		case labelNameStart[c]:
			return l.emit(l.i+1+scanRun(l.b[l.i+1:], &labelNameChar), sExemplar, tLName)
		case c == '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sExemplar, tQString)
			}
		case c == '=':
			return l.emit(l.i+1, sEValue, tEqual)
		case c == ',':
			return l.emit(l.i+1, sExemplar, tComma)
		case c == '}':
			return l.emit(l.i+1, sEValue, tBraceClose)
		}
	case sEValue:
		switch c {
		case '"':
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sExemplar, tLValue)
			}
		case ' ':
			if t, ok := l.lexSpaced(sETimestamp, tValue); ok {
				return t
			}
		}
	case sETimestamp:
		switch c {
		case '\n':
			return l.emit(l.i+1, sInit, tLinebreak)
		case ' ':
			if t, ok := l.lexSpaced(sETimestamp, tTimestamp); ok {
				return t
			}
		}
	}
	return l.lexDFA()
}
