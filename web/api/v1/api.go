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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/web/api/v1/structs"
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
}

type targetRetriever interface {
	Targets() []*retrieval.Target
}

type alertmanagerRetriever interface {
	Alertmanagers() []*url.URL
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

type apiFunc func(r *http.Request) (interface{}, *structs.ApiError)

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	Queryable   promql.Queryable
	QueryEngine *promql.Engine

	targetRetriever       targetRetriever
	alertmanagerRetriever alertmanagerRetriever

	now    func() time.Time
	config func() config.Config
	ready  func(http.HandlerFunc) http.HandlerFunc
}

// NewAPI returns an initialized API type.
func NewAPI(
	qe *promql.Engine,
	q promql.Queryable,
	tr targetRetriever,
	ar alertmanagerRetriever,
	configFunc func() config.Config,
	readyFunc func(http.HandlerFunc) http.HandlerFunc,
) *API {
	return &API{
		QueryEngine:           qe,
		Queryable:             q,
		targetRetriever:       tr,
		alertmanagerRetriever: ar,
		now:    time.Now,
		config: configFunc,
		ready:  readyFunc,
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
		return api.ready(prometheus.InstrumentHandler(name, httputil.CompressionHandler{
			Handler: hf,
		}))
	}

	r.Options("/*path", instr("options", api.options))

	r.Get("/query", instr("query", api.query))
	r.Post("/query", instr("query", api.query))
	r.Get("/query_range", instr("query_range", api.queryRange))
	r.Post("/query_range", instr("query_range", api.queryRange))

	r.Get("/label/:name/values", instr("label_values", api.labelValues))

	r.Get("/series", instr("series", api.series))
	r.Del("/series", instr("drop_series", api.dropSeries))

	r.Get("/targets", instr("targets", api.targets))
	r.Get("/alertmanagers", instr("alertmanagers", api.alertmanagers))

	r.Get("/status/config", instr("config", api.serveConfig))
	r.Post("/read", api.ready(prometheus.InstrumentHandler("read", http.HandlerFunc(api.remoteRead))))
}

func (api *API) options(r *http.Request) (interface{}, *structs.ApiError) {
	return nil, nil
}

func (api *API) query(r *http.Request) (interface{}, *structs.ApiError) {
	var ts time.Time
	if t := r.FormValue("time"); t != "" {
		var err error
		ts, err = parseTime(t)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}
	} else {
		ts = api.now()
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewInstantQuery(r.FormValue("query"), ts)
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &structs.ApiError{structs.ErrorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &structs.ApiError{structs.ErrorTimeout, res.Err}
		case promql.ErrStorage:
			return nil, &structs.ApiError{structs.ErrorInternal, res.Err}
		}
		return nil, &structs.ApiError{structs.ErrorExec, res.Err}
	}
	return &structs.QueryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
	}, nil
}

func (api *API) queryRange(r *http.Request) (interface{}, *structs.ApiError) {
	start, err := parseTime(r.FormValue("start"))
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}
	end, err := parseTime(r.FormValue("end"))
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}
	if end.Before(start) {
		err := errors.New("end timestamp must not be before start time")
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	step, err := parseDuration(r.FormValue("step"))
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	if step <= 0 {
		err := errors.New("zero or negative query resolution step widths are not accepted. Try a positive integer")
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewRangeQuery(r.FormValue("query"), start, end, step)
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorBadData, err}
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &structs.ApiError{structs.ErrorCanceled, res.Err}
		case promql.ErrQueryTimeout:
			return nil, &structs.ApiError{structs.ErrorTimeout, res.Err}
		}
		return nil, &structs.ApiError{structs.ErrorExec, res.Err}
	}

	return &structs.QueryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
	}, nil
}

