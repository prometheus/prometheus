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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/tsdb"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
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
	errorNotFound    errorType = "not_found"
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
	TargetsActive() []*scrape.Target
	TargetsDropped() []*scrape.Target
}

type alertmanagerRetriever interface {
	Alertmanagers() []*url.URL
	DroppedAlertmanagers() []*url.URL
}

type rulesRetriever interface {
	RuleGroups() []*rules.Group
	AlertingRules() []*rules.AlertingRule
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

type apiFunc func(r *http.Request) (interface{}, *apiError, func())

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	Queryable   storage.Queryable
	QueryEngine *promql.Engine

	targetRetriever       targetRetriever
	alertmanagerRetriever alertmanagerRetriever
	rulesRetriever        rulesRetriever
	now                   func() time.Time
	config                func() config.Config
	flagsMap              map[string]string
	ready                 func(http.HandlerFunc) http.HandlerFunc

	db          func() *tsdb.DB
	enableAdmin bool
	logger      log.Logger
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
	logger log.Logger,
	rr rulesRetriever,
) *API {
	return &API{
		QueryEngine:           qe,
		Queryable:             q,
		targetRetriever:       tr,
		alertmanagerRetriever: ar,
		now:            time.Now,
		config:         configFunc,
		flagsMap:       flagsMap,
		ready:          readyFunc,
		db:             db,
		enableAdmin:    enableAdmin,
		rulesRetriever: rr,
	}
}

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	wrap := func(f apiFunc) http.HandlerFunc {
		hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			data, err, finalizer := f(r)
			if err != nil {
				api.respondError(w, err, data)
			} else if data != nil {
				api.respond(w, data)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			if finalizer != nil {
				finalizer()
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
	r.Get("/targets/metadata", wrap(api.targetMetadata))
	r.Get("/alertmanagers", wrap(api.alertmanagers))

	r.Get("/status/config", wrap(api.serveConfig))
	r.Get("/status/flags", wrap(api.serveFlags))
	r.Post("/read", api.ready(http.HandlerFunc(api.remoteRead)))

	r.Get("/alerts", wrap(api.alerts))
	r.Get("/rules", wrap(api.rules))

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

func (api *API) options(r *http.Request) (interface{}, *apiError, func()) {
	return nil, nil, nil
}

func (api *API) query(r *http.Request) (interface{}, *apiError, func()) {
	var ts time.Time
	if t := r.FormValue("time"); t != "" {
		var err error
		ts, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
	} else {
		ts = api.now()
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewInstantQuery(api.Queryable, r.FormValue("query"), ts)
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}, qry.Close
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}, qry.Close
		case promql.ErrStorage:
			return nil, &apiError{errorInternal, res.Err}, qry.Close
		}
		return nil, &apiError{errorExec, res.Err}, qry.Close
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
	}, nil, qry.Close
}

func (api *API) queryRange(r *http.Request) (interface{}, *apiError, func()) {
	start, err := parseTime(r.FormValue("start"))
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}
	end, err := parseTime(r.FormValue("end"))
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}
	if end.Before(start) {
		err := errors.New("end timestamp must not be before start time")
		return nil, &apiError{errorBadData, err}, nil
	}

	step, err := parseDuration(r.FormValue("step"))
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}

	if step <= 0 {
		err := errors.New("zero or negative query resolution step widths are not accepted. Try a positive integer")
		return nil, &apiError{errorBadData, err}, nil
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return nil, &apiError{errorBadData, err}, nil
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	qry, err := api.QueryEngine.NewRangeQuery(api.Queryable, r.FormValue("query"), start, end, step)
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, &apiError{errorCanceled, res.Err}, qry.Close
		case promql.ErrQueryTimeout:
			return nil, &apiError{errorTimeout, res.Err}, qry.Close
		}
		return nil, &apiError{errorExec, res.Err}, qry.Close
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
	}, nil, qry.Close
}

