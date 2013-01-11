package api

import (
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

        format := ast.TEXT
        if Json != "" {
                format = ast.JSON
        }
        return ast.EvalToString(exprNode, &timestamp, format)
}
