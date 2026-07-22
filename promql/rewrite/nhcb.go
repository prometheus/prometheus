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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
)

const (
	bucketSuffix = "_bucket"
	sumSuffix    = "_sum"
	countSuffix  = "_count"
)

var (
	histogramAvg      = parser.MustGetFunction("histogram_avg")
	histogramQuantile = parser.MustGetFunction("histogram_quantile")
	histogramCount    = parser.MustGetFunction("histogram_count")
	histogramFraction = parser.MustGetFunction("histogram_fraction")
	histogramSum      = parser.MustGetFunction("histogram_sum")
)

func TransformClassicHistograms(histograms []string) Transform {
	return List{
		// histogram_quantile(X, sum by (le, W) (rate(Y_bucket[Z]))) -> histogram_quantile(X, sum by (W) (rate(Y[Z])))
		TransformFunc(func(expr parser.Expr) (parser.Expr, error) {
			call, ok := expr.(*parser.Call)
			if !ok {
				return expr, nil
			}
			if call.Func != histogramQuantile {
				return expr, nil
			}
			if len(call.Args) != 2 {
				return expr, nil
			}
			sumAgg, ok := call.Args[1].(*parser.AggregateExpr)
			if !ok {
				return expr, nil
			}
			if !slices.Contains(sumAgg.Grouping, labels.BucketLabel) {
				return expr, nil
			}
			innerCall, ok := sumAgg.Expr.(*parser.Call)
			if !ok {
				return expr, nil
			}
			switch innerCall.Func.Name {
			case "delta", "idelta", "increase", "irate", "rate":
			default:
				return expr, fmt.Errorf("inner function %q cannot be converted", innerCall.Func.Name)
			}
			if len(innerCall.Args) != 1 {
				return expr, nil
			}
			ms, ok := innerCall.Args[0].(*parser.MatrixSelector)
			if !ok {
				return expr, nil
			}
			vs, ok := ms.VectorSelector.(*parser.VectorSelector)
			if !ok {
				return expr, nil
			}
			if !strings.HasSuffix(vs.Name, bucketSuffix) {
				return expr, nil
			}
			newName := histogramBaseName(vs.Name)
			if !slices.Contains(histograms, newName) {
				return expr, nil
			}
			newVs := cloneExpr(vs).(*parser.VectorSelector)
			renameVectorSelector(newVs, newName)

			return &parser.Call{
				Func: call.Func,
				Args: parser.Expressions{
					call.Args[0],
					&parser.AggregateExpr{
						Op: sumAgg.Op,
						Expr: &parser.Call{
							Func: innerCall.Func,
							Args: parser.Expressions{
								&parser.MatrixSelector{
									VectorSelector: newVs,
									Range:          ms.Range,
									RangeExpr:      ms.RangeExpr,
									EndPos:         ms.EndPos,
								},
							},
						},
						Param:    sumAgg.Param,
						Grouping: removeItem(sumAgg.Grouping, labels.BucketLabel),
						Without:  sumAgg.Without,
					},
				},
			}, nil
		}),
		// X_bucket{le="Y"} -> histogram_count(X) * histogram_fraction(-Inf, Y, X)
		TransformFunc(func(expr parser.Expr) (parser.Expr, error) {
			vs, ok := expr.(*parser.VectorSelector)
			if !ok {
				return expr, nil
			}
			if !strings.HasSuffix(vs.Name, bucketSuffix) {
				return expr, nil
			}
			newName := histogramBaseName(vs.Name)
			if !slices.Contains(histograms, newName) {
				return expr, nil
			}
			// Ensure matching on LE label.
			le := -1.0
			for _, l := range vs.LabelMatchers {
				if l.Name == labels.BucketLabel {
					parsed, err := strconv.ParseFloat(l.Value, 64)
					if err != nil {
						return expr, fmt.Errorf("invalid le label value: %s", l.Value)
					}
					le = parsed
					break
				}
			}
			if le == -1 {
				return expr, nil
			}

			newVs := cloneExpr(vs).(*parser.VectorSelector)
			renameVectorSelector(newVs, newName)
			removeLabelMatchers(newVs, labels.BucketLabel)

			lhs := &parser.Call{
				Func: histogramCount,
				Args: parser.Expressions{newVs},
			}
			rhs := &parser.Call{
				Func: histogramFraction,
				Args: parser.Expressions{
					&parser.NumberLiteral{
						Val: math.Inf(-1),
					},
					&parser.NumberLiteral{
						Val: le,
					},
					newVs,
				},
			}
			return &parser.BinaryExpr{
				Op:  parser.MUL,
				LHS: lhs,
				RHS: rhs,
			}, nil
		}),
		// X_sum / X_count -> histogram_avg(X)
		TransformFunc(func(node parser.Expr) (parser.Expr, error) {
			bin, ok := node.(*parser.BinaryExpr)
			if !ok {
				return node, nil
			}
			if bin.Op != parser.DIV {
				return node, nil
			}
			left, ok := bin.LHS.(*parser.VectorSelector)
			if !ok {
				return node, nil
			}
			right, ok := bin.RHS.(*parser.VectorSelector)
			if !ok {
				return node, nil
			}
			if selectorLabelsHash(left) != selectorLabelsHash(right) {
				return node, nil
			}
			leftName, rightName := histogramBaseName(left.Name), histogramBaseName(right.Name)
			if leftName != rightName {
				return node, nil
			}
			if !slices.Contains(histograms, leftName) {
				return node, nil
			}
			newVs := cloneExpr(left).(*parser.VectorSelector)
			renameVectorSelector(newVs, leftName)
			return &parser.Call{
				Func:     histogramAvg,
				Args:     parser.Expressions{newVs},
				PosRange: posrange.PositionRange{},
			}, nil
		}),
		// X_count -> histogram_count(X)
		TransformFunc(func(expr parser.Expr) (parser.Expr, error) {
			vs, ok := expr.(*parser.VectorSelector)
			if !ok {
				return expr, nil
			}
			if !strings.HasSuffix(vs.Name, countSuffix) {
				return expr, nil
			}
			baseName := histogramBaseName(vs.Name)
			if !slices.Contains(histograms, baseName) {
				return expr, nil
			}
			newVs := cloneExpr(vs).(*parser.VectorSelector)
			renameVectorSelector(newVs, histogramBaseName(vs.Name))
			return &parser.Call{
				Func: histogramCount,
				Args: parser.Expressions{newVs},
			}, nil
		}),
		// X_sum -> histogram_sum(X)
		TransformFunc(func(expr parser.Expr) (parser.Expr, error) {
			vs, ok := expr.(*parser.VectorSelector)
			if !ok {
				return expr, nil
			}
			if !strings.HasSuffix(vs.Name, sumSuffix) {
				return expr, nil
			}
			baseName := histogramBaseName(vs.Name)
			if !slices.Contains(histograms, baseName) {
				return expr, nil
			}
			newVs := cloneExpr(vs).(*parser.VectorSelector)
			renameVectorSelector(newVs, histogramBaseName(vs.Name))
			return &parser.Call{
				Func: histogramSum,
				Args: parser.Expressions{newVs},
			}, nil
		}),
	}
}

func selectorLabelsHash(vs *parser.VectorSelector) string {
	h := md5.New()
	for _, l := range vs.LabelMatchers {
		// Skip reserved labels:
		if strings.HasPrefix(l.Name, "__") && strings.HasSuffix(l.Name, "__") {
			continue
		}
		h.Write([]byte(l.Name + ":" + l.Value + ","))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func histogramBaseName(s string) string {
	for _, suffix := range []string{countSuffix, sumSuffix, bucketSuffix} {
		if strings.HasSuffix(s, suffix) {
			return s[:len(s)-len(suffix)]
		}
	}
	return s
}