func (api *API) labelValues(r *http.Request) (interface{}, *apiError, func()) {
	ctx := r.Context()
	name := route.Param(ctx, "name")

	if !model.LabelNameRE.MatchString(name) {
		return nil, &apiError{errorBadData, fmt.Errorf("invalid label name: %q", name)}, nil
	}
	q, err := api.Queryable.Querier(ctx, math.MinInt64, math.MaxInt64)
	if err != nil {
		return nil, &apiError{errorExec, err}, nil
	}
	defer q.Close()

	vals, err := q.LabelValues(name)
	if err != nil {
		return nil, &apiError{errorExec, err}, nil
	}

	return vals, nil, nil
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0)
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999)
)

func (api *API) series(r *http.Request) (interface{}, *apiError, func()) {
	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}, nil
	}

	var start time.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
	} else {
		start = minTime
	}

	var end time.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
	} else {
		end = maxTime
	}

	var matcherSets [][]*labels.Matcher
	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
		matcherSets = append(matcherSets, matchers)
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, &apiError{errorExec, err}, nil
	}
	defer q.Close()

	var sets []storage.SeriesSet
	for _, mset := range matcherSets {
		s, err := q.Select(nil, mset...)
		if err != nil {
			return nil, &apiError{errorExec, err}, nil
		}
		sets = append(sets, s)
	}

	set := storage.NewMergeSeriesSet(sets)
	metrics := []labels.Labels{}
	for set.Next() {
		metrics = append(metrics, set.At().Labels())
	}
	if set.Err() != nil {
		return nil, &apiError{errorExec, set.Err()}, nil
	}

	return metrics, nil, nil
}

