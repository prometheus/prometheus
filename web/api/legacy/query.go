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

package legacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

// Enables cross-site script calls.
func setAccessControlHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Date")
}

func httpJSONError(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	errorJSON(w, err)
}

func parseTimestampOrNow(t string, now model.Time) (model.Time, error) {
	if t == "" {
		return now, nil
	}

	tFloat, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, err
	}
	return model.TimeFromUnixNano(int64(tFloat * float64(time.Second/time.Nanosecond))), nil
}

func parseDuration(d string) (time.Duration, error) {
	dFloat, err := strconv.ParseFloat(d, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(dFloat * float64(time.Second/time.Nanosecond)), nil
}

// Options handles OPTIONS requests to /api/... endpoints.
func (api *API) Options(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

// Query handles the /api/query endpoint.
func (api *API) Query(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	params := getQueryParams(r)
	expr := params.Get("expr")

	timestamp, err := parseTimestampOrNow(params.Get("timestamp"), api.Now())
	if err != nil {
		httpJSONError(w, fmt.Errorf("invalid query timestamp %s", err), http.StatusBadRequest)
		return
	}

	query, err := api.QueryEngine.NewInstantQuery(expr, timestamp)
	if err != nil {
		httpJSONError(w, err, http.StatusOK)
		return
	}
	res := query.Exec()
	if res.Err != nil {
		httpJSONError(w, res.Err, http.StatusOK)
		return
	}
	log.Debugf("Instant query: %s\nQuery stats:\n%s\n", expr, query.Stats())

	if vec, ok := res.Value.(model.Vector); ok {
		respondJSON(w, plainVec(vec))
		return
	}
	if sca, ok := res.Value.(*model.Scalar); ok {
		respondJSON(w, (*plainScalar)(sca))
		return
	}
	if str, ok := res.Value.(*model.String); ok {
		respondJSON(w, (*plainString)(str))
		return
	}

	respondJSON(w, res.Value)
}

// plainVec is an indirection that hides the original MarshalJSON method
// which does not fit the response format for the legacy API.
type plainVec model.Vector

func (pv plainVec) MarshalJSON() ([]byte, error) {
	type plainSmpl model.Sample

	v := make([]*plainSmpl, len(pv))
	for i, sv := range pv {
		v[i] = (*plainSmpl)(sv)
	}

	return json.Marshal(&v)
}

func (pv plainVec) Type() model.ValueType {
	return model.ValVector
}

func (pv plainVec) String() string {
	return ""
}

// plainScalar is an indirection that hides the original MarshalJSON method
// which does not fit the response format for the legacy API.
type plainScalar model.Scalar

func (ps plainScalar) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(ps.Value), 'f', -1, 64)
	return json.Marshal(&s)
}

func (plainScalar) Type() model.ValueType {
	return model.ValScalar
}

func (plainScalar) String() string {
	return ""
}

// plainString is an indirection that hides the original MarshalJSON method
// which does not fit the response format for the legacy API.
type plainString model.String

func (pv plainString) Type() model.ValueType {
	return model.ValString
}

func (pv plainString) String() string {
	return ""
}

// QueryRange handles the /api/query_range endpoint.
func (api *API) QueryRange(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	params := getQueryParams(r)
	expr := params.Get("expr")

	duration, err := parseDuration(params.Get("range"))
	if err != nil {
		httpJSONError(w, fmt.Errorf("invalid query range: %s", err), http.StatusBadRequest)
		return
	}

	step, err := parseDuration(params.Get("step"))
	if err != nil {
		httpJSONError(w, fmt.Errorf("invalid query resolution: %s", err), http.StatusBadRequest)
		return
	}

	end, err := parseTimestampOrNow(params.Get("end"), api.Now())
	if err != nil {
		httpJSONError(w, fmt.Errorf("invalid query timestamp: %s", err), http.StatusBadRequest)
		return
	}
	// TODO(julius): Remove this special-case handling a while after PromDash and
	// other API consumers have been changed to no longer set "end=0" for setting
	// the current time as the end time. Instead, the "end" parameter should
	// simply be omitted or set to an empty string for that case.
	if end == 0 {
		end = api.Now()
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if duration/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	// Align the start to step "tick" boundary.
	end = end.Add(-time.Duration(end.UnixNano() % int64(step)))
	start := end.Add(-duration)

	query, err := api.QueryEngine.NewRangeQuery(expr, start, end, step)
	if err != nil {
		httpJSONError(w, err, http.StatusOK)
		return
	}
	matrix, err := query.Exec().Matrix()
	if err != nil {
		httpJSONError(w, err, http.StatusOK)
		return
	}

	log.Debugf("Range query: %s\nQuery stats:\n%s\n", expr, query.Stats())
	respondJSON(w, matrix)
}

// Metrics handles the /api/metrics endpoint.
func (api *API) Metrics(w http.ResponseWriter, r *http.Request) {
	setAccessControlHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	metricNames := api.Storage.LabelValuesForLabelName(model.MetricNameLabel)
	sort.Sort(metricNames)
	resultBytes, err := json.Marshal(metricNames)
	if err != nil {
		log.Error("Error marshalling metric names: ", err)
		httpJSONError(w, fmt.Errorf("error marshalling metric names: %s", err), http.StatusInternalServerError)
		return
	}
	w.Write(resultBytes)
}

// GetQueryParams calls r.ParseForm and returns r.Form.
func getQueryParams(r *http.Request) url.Values {
	r.ParseForm()
	return r.Form
}

var jsonFormatVersion = 1

// ErrorJSON writes the given error JSON-formatted to w.
func errorJSON(w io.Writer, err error) error {
	data := struct {
		Type    string `json:"type"`
		Value   string `json:"value"`
		Version int    `json:"version"`
	}{
		Type:    "error",
		Value:   err.Error(),
		Version: jsonFormatVersion,
	}
	enc := json.NewEncoder(w)
	return enc.Encode(data)
}

// RespondJSON converts the given data value to JSON and writes it to w.
func respondJSON(w io.Writer, val model.Value) error {
	data := struct {
		Type    string      `json:"type"`
		Value   interface{} `json:"value"`
		Version int         `json:"version"`
	}{
		Type:    val.Type().String(),
		Value:   val,
		Version: jsonFormatVersion,
	}
	// TODO(fabxc): Adding MarshalJSON to promql.Values might be a good idea.
	if sc, ok := val.(*model.Scalar); ok {
		data.Value = sc.Value
	}
	enc := json.NewEncoder(w)
	return enc.Encode(data)
}
