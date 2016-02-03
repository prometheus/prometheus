package v1

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
	"github.com/prometheus/prometheus/rules"
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

type apiFunc func(r *http.Request) (interface{}, *apiError)

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	Storage     local.Storage
	QueryEngine *promql.Engine
	PathPrefix  string

	context func(r *http.Request) context.Context
	now     func() model.Time
}

// NewAPI returns an initialized API type.
func NewAPI(qe *promql.Engine, st local.Storage, pathPrefix string) *API {
	return &API{
		QueryEngine: qe,
		Storage:     st,
		PathPrefix:  pathPrefix,
		context:     route.Context,
		now:         model.Now,
	}
}

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	instr := func(name string, f apiFunc) http.HandlerFunc {
		hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			if data, err := f(r); err != nil {
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
	r.Get("/query_rule", instr("query_rule", api.queryRule))

	r.Get("/label/:name/values", instr("label_values", api.labelValues))

	r.Get("/series", instr("series", api.series))
	r.Del("/series", instr("drop_series", api.dropSeries))
}

type queryData struct {
	ResultType model.ValueType `json:"resultType"`
	Result     model.Value     `json:"result"`
}

func (api *API) options(r *http.Request) (interface{}, *apiError) {
	return nil, nil
}

func (api *API) query(r *http.Request) (interface{}, *apiError) {
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

	qry, err := api.QueryEngine.NewInstantQuery(r.FormValue("query"), ts)
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

func (api *API) queryRange(r *http.Request) (interface{}, *apiError) {
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

	qry, err := api.QueryEngine.NewRangeQuery(r.FormValue("query"), start, end, step)
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

func (api *API) queryRule(r *http.Request) (interface{}, *apiError) {
	end, err := defaultTime(r.FormValue("end"), api.now())
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	count, err := defaultInt(r.FormValue("count"), 1)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	step, err := defaultInt(r.FormValue("step"), 60)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	// start evaluating rules based on the 'end' time,
	// this is user friendly:
	// "what alerts would fire at 'end' time, based on the past 5 minutes"
	start := end.Add(-time.Second * time.Duration((count-1)*step))

	st, err := promql.ParseStmts(string(r.FormValue("query")))
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	rls, err := rules.NewRules(st)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	var v []interface{}
	for _, r := range rls {
		for i := 0; i < count; i++ {
			ts := start.Add(time.Duration(step*i) * time.Second)
			_, err := r.Eval(ts, api.QueryEngine)
			if err != nil {
				switch err.(type) {
				case promql.ErrQueryCanceled:
					return nil, &apiError{errorCanceled, err}
				case promql.ErrQueryTimeout:
					return nil, &apiError{errorTimeout, err}
				}
				return nil, &apiError{errorExec, err}
			}
		}

		if r, ok := r.(*rules.AlertingRule); ok {
			for _, a := range r.Expand(end, api.QueryEngine, api.PathPrefix) {
				v = append(v, a)
			}
		}
	}
	return v, nil
}

func (api *API) labelValues(r *http.Request) (interface{}, *apiError) {
	name := route.Param(api.context(r), "name")

	if !model.LabelNameRE.MatchString(name) {
		return nil, &apiError{errorBadData, fmt.Errorf("invalid label name: %q", name)}
	}
	vals := api.Storage.LabelValuesForLabelName(model.LabelName(name))
	sort.Sort(vals)

	return vals, nil
}

func (api *API) series(r *http.Request) (interface{}, *apiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}
	res := map[model.Fingerprint]metric.Metric{}

	for _, lm := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(lm)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
		for fp, met := range api.Storage.MetricsForLabelMatchers(matchers...) {
			res[fp] = met
		}
	}

	metrics := make([]model.Metric, 0, len(res))
	for _, met := range res {
		metrics = append(metrics, met.Metric)
	}
	return metrics, nil
}

func (api *API) dropSeries(r *http.Request) (interface{}, *apiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}
	fps := map[model.Fingerprint]struct{}{}

	for _, lm := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(lm)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
		for fp := range api.Storage.MetricsForLabelMatchers(matchers...) {
			fps[fp] = struct{}{}
		}
	}
	for fp := range fps {
		api.Storage.DropMetricsForFingerprints(fp)
	}

	res := struct {
		NumDeleted int `json:"numDeleted"`
	}{
		NumDeleted: len(fps),
	}
	return res, nil
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

func defaultTime(value string, t model.Time) (model.Time, error) {
	if value == "" {
		return t, nil
	}
	return parseTime(value)
}

func defaultInt(value string, i int) (int, error) {
	if value == "" {
		return i, nil
	}
	return strconv.Atoi(value)
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
