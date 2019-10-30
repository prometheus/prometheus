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
	"go/token"
	"time"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

// Node is a generic interface for all nodes in an AST.
//
// Whenever numerous nodes are listed such as in a switch-case statement
// or a chain of function definitions (e.g. String(), expr(), etc.) convention is
// to list them as follows:
//
// 	- Statements
// 	- statement types (alphabetical)
// 	- ...
// 	- Expressions
// 	- expression types (alphabetical)
// 	- ...
//
type Node interface {
	// String representation of the node that returns the given node when parsed
	// as part of a valid query.
	String() string
	// position of first character belonging to the node
	Pos() token.Pos
	// position of first character immediately after the node
	EndPos() token.Pos
}

// Statement is a generic interface for all statements.
type Statement interface {
	Node

	// stmt ensures that no other type accidentally implements the interface
	stmt()
}

// EvalStmt holds an expression and information on the range it should
// be evaluated on.
// TODO(slrtbtfs) split into an Expr and an RangeExpr
type EvalStmt struct {
	Expr   Expr // Expression to be evaluated.
	endPos token.Pos

	// The time boundaries for the evaluation. If Start equals End an instant
	// is evaluated.
	Start, End time.Time
	// Time between two evaluated instants for the range [Start:End].
	Interval time.Duration
}

func (e *EvalStmt) Pos() token.Pos {
	return e.Expr.Pos()
}

func (e *EvalStmt) EndPos() token.Pos {
	return e.endPos
}

func (*EvalStmt) stmt() {}

// Expr is a generic interface for all expression types.
type Expr interface {
	Node

	// Type returns the type the expression evaluates to. It does not perform
	// in-depth checks as this is done at parsing-time.
	Type() ValueType
	// expr ensures that no other types accidentally implement the interface.
	expr()
}

// Expressions is a list of expression nodes that implements Node.
type Expressions []Expr

// AggregateExpr represents an aggregation operation on a Vector.
type AggregateExpr struct {
	pos      token.Pos
	endPos   token.Pos
	Op       ItemType // The used aggregation operation.
	Expr     Expr     // The Vector expression over which is aggregated.
	Param    Expr     // Parameter used by some aggregators.
	Grouping []string // The labels by which to group the Vector.
	Without  bool     // Whether to drop the given labels rather than keep them.
}

