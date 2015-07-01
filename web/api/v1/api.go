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
	"golang.org/x/net/context"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/util/route"
	"github.com/prometheus/prometheus/util/strutil"
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

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	Storage     local.Storage
	QueryEngine *promql.Engine

	context func(r *http.Request) context.Context
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Date")
}

type apiFunc func(r *http.Request) (interface{}, *apiError)

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	if api.context == nil {
		api.context = route.Context
	}

	instr := func(name string, f apiFunc) http.HandlerFunc {
		return prometheus.InstrumentHandlerFunc(name, func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			if data, err := f(r); err != nil {
				respondError(w, err, data)
			} else {
				respond(w, data)
			}
		})
	}

	r.Get("/query", instr("query", api.query))
	r.Get("/query_range", instr("query_range", api.queryRange))

	r.Get("/label/:name/values", instr("label_values", api.labelValues))

	r.Get("/series", instr("series", api.series))
	r.Del("/series", instr("drop_series", api.dropSeries))
}

type queryData struct {
	ResultType promql.ExprType `json:"resultType"`
	Result     promql.Value    `json:"result"`
}

func (api *API) query(r *http.Request) (interface{}, *apiError) {
	ts, err := parseTime(r.FormValue("time"))
	if err != nil {
		return nil, &apiError{errorBadData, err}
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

func (api *API) labelValues(r *http.Request) (interface{}, *apiError) {
	name := route.Param(api.context(r), "name")

	if !clientmodel.LabelNameRE.MatchString(name) {
		return nil, &apiError{errorBadData, fmt.Errorf("invalid label name: %q", name)}
	}
	vals := api.Storage.LabelValuesForLabelName(clientmodel.LabelName(name))
	sort.Sort(vals)

	return vals, nil
}

func (api *API) series(r *http.Request) (interface{}, *apiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}
	res := map[clientmodel.Fingerprint]clientmodel.COWMetric{}

	for _, lm := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(lm)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
		for fp, met := range api.Storage.MetricsForLabelMatchers(matchers...) {
			res[fp] = met
		}
	}

	metrics := make([]clientmodel.Metric, 0, len(res))
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
	fps := map[clientmodel.Fingerprint]struct{}{}

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
	w.WriteHeader(200)

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
	w.WriteHeader(422)

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

func parseTime(s string) (clientmodel.Timestamp, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		ts := int64(t * float64(time.Second))
		return clientmodel.TimestampFromUnixNano(ts), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return clientmodel.TimestampFromTime(t), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(d * float64(time.Second)), nil
	}
	if d, err := strutil.StringToDuration(s); err == nil {
		return d, nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid duration", s)
}
