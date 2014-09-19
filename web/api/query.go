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
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/web/http_utils"
)

// Enables cross-site script calls.
func setAccessControlHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Date")
}

func (serv MetricsService) Query(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)

	params := http_utils.GetQueryParams(r)
	expr := params.Get("expr")
	asText := params.Get("asText")

	var format ast.OutputFormat
	// BUG(julius): Use Content-Type negotiation.
	if asText == "" {
		format = ast.JSON
		w.Header().Set("Content-Type", "application/json")
	} else {
		format = ast.TEXT
		w.Header().Set("Content-Type", "text/plain")
	}

	exprNode, err := rules.LoadExprFromString(expr)
	if err != nil {
		fmt.Fprint(w, ast.ErrorToJSON(err))
		return
	}

	timestamp := clientmodel.TimestampFromTime(serv.time.Now())

	queryStats := stats.NewTimerGroup()
	result := ast.EvalToString(exprNode, timestamp, format, serv.Storage, queryStats)
	glog.V(1).Infof("Instant query: %s\nQuery stats:\n%s\n", expr, queryStats)
	fmt.Fprint(w, result)
}

func (serv MetricsService) QueryRange(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	params := http_utils.GetQueryParams(r)
	expr := params.Get("expr")
	end, _ := strconv.ParseInt(params.Get("end"), 0, 64)
	duration, _ := strconv.ParseInt(params.Get("range"), 0, 64)
	step, _ := strconv.ParseInt(params.Get("step"), 0, 64)

	exprNode, err := rules.LoadExprFromString(expr)
	if err != nil {
		fmt.Fprint(w, ast.ErrorToJSON(err))
		return
	}
	if exprNode.Type() != ast.VECTOR {
		fmt.Fprint(w, ast.ErrorToJSON(errors.New("Expression does not evaluate to vector type")))
		return
	}

	if end == 0 {
		end = clientmodel.Now().Unix()
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
		clientmodel.TimestampFromUnix(end-duration),
		clientmodel.TimestampFromUnix(end),
		time.Duration(step)*time.Second,
		serv.Storage,
		queryStats)
	if err != nil {
		fmt.Fprint(w, ast.ErrorToJSON(err))
		return
	}
	evalTimer.Stop()

	sortTimer := queryStats.GetTimer(stats.ResultSortTime).Start()
	sort.Sort(matrix)
	sortTimer.Stop()

	jsonTimer := queryStats.GetTimer(stats.JsonEncodeTime).Start()
	result := ast.TypedValueToJSON(matrix, "matrix")
	jsonTimer.Stop()

	glog.V(1).Infof("Range query: %s\nQuery stats:\n%s\n", expr, queryStats)
	fmt.Fprint(w, result)
}

func (serv MetricsService) Metrics(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)

	metricNames := serv.Storage.GetLabelValuesForLabelName(clientmodel.MetricNameLabel)
	sort.Sort(metricNames)
	resultBytes, err := json.Marshal(metricNames)
	if err != nil {
		glog.Error("Error marshalling metric names: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(resultBytes)
}
