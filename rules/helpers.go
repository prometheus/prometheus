package rules

import (
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/rules/ast"
	"regexp"
	"strconv"
	"time"
)

func rulesError(error string, v ...interface{}) error {
	return errors.New(fmt.Sprintf(error, v...))
}

// TODO move to common place, currently duplicated in config/
func stringToDuration(durationStr string) (time.Duration, error) {
	durationRE := regexp.MustCompile("^([0-9]+)([ywdhms]+)$")
	matches := durationRE.FindStringSubmatch(durationStr)
	if len(matches) != 3 {
		return 0, rulesError("Not a valid duration string: '%v'", durationStr)
	}
	value, _ := strconv.Atoi(matches[1])
	unit := matches[2]
	switch unit {
	case "y":
		value *= 60 * 60 * 24 * 365
	case "w":
		value *= 60 * 60 * 24
	case "d":
		value *= 60 * 60 * 24
	case "h":
		value *= 60 * 60
	case "m":
		value *= 60
	case "s":
		value *= 1
	}
	return time.Duration(value) * time.Second, nil
}

func CreateRule(name string, labels model.LabelSet, root ast.Node, permanent bool) (*Rule, error) {
	if root.Type() != ast.VECTOR {
		return nil, rulesError("Rule %v does not evaluate to vector type", name)
	}
	return NewRule(name, labels, root.(ast.VectorNode), permanent), nil
}

func NewFunctionCall(name string, args []ast.Node) (ast.Node, error) {
	function, err := ast.GetFunction(name)
	if err != nil {
		return nil, rulesError("Unknown function \"%v\"", name)
	}
	functionCall, err := ast.NewFunctionCall(function, args)
	if err != nil {
		return nil, rulesError(err.Error())
	}
	return functionCall, nil
}

func NewVectorAggregation(aggrTypeStr string, vector ast.Node, groupBy []model.LabelName) (*ast.VectorAggregation, error) {
	if vector.Type() != ast.VECTOR {
		return nil, rulesError("Operand of %v aggregation must be of vector type", aggrTypeStr)
	}
	var aggrTypes = map[string]ast.AggrType{
		"SUM": ast.SUM,
		"MAX": ast.MAX,
		"MIN": ast.MIN,
		"AVG": ast.AVG,
	}
	aggrType, ok := aggrTypes[aggrTypeStr]
	if !ok {
		return nil, rulesError("Unknown aggregation type '%v'", aggrTypeStr)
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
		return nil, rulesError("Invalid binary operator \"%v\"", opTypeStr)
	}
	expr, err := ast.NewArithExpr(opType, lhs, rhs)
	if err != nil {
		return nil, rulesError(err.Error())
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
		return nil, rulesError("Intervals are currently only supported for vector literals.")
	}
	interval, err := stringToDuration(intervalStr)
	if err != nil {
		return nil, err
	}
	vectorLiteral := vector.(*ast.VectorLiteral)
	return ast.NewMatrixLiteral(vectorLiteral, interval), nil
}
