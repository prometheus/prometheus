package rules

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// NOTE: This parser is non-reentrant due to its dependence on global state.

// GoLex sadly needs these global variables for storing temporary token/parsing information.
var yylval *yySymType   // For storing extra token information, like the contents of a string.
var yyline int          // Line number within the current file or buffer.
var yypos int           // Character position within the current line.
var parsedRules []*Rule // Parsed rules.

type RulesLexer struct {
	errors []string
}

func addRule(rule *Rule) {
	parsedRules = append(parsedRules, rule)
}

func (lexer *RulesLexer) Lex(lval *yySymType) int {
	yylval = lval
	token_type := yylex()
	return token_type
}

func (lexer *RulesLexer) Error(errorStr string) {
	err := fmt.Sprintf("Error parsing rules at line %v, char %v: %v", yyline, yypos, errorStr)
	lexer.errors = append(lexer.errors, err)
}

func LoadFromReader(rulesReader io.Reader) ([]*Rule, error) {
	yyin = rulesReader
	yypos = 1
	yyline = 1

	parsedRules = []*Rule{}
	lexer := &RulesLexer{}
	ret := yyParse(lexer)
	if ret != 0 && len(lexer.errors) == 0 {
		lexer.Error("Unknown parser error")
	}

	if len(lexer.errors) > 0 {
		err := errors.New(strings.Join(lexer.errors, "\n"))
		return []*Rule{}, err
	}

	return parsedRules, nil
}

func LoadFromString(rulesString string) ([]*Rule, error) {
	rulesReader := strings.NewReader(rulesString)
	return LoadFromReader(rulesReader)
}

func LoadFromFile(fileName string) ([]*Rule, error) {
	rulesReader, err := os.Open(fileName)
	if err != nil {
		return []*Rule{}, err
	}
	return LoadFromReader(rulesReader)
}
