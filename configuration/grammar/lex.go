package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type itemType int

const (
	itemError itemType = iota
	itemEof
	itemRuleContextOpen
	itemRuleContextClose
	itemSetKeyword
	itemKey
	itemEqual
	itemValue
	itemSemicolon
	itemQuote
	itemRulesKeyword
	itemRuleKeyword
	itemRuleName
	itemRuleValue
	itemOpenBracket
	itemCloseBracket
)

const (
	literalCloseBracket = "}"
	literalEof          = -1
	literalOpenBracket  = "{"
	literalRule         = "rule"
	literalRules        = "rules"
	literalSet          = "set"
	literalEqual        = "="
	literalQuote        = `"`
	literalSemicolon    = ";"
)

type element struct {
	itemType itemType
	value    string
}

func (e element) String() string {
	switch e.itemType {
	case itemError:
		return e.value
	case itemEof:
		return "EOF"
	}
	return fmt.Sprintf("%s %q", e.itemType, e.value)
}

type lexer struct {
	input     string
	elements  chan element
	start     int
	position  int
	runeWidth int
}

func lex(name, input string) (*lexer, chan element) {
	l := &lexer{
		input:    input,
		elements: make(chan element),
	}
	go l.run()
	return l, l.elements
}

func (l *lexer) run() {
	for state := lexBody; state != nil; {
		state = state(l)
	}
	close(l.elements)
}

func (l *lexer) next() (rune rune) {
	if l.position >= len(l.input) {
		l.runeWidth = 0
		return literalEof
	}

	rune, l.runeWidth = utf8.DecodeRuneInString(l.input[l.position:])
	l.position += l.runeWidth
	return rune
}

func lexBody(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalOpenBracket) {
		return lexRulesetOpen
	}

	switch rune := l.next(); {
	case rune == literalEof:
		l.emit(itemEof)
		return nil
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("illegal input")
	}
	return lexBody
}

func lexRulesetOpen(l *lexer) lexFunction {
	l.position += len(literalOpenBracket)
	l.emit(itemRuleContextOpen)

	return lexRulesetInside
}

func lexRulesetInside(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalCloseBracket) {
		return lexRulesetClose
	}

	if strings.HasPrefix(l.input[l.position:], literalSet) {
		return lexRuleSetKeyword
	}

	if strings.HasPrefix(l.input[l.position:], literalRules) {
		return lexRulesetRules
	}

	switch rune := l.next(); {
	case rune == literalEof:
		return l.errorf("unterminated ruleset")
	case isSpace(rune):
		l.ignore()
	case rune == ';':
		l.ignore()
	default:
		return l.errorf("unrecognized input")
	}
	return lexRulesetInside
}

func lexRulesetRules(l *lexer) lexFunction {
	l.position += len(literalRules)
	l.emit(itemRulesKeyword)

	return lexRulesetRulesBlockOpen
}

func lexRulesetRulesBlockOpen(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalOpenBracket) {
		l.position += len(literalOpenBracket)
		l.emit(itemOpenBracket)
		return lexRulesetRulesBlockInside
	}

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unrecognized input")
	}

	return lexRulesetRulesBlockOpen
}

func lexRulesetRulesBlockInside(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalRule) {
		return lexRulesetRuleBegin
	}

	if strings.HasPrefix(l.input[l.position:], literalCloseBracket) {
		return lexRulesetRulesBlockClose
	}

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unrecognized input")
	}

	return lexRulesetRulesBlockInside
}

func lexRulesetRulesBlockClose(l *lexer) lexFunction {
	l.position += len(literalCloseBracket)
	l.emit(itemCloseBracket)

	return lexRulesetInside
}

func lexRulesetRuleBegin(l *lexer) lexFunction {
	l.position += len(literalRule)
	l.emit(itemRuleKeyword)

	return lexRulesetRuleName
}

func lexRulesetRuleName(l *lexer) lexFunction {

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	case isIdentifierOpen(rune):
		for {
			switch rune := l.next(); {
			case isMetricIdentifier(rune):
			case rune == '=':
				l.backup()
				l.emit(itemRuleName)
				return lexRulesetRuleEqual
			default:
				return l.errorf("bad rule name")
			}
		}
	default:
		return l.errorf("unrecognized input")
	}

	return lexRulesetRuleName
}

func lexRulesetRuleEqual(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalEqual) {
		l.position += len(literalEqual)
		l.emit(itemEqual)
		return lexRulesetRuleDefinitionBegin
	}

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unrecognized input")
	}

	return lexRulesetRuleEqual
}

func lexRulesetRuleDefinitionBegin(l *lexer) lexFunction {
	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	case isIdentifierOpen(rune):
		for {
			switch rune := l.next(); {
			case isMetricIdentifier(rune):
			case rune == ';':
				l.emit(itemRuleValue)
				return lexRulesetRulesBlockInside
			default:
				return l.errorf("unrecognized input")
			}
		}
	default:
		return l.errorf("unrecognized input")
	}

	return lexRulesetRuleDefinitionBegin
}

