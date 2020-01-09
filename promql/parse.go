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
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/strutil"
)

var parserPool = sync.Pool{
	New: func() interface{} {
		return &parser{}
	},
}

type parser struct {
	lex Lexer

	inject    ItemType
	injecting bool

	yyParser yyParserImpl

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
func ParseExpr(input string) (expr Expr, err error) {
	p := newParser(input)
	defer parserPool.Put(p)
	defer p.recover(&err)

	expr = p.parseGenerated(START_EXPRESSION).(Expr)
	err = p.typecheck(expr)

	return expr, err
}

// ParseMetric parses the input into a metric
func ParseMetric(input string) (m labels.Labels, err error) {
	p := newParser(input)
	defer parserPool.Put(p)
	defer p.recover(&err)

	return p.parseGenerated(START_METRIC).(labels.Labels), nil
}

// ParseMetricSelector parses the provided textual metric selector into a list of
// label matchers.
func ParseMetricSelector(input string) (m []*labels.Matcher, err error) {
	p := newParser(input)
	defer parserPool.Put(p)
	defer p.recover(&err)

	return p.parseGenerated(START_METRIC_SELECTOR).(*VectorSelector).LabelMatchers, nil
}

// newParser returns a new parser.
func newParser(input string) *parser {
	p := parserPool.Get().(*parser)

	p.injecting = false

	// Clear lexer struct before reusing.
	p.lex = Lexer{
		input: input,
		state: lexStatements,
	}
	return p
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

	defer parserPool.Put(p)
	defer p.recover(&err)

	result := p.parseGenerated(START_SERIES_DESCRIPTION).(*seriesDescription)

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
	errMsg.WriteString(p.yyParser.lval.item.desc())

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
	var typ ItemType

	if p.injecting {
		p.injecting = false
		return int(p.inject)
	} else {
		// Skip comments.
		for {
			p.lex.NextItem(&lval.item)
			typ = lval.item.Typ
			if typ != COMMENT {
				break
			}
		}
	}

	if typ == ERROR {
		p.errorf("%s", lval.item.Val)
	}

	if typ == EOF {
		lval.item.Typ = EOF
		p.InjectItem(0)
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

	p.inject = typ
	p.injecting = true
}
func (p *parser) newBinaryExpression(lhs Node, op Item, modifiers Node, rhs Node) *BinaryExpr {
	ret := modifiers.(*BinaryExpr)

	ret.LHS = lhs.(Expr)
	ret.RHS = rhs.(Expr)
	ret.Op = op.Typ

	if ret.ReturnBool && !op.Typ.isComparisonOperator() {
		p.errorf("bool modifier can only be used on comparison operators")
	}

	if op.Typ.isComparisonOperator() && !ret.ReturnBool && ret.RHS.Type() == ValueTypeScalar && ret.LHS.Type() == ValueTypeScalar {
		p.errorf("comparisons between scalars must use BOOL modifier")
	}

	if op.Typ.isSetOperator() && ret.VectorMatching.Card == CardOneToOne {
		ret.VectorMatching.Card = CardManyToMany
	}

	for _, l1 := range ret.VectorMatching.MatchingLabels {
		for _, l2 := range ret.VectorMatching.Include {
			if l1 == l2 && ret.VectorMatching.On {
				p.errorf("label %q must not occur in ON and GROUP clause at once", l1)
			}
		}
	}

	return ret
}

func (p *parser) newVectorSelector(name string, labelMatchers []*labels.Matcher) *VectorSelector {
	ret := &VectorSelector{LabelMatchers: labelMatchers}

	if name != "" {
		ret.Name = name

		for _, m := range ret.LabelMatchers {
			if m.Name == labels.MetricName {
				p.errorf("metric name must not be set twice: %q or %q", name, m.Value)
			}
		}

		nameMatcher, err := labels.NewMatcher(labels.MatchEqual, labels.MetricName, name)
		if err != nil {
			panic(err) // Must not happen with labels.MatchEqual
		}
		ret.LabelMatchers = append(ret.LabelMatchers, nameMatcher)
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

func (p *parser) newAggregateExpr(op Item, modifier Node, args Node) (ret *AggregateExpr) {
	ret = modifier.(*AggregateExpr)
	arguments := args.(Expressions)

	ret.Op = op.Typ

	if len(arguments) == 0 {
		p.errorf("no arguments for aggregate expression provided")

		// Currently p.errorf() panics, so this return is not needed
		// at the moment.
		// However, this behaviour is likely to be changed in the
		// future. In case of having non-panicking errors this
		// return prevents invalid array accesses
		return
	}

	desiredArgs := 1
	if ret.Op.isAggregatorWithParam() {
		desiredArgs = 2

		ret.Param = arguments[0]
	}

	if len(arguments) != desiredArgs {
		p.errorf("wrong number of arguments for aggregate expression provided, expected %d, got %d", desiredArgs, len(arguments))
		return
	}

	ret.Expr = arguments[desiredArgs-1]

	return ret
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
func (p *parser) parseGenerated(startSymbol ItemType) interface{} {
	p.InjectItem(startSymbol)

	p.yyParser.Parse(p)

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

func (p *parser) addOffset(e Node, offset time.Duration) {
	var offsetp *time.Duration

	switch s := e.(type) {
	case *VectorSelector:
		offsetp = &s.Offset
	case *MatrixSelector:
		offsetp = &s.Offset
	case *SubqueryExpr:
		offsetp = &s.Offset
	default:
		p.errorf("offset modifier must be preceded by an instant or range selector, but follows a %T instead", e)
		return
	}

	// it is already ensured by parseDuration func that there never will be a zero offset modifier
	if *offsetp != 0 {
		p.errorf("offset may not be set multiple times")
	} else {
		*offsetp = offset
	}

}
