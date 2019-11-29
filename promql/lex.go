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
	"strings"
	"unicode"
	"unicode/utf8"
)

// item represents a token or text string returned from the scanner.
type item struct {
	typ ItemType // The type of this item.
	pos Pos      // The starting position, in bytes, of this item in the input string.
	val string   // The value of this item.
}

// String returns a descriptive string for the item.
func (i item) String() string {
	switch {
	case i.typ == EOF:
		return "EOF"
	case i.typ == ERROR:
		return i.val
	case i.typ == IDENTIFIER || i.typ == METRIC_IDENTIFIER:
		return fmt.Sprintf("%q", i.val)
	case i.typ.isKeyword():
		return fmt.Sprintf("<%s>", i.val)
	case i.typ.isOperator():
		return fmt.Sprintf("<op:%s>", i.val)
	case i.typ.isAggregator():
		return fmt.Sprintf("<aggr:%s>", i.val)
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

// isOperator returns true if the item corresponds to a arithmetic or set operator.
// Returns false otherwise.
func (i ItemType) isOperator() bool { return i > operatorsStart && i < operatorsEnd }

// isAggregator returns true if the item belongs to the aggregator functions.
// Returns false otherwise
func (i ItemType) isAggregator() bool { return i > aggregatorsStart && i < aggregatorsEnd }

// isAggregator returns true if the item is an aggregator that takes a parameter.
// Returns false otherwise
func (i ItemType) isAggregatorWithParam() bool {
	return i == TOPK || i == BOTTOMK || i == COUNT_VALUES || i == QUANTILE
}

// isKeyword returns true if the item corresponds to a keyword.
// Returns false otherwise.
func (i ItemType) isKeyword() bool { return i > keywordsStart && i < keywordsEnd }

// isComparisonOperator returns true if the item corresponds to a comparison operator.
// Returns false otherwise.
func (i ItemType) isComparisonOperator() bool {
	switch i {
	case EQL, NEQ, LTE, LSS, GTE, GTR:
		return true
	default:
		return false
	}
}

// isSetOperator returns whether the item corresponds to a set operator.
func (i ItemType) isSetOperator() bool {
	switch i {
	case LAND, LOR, LUNLESS:
		return true
	}
	return false
}

// LowestPrec is a constant for operator precedence in expressions.
const LowestPrec = 0 // Non-operators.

// Precedence returns the operator precedence of the binary
// operator op. If op is not a binary operator, the result
// is LowestPrec.
func (i ItemType) precedence() int {
	switch i {
	case LOR:
		return 1
	case LAND, LUNLESS:
		return 2
	case EQL, NEQ, LTE, LSS, GTE, GTR:
		return 3
	case ADD, SUB:
		return 4
	case MUL, DIV, MOD:
		return 5
	case POW:
		return 6
	default:
		return LowestPrec
	}
}

func (i ItemType) isRightAssociative() bool {
	switch i {
	case POW:
		return true
	default:
		return false
	}

}

type ItemType int

const (
	ERROR ItemType = iota // Error occurred, value is error message
	EOF
	COMMENT
	IDENTIFIER
	METRIC_IDENTIFIER
	LEFT_PAREN
	RIGHT_PAREN
	LEFT_BRACE
	RIGHT_BRACE
	LEFT_BRACKET
	RIGHT_BRACKET
	COMMA
	ASSIGN
	COLON
	SEMICOLON
	STRING
	NUMBER
	DURATION
	BLANK
	TIMES
	SPACE

	operatorsStart
	// Operators.
	SUB
	ADD
	MUL
	MOD
	DIV
	LAND
	LOR
	LUNLESS
	EQL
	NEQ
	LTE
	LSS
	GTE
	GTR
	EQL_REGEX
	NEQ_REGEX
	POW
	operatorsEnd

	aggregatorsStart
	// Aggregators.
	AVG
	COUNT
	SUM
	MIN
	MAX
	STDDEV
	STDVAR
	TOPK
	BOTTOMK
	COUNT_VALUES
	QUANTILE
	aggregatorsEnd

	keywordsStart
	// Keywords.
	OFFSET
	BY
	WITHOUT
	ON
	IGNORING
	GROUP_LEFT
	GROUP_RIGHT
	BOOL
	keywordsEnd

	startSymbolsStart
	// Start symbols for the generated parser.
	startSymbolsEnd
)

var key = map[string]ItemType{
	// Operators.
	"and":    LAND,
	"or":     LOR,
	"unless": LUNLESS,

	// Aggregators.
	"sum":          SUM,
	"avg":          AVG,
	"count":        COUNT,
	"min":          MIN,
	"max":          MAX,
	"stddev":       STDDEV,
	"stdvar":       STDVAR,
	"topk":         TOPK,
	"bottomk":      BOTTOMK,
	"count_values": COUNT_VALUES,
	"quantile":     QUANTILE,

	// Keywords.
	"offset":      OFFSET,
	"by":          BY,
	"without":     WITHOUT,
	"on":          ON,
	"ignoring":    IGNORING,
	"group_left":  GROUP_LEFT,
	"group_right": GROUP_RIGHT,
	"bool":        BOOL,
}

// These are the default string representations for common items. It does not
// imply that those are the only character sequences that can be lexed to such an item.
var itemTypeStr = map[ItemType]string{
	LEFT_PAREN:    "(",
	RIGHT_PAREN:   ")",
	LEFT_BRACE:    "{",
	RIGHT_BRACE:   "}",
	LEFT_BRACKET:  "[",
	RIGHT_BRACKET: "]",
	COMMA:         ",",
	ASSIGN:        "=",
	COLON:         ":",
	SEMICOLON:     ";",
	BLANK:         "_",
	TIMES:         "x",
	SPACE:         "<space>",

	SUB:       "-",
	ADD:       "+",
	MUL:       "*",
	MOD:       "%",
	DIV:       "/",
	EQL:       "==",
	NEQ:       "!=",
	LTE:       "<=",
	LSS:       "<",
	GTE:       ">=",
	GTR:       ">",
	EQL_REGEX: "=~",
	NEQ_REGEX: "!~",
	POW:       "^",
}

func init() {
	// Add keywords to item type strings.
	for s, ty := range key {
		itemTypeStr[ty] = s
	}
	// Special numbers.
	key["inf"] = NUMBER
	key["nan"] = NUMBER
}

func (i ItemType) String() string {
	if s, ok := itemTypeStr[i]; ok {
		return s
	}
	return fmt.Sprintf("<item %d>", i)
}

func (i item) desc() string {
	if _, ok := itemTypeStr[i.typ]; ok {
		return i.String()
	}
	if i.typ == EOF {
		return i.typ.desc()
	}
	return fmt.Sprintf("%s %s", i.typ.desc(), i)
}

func (i ItemType) desc() string {
	switch i {
	case ERROR:
		return "error"
	case EOF:
		return "end of input"
	case COMMENT:
		return "comment"
	case IDENTIFIER:
		return "identifier"
	case METRIC_IDENTIFIER:
		return "metric identifier"
	case STRING:
		return "string"
	case NUMBER:
		return "number"
	case DURATION:
		return "duration"
	}
	return fmt.Sprintf("%q", i)
}

const eof = -1

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*lexer) stateFn

// Pos is the position in a string.
type Pos int

// lexer holds the state of the scanner.
type lexer struct {
	input   string  // The string being scanned.
	state   stateFn // The next lexing function to enter.
	pos     Pos     // Current position in the input.
	start   Pos     // Start position of this item.
	width   Pos     // Width of last rune read from input.
	lastPos Pos     // Position of most recent item returned by nextItem.
	items   []item  // Slice buffer of scanned items.

	parenDepth  int  // Nesting depth of ( ) exprs.
	braceOpen   bool // Whether a { is opened.
	bracketOpen bool // Whether a [ is opened.
	gotColon    bool // Whether we got a ':' after [ was opened.
	stringOpen  rune // Quote rune of the string currently being read.

	// seriesDesc is set when a series description for the testing
	// language is lexed.
	seriesDesc bool
}

// next returns the next rune in the input.
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
}

