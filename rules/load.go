// Copyright 2013 Prometheus Team
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

package rules

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/prometheus/prometheus/rules/ast"
)

type RulesLexer struct {
	// Errors encountered during parsing.
	errors []string
	// Dummy token to simulate multiple start symbols (see below).
	startToken int
	// Parsed full rules.
	parsedRules []Rule
	// Parsed single expression.
	parsedExpr ast.Node

	// Current character.
	current byte
	// Current token buffer.
	buf []byte
	// Input text.
	src *bufio.Reader
	// Whether we have a current char.
	empty bool

	// Current input line.
	line int
	// Current character position within the current input line.
	pos int
}

func (lexer *RulesLexer) Error(errorStr string) {
	err := fmt.Sprintf("Error parsing rules at line %v, char %v: %v", lexer.line, lexer.pos, errorStr)
	lexer.errors = append(lexer.errors, err)
}

func (lexer *RulesLexer) getChar() byte {
	if lexer.current != 0 {
		lexer.buf = append(lexer.buf, lexer.current)
	}
	lexer.current = 0
	if b, err := lexer.src.ReadByte(); err == nil {
		if b == '\n' {
			lexer.line++
			lexer.pos = 0
		} else {
			lexer.pos++
		}
		lexer.current = b
	} else if err != io.EOF {
		log.Fatal(err)
	}
	return lexer.current
}

func (lexer *RulesLexer) token() string {
	return string(lexer.buf)
}

func newRulesLexer(src io.Reader, singleExpr bool) *RulesLexer {
	lexer := &RulesLexer{
		startToken: START_RULES,
		src:        bufio.NewReader(src),
		pos:        1,
		line:       1,
	}

	if singleExpr {
		lexer.startToken = START_EXPRESSION
	}
	lexer.getChar()
	return lexer
}

func LoadFromReader(rulesReader io.Reader, singleExpr bool) (interface{}, error) {
	lexer := newRulesLexer(rulesReader, singleExpr)
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
}

func LoadRulesFromReader(rulesReader io.Reader) ([]Rule, error) {
	expr, err := LoadFromReader(rulesReader, false)
	if err != nil {
		return nil, err
	}
	return expr.([]Rule), err
}

func LoadRulesFromString(rulesString string) ([]Rule, error) {
	rulesReader := strings.NewReader(rulesString)
	return LoadRulesFromReader(rulesReader)
}

func LoadRulesFromFile(fileName string) ([]Rule, error) {
	rulesReader, err := os.Open(fileName)
	if err != nil {
		return []Rule{}, err
	}
	defer rulesReader.Close()
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
	if err != nil {
		return nil, err
	}
	defer exprReader.Close()
	return LoadExprFromReader(exprReader)
}
