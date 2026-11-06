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
	"math/bits"

	"github.com/prometheus/prometheus/storage"
)

// The deepest parsed AST has one term below 31 binary nodes and 16 NOT nodes.
const maxCompiledExpressionDepth = maxExpressionTerms + maxExpressionNestingDepth

// TermFilterFactory returns the filter for one term. The filter must match the
// term itself, because Compile applies the negation. negated is true when the
// term is under an odd number of NOTs. Filters must score in [0, 1].
type TermFilterFactory func(term string, negated bool) (storage.Filter, error)

// Compile validates expr and converts it into a storage.Filter. The score is
// the highest mean of the positive term scores on any accepting path. Negated
// terms do not add to a path's score.
// The filter is safe for concurrent use if its term filters are.
func Compile(expr Expr, newTermFilter TermFilterFactory) (storage.Filter, error) {
	if newTermFilter == nil {
		return nil, errors.New("search expression: term filter factory is nil")
	}
	if err := Validate(expr); err != nil {
		return nil, err
	}
	root, err := compileEvaluator(expr, false, false, newTermFilter)
	if err != nil {
		return nil, err
	}
	return &compiledFilter{root: root.scalar}, nil
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
	sum      float64
	count    int
}

func (r evalResult) mean() float64 {
	return r.sum / float64(r.count)
}

// perfect relies on no term scoring more than 1.0. An accepted result without
// positive terms is perfect too: a scalar OR beneath an AND has operands with
// equal counts, so the other operand cannot score higher.
func (r evalResult) perfect() bool {
	return r.sum >= float64(r.count)
}

// evaluator scores a subtree whose best accepting path its parent can choose
// by mean alone: every accepting path has the same positive-term count, or
// only ORs enclose the subtree.
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
	return evalResult{accepted: true, sum: score, count: 1}
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
	return evalResult{accepted: true, sum: left.sum + right.sum, count: left.count + right.count}
}

type orEvaluator struct {
	left  evaluator
	right evaluator
}

func (e *orEvaluator) evaluate(value string) evalResult {
	left := e.left.evaluate(value)
	if left.accepted && left.perfect() {
		return left
	}
	right := e.right.evaluate(value)
	if !right.accepted || (left.accepted && left.mean() > right.mean()) {
		return left
	}
	return right
}

// scoreAlternatives retains the highest sum for each attainable positive-term
// count. Bit n of counts marks sums[n] as valid; other sums are stale. Bit
// zero is an accepting path without positive terms, and no bits means
// rejection.
type scoreAlternatives struct {
	counts uint64
	sums   [maxExpressionTerms + 1]float64
}

// This fails to compile if an attainable count cannot fit in the counts bitmask.
const _ uint64 = 1 << maxExpressionTerms

func (r *scoreAlternatives) add(count int, sum float64) {
	bit := uint64(1) << count
	if r.counts&bit == 0 || sum > r.sums[count] {
		r.sums[count] = sum
		r.counts |= bit
	}
}

// merge adds the paths of an OR operand.
func (r *scoreAlternatives) merge(other *scoreAlternatives) {
	for counts := other.counts; counts != 0; counts &= counts - 1 {
		count := bits.TrailingZeros64(counts)
		r.add(count, other.sums[count])
	}
}

// shift adds a fixed-count AND operand to every path.
func (r *scoreAlternatives) shift(count int, sum float64) {
	// Move the highest counts first, so no sum is overwritten before it moves.
	for counts := r.counts; counts != 0; {
		from := bits.Len64(counts) - 1
		counts &^= 1 << from
		r.sums[from+count] = r.sums[from] + sum
	}
	r.counts <<= count
}

// combine pairs every path with every path of another AND operand. For any
// resulting count, smaller sums cannot beat the largest after adding
// surrounding terms.
func (r *scoreAlternatives) combine(other *scoreAlternatives) {
	pending := r.counts
	r.counts = 0
	// Pairs write at or above the count being read. Reading the highest counts
	// first therefore reads every original sum before a pair overwrites it.
	for pending != 0 {
		count := bits.Len64(pending) - 1
		pending &^= 1 << count
		sum := r.sums[count]
		for counts := other.counts; counts != 0; counts &= counts - 1 {
			otherCount := bits.TrailingZeros64(counts)
			r.add(count+otherCount, sum+other.sums[otherCount])
		}
	}
}

// alternativesNode scores a subtree beneath an AND whose accepting paths can
// have different positive-term counts. Its parent needs every count, because
// surrounding terms decide which count gives the best final mean. A concrete
// type keeps the calls static, so scratch alternatives stay on the stack.
type alternativesNode struct {
	// fixed is set for a fixed-count operand. Otherwise left and right are the
	// operands of an AND, if conjunction is true, or of an OR.
	fixed       evaluator
	conjunction bool
	left        *alternativesNode
	right       *alternativesNode
}

// evaluate replaces out with the subtree's paths.
func (n *alternativesNode) evaluate(value string, out *scoreAlternatives) {
	switch {
	case n.fixed != nil:
		out.counts = 0
		n.addTo(value, out)
	case n.conjunction:
		n.left.evaluate(value, out)
		if out.counts != 0 {
			n.right.pairWith(value, out)
		}
	default:
		n.left.evaluate(value, out)
		n.right.addTo(value, out)
	}
}

// addTo adds the subtree's paths to out, as an OR does.
func (n *alternativesNode) addTo(value string, out *scoreAlternatives) {
	switch {
	case n.fixed != nil:
		if result := n.fixed.evaluate(value); result.accepted {
			out.add(result.count, result.sum)
		}
	case n.conjunction:
		var paths scoreAlternatives
		n.evaluate(value, &paths)
		out.merge(&paths)
	default:
		n.left.addTo(value, out)
		n.right.addTo(value, out)
	}
}

