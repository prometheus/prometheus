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
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"time"

	"code.google.com/p/gorest"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
)

func (serv MetricsService) setAccessControlHeaders(rb *gorest.ResponseBuilder) {
	rb.AddHeader("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin")
	rb.AddHeader("Access-Control-Allow-Methods", "GET")
	rb.AddHeader("Access-Control-Allow-Origin", "*")
	rb.AddHeader("Access-Control-Expose-Headers", "Date")
}

func (serv MetricsService) Query(expr string, asText string) string {
	exprNode, err := rules.LoadExprFromString(expr)
	if err != nil {
		return ast.ErrorToJSON(err)
	}

	timestamp := serv.time.Now()

	rb := serv.ResponseBuilder()
	serv.setAccessControlHeaders(rb)
	var format ast.OutputFormat
	// BUG(julius): Use Content-Type negotiation.
	if asText == "" {
		format = ast.JSON
		rb.SetContentType(gorest.Application_Json)
	} else {
		format = ast.TEXT
		rb.SetContentType(gorest.Text_Plain)
	}

	queryStats := stats.NewTimerGroup()
	result := ast.EvalToString(exprNode, timestamp, format, serv.Storage, queryStats)
	log.Printf("Instant query: %s\nQuery stats:\n%s\n", expr, queryStats)
	return result
}

func (serv MetricsService) QueryRange(expr string, end int64, duration int64, step int64) string {
	exprNode, err := rules.LoadExprFromString(expr)
	if err != nil {
		return ast.ErrorToJSON(err)
	}
	if exprNode.Type() != ast.VECTOR {
		return ast.ErrorToJSON(errors.New("Expression does not evaluate to vector type"))
	}
	rb := serv.ResponseBuilder()
	serv.setAccessControlHeaders(rb)
	rb.SetContentType(gorest.Application_Json)

	if end == 0 {
		end = serv.time.Now().Unix()
	}

	if step < 1 {
		step = 1
	}

	if end-duration < 0 {
		duration = end
	}

	// Align the start to step "tick" boundary.
	end -= end % step

	queryStats := stats.NewTimerGroup()

	evalTimer := queryStats.GetTimer(stats.TotalEvalTime).Start()
	matrix, err := ast.EvalVectorRange(
		exprNode.(ast.VectorNode),
		time.Unix(end-duration, 0).UTC(),
		time.Unix(end, 0).UTC(),
		time.Duration(step)*time.Second,
		serv.Storage,
		queryStats)
	if err != nil {
		return ast.ErrorToJSON(err)
	}
	evalTimer.Stop()

	sortTimer := queryStats.GetTimer(stats.ResultSortTime).Start()
	sort.Sort(matrix)
	sortTimer.Stop()

	jsonTimer := queryStats.GetTimer(stats.JsonEncodeTime).Start()
	result := ast.TypedValueToJSON(matrix, "matrix")
	jsonTimer.Stop()

	log.Printf("Range query: %s\nQuery stats:\n%s\n", expr, queryStats)
	return result
}

func (serv MetricsService) Metrics() string {
	metricNames, err := serv.Storage.GetAllValuesForLabel(clientmodel.MetricNameLabel)
	rb := serv.ResponseBuilder()
	serv.setAccessControlHeaders(rb)
	rb.SetContentType(gorest.Application_Json)
	if err != nil {
		log.Printf("Error loading metric names: %v", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	sort.Sort(metricNames)
	resultBytes, err := json.Marshal(metricNames)
	if err != nil {
		log.Printf("Error marshalling metric names: %v", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}
