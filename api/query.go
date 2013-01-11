package api

import (
        "code.google.com/p/gorest"
	"github.com/matttproud/prometheus/rules"
	"github.com/matttproud/prometheus/rules/ast"
        "time"
)
func (serv MetricsService) Query(Expr string, Json string, Start string, End string) (result string) {
        exprNode, err := rules.LoadExprFromString(Expr)
        if err != nil {
                return err.Error()
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
