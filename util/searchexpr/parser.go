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

// Package searchexpr parses boolean search expressions that combine terms with
// NOT, AND, OR, and parentheses.
package searchexpr

import (
	"fmt"
	"strings"
)

const (
	maxExpressionBytes        = 4096
	maxExpressionTerms        = 32
	maxExpressionNestingDepth = 16
)

// Expr is a search expression AST node.
//
// The interface is sealed so that Term, And, Or, and Not are the only node
// types and consumers can handle every expression this package produces.
type Expr interface {
	expr()
}

// Term matches one search term.
type Term struct {
	Value string
}

func (Term) expr() {}

// And accepts candidates accepted by both children.
type And struct {
	Left  Expr
	Right Expr
}

func (And) expr() {}

// Or accepts candidates accepted by either child.
type Or struct {
	Left  Expr
	Right Expr
}

func (Or) expr() {}

// Not rejects candidates accepted by its child.
type Not struct {
	Expr Expr
}

func (Not) expr() {}

// Parse parses terms combined with NOT, AND, OR, and parentheses. NOT binds
// more tightly than AND, which binds more tightly than OR. Operators are
// case-insensitive; quote a term to search for literal operator text. Inside
// quotes, \" and \\ are the only escapes. Parse rejects input longer than 4096
// bytes, with more than 32 terms, or nested more than 16 levels deep, where
// each NOT and each parenthesised group adds one level. Error positions are
// 0-based byte offsets into input. The returned AST owns its term strings and
// does not alias input.
func Parse(input string) (Expr, error) {
	if len(input) > maxExpressionBytes {
		return nil, fmt.Errorf("search expression: length %d exceeds maximum of %d bytes", len(input), maxExpressionBytes)
	}

	p := parser{input: input}
	if err := p.advance(); err != nil {
		return nil, err
	}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != tokenEOF {
		return nil, p.errorf("unexpected %s", p.cur.describe())
	}
	return expr, nil
}

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenTerm
	tokenNot
	tokenAnd
	tokenOr
	tokenLeftParen
	tokenRightParen
)

type token struct {
	kind  tokenKind
	value string
	pos   int
}

func (t token) describe() string {
	switch t.kind {
	case tokenEOF:
		return "end of expression"
	case tokenTerm:
		return fmt.Sprintf("term %q", t.value)
	case tokenNot:
		return "NOT"
	case tokenAnd:
		return "AND"
	case tokenOr:
		return "OR"
	case tokenLeftParen:
		return "("
	case tokenRightParen:
		return ")"
	default:
		return "token"
	}
}

// The parser lexes one token of lookahead on demand, so hostile input fails at
// the first bad token without the cost of lexing the rest. It consumes a token
// only after accepting it, so the first error in the input wins.
type parser struct {
	input     string
	pos       int
	cur       token
	termCount int
	depth     int
}

func (p *parser) advance() error {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
	start := p.pos
	if start == len(p.input) {
		p.cur = token{kind: tokenEOF, pos: start}
		return nil
	}

	switch p.input[start] {
	case '(':
		p.cur = token{kind: tokenLeftParen, pos: start}
		p.pos++
		return nil
	case ')':
		p.cur = token{kind: tokenRightParen, pos: start}
		p.pos++
		return nil
	case '"':
		value, next, err := quotedTerm(p.input, start)
		if err != nil {
			return err
		}
		if value == "" {
			return fmt.Errorf("search expression: empty quoted term at position %d", start)
		}
		p.cur = token{kind: tokenTerm, value: value, pos: start}
		p.pos = next
	default:
		next, end, err := bareToken(p.input, start)
		if err != nil {
			return err
		}
		p.cur = next
		p.pos = end
		if next.kind != tokenTerm {
			return nil
		}
	}

	p.termCount++
	if p.termCount > maxExpressionTerms {
		return fmt.Errorf("search expression: term count exceeds maximum of %d at position %d", maxExpressionTerms, start)
	}
	return nil
}

