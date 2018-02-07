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

package v1

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/tsdb"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/util/stats"
	tsdbLabels "github.com/prometheus/tsdb/labels"
)

type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

type errorType string

const (
	errorNone        errorType = ""
	errorTimeout     errorType = "timeout"
	errorCanceled    errorType = "canceled"
	errorExec        errorType = "execution"
	errorBadData     errorType = "bad_data"
	errorInternal    errorType = "internal"
	errorUnavailable errorType = "unavailable"
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

type targetRetriever interface {
	Targets() []*scrape.Target
	DroppedTargets() []*scrape.Target
}

type alertmanagerRetriever interface {
	Alertmanagers() []*url.URL
	DroppedAlertmanagers() []*url.URL
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
	Queryable   storage.Queryable
	QueryEngine *promql.Engine

	targetRetriever       targetRetriever
	alertmanagerRetriever alertmanagerRetriever

	now      func() time.Time
	config   func() config.Config
	flagsMap map[string]string
	ready    func(http.HandlerFunc) http.HandlerFunc

	db          func() *tsdb.DB
	enableAdmin bool
}

// NewAPI returns an initialized API type.
func NewAPI(
	qe *promql.Engine,
	q storage.Queryable,
	tr targetRetriever,
	ar alertmanagerRetriever,
	configFunc func() config.Config,
	flagsMap map[string]string,
	readyFunc func(http.HandlerFunc) http.HandlerFunc,
	db func() *tsdb.DB,
	enableAdmin bool,
) *API {
	return &API{
		QueryEngine:           qe,
		Queryable:             q,
		targetRetriever:       tr,
		alertmanagerRetriever: ar,
		now:         time.Now,
		config:      configFunc,
		flagsMap:    flagsMap,
		ready:       readyFunc,
		db:          db,
		enableAdmin: enableAdmin,
	}
}

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	wrap := func(f apiFunc) http.HandlerFunc {
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
		return api.ready(httputil.CompressionHandler{
			Handler: hf,
		}.ServeHTTP)
	}

	r.Options("/*path", wrap(api.options))

	r.Get("/query", wrap(api.query))
	r.Post("/query", wrap(api.query))
	r.Get("/query_range", wrap(api.queryRange))
	r.Post("/query_range", wrap(api.queryRange))

	r.Get("/label/:name/values", wrap(api.labelValues))

	r.Get("/series", wrap(api.series))
	r.Del("/series", wrap(api.dropSeries))

	r.Get("/targets", wrap(api.targets))
	r.Get("/alertmanagers", wrap(api.alertmanagers))

	r.Get("/status/config", wrap(api.serveConfig))
	r.Get("/status/flags", wrap(api.serveFlags))
	r.Post("/read", api.ready(http.HandlerFunc(api.remoteRead)))

	// Admin APIs
	r.Post("/admin/tsdb/delete_series", wrap(api.deleteSeries))
	r.Post("/admin/tsdb/clean_tombstones", wrap(api.cleanTombstones))
	r.Post("/admin/tsdb/snapshot", wrap(api.snapshot))
}

type queryData struct {
	ResultType promql.ValueType  `json:"resultType"`
	Result     promql.Value      `json:"result"`
	Stats      *stats.QueryStats `json:"stats,omitempty"`
}

func (api *API) options(r *http.Request) (interface{}, *apiError) {
	return nil, nil
}

