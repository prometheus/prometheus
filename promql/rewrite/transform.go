// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rewrite

import (
	"fmt"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type Transform interface {
	Transform(parser.Expr) (parser.Expr, error)
}

type TransformFunc func(parser.Expr) (parser.Expr, error)

func (t TransformFunc) Transform(node parser.Expr) (parser.Expr, error) {
	return t(node)
}

// Rewrite a node by applying transform recursively.
func Rewrite(node parser.Expr, transform Transform) (parser.Expr, error) {
	var err error
	node, err = transform.Transform(node)
	if err != nil {
		return nil, err
	}
	switch n := node.(type) {
	case *parser.Call:
		for i := range n.Args {
			n.Args[i], err = Rewrite(n.Args[i], transform)
			if err != nil {
				return nil, err
			}
		}
	case *parser.StepInvariantExpr:
		n.Expr, err = Rewrite(n.Expr, transform)
	case *parser.SubqueryExpr:
		n.Expr, err = Rewrite(n.Expr, transform)
	case *parser.MatrixSelector:
		n.VectorSelector, err = Rewrite(n.VectorSelector, transform)
	case *parser.UnaryExpr:
		n.Expr, err = Rewrite(n.Expr, transform)
	case *parser.AggregateExpr:
		n.Expr, err = Rewrite(n.Expr, transform)
	case *parser.BinaryExpr:
		n.LHS, err = Rewrite(n.LHS, transform)
		if err != nil {
			return nil, err
		}
		n.RHS, err = Rewrite(n.RHS, transform)
		if err != nil {
			return nil, err
		}
	case *parser.ParenExpr:
		n.Expr, err = Rewrite(n.Expr, transform)
	}
	if err != nil {
		return nil, err
	}
	return node, nil
}

type List []Transform

func (l List) Transform(node parser.Expr) (parser.Expr, error) {
	for _, t := range l {
		newNode, err := t.Transform(node)
		if err != nil {
			return nil, err
		}
		// If this is a newNode, we apply it and return.
		if node != newNode {
			return newNode, nil
		}
	}
	return node, nil
}

func cloneExpr(n parser.Expr) parser.Expr {
	switch n := n.(type) {
	case *parser.VectorSelector:
		matchers := make([]*labels.Matcher, len(n.LabelMatchers))
		copy(matchers, n.LabelMatchers)
		return &parser.VectorSelector{
			Name:                    n.Name,
			OriginalOffset:          n.OriginalOffset,
			OriginalOffsetExpr:      n.OriginalOffsetExpr,
			Offset:                  n.Offset,
			Timestamp:               n.Timestamp,
			SkipHistogramBuckets:    n.SkipHistogramBuckets,
			StartOrEnd:              n.StartOrEnd,
			LabelMatchers:           matchers,
			UnexpandedSeriesSet:     n.UnexpandedSeriesSet,
			Series:                  n.Series,
			BypassEmptyMatcherCheck: n.BypassEmptyMatcherCheck,
			Anchored:                n.Anchored,
			Smoothed:                n.Smoothed,
		}
	default:
		panic(fmt.Sprintf("cannot clone %T", n))
	}
}

func renameVectorSelector(vs *parser.VectorSelector, newName string) {
	vs.Name = newName
	for i := len(vs.LabelMatchers) - 1; i >= 0; i-- {
		if vs.LabelMatchers[i].Name == model.MetricNameLabel {
			vs.LabelMatchers = append(vs.LabelMatchers[:i], vs.LabelMatchers[i+1:]...)
			return
		}
	}
}

func removeLabelMatchers(vs *parser.VectorSelector, name string) {
	for i := len(vs.LabelMatchers) - 1; i >= 0; i-- {
		if vs.LabelMatchers[i].Name == name {
			vs.LabelMatchers = append(vs.LabelMatchers[:i], vs.LabelMatchers[i+1:]...)
		}
	}
}

func removeItem[T comparable](arr []T, el T) []T {
	out := make([]T, 0, len(arr))
	for _, e := range arr {
		if e != el {
			out = append(out, e)
		}
	}
	return out
}