// emit passes an item back to the client.
func (l *lexer) emit(t ItemType) {
	l.items = append(l.items, item{t, l.start, l.input[l.start:l.pos]})
	l.start = l.pos
}

// ignore skips over the pending input before this point.
func (l *lexer) ignore() {
	l.start = l.pos
}

// accept consumes the next rune if it's from the valid set.
func (l *lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes from the valid set.
func (l *lexer) acceptRun(valid string) {
	for strings.ContainsRune(valid, l.next()) {
		// consume
	}
	l.backup()
}

// lineNumber reports which line we're on, based on the position of
// the previous item returned by nextItem. Doing it this way
// means we don't have to worry about peek double counting.
func (l *lexer) lineNumber() int {
	return 1 + strings.Count(l.input[:l.lastPos], "\n")
}

// linePosition reports at which character in the current line
// we are on.
func (l *lexer) linePosition() int {
	lb := strings.LastIndex(l.input[:l.lastPos], "\n")
	if lb == -1 {
		return 1 + int(l.lastPos)
	}
	return 1 + int(l.lastPos) - lb
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items = append(l.items, item{ERROR, l.start, fmt.Sprintf(format, args...)})
	return nil
}

// nextItem returns the next item from the input.
func (l *lexer) nextItem() item {
	for len(l.items) == 0 {
		if l.state != nil {
			l.state = l.state(l)
		} else {
			l.emit(EOF)
		}
	}
	item := l.items[0]
	l.items = l.items[1:]
	l.lastPos = item.pos
	return item
}

// lex creates a new scanner for the input string.
func lex(input string) *lexer {
	l := &lexer{
		input: input,
		state: lexStatements,
	}
	return l
}

// run runs the state machine for the lexer.
func (l *lexer) run() {
	for l.state = lexStatements; l.state != nil; {
		l.state = l.state(l)
	}
}

// Release resources used by lexer.
func (l *lexer) close() {
	for range l.items {
		// Consume.
	}
}

// lineComment is the character that starts a line comment.
const lineComment = "#"

// lexStatements is the top-level state for lexing.
func lexStatements(l *lexer) stateFn {
	if l.braceOpen {
		return lexInsideBraces
	}
	if strings.HasPrefix(l.input[l.pos:], lineComment) {
		return lexLineComment
	}

	switch r := l.next(); {
	case r == eof:
		if l.parenDepth != 0 {
			return l.errorf("unclosed left parenthesis")
		} else if l.bracketOpen {
			return l.errorf("unclosed left bracket")
		}
		l.emit(EOF)
		return nil
	case r == ',':
		l.emit(COMMA)
	case isSpace(r):
		return lexSpace
	case r == '*':
		l.emit(MUL)
	case r == '/':
		l.emit(DIV)
	case r == '%':
		l.emit(MOD)
	case r == '+':
		l.emit(ADD)
	case r == '-':
		l.emit(SUB)
	case r == '^':
		l.emit(POW)
	case r == '=':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(EQL)
		} else if t == '~' {
			return l.errorf("unexpected character after '=': %q", t)
		} else {
			l.emit(ASSIGN)
		}
	case r == '!':
		if t := l.next(); t == '=' {
			l.emit(NEQ)
		} else {
			return l.errorf("unexpected character after '!': %q", t)
		}
	case r == '<':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(LTE)
		} else {
			l.emit(LSS)
		}
	case r == '>':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(GTE)
		} else {
			l.emit(GTR)
		}
	case isDigit(r) || (r == '.' && isDigit(l.peek())):
		l.backup()
		return lexNumberOrDuration
	case r == '"' || r == '\'':
		l.stringOpen = r
		return lexString
	case r == '`':
		l.stringOpen = r
		return lexRawString
	case isAlpha(r) || r == ':':
		if !l.bracketOpen {
			l.backup()
			return lexKeywordOrIdentifier
		}
		if l.gotColon {
			return l.errorf("unexpected colon %q", r)
		}
		l.emit(COLON)
		l.gotColon = true
	case r == '(':
		l.emit(LEFT_PAREN)
		l.parenDepth++
		return lexStatements
	case r == ')':
		l.emit(RIGHT_PAREN)
		l.parenDepth--
		if l.parenDepth < 0 {
			return l.errorf("unexpected right parenthesis %q", r)
		}
		return lexStatements
	case r == '{':
		l.emit(LEFT_BRACE)
		l.braceOpen = true
		return lexInsideBraces(l)
	case r == '[':
		if l.bracketOpen {
			return l.errorf("unexpected left bracket %q", r)
		}
		l.gotColon = false
		l.emit(LEFT_BRACKET)
		if isSpace(l.peek()) {
			skipSpaces(l)
		}
		l.bracketOpen = true
		return lexDuration
	case r == ']':
		if !l.bracketOpen {
			return l.errorf("unexpected right bracket %q", r)
		}
		l.emit(RIGHT_BRACKET)
		l.bracketOpen = false

	default:
		return l.errorf("unexpected character: %q", r)
	}
	return lexStatements
}