func (api *API) query(r *http.Request) (interface{}, *apiError) {
	var ts time.Time
	if t := r.FormValue("time"); t != "" {
		var err error
		ts, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		ts = api.now()
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewInstantQuery(api.Queryable, r.FormValue("query"), ts)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}
		case promql.ErrStorage:
			return nil, &apiError{errorInternal, res.Err}
		}
		return nil, &apiError{errorExec, res.Err}
	}

	// Optional stats field in response if parameter "stats" is not empty.
	var qs *stats.QueryStats
	if r.FormValue("stats") != "" {
		qs = stats.NewQueryStats(qry.Stats())
	}

	return &queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
		Stats:      qs,
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
	if end.Before(start) {
		err := errors.New("end timestamp must not be before start time")
		return nil, &apiError{errorBadData, err}
	}

	step, err := parseDuration(r.FormValue("step"))
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	if step <= 0 {
		err := errors.New("zero or negative query resolution step widths are not accepted. Try a positive integer")
		return nil, &apiError{errorBadData, err}
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return nil, &apiError{errorBadData, err}
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewRangeQuery(api.Queryable, r.FormValue("query"), start, end, step)
	if err != nil {
		return nil, &apiError{errorBadData, err}
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}
		}
		return nil, &apiError{errorExec, res.Err}
	}

	// Optional stats field in response if parameter "stats" is not empty.
	var qs *stats.QueryStats
	if r.FormValue("stats") != "" {
		qs = stats.NewQueryStats(qry.Stats())
	}

	return &queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
		Stats:      qs,
	}, nil
}

func (api *API) labelValues(r *http.Request) (interface{}, *apiError) {
	ctx := r.Context()
	name := route.Param(ctx, "name")

	if !model.LabelNameRE.MatchString(name) {
		return nil, &apiError{errorBadData, fmt.Errorf("invalid label name: %q", name)}
	}
	q, err := api.Queryable.Querier(ctx, math.MinInt64, math.MaxInt64)
	if err != nil {
		return nil, &apiError{errorExec, err}
	}
	defer q.Close()

	vals, err := q.LabelValues(name)
	if err != nil {
		return nil, &apiError{errorExec, err}
	}

	return vals, nil
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0)
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999)
)

func (api *API) series(r *http.Request) (interface{}, *apiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}

	var start time.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		start = minTime
	}

	var end time.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		end = maxTime
	}

	var matcherSets [][]*labels.Matcher
	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
		matcherSets = append(matcherSets, matchers)
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, &apiError{errorExec, err}
	}
	defer q.Close()

	var sets []storage.SeriesSet
	for _, mset := range matcherSets {
		s, err := q.Select(nil, mset...)
		if err != nil {
			return nil, &apiError{errorExec, err}
		}
		sets = append(sets, s)
	}

	set := storage.NewMergeSeriesSet(sets)
	metrics := []labels.Labels{}
	for set.Next() {
		metrics = append(metrics, set.At().Labels())
	}
	if set.Err() != nil {
		return nil, &apiError{errorExec, set.Err()}
	}

	return metrics, nil
}

func (api *API) dropSeries(r *http.Request) (interface{}, *apiError) {
	return nil, &apiError{errorInternal, fmt.Errorf("not implemented")}
}

// Target has the information for one target.
type Target struct {
	// Labels before any processing.
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
	// Any labels that are added to this target and its metrics.
	Labels map[string]string `json:"labels"`

	ScrapeURL string `json:"scrapeUrl"`

	LastError  string              `json:"lastError"`
	LastScrape time.Time           `json:"lastScrape"`
	Health     scrape.TargetHealth `json:"health"`
}

// DroppedTarget has the information for one target that was dropped during relabelling.
type DroppedTarget struct {
	// Labels before any processing.
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
}

// TargetDiscovery has all the active targets.
type TargetDiscovery struct {
	ActiveTargets  []*Target        `json:"activeTargets"`
	DroppedTargets []*DroppedTarget `json:"droppedTargets"`
}

func (api *API) targets(r *http.Request) (interface{}, *apiError) {
	targets := api.targetRetriever.Targets()
	droppedTargets := api.targetRetriever.DroppedTargets()
	res := &TargetDiscovery{ActiveTargets: make([]*Target, len(targets)), DroppedTargets: make([]*DroppedTarget, len(droppedTargets))}

	for i, t := range targets {
		lastErrStr := ""
		lastErr := t.LastError()
		if lastErr != nil {
			lastErrStr = lastErr.Error()
		}

		res.ActiveTargets[i] = &Target{
			DiscoveredLabels: t.DiscoveredLabels().Map(),
			Labels:           t.Labels().Map(),
			ScrapeURL:        t.URL().String(),
			LastError:        lastErrStr,
			LastScrape:       t.LastScrape(),
			Health:           t.Health(),
		}
	}

	for i, t := range droppedTargets {
		res.DroppedTargets[i] = &DroppedTarget{
			DiscoveredLabels: t.DiscoveredLabels().Map(),
		}
	}

	return res, nil
}