func bareToken(input string, start int) (token, int, error) {
	pos := start
	for pos < len(input) && input[pos] != ' ' && input[pos] != '\t' && input[pos] != '\n' && input[pos] != '\r' && input[pos] != '(' && input[pos] != ')' {
		if input[pos] == '"' {
			return token{}, 0, fmt.Errorf("search expression: unexpected quote at position %d", pos)
		}
		pos++
	}
	return keywordToken(input[start:pos], start), pos, nil
}

func quotedTerm(input string, start int) (string, int, error) {
	body := start + 1
	i := strings.IndexAny(input[body:], `"\`)
	if i < 0 {
		return "", 0, fmt.Errorf("search expression: unterminated quoted term at position %d", start)
	}
	if input[body+i] == '"' {
		return strings.Clone(input[body : body+i]), body + i + 1, nil
	}

	var value strings.Builder
	value.WriteString(input[body : body+i])
	for pos := body + i; pos < len(input); pos++ {
		switch input[pos] {
		case '"':
			return value.String(), pos + 1, nil
		case '\\':
			if pos+1 == len(input) {
				return "", 0, fmt.Errorf("search expression: unterminated quoted term at position %d", start)
			}
			pos++
			if input[pos] != '"' && input[pos] != '\\' {
				return "", 0, fmt.Errorf("search expression: invalid escape at position %d", pos-1)
			}
			value.WriteByte(input[pos])
		default:
			value.WriteByte(input[pos])
		}
	}
	return "", 0, fmt.Errorf("search expression: unterminated quoted term at position %d", start)
}

func keywordToken(value string, pos int) token {
	switch {
	case strings.EqualFold(value, "NOT"):
		return token{kind: tokenNot, pos: pos}
	case strings.EqualFold(value, "AND"):
		return token{kind: tokenAnd, pos: pos}
	case strings.EqualFold(value, "OR"):
		return token{kind: tokenOr, pos: pos}
	default:
		return token{kind: tokenTerm, value: strings.Clone(value), pos: pos}
	}
}

func (p *parser) parseOr() (Expr, error) {
	return p.parseBinary(tokenOr, p.parseAnd, func(left, right Expr) Expr {
		return Or{Left: left, Right: right}
	})
}

func (p *parser) parseAnd() (Expr, error) {
	return p.parseBinary(tokenAnd, p.parseUnary, func(left, right Expr) Expr {
		return And{Left: left, Right: right}
	})
}

func (p *parser) parseBinary(operator tokenKind, parseOperand func() (Expr, error), combine func(Expr, Expr) Expr) (Expr, error) {
	left, err := parseOperand()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == operator {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := parseOperand()
		if err != nil {
			return nil, err
		}
		left = combine(left, right)
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.cur.kind != tokenNot {
		return p.parsePrimary()
	}
	expr, err := p.parseNested(p.parseUnary)
	if err != nil {
		return nil, err
	}
	return Not{Expr: expr}, nil
}

func (p *parser) parsePrimary() (Expr, error) {
	switch p.cur.kind {
	case tokenTerm:
		term := Term{Value: p.cur.value}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return term, nil
	case tokenLeftParen:
		expr, err := p.parseNested(p.parseOr)
		if err != nil {
			return nil, err
		}
		if p.cur.kind != tokenRightParen {
			return nil, p.errorf("expected ), got %s", p.cur.describe())
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return expr, nil
	default:
		return nil, p.errorf("expected term or (, got %s", p.cur.describe())
	}
}

// parseNested consumes the current NOT or ( token and parses its operand one
// nesting level deeper.
func (p *parser) parseNested(parse func() (Expr, error)) (Expr, error) {
	if p.depth >= maxExpressionNestingDepth {
		return nil, p.errorf("nesting depth exceeds maximum of %d", maxExpressionNestingDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	if err := p.advance(); err != nil {
		return nil, err
	}
	return parse()
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("search expression: "+format+" at position %d", append(args, p.cur.pos)...)
}
