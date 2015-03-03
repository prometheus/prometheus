// Copyright 2013 The Prometheus Authors
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

package rules

import (
	"fmt"
	"net/url"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

// CreateRecordingRule is a convenience function to create a recording rule.
func CreateRecordingRule(name string, labels clientmodel.LabelSet, expr ast.Node, permanent bool) (*RecordingRule, error) {
	if _, ok := expr.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("recording rule expression %v does not evaluate to vector type", expr)
	}
	return &RecordingRule{
		name:      name,
		labels:    labels,
		vector:    expr.(ast.VectorNode),
		permanent: permanent,
	}, nil
}

// CreateAlertingRule is a convenience function to create a new alerting rule.
func CreateAlertingRule(name string, expr ast.Node, holdDurationStr string, labels clientmodel.LabelSet, summary string, description string) (*AlertingRule, error) {
	if _, ok := expr.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("alert rule expression %v does not evaluate to vector type", expr)
	}
	holdDuration, err := utility.StringToDuration(holdDurationStr)
	if err != nil {
		return nil, err
	}
	return NewAlertingRule(name, expr.(ast.VectorNode), holdDuration, labels, summary, description), nil
}

// NewFunctionCall is a convenience function to create a new AST function-call node.
func NewFunctionCall(name string, args []ast.Node) (ast.Node, error) {
	function, err := ast.GetFunction(name)
	if err != nil {
		return nil, fmt.Errorf("unknown function %q", name)
	}
	functionCall, err := ast.NewFunctionCall(function, args)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	return functionCall, nil
}

// NewVectorAggregation is a convenience function to create a new AST vector aggregation.
func NewVectorAggregation(aggrTypeStr string, vector ast.Node, groupBy clientmodel.LabelNames, keepExtraLabels bool) (*ast.VectorAggregation, error) {
	if _, ok := vector.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("operand of %v aggregation must be of vector type", aggrTypeStr)
	}
	var aggrTypes = map[string]ast.AggrType{
		"SUM":   ast.Sum,
		"MAX":   ast.Max,
		"MIN":   ast.Min,
		"AVG":   ast.Avg,
		"COUNT": ast.Count,
	}
	aggrType, ok := aggrTypes[aggrTypeStr]
	if !ok {
		return nil, fmt.Errorf("unknown aggregation type %q", aggrTypeStr)
	}
	return ast.NewVectorAggregation(aggrType, vector.(ast.VectorNode), groupBy, keepExtraLabels), nil
}

// vectorMatching combines data used to match samples between vectors.
type vectorMatching struct {
	matchCardinality ast.VectorMatchCardinality
	matchOn          clientmodel.LabelNames
	includeLabels    clientmodel.LabelNames
}

// newVectorMatching is a convenience function to create a new vectorMatching.
func newVectorMatching(card string, matchOn, include clientmodel.LabelNames) (*vectorMatching, error) {
	var matchCardinalities = map[string]ast.VectorMatchCardinality{
		"":            ast.MatchOneToOne,
		"GROUP_LEFT":  ast.MatchManyToOne,
		"GROUP_RIGHT": ast.MatchOneToMany,
	}
	matchCard, ok := matchCardinalities[card]
	if !ok {
		return nil, fmt.Errorf("invalid vector match cardinality %q", card)
	}
	if matchCard != ast.MatchOneToOne && len(include) == 0 {
		return nil, fmt.Errorf("grouped vector matching must provide labels")
	}
	// There must be no overlap between both labelname lists.
	for _, matchLabel := range matchOn {
		for _, incLabel := range include {
			if matchLabel == incLabel {
				return nil, fmt.Errorf("use of label %s in ON and %s clauses not allowed", incLabel, card)
			}
		}
	}
	return &vectorMatching{matchCard, matchOn, include}, nil
}

// NewArithExpr is a convenience function to create a new AST arithmetic expression.
func NewArithExpr(opTypeStr string, lhs ast.Node, rhs ast.Node, vecMatching *vectorMatching) (ast.Node, error) {
	var opTypes = map[string]ast.BinOpType{
		"+":   ast.Add,
		"-":   ast.Sub,
		"*":   ast.Mul,
		"/":   ast.Div,
		"%":   ast.Mod,
		">":   ast.GT,
		"<":   ast.LT,
		"==":  ast.EQ,
		"!=":  ast.NE,
		">=":  ast.GE,
		"<=":  ast.LE,
		"AND": ast.And,
		"OR":  ast.Or,
	}
	opType, ok := opTypes[opTypeStr]
	if !ok {
		return nil, fmt.Errorf("invalid binary operator %q", opTypeStr)
	}
	var vm vectorMatching
	if vecMatching != nil {
		vm = *vecMatching
		// And/or always do many-to-many matching.
		if opType == ast.And || opType == ast.Or {
			vm.matchCardinality = ast.MatchManyToMany
		}
	}
	expr, err := ast.NewArithExpr(opType, lhs, rhs, vm.matchCardinality, vm.matchOn, vm.includeLabels)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	return expr, nil
}

// NewVectorSelector is a convenience function to create a new AST vector selector.
func NewVectorSelector(m metric.LabelMatchers, offsetStr string) (ast.VectorNode, error) {
	offset, err := utility.StringToDuration(offsetStr)
	if err != nil {
		return nil, err
	}
	return ast.NewVectorSelector(m, offset), nil
}

// NewMatrixSelector is a convenience function to create a new AST matrix selector.
func NewMatrixSelector(vector ast.Node, intervalStr string, offsetStr string) (ast.MatrixNode, error) {
	interval, err := utility.StringToDuration(intervalStr)
	if err != nil {
		return nil, err
	}
	offset, err := utility.StringToDuration(offsetStr)
	if err != nil {
		return nil, err
	}
	vectorSelector, ok := vector.(*ast.VectorSelector)
	if !ok {
		return nil, fmt.Errorf("intervals are currently only supported for vector selectors")
	}
	return ast.NewMatrixSelector(vectorSelector, interval, offset), nil
}

func newLabelMatcher(matchTypeStr string, name clientmodel.LabelName, value clientmodel.LabelValue) (*metric.LabelMatcher, error) {
	matchTypes := map[string]metric.MatchType{
		"=":  metric.Equal,
		"!=": metric.NotEqual,
		"=~": metric.RegexMatch,
		"!~": metric.RegexNoMatch,
	}
	matchType, ok := matchTypes[matchTypeStr]
	if !ok {
		return nil, fmt.Errorf("invalid label matching operator %q", matchTypeStr)
	}
	return metric.NewLabelMatcher(matchType, name, value)
}

// TableLinkForExpression creates an escaped relative link to the table view of
// the provided expression.
func TableLinkForExpression(expr string) string {
	// url.QueryEscape percent-escapes everything except spaces, for which it
	// uses "+". However, in the non-query part of a URI, only percent-escaped
	// spaces are legal, so we need to manually replace "+" with "%20" after
	// query-escaping the string.
	//
	// See also:
	// http://stackoverflow.com/questions/1634271/url-encoding-the-space-character-or-20.
	urlData := url.QueryEscape(fmt.Sprintf(`[{"expr":%q,"tab":1}]`, expr))
	return fmt.Sprintf("/graph#%s", strings.Replace(urlData, "+", "%20", -1))
}

// GraphLinkForExpression creates an escaped relative link to the graph view of
// the provided expression.
func GraphLinkForExpression(expr string) string {
	urlData := url.QueryEscape(fmt.Sprintf(`[{"expr":%q,"tab":0}]`, expr))
	return fmt.Sprintf("/graph#%s", strings.Replace(urlData, "+", "%20", -1))
}
