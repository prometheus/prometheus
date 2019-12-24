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
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/strutil"
)

type parser struct {
	lex     *Lexer
	token   Item
	peeking bool

	inject    Item
	injecting bool

	switchSymbols []ItemType

	generatedParserResult interface{}
}

// ParseErr wraps a parsing error with line and position context.
// If the parsing input was a single line, line will be 0 and omitted
// from the error string.
type ParseErr struct {
	Line, Pos int
	Err       error
}

func (e *ParseErr) Error() string {
	return fmt.Sprintf("%d:%d: parse error: %s", e.Line+1, e.Pos, e.Err)
}

// ParseExpr returns the expression parsed from the input.
func ParseExpr(input string) (Expr, error) {
	p := newParser(input)

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	err = p.typecheck(expr)
	return expr, err
}

// ParseMetric parses the input into a metric
func ParseMetric(input string) (m labels.Labels, err error) {
	p := newParser(input)
	defer p.recover(&err)

	return p.parseGenerated(START_METRIC, []ItemType{RIGHT_BRACE, EOF}).(labels.Labels), nil
}

// ParseMetricSelector parses the provided textual metric selector into a list of
// label matchers.
func ParseMetricSelector(input string) (m []*labels.Matcher, err error) {
	p := newParser(input)
	defer p.recover(&err)

	name := ""
	if t := p.peek().Typ; t == METRIC_IDENTIFIER || t == IDENTIFIER {
		name = p.next().Val
	}
	vs := p.VectorSelector(name)
	if p.peek().Typ != EOF {
		p.errorf("could not parse remaining input %.15q...", p.lex.input[p.lex.lastPos:])
	}
	return vs.LabelMatchers, nil
}

// newParser returns a new parser.
func newParser(input string) *parser {
	p := &parser{
		lex: Lex(input),
	}
	return p
}

// parseExpr parses a single expression from the input.
func (p *parser) parseExpr() (expr Expr, err error) {
	defer p.recover(&err)

	for p.peek().Typ != EOF {
		if expr != nil {
			p.errorf("could not parse remaining input %.15q...", p.lex.input[p.lex.lastPos:])
		}
		expr = p.expr()
	}

	if expr == nil {
		p.errorf("no expression found in input")
	}
	return
}

// sequenceValue is an omittable value in a sequence of time series values.
type sequenceValue struct {
	value   float64
	omitted bool
}

func (v sequenceValue) String() string {
	if v.omitted {
		return "_"
	}
	return fmt.Sprintf("%f", v.value)
}

type seriesDescription struct {
	labels labels.Labels
	values []sequenceValue
}

// parseSeriesDesc parses the description of a time series.
func parseSeriesDesc(input string) (labels labels.Labels, values []sequenceValue, err error) {

	p := newParser(input)
	p.lex.seriesDesc = true

	defer p.recover(&err)

	result := p.parseGenerated(START_SERIES_DESCRIPTION, []ItemType{EOF}).(*seriesDescription)

	labels = result.labels
	values = result.values

	return
}

// typecheck checks correct typing of the parsed statements or expression.
func (p *parser) typecheck(node Node) (err error) {
	defer p.recover(&err)

	p.checkType(node)
	return nil
}

// next returns the next token.
func (p *parser) next() Item {
	if !p.peeking {
		t := p.lex.NextItem()
		// Skip comments.
		for t.Typ == COMMENT {
			t = p.lex.NextItem()
		}
		p.token = t
	}

	p.peeking = false

	if p.token.Typ == ERROR {
		p.errorf("%s", p.token.Val)
	}
	return p.token
}

// peek returns but does not consume the next token.
func (p *parser) peek() Item {
	if p.peeking {
		return p.token
	}
	p.peeking = true

	t := p.lex.NextItem()
	// Skip comments.
	for t.Typ == COMMENT {
		t = p.lex.NextItem()
	}
	p.token = t
	return p.token
}