// lexInsideBraces scans the inside of a vector selector. Keywords are ignored and
// scanned as identifiers.
func lexInsideBraces(l *lexer) stateFn {
	if strings.HasPrefix(l.input[l.pos:], lineComment) {
		return lexLineComment
	}

	switch r := l.next(); {
	case r == eof:
		return l.errorf("unexpected end of input inside braces")
	case isSpace(r):
		return lexSpace
	case isAlpha(r):
		l.backup()
		return lexIdentifier
	case r == ',':
		l.emit(COMMA)
	case r == '"' || r == '\'':
		l.stringOpen = r
		return lexString
	case r == '`':
		l.stringOpen = r
		return lexRawString
	case r == '=':
		if l.next() == '~' {
			l.emit(EQL_REGEX)
			break
		}
		l.backup()
		l.emit(EQL)
	case r == '!':
		switch nr := l.next(); {
		case nr == '~':
			l.emit(NEQ_REGEX)
		case nr == '=':
			l.emit(NEQ)
		default:
			return l.errorf("unexpected character after '!' inside braces: %q", nr)
		}
	case r == '{':
		return l.errorf("unexpected left brace %q", r)
	case r == '}':
		l.emit(RIGHT_BRACE)
		l.braceOpen = false

		if l.seriesDesc {
			return lexValueSequence
		}
		return lexStatements
	default:
		return l.errorf("unexpected character inside braces: %q", r)
	}
	return lexInsideBraces
}