// AlertmanagerDiscovery has all the active Alertmanagers.
type AlertmanagerDiscovery struct {
	ActiveAlertmanagers  []*AlertmanagerTarget `json:"activeAlertmanagers"`
	DroppedAlertmanagers []*AlertmanagerTarget `json:"droppedAlertmanagers"`
}

// AlertmanagerTarget has info on one AM.
type AlertmanagerTarget struct {
	URL string `json:"url"`
}

func (api *API) alertmanagers(r *http.Request) (interface{}, *apiError) {
	urls := api.alertmanagerRetriever.Alertmanagers()
	droppedURLS := api.alertmanagerRetriever.DroppedAlertmanagers()
	ams := &AlertmanagerDiscovery{ActiveAlertmanagers: make([]*AlertmanagerTarget, len(urls)), DroppedAlertmanagers: make([]*AlertmanagerTarget, len(droppedURLS))}
	for i, url := range urls {
		ams.ActiveAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	for i, url := range droppedURLS {
		ams.DroppedAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	return ams, nil
}

type prometheusConfig struct {
	YAML string `json:"yaml"`
}

func (api *API) serveConfig(r *http.Request) (interface{}, *apiError) {
	cfg := &prometheusConfig{
		YAML: api.config().String(),
	}
	return cfg, nil
}

func (api *API) serveFlags(r *http.Request) (interface{}, *apiError) {
	return api.flagsMap, nil
}

func (api *API) remoteRead(w http.ResponseWriter, r *http.Request) {
	req, err := remote.DecodeReadRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := prompb.ReadResponse{
		Results: make([]*prompb.QueryResult, len(req.Queries)),
	}
	for i, query := range req.Queries {
		from, through, matchers, err := remote.FromQuery(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		querier, err := api.Queryable.Querier(r.Context(), from, through)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer querier.Close()

		// Change equality matchers which match external labels
		// to a matcher that looks for an empty label,
		// as that label should not be present in the storage.
		externalLabels := api.config().GlobalConfig.ExternalLabels.Clone()
		filteredMatchers := make([]*labels.Matcher, 0, len(matchers))
		for _, m := range matchers {
			value := externalLabels[model.LabelName(m.Name)]
			if m.Type == labels.MatchEqual && value == model.LabelValue(m.Value) {
				matcher, err := labels.NewMatcher(labels.MatchEqual, m.Name, "")
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				filteredMatchers = append(filteredMatchers, matcher)
			} else {
				filteredMatchers = append(filteredMatchers, m)
			}
		}

		set, err := querier.Select(nil, filteredMatchers...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Results[i], err = remote.ToQueryResult(set)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Add external labels back in, in sorted order.
		sortedExternalLabels := make([]*prompb.Label, 0, len(externalLabels))
		for name, value := range externalLabels {
			sortedExternalLabels = append(sortedExternalLabels, &prompb.Label{
				Name:  string(name),
				Value: string(value),
			})
		}
		sort.Slice(sortedExternalLabels, func(i, j int) bool {
			return sortedExternalLabels[i].Name < sortedExternalLabels[j].Name
		})

		for _, ts := range resp.Results[i].Timeseries {
			ts.Labels = mergeLabels(ts.Labels, sortedExternalLabels)
		}
	}

	if err := remote.EncodeReadResponse(&resp, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (api *API) deleteSeries(r *http.Request) (interface{}, *apiError) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}
	}
	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}
	}

	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}
	}

	var start time.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		start = minTime
	}

	var end time.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}
	} else {
		end = maxTime
	}

	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &apiError{errorBadData, err}
		}

		var selector tsdbLabels.Selector
		for _, m := range matchers {
			selector = append(selector, convertMatcher(m))
		}

		if err := db.Delete(timestamp.FromTime(start), timestamp.FromTime(end), selector...); err != nil {
			return nil, &apiError{errorInternal, err}
		}
	}

	return nil, nil
}