// pairWith pairs out's paths with the subtree's paths, as an AND does. out
// must not be empty.
func (n *alternativesNode) pairWith(value string, out *scoreAlternatives) {
	switch {
	case n.fixed != nil:
		result := n.fixed.evaluate(value)
		if !result.accepted {
			out.counts = 0
			return
		}
		out.shift(result.count, result.sum)
	case n.conjunction:
		// Pairing is associative, so pair out with each operand in turn.
		n.left.pairWith(value, out)
		if out.counts != 0 {
			n.right.pairWith(value, out)
		}
	default:
		var paths scoreAlternatives
		n.evaluate(value, &paths)
		out.combine(&paths)
	}
}

// bestAlternative scores a count-varying AND that only ORs enclose. Those ORs
// compare means, so a single term with the best mean stands in for its paths.
type bestAlternative struct {
	and *alternativesNode
}

func (e *bestAlternative) evaluate(value string) evalResult {
	var paths scoreAlternatives
	e.and.evaluate(value, &paths)
	// Validate requires a positive term on each of this AND's paths, so count
	// zero is never present.
	var best float64
	for counts := paths.counts; counts != 0; counts &= counts - 1 {
		count := bits.TrailingZeros64(counts)
		best = max(best, paths.sums[count]/float64(count))
	}
	return evalResult{accepted: paths.counts != 0, sum: best, count: 1}
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

// compiledNode is a compiled subtree with the range of positive-term counts
// on its accepting paths. It is conservative: some paths may never accept.
// Exactly one evaluator is set; alternatives is set only for a count-varying
// subtree beneath an AND.
type compiledNode struct {
	scalar       evaluator
	alternatives *alternativesNode
	minCount     int
	maxCount     int
}

func (n compiledNode) asAlternatives() *alternativesNode {
	if n.alternatives != nil {
		return n.alternatives
	}
	return &alternativesNode{fixed: n.scalar}
}

// compileEvaluator expects an expression that Validate accepts. inAnd is true
// beneath an effective AND, after the De Morgan rewrite.
func compileEvaluator(expr Expr, negated, inAnd bool, newTermFilter TermFilterFactory) (compiledNode, error) {
	switch expr := expr.(type) {
	case Term:
		filter, err := newTermFilter(expr.Value, negated)
		if err != nil {
			return compiledNode{}, fmt.Errorf("search expression: compile term %q: %w", expr.Value, err)
		}
		if filter == nil {
			return compiledNode{}, fmt.Errorf("search expression: term filter factory returned nil for %q", expr.Value)
		}
		count := 1
		if negated {
			count = 0
		}
		return compiledNode{scalar: &termEvaluator{filter: filter, negated: negated}, minCount: count, maxCount: count}, nil

	case And:
		if negated {
			return compileDisjunction(expr.Left, expr.Right, negated, inAnd, newTermFilter)
		}
		return compileConjunction(expr.Left, expr.Right, negated, inAnd, newTermFilter)

	case Or:
		if negated {
			return compileConjunction(expr.Left, expr.Right, negated, inAnd, newTermFilter)
		}
		return compileDisjunction(expr.Left, expr.Right, negated, inAnd, newTermFilter)

	case Not:
		return compileEvaluator(expr.Expr, !negated, inAnd, newTermFilter)

	default:
		return compiledNode{}, fmt.Errorf("search expression: cannot compile node %T", expr)
	}
}

func compileConjunction(leftExpr, rightExpr Expr, negated, inAnd bool, newTermFilter TermFilterFactory) (compiledNode, error) {
	left, right, err := compileChildren(leftExpr, rightExpr, negated, true, newTermFilter)
	if err != nil {
		return compiledNode{}, err
	}
	node := compiledNode{minCount: left.minCount + right.minCount, maxCount: left.maxCount + right.maxCount}
	if node.minCount == node.maxCount {
		// Every path has the same count, so the highest sum has the best mean.
		node.scalar = &andEvaluator{left: left.scalar, right: right.scalar}
		return node, nil
	}
	and := &alternativesNode{conjunction: true, left: left.asAlternatives(), right: right.asAlternatives()}
	if inAnd {
		node.alternatives = and
		return node, nil
	}
	// The topmost AND reduces its paths to a mean for the ORs that enclose it.
	node.scalar = &bestAlternative{and: and}
	return node, nil
}

func compileDisjunction(leftExpr, rightExpr Expr, negated, inAnd bool, newTermFilter TermFilterFactory) (compiledNode, error) {
	left, right, err := compileChildren(leftExpr, rightExpr, negated, inAnd, newTermFilter)
	if err != nil {
		return compiledNode{}, err
	}
	node := compiledNode{minCount: min(left.minCount, right.minCount), maxCount: max(left.maxCount, right.maxCount)}
	// Without an enclosing AND, the operand with the higher mean is the best
	// path. Beneath one, operands with equal counts compare the same way.
	if !inAnd || node.minCount == node.maxCount {
		node.scalar = &orEvaluator{left: left.scalar, right: right.scalar}
		return node, nil
	}
	node.alternatives = &alternativesNode{left: left.asAlternatives(), right: right.asAlternatives()}
	return node, nil
}

func compileChildren(left, right Expr, negated, inAnd bool, newTermFilter TermFilterFactory) (compiledNode, compiledNode, error) {
	return mapChildren(left, right, func(expr Expr) (compiledNode, error) {
		return compileEvaluator(expr, negated, inAnd, newTermFilter)
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
