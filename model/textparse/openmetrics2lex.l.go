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

// This file was hand-written following the DFA pattern produced by golex for
// openmetricslex.l, adapted for the OpenMetrics 2.0 format.  The main
// differences from the OM 1.0 lexer are:
//   - Uses openMetrics2Lexer instead of openMetricsLexer.
//   - sTimestamp state recognises a new "st@<float>" token (tStartTimestamp).
//   - sETimestamp state recognises "{S}#{S}\{" to allow multiple exemplars per
//     sample.
//
// To regenerate, update openmetrics2lex.l and run:
//
//	golex -o=openmetrics2lex.l.go openmetrics2lex.l

package textparse

import (
	"fmt"
	"io"
)

// openMetrics2Lexer is the lexer for the OpenMetrics 2.0 text format.
type openMetrics2Lexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

// buf returns the bytes of the current token.
func (l *openMetrics2Lexer) buf() []byte {
	return l.b[l.start:l.i]
}

// next advances the openMetrics2Lexer to the next character and returns it.
func (l *openMetrics2Lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return byte(tEOF)
	}
	// Lex struggles with null bytes. If we are in a label value or help
	// string, where they are allowed, consume them here immediately.
	for l.b[l.i] == 0 && (l.state == sLValue || l.state == sMeta2 || l.state == sComment) {
		l.i++
		if l.i >= len(l.b) {
			l.err = io.EOF
			return byte(tEOF)
		}
	}
	return l.b[l.i]
}

// Lex returns the next token from the input byte slice.
func (l *openMetrics2Lexer) Lex() token {
	if l.i >= len(l.b) {
		return tEOF
	}
	c := l.b[l.i]
	l.start = l.i

yystate0:

	switch yyt := l.state; yyt {
	default:
		panic(fmt.Errorf(`invalid start condition %d`, yyt))
	case 0: // start condition: INITIAL
		goto yystart1
	case 1: // start condition: sComment
		goto yystart6
	case 2: // start condition: sMeta1
		goto yystart26
	case 3: // start condition: sMeta2
		goto yystart31
	case 4: // start condition: sLabels
		goto yystart34
	case 5: // start condition: sLValue
		goto yystart42
	case 6: // start condition: sValue
		goto yystart46
	case 7: // start condition: sTimestamp
		goto yystart50
	case 8: // start condition: sExemplar
		goto yystart57
	case 9: // start condition: sEValue
		goto yystart65
	case 10: // start condition: sETimestamp
		goto yystart71
	}

	// ---- INITIAL state ---------------------------------------------------------

yystate1:
	c = l.next()
yystart1:
	switch {
	default:
		goto yyabort
	case c == '#':
		goto yystate2
	case c == ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate4
	case c == '{':
		goto yystate5
	}

yystate2:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate3
	}

yystate3:
	c = l.next()
	goto yyrule1

yystate4:
	c = l.next()
	switch {
	default:
		goto yyrule9
	case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate4
	}

yystate5:
	c = l.next()
	goto yyrule11

	// ---- sComment state --------------------------------------------------------

yystate6:
	c = l.next()
yystart6:
	switch c {
	default:
		goto yyabort
	case 'E':
		goto yystate7
	case 'H':
		goto yystate11
	case 'T':
		goto yystate16
	case 'U':
		goto yystate21
	}

yystate7:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'O':
		goto yystate8
	}

yystate8:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'F':
		goto yystate9
	}

yystate9:
	c = l.next()
	switch c {
	default:
		goto yyrule5
	case '\n':
		goto yystate10
	}

yystate10:
	c = l.next()
	goto yyrule5

yystate11:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'E':
		goto yystate12
	}

yystate12:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'L':
		goto yystate13
	}

yystate13:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'P':
		goto yystate14
	}

yystate14:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate15
	}

yystate15:
	c = l.next()
	goto yyrule2

yystate16:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'Y':
		goto yystate17
	}

yystate17:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'P':
		goto yystate18
	}

yystate18:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'E':
		goto yystate19
	}

yystate19:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate20
	}

yystate20:
	c = l.next()
	goto yyrule3

yystate21:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'N':
		goto yystate22
	}

yystate22:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'I':
		goto yystate23
	}

yystate23:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case 'T':
		goto yystate24
	}