func (api *API) snapshot(r *http.Request) (interface{}, *apiError) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}
	}
	skipHead, _ := strconv.ParseBool(r.FormValue("skip_head"))

	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}
	}

	var (
		snapdir = filepath.Join(db.Dir(), "snapshots")
		name    = fmt.Sprintf("%s-%x",
			time.Now().UTC().Format("20060102T150405Z0700"),
			rand.Int())
		dir = filepath.Join(snapdir, name)
	)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, &apiError{errorInternal, fmt.Errorf("create snapshot directory: %s", err)}
	}
	if err := db.Snapshot(dir, !skipHead); err != nil {
		return nil, &apiError{errorInternal, fmt.Errorf("create snapshot: %s", err)}
	}

	return struct {
		Name string `json:"name"`
	}{name}, nil
}

func (api *API) cleanTombstones(r *http.Request) (interface{}, *apiError) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}
	}
	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}
	}

	if err := db.CleanTombstones(); err != nil {
		return nil, &apiError{errorInternal, err}
	}

	return nil, nil
}

func convertMatcher(m *labels.Matcher) tsdbLabels.Matcher {
	switch m.Type {
	case labels.MatchEqual:
		return tsdbLabels.NewEqualMatcher(m.Name, m.Value)

	case labels.MatchNotEqual:
		return tsdbLabels.Not(tsdbLabels.NewEqualMatcher(m.Name, m.Value))

	case labels.MatchRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, "^(?:"+m.Value+")$")
		if err != nil {
			panic(err)
		}
		return res

	case labels.MatchNotRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, "^(?:"+m.Value+")$")
		if err != nil {
			panic(err)
		}
		return tsdbLabels.Not(res)
	}
	panic("storage.convertMatcher: invalid matcher type")
}

// mergeLabels merges two sets of sorted proto labels, preferring those in
// primary to those in secondary when there is an overlap.
func mergeLabels(primary, secondary []*prompb.Label) []*prompb.Label {
	result := make([]*prompb.Label, 0, len(primary)+len(secondary))
	i, j := 0, 0
	for i < len(primary) && j < len(secondary) {
		if primary[i].Name < secondary[j].Name {
			result = append(result, primary[i])
			i++
		} else if primary[i].Name > secondary[j].Name {
			result = append(result, secondary[j])
			j++
		} else {
			result = append(result, primary[i])
			i++
			j++
		}
	}
	for ; i < len(primary); i++ {
		result = append(result, primary[i])
	}
	for ; j < len(secondary); j++ {
		result = append(result, secondary[j])
	}
	return result
}

func respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json := jsoniter.ConfigCompatibleWithStandardLibrary
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
	case errorInternal:
		code = http.StatusInternalServerError
	default:
		code = http.StatusInternalServerError
	}
	w.WriteHeader(code)

	json := jsoniter.ConfigCompatibleWithStandardLibrary
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

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		ts := d * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return 0, fmt.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		return time.Duration(ts), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, fmt.Errorf("cannot parse %q to a valid duration", s)
}

func init() {
	jsoniter.RegisterTypeEncoderFunc("promql.Point", marshalPointJSON, marshalPointJSONIsEmpty)
}

func marshalPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.Point)(ptr))
	stream.WriteArrayStart()
	// Write out the timestamp as a float divided by 1000.
	// This is ~3x faster than converting to a float.
	t := p.T
	if t < 0 {
		stream.WriteRaw(`-`)
		t = -t
	}
	stream.WriteInt64(t / 1000)
	fraction := t % 1000
	if fraction != 0 {
		stream.WriteRaw(`.`)
		if fraction < 100 {
			stream.WriteRaw(`0`)
		}
		if fraction < 10 {
			stream.WriteRaw(`0`)
		}
		stream.WriteInt64(fraction)
	}
	stream.WriteMore()
	stream.WriteRaw(`"`)
	stream.WriteFloat64(p.V)
	stream.WriteRaw(`"`)
	stream.WriteArrayEnd()

}

func marshalPointJSONIsEmpty(ptr unsafe.Pointer) bool {
	return false
}
