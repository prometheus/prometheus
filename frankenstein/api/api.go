// Copyright 2016 The Prometheus Authors
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

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/httputil"
)

type status string

const (
	statusSuccess status = "success"
	statusError          = "error"
)

type errorType string

const (
	errorNone     errorType = ""
	errorTimeout            = "timeout"
	errorCanceled           = "canceled"
	errorExec               = "execution"
	errorBadData            = "bad_data"
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
}

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

type apiFunc func(ctx context.Context, r *http.Request) (interface{}, *apiError)

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	newQuerier func(ctx context.Context) local.Querier

	context func(r *http.Request) context.Context
	now     func() model.Time
}

// New returns an initialized API type.
func New(newQuerier func(ctx context.Context) local.Querier) *API {
	return &API{
		newQuerier: newQuerier,
		context:    route.Context,
		now:        model.Now,
	}
}

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	instr := func(name string, f apiFunc) http.HandlerFunc {
		hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Get user ID from header again once we have a proxy in front of
			// Frankenstein that creates the header:
			//
			// userID := r.Header.Get(userIDHeaderName)
			//
			// For now, getting the user ID from basic auth allows for easy testing
			// with Grafana.
			userID, _, _ := r.BasicAuth()
			if r.Method != "OPTIONS" && userID == "" {
				respondError(w, &apiError{errorBadData, fmt.Errorf("missing user ID")}, nil)
				return
			}
			ctx := context.WithValue(context.Background(), local.UserIDContextKey, userID) // TODO: replace string with constant
			setCORS(w)

			if data, err := f(ctx, r); err != nil {
				respondError(w, err, data)
			} else if data != nil {
				respond(w, data)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		})
		return prometheus.InstrumentHandler(name, httputil.CompressionHandler{
			Handler: hf,
		})
	}

	r.Options("/*path", instr("options", api.options))

	r.Get("/query", instr("query", api.query))
	r.Get("/query_range", instr("query_range", api.queryRange))

	r.Get("/label/:name/values", instr("label_values", api.labelValues))

	r.Get("/series", instr("series", api.series))
	r.Del("/series", instr("drop_series", api.dropSeries))
}

type queryData struct {
	ResultType model.ValueType `json:"resultType"`
	Result     model.Value     `json:"result"`
}

func (api *API) options(_ context.Context, r *http.Request) (interface{}, *apiError) {
	return nil, nil
}

func (api *API) query(ctx context.Context, r *http.Request) (interface{}, *apiError) {
	var ts model.Time
	if t := r.FormValue("time"); t != "" {
		var err error
		ts, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		ts = api.now()
	}

	qry, err := api.newEngine(ctx).NewInstantQuery(r.FormValue("query"), ts)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	res := qry.Exec()
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}
		}
		return nil, &apiError{errorExec, res.Err}
	}
	return &queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
	}, nil
}

func (api *API) queryRange(ctx context.Context, r *http.Request) (interface{}, *apiError) {
	start, err := parseTime(r.FormValue("start"))
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}
	end, err := parseTime(r.FormValue("end"))
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}
	step, err := parseDuration(r.FormValue("step"))
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return nil, &apiError{errorBadData, err}
	}

	qry, err := api.newEngine(ctx).NewRangeQuery(r.FormValue("query"), start, end, step)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	res := qry.Exec()
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}
		}
		return nil, &apiError{errorExec, res.Err}
	}
	return &queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
	}, nil
}

func (api *API) labelValues(ctx context.Context, r *http.Request) (interface{}, *apiError) {
	name := route.Param(api.context(r), "name")

	if !model.LabelNameRE.MatchString(name) {
		return nil, &apiError{errorBadData, fmt.Errorf("invalid label name: %q", name)}
	}
	vals, err := api.newQuerier(ctx).LabelValuesForLabelName(model.LabelName(name))
	if err != nil {
		return nil, &apiError{errorExec, err}
	}
	sort.Sort(vals)

	return vals, nil
}

func (api *API) series(ctx context.Context, r *http.Request) (interface{}, *apiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}

	var start model.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		start = model.Earliest
	}

	var end model.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		end = model.Latest
	}

	var matcherSets []metric.LabelMatchers
	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
		matcherSets = append(matcherSets, matchers)
	}

	res, err := api.newQuerier(ctx).MetricsForLabelMatchers(start, end, matcherSets...)
	if err != nil {
		return nil, &apiError{errorExec, err}
	}

	metrics := make([]model.Metric, 0, len(res))
	for _, met := range res {
		metrics = append(metrics, met.Metric)
	}
	return metrics, nil
}

func (api *API) dropSeries(ctx context.Context, r *http.Request) (interface{}, *apiError) {
	return nil, &apiError{errorExec, fmt.Errorf("dropping series is not supported")}
}

func (api *API) newEngine(ctx context.Context) *promql.Engine {
	return promql.NewEngine(api.newQuerier(ctx), nil)
}

func respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	b, err := json.Marshal(&response{
		Status: statusSuccess,
		Data:   data,
	})
	if err != nil {
		return
	}
	w.Write(b)
}

func respondError(w http.ResponseWriter, apiErr *apiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	var code int
	switch apiErr.typ {
	case errorBadData:
		code = http.StatusBadRequest
	case errorExec:
		code = 422
	case errorCanceled, errorTimeout:
		code = http.StatusServiceUnavailable
	default:
		code = http.StatusInternalServerError
	}
	w.WriteHeader(code)

	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})
	if err != nil {
		return
	}
	w.Write(b)
}

func parseTime(s string) (model.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		ts := int64(t * float64(time.Second))
		return model.TimeFromUnixNano(ts), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return model.TimeFromUnixNano(t.UnixNano()), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(d * float64(time.Second)), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid duration", s)
}
