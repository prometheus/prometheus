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

package searchexpr

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/storage"
)

// The deepest parsed AST has one term below 31 binary nodes and 16 NOT nodes.
const maxCompiledExpressionDepth = maxExpressionTerms + maxExpressionNestingDepth

// TermFilterFactory returns the filter for one term. The filter must match the
// term itself, because Compile applies the negation. negated is true when the
// term is under an odd number of NOTs. Filters must score in [0, 1].
type TermFilterFactory func(term string, negated bool) (storage.Filter, error)

// Compile validates expr and converts it into a storage.Filter. The score is
// the mean score of the positive terms that the match depends on. Negated
// terms do not add to the score. OR uses the operand with the higher mean.
// The filter is safe for concurrent use if its term filters are.
func Compile(expr Expr, newTermFilter TermFilterFactory) (storage.Filter, error) {
	if newTermFilter == nil {
		return nil, errors.New("search expression: term filter factory is nil")
	}
	if err := Validate(expr); err != nil {
		return nil, err
	}
	root, err := compileEvaluator(expr, false, newTermFilter)
	if err != nil {
		return nil, err
	}
	return &compiledFilter{root: root}, nil
}

// Validate returns an error if Compile cannot compile expr. Every accepting
// path must require a positive term, and the Parse limits apply.
func Validate(expr Expr) error {
	if expr == nil {
		return errors.New("search expression: cannot compile a nil expression")
	}
	anchored, err := validateExpression(expr, false, 1, new(int))
	if err != nil {
		return err
	}
	if !anchored {
		return errors.New("search expression: every accepting path must require a positive term")
	}
	return nil
}

// sum and count give the mean of the positive term scores.
type evalResult struct {
	accepted bool
	scored   bool
	sum      float64
	count    int
}

func (r evalResult) mean() float64 {
	return r.sum / float64(r.count)
}

// perfect relies on no term scoring more than 1.0.
func (r evalResult) perfect() bool {
	return r.sum >= float64(r.count)
}

type evaluator interface {
	evaluate(value string) evalResult
}

type compiledFilter struct {
	root evaluator
}

// Validate guarantees that an accepted root has a score.
func (f *compiledFilter) Accept(value string) (bool, float64) {
	result := f.root.evaluate(value)
	if !result.accepted {
		return false, 0
	}
	return true, result.mean()
}

type termEvaluator struct {
	filter  storage.Filter
	negated bool
}

func (e *termEvaluator) evaluate(value string) evalResult {
	accepted, score := e.filter.Accept(value)
	if e.negated {
		return evalResult{accepted: !accepted}
	}
	if !accepted {
		return evalResult{}
	}
	return evalResult{accepted: true, scored: true, sum: score, count: 1}
}

type andEvaluator struct {
	left  evaluator
	right evaluator
}

func (e *andEvaluator) evaluate(value string) evalResult {
	left := e.left.evaluate(value)
	if !left.accepted {
		return evalResult{}
	}
	right := e.right.evaluate(value)
	if !right.accepted {
		return evalResult{}
	}
	return combineScores(left, right)
}

type orEvaluator struct {
	left  evaluator
	right evaluator
}

func (e *orEvaluator) evaluate(value string) evalResult {
	left := e.left.evaluate(value)
	if left.accepted && left.scored && left.perfect() {
		return left
	}
	right := e.right.evaluate(value)

	switch {
	case !left.accepted:
		return right
	case !right.accepted:
		return left
	default:
		return bestScore(left, right)
	}
}

func bestScore(left, right evalResult) evalResult {
	switch {
	case left.scored && right.scored:
		if left.mean() > right.mean() {
			return left
		}
		return right
	case left.scored:
		return left
	case right.scored:
		return right
	default:
		return evalResult{accepted: true}
	}
}