// lexValueSequence scans a value sequence of a series description.
func lexValueSequence(l *lexer) stateFn {
	switch r := l.next(); {
	case r == eof:
		return lexStatements
	case isSpace(r):
		l.emit(SPACE)
		lexSpace(l)
	case r == '+':
		l.emit(ADD)
	case r == '-':
		l.emit(SUB)
	case r == 'x':
		l.emit(TIMES)
	case r == '_':
		l.emit(BLANK)
	case isDigit(r) || (r == '.' && isDigit(l.peek())):
		l.backup()
		lexNumber(l)
	case isAlpha(r):
		l.backup()
		// We might lex invalid items here but this will be caught by the parser.
		return lexKeywordOrIdentifier
	default:
		return l.errorf("unexpected character in series sequence: %q", r)
	}
	return lexValueSequence
}

// lexEscape scans a string escape sequence. The initial escaping character (\)
// has already been seen.
//
// NOTE: This function as well as the helper function digitVal() and associated
// tests have been adapted from the corresponding functions in the "go/scanner"
// package of the Go standard library to work for Prometheus-style strings.
// None of the actual escaping/quoting logic was changed in this function - it
// was only modified to integrate with our lexer.
func lexEscape(l *lexer) {
	var n int
	var base, max uint32

	ch := l.next()
	switch ch {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', l.stringOpen:
		return
	case '0', '1', '2', '3', '4', '5', '6', '7':
		n, base, max = 3, 8, 255
	case 'x':
		ch = l.next()
		n, base, max = 2, 16, 255
	case 'u':
		ch = l.next()
		n, base, max = 4, 16, unicode.MaxRune
	case 'U':
		ch = l.next()
		n, base, max = 8, 16, unicode.MaxRune
	case eof:
		l.errorf("escape sequence not terminated")
	default:
		l.errorf("unknown escape sequence %#U", ch)
	}

	var x uint32
	for n > 0 {
		d := uint32(digitVal(ch))
		if d >= base {
			if ch == eof {
				l.errorf("escape sequence not terminated")
			}
			l.errorf("illegal character %#U in escape sequence", ch)
		}
		x = x*base + d
		ch = l.next()
		n--
	}

	if x > max || 0xD800 <= x && x < 0xE000 {
		l.errorf("escape sequence is an invalid Unicode code point")
	}
}

// digitVal returns the digit value of a rune or 16 in case the rune does not
// represent a valid digit.
func digitVal(ch rune) int {
	switch {
	case '0' <= ch && ch <= '9':
		return int(ch - '0')
	case 'a' <= ch && ch <= 'f':
		return int(ch - 'a' + 10)
	case 'A' <= ch && ch <= 'F':
		return int(ch - 'A' + 10)
	}
	return 16 // Larger than any legal digit val.
}

// skipSpaces skips the spaces until a non-space is encountered.
func skipSpaces(l *lexer) {
	for isSpace(l.peek()) {
		l.next()
	}
	l.ignore()
}

// lexString scans a quoted string. The initial quote has already been seen.
func lexString(l *lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			lexEscape(l)
		case utf8.RuneError:
			return l.errorf("invalid UTF-8 rune")
		case eof, '\n':
			return l.errorf("unterminated quoted string")
		case l.stringOpen:
			break Loop
		}
	}
	l.emit(STRING)
	return lexStatements
}