yystate24:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate25
	}

yystate25:
	c = l.next()
	goto yyrule4

	// ---- sMeta1 state ----------------------------------------------------------

yystate26:
	c = l.next()
yystart26:
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate27
	case c == ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate30
	}

yystate27:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate28
	case c == '\\':
		goto yystate29
	case c >= '\x01' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
		goto yystate27
	}

yystate28:
	c = l.next()
	goto yyrule6

yystate29:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate27
	}

yystate30:
	c = l.next()
	switch {
	default:
		goto yyrule7
	case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate30
	}

	// ---- sMeta2 state ----------------------------------------------------------

yystate31:
	c = l.next()
yystart31:
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate32
	}

yystate32:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '\n':
		goto yystate33
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate32
	}

yystate33:
	c = l.next()
	goto yyrule8

	// ---- sLabels state ---------------------------------------------------------

yystate34:
	c = l.next()
yystart34:
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate35
	case c == ',':
		goto yystate38
	case c == '=':
		goto yystate39
	case c == '}':
		goto yystate41
	case c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate40
	}

yystate35:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate36
	case c == '\\':
		goto yystate37
	case c >= '\x01' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
		goto yystate35
	}

yystate36:
	c = l.next()
	goto yyrule13

yystate37:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate35
	}

yystate38:
	c = l.next()
	goto yyrule16

yystate39:
	c = l.next()
	goto yyrule15

yystate40:
	c = l.next()
	switch {
	default:
		goto yyrule12
	case c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate40
	}

yystate41:
	c = l.next()
	goto yyrule14

	// ---- sLValue state ---------------------------------------------------------

yystate42:
	c = l.next()
yystart42:
	switch c {
	default:
		goto yyabort
	case '"':
		goto yystate43
	}

yystate43:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate44
	case c == '\\':
		goto yystate45
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
		goto yystate43
	}

yystate44:
	c = l.next()
	goto yyrule17

yystate45:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate43
	}

	// ---- sValue state ----------------------------------------------------------

yystate46:
	c = l.next()
yystart46:
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate47
	case '{':
		goto yystate49
	}

yystate47:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate48
	}

yystate48:
	c = l.next()
	switch {
	default:
		goto yyrule18
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate48
	}

yystate49:
	c = l.next()
	goto yyrule10

	// ---- sTimestamp state ------------------------------------------------------
	//
	// OM2 additions vs OM1:
	//   - After "{S}", if next chars are "st@", return tStartTimestamp instead of
	//     tTimestamp.
	//   - States 75-79 implement the "st@" recognition.

yystate50:
	c = l.next()
yystart50:
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate52
	case '\n':
		goto yystate51
	}

yystate51:
	c = l.next()
	goto yyrule20

	// yystate52: space consumed in sTimestamp; branch on first char of next token.
yystate52:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '#':
		goto yystate54
	case c == 's':
		goto yystate75 // might be "st@<float>"
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c == '!' || c == '"' || c >= '$' && c < 's' || c > 's' && c <= 'ÿ':
		goto yystate53
	}

	// yystate53: consuming chars of a plain tTimestamp value.
yystate53:
	c = l.next()
	switch {
	default:
		goto yyrule19
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate53
	}

	// yystate54–56: " # {" → tComment (exemplar).
yystate54:
	c = l.next()
	switch {
	default:
		goto yyrule19
	case c == ' ':
		goto yystate55
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= '"' || c >= '$' && c <= 'ÿ':
		goto yystate53
	}

yystate55:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case '{':
		goto yystate56
	}

yystate56:
	c = l.next()
	goto yyrule21

	// ---- sTimestamp new states for "st@<float>" recognition --------------------

	// yystate75: consumed " s"; check if next char is 't'.
yystate75:
	c = l.next()
	switch {
	default:
		goto yyrule19 // " s" alone → tTimestamp
	case c == 't':
		goto yystate76
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c < 't' || c > 't' && c <= 'ÿ':
		goto yystate79 // " s<non-t>" → fall through as plain tTimestamp
	}

	// yystate76: consumed " st"; check if next char is '@'.
