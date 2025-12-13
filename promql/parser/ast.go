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

package parser

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/storage"
)

// Node is a generic interface for all nodes in an AST.
//
// Whenever numerous nodes are listed such as in a switch-case statement
// or a chain of function definitions (e.g. String(), PromQLExpr(), etc.) convention is
// to list them as follows:
//
//   - Statements
//   - statement types (alphabetical)
//   - ...
//   - Expressions
//   - expression types (alphabetical)
//   - ...
type Node interface {
	// String representation of the node that returns the given node when parsed
	// as part of a valid query.
	String() string

	// Pretty returns the prettified representation of the node.
	// It uses the level information to determine at which level/depth the current
	// node is in the AST and uses this to apply indentation.
	Pretty(level int) string

	// PositionRange returns the position of the AST Node in the query string.
	PositionRange() posrange.PositionRange
}

// Statement is a generic interface for all statements.
type Statement interface {
	Node

	// PromQLStmt ensures that no other type accidentally implements the interface
	PromQLStmt()
}

// EvalStmt holds an expression and information on the range it should
// be evaluated on.
type EvalStmt struct {
	Expr Expr // Expression to be evaluated.

	// The time boundaries for the evaluation. If Start equals End an instant
	// is evaluated.
	Start, End time.Time
	// Time between two evaluated instants for the range [Start:End].
	Interval time.Duration
	// Lookback delta to use for this evaluation.
	LookbackDelta time.Duration
}

func (*EvalStmt) PromQLStmt() {}

// Expr is a generic interface for all expression types.
type Expr interface {
	Node

	// Type returns the type the expression evaluates to. It does not perform
	// in-depth checks as this is done at parsing-time.
	Type() ValueType
	// PromQLExpr ensures that no other types accidentally implement the interface.
	PromQLExpr()
}

// Expressions is a list of expression nodes that implements Node.
type Expressions []Expr

// AggregateExpr represents an aggregation operation on a Vector.
type AggregateExpr struct {
	Op       ItemType // The used aggregation operation.
	Expr     Expr     // The Vector expression over which is aggregated.
	Param    Expr     // Parameter used by some aggregators.
	Grouping []string // The labels by which to group the Vector.
	Without  bool     // Whether to drop the given labels rather than keep them.
	PosRange posrange.PositionRange
}

// BinaryExpr represents a binary expression between two child expressions.
type BinaryExpr struct {
	Op       ItemType // The operation of the expression.
	LHS, RHS Expr     // The operands on the respective sides of the operator.

	// The matching behavior for the operation if both operands are Vectors.
	// If they are not this field is nil.
	VectorMatching *VectorMatching

	// If a comparison operator, return 0/1 rather than filtering.
	ReturnBool bool
}

// DurationExpr represents a binary expression between two duration expressions.
type DurationExpr struct {
	Op       ItemType // The operation of the expression.
	LHS, RHS Expr     // The operands on the respective sides of the operator.
	Wrapped  bool     // Set when the duration is wrapped in parentheses.

	StartPos posrange.Pos // For unary operations, step(), and range(), the start position of the operator.
	EndPos   posrange.Pos // For step() and range(), the end position of the operator.
}

// Call represents a function call.
type Call struct {
	Func *Function   // The function that was called.
	Args Expressions // Arguments used in the call.

	PosRange posrange.PositionRange
}

// MatrixSelector represents a Matrix selection.
type MatrixSelector struct {
	// It is safe to assume that this is an VectorSelector
	// if the parser hasn't returned an error.
	VectorSelector Expr
	Range          time.Duration
	RangeExpr      *DurationExpr
	EndPos         posrange.Pos
}

// SubqueryExpr represents a subquery.
type SubqueryExpr struct {
	Expr      Expr
	Range     time.Duration
	RangeExpr *DurationExpr
	// OriginalOffset is the actual offset that was set in the query.
	OriginalOffset time.Duration
	// OriginalOffsetExpr is the actual offset expression that was set in the query.
	OriginalOffsetExpr *DurationExpr
	// Offset is the offset used during the query execution
	// which is calculated using the original offset, offset expression, at modifier time,
	// eval time, and subquery offsets in the AST tree.
	Offset     time.Duration
	Timestamp  *int64
	StartOrEnd ItemType // Set when @ is used with start() or end()
	Step       time.Duration
	StepExpr   *DurationExpr

	EndPos posrange.Pos
}

