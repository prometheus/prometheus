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
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/regexp"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/util/stats"
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

var LocalhostRepresentations = []string{"127.0.0.1", "localhost", "::1"}

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

// TargetRetriever provides the list of active/dropped targets to scrape or not.
type TargetRetriever interface {
	TargetsActive() map[string][]*scrape.Target
	TargetsDropped() map[string][]*scrape.Target
}

// AlertmanagerRetriever provides a list of all/dropped AlertManager URLs.
type AlertmanagerRetriever interface {
	Alertmanagers() []*url.URL
	DroppedAlertmanagers() []*url.URL
}

// RulesRetriever provides a list of active rules and alerts.
type RulesRetriever interface {
	RuleGroups() []*rules.Group
	AlertingRules() []*rules.AlertingRule
}

type StatsRenderer func(context.Context, *stats.Statistics, string) stats.QueryStats

func defaultStatsRenderer(ctx context.Context, s *stats.Statistics, param string) stats.QueryStats {
	if param != "" {
		return stats.NewQueryStats(s)
	}
	return nil
}

// PrometheusVersion contains build information about Prometheus.
type PrometheusVersion struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildUser string `json:"buildUser"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

// RuntimeInfo contains runtime information about Prometheus.
type RuntimeInfo struct {
	StartTime           time.Time `json:"startTime"`
	CWD                 string    `json:"CWD"`
	ReloadConfigSuccess bool      `json:"reloadConfigSuccess"`
	LastConfigTime      time.Time `json:"lastConfigTime"`
	CorruptionCount     int64     `json:"corruptionCount"`
	GoroutineCount      int       `json:"goroutineCount"`
	GOMAXPROCS          int       `json:"GOMAXPROCS"`
	GOGC                string    `json:"GOGC"`
	GODEBUG             string    `json:"GODEBUG"`
	StorageRetention    string    `json:"storageRetention"`
}

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

type apiFuncResult struct {
	data      interface{}
	err       *apiError
	warnings  storage.Warnings
	finalizer func()
}

type apiFunc func(r *http.Request) apiFuncResult

// TSDBAdminStats defines the tsdb interfaces used by the v1 API for admin operations as well as statistics.
type TSDBAdminStats interface {
	CleanTombstones() error
	Delete(mint, maxt int64, ms ...*labels.Matcher) error
	Snapshot(dir string, withHead bool) error
	Stats(statsByLabelName string) (*tsdb.Stats, error)
	WALReplayStatus() (tsdb.WALReplayStatus, error)
}

// QueryEngine defines the interface for the *promql.Engine, so it can be replaced, wrapped or mocked.
type QueryEngine interface {
	SetQueryLogger(l promql.QueryLogger)
	NewInstantQuery(q storage.Queryable, opts *promql.QueryOpts, qs string, ts time.Time) (promql.Query, error)
	NewRangeQuery(q storage.Queryable, opts *promql.QueryOpts, qs string, start, end time.Time, interval time.Duration) (promql.Query, error)
}

// API can register a set of endpoints in a router and handle
// them using the provided storage and query engine.
type API struct {
	Queryable         storage.SampleAndChunkQueryable
	QueryEngine       QueryEngine
	ExemplarQueryable storage.ExemplarQueryable

	targetRetriever       func(context.Context) TargetRetriever
	alertmanagerRetriever func(context.Context) AlertmanagerRetriever
	rulesRetriever        func(context.Context) RulesRetriever
	now                   func() time.Time
	config                func() config.Config
	flagsMap              map[string]string
	ready                 func(http.HandlerFunc) http.HandlerFunc
	globalURLOptions      GlobalURLOptions

	db            TSDBAdminStats
	dbDir         string
	enableAdmin   bool
	logger        log.Logger
	CORSOrigin    *regexp.Regexp
	buildInfo     *PrometheusVersion
	runtimeInfo   func() (RuntimeInfo, error)
	gatherer      prometheus.Gatherer
	isAgent       bool
	statsRenderer StatsRenderer

	remoteWriteHandler http.Handler
	remoteReadHandler  http.Handler
}

func init() {
	jsoniter.RegisterTypeEncoderFunc("promql.Point", marshalPointJSON, marshalPointJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("exemplar.Exemplar", marshalExemplarJSON, marshalExemplarJSONEmpty)
}

// NewAPI returns an initialized API type.
func NewAPI(
	qe QueryEngine,
	q storage.SampleAndChunkQueryable,
	ap storage.Appendable,
	eq storage.ExemplarQueryable,
	tr func(context.Context) TargetRetriever,
	ar func(context.Context) AlertmanagerRetriever,
	configFunc func() config.Config,
	flagsMap map[string]string,
	globalURLOptions GlobalURLOptions,
	readyFunc func(http.HandlerFunc) http.HandlerFunc,
	db TSDBAdminStats,
	dbDir string,
	enableAdmin bool,
	logger log.Logger,
	rr func(context.Context) RulesRetriever,
	remoteReadSampleLimit int,
	remoteReadConcurrencyLimit int,
	remoteReadMaxBytesInFrame int,
	isAgent bool,
	CORSOrigin *regexp.Regexp,
	runtimeInfo func() (RuntimeInfo, error),
	buildInfo *PrometheusVersion,
	gatherer prometheus.Gatherer,
	registerer prometheus.Registerer,
	statsRenderer StatsRenderer,
) *API {
	a := &API{
		QueryEngine:       qe,
		Queryable:         q,
		ExemplarQueryable: eq,

		targetRetriever:       tr,
		alertmanagerRetriever: ar,

		now:              time.Now,
		config:           configFunc,
		flagsMap:         flagsMap,
		ready:            readyFunc,
		globalURLOptions: globalURLOptions,
		db:               db,
		dbDir:            dbDir,
		enableAdmin:      enableAdmin,
		rulesRetriever:   rr,
		logger:           logger,
		CORSOrigin:       CORSOrigin,
		runtimeInfo:      runtimeInfo,
		buildInfo:        buildInfo,
		gatherer:         gatherer,
		isAgent:          isAgent,
		statsRenderer:    defaultStatsRenderer,

		remoteReadHandler: remote.NewReadHandler(logger, registerer, q, configFunc, remoteReadSampleLimit, remoteReadConcurrencyLimit, remoteReadMaxBytesInFrame),
	}

	if statsRenderer != nil {
		a.statsRenderer = statsRenderer
	}

	if ap != nil {
		a.remoteWriteHandler = remote.NewWriteHandler(logger, ap)
	}

	return a
}

func setUnavailStatusOnTSDBNotReady(r apiFuncResult) apiFuncResult {
	if r.err != nil && errors.Cause(r.err.err) == tsdb.ErrNotReady {
		r.err.typ = errorUnavailable
	}
	return r
}

// Register the API's endpoints in the given router.
func (api *API) Register(r *route.Router) {
	wrap := func(f apiFunc) http.HandlerFunc {
		hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httputil.SetCORS(w, api.CORSOrigin, r)
			result := setUnavailStatusOnTSDBNotReady(f(r))
			if result.finalizer != nil {
				defer result.finalizer()
			}
			if result.err != nil {
				api.respondError(w, result.err, result.data)
				return
			}

			if result.data != nil {
				api.respond(w, result.data, result.warnings)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		return api.ready(httputil.CompressionHandler{
			Handler: hf,
		}.ServeHTTP)
	}

	wrapAgent := func(f apiFunc) http.HandlerFunc {
		return wrap(func(r *http.Request) apiFuncResult {
			if api.isAgent {
				return apiFuncResult{nil, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil, nil}
			}
			return f(r)
		})
	}

	r.Options("/*path", wrap(api.options))

	r.Get("/query", wrapAgent(api.query))
	r.Post("/query", wrapAgent(api.query))
	r.Get("/query_range", wrapAgent(api.queryRange))
	r.Post("/query_range", wrapAgent(api.queryRange))
	r.Get("/query_exemplars", wrapAgent(api.queryExemplars))
	r.Post("/query_exemplars", wrapAgent(api.queryExemplars))

	r.Get("/labels", wrapAgent(api.labelNames))
	r.Post("/labels", wrapAgent(api.labelNames))
	r.Get("/label/:name/values", wrapAgent(api.labelValues))

	r.Get("/series", wrapAgent(api.series))
	r.Post("/series", wrapAgent(api.series))
	r.Del("/series", wrapAgent(api.dropSeries))

	r.Get("/targets", wrap(api.targets))
	r.Get("/targets/metadata", wrap(api.targetMetadata))
	r.Get("/alertmanagers", wrapAgent(api.alertmanagers))

	r.Get("/metadata", wrap(api.metricMetadata))

	r.Get("/status/config", wrap(api.serveConfig))
	r.Get("/status/runtimeinfo", wrap(api.serveRuntimeInfo))
	r.Get("/status/buildinfo", wrap(api.serveBuildInfo))
	r.Get("/status/flags", wrap(api.serveFlags))
	r.Get("/status/tsdb", wrapAgent(api.serveTSDBStatus))
	r.Get("/status/walreplay", api.serveWALReplayStatus)
	r.Post("/read", api.ready(api.remoteRead))
	r.Post("/write", api.ready(api.remoteWrite))

	r.Get("/alerts", wrapAgent(api.alerts))
	r.Get("/rules", wrapAgent(api.rules))

	// Admin APIs
	r.Post("/admin/tsdb/delete_series", wrapAgent(api.deleteSeries))
	r.Post("/admin/tsdb/clean_tombstones", wrapAgent(api.cleanTombstones))
	r.Post("/admin/tsdb/snapshot", wrapAgent(api.snapshot))

	r.Put("/admin/tsdb/delete_series", wrapAgent(api.deleteSeries))
	r.Put("/admin/tsdb/clean_tombstones", wrapAgent(api.cleanTombstones))
	r.Put("/admin/tsdb/snapshot", wrapAgent(api.snapshot))
}

type queryData struct {
	ResultType parser.ValueType `json:"resultType"`
	Result     parser.Value     `json:"result"`
	Stats      stats.QueryStats `json:"stats,omitempty"`
}

func invalidParamError(err error, parameter string) apiFuncResult {
	return apiFuncResult{nil, &apiError{
		errorBadData, errors.Wrapf(err, "invalid parameter %q", parameter),
	}, nil, nil}
}

func (api *API) options(r *http.Request) apiFuncResult {
	return apiFuncResult{nil, nil, nil, nil}
}

func (api *API) query(r *http.Request) (result apiFuncResult) {
	ts, err := parseTimeParam(r, "time", api.now())
	if err != nil {
		return invalidParamError(err, "time")
	}
	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return invalidParamError(err, "timeout")
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	opts := extractQueryOpts(r)
	qry, err := api.QueryEngine.NewInstantQuery(api.Queryable, opts, r.FormValue("query"), ts)
	if err != nil {
		return invalidParamError(err, "query")
	}

	// From now on, we must only return with a finalizer in the result (to
	// be called by the caller) or call qry.Close ourselves (which is
	// required in the case of a panic).
	defer func() {
		if result.finalizer == nil {
			qry.Close()
		}
	}()

	ctx = httputil.ContextFromRequest(ctx, r)

	res := qry.Exec(ctx)
	if res.Err != nil {
		return apiFuncResult{nil, returnAPIError(res.Err), res.Warnings, qry.Close}
	}

	// Optional stats field in response if parameter "stats" is not empty.
	sr := api.statsRenderer
	if sr == nil {
		sr = defaultStatsRenderer
	}
	qs := sr(ctx, qry.Stats(), r.FormValue("stats"))

	return apiFuncResult{&queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
		Stats:      qs,
	}, nil, res.Warnings, qry.Close}
}

func extractQueryOpts(r *http.Request) *promql.QueryOpts {
	return &promql.QueryOpts{
		EnablePerStepStats: r.FormValue("stats") == "all",
	}
}

func (api *API) queryRange(r *http.Request) (result apiFuncResult) {
	start, err := parseTime(r.FormValue("start"))
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTime(r.FormValue("end"))
	if err != nil {
		return invalidParamError(err, "end")
	}
	if end.Before(start) {
		return invalidParamError(errors.New("end timestamp must not be before start time"), "end")
	}

	step, err := parseDuration(r.FormValue("step"))
	if err != nil {
		return invalidParamError(err, "step")
	}

	if step <= 0 {
		return invalidParamError(errors.New("zero or negative query resolution step widths are not accepted. Try a positive integer"), "step")
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		err := errors.New("exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return invalidParamError(err, "timeout")
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	opts := extractQueryOpts(r)
	qry, err := api.QueryEngine.NewRangeQuery(api.Queryable, opts, r.FormValue("query"), start, end, step)
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}
	// From now on, we must only return with a finalizer in the result (to
	// be called by the caller) or call qry.Close ourselves (which is
	// required in the case of a panic).
	defer func() {
		if result.finalizer == nil {
			qry.Close()
		}
	}()

	ctx = httputil.ContextFromRequest(ctx, r)

	res := qry.Exec(ctx)
	if res.Err != nil {
		return apiFuncResult{nil, returnAPIError(res.Err), res.Warnings, qry.Close}
	}

	// Optional stats field in response if parameter "stats" is not empty.
	sr := api.statsRenderer
	if sr == nil {
		sr = defaultStatsRenderer
	}
	qs := sr(ctx, qry.Stats(), r.FormValue("stats"))

	return apiFuncResult{&queryData{
		ResultType: res.Value.Type(),
		Result:     res.Value,
		Stats:      qs,
	}, nil, res.Warnings, qry.Close}
}

func (api *API) queryExemplars(r *http.Request) apiFuncResult {
	start, err := parseTimeParam(r, "start", minTime)
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTimeParam(r, "end", maxTime)
	if err != nil {
		return invalidParamError(err, "end")
	}
	if end.Before(start) {
		err := errors.New("end timestamp must not be before start timestamp")
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	expr, err := parser.ParseExpr(r.FormValue("query"))
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	selectors := parser.ExtractSelectors(expr)
	if len(selectors) < 1 {
		return apiFuncResult{nil, nil, nil, nil}
	}

	ctx := r.Context()
	eq, err := api.ExemplarQueryable.ExemplarQuerier(ctx)
	if err != nil {
		return apiFuncResult{nil, &apiError{errorExec, err}, nil, nil}
	}

	res, err := eq.Select(timestamp.FromTime(start), timestamp.FromTime(end), selectors...)
	if err != nil {
		return apiFuncResult{nil, &apiError{errorExec, err}, nil, nil}
	}

	return apiFuncResult{res, nil, nil, nil}
}

func returnAPIError(err error) *apiError {
	if err == nil {
		return nil
	}

	cause := errors.Unwrap(err)
	if cause == nil {
		cause = err
	}

	switch cause.(type) {
	case promql.ErrQueryCanceled:
		return &apiError{errorCanceled, err}
	case promql.ErrQueryTimeout:
		return &apiError{errorTimeout, err}
	case promql.ErrStorage:
		return &apiError{errorInternal, err}
	}

	return &apiError{errorExec, err}
}

func (api *API) labelNames(r *http.Request) apiFuncResult {
	start, err := parseTimeParam(r, "start", minTime)
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTimeParam(r, "end", maxTime)
	if err != nil {
		return invalidParamError(err, "end")
	}

	matcherSets, err := parseMatchersParam(r.Form["match[]"])
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return apiFuncResult{nil, &apiError{errorExec, err}, nil, nil}
	}
	defer q.Close()

	var (
		names    []string
		warnings storage.Warnings
	)
	if len(matcherSets) > 0 {
		labelNamesSet := make(map[string]struct{})

		for _, matchers := range matcherSets {
			vals, callWarnings, err := q.LabelNames(matchers...)
			if err != nil {
				return apiFuncResult{nil, &apiError{errorExec, err}, warnings, nil}
			}

			warnings = append(warnings, callWarnings...)
			for _, val := range vals {
				labelNamesSet[val] = struct{}{}
			}
		}

		// Convert the map to an array.
		names = make([]string, 0, len(labelNamesSet))
		for key := range labelNamesSet {
			names = append(names, key)
		}
		sort.Strings(names)
	} else {
		names, warnings, err = q.LabelNames()
		if err != nil {
			return apiFuncResult{nil, &apiError{errorExec, err}, warnings, nil}
		}
	}

	if names == nil {
		names = []string{}
	}
	return apiFuncResult{names, nil, warnings, nil}
}

func (api *API) labelValues(r *http.Request) (result apiFuncResult) {
	ctx := r.Context()
	name := route.Param(ctx, "name")

	if !model.LabelNameRE.MatchString(name) {
		return apiFuncResult{nil, &apiError{errorBadData, errors.Errorf("invalid label name: %q", name)}, nil, nil}
	}

	start, err := parseTimeParam(r, "start", minTime)
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTimeParam(r, "end", maxTime)
	if err != nil {
		return invalidParamError(err, "end")
	}

	matcherSets, err := parseMatchersParam(r.Form["match[]"])
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return apiFuncResult{nil, &apiError{errorExec, err}, nil, nil}
	}
	// From now on, we must only return with a finalizer in the result (to
	// be called by the caller) or call q.Close ourselves (which is required
	// in the case of a panic).
	defer func() {
		if result.finalizer == nil {
			q.Close()
		}
	}()
	closer := func() {
		q.Close()
	}

	var (
		vals     []string
		warnings storage.Warnings
	)
	if len(matcherSets) > 0 {
		var callWarnings storage.Warnings
		labelValuesSet := make(map[string]struct{})
		for _, matchers := range matcherSets {
			vals, callWarnings, err = q.LabelValues(name, matchers...)
			if err != nil {
				return apiFuncResult{nil, &apiError{errorExec, err}, warnings, closer}
			}
			warnings = append(warnings, callWarnings...)
			for _, val := range vals {
				labelValuesSet[val] = struct{}{}
			}
		}

		vals = make([]string, 0, len(labelValuesSet))
		for val := range labelValuesSet {
			vals = append(vals, val)
		}
	} else {
		vals, warnings, err = q.LabelValues(name)
		if err != nil {
			return apiFuncResult{nil, &apiError{errorExec, err}, warnings, closer}
		}

		if vals == nil {
			vals = []string{}
		}
	}

	sort.Strings(vals)

	return apiFuncResult{vals, nil, warnings, closer}
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()

	minTimeFormatted = minTime.Format(time.RFC3339Nano)
	maxTimeFormatted = maxTime.Format(time.RFC3339Nano)
)

func (api *API) series(r *http.Request) (result apiFuncResult) {
	if err := r.ParseForm(); err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, errors.Wrapf(err, "error parsing form values")}, nil, nil}
	}
	if len(r.Form["match[]"]) == 0 {
		return apiFuncResult{nil, &apiError{errorBadData, errors.New("no match[] parameter provided")}, nil, nil}
	}

	start, err := parseTimeParam(r, "start", minTime)
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTimeParam(r, "end", maxTime)
	if err != nil {
		return invalidParamError(err, "end")
	}

	matcherSets, err := parseMatchersParam(r.Form["match[]"])
	if err != nil {
		return invalidParamError(err, "match[]")
	}

	q, err := api.Queryable.Querier(r.Context(), timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return apiFuncResult{nil, &apiError{errorExec, err}, nil, nil}
	}
	// From now on, we must only return with a finalizer in the result (to
	// be called by the caller) or call q.Close ourselves (which is required
	// in the case of a panic).
	defer func() {
		if result.finalizer == nil {
			q.Close()
		}
	}()
	closer := func() {
		q.Close()
	}

	hints := &storage.SelectHints{
		Start: timestamp.FromTime(start),
		End:   timestamp.FromTime(end),
		Func:  "series", // There is no series function, this token is used for lookups that don't need samples.
	}

	var sets []storage.SeriesSet
	for _, mset := range matcherSets {
		// We need to sort this select results to merge (deduplicate) the series sets later.
		s := q.Select(true, hints, mset...)
		sets = append(sets, s)
	}

	set := storage.NewMergeSeriesSet(sets, storage.ChainedSeriesMerge)
	metrics := []labels.Labels{}
	for set.Next() {
		metrics = append(metrics, set.At().Labels())
	}

	warnings := set.Warnings()
	if set.Err() != nil {
		return apiFuncResult{nil, &apiError{errorExec, set.Err()}, warnings, closer}
	}

	return apiFuncResult{metrics, nil, warnings, closer}
}

func (api *API) dropSeries(_ *http.Request) apiFuncResult {
	return apiFuncResult{nil, &apiError{errorInternal, errors.New("not implemented")}, nil, nil}
}

// Target has the information for one target.
type Target struct {
	// Labels before any processing.
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
	// Any labels that are added to this target and its metrics.
	Labels map[string]string `json:"labels"`

	ScrapePool string `json:"scrapePool"`
	ScrapeURL  string `json:"scrapeUrl"`
	GlobalURL  string `json:"globalUrl"`

	LastError          string              `json:"lastError"`
	LastScrape         time.Time           `json:"lastScrape"`
	LastScrapeDuration float64             `json:"lastScrapeDuration"`
	Health             scrape.TargetHealth `json:"health"`

	ScrapeInterval string `json:"scrapeInterval"`
	ScrapeTimeout  string `json:"scrapeTimeout"`
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

// GlobalURLOptions contains fields used for deriving the global URL for local targets.
type GlobalURLOptions struct {
	ListenAddress string
	Host          string
	Scheme        string
}

// sanitizeSplitHostPort acts like net.SplitHostPort.
// Additionally, if there is no port in the host passed as input, we return the
// original host, making sure that IPv6 addresses are not surrounded by square
// brackets.
func sanitizeSplitHostPort(input string) (string, string, error) {
	host, port, err := net.SplitHostPort(input)
	if err != nil && strings.HasSuffix(err.Error(), "missing port in address") {
		var errWithPort error
		host, _, errWithPort = net.SplitHostPort(input + ":80")
		if errWithPort == nil {
			err = nil
		}
	}
	return host, port, err
}

func getGlobalURL(u *url.URL, opts GlobalURLOptions) (*url.URL, error) {
	host, port, err := sanitizeSplitHostPort(u.Host)
	if err != nil {
		return u, err
	}

	for _, lhr := range LocalhostRepresentations {
		if host == lhr {
			_, ownPort, err := net.SplitHostPort(opts.ListenAddress)
			if err != nil {
				return u, err
			}

			if port == ownPort {
				// Only in the case where the target is on localhost and its port is
				// the same as the one we're listening on, we know for sure that
				// we're monitoring our own process and that we need to change the
				// scheme, hostname, and port to the externally reachable ones as
				// well. We shouldn't need to touch the path at all, since if a
				// path prefix is defined, the path under which we scrape ourselves
				// should already contain the prefix.
				u.Scheme = opts.Scheme
				u.Host = opts.Host
			} else {
				// Otherwise, we only know that localhost is not reachable
				// externally, so we replace only the hostname by the one in the
				// external URL. It could be the wrong hostname for the service on
				// this port, but it's still the best possible guess.
				host, _, err := sanitizeSplitHostPort(opts.Host)
				if err != nil {
					return u, err
				}
				u.Host = host
				if port != "" {
					u.Host = net.JoinHostPort(u.Host, port)
				}
			}
			break
		}
	}

	return u, nil
}

func (api *API) targets(r *http.Request) apiFuncResult {
	sortKeys := func(targets map[string][]*scrape.Target) ([]string, int) {
		var n int
		keys := make([]string, 0, len(targets))
		for k := range targets {
			keys = append(keys, k)
			n += len(targets[k])
		}
		sort.Strings(keys)
		return keys, n
	}

	flatten := func(targets map[string][]*scrape.Target) []*scrape.Target {
		keys, n := sortKeys(targets)
		res := make([]*scrape.Target, 0, n)
		for _, k := range keys {
			res = append(res, targets[k]...)
		}
		return res
	}

	state := strings.ToLower(r.URL.Query().Get("state"))
	showActive := state == "" || state == "any" || state == "active"
	showDropped := state == "" || state == "any" || state == "dropped"
	res := &TargetDiscovery{}

	if showActive {
		targetsActive := api.targetRetriever(r.Context()).TargetsActive()
		activeKeys, numTargets := sortKeys(targetsActive)
		res.ActiveTargets = make([]*Target, 0, numTargets)

		for _, key := range activeKeys {
			for _, target := range targetsActive[key] {
				lastErrStr := ""
				lastErr := target.LastError()
				if lastErr != nil {
					lastErrStr = lastErr.Error()
				}

				globalURL, err := getGlobalURL(target.URL(), api.globalURLOptions)

				res.ActiveTargets = append(res.ActiveTargets, &Target{
					DiscoveredLabels: target.DiscoveredLabels().Map(),
					Labels:           target.Labels().Map(),
					ScrapePool:       key,
					ScrapeURL:        target.URL().String(),
					GlobalURL:        globalURL.String(),
					LastError: func() string {
						if err == nil && lastErrStr == "" {
							return ""
						} else if err != nil {
							return errors.Wrapf(err, lastErrStr).Error()
						}
						return lastErrStr
					}(),
					LastScrape:         target.LastScrape(),
					LastScrapeDuration: target.LastScrapeDuration().Seconds(),
					Health:             target.Health(),
					ScrapeInterval:     target.GetValue(model.ScrapeIntervalLabel),
					ScrapeTimeout:      target.GetValue(model.ScrapeTimeoutLabel),
				})
			}
		}
	} else {
		res.ActiveTargets = []*Target{}
	}
	if showDropped {
		tDropped := flatten(api.targetRetriever(r.Context()).TargetsDropped())
		res.DroppedTargets = make([]*DroppedTarget, 0, len(tDropped))
		for _, t := range tDropped {
			res.DroppedTargets = append(res.DroppedTargets, &DroppedTarget{
				DiscoveredLabels: t.DiscoveredLabels().Map(),
			})
		}
	} else {
		res.DroppedTargets = []*DroppedTarget{}
	}
	return apiFuncResult{res, nil, nil, nil}
}

func matchLabels(lset labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(lset.Get(m.Name)) {
			return false
		}
	}
	return true
}

func (api *API) targetMetadata(r *http.Request) apiFuncResult {
	limit := -1
	if s := r.FormValue("limit"); s != "" {
		var err error
		if limit, err = strconv.Atoi(s); err != nil {
			return apiFuncResult{nil, &apiError{errorBadData, errors.New("limit must be a number")}, nil, nil}
		}
	}

	matchTarget := r.FormValue("match_target")
	var matchers []*labels.Matcher
	var err error
	if matchTarget != "" {
		matchers, err = parser.ParseMetricSelector(matchTarget)
		if err != nil {
			return invalidParamError(err, "match_target")
		}
	}

	metric := r.FormValue("metric")
	res := []metricMetadata{}
	for _, tt := range api.targetRetriever(r.Context()).TargetsActive() {
		for _, t := range tt {
			if limit >= 0 && len(res) >= limit {
				break
			}
			// Filter targets that don't satisfy the label matchers.
			if matchTarget != "" && !matchLabels(t.Labels(), matchers) {
				continue
			}
			// If no metric is specified, get the full list for the target.
			if metric == "" {
				for _, md := range t.MetadataList() {
					res = append(res, metricMetadata{
						Target: t.Labels(),
						Metric: md.Metric,
						Type:   md.Type,
						Help:   md.Help,
						Unit:   md.Unit,
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
					Unit:   md.Unit,
				})
			}
		}
	}

	return apiFuncResult{res, nil, nil, nil}
}

type metricMetadata struct {
	Target labels.Labels        `json:"target"`
	Metric string               `json:"metric,omitempty"`
	Type   textparse.MetricType `json:"type"`
	Help   string               `json:"help"`
	Unit   string               `json:"unit"`
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

func (api *API) alertmanagers(r *http.Request) apiFuncResult {
	urls := api.alertmanagerRetriever(r.Context()).Alertmanagers()
	droppedURLS := api.alertmanagerRetriever(r.Context()).DroppedAlertmanagers()
	ams := &AlertmanagerDiscovery{ActiveAlertmanagers: make([]*AlertmanagerTarget, len(urls)), DroppedAlertmanagers: make([]*AlertmanagerTarget, len(droppedURLS))}
	for i, url := range urls {
		ams.ActiveAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	for i, url := range droppedURLS {
		ams.DroppedAlertmanagers[i] = &AlertmanagerTarget{URL: url.String()}
	}
	return apiFuncResult{ams, nil, nil, nil}
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
	Value       string        `json:"value"`
}

func (api *API) alerts(r *http.Request) apiFuncResult {
	alertingRules := api.rulesRetriever(r.Context()).AlertingRules()
	alerts := []*Alert{}

	for _, alertingRule := range alertingRules {
		alerts = append(
			alerts,
			rulesAlertsToAPIAlerts(alertingRule.ActiveAlerts())...,
		)
	}

	res := &AlertDiscovery{Alerts: alerts}

	return apiFuncResult{res, nil, nil, nil}
}

func rulesAlertsToAPIAlerts(rulesAlerts []*rules.Alert) []*Alert {
	apiAlerts := make([]*Alert, len(rulesAlerts))
	for i, ruleAlert := range rulesAlerts {
		apiAlerts[i] = &Alert{
			Labels:      ruleAlert.Labels,
			Annotations: ruleAlert.Annotations,
			State:       ruleAlert.State.String(),
			ActiveAt:    &ruleAlert.ActiveAt,
			Value:       strconv.FormatFloat(ruleAlert.Value, 'e', -1, 64),
		}
	}

	return apiAlerts
}

type metadata struct {
	Type textparse.MetricType `json:"type"`
	Help string               `json:"help"`
	Unit string               `json:"unit"`
}

func (api *API) metricMetadata(r *http.Request) apiFuncResult {
	metrics := map[string]map[metadata]struct{}{}

	limit := -1
	if s := r.FormValue("limit"); s != "" {
		var err error
		if limit, err = strconv.Atoi(s); err != nil {
			return apiFuncResult{nil, &apiError{errorBadData, errors.New("limit must be a number")}, nil, nil}
		}
	}

	metric := r.FormValue("metric")
	for _, tt := range api.targetRetriever(r.Context()).TargetsActive() {
		for _, t := range tt {

			if metric == "" {
				for _, mm := range t.MetadataList() {
					m := metadata{Type: mm.Type, Help: mm.Help, Unit: mm.Unit}
					ms, ok := metrics[mm.Metric]

					if !ok {
						ms = map[metadata]struct{}{}
						metrics[mm.Metric] = ms
					}
					ms[m] = struct{}{}
				}
				continue
			}

			if md, ok := t.Metadata(metric); ok {
				m := metadata{Type: md.Type, Help: md.Help, Unit: md.Unit}
				ms, ok := metrics[md.Metric]

				if !ok {
					ms = map[metadata]struct{}{}
					metrics[md.Metric] = ms
				}
				ms[m] = struct{}{}
			}
		}
	}

	// Put the elements from the pseudo-set into a slice for marshaling.
	res := map[string][]metadata{}
	for name, set := range metrics {
		if limit >= 0 && len(res) >= limit {
			break
		}

		s := []metadata{}
		for metadata := range set {
			s = append(s, metadata)
		}
		res[name] = s
	}

	return apiFuncResult{res, nil, nil, nil}
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
	Rules          []Rule    `json:"rules"`
	Interval       float64   `json:"interval"`
	Limit          int       `json:"limit"`
	EvaluationTime float64   `json:"evaluationTime"`
	LastEvaluation time.Time `json:"lastEvaluation"`
}

type Rule interface{}

type AlertingRule struct {
	// State can be "pending", "firing", "inactive".
	State          string           `json:"state"`
	Name           string           `json:"name"`
	Query          string           `json:"query"`
	Duration       float64          `json:"duration"`
	Labels         labels.Labels    `json:"labels"`
	Annotations    labels.Labels    `json:"annotations"`
	Alerts         []*Alert         `json:"alerts"`
	Health         rules.RuleHealth `json:"health"`
	LastError      string           `json:"lastError,omitempty"`
	EvaluationTime float64          `json:"evaluationTime"`
	LastEvaluation time.Time        `json:"lastEvaluation"`
	// Type of an alertingRule is always "alerting".
	Type string `json:"type"`
}

type RecordingRule struct {
	Name           string           `json:"name"`
	Query          string           `json:"query"`
	Labels         labels.Labels    `json:"labels,omitempty"`
	Health         rules.RuleHealth `json:"health"`
	LastError      string           `json:"lastError,omitempty"`
	EvaluationTime float64          `json:"evaluationTime"`
	LastEvaluation time.Time        `json:"lastEvaluation"`
	// Type of a recordingRule is always "recording".
	Type string `json:"type"`
}

func (api *API) rules(r *http.Request) apiFuncResult {
	ruleGroups := api.rulesRetriever(r.Context()).RuleGroups()
	res := &RuleDiscovery{RuleGroups: make([]*RuleGroup, len(ruleGroups))}
	typ := strings.ToLower(r.URL.Query().Get("type"))

	if typ != "" && typ != "alert" && typ != "record" {
		return invalidParamError(errors.Errorf("not supported value %q", typ), "type")
	}

	returnAlerts := typ == "" || typ == "alert"
	returnRecording := typ == "" || typ == "record"

	for i, grp := range ruleGroups {
		apiRuleGroup := &RuleGroup{
			Name:           grp.Name(),
			File:           grp.File(),
			Interval:       grp.Interval().Seconds(),
			Limit:          grp.Limit(),
			Rules:          []Rule{},
			EvaluationTime: grp.GetEvaluationTime().Seconds(),
			LastEvaluation: grp.GetLastEvaluation(),
		}
		for _, r := range grp.Rules() {
			var enrichedRule Rule

			lastError := ""
			if r.LastError() != nil {
				lastError = r.LastError().Error()
			}
			switch rule := r.(type) {
			case *rules.AlertingRule:
				if !returnAlerts {
					break
				}
				enrichedRule = AlertingRule{
					State:          rule.State().String(),
					Name:           rule.Name(),
					Query:          rule.Query().String(),
					Duration:       rule.HoldDuration().Seconds(),
					Labels:         rule.Labels(),
					Annotations:    rule.Annotations(),
					Alerts:         rulesAlertsToAPIAlerts(rule.ActiveAlerts()),
					Health:         rule.Health(),
					LastError:      lastError,
					EvaluationTime: rule.GetEvaluationDuration().Seconds(),
					LastEvaluation: rule.GetEvaluationTimestamp(),
					Type:           "alerting",
				}
			case *rules.RecordingRule:
				if !returnRecording {
					break
				}
				enrichedRule = RecordingRule{
					Name:           rule.Name(),
					Query:          rule.Query().String(),
					Labels:         rule.Labels(),
					Health:         rule.Health(),
					LastError:      lastError,
					EvaluationTime: rule.GetEvaluationDuration().Seconds(),
					LastEvaluation: rule.GetEvaluationTimestamp(),
					Type:           "recording",
				}
			default:
				err := errors.Errorf("failed to assert type of rule '%v'", rule.Name())
				return apiFuncResult{nil, &apiError{errorInternal, err}, nil, nil}
			}
			if enrichedRule != nil {
				apiRuleGroup.Rules = append(apiRuleGroup.Rules, enrichedRule)
			}
		}
		res.RuleGroups[i] = apiRuleGroup
	}
	return apiFuncResult{res, nil, nil, nil}
}

type prometheusConfig struct {
	YAML string `json:"yaml"`
}

func (api *API) serveRuntimeInfo(_ *http.Request) apiFuncResult {
	status, err := api.runtimeInfo()
	if err != nil {
		return apiFuncResult{status, &apiError{errorInternal, err}, nil, nil}
	}
	return apiFuncResult{status, nil, nil, nil}
}

func (api *API) serveBuildInfo(_ *http.Request) apiFuncResult {
	return apiFuncResult{api.buildInfo, nil, nil, nil}
}

func (api *API) serveConfig(_ *http.Request) apiFuncResult {
	cfg := &prometheusConfig{
		YAML: api.config().String(),
	}
	return apiFuncResult{cfg, nil, nil, nil}
}

func (api *API) serveFlags(_ *http.Request) apiFuncResult {
	return apiFuncResult{api.flagsMap, nil, nil, nil}
}

// TSDBStat holds the information about individual cardinality.
type TSDBStat struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// HeadStats has information about the TSDB head.
type HeadStats struct {
	NumSeries     uint64 `json:"numSeries"`
	NumLabelPairs int    `json:"numLabelPairs"`
	ChunkCount    int64  `json:"chunkCount"`
	MinTime       int64  `json:"minTime"`
	MaxTime       int64  `json:"maxTime"`
}

// TSDBStatus has information of cardinality statistics from postings.
type TSDBStatus struct {
	HeadStats                   HeadStats  `json:"headStats"`
	SeriesCountByMetricName     []TSDBStat `json:"seriesCountByMetricName"`
	LabelValueCountByLabelName  []TSDBStat `json:"labelValueCountByLabelName"`
	MemoryInBytesByLabelName    []TSDBStat `json:"memoryInBytesByLabelName"`
	SeriesCountByLabelValuePair []TSDBStat `json:"seriesCountByLabelValuePair"`
}

// TSDBStatsFromIndexStats converts a index.Stat slice to a TSDBStat slice.
func TSDBStatsFromIndexStats(stats []index.Stat) []TSDBStat {
	result := make([]TSDBStat, 0, len(stats))
	for _, item := range stats {
		item := TSDBStat{Name: item.Name, Value: item.Count}
		result = append(result, item)
	}
	return result
}

func (api *API) serveTSDBStatus(*http.Request) apiFuncResult {
	s, err := api.db.Stats(labels.MetricName)
	if err != nil {
		return apiFuncResult{nil, &apiError{errorInternal, err}, nil, nil}
	}
	metrics, err := api.gatherer.Gather()
	if err != nil {
		return apiFuncResult{nil, &apiError{errorInternal, fmt.Errorf("error gathering runtime status: %s", err)}, nil, nil}
	}
	chunkCount := int64(math.NaN())
	for _, mF := range metrics {
		if *mF.Name == "prometheus_tsdb_head_chunks" {
			m := *mF.Metric[0]
			if m.Gauge != nil {
				chunkCount = int64(m.Gauge.GetValue())
				break
			}
		}
	}
	return apiFuncResult{TSDBStatus{
		HeadStats: HeadStats{
			NumSeries:     s.NumSeries,
			ChunkCount:    chunkCount,
			MinTime:       s.MinTime,
			MaxTime:       s.MaxTime,
			NumLabelPairs: s.IndexPostingStats.NumLabelPairs,
		},
		SeriesCountByMetricName:     TSDBStatsFromIndexStats(s.IndexPostingStats.CardinalityMetricsStats),
		LabelValueCountByLabelName:  TSDBStatsFromIndexStats(s.IndexPostingStats.CardinalityLabelStats),
		MemoryInBytesByLabelName:    TSDBStatsFromIndexStats(s.IndexPostingStats.LabelValueStats),
		SeriesCountByLabelValuePair: TSDBStatsFromIndexStats(s.IndexPostingStats.LabelValuePairsStats),
	}, nil, nil, nil}
}

type walReplayStatus struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Current int `json:"current"`
}

func (api *API) serveWALReplayStatus(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)
	status, err := api.db.WALReplayStatus()
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
	}
	api.respond(w, walReplayStatus{
		Min:     status.Min,
		Max:     status.Max,
		Current: status.Current,
	}, nil)
}

func (api *API) remoteRead(w http.ResponseWriter, r *http.Request) {
	// This is only really for tests - this will never be nil IRL.
	if api.remoteReadHandler != nil {
		api.remoteReadHandler.ServeHTTP(w, r)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (api *API) remoteWrite(w http.ResponseWriter, r *http.Request) {
	if api.remoteWriteHandler != nil {
		api.remoteWriteHandler.ServeHTTP(w, r)
	} else {
		http.Error(w, "remote write receiver needs to be enabled with --enable-feature=remote-write-receiver", http.StatusNotFound)
	}
}

func (api *API) deleteSeries(r *http.Request) apiFuncResult {
	if !api.enableAdmin {
		return apiFuncResult{nil, &apiError{errorUnavailable, errors.New("admin APIs disabled")}, nil, nil}
	}
	if err := r.ParseForm(); err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, errors.Wrap(err, "error parsing form values")}, nil, nil}
	}
	if len(r.Form["match[]"]) == 0 {
		return apiFuncResult{nil, &apiError{errorBadData, errors.New("no match[] parameter provided")}, nil, nil}
	}

	start, err := parseTimeParam(r, "start", minTime)
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTimeParam(r, "end", maxTime)
	if err != nil {
		return invalidParamError(err, "end")
	}

	for _, s := range r.Form["match[]"] {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return invalidParamError(err, "match[]")
		}
		if err := api.db.Delete(timestamp.FromTime(start), timestamp.FromTime(end), matchers...); err != nil {
			return apiFuncResult{nil, &apiError{errorInternal, err}, nil, nil}
		}
	}

	return apiFuncResult{nil, nil, nil, nil}
}

func (api *API) snapshot(r *http.Request) apiFuncResult {
	if !api.enableAdmin {
		return apiFuncResult{nil, &apiError{errorUnavailable, errors.New("admin APIs disabled")}, nil, nil}
	}
	var (
		skipHead bool
		err      error
	)
	if r.FormValue("skip_head") != "" {
		skipHead, err = strconv.ParseBool(r.FormValue("skip_head"))
		if err != nil {
			return invalidParamError(errors.Wrapf(err, "unable to parse boolean"), "skip_head")
		}
	}

	var (
		snapdir = filepath.Join(api.dbDir, "snapshots")
		name    = fmt.Sprintf("%s-%016x",
			time.Now().UTC().Format("20060102T150405Z0700"),
			rand.Int63())
		dir = filepath.Join(snapdir, name)
	)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return apiFuncResult{nil, &apiError{errorInternal, errors.Wrap(err, "create snapshot directory")}, nil, nil}
	}
	if err := api.db.Snapshot(dir, !skipHead); err != nil {
		return apiFuncResult{nil, &apiError{errorInternal, errors.Wrap(err, "create snapshot")}, nil, nil}
	}

	return apiFuncResult{struct {
		Name string `json:"name"`
	}{name}, nil, nil, nil}
}

func (api *API) cleanTombstones(r *http.Request) apiFuncResult {
	if !api.enableAdmin {
		return apiFuncResult{nil, &apiError{errorUnavailable, errors.New("admin APIs disabled")}, nil, nil}
	}
	if err := api.db.CleanTombstones(); err != nil {
		return apiFuncResult{nil, &apiError{errorInternal, err}, nil, nil}
	}

	return apiFuncResult{nil, nil, nil, nil}
}

func (api *API) respond(w http.ResponseWriter, data interface{}, warnings storage.Warnings) {
	statusMessage := statusSuccess
	var warningStrings []string
	for _, warning := range warnings {
		warningStrings = append(warningStrings, warning.Error())
	}
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&response{
		Status:   statusMessage,
		Data:     data,
		Warnings: warningStrings,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "error marshaling json response", "err", err)
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
		level.Error(api.logger).Log("msg", "error marshaling json response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var code int
	switch apiErr.typ {
	case errorBadData:
		code = http.StatusBadRequest
	case errorExec:
		code = http.StatusUnprocessableEntity
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

func parseTimeParam(r *http.Request, paramName string, defaultValue time.Time) (time.Time, error) {
	val := r.FormValue(paramName)
	if val == "" {
		return defaultValue, nil
	}
	result, err := parseTime(val)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "Invalid time value for '%s'", paramName)
	}
	return result, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	// Stdlib's time parser can only handle 4 digit years. As a workaround until
	// that is fixed we want to at least support our own boundary times.
	// Context: https://github.com/prometheus/client_golang/issues/614
	// Upstream issue: https://github.com/golang/go/issues/20555
	switch s {
	case minTimeFormatted:
		return minTime, nil
	case maxTimeFormatted:
		return maxTime, nil
	}
	return time.Time{}, errors.Errorf("cannot parse %q to a valid timestamp", s)
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		ts := d * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return 0, errors.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		return time.Duration(ts), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, errors.Errorf("cannot parse %q to a valid duration", s)
}

func parseMatchersParam(matchers []string) ([][]*labels.Matcher, error) {
	var matcherSets [][]*labels.Matcher
	for _, s := range matchers {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return nil, err
		}
		matcherSets = append(matcherSets, matchers)
	}

OUTER:
	for _, ms := range matcherSets {
		for _, lm := range ms {
			if lm != nil && !lm.Matches("") {
				continue OUTER
			}
		}
		return nil, errors.New("match[] must contain at least one non-empty matcher")
	}
	return matcherSets, nil
}

// marshalPointJSON writes `[ts, "val"]`.
func marshalPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.Point)(ptr))
	stream.WriteArrayStart()
	marshalTimestamp(p.T, stream)
	stream.WriteMore()
	marshalValue(p.V, stream)
	stream.WriteArrayEnd()
}

func marshalPointJSONIsEmpty(ptr unsafe.Pointer) bool {
	return false
}

// marshalExemplarJSON writes.
// {
//    labels: <labels>,
//    value: "<string>",
//    timestamp: <float>
// }
func marshalExemplarJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*exemplar.Exemplar)(ptr))
	stream.WriteObjectStart()

	// "labels" key.
	stream.WriteObjectField(`labels`)
	lbls, err := p.Labels.MarshalJSON()
	if err != nil {
		stream.Error = err
		return
	}
	stream.SetBuffer(append(stream.Buffer(), lbls...))

	// "value" key.
	stream.WriteMore()
	stream.WriteObjectField(`value`)
	marshalValue(p.Value, stream)

	// "timestamp" key.
	stream.WriteMore()
	stream.WriteObjectField(`timestamp`)
	marshalTimestamp(p.Ts, stream)
	// marshalTimestamp(p.Ts, stream)

	stream.WriteObjectEnd()
}

func marshalExemplarJSONEmpty(ptr unsafe.Pointer) bool {
	return false
}

func marshalTimestamp(t int64, stream *jsoniter.Stream) {
	// Write out the timestamp as a float divided by 1000.
	// This is ~3x faster than converting to a float.
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
}

func marshalValue(v float64, stream *jsoniter.Stream) {
	stream.WriteRaw(`"`)
	// Taken from https://github.com/json-iterator/go/blob/master/stream_float.go#L71 as a workaround
	// to https://github.com/json-iterator/go/issues/365 (jsoniter, to follow json standard, doesn't allow inf/nan).
	buf := stream.Buffer()
	abs := math.Abs(v)
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if abs < 1e-6 || abs >= 1e21 {
			fmt = 'e'
		}
	}
	buf = strconv.AppendFloat(buf, v, fmt, -1, 64)
	stream.SetBuffer(buf)
	stream.WriteRaw(`"`)
}