// backup backs the input stream up one token.
func (p *parser) backup() {
	p.peeking = true
}

// errorf formats the error and terminates processing.
func (p *parser) errorf(format string, args ...interface{}) {
	p.error(errors.Errorf(format, args...))
}

// error terminates processing.
func (p *parser) error(err error) {
	perr := &ParseErr{
		Line: p.lex.lineNumber(),
		Pos:  p.lex.linePosition(),
		Err:  err,
	}
	if strings.Count(strings.TrimSpace(p.lex.input), "\n") == 0 {
		perr.Line = 0
	}
	panic(perr)
}

// unexpected creates a parser error complaining about an unexpected lexer item.
// The item that is presented as unexpected is always the last item produced
// by the lexer.
func (p *parser) unexpected(context string, expected string) {
	var errMsg strings.Builder

	errMsg.WriteString("unexpected ")
	errMsg.WriteString(p.token.desc())

	if context != "" {
		errMsg.WriteString(" in ")
		errMsg.WriteString(context)
	}

	if expected != "" {
		errMsg.WriteString(", expected ")
		errMsg.WriteString(expected)
	}

	p.error(errors.New(errMsg.String()))
}

// expect consumes the next token and guarantees it has the required type.
func (p *parser) expect(exp ItemType, context string) Item {
	token := p.next()
	if token.Typ != exp {
		p.unexpected(context, exp.desc())
	}
	return token
}

// expectOneOf consumes the next token and guarantees it has one of the required types.
func (p *parser) expectOneOf(exp1, exp2 ItemType, context string) Item {
	token := p.next()
	if token.Typ != exp1 && token.Typ != exp2 {
		expected := exp1.desc() + " or " + exp2.desc()
		p.unexpected(context, expected)
	}
	return token
}

var errUnexpected = errors.New("unexpected error")

// recover is the handler that turns panics into returns from the top level of Parse.
func (p *parser) recover(errp *error) {
	e := recover()
	if _, ok := e.(runtime.Error); ok {
		// Print the stack trace but do not inhibit the running application.
		buf := make([]byte, 64<<10)
		buf = buf[:runtime.Stack(buf, false)]

		fmt.Fprintf(os.Stderr, "parser panic: %v\n%s", e, buf)
		*errp = errUnexpected
	} else if e != nil {
		*errp = e.(error)
	}
	p.lex.close()
}

// Lex is expected by the yyLexer interface of the yacc generated parser.
// It writes the next Item provided by the lexer to the provided pointer address.
// Comments are skipped.
//
// The yyLexer interface is currently implemented by the parser to allow
// the generated and non-generated parts to work together with regards to lookahead
// and error handling.
//
// For more information, see https://godoc.org/golang.org/x/tools/cmd/goyacc.
func (p *parser) Lex(lval *yySymType) int {
	if p.injecting {
		lval.item = p.inject
		p.injecting = false
	} else {
		lval.item = p.next()
	}

	typ := lval.item.Typ

	for _, t := range p.switchSymbols {
		if t == typ {
			p.InjectItem(0)
		}
	}

	return int(typ)
}

// Error is expected by the yyLexer interface of the yacc generated parser.
//
// It is a no-op since the parsers error routines are triggered
// by mechanisms that allow more fine-grained control
// For more information, see https://godoc.org/golang.org/x/tools/cmd/goyacc.
func (p *parser) Error(e string) {
}

// InjectItem allows injecting a single Item at the beginning of the token stream
// consumed by the generated parser.
// This allows having multiple start symbols as described in
// https://www.gnu.org/software/bison/manual/html_node/Multiple-start_002dsymbols.html .
// Only the Lex function used by the generated parser is affected by this injected Item.
// Trying to inject when a previously injected Item has not yet been consumed will panic.
// Only Item types that are supposed to be used as start symbols are allowed as an argument.
func (p *parser) InjectItem(typ ItemType) {
	if p.injecting {
		panic("cannot inject multiple Items into the token stream")
	}

	if typ != 0 && (typ <= startSymbolsStart || typ >= startSymbolsEnd) {
		panic("cannot inject symbol that isn't start symbol")
	}

	p.inject = Item{Typ: typ}
	p.injecting = true
}