// NumberLiteral represents a number.
type NumberLiteral struct {
	Val float64

	Duration bool // Used to format the number as a duration.
	PosRange posrange.PositionRange
}

// ParenExpr wraps an expression so it cannot be disassembled as a consequence
// of operator precedence.
type ParenExpr struct {
	Expr     Expr
	PosRange posrange.PositionRange
}

// StringLiteral represents a string.
type StringLiteral struct {
	Val      string
	PosRange posrange.PositionRange
}

// UnaryExpr represents a unary operation on another expression.
// Currently unary operations are only supported for Scalars.
type UnaryExpr struct {
	Op   ItemType
	Expr Expr

	StartPos posrange.Pos
}

// StepInvariantExpr represents a query which evaluates to the same result
// irrespective of the evaluation time given the raw samples from TSDB remain unchanged.
// Currently this is only used for engine optimisations and the parser does not produce this.
type StepInvariantExpr struct {
	Expr Expr
}

func (e *StepInvariantExpr) String() string { return e.Expr.String() }

func (e *StepInvariantExpr) PositionRange() posrange.PositionRange {
	return e.Expr.PositionRange()
}

// VectorSelector represents a Vector selection.
type VectorSelector struct {
	Name string
	// OriginalOffset is the actual offset calculated from OriginalOffsetExpr.
	OriginalOffset time.Duration
	// OriginalOffsetExpr is the actual offset that was set in the query.
	OriginalOffsetExpr *DurationExpr
	// Offset is the offset used during the query execution
	// which is calculated using the original offset, at modifier time,
	// eval time, and subquery offsets in the AST tree.
	Offset               time.Duration
	Timestamp            *int64
	SkipHistogramBuckets bool     // Set when decoding native histogram buckets is not needed for query evaluation.
	StartOrEnd           ItemType // Set when @ is used with start() or end()
	LabelMatchers        []*labels.Matcher

	// The unexpanded seriesSet populated at query preparation time.
	UnexpandedSeriesSet storage.SeriesSet
	Series              []storage.Series

	// BypassEmptyMatcherCheck is true when the VectorSelector isn't required to have at least one matcher matching the empty string.
	// This is the case when VectorSelector is used to represent the info function's second argument.
	BypassEmptyMatcherCheck bool

	// Anchored is true when the VectorSelector is anchored.
	Anchored bool
	// Smoothed is true when the VectorSelector is smoothed.
	Smoothed bool

	PosRange posrange.PositionRange
}

// TestStmt is an internal helper statement that allows execution
// of an arbitrary function during handling. It is used to test the Engine.
type TestStmt func(context.Context) error

func (TestStmt) String() string      { return "test statement" }
func (TestStmt) PromQLStmt()         {}
func (t TestStmt) Pretty(int) string { return t.String() }

func (TestStmt) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: -1,
		End:   -1,
	}
}
func (*AggregateExpr) Type() ValueType  { return ValueTypeVector }
func (e *Call) Type() ValueType         { return e.Func.ReturnType }
func (*MatrixSelector) Type() ValueType { return ValueTypeMatrix }
func (*SubqueryExpr) Type() ValueType   { return ValueTypeMatrix }
func (*NumberLiteral) Type() ValueType  { return ValueTypeScalar }
func (e *ParenExpr) Type() ValueType    { return e.Expr.Type() }
func (*StringLiteral) Type() ValueType  { return ValueTypeString }
func (e *UnaryExpr) Type() ValueType    { return e.Expr.Type() }
func (*VectorSelector) Type() ValueType { return ValueTypeVector }
func (e *BinaryExpr) Type() ValueType {
	if e.LHS.Type() == ValueTypeScalar && e.RHS.Type() == ValueTypeScalar {
		return ValueTypeScalar
	}
	return ValueTypeVector
}
func (e *StepInvariantExpr) Type() ValueType { return e.Expr.Type() }
func (*DurationExpr) Type() ValueType        { return ValueTypeScalar }

