package api

import (
	"code.google.com/p/gorest"
	"errors"
	"github.com/matttproud/prometheus/rules"
	"github.com/matttproud/prometheus/rules/ast"
	"sort"
	"time"
)

func (serv MetricsService) Query(Expr string, Json string) (result string) {
	exprNode, err := rules.LoadExprFromString(Expr)
	if err != nil {
		return ast.ErrorToJSON(err)
	}

	timestamp := time.Now()

	rb := serv.ResponseBuilder()
	var format ast.OutputFormat
	if Json != "" {
		format = ast.JSON
		rb.SetContentType(gorest.Application_Json)
	} else {
		format = ast.TEXT
		rb.SetContentType(gorest.Text_Plain)
	}

	return ast.EvalToString(exprNode, &timestamp, format)
}

func (serv MetricsService) QueryRange(Expr string, End int64, Range int64, Step int64) string {
	exprNode, err := rules.LoadExprFromString(Expr)
	if err != nil {
		return ast.ErrorToJSON(err)
	}
	if exprNode.Type() != ast.VECTOR {
		return ast.ErrorToJSON(errors.New("Expression does not evaluate to vector type"))
	}
	rb := serv.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)

	if End == 0 {
		End = time.Now().Unix()
	}

	if Step < 1 {
		Step = 1
	}

	if End-Range < 0 {
		Range = End
	}

	// Align the start to step "tick" boundary.
	End -= End % Step

	matrix := ast.EvalVectorRange(
		exprNode.(ast.VectorNode),
		time.Unix(End-Range, 0),
		time.Unix(End, 0),
		time.Duration(Step)*time.Second)

	sort.Sort(matrix)
	return ast.TypedValueToJSON(matrix, "matrix")
}