func lexRuleSetKeyword(l *lexer) lexFunction {
	l.position += len(literalSet)

	l.emit(itemSetKeyword)

	return lexRuleSetInside
}

func (l *lexer) backup() {
	l.position -= l.runeWidth
}

func isIdentifierOpen(rune rune) bool {
	switch rune := rune; {
	case unicode.IsLetter(rune):
		return true
	case rune == '_':
		return true
	}

	return false
}

func lexRuleSetInside(l *lexer) lexFunction {
	switch rune := l.next(); {
	case rune == literalEof:
		return l.errorf("unterminated set statement")
	case isSpace(rune):
		l.ignore()
	case rune == ';':
		return l.errorf("unexpected ;")
	case rune == '=':
		return l.errorf("unexpected =")
	case isIdentifierOpen(rune):
		l.backup()
		return lexRuleSetKey
	default:
		return l.errorf("unrecognized input")
	}

	return lexRuleSetInside
}

func isIdentifier(rune rune) bool {
	switch rune := rune; {
	case isIdentifierOpen(rune):
		return true
	case unicode.IsDigit(rune):
		return true
	}
	return false
}

func isMetricIdentifier(rune rune) bool {
	switch rune := rune; {
	case isIdentifier(rune):
		return true
	case rune == ':':
		return true
	}

	return false
}

func (l *lexer) peek() rune {
	rune := l.next()
	l.backup()
	return rune
}

func (l *lexer) atTerminator() bool {
	switch rune := l.peek(); {
	case isSpace(rune):
		return true
	case rune == ';':
		return true
	}

	return false
}

func lexRuleSetKey(l *lexer) lexFunction {
	switch rune := l.next(); {
	case rune == literalEof:
		return l.errorf("incomplete set statement")
	case isIdentifier(rune):
	default:
		l.backup()
		if !l.atTerminator() {
			return l.errorf("unexpected character %+U %q", rune, rune)
		}
		l.emit(itemKey)
		return lexRuleSetEqual
	}
	return lexRuleSetKey
}

func lexRuleSetEqual(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalEqual) {
		l.position += len(literalEqual)
		l.emit(itemEqual)
		return lexRuleSetValueOpenQuote
	}

	switch rune := l.next(); {
	case rune == literalEof:
		return l.errorf("incomplete set statement")
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unexpected character %+U %q", rune, rune)
	}
	return lexRuleSetEqual
}

func lexRuleSetValueOpenQuote(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalQuote) {
		l.position += len(literalQuote)
		l.emit(itemQuote)

		return lexRuleSetValue
	}

	switch rune := l.next(); {
	case rune == literalEof:
		return l.errorf("incomplete set statement")
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unexpected character %+U %q", rune, rune)
	}
	return lexRuleSetValueOpenQuote
}

func lexRuleSetValue(l *lexer) lexFunction {
	var lastRuneEscapes bool = false

	for {
		rune := l.next()
		{
			if rune == '"' && !lastRuneEscapes {
				l.backup()
				l.emit(itemValue)
				return lexRuleSetValueCloseQuote
			}

			if !lastRuneEscapes && rune == '\\' {
				lastRuneEscapes = true
			} else {
				lastRuneEscapes = false
			}
		}
	}

	panic("unreachable")
}

func lexRuleSetValueCloseQuote(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalQuote) {
		l.position += len(literalQuote)
		l.emit(itemQuote)

		return lexRuleSetSemicolon
	}

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unexpected character %+U %q", rune, rune)

	}
	return lexRuleSetValueCloseQuote

}

func lexRuleSetSemicolon(l *lexer) lexFunction {
	if strings.HasPrefix(l.input[l.position:], literalSemicolon) {
		l.position += len(literalSemicolon)
		l.emit(itemSemicolon)
		return lexRulesetInside
	}

	switch rune := l.next(); {
	case isSpace(rune):
		l.ignore()
	default:
		return l.errorf("unexpected character %+U %q", rune, rune)
	}
	return lexRuleSetSemicolon
}

func (l *lexer) ignore() {
	l.start = l.position
}

func (l *lexer) errorf(format string, args ...interface{}) lexFunction {
	l.elements <- element{itemError, fmt.Sprintf(format, args...)}
	return nil
}

func isSpace(rune rune) bool {
	switch rune {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

func lexRulesetClose(l *lexer) lexFunction {
	l.position += len(literalCloseBracket)
	l.emit(itemCloseBracket)

	return lexBody
}

func (l *lexer) emit(i itemType) {
	l.elements <- element{i, l.input[l.start:l.position]}
	l.start = l.position
}

type lexFunction func(*lexer) lexFunction

func main() {
	in := `{
	set evaluation_interval = "10m";

     rules {
     }


  set name = "your mom";


	}
  {
	set evaluation_interval = "30m";
	}`
	fmt.Println(in)
	_, v := lex("", in)
	for value := range v {
		fmt.Println(value)
	}
}