func (api *API) dropSeries(r *http.Request) (interface{}, *apiError, func()) {
	return nil, &apiError{errorInternal, fmt.Errorf("not implemented")}, nil
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

func (api *API) targets(r *http.Request) (interface{}, *apiError, func()) {
	tActive := api.targetRetriever.TargetsActive()
	tDropped := api.targetRetriever.TargetsDropped()
	res := &TargetDiscovery{ActiveTargets: make([]*Target, len(tActive)), DroppedTargets: make([]*DroppedTarget, len(tDropped))}

	for i, t := range tActive {
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

	for i, t := range tDropped {
		res.DroppedTargets[i] = &DroppedTarget{
			DiscoveredLabels: t.DiscoveredLabels().Map(),
		}
	}
	return res, nil, nil
}

func (api *API) targetMetadata(r *http.Request) (interface{}, *apiError, func()) {
	limit := -1
	if s := r.FormValue("limit"); s != "" {
		var err error
		if limit, err = strconv.Atoi(s); err != nil {
			return nil, &apiError{errorBadData, fmt.Errorf("limit must be a number")}, nil
		}
	}

	matchers, err := promql.ParseMetricSelector(r.FormValue("match_target"))
	if err != nil {
		return nil, &apiError{errorBadData, err}, nil
	}

	metric := r.FormValue("metric")

	var res []metricMetadata
Outer:
	for _, t := range api.targetRetriever.TargetsActive() {
		if limit >= 0 && len(res) >= limit {
			break
		}
		for _, m := range matchers {
			// Filter targets that don't satisfy the label matchers.
			if !m.Matches(t.Labels().Get(m.Name)) {
				continue Outer
			}
		}
		// If no metric is specified, get the full list for the target.
		if metric == "" {
			for _, md := range t.MetadataList() {
				res = append(res, metricMetadata{
					Target: t.Labels(),
					Metric: md.Metric,
					Type:   md.Type,
					Help:   md.Help,
				})
			}
			continue
		}
		// Get metadata for the specified metric.
		if md, ok := t.Metadata(metric); ok {
			res = append(res, metricMetadata{
				Target: t.Labels(),
				Type:   md.Type,
				Help:   md.Help,
			})
		}
	}
	if len(res) == 0 {
		return nil, &apiError{errorNotFound, errors.New("specified metadata not found")}, nil
	}
	return res, nil, nil
}

type metricMetadata struct {
	Target labels.Labels        `json:"target"`
	Metric string               `json:"metric,omitempty"`
	Type   textparse.MetricType `json:"type"`
	Help   string               `json:"help"`
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

func (api *API) alertmanagers(r *http.Request) (interface{}, *apiError, func()) {
	urls := api.alertmanagerRetriever.Alertmanagers()
	droppedURLS := api.alertmanagerRetriever.DroppedAlertmanagers()
	ams := &AlertmanagerDiscovery{ActiveAlertmanagers: make([]*AlertmanagerTarget, len(urls)), DroppedAlertmanagers: make([]*AlertmanagerTarget, len(droppedURLS))}
	for i, url := range urls {
		ams.ActiveAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	for i, url := range droppedURLS {
		ams.DroppedAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	return ams, nil, nil
}

// AlertDiscovery has info for all active alerts.
type AlertDiscovery struct {
	Alerts []*Alert `json:"alerts"`
}

// Alert has info for an alert.
type Alert struct {
	Labels      labels.Labels `json:"labels"`
	Annotations labels.Labels `json:"annotations"`
	State       string        `json:"state"`
	ActiveAt    *time.Time    `json:"activeAt,omitempty"`
	Value       float64       `json:"value"`
}

func (api *API) alerts(r *http.Request) (interface{}, *apiError, func()) {
	alertingRules := api.rulesRetriever.AlertingRules()
	alerts := []*Alert{}

	for _, alertingRule := range alertingRules {
		alerts = append(
			alerts,
			rulesAlertsToAPIAlerts(alertingRule.ActiveAlerts())...,
		)
	}

	res := &AlertDiscovery{Alerts: alerts}

	return res, nil, nil
}

func rulesAlertsToAPIAlerts(rulesAlerts []*rules.Alert) []*Alert {
	apiAlerts := make([]*Alert, len(rulesAlerts))
	for i, ruleAlert := range rulesAlerts {
		apiAlerts[i] = &Alert{
			Labels:      ruleAlert.Labels,
			Annotations: ruleAlert.Annotations,
			State:       ruleAlert.State.String(),
			ActiveAt:    &ruleAlert.ActiveAt,
			Value:       ruleAlert.Value,
		}
	}

	return apiAlerts
}

// RuleDiscovery has info for all rules
type RuleDiscovery struct {
	RuleGroups []*RuleGroup `json:"groups"`
}

// RuleGroup has info for rules which are part of a group
type RuleGroup struct {
	Name string `json:"name"`
	File string `json:"file"`
	// In order to preserve rule ordering, while exposing type (alerting or recording)
	// specific properties, both alerting and recording rules are exposed in the
	// same array.
	Rules    []rule  `json:"rules"`
	Interval float64 `json:"interval"`
}

type rule interface{}

type alertingRule struct {
	Name        string        `json:"name"`
	Query       string        `json:"query"`
	Duration    float64       `json:"duration"`
	Labels      labels.Labels `json:"labels"`
	Annotations labels.Labels `json:"annotations"`
	Alerts      []*Alert      `json:"alerts"`
	// Type of an alertingRule is always "alerting".
	Type string `json:"type"`
}

type recordingRule struct {
	Name   string        `json:"name"`
	Query  string        `json:"query"`
	Labels labels.Labels `json:"labels,omitempty"`
	// Type of a recordingRule is always "recording".
	Type string `json:"type"`
}

func (api *API) rules(r *http.Request) (interface{}, *apiError, func()) {
	ruleGroups := api.rulesRetriever.RuleGroups()
	res := &RuleDiscovery{RuleGroups: make([]*RuleGroup, len(ruleGroups))}
	for i, grp := range ruleGroups {
		apiRuleGroup := &RuleGroup{
			Name:     grp.Name(),
			File:     grp.File(),
			Interval: grp.Interval().Seconds(),
			Rules:    []rule{},
		}

		for _, r := range grp.Rules() {
			var enrichedRule rule

			switch rule := r.(type) {
			case *rules.AlertingRule:
				enrichedRule = alertingRule{
					Name:        rule.Name(),
					Query:       rule.Query().String(),
					Duration:    rule.Duration().Seconds(),
					Labels:      rule.Labels(),
					Annotations: rule.Annotations(),
					Alerts:      rulesAlertsToAPIAlerts(rule.ActiveAlerts()),
					Type:        "alerting",
				}
			case *rules.RecordingRule:
				enrichedRule = recordingRule{
					Name:   rule.Name(),
					Query:  rule.Query().String(),
					Labels: rule.Labels(),
					Type:   "recording",
				}
			default:
				err := fmt.Errorf("failed to assert type of rule '%v'", rule.Name())
				return nil, &apiError{errorInternal, err}, nil
			}

			apiRuleGroup.Rules = append(apiRuleGroup.Rules, enrichedRule)
		}
		res.RuleGroups[i] = apiRuleGroup
	}
	return res, nil, nil
}

type prometheusConfig struct {
	YAML string `json:"yaml"`
}

func (api *API) serveConfig(r *http.Request) (interface{}, *apiError, func()) {
	cfg := &prometheusConfig{
		YAML: api.config().String(),
	}
	return cfg, nil, nil
}

func (api *API) serveFlags(r *http.Request) (interface{}, *apiError, func()) {
	return api.flagsMap, nil, nil
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
		from, through, matchers, selectParams, err := remote.FromQuery(query)
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

		set, err := querier.Select(selectParams, filteredMatchers...)
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

func (api *API) deleteSeries(r *http.Request) (interface{}, *apiError, func()) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}, nil
	}
	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}, nil
	}

	r.ParseForm()
	if len(r.Form["match[]"]) == 0 {
		return nil, &apiError{errorBadData, fmt.Errorf("no match[] parameter provided")}, nil
	}

	var start time.Time
	if t := r.FormValue("start"); t != "" {
		var err error
		start, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
	} else {
		start = minTime
	}

	var end time.Time
	if t := r.FormValue("end"); t != "" {
		var err error
		end, err = parseTime(t)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}
	} else {
		end = maxTime
	}

	for _, s := range r.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			return nil, &apiError{errorBadData, err}, nil
		}

		var selector tsdbLabels.Selector
		for _, m := range matchers {
			selector = append(selector, convertMatcher(m))
		}

		if err := db.Delete(timestamp.FromTime(start), timestamp.FromTime(end), selector...); err != nil {
			return nil, &apiError{errorInternal, err}, nil
		}
	}

	return nil, nil, nil
}