// expr parses any expression.
func (p *parser) expr() Expr {
	// Parse the starting expression.
	expr := p.unaryExpr()

	// Loop through the operations and construct a binary operation tree based
	// on the operators' precedence.
	for {
		// If the next token is not an operator the expression is done.
		op := p.peek().Typ
		if !op.isOperator() {
			// Check for subquery.
			if op == LEFT_BRACKET {
				expr = p.subqueryOrRangeSelector(expr, false)
				if s, ok := expr.(*SubqueryExpr); ok {
					// Parse optional offset.
					if p.peek().Typ == OFFSET {
						offset := p.offset()
						s.Offset = offset
					}
				}
			}
			return expr
		}
		p.next() // Consume operator.

		// Parse optional operator matching options. Its validity
		// is checked in the type-checking stage.
		vecMatching := &VectorMatching{
			Card: CardOneToOne,
		}
		if op.isSetOperator() {
			vecMatching.Card = CardManyToMany
		}

		returnBool := false
		// Parse bool modifier.
		if p.peek().Typ == BOOL {
			if !op.isComparisonOperator() {
				p.errorf("bool modifier can only be used on comparison operators")
			}
			p.next()
			returnBool = true
		}

		// Parse ON/IGNORING clause.
		if p.peek().Typ == ON || p.peek().Typ == IGNORING {
			if p.peek().Typ == ON {
				vecMatching.On = true
			}
			p.next()
			vecMatching.MatchingLabels = p.labels()

			// Parse grouping.
			if t := p.peek().Typ; t == GROUP_LEFT || t == GROUP_RIGHT {
				p.next()
				if t == GROUP_LEFT {
					vecMatching.Card = CardManyToOne
				} else {
					vecMatching.Card = CardOneToMany
				}
				if p.peek().Typ == LEFT_PAREN {
					vecMatching.Include = p.labels()
				}
			}
		}

		for _, ln := range vecMatching.MatchingLabels {
			for _, ln2 := range vecMatching.Include {
				if ln == ln2 && vecMatching.On {
					p.errorf("label %q must not occur in ON and GROUP clause at once", ln)
				}
			}
		}

		// Parse the next operand.
		rhs := p.unaryExpr()

		// Assign the new root based on the precedence of the LHS and RHS operators.
		expr = p.balance(expr, op, rhs, vecMatching, returnBool)
	}
}

func (p *parser) balance(lhs Expr, op ItemType, rhs Expr, vecMatching *VectorMatching, returnBool bool) *BinaryExpr {
	if lhsBE, ok := lhs.(*BinaryExpr); ok {
		precd := lhsBE.Op.precedence() - op.precedence()
		if (precd < 0) || (precd == 0 && op.isRightAssociative()) {
			balanced := p.balance(lhsBE.RHS, op, rhs, vecMatching, returnBool)
			if lhsBE.Op.isComparisonOperator() && !lhsBE.ReturnBool && balanced.Type() == ValueTypeScalar && lhsBE.LHS.Type() == ValueTypeScalar {
				p.errorf("comparisons between scalars must use BOOL modifier")
			}
			return &BinaryExpr{
				Op:             lhsBE.Op,
				LHS:            lhsBE.LHS,
				RHS:            balanced,
				VectorMatching: lhsBE.VectorMatching,
				ReturnBool:     lhsBE.ReturnBool,
			}
		}
	}
	if op.isComparisonOperator() && !returnBool && rhs.Type() == ValueTypeScalar && lhs.Type() == ValueTypeScalar {
		p.errorf("comparisons between scalars must use BOOL modifier")
	}
	return &BinaryExpr{
		Op:             op,
		LHS:            lhs,
		RHS:            rhs,
		VectorMatching: vecMatching,
		ReturnBool:     returnBool,
	}
}