yystate76:
	c = l.next()
	switch {
	default:
		goto yyrule19 // " st" alone → tTimestamp
	case c == '@':
		goto yystate77
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c < '@' || c > '@' && c <= 'ÿ':
		goto yystate79 // " st<non-@>" → plain tTimestamp
	}

	// yystate77: consumed " st@"; need at least one body char.
yystate77:
	c = l.next()
	switch {
	default:
		goto yyabort // " st@" with nothing after is invalid
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate78
	}

	// yystate78: consuming body of " st@<chars>".
yystate78:
	c = l.next()
	switch {
	default:
		goto yyrule31 // end of non-space chars → tStartTimestamp
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate78
	}

	// yystate79: "no-advance" fallback when we have a plain timestamp char in
	// hand (c already set by the caller state).  Equivalent to re-entering
	// yystate53's body without calling l.next() first.
yystate79:
	switch {
	default:
		goto yyrule19 // current char is a delimiter → return tTimestamp
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate53 // advance and keep consuming
	}

	// ---- sExemplar state -------------------------------------------------------

yystate57:
	c = l.next()
yystart57:
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate58
	case c == ',':
		goto yystate61
	case c == '=':
		goto yystate62
	case c == '}':
		goto yystate64
	case c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate63
	}

yystate58:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate59
	case c == '\\':
		goto yystate60
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
		goto yystate58
	}

yystate59:
	c = l.next()
	goto yyrule23

yystate60:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate58
	}

yystate61:
	c = l.next()
	goto yyrule27

yystate62:
	c = l.next()
	goto yyrule25

yystate63:
	c = l.next()
	switch {
	default:
		goto yyrule22
	case c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
		goto yystate63
	}

yystate64:
	c = l.next()
	goto yyrule24

	// ---- sEValue state ---------------------------------------------------------

yystate65:
	c = l.next()
yystart65:
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate66
	case '"':
		goto yystate68
	}

yystate66:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate67
	}

yystate67:
	c = l.next()
	switch {
	default:
		goto yyrule28
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate67
	}

yystate68:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '"':
		goto yystate69
	case c == '\\':
		goto yystate70
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
		goto yystate68
	}

yystate69:
	c = l.next()
	goto yyrule26

yystate70:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
		goto yystate68
	}

	// ---- sETimestamp state -----------------------------------------------------
	//
	// OM2 addition: after the exemplar timestamp we may see another exemplar
	// introduced by "{S}#{S}\{".  States 80-83 implement this.

yystate71:
	c = l.next()
yystart71:
	switch c {
	default:
		goto yyabort
	case ' ':
		goto yystate73
	case '\n':
		goto yystate72
	}

yystate72:
	c = l.next()
	goto yyrule30

	// yystate73: space consumed in sETimestamp; branch on first char of next token.
yystate73:
	c = l.next()
	switch {
	default:
		goto yyabort
	case c == '#':
		goto yystate80 // might be another exemplar " # {"
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c == '!' || c == '"' || c >= '$' && c <= 'ÿ':
		goto yystate74
	}

	// yystate74: consuming chars of a plain tTimestamp in sETimestamp.
yystate74:
	c = l.next()
	switch {
	default:
		goto yyrule29
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate74
	}

	// ---- sETimestamp new states for multiple-exemplar recognition --------------

	// yystate80: consumed " #" in sETimestamp; check if next char is ' '.
yystate80:
	c = l.next()
	switch {
	default:
		goto yyrule29 // " #" alone → tTimestamp
	case c == ' ':
		goto yystate81
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c == '!' || c == '"' || c >= '$' && c <= 'ÿ':
		goto yystate83 // " #<non-space>" → plain tTimestamp (use current c)
	}

	// yystate81: consumed " # " in sETimestamp; need '{'.
yystate81:
	c = l.next()
	switch c {
	default:
		goto yyabort
	case '{':
		goto yystate82
	}

	// yystate82: consumed " # {" in sETimestamp → return tComment.
yystate82:
	c = l.next()
	goto yyrule32

	// yystate83: "no-advance" fallback for sETimestamp timestamp body.
yystate83:
	switch {
	default:
		goto yyrule29
	case c >= '\x01' && c <= '\t' || c >= '\v' && c <= '\x1f' || c >= '!' && c <= 'ÿ':
		goto yystate74
	}

	// ---- Rules (actions) -------------------------------------------------------