func (api *API) labelValues(r *http.Request) (interface{}, *structs.ApiError) {
	ctx := r.Context()
	name := route.Param(ctx, "name")

	if !model.LabelNameRE.MatchString(name) {
		return nil, &structs.ApiError{structs.ErrorBadData, fmt.Errorf("invalid label name: %q", name)}
	}
	q, err := api.Queryable.Querier(ctx, math.MinInt64, math.MaxInt64)
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorExec, err}
	}
	defer q.Close()

	// TODO(fabxc): add back request context.
	vals, err := q.LabelValues(name)
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorExec, err}
	}

	return vals, nil
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0)
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999)
)

func (api *API) series(r *http.Request) (interface{}, *structs.ApiError) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &structs.ApiError{structs.ErrorBadData, fmt.Errorf("no match[] parameter provided")}
	}

	var start time.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}
	} else {
		start = minTime
	}

	var end time.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}
	} else {
		end = maxTime
	}

	var matcherSets [][]*labels.Matcher
	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &structs.ApiError{structs.ErrorBadData, err}
		}
		matcherSets = append(matcherSets, matchers)
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, &structs.ApiError{structs.ErrorExec, err}
	}
	defer q.Close()

	var set storage.SeriesSet

	for _, mset := range matcherSets {
		set = storage.DeduplicateSeriesSet(set, q.Select(mset...))
	}

	metrics := []labels.Labels{}

	for set.Next() {
		metrics = append(metrics, set.At().Labels())
	}
	if set.Err() != nil {
		return nil, &structs.ApiError{structs.ErrorExec, set.Err()}
	}

	return metrics, nil
}

func (api *API) dropSeries(r *http.Request) (interface{}, *structs.ApiError) {
	return nil, &structs.ApiError{structs.ErrorInternal, fmt.Errorf("not implemented")}
}

func (api *API) targets(r *http.Request) (interface{}, *structs.ApiError) {
	targets := api.targetRetriever.Targets()
	res := &structs.TargetDiscovery{ActiveTargets: make([]*structs.Target, len(targets))}

	for i, t := range targets {
		lastErrStr := ""
		lastErr := t.LastError()
		if lastErr != nil {
			lastErrStr = lastErr.Error()
		}

		res.ActiveTargets[i] = &structs.Target{
			DiscoveredLabels: t.DiscoveredLabels().Map(),
			Labels:           t.Labels().Map(),
			ScrapeURL:        t.URL().String(),
			LastError:        lastErrStr,
			LastScrape:       t.LastScrape(),
			Health:           t.Health(),
		}
	}

	return res, nil
}

func (api *API) alertmanagers(r *http.Request) (interface{}, *structs.ApiError) {
	urls := api.alertmanagerRetriever.Alertmanagers()
	ams := &structs.AlertmanagerDiscovery{ActiveAlertmanagers: make([]*structs.AlertmanagerTarget, len(urls))}

	for i, url := range urls {
		ams.ActiveAlertmanagers[i] = &structs.AlertmanagerTarget{URL: url.String()}
	}

	return ams, nil
}

func (api *API) serveConfig(r *http.Request) (interface{}, *structs.ApiError) {
	cfg := &structs.PrometheusConfig{
		YAML: api.config().String(),
	}
	return cfg, nil
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

		resp.Results[i], err = remote.ToQueryResult(querier.Select(filteredMatchers...))
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

	b, err := json.Marshal(&structs.Response{
		Status: structs.StatusSuccess,
		Data:   data,
	})
	if err != nil {
		return
	}
	w.Write(b)
}

func respondError(w http.ResponseWriter, apiErr *structs.ApiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	var code int
	switch apiErr.Typ {
	case structs.ErrorBadData:
		code = http.StatusBadRequest
	case structs.ErrorExec:
		code = 422
	case structs.ErrorCanceled, structs.ErrorTimeout:
		code = http.StatusServiceUnavailable
	case structs.ErrorInternal:
		code = http.StatusInternalServerError
	default:
		code = http.StatusInternalServerError
	}
	w.WriteHeader(code)

	b, err := json.Marshal(&structs.Response{
		Status:    structs.StatusError,
		ErrorType: apiErr.Typ,
		Error:     apiErr.Err.Error(),
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
