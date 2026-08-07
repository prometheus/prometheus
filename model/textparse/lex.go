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

import "io"

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

	// quotedStop marks the bytes that terminate or complicate a quoted
	// string. A '"' closes the string, a '\\' starts an escape sequence,
	// and 0 and '\n' are accepted inside a string by some of the lexer
	// rules but not by others. The quoted string fast path bails out on all
	// but the closing quote.
	quotedStop = [256]bool{'"': true, '\\': true, 0: true, '\n': true}
)

// scanName returns the length of the run of bytes at the start of b that are
// marked in tab.
func scanName(b []byte, tab *[256]bool) int {
	i := 0
	for i < len(b) && tab[b[i]] {
		i++
	}
	return i
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

// Lex returns the next token of the Prometheus text format input. It is the
// entry point used by PromParser; lexDFA in promlex.l.go holds the generated
// state machine that recognises the full grammar.
func (l *promlexer) Lex() token {
	if l.i >= len(l.b) {
		return tEOF
	}
	// Each case below covers the rules of one start condition that are worth
	// recognising directly. On a leading byte that no such rule accepts the
	// generated lexer takes over. See promlex.l for the rules themselves.
	switch c := l.b[l.i]; l.state {
	case sInit:
		if metricNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &metricNameChar), sValue, tMName)
		}
	case sMeta1:
		if metricNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &metricNameChar), sMeta2, tMName)
		}
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sMeta2, tMName)
			}
		}
	case sLabels:
		if labelNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &labelNameChar), sLabels, tLName)
		}
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tQString)
			}
		}
	case sLValue:
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tLValue)
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

// Lex returns the next token of the OpenMetrics text format input. It is the
// entry point used by OpenMetricsParser; lexDFA in openmetricslex.l.go holds
// the generated state machine that recognises the full grammar.
func (l *openMetricsLexer) Lex() token {
	if l.i >= len(l.b) {
		return tEOF
	}
	// Each case below covers the rules of one start condition that are worth
	// recognising directly. On a leading byte that no such rule accepts the
	// generated lexer takes over. See openmetricslex.l for the rules
	// themselves.
	switch c := l.b[l.i]; l.state {
	case sInit:
		if metricNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &metricNameChar), sValue, tMName)
		}
	case sMeta1:
		if metricNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &metricNameChar), sMeta2, tMName)
		}
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sMeta2, tMName)
			}
		}
	case sLabels:
		if labelNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &labelNameChar), sLabels, tLName)
		}
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tQString)
			}
		}
	case sLValue:
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sLabels, tLValue)
			}
		}
	case sExemplar:
		if labelNameStart[c] {
			return l.emit(l.i+1+scanName(l.b[l.i+1:], &labelNameChar), sExemplar, tLName)
		}
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sExemplar, tQString)
			}
		}
	case sEValue:
		if c == '"' {
			if n := scanQuoted(l.b[l.i:]); n > 0 {
				return l.emit(l.i+n, sExemplar, tLValue)
			}
		}
	}
	return l.lexDFA()
}