yyrule1: // #{S}
	{
		l.state = sComment
		goto yystate0
	}
yyrule2: // <sComment>HELP{S}
	{
		l.state = sMeta1
		return tHelp
	}
yyrule3: // <sComment>TYPE{S}
	{
		l.state = sMeta1
		return tType
	}
yyrule4: // <sComment>UNIT{S}
	{
		l.state = sMeta1
		return tUnit
	}
yyrule5: // <sComment>"EOF"\n?
	{
		l.state = sInit
		return tEOFWord
	}
yyrule6: // <sMeta1>\"(\\.|[^\\"])*\"
	{
		l.state = sMeta2
		return tMName
	}
yyrule7: // <sMeta1>{M}({M}|{D})*
	{
		l.state = sMeta2
		return tMName
	}
yyrule8: // <sMeta2>{S}{C}*\n
	{
		l.state = sInit
		return tText
	}
yyrule9: // {M}({M}|{D})*
	{
		l.state = sValue
		return tMName
	}
yyrule10: // <sValue>\{
	{
		l.state = sLabels
		return tBraceOpen
	}
yyrule11: // \{
	{
		l.state = sLabels
		return tBraceOpen
	}
yyrule12: // <sLabels>{L}({L}|{D})*
	{
		return tLName
	}
yyrule13: // <sLabels>\"(\\.|[^\\"])*\"
	{
		l.state = sLabels
		return tQString
	}
yyrule14: // <sLabels>\}
	{
		l.state = sValue
		return tBraceClose
	}
yyrule15: // <sLabels>=
	{
		l.state = sLValue
		return tEqual
	}
yyrule16: // <sLabels>,
	{
		return tComma
	}
yyrule17: // <sLValue>\"(\\.|[^\\"\n])*\"
	{
		l.state = sLabels
		return tLValue
	}
yyrule18: // <sValue>{S}[^ \n]+
	{
		l.state = sTimestamp
		return tValue
	}
yyrule19: // <sTimestamp>{S}[^ \n]+
	{
		return tTimestamp
	}
yyrule20: // <sTimestamp>\n
	{
		l.state = sInit
		return tLinebreak
	}
yyrule21: // <sTimestamp>{S}#{S}\{
	{
		l.state = sExemplar
		return tComment
	}
yyrule22: // <sExemplar>{L}({L}|{D})*
	{
		return tLName
	}
yyrule23: // <sExemplar>\"(\\.|[^\\"\n])*\"
	{
		l.state = sExemplar
		return tQString
	}
yyrule24: // <sExemplar>\}
	{
		l.state = sEValue
		return tBraceClose
	}
yyrule25: // <sExemplar>=
	{
		l.state = sEValue
		return tEqual
	}
yyrule26: // <sEValue>\"(\\.|[^\\"\n])*\"
	{
		l.state = sExemplar
		return tLValue
	}
yyrule27: // <sExemplar>,
	{
		return tComma
	}
yyrule28: // <sEValue>{S}[^ \n]+
	{
		l.state = sETimestamp
		return tValue
	}
yyrule29: // <sETimestamp>{S}[^ \n]+
	{
		return tTimestamp
	}
yyrule30: // <sETimestamp>\n
	if true { // avoid go vet determining the below panic will not be reached
		l.state = sInit
		return tLinebreak
	}
	panic("unreachable")

	// OM2-specific rules:

yyrule31: // <sTimestamp>{S}st@[^ \n]+
	{
		l.state = sTimestamp
		return tStartTimestamp
	}
yyrule32: // <sETimestamp>{S}#{S}\{
	{
		l.state = sExemplar
		return tComment
	}

yyabort: // no lexem recognized
	// Silence unused-label errors and satisfy go vet reachability analysis.
	{
		if false {
			goto yyabort
		}
		if false {
			goto yystate0
		}
		if false {
			goto yystate1
		}
		if false {
			goto yystate6
		}
		if false {
			goto yystate26
		}
		if false {
			goto yystate31
		}
		if false {
			goto yystate34
		}
		if false {
			goto yystate42
		}
		if false {
			goto yystate46
		}
		if false {
			goto yystate50
		}
		if false {
			goto yystate57
		}
		if false {
			goto yystate65
		}
		if false {
			goto yystate71
		}
	}

	return tInvalid
}