func combineScores(left, right evalResult) evalResult {
	switch {
	case left.scored && right.scored:
		return evalResult{accepted: true, scored: true, sum: left.sum + right.sum, count: left.count + right.count}
	case left.scored:
		return left
	case right.scored:
		return right
	default:
		return evalResult{accepted: true}
	}
}

// validateExpression reports whether every accepting path has a positive term.
func validateExpression(expr Expr, negated bool, depth int, termCount *int) (bool, error) {
	if depth > maxCompiledExpressionDepth {
		return false, fmt.Errorf("search expression: AST depth exceeds maximum of %d", maxCompiledExpressionDepth)
	}
	validateChildren := func(left, right Expr) (bool, bool, error) {
		return mapChildren(left, right, func(expr Expr) (bool, error) {
			return validateExpression(expr, negated, depth+1, termCount)
		})
	}

	switch expr := expr.(type) {
	case Term:
		if expr.Value == "" {
			return false, errors.New("search expression: cannot compile an empty term")
		}
		(*termCount)++
		if *termCount > maxExpressionTerms {
			return false, fmt.Errorf("search expression: term count exceeds maximum of %d", maxExpressionTerms)
		}
		return !negated, nil

	case And:
		left, right, err := validateChildren(expr.Left, expr.Right)
		if err != nil {
			return false, err
		}
		if negated {
			return left && right, nil
		}
		return left || right, nil

	case Or:
		left, right, err := validateChildren(expr.Left, expr.Right)
		if err != nil {
			return false, err
		}
		if negated {
			return left || right, nil
		}
		return left && right, nil

	case Not:
		if expr.Expr == nil {
			return false, errors.New("search expression: cannot compile NOT with a nil operand")
		}
		return validateExpression(expr.Expr, !negated, depth+1, termCount)

	default:
		return false, fmt.Errorf("search expression: cannot compile node %T", expr)
	}
}

// compileEvaluator expects an expression that Validate accepts.
func compileEvaluator(expr Expr, negated bool, newTermFilter TermFilterFactory) (evaluator, error) {
	switch expr := expr.(type) {
	case Term:
		filter, err := newTermFilter(expr.Value, negated)
		if err != nil {
			return nil, fmt.Errorf("search expression: compile term %q: %w", expr.Value, err)
		}
		if filter == nil {
			return nil, fmt.Errorf("search expression: term filter factory returned nil for %q", expr.Value)
		}
		return &termEvaluator{filter: filter, negated: negated}, nil

	case And:
		left, right, err := compileChildren(expr.Left, expr.Right, negated, newTermFilter)
		if err != nil {
			return nil, err
		}
		if negated {
			return &orEvaluator{left: left, right: right}, nil
		}
		return &andEvaluator{left: left, right: right}, nil

	case Or:
		left, right, err := compileChildren(expr.Left, expr.Right, negated, newTermFilter)
		if err != nil {
			return nil, err
		}
		if negated {
			return &andEvaluator{left: left, right: right}, nil
		}
		return &orEvaluator{left: left, right: right}, nil

	case Not:
		return compileEvaluator(expr.Expr, !negated, newTermFilter)

	default:
		return nil, fmt.Errorf("search expression: cannot compile node %T", expr)
	}
}

func compileChildren(left, right Expr, negated bool, newTermFilter TermFilterFactory) (evaluator, evaluator, error) {
	return mapChildren(left, right, func(expr Expr) (evaluator, error) {
		return compileEvaluator(expr, negated, newTermFilter)
	})
}

func mapChildren[T any](leftExpr, rightExpr Expr, mapChild func(Expr) (T, error)) (T, T, error) {
	var zero T
	if leftExpr == nil || rightExpr == nil {
		return zero, zero, errors.New("search expression: cannot compile a binary expression with a nil operand")
	}
	left, err := mapChild(leftExpr)
	if err != nil {
		return zero, zero, err
	}
	right, err := mapChild(rightExpr)
	if err != nil {
		return zero, zero, err
	}
	return left, right, nil
}
