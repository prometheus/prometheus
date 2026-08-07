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
// re-test conditions that only apply to a handful of states. The scanners below
// consume the same input with a local index and a byte-class lookup instead.
//
// Every fast path is optional. When it does not recognise its input it leaves
// the lexer state untouched and reports failure, and the generated state
// machine lexes the token instead. The generated lexers therefore remain the
// single source of truth for the grammar: a fast path may only ever be more
// conservative than the rule it stands in for.

// quotedStop marks the bytes that terminate or complicate a quoted string.
// A '"' closes the string, a '\\' starts an escape sequence, and 0 and '\n'
// are accepted inside a string by some of the lexer rules but not by others,
// and null bytes are additionally swallowed by the next methods. The quoted
// string fast path bails out on all but the closing quote.
var quotedStop = [256]bool{'"': true, '\\': true, 0: true, '\n': true}

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

// Lex returns the next token of the Prometheus text format input. It is the
// entry point used by PromParser; lexDFA in promlex.l.go holds the generated
// state machine that recognises the full grammar.
func (l *promlexer) Lex() token {
	if l.i < len(l.b) && l.b[l.i] == '"' {
		// The states below reach a quoted string rule, and only that rule, on
		// a leading '"'. See the corresponding rules in promlex.l.
		var (
			t    token
			next int
		)
		switch l.state {
		case sLValue:
			t, next = tLValue, sLabels
		case sLabels:
			t, next = tQString, sLabels
		case sMeta1:
			t, next = tMName, sMeta2
		default:
			return l.lexDFA()
		}
		if n := scanQuoted(l.b[l.i:]); n > 0 {
			l.start = l.i
			l.i += n
			l.state = next
			if l.i >= len(l.b) {
				l.err = io.EOF
			}
			return t
		}
	}
	return l.lexDFA()
}

// Lex returns the next token of the OpenMetrics text format input. It is the
// entry point used by OpenMetricsParser; lexDFA in openmetricslex.l.go holds
// the generated state machine that recognises the full grammar.
func (l *openMetricsLexer) Lex() token {
	if l.i < len(l.b) && l.b[l.i] == '"' {
		// The states below reach a quoted string rule, and only that rule, on
		// a leading '"'. See the corresponding rules in openmetricslex.l.
		var (
			t    token
			next int
		)
		switch l.state {
		case sLValue:
			t, next = tLValue, sLabels
		case sLabels:
			t, next = tQString, sLabels
		case sEValue:
			t, next = tLValue, sExemplar
		case sExemplar:
			t, next = tQString, sExemplar
		case sMeta1:
			t, next = tMName, sMeta2
		default:
			return l.lexDFA()
		}
		if n := scanQuoted(l.b[l.i:]); n > 0 {
			l.start = l.i
			l.i += n
			l.state = next
			if l.i >= len(l.b) {
				l.err = io.EOF
			}
			return t
		}
	}
	return l.lexDFA()
}