func (e *AggregateExpr) Pos() token.Pos {
	return e.pos
}
func (e *AggregateExpr) EndPos() token.Pos {
	return e.endPos
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

func (e *BinaryExpr) Pos() token.Pos {
	return e.LHS.Pos()
}
func (e *BinaryExpr) EndPos() token.Pos {
	return e.RHS.EndPos()
}

// Call represents a function call.
type Call struct {
	pos    token.Pos
	LParen token.Pos
	RParen token.Pos
	Func   *Function // The function that was called.
	Args   []Expr    // Arguments used in the call.
}

func (e *Call) Pos() token.Pos {
	return e.pos
}
func (e *Call) EndPos() token.Pos {
	return e.RParen + 1
}

// MatrixSelector represents a Matrix selection.
type MatrixSelector struct {
	pos           token.Pos
	LBracket      token.Pos
	RBracket      token.Pos
	endPos        token.Pos
	Name          string
	Range         time.Duration
	Offset        time.Duration
	LabelMatchers []*labels.Matcher

	// The unexpanded seriesSet populated at query preparation time.
	unexpandedSeriesSet storage.SeriesSet
	series              []storage.Series
}

func (e *MatrixSelector) Pos() token.Pos {
	return e.pos
}
func (e *MatrixSelector) EndPos() token.Pos {
	return e.endPos
}

// SubqueryExpr represents a subquery.
type SubqueryExpr struct {
	Expr     Expr
	LBracket token.Pos
	RBracket token.Pos
	endPos   token.Pos
	Range    time.Duration
	Offset   time.Duration
	Step     time.Duration
}

func (e *SubqueryExpr) Pos() token.Pos {
	return e.Expr.Pos()
}
func (e *SubqueryExpr) EndPos() token.Pos {
	return e.endPos
}

// NumberLiteral represents a number.
type NumberLiteral struct {
	pos    token.Pos
	endPos token.Pos
	Val    float64
}

func (e *NumberLiteral) Pos() token.Pos {
	return e.pos
}
func (e *NumberLiteral) EndPos() token.Pos {
	return e.endPos
}

// ParenExpr wraps an expression so it cannot be disassembled as a consequence
// of operator precedence.
type ParenExpr struct {
	LParen token.Pos
	RParen token.Pos
	Expr   Expr
}

func (e *ParenExpr) Pos() token.Pos {
	return e.LParen
}

func (e *ParenExpr) EndPos() token.Pos {
	return e.RParen + 1
}

// StringLiteral represents a string.
type StringLiteral struct {
	LQuote token.Pos
	RQuote token.Pos
	Val    string
}

func (e *StringLiteral) Pos() token.Pos {
	return e.LQuote
}

func (e *StringLiteral) EndPos() token.Pos {
	return e.RQuote + 1
}

// UnaryExpr represents a unary operation on another expression.
// Currently unary operations are only supported for Scalars.
type UnaryExpr struct {
	pos  token.Pos
	Op   ItemType
	Expr Expr
}

func (e *UnaryExpr) Pos() token.Pos {
	return e.pos
}

func (e *UnaryExpr) EndPos() token.Pos {
	return e.Expr.EndPos()
}

// VectorSelector represents a Vector selection.
type VectorSelector struct {
	pos           token.Pos
	LBrace        token.Pos
	RBrace        token.Pos
	endPos        token.Pos
	Name          string
	Offset        time.Duration
	LabelMatchers []*labels.Matcher

	// The unexpanded seriesSet populated at query preparation time.
	unexpandedSeriesSet storage.SeriesSet
	series              []storage.Series
}

func (e *VectorSelector) Pos() token.Pos {
	return e.pos
}
func (e *VectorSelector) EndPos() token.Pos {
	return e.endPos
}
func (e *AggregateExpr) Type() ValueType  { return ValueTypeVector }
func (e *Call) Type() ValueType           { return e.Func.ReturnType }
func (e *MatrixSelector) Type() ValueType { return ValueTypeMatrix }
func (e *SubqueryExpr) Type() ValueType   { return ValueTypeMatrix }
func (e *NumberLiteral) Type() ValueType  { return ValueTypeScalar }
func (e *ParenExpr) Type() ValueType      { return e.Expr.Type() }
func (e *StringLiteral) Type() ValueType  { return ValueTypeString }
func (e *UnaryExpr) Type() ValueType      { return e.Expr.Type() }
func (e *VectorSelector) Type() ValueType { return ValueTypeVector }
func (e *BinaryExpr) Type() ValueType {
	if e.LHS.Type() == ValueTypeScalar && e.RHS.Type() == ValueTypeScalar {
		return ValueTypeScalar
	}
	return ValueTypeVector
}

func (*AggregateExpr) expr()  {}
func (*BinaryExpr) expr()     {}
func (*Call) expr()           {}
func (*MatrixSelector) expr() {}
func (*SubqueryExpr) expr()   {}
func (*NumberLiteral) expr()  {}
func (*ParenExpr) expr()      {}
func (*StringLiteral) expr()  {}
func (*UnaryExpr) expr()      {}
func (*VectorSelector) expr() {}

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

// TODO(slrtbtfs) Implement Node interface and move to ast.go
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
	path = append(path, node)

	switch n := node.(type) {
	case *EvalStmt:
		if err := Walk(v, n.Expr, path); err != nil {
			return err
		}

	case *AggregateExpr:
		if n.Param != nil {
			if err := Walk(v, n.Param, path); err != nil {
				return err
			}
		}
		if err := Walk(v, n.Expr, path); err != nil {
			return err
		}

	case *BinaryExpr:
		if err := Walk(v, n.LHS, path); err != nil {
			return err
		}
		if err := Walk(v, n.RHS, path); err != nil {
			return err
		}

	case *Call:
		for _, e := range n.Args {
			if err := Walk(v, e, path); err != nil {
				return err
			}
		}

	case *SubqueryExpr:
		if err := Walk(v, n.Expr, path); err != nil {
			return err
		}

	case *ParenExpr:
		if err := Walk(v, n.Expr, path); err != nil {
			return err
		}

	case *UnaryExpr:
		if err := Walk(v, n.Expr, path); err != nil {
			return err
		}

	case *MatrixSelector, *NumberLiteral, *StringLiteral, *VectorSelector:
		// nothing to do

	default:
		panic(errors.Errorf("promql.Walk: unhandled node type %T", node))
	}

	_, err = v.Visit(nil, nil)
	return err
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
func Inspect(node Node, f inspector) {
	//nolint: errcheck
	Walk(inspector(f), node, nil)
}
