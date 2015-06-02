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
	"runtime"
	"strconv"
	"strings"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/strutil"
)

type parser struct {
	lex       *lexer
	token     [3]item
	peekCount int
}

// ParseErr wraps a parsing error with line and position context.
// If the parsing input was a single line, line will be 0 and omitted
// from the error string.
type ParseErr struct {
	Line, Pos int
	Err       error
}

func (e *ParseErr) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("Parse error at char %d: %s", e.Pos, e.Err)
	}
	return fmt.Sprintf("Parse error at line %d, char %d: %s", e.Line, e.Pos, e.Err)
}

// ParseStmts parses the input and returns the resulting statements or any ocurring error.
func ParseStmts(input string) (Statements, error) {
	p := newParser(input)

	stmts, err := p.parseStmts()
	if err != nil {
		return nil, err
	}
	err = p.typecheck(stmts)
	return stmts, err
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

// parseSeriesDesc parses the description of a time series.
func parseSeriesDesc(input string) (clientmodel.Metric, []sequenceValue, error) {
	p := newParser(input)
	p.lex.seriesDesc = true

	return p.parseSeriesDesc()
}

// newParser returns a new parser.
func newParser(input string) *parser {
	p := &parser{
		lex: lex(input),
	}
	return p
}

// parseStmts parses a sequence of statements from the input.
func (p *parser) parseStmts() (stmts Statements, err error) {
	defer p.recover(&err)
	stmts = Statements{}

	for p.peek().typ != itemEOF {
		if p.peek().typ == itemComment {
			continue
		}
		stmts = append(stmts, p.stmt())
	}
	return
}

// parseExpr parses a single expression from the input.
func (p *parser) parseExpr() (expr Expr, err error) {
	defer p.recover(&err)

	for p.peek().typ != itemEOF {
		if p.peek().typ == itemComment {
			continue
		}
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
	value   clientmodel.SampleValue
	omitted bool
}

func (v sequenceValue) String() string {
	if v.omitted {
		return "_"
	}
	return v.value.String()
}

// parseSeriesDesc parses a description of a time series into its metric and value sequence.
func (p *parser) parseSeriesDesc() (m clientmodel.Metric, vals []sequenceValue, err error) {
	defer p.recover(&err)

	name := ""
	m = clientmodel.Metric{}

	t := p.peek().typ
	if t == itemIdentifier || t == itemMetricIdentifier {
		name = p.next().val
		t = p.peek().typ
	}
	if t == itemLeftBrace {
		m = clientmodel.Metric(p.labelSet())
	}
	if name != "" {
		m[clientmodel.MetricNameLabel] = clientmodel.LabelValue(name)
	}

	const ctx = "series values"
	for {
		if p.peek().typ == itemEOF {
			break
		}

		// Extract blanks.
		if p.peek().typ == itemBlank {
			p.next()
			times := uint64(1)
			if p.peek().typ == itemTimes {
				p.next()
				times, err = strconv.ParseUint(p.expect(itemNumber, ctx).val, 10, 64)
				if err != nil {
					p.errorf("invalid repetition in %s: %s", ctx, err)
				}
			}
			for i := uint64(0); i < times; i++ {
				vals = append(vals, sequenceValue{omitted: true})
			}
			continue
		}

		// Extract values.
		sign := 1.0
		if t := p.peek().typ; t == itemSUB || t == itemADD {
			if p.next().typ == itemSUB {
				sign = -1
			}
		}
		k := sign * p.number(p.expect(itemNumber, ctx).val)
		vals = append(vals, sequenceValue{
			value: clientmodel.SampleValue(k),
		})

		// If there are no offset repetitions specified, proceed with the next value.
		if t := p.peek().typ; t == itemNumber || t == itemBlank {
			continue
		} else if t == itemEOF {
			break
		} else if t != itemADD && t != itemSUB {
			p.errorf("expected next value or relative expansion in %s but got %s", ctx, t.desc())
		}

		// Expand the repeated offsets into values.
		sign = 1.0
		if p.next().typ == itemSUB {
			sign = -1.0
		}
		offset := sign * p.number(p.expect(itemNumber, ctx).val)
		p.expect(itemTimes, ctx)

		times, err := strconv.ParseUint(p.expect(itemNumber, ctx).val, 10, 64)
		if err != nil {
			p.errorf("invalid repetition in %s: %s", ctx, err)
		}

		for i := uint64(0); i < times; i++ {
			k += offset
			vals = append(vals, sequenceValue{
				value: clientmodel.SampleValue(k),
			})
		}
	}
	return m, vals, nil
}

// typecheck checks correct typing of the parsed statements or expression.
func (p *parser) typecheck(node Node) (err error) {
	defer p.recover(&err)

	p.checkType(node)
	return nil
}

// next returns the next token.
func (p *parser) next() item {
	if p.peekCount > 0 {
		p.peekCount--
	} else {
		t := p.lex.nextItem()
		// Skip comments.
		for t.typ == itemComment {
			t = p.lex.nextItem()
		}
		p.token[0] = t
	}
	if p.token[p.peekCount].typ == itemError {
		p.errorf("%s", p.token[p.peekCount].val)
	}
	return p.token[p.peekCount]
}

// peek returns but does not consume the next token.
func (p *parser) peek() item {
	if p.peekCount > 0 {
		return p.token[p.peekCount-1]
	}
	p.peekCount = 1

	t := p.lex.nextItem()
	// Skip comments.
	for t.typ == itemComment {
		t = p.lex.nextItem()
	}
	p.token[0] = t
	return p.token[0]
}

// backup backs the input stream up one token.
func (p *parser) backup() {
	p.peekCount++
}

// errorf formats the error and terminates processing.
func (p *parser) errorf(format string, args ...interface{}) {
	p.error(fmt.Errorf(format, args...))
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

// expect consumes the next token and guarantees it has the required type.
func (p *parser) expect(exp itemType, context string) item {
	token := p.next()
	if token.typ != exp {
		p.errorf("unexpected %s in %s, expected %s", token.desc(), context, exp.desc())
	}
	return token
}

// expectOneOf consumes the next token and guarantees it has one of the required types.
func (p *parser) expectOneOf(exp1, exp2 itemType, context string) item {
	token := p.next()
	if token.typ != exp1 && token.typ != exp2 {
		p.errorf("unexpected %s in %s, expected %s or %s", token.desc(), context, exp1.desc(), exp2.desc())
	}
	return token
}

// recover is the handler that turns panics into returns from the top level of Parse.
func (p *parser) recover(errp *error) {
	e := recover()
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			panic(e)
		}
		*errp = e.(error)
	}
	return
}

// stmt parses any statement.
//
// 		alertStatement | recordStatement
//
func (p *parser) stmt() Statement {
	switch tok := p.peek(); tok.typ {
	case itemAlert:
		return p.alertStmt()
	case itemIdentifier, itemMetricIdentifier:
		return p.recordStmt()
	}
	p.errorf("no valid statement detected")
	return nil
}

// alertStmt parses an alert rule.
//
//		ALERT name IF expr [FOR duration] [WITH label_set]
//			SUMMARY "summary"
//			DESCRIPTION "description"
//
func (p *parser) alertStmt() *AlertStmt {
	const ctx = "alert statement"

	p.expect(itemAlert, ctx)
	name := p.expect(itemIdentifier, ctx)
	// Alerts require a vector typed expression.
	p.expect(itemIf, ctx)
	expr := p.expr()

	// Optional for clause.
	var duration time.Duration
	var err error

	if p.peek().typ == itemFor {
		p.next()
		dur := p.expect(itemDuration, ctx)
		duration, err = parseDuration(dur.val)
		if err != nil {
			p.error(err)
		}
	}

	lset := clientmodel.LabelSet{}
	if p.peek().typ == itemWith {
		p.expect(itemWith, ctx)
		lset = p.labelSet()
	}

	p.expect(itemSummary, ctx)
	sum := trimOne(p.expect(itemString, ctx).val)

	p.expect(itemDescription, ctx)
	desc := trimOne(p.expect(itemString, ctx).val)

	return &AlertStmt{
		Name:        name.val,
		Expr:        expr,
		Duration:    duration,
		Labels:      lset,
		Summary:     sum,
		Description: desc,
	}
}

// recordStmt parses a recording rule.
func (p *parser) recordStmt() *RecordStmt {
	const ctx = "record statement"

	name := p.expectOneOf(itemIdentifier, itemMetricIdentifier, ctx).val

	var lset clientmodel.LabelSet
	if p.peek().typ == itemLeftBrace {
		lset = p.labelSet()
	}

	p.expect(itemAssign, ctx)
	expr := p.expr()

	return &RecordStmt{
		Name:   name,
		Labels: lset,
		Expr:   expr,
	}
}

// expr parses any expression.
func (p *parser) expr() Expr {
	// Parse the starting expression.
	expr := p.unaryExpr()
	if expr == nil {
		p.errorf("no valid expression found")
	}

	// Loop through the operations and construct a binary operation tree based
	// on the operators' precedence.
	for {
		// If the next token is not an operator the expression is done.
		op := p.peek().typ
		if !op.isOperator() {
			return expr
		}
		p.next() // Consume operator.

		// Parse optional operator matching options. Its validity
		// is checked in the type-checking stage.
		vecMatching := &VectorMatching{
			Card: CardOneToOne,
		}
		if op == itemLAND || op == itemLOR {
			vecMatching.Card = CardManyToMany
		}

		// Parse ON clause.
		if p.peek().typ == itemOn {
			p.next()
			vecMatching.On = p.labels()

			// Parse grouping.
			if t := p.peek().typ; t == itemGroupLeft {
				p.next()
				vecMatching.Card = CardManyToOne
				vecMatching.Include = p.labels()
			} else if t == itemGroupRight {
				p.next()
				vecMatching.Card = CardOneToMany
				vecMatching.Include = p.labels()
			}
		}

		for _, ln := range vecMatching.On {
			for _, ln2 := range vecMatching.Include {
				if ln == ln2 {
					p.errorf("label %q must not occur in ON and INCLUDE clause at once", ln)
				}
			}
		}

		// Parse the next operand.
		rhs := p.unaryExpr()
		if rhs == nil {
			p.errorf("missing right-hand side in binary expression")
		}

		// Assign the new root based on the precendence of the LHS and RHS operators.
		if lhs, ok := expr.(*BinaryExpr); ok && lhs.Op.precedence() < op.precedence() {
			expr = &BinaryExpr{
				Op:  lhs.Op,
				LHS: lhs.LHS,
				RHS: &BinaryExpr{
					Op:             op,
					LHS:            lhs.RHS,
					RHS:            rhs,
					VectorMatching: vecMatching,
				},
				VectorMatching: lhs.VectorMatching,
			}
		} else {
			expr = &BinaryExpr{
				Op:             op,
				LHS:            expr,
				RHS:            rhs,
				VectorMatching: vecMatching,
			}
		}
	}
	return nil
}

// unaryExpr parses a unary expression.
//
//		<vector_selector> | <matrix_selector> | (+|-) <number_literal> | '(' <expr> ')'
//
func (p *parser) unaryExpr() Expr {
	switch t := p.peek(); t.typ {
	case itemADD, itemSUB:
		p.next()
		e := p.unaryExpr()
		// Simplify unary expressions for number literals.
		if nl, ok := e.(*NumberLiteral); ok {
			if t.typ == itemSUB {
				nl.Val *= -1
			}
			return nl
		}
		return &UnaryExpr{Op: t.typ, Expr: e}

	case itemLeftParen:
		p.next()
		e := p.expr()
		p.expect(itemRightParen, "paren expression")

		return &ParenExpr{Expr: e}
	}
	e := p.primaryExpr()

	// Expression might be followed by a range selector.
	if p.peek().typ == itemLeftBracket {
		vs, ok := e.(*VectorSelector)
		if !ok {
			p.errorf("range specification must be preceded by a metric selector, but follows a %T instead", e)
		}
		e = p.rangeSelector(vs)
	}
	return e
}

// rangeSelector parses a matrix selector based on a given vector selector.
//
//		<vector_selector> '[' <duration> ']'
//
func (p *parser) rangeSelector(vs *VectorSelector) *MatrixSelector {
	const ctx = "matrix selector"
	p.next()

	var erange, offset time.Duration
	var err error

	erangeStr := p.expect(itemDuration, ctx).val
	erange, err = parseDuration(erangeStr)
	if err != nil {
		p.error(err)
	}

	p.expect(itemRightBracket, ctx)

	// Parse optional offset.
	if p.peek().typ == itemOffset {
		p.next()
		offi := p.expect(itemDuration, ctx)

		offset, err = parseDuration(offi.val)
		if err != nil {
			p.error(err)
		}
	}

	e := &MatrixSelector{
		Name:          vs.Name,
		LabelMatchers: vs.LabelMatchers,
		Range:         erange,
		Offset:        offset,
	}
	return e
}

// parseNumber parses a number.
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
//		<metric_name> | <function_call> | <vector_aggregation> | <literal>
//
func (p *parser) primaryExpr() Expr {
	switch t := p.next(); {
	case t.typ == itemNumber:
		f := p.number(t.val)
		return &NumberLiteral{clientmodel.SampleValue(f)}

	case t.typ == itemString:
		s := t.val[1 : len(t.val)-1]
		return &StringLiteral{s}

	case t.typ == itemLeftBrace:
		// Metric selector without metric name.
		p.backup()
		return p.vectorSelector("")

	case t.typ == itemIdentifier:
		// Check for function call.
		if p.peek().typ == itemLeftParen {
			return p.call(t.val)
		}
		fallthrough // Else metric selector.

	case t.typ == itemMetricIdentifier:
		return p.vectorSelector(t.val)

	case t.typ.isAggregator():
		p.backup()
		return p.aggrExpr()
	}
	return nil
}

// labels parses a list of labelnames.
//
//		'(' <label_name>, ... ')'
//
func (p *parser) labels() clientmodel.LabelNames {
	const ctx = "grouping opts"

	p.expect(itemLeftParen, ctx)

	labels := clientmodel.LabelNames{}
	for {
		id := p.expect(itemIdentifier, ctx)
		labels = append(labels, clientmodel.LabelName(id.val))

		if p.peek().typ != itemComma {
			break
		}
		p.next()
	}
	p.expect(itemRightParen, ctx)

	return labels
}

// aggrExpr parses an aggregation expression.
//
//		<aggr_op> (<vector_expr>) [by <labels>] [keeping_extra]
//		<aggr_op> [by <labels>] [keeping_extra] (<vector_expr>)
//
func (p *parser) aggrExpr() *AggregateExpr {
	const ctx = "aggregation"

	agop := p.next()
	if !agop.typ.isAggregator() {
		p.errorf("expected aggregation operator but got %s", agop)
	}
	var grouping clientmodel.LabelNames
	var keepExtra bool

	modifiersFirst := false

	if p.peek().typ == itemBy {
		p.next()
		grouping = p.labels()
		modifiersFirst = true
	}
	if p.peek().typ == itemKeepingExtra {
		p.next()
		keepExtra = true
		modifiersFirst = true
	}

	p.expect(itemLeftParen, ctx)
	e := p.expr()
	p.expect(itemRightParen, ctx)

	if !modifiersFirst {
		if p.peek().typ == itemBy {
			if len(grouping) > 0 {
				p.errorf("aggregation must only contain one grouping clause")
			}
			p.next()
			grouping = p.labels()
		}
		if p.peek().typ == itemKeepingExtra {
			p.next()
			keepExtra = true
		}
	}

	return &AggregateExpr{
		Op:              agop.typ,
		Expr:            e,
		Grouping:        grouping,
		KeepExtraLabels: keepExtra,
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

	p.expect(itemLeftParen, ctx)
	// Might be call without args.
	if p.peek().typ == itemRightParen {
		p.next() // Consume.
		return &Call{fn, nil}
	}

	var args []Expr
	for {
		e := p.expr()
		args = append(args, e)

		// Terminate if no more arguments.
		if p.peek().typ != itemComma {
			break
		}
		p.next()
	}

	// Call must be closed.
	p.expect(itemRightParen, ctx)

	return &Call{Func: fn, Args: args}
}

// labelSet parses a set of label matchers
//
//		'{' [ <labelname> '=' <match_string>, ... ] '}'
//
func (p *parser) labelSet() clientmodel.LabelSet {
	set := clientmodel.LabelSet{}
	for _, lm := range p.labelMatchers(itemEQL) {
		set[lm.Name] = lm.Value
	}
	return set
}

// labelMatchers parses a set of label matchers.
//
//		'{' [ <labelname> <match_op> <match_string>, ... ] '}'
//
func (p *parser) labelMatchers(operators ...itemType) metric.LabelMatchers {
	const ctx = "label matching"

	matchers := metric.LabelMatchers{}

	p.expect(itemLeftBrace, ctx)

	// Check if no matchers are provided.
	if p.peek().typ == itemRightBrace {
		p.next()
		return matchers
	}

	for {
		label := p.expect(itemIdentifier, ctx)

		op := p.next().typ
		if !op.isOperator() {
			p.errorf("expected label matching operator but got %s", op)
		}
		var validOp = false
		for _, allowedOp := range operators {
			if op == allowedOp {
				validOp = true
			}
		}
		if !validOp {
			p.errorf("operator must be one of %q, is %q", operators, op)
		}

		val := trimOne(p.expect(itemString, ctx).val)

		// Map the item to the respective match type.
		var matchType metric.MatchType
		switch op {
		case itemEQL:
			matchType = metric.Equal
		case itemNEQ:
			matchType = metric.NotEqual
		case itemEQLRegex:
			matchType = metric.RegexMatch
		case itemNEQRegex:
			matchType = metric.RegexNoMatch
		default:
			p.errorf("item %q is not a metric match type", op)
		}

		m, err := metric.NewLabelMatcher(
			matchType,
			clientmodel.LabelName(label.val),
			clientmodel.LabelValue(val),
		)
		if err != nil {
			p.error(err)
		}

		matchers = append(matchers, m)

		// Terminate list if last matcher.
		if p.peek().typ != itemComma {
			break
		}
		p.next()
	}

	p.expect(itemRightBrace, ctx)

	return matchers
}

// metricSelector parses a new metric selector.
//
//		<metric_identifier> [<label_matchers>] [ offset <duration> ]
//		[<metric_identifier>] <label_matchers> [ offset <duration> ]
//
func (p *parser) vectorSelector(name string) *VectorSelector {
	const ctx = "metric selector"

	var matchers metric.LabelMatchers
	// Parse label matching if any.
	if t := p.peek(); t.typ == itemLeftBrace {
		matchers = p.labelMatchers(itemEQL, itemNEQ, itemEQLRegex, itemNEQRegex)
	}
	// Metric name must not be set in the label matchers and before at the same time.
	if name != "" {
		for _, m := range matchers {
			if m.Name == clientmodel.MetricNameLabel {
				p.errorf("metric name must not be set twice: %q or %q", name, m.Value)
			}
		}
		// Set name label matching.
		matchers = append(matchers, &metric.LabelMatcher{
			Type:  metric.Equal,
			Name:  clientmodel.MetricNameLabel,
			Value: clientmodel.LabelValue(name),
		})
	}

	if len(matchers) == 0 {
		p.errorf("vector selector must contain label matchers or metric name")
	}

	var err error
	var offset time.Duration
	// Parse optional offset.
	if p.peek().typ == itemOffset {
		p.next()
		offi := p.expect(itemDuration, ctx)

		offset, err = parseDuration(offi.val)
		if err != nil {
			p.error(err)
		}
	}
	return &VectorSelector{
		Name:          name,
		LabelMatchers: matchers,
		Offset:        offset,
	}
}

// expectType checks the type of the node and raises an error if it
// is not of the expected type.
func (p *parser) expectType(node Node, want ExprType, context string) {
	t := p.checkType(node)
	if t != want {
		p.errorf("expected type %s in %s, got %s", want, context, t)
	}
}

// check the types of the children of each node and raise an error
// if they do not form a valid node.
//
// Some of these checks are redundant as the the parsing stage does not allow
// them, but the costs are small and might reveal errors when making changes.
func (p *parser) checkType(node Node) (typ ExprType) {
	// For expressions the type is determined by their Type function.
	// Statements and lists do not have a type but are not invalid either.
	switch n := node.(type) {
	case Statements, Expressions, Statement:
		typ = ExprNone
	case Expr:
		typ = n.Type()
	default:
		p.errorf("unknown node type: %T", node)
	}

	// Recursively check correct typing for child nodes and raise
	// errors in case of bad typing.
	switch n := node.(type) {
	case Statements:
		for _, s := range n {
			p.expectType(s, ExprNone, "statement list")
		}
	case *AlertStmt:
		p.expectType(n.Expr, ExprVector, "alert statement")

	case *EvalStmt:
		ty := p.checkType(n.Expr)
		if ty == ExprNone {
			p.errorf("evaluation statement must have a valid expression type but got %s", ty)
		}

	case *RecordStmt:
		p.expectType(n.Expr, ExprVector, "record statement")

	case Expressions:
		for _, e := range n {
			ty := p.checkType(e)
			if ty == ExprNone {
				p.errorf("expression must have a valid expression type but got %s", ty)
			}
		}
	case *AggregateExpr:
		if !n.Op.isAggregator() {
			p.errorf("aggregation operator expected in aggregation expression but got %q", n.Op)
		}
		p.expectType(n.Expr, ExprVector, "aggregation expression")

	case *BinaryExpr:
		lt := p.checkType(n.LHS)
		rt := p.checkType(n.RHS)

		if !n.Op.isOperator() {
			p.errorf("only logical and arithmetic operators allowed in binary expression, got %q", n.Op)
		}
		if (lt != ExprScalar && lt != ExprVector) || (rt != ExprScalar && rt != ExprVector) {
			p.errorf("binary expression must contain only scalar and vector types")
		}

		if (lt != ExprVector || rt != ExprVector) && n.VectorMatching != nil {
			if len(n.VectorMatching.On) > 0 {
				p.errorf("vector matching only allowed between vectors")
			}
			n.VectorMatching = nil
		} else {
			// Both operands are vectors.
			if n.Op == itemLAND || n.Op == itemLOR {
				if n.VectorMatching.Card == CardOneToMany || n.VectorMatching.Card == CardManyToOne {
					p.errorf("no grouping allowed for AND and OR operations")
				}
				if n.VectorMatching.Card != CardManyToMany {
					p.errorf("AND and OR operations must always be many-to-many")
				}
			}
		}

		if (lt == ExprScalar || rt == ExprScalar) && (n.Op == itemLAND || n.Op == itemLOR) {
			p.errorf("AND and OR not allowed in binary scalar expression")
		}

	case *Call:
		nargs := len(n.Func.ArgTypes)
		if na := nargs - n.Func.OptionalArgs; na > len(n.Args) {
			p.errorf("expected at least %d argument(s) in call to %q, got %d", na, n.Func.Name, len(n.Args))
		}
		if nargs < len(n.Args) {
			p.errorf("expected at most %d argument(s) in call to %q, got %d", nargs, n.Func.Name, len(n.Args))
		}
		for i, arg := range n.Args {
			p.expectType(arg, n.Func.ArgTypes[i], fmt.Sprintf("call to function %q", n.Func.Name))
		}

	case *ParenExpr:
		p.checkType(n.Expr)

	case *UnaryExpr:
		if n.Op != itemADD && n.Op != itemSUB {
			p.errorf("only + and - operators allowed for unary expressions")
		}
		p.expectType(n.Expr, ExprScalar, "unary expression")

	case *NumberLiteral, *MatrixSelector, *StringLiteral, *VectorSelector:
		// Nothing to do for terminals.

	default:
		p.errorf("unknown node type: %T", node)
	}
	return
}

func parseDuration(ds string) (time.Duration, error) {
	dur, err := strutil.StringToDuration(ds)
	if err != nil {
		return 0, err
	}
	if dur == 0 {
		return 0, fmt.Errorf("duration must be greater than 0")
	}
	return dur, nil
}

// trimOne removes the first and last character from a string.
func trimOne(s string) string {
	if len(s) > 0 {
		s = s[1:]
	}
	if len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}