// unaryExpr parses a unary expression.
//
//		<Vector_selector> | <Matrix_selector> | (+|-) <number_literal> | '(' <expr> ')'
//
func (p *parser) unaryExpr() Expr {
	switch t := p.peek(); t.Typ {
	case ADD, SUB:
		p.next()
		e := p.unaryExpr()

		// Simplify unary expressions for number literals.
		if nl, ok := e.(*NumberLiteral); ok {
			if t.Typ == SUB {
				nl.Val *= -1
			}
			return nl
		}
		return &UnaryExpr{Op: t.Typ, Expr: e}

	case LEFT_PAREN:
		p.next()
		e := p.expr()
		p.expect(RIGHT_PAREN, "paren expression")

		return &ParenExpr{Expr: e}
	}
	e := p.primaryExpr()

	// Expression might be followed by a range selector.
	if p.peek().Typ == LEFT_BRACKET {
		e = p.subqueryOrRangeSelector(e, true)
	}

	// Parse optional offset.
	if p.peek().Typ == OFFSET {
		offset := p.offset()

		switch s := e.(type) {
		case *VectorSelector:
			s.Offset = offset
		case *MatrixSelector:
			s.Offset = offset
		case *SubqueryExpr:
			s.Offset = offset
		default:
			p.errorf("offset modifier must be preceded by an instant or range selector, but follows a %T instead", e)
		}
	}

	return e
}

// subqueryOrRangeSelector parses a Subquery based on given Expr (or)
// a Matrix (a.k.a. range) selector based on a given Vector selector.
//
//		<Vector_selector> '[' <duration> ']' | <Vector_selector> '[' <duration> ':' [<duration>] ']'
//
func (p *parser) subqueryOrRangeSelector(expr Expr, checkRange bool) Expr {
	ctx := "subquery selector"
	if checkRange {
		ctx = "range/subquery selector"
	}

	p.next()

	var erange time.Duration
	var err error

	erangeStr := p.expect(DURATION, ctx).Val
	erange, err = parseDuration(erangeStr)
	if err != nil {
		p.error(err)
	}

	var itm Item
	if checkRange {
		itm = p.expectOneOf(RIGHT_BRACKET, COLON, ctx)
		if itm.Typ == RIGHT_BRACKET {
			// Range selector.
			vs, ok := expr.(*VectorSelector)
			if !ok {
				p.errorf("range specification must be preceded by a metric selector, but follows a %T instead", expr)
			}
			return &MatrixSelector{
				Name:          vs.Name,
				LabelMatchers: vs.LabelMatchers,
				Range:         erange,
			}
		}
	} else {
		itm = p.expect(COLON, ctx)
	}

	// Subquery.
	var estep time.Duration

	itm = p.expectOneOf(RIGHT_BRACKET, DURATION, ctx)
	if itm.Typ == DURATION {
		estepStr := itm.Val
		estep, err = parseDuration(estepStr)
		if err != nil {
			p.error(err)
		}
		p.expect(RIGHT_BRACKET, ctx)
	}

	return &SubqueryExpr{
		Expr:  expr,
		Range: erange,
		Step:  estep,
	}
}

// number parses a number.
func (p *parser) number(val string) float64 {
	n, err := strconv.ParseInt(val, 0, 64)
	f := float64(n)
	if err != nil {
		f, err = strconv.ParseFloat(val, 64)
	}
	if err != nil {
		p.errorf("error parsing number: %s", err)
	}
	return f
}