func (*AggregateExpr) PromQLExpr()     {}
func (*BinaryExpr) PromQLExpr()        {}
func (*Call) PromQLExpr()              {}
func (*MatrixSelector) PromQLExpr()    {}
func (*SubqueryExpr) PromQLExpr()      {}
func (*NumberLiteral) PromQLExpr()     {}
func (*ParenExpr) PromQLExpr()         {}
func (*StringLiteral) PromQLExpr()     {}
func (*UnaryExpr) PromQLExpr()         {}
func (*VectorSelector) PromQLExpr()    {}
func (*StepInvariantExpr) PromQLExpr() {}
func (*DurationExpr) PromQLExpr()      {}

// VectorMatchCardinality describes the cardinality relationship
// of two Vectors in a binary operation.
type VectorMatchCardinality int

const (
	CardOneToOne VectorMatchCardinality = iota
	CardManyToOne
	CardOneToMany
	CardManyToMany
)

func (vmc VectorMatchCardinality) String() string {
	switch vmc {
	case CardOneToOne:
		return "one-to-one"
	case CardManyToOne:
		return "many-to-one"
	case CardOneToMany:
		return "one-to-many"
	case CardManyToMany:
		return "many-to-many"
	}
	panic("promql.VectorMatchCardinality.String: unknown match cardinality")
}

// VectorMatching describes how elements from two Vectors in a binary
// operation are supposed to be matched.
type VectorMatching struct {
	// The cardinality of the two Vectors.
	Card VectorMatchCardinality
	// MatchingLabels contains the labels which define equality of a pair of
	// elements from the Vectors.
	MatchingLabels []string
	// On includes the given label names from matching,
	// rather than excluding them.
	On bool
	// Include contains additional labels that should be included in
	// the result from the side with the lower cardinality.
	Include []string
}

// Visitor allows visiting a Node and its child nodes. The Visit method is
// invoked for each node with the path leading to the node provided additionally.
// If the result visitor w is not nil and no error, Walk visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil, nil).
type Visitor interface {
	Visit(node Node, path []Node) (w Visitor, err error)
}

// Walk traverses an AST in depth-first order: It starts by calling
// v.Visit(node, path); node must not be nil. If the visitor w returned by
// v.Visit(node, path) is not nil and the visitor returns no error, Walk is
// invoked recursively with visitor w for each of the non-nil children of node,
// followed by a call of w.Visit(nil), returning an error
// As the tree is descended the path of previous nodes is provided.
func Walk(v Visitor, node Node, path []Node) error {
	var err error
	if v, err = v.Visit(node, path); v == nil || err != nil {
		return err
	}
	var pathToHere []Node // Initialized only when needed.

	for e := range ChildrenIter(node) {
		if pathToHere == nil {
			pathToHere = append(path, node)
		}
		if err := Walk(v, e, pathToHere); err != nil {
			return err
		}
	}

	_, err = v.Visit(nil, nil)
	return err
}

func ExtractSelectors(expr Expr) [][]*labels.Matcher {
	var selectors [][]*labels.Matcher
	Inspect(expr, func(node Node, _ []Node) error {
		vs, ok := node.(*VectorSelector)
		if ok {
			selectors = append(selectors, vs.LabelMatchers)
		}
		return nil
	})
	return selectors
}

type inspector func(Node, []Node) error

func (f inspector) Visit(node Node, path []Node) (Visitor, error) {
	if err := f(node, path); err != nil {
		return nil, err
	}

	return f, nil
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node, path); node must not be nil. If f returns a nil error, Inspect invokes f
// for all the non-nil children of node, recursively.
// Note: path may be overwritten after f returns; copy path if you need to retain it.
func Inspect(node Node, f inspector) {
	var pathBuf [4]Node        // To reduce allocations during recursion.
	Walk(f, node, pathBuf[:0]) //nolint:errcheck
}

