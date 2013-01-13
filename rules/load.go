package rules

import (
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/rules/ast"
	"io"
	"os"
	"strings"
)

// NOTE: This parser is non-reentrant due to its dependence on global state.

// GoLex sadly needs these global variables for storing temporary token/parsing information.
var yylval *yySymType // For storing extra token information, like the contents of a string.
var yyline int        // Line number within the current file or buffer.
var yypos int         // Character position within the current line.

type RulesLexer struct {
	errors      []string // Errors encountered during parsing.
	startToken  int      // Dummy token to simulate multiple start symbols (see below).
	parsedRules []*Rule  // Parsed full rules.
	parsedExpr  ast.Node // Parsed single expression.
}

func (lexer *RulesLexer) Lex(lval *yySymType) int {
	yylval = lval

	// We simulate multiple start symbols for closely-related grammars via dummy tokens. See
	// http://www.gnu.org/software/bison/manual/html_node/Multiple-start_002dsymbols.html
	// Reason: we want to be able to parse lists of named rules as well as single expressions.
	if lexer.startToken != 0 {
		startToken := lexer.startToken
		lexer.startToken = 0
		return startToken
	}

	tokenType := yylex()
	return tokenType
}

func (lexer *RulesLexer) Error(errorStr string) {
	err := fmt.Sprintf("Error parsing rules at line %v, char %v: %v", yyline, yypos, errorStr)
	lexer.errors = append(lexer.errors, err)
}

func LoadFromReader(rulesReader io.Reader, singleExpr bool) (interface{}, error) {
	yyin = rulesReader
	yypos = 1
	yyline = 1
	yydata = ""
	yytext = ""

	lexer := &RulesLexer{
		startToken: START_RULES,
	}

	if singleExpr {
		lexer.startToken = START_EXPRESSION
	}

	ret := yyParse(lexer)
	if ret != 0 && len(lexer.errors) == 0 {
		lexer.Error("Unknown parser error")
	}

	if len(lexer.errors) > 0 {
		err := errors.New(strings.Join(lexer.errors, "\n"))
		return nil, err
	}

	if singleExpr {
		return lexer.parsedExpr, nil
	} else {
		return lexer.parsedRules, nil
	}
	panic("")
}

func LoadRulesFromReader(rulesReader io.Reader) ([]*Rule, error) {
	expr, err := LoadFromReader(rulesReader, false)
	if err != nil {
		return nil, err
	}
	return expr.([]*Rule), err
}

func LoadRulesFromString(rulesString string) ([]*Rule, error) {
	rulesReader := strings.NewReader(rulesString)
	return LoadRulesFromReader(rulesReader)
}

func LoadRulesFromFile(fileName string) ([]*Rule, error) {
	rulesReader, err := os.Open(fileName)
	defer rulesReader.Close()
	if err != nil {
		return []*Rule{}, err
	}
	return LoadRulesFromReader(rulesReader)
}

func LoadExprFromReader(exprReader io.Reader) (ast.Node, error) {
	expr, err := LoadFromReader(exprReader, true)
	if err != nil {
		return nil, err
	}
	return expr.(ast.Node), err
}

func LoadExprFromString(exprString string) (ast.Node, error) {
	exprReader := strings.NewReader(exprString)
	return LoadExprFromReader(exprReader)
}

func LoadExprFromFile(fileName string) (ast.Node, error) {
	exprReader, err := os.Open(fileName)
	defer exprReader.Close()
	if err != nil {
		return nil, err
	}
	return LoadExprFromReader(exprReader)
}