// lexRawString scans a raw quoted string. The initial quote has already been seen.
func lexRawString(l *lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case utf8.RuneError:
			return l.errorf("invalid UTF-8 rune")
		case eof:
			return l.errorf("unterminated raw string")
		case l.stringOpen:
			break Loop
		}
	}
	l.emit(STRING)
	return lexStatements
}

// lexSpace scans a run of space characters. One space has already been seen.
func lexSpace(l *lexer) stateFn {
	for isSpace(l.peek()) {
		l.next()
	}
	l.ignore()
	return lexStatements
}

// lexLineComment scans a line comment. Left comment marker is known to be present.
func lexLineComment(l *lexer) stateFn {
	l.pos += Pos(len(lineComment))
	for r := l.next(); !isEndOfLine(r) && r != eof; {
		r = l.next()
	}
	l.backup()
	l.emit(COMMENT)
	return lexStatements
}

func lexDuration(l *lexer) stateFn {
	if l.scanNumber() {
		return l.errorf("missing unit character in duration")
	}
	// Next two chars must be a valid unit and a non-alphanumeric.
	if l.accept("smhdwy") {
		if isAlphaNumeric(l.next()) {
			return l.errorf("bad duration syntax: %q", l.input[l.start:l.pos])
		}
		l.backup()
		l.emit(DURATION)
		return lexStatements
	}
	return l.errorf("bad duration syntax: %q", l.input[l.start:l.pos])
}

// lexNumber scans a number: decimal, hex, oct or float.
func lexNumber(l *lexer) stateFn {
	if !l.scanNumber() {
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	}
	l.emit(NUMBER)
	return lexStatements
}

// lexNumberOrDuration scans a number or a duration item.
func lexNumberOrDuration(l *lexer) stateFn {
	if l.scanNumber() {
		l.emit(NUMBER)
		return lexStatements
	}
	// Next two chars must be a valid unit and a non-alphanumeric.
	if l.accept("smhdwy") {
		if isAlphaNumeric(l.next()) {
			return l.errorf("bad number or duration syntax: %q", l.input[l.start:l.pos])
		}
		l.backup()
		l.emit(DURATION)
		return lexStatements
	}
	return l.errorf("bad number or duration syntax: %q", l.input[l.start:l.pos])
}

// scanNumber scans numbers of different formats. The scanned item is
// not necessarily a valid number. This case is caught by the parser.
func (l *lexer) scanNumber() bool {
	digits := "0123456789"
	// Disallow hexadecimal in series descriptions as the syntax is ambiguous.
	if !l.seriesDesc && l.accept("0") && l.accept("xX") {
		digits = "0123456789abcdefABCDEF"
	}
	l.acceptRun(digits)
	if l.accept(".") {
		l.acceptRun(digits)
	}
	if l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789")
	}
	// Next thing must not be alphanumeric unless it's the times token
	// for series repetitions.
	if r := l.peek(); (l.seriesDesc && r == 'x') || !isAlphaNumeric(r) {
		return true
	}
	return false
}

// lexIdentifier scans an alphanumeric identifier. The next character
// is known to be a letter.
func lexIdentifier(l *lexer) stateFn {
	for isAlphaNumeric(l.next()) {
		// absorb
	}
	l.backup()
	l.emit(IDENTIFIER)
	return lexStatements
}

// lexKeywordOrIdentifier scans an alphanumeric identifier which may contain
// a colon rune. If the identifier is a keyword the respective keyword item
// is scanned.
func lexKeywordOrIdentifier(l *lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || r == ':':
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if kw, ok := key[strings.ToLower(word)]; ok {
				l.emit(kw)
			} else if !strings.Contains(word, ":") {
				l.emit(IDENTIFIER)
			} else {
				l.emit(METRIC_IDENTIFIER)
			}
			break Loop
		}
	}
	if l.seriesDesc && l.peek() != '{' {
		return lexValueSequence
	}
	return lexStatements
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// isEndOfLine reports whether r is an end-of-line character.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return isAlpha(r) || isDigit(r)
}

// isDigit reports whether r is a digit. Note: we cannot use unicode.IsDigit()
// instead because that also classifies non-Latin digits as digits. See
// https://github.com/prometheus/prometheus/issues/939.
func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

// isAlpha reports whether r is an alphabetic or underscore.
func isAlpha(r rune) bool {
	return r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

// isLabel reports whether the string can be used as label.
func isLabel(s string) bool {
	if len(s) == 0 || !isAlpha(rune(s[0])) {
		return false
	}
	for _, c := range s[1:] {
		if !isAlphaNumeric(c) {
			return false
		}
	}
	return true
}
