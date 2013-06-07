// Copyright 2013 Prometheus Team
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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/utility"
)

func CreateRecordingRule(name string, labels model.LabelSet, expr ast.Node, permanent bool) (*RecordingRule, error) {
	if _, ok := expr.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("Recording rule expression %v does not evaluate to vector type", expr)
	}
	return NewRecordingRule(name, labels, expr.(ast.VectorNode), permanent), nil
}

func CreateAlertingRule(name string, expr ast.Node, holdDurationStr string, labels model.LabelSet) (*AlertingRule, error) {
	if _, ok := expr.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("Alert rule expression %v does not evaluate to vector type", expr)
	}
	holdDuration, err := utility.StringToDuration(holdDurationStr)
	if err != nil {
		return nil, err
	}
	return NewAlertingRule(name, expr.(ast.VectorNode), holdDuration, labels), nil
}

func NewFunctionCall(name string, args []ast.Node) (ast.Node, error) {
	function, err := ast.GetFunction(name)
	if err != nil {
		return nil, fmt.Errorf("Unknown function \"%v\"", name)
	}
	functionCall, err := ast.NewFunctionCall(function, args)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	return functionCall, nil
}

func NewVectorAggregation(aggrTypeStr string, vector ast.Node, groupBy model.LabelNames) (*ast.VectorAggregation, error) {
	if _, ok := vector.(ast.VectorNode); !ok {
		return nil, fmt.Errorf("Operand of %v aggregation must be of vector type", aggrTypeStr)
	}
	var aggrTypes = map[string]ast.AggrType{
		"SUM":   ast.SUM,
		"MAX":   ast.MAX,
		"MIN":   ast.MIN,
		"AVG":   ast.AVG,
		"COUNT": ast.COUNT,
	}
	aggrType, ok := aggrTypes[aggrTypeStr]
	if !ok {
		return nil, fmt.Errorf("Unknown aggregation type '%v'", aggrTypeStr)
	}
	return ast.NewVectorAggregation(aggrType, vector.(ast.VectorNode), groupBy), nil
}

func NewArithExpr(opTypeStr string, lhs ast.Node, rhs ast.Node) (ast.Node, error) {
	var opTypes = map[string]ast.BinOpType{
		"+":   ast.ADD,
		"-":   ast.SUB,
		"*":   ast.MUL,
		"/":   ast.DIV,
		"%":   ast.MOD,
		">":   ast.GT,
		"<":   ast.LT,
		"==":  ast.EQ,
		"!=":  ast.NE,
		">=":  ast.GE,
		"<=":  ast.LE,
		"AND": ast.AND,
		"OR":  ast.OR,
	}
	opType, ok := opTypes[opTypeStr]
	if !ok {
		return nil, fmt.Errorf("Invalid binary operator \"%v\"", opTypeStr)
	}
	expr, err := ast.NewArithExpr(opType, lhs, rhs)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	return expr, nil
}

func NewMatrix(vector ast.Node, intervalStr string) (ast.MatrixNode, error) {
	switch vector.(type) {
	case *ast.VectorLiteral:
		{
			break
		}
	default:
		return nil, fmt.Errorf("Intervals are currently only supported for vector literals.")
	}
	interval, err := utility.StringToDuration(intervalStr)
	if err != nil {
		return nil, err
	}
	vectorLiteral := vector.(*ast.VectorLiteral)
	return ast.NewMatrixLiteral(vectorLiteral, interval), nil
}
