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

package api

import (
	"code.google.com/p/gorest"
	"encoding/json"
	"errors"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"log"
	"net/http"
	"sort"
	"time"
)

func (serv MetricsService) Query(Expr string, Json string) (result string) {
	exprNode, err := rules.LoadExprFromString(Expr)
	if err != nil {
		return ast.ErrorToJSON(err)
	}

	timestamp := serv.time.Now()

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
		End = serv.time.Now().Unix()
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

func (serv MetricsService) Metrics() string {
	metricNames, err := serv.appState.Persistence.GetAllMetricNames()
	rb := serv.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)
	if err != nil {
		log.Printf("Error loading metric names: %v", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	sort.Strings(metricNames)
	resultBytes, err := json.Marshal(metricNames)
	if err != nil {
		log.Printf("Error marshalling metric names: %v", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}