func (api *API) snapshot(r *http.Request) (interface{}, *apiError, func()) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}, nil
	}
	skipHead, _ := strconv.ParseBool(r.FormValue("skip_head"))

	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}, nil
	}

	var (
		snapdir = filepath.Join(db.Dir(), "snapshots")
		name    = fmt.Sprintf("%s-%x",
			time.Now().UTC().Format("20060102T150405Z0700"),
			rand.Int())
		dir = filepath.Join(snapdir, name)
	)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, &apiError{errorInternal, fmt.Errorf("create snapshot directory: %s", err)}, nil
	}
	if err := db.Snapshot(dir, !skipHead); err != nil {
		return nil, &apiError{errorInternal, fmt.Errorf("create snapshot: %s", err)}, nil
	}

	return struct {
		Name string `json:"name"`
	}{name}, nil, nil
}

func (api *API) cleanTombstones(r *http.Request) (interface{}, *apiError, func()) {
	if !api.enableAdmin {
		return nil, &apiError{errorUnavailable, errors.New("Admin APIs disabled")}, nil
	}
	db := api.db()
	if db == nil {
		return nil, &apiError{errorUnavailable, errors.New("TSDB not ready")}, nil
	}

	if err := db.CleanTombstones(); err != nil {
		return nil, &apiError{errorInternal, err}, nil
	}

	return nil, nil, nil
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

func (api *API) respond(w http.ResponseWriter, data interface{}) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&response{
		Status: statusSuccess,
		Data:   data,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "error marshalling json response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if n, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "error writing response", "bytesWritten", n, "err", err)
	}
}

func (api *API) respondError(w http.ResponseWriter, apiErr *apiError, data interface{}) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "error marshalling json response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
	case errorNotFound:
		code = http.StatusNotFound
	default:
		code = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if n, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "error writing response", "bytesWritten", n, "err", err)
	}
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