// primaryExpr parses a primary expression.
//
//		<metric_name> | <function_call> | <Vector_aggregation> | <literal>
//
func (p *parser) primaryExpr() Expr {
	switch t := p.next(); {
	case t.Typ == NUMBER:
		f := p.number(t.Val)
		return &NumberLiteral{f}

	case t.Typ == STRING:
		return &StringLiteral{p.unquoteString(t.Val)}

	case t.Typ == LEFT_BRACE:
		// Metric selector without metric name.
		p.backup()
		return p.VectorSelector("")

	case t.Typ == IDENTIFIER:
		// Check for function call.
		if p.peek().Typ == LEFT_PAREN {
			return p.call(t.Val)
		}
		fallthrough // Else metric selector.

	case t.Typ == METRIC_IDENTIFIER:
		return p.VectorSelector(t.Val)

	case t.Typ.isAggregator():
		p.backup()
		return p.aggrExpr()

	default:
		p.errorf("no valid expression found")
	}
	return nil
}

// labels parses a list of labelnames.
//
//		'(' <label_name>, ... ')'
//
func (p *parser) labels() []string {
	return p.parseGenerated(START_GROUPING_LABELS, []ItemType{RIGHT_PAREN, EOF}).([]string)
}

// aggrExpr parses an aggregation expression.
//
//		<aggr_op> (<Vector_expr>) [by|without <labels>]
//		<aggr_op> [by|without <labels>] (<Vector_expr>)
//
func (p *parser) aggrExpr() *AggregateExpr {
	const ctx = "aggregation"

	agop := p.next()
	if !agop.Typ.isAggregator() {
		p.errorf("expected aggregation operator but got %s", agop)
	}
	var grouping []string
	var without bool

	modifiersFirst := false

	if t := p.peek().Typ; t == BY || t == WITHOUT {
		if t == WITHOUT {
			without = true
		}
		p.next()
		grouping = p.labels()
		modifiersFirst = true
	}

	p.expect(LEFT_PAREN, ctx)
	var param Expr
	if agop.Typ.isAggregatorWithParam() {
		param = p.expr()
		p.expect(COMMA, ctx)
	}
	e := p.expr()
	p.expect(RIGHT_PAREN, ctx)

	if !modifiersFirst {
		if t := p.peek().Typ; t == BY || t == WITHOUT {
			if len(grouping) > 0 {
				p.errorf("aggregation must only contain one grouping clause")
			}
			if t == WITHOUT {
				without = true
			}
			p.next()
			grouping = p.labels()
		}
	}

	return &AggregateExpr{
		Op:       agop.Typ,
		Expr:     e,
		Param:    param,
		Grouping: grouping,
		Without:  without,
	}
}

// call parses a function call.
//
//		<func_name> '(' [ <arg_expr>, ...] ')'
//
func (p *parser) call(name string) *Call {
	const ctx = "function call"

	fn, exist := getFunction(name)
	if !exist {
		p.errorf("unknown function with name %q", name)
	}

	p.expect(LEFT_PAREN, ctx)
	// Might be call without args.
	if p.peek().Typ == RIGHT_PAREN {
		p.next() // Consume.
		return &Call{fn, nil}
	}

	var args []Expr
	for {
		e := p.expr()
		args = append(args, e)

		// Terminate if no more arguments.
		if p.peek().Typ != COMMA {
			break
		}
		p.next()
	}

	// Call must be closed.
	p.expect(RIGHT_PAREN, ctx)

	return &Call{Func: fn, Args: args}
}

// offset parses an offset modifier.
//
//		offset <duration>
//
func (p *parser) offset() time.Duration {
	const ctx = "offset"

	p.next()
	offi := p.expect(DURATION, ctx)

	offset, err := parseDuration(offi.Val)
	if err != nil {
		p.error(err)
	}

	return offset
}