// ChildrenIter returns an iterator over all child nodes of a syntax tree node.
func ChildrenIter(node Node) func(func(Node) bool) {
	return func(yield func(Node) bool) {
		// According to lore, these switches have significantly better performance than interfaces
		switch n := node.(type) {
		case *EvalStmt:
			yield(n.Expr)
		case Expressions:
			for _, e := range n {
				if !yield(e) {
					return
				}
			}
		case *AggregateExpr:
			if n.Expr != nil {
				if !yield(n.Expr) {
					return
				}
			}
			if n.Param != nil {
				yield(n.Param)
			}
		case *BinaryExpr:
			if !yield(n.LHS) {
				return
			}
			yield(n.RHS)
		case *Call:
			for _, e := range n.Args {
				if !yield(e) {
					return
				}
			}
		case *SubqueryExpr:
			yield(n.Expr)
		case *ParenExpr:
			yield(n.Expr)
		case *UnaryExpr:
			yield(n.Expr)
		case *MatrixSelector:
			yield(n.VectorSelector)
		case *StepInvariantExpr:
			yield(n.Expr)
		case *NumberLiteral, *StringLiteral, *VectorSelector:
			// nothing to do
		default:
			panic(fmt.Errorf("promql.ChildrenIter: unhandled node type %T", node))
		}
	}
}

// Children returns a list of all child nodes of a syntax tree node.
// Implemented for backwards-compatibility; prefer ChildrenIter().
func Children(node Node) []Node {
	ret := []Node{}
	for e := range ChildrenIter(node) {
		ret = append(ret, e)
	}
	return ret
}

// mergeRanges is a helper function to merge the PositionRanges of two Nodes.
// Note that the arguments must be in the same order as they
// occur in the input string.
func mergeRanges(first, last Node) posrange.PositionRange {
	return posrange.PositionRange{
		Start: first.PositionRange().Start,
		End:   last.PositionRange().End,
	}
}

// PositionRange implements the Node interface.
// This makes it possible to call mergeRanges on them.
func (i *Item) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: i.Pos,
		End:   i.Pos + posrange.Pos(len(i.Val)),
	}
}

func (e *AggregateExpr) PositionRange() posrange.PositionRange {
	return e.PosRange
}

func (e *BinaryExpr) PositionRange() posrange.PositionRange {
	return mergeRanges(e.LHS, e.RHS)
}

func (e *DurationExpr) PositionRange() posrange.PositionRange {
	if e.Op == STEP || e.Op == RANGE {
		return posrange.PositionRange{
			Start: e.StartPos,
			End:   e.EndPos,
		}
	}
	if e.RHS == nil {
		return posrange.PositionRange{
			Start: e.StartPos,
			End:   e.RHS.PositionRange().End,
		}
	}
	if e.LHS == nil {
		return posrange.PositionRange{
			Start: e.StartPos,
			End:   e.RHS.PositionRange().End,
		}
	}
	return mergeRanges(e.LHS, e.RHS)
}

func (e *Call) PositionRange() posrange.PositionRange {
	return e.PosRange
}

func (e *EvalStmt) PositionRange() posrange.PositionRange {
	return e.Expr.PositionRange()
}

func (e Expressions) PositionRange() posrange.PositionRange {
	if len(e) == 0 {
		// Position undefined.
		return posrange.PositionRange{
			Start: -1,
			End:   -1,
		}
	}
	return mergeRanges(e[0], e[len(e)-1])
}

func (e *MatrixSelector) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: e.VectorSelector.PositionRange().Start,
		End:   e.EndPos,
	}
}

func (e *SubqueryExpr) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: e.Expr.PositionRange().Start,
		End:   e.EndPos,
	}
}

func (e *NumberLiteral) PositionRange() posrange.PositionRange {
	return e.PosRange
}

func (e *ParenExpr) PositionRange() posrange.PositionRange {
	return e.PosRange
}

func (e *StringLiteral) PositionRange() posrange.PositionRange {
	return e.PosRange
}

func (e *UnaryExpr) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: e.StartPos,
		End:   e.Expr.PositionRange().End,
	}
}

func (e *VectorSelector) PositionRange() posrange.PositionRange {
	return e.PosRange
}