// VectorSelector parses a new (instant) vector selector.
//
//		<metric_identifier> [<label_matchers>]
//		[<metric_identifier>] <label_matchers>
//
func (p *parser) VectorSelector(name string) *VectorSelector {
	ret := &VectorSelector{
		Name: name,
	}
	// Parse label matching if any.
	if t := p.peek(); t.Typ == LEFT_BRACE {
		p.generatedParserResult = ret

		p.parseGenerated(START_LABELS, []ItemType{RIGHT_BRACE, EOF})
	}
	// Metric name must not be set in the label matchers and before at the same time.
	if name != "" {
		for _, m := range ret.LabelMatchers {
			if m.Name == labels.MetricName {
				p.errorf("metric name must not be set twice: %q or %q", name, m.Value)
			}
		}
		// Set name label matching.
		m, err := labels.NewMatcher(labels.MatchEqual, labels.MetricName, name)
		if err != nil {
			panic(err) // Must not happen with metric.Equal.
		}
		ret.LabelMatchers = append(ret.LabelMatchers, m)
	}

	if len(ret.LabelMatchers) == 0 {
		p.errorf("vector selector must contain label matchers or metric name")
	}
	// A Vector selector must contain at least one non-empty matcher to prevent
	// implicit selection of all metrics (e.g. by a typo).
	notEmpty := false
	for _, lm := range ret.LabelMatchers {
		if !lm.Matches("") {
			notEmpty = true
			break
		}
	}
	if !notEmpty {
		p.errorf("vector selector must contain at least one non-empty matcher")
	}

	return ret
}

// expectType checks the type of the node and raises an error if it
// is not of the expected type.
func (p *parser) expectType(node Node, want ValueType, context string) {
	t := p.checkType(node)
	if t != want {
		p.errorf("expected type %s in %s, got %s", documentedType(want), context, documentedType(t))
	}
}

// check the types of the children of each node and raise an error
// if they do not form a valid node.
//
// Some of these checks are redundant as the parsing stage does not allow
// them, but the costs are small and might reveal errors when making changes.
func (p *parser) checkType(node Node) (typ ValueType) {
	// For expressions the type is determined by their Type function.
	// Lists do not have a type but are not invalid either.
	switch n := node.(type) {
	case Expressions:
		typ = ValueTypeNone
	case Expr:
		typ = n.Type()
	default:
		p.errorf("unknown node type: %T", node)
	}

	// Recursively check correct typing for child nodes and raise
	// errors in case of bad typing.
	switch n := node.(type) {
	case *EvalStmt:
		ty := p.checkType(n.Expr)
		if ty == ValueTypeNone {
			p.errorf("evaluation statement must have a valid expression type but got %s", documentedType(ty))
		}

	case Expressions:
		for _, e := range n {
			ty := p.checkType(e)
			if ty == ValueTypeNone {
				p.errorf("expression must have a valid expression type but got %s", documentedType(ty))
			}
		}
	case *AggregateExpr:
		if !n.Op.isAggregator() {
			p.errorf("aggregation operator expected in aggregation expression but got %q", n.Op)
		}
		p.expectType(n.Expr, ValueTypeVector, "aggregation expression")
		if n.Op == TOPK || n.Op == BOTTOMK || n.Op == QUANTILE {
			p.expectType(n.Param, ValueTypeScalar, "aggregation parameter")
		}
		if n.Op == COUNT_VALUES {
			p.expectType(n.Param, ValueTypeString, "aggregation parameter")
		}

	case *BinaryExpr:
		lt := p.checkType(n.LHS)
		rt := p.checkType(n.RHS)

		if !n.Op.isOperator() {
			p.errorf("binary expression does not support operator %q", n.Op)
		}
		if (lt != ValueTypeScalar && lt != ValueTypeVector) || (rt != ValueTypeScalar && rt != ValueTypeVector) {
			p.errorf("binary expression must contain only scalar and instant vector types")
		}

		if (lt != ValueTypeVector || rt != ValueTypeVector) && n.VectorMatching != nil {
			if len(n.VectorMatching.MatchingLabels) > 0 {
				p.errorf("vector matching only allowed between instant vectors")
			}
			n.VectorMatching = nil
		} else {
			// Both operands are Vectors.
			if n.Op.isSetOperator() {
				if n.VectorMatching.Card == CardOneToMany || n.VectorMatching.Card == CardManyToOne {
					p.errorf("no grouping allowed for %q operation", n.Op)
				}
				if n.VectorMatching.Card != CardManyToMany {
					p.errorf("set operations must always be many-to-many")
				}
			}
		}

		if (lt == ValueTypeScalar || rt == ValueTypeScalar) && n.Op.isSetOperator() {
			p.errorf("set operator %q not allowed in binary scalar expression", n.Op)
		}

	case *Call:
		nargs := len(n.Func.ArgTypes)
		if n.Func.Variadic == 0 {
			if nargs != len(n.Args) {
				p.errorf("expected %d argument(s) in call to %q, got %d", nargs, n.Func.Name, len(n.Args))
			}
		} else {
			na := nargs - 1
			if na > len(n.Args) {
				p.errorf("expected at least %d argument(s) in call to %q, got %d", na, n.Func.Name, len(n.Args))
			} else if nargsmax := na + n.Func.Variadic; n.Func.Variadic > 0 && nargsmax < len(n.Args) {
				p.errorf("expected at most %d argument(s) in call to %q, got %d", nargsmax, n.Func.Name, len(n.Args))
			}
		}

		for i, arg := range n.Args {
			if i >= len(n.Func.ArgTypes) {
				i = len(n.Func.ArgTypes) - 1
			}
			p.expectType(arg, n.Func.ArgTypes[i], fmt.Sprintf("call to function %q", n.Func.Name))
		}

	case *ParenExpr:
		p.checkType(n.Expr)

	case *UnaryExpr:
		if n.Op != ADD && n.Op != SUB {
			p.errorf("only + and - operators allowed for unary expressions")
		}
		if t := p.checkType(n.Expr); t != ValueTypeScalar && t != ValueTypeVector {
			p.errorf("unary expression only allowed on expressions of type scalar or instant vector, got %q", documentedType(t))
		}

	case *SubqueryExpr:
		ty := p.checkType(n.Expr)
		if ty != ValueTypeVector {
			p.errorf("subquery is only allowed on instant vector, got %s in %q instead", ty, n.String())
		}

	case *NumberLiteral, *MatrixSelector, *StringLiteral, *VectorSelector:
		// Nothing to do for terminals.

	default:
		p.errorf("unknown node type: %T", node)
	}
	return
}

func (p *parser) unquoteString(s string) string {
	unquoted, err := strutil.Unquote(s)
	if err != nil {
		p.errorf("error unquoting string %q: %s", s, err)
	}
	return unquoted
}

func parseDuration(ds string) (time.Duration, error) {
	dur, err := model.ParseDuration(ds)
	if err != nil {
		return 0, err
	}
	if dur == 0 {
		return 0, errors.New("duration must be greater than 0")
	}
	return time.Duration(dur), nil
}

// parseGenerated invokes the yacc generated parser.
// The generated parser gets the provided startSymbol injected into
// the lexer stream, based on which grammar will be used.
//
// The generated parser will consume the lexer Stream until one of the
// tokens listed in switchSymbols is encountered. switchSymbols
// should at least contain EOF
func (p *parser) parseGenerated(startSymbol ItemType, switchSymbols []ItemType) interface{} {
	p.InjectItem(startSymbol)

	p.switchSymbols = switchSymbols

	yyParse(p)

	return p.generatedParserResult

}

func (p *parser) newLabelMatcher(label Item, operator Item, value Item) *labels.Matcher {
	op := operator.Typ
	val := p.unquoteString(value.Val)

	// Map the Item to the respective match type.
	var matchType labels.MatchType
	switch op {
	case EQL:
		matchType = labels.MatchEqual
	case NEQ:
		matchType = labels.MatchNotEqual
	case EQL_REGEX:
		matchType = labels.MatchRegexp
	case NEQ_REGEX:
		matchType = labels.MatchNotRegexp
	default:
		// This should never happen, since the error should have been caught
		// by the generated parser.
		panic("invalid operator")
	}

	m, err := labels.NewMatcher(matchType, label.Val, val)
	if err != nil {
		p.error(err)
	}

	return m
}
