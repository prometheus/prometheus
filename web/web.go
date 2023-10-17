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

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/regexp"
	"github.com/mwitkow/go-conntrack"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/server"
	toolkit_web "github.com/prometheus/exporter-toolkit/web"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/atomic"
	"golang.org/x/net/netutil"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/util/httputil"
	api_v1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/prometheus/prometheus/web/ui"
)

// Paths that are handled by the React / Reach router that should all be served the main React app's index.html.
var reactRouterPaths = []string{
	"/config",
	"/flags",
	"/service-discovery",
	"/status",
	"/targets",
	"/starting",
}

// Paths that are handled by the React router when the Agent mode is set.
var reactRouterAgentPaths = []string{
	"/agent",
}

// Paths that are handled by the React router when the Agent mode is not set.
var reactRouterServerPaths = []string{
	"/alerts",
	"/graph",
	"/rules",
	"/tsdb-status",
}

// withStackTrace logs the stack trace in case the request panics. The function
// will re-raise the error which will then be handled by the net/http package.
// It is needed because the go-kit log package doesn't manage properly the
// panics from net/http (see https://github.com/go-kit/kit/issues/233).
func withStackTracer(h http.Handler, l log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				const size = 64 << 10
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]
				level.Error(l).Log("msg", "panic while serving request", "client", r.RemoteAddr, "url", r.URL, "err", err, "stack", buf)
				panic(err)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

type metrics struct {
	requestCounter  *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	readyStatus     prometheus.Gauge
}

func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{
		requestCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prometheus_http_requests_total",
				Help: "Counter of HTTP requests.",
			},
			[]string{"handler", "code"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "prometheus_http_request_duration_seconds",
				Help:    "Histogram of latencies for HTTP requests.",
				Buckets: []float64{.1, .2, .4, 1, 3, 8, 20, 60, 120},
			},
			[]string{"handler"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "prometheus_http_response_size_bytes",
				Help:    "Histogram of response size for HTTP requests.",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"handler"},
		),
		readyStatus: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_ready",
			Help: "Whether Prometheus startup was fully completed and the server is ready for normal operation.",
		}),
	}

	if r != nil {
		r.MustRegister(m.requestCounter, m.requestDuration, m.responseSize, m.readyStatus)
		registerFederationMetrics(r)
	}
	return m
}

func (m *metrics) instrumentHandlerWithPrefix(prefix string) func(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
	return func(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
		return m.instrumentHandler(prefix+handlerName, handler)
	}
}

func (m *metrics) instrumentHandler(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
	m.requestCounter.WithLabelValues(handlerName, "200")
	return promhttp.InstrumentHandlerCounter(
		m.requestCounter.MustCurryWith(prometheus.Labels{"handler": handlerName}),
		promhttp.InstrumentHandlerDuration(
			m.requestDuration.MustCurryWith(prometheus.Labels{"handler": handlerName}),
			promhttp.InstrumentHandlerResponseSize(
				m.responseSize.MustCurryWith(prometheus.Labels{"handler": handlerName}),
				handler,
			),
		),
	)
}

// PrometheusVersion contains build information about Prometheus.
type PrometheusVersion = api_v1.PrometheusVersion

type LocalStorage interface {
	storage.Storage
	api_v1.TSDBAdminStats
}

// Handler serves various HTTP endpoints of the Prometheus server
type Handler struct {
	logger log.Logger

	gatherer prometheus.Gatherer
	metrics  *metrics

	scrapeManager   *scrape.Manager
	ruleManager     *rules.Manager
	queryEngine     *promql.Engine
	lookbackDelta   time.Duration
	context         context.Context
	storage         storage.Storage
	localStorage    LocalStorage
	exemplarStorage storage.ExemplarQueryable
	notifier        *notifier.Manager

	apiV1 *api_v1.API

	router      *route.Router
	quitCh      chan struct{}
	quitOnce    sync.Once
	reloadCh    chan chan error
	options     *Options
	config      *config.Config
	versionInfo *PrometheusVersion
	birth       time.Time
	cwd         string
	flagsMap    map[string]string

	mtx sync.RWMutex
	now func() model.Time

	ready atomic.Uint32 // ready is uint32 rather than boolean to be able to use atomic functions.
}

// ApplyConfig updates the config field of the Handler struct
func (h *Handler) ApplyConfig(conf *config.Config) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	h.config = conf

	return nil
}

// Options for the web Handler.
type Options struct {
	Context               context.Context
	TSDBRetentionDuration model.Duration
	TSDBDir               string
	TSDBMaxBytes          units.Base2Bytes
	LocalStorage          LocalStorage
	Storage               storage.Storage
	ExemplarStorage       storage.ExemplarQueryable
	QueryEngine           *promql.Engine
	LookbackDelta         time.Duration
	ScrapeManager         *scrape.Manager
	RuleManager           *rules.Manager
	Notifier              *notifier.Manager
	Version               *PrometheusVersion
	Flags                 map[string]string

	ListenAddress              string
	CORSOrigin                 *regexp.Regexp
	ReadTimeout                time.Duration
	MaxConnections             int
	ExternalURL                *url.URL
	RoutePrefix                string
	UseLocalAssets             bool
	UserAssetsPath             string
	ConsoleTemplatesPath       string
	ConsoleLibrariesPath       string
	EnableLifecycle            bool
	EnableAdminAPI             bool
	PageTitle                  string
	RemoteReadSampleLimit      int
	RemoteReadConcurrencyLimit int
	RemoteReadBytesInFrame     int
	EnableRemoteWriteReceiver  bool
	EnableOTLPWriteReceiver    bool
	IsAgent                    bool
	AppName                    string

	Gatherer   prometheus.Gatherer
	Registerer prometheus.Registerer
}

// New initializes a new web Handler.
func New(logger log.Logger, o *Options) *Handler {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	m := newMetrics(o.Registerer)
	router := route.New().
		WithInstrumentation(m.instrumentHandler).
		WithInstrumentation(setPathWithPrefix(""))

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "<error retrieving current working directory>"
	}

	h := &Handler{
		logger: logger,

		gatherer: o.Gatherer,
		metrics:  m,

		router:      router,
		quitCh:      make(chan struct{}),
		reloadCh:    make(chan chan error),
		options:     o,
		versionInfo: o.Version,
		birth:       time.Now().UTC(),
		cwd:         cwd,
		flagsMap:    o.Flags,

		context:         o.Context,
		scrapeManager:   o.ScrapeManager,
		ruleManager:     o.RuleManager,
		queryEngine:     o.QueryEngine,
		lookbackDelta:   o.LookbackDelta,
		storage:         o.Storage,
		localStorage:    o.LocalStorage,
		exemplarStorage: o.ExemplarStorage,
		notifier:        o.Notifier,

		now: model.Now,
	}
	h.SetReady(false)

	factorySPr := func(_ context.Context) api_v1.ScrapePoolsRetriever { return h.scrapeManager }
	factoryTr := func(_ context.Context) api_v1.TargetRetriever { return h.scrapeManager }
	factoryAr := func(_ context.Context) api_v1.AlertmanagerRetriever { return h.notifier }
	FactoryRr := func(_ context.Context) api_v1.RulesRetriever { return h.ruleManager }

	var app storage.Appendable
	if o.EnableRemoteWriteReceiver || o.EnableOTLPWriteReceiver {
		app = h.storage
	}

	h.apiV1 = api_v1.NewAPI(h.queryEngine, h.storage, app, h.exemplarStorage, factorySPr, factoryTr, factoryAr,
		func() config.Config {
			h.mtx.RLock()
			defer h.mtx.RUnlock()
			return *h.config
		},
		o.Flags,
		api_v1.GlobalURLOptions{
			ListenAddress: o.ListenAddress,
			Host:          o.ExternalURL.Host,
			Scheme:        o.ExternalURL.Scheme,
		},
		h.testReady,
		h.options.LocalStorage,
		h.options.TSDBDir,
		h.options.EnableAdminAPI,
		logger,
		FactoryRr,
		h.options.RemoteReadSampleLimit,
		h.options.RemoteReadConcurrencyLimit,
		h.options.RemoteReadBytesInFrame,
		h.options.IsAgent,
		h.options.CORSOrigin,
		h.runtimeInfo,
		h.versionInfo,
		o.Gatherer,
		o.Registerer,
		nil,
		o.EnableRemoteWriteReceiver,
		o.EnableOTLPWriteReceiver,
	)

	if o.RoutePrefix != "/" {
		// If the prefix is missing for the root path, prepend it.
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, o.RoutePrefix, http.StatusFound)
		})
		router = router.WithPrefix(o.RoutePrefix)
	}

	homePage := "/graph"
	if o.IsAgent {
		homePage = "/agent"
	}

	readyf := h.testReady

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, path.Join(o.ExternalURL.Path, homePage), http.StatusFound)
	})

	// The console library examples at 'console_libraries/prom.lib' still depend on old asset files being served under `classic`.
	router.Get("/classic/static/*filepath", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = path.Join("/static", route.Param(r.Context(), "filepath"))
		fs := server.StaticFileServer(ui.Assets)
		fs.ServeHTTP(w, r)
	})

	router.Get("/version", h.version)
	router.Get("/metrics", promhttp.Handler().ServeHTTP)

	router.Get("/federate", readyf(httputil.CompressionHandler{
		Handler: http.HandlerFunc(h.federation),
	}.ServeHTTP))

	router.Get("/consoles/*filepath", readyf(h.consoles))

	serveReactApp := func(w http.ResponseWriter, r *http.Request) {
		f, err := ui.Assets.Open("/static/react/index.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error opening React index.html: %v", err)
			return
		}
		defer func() { _ = f.Close() }()
		idx, err := io.ReadAll(f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error reading React index.html: %v", err)
			return
		}
		replacedIdx := bytes.ReplaceAll(idx, []byte("CONSOLES_LINK_PLACEHOLDER"), []byte(h.consolesPath()))
		replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("TITLE_PLACEHOLDER"), []byte(h.options.PageTitle))
		replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("AGENT_MODE_PLACEHOLDER"), []byte(strconv.FormatBool(h.options.IsAgent)))
		replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("READY_PLACEHOLDER"), []byte(strconv.FormatBool(h.isReady())))
		w.Write(replacedIdx)
	}

	// Serve the React app.
	for _, p := range reactRouterPaths {
		router.Get(p, serveReactApp)
	}

	if h.options.IsAgent {
		for _, p := range reactRouterAgentPaths {
			router.Get(p, serveReactApp)
		}
	} else {
		for _, p := range reactRouterServerPaths {
			router.Get(p, serveReactApp)
		}
	}

	// The favicon and manifest are bundled as part of the React app, but we want to serve
	// them on the root.
	for _, p := range []string{"/favicon.ico", "/manifest.json"} {
		assetPath := "/static/react" + p
		router.Get(p, func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = assetPath
			fs := server.StaticFileServer(ui.Assets)
			fs.ServeHTTP(w, r)
		})
	}

	// Static files required by the React app.
	router.Get("/static/*filepath", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = path.Join("/static/react/static", route.Param(r.Context(), "filepath"))
		fs := server.StaticFileServer(ui.Assets)
		fs.ServeHTTP(w, r)
	})

	if o.UserAssetsPath != "" {
		router.Get("/user/*filepath", route.FileServe(o.UserAssetsPath))
	}

	if o.EnableLifecycle {
		router.Post("/-/quit", h.quit)
		router.Put("/-/quit", h.quit)
		router.Post("/-/reload", h.reload)
		router.Put("/-/reload", h.reload)
	} else {
		forbiddenAPINotEnabled := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Lifecycle API is not enabled."))
		}
		router.Post("/-/quit", forbiddenAPINotEnabled)
		router.Put("/-/quit", forbiddenAPINotEnabled)
		router.Post("/-/reload", forbiddenAPINotEnabled)
		router.Put("/-/reload", forbiddenAPINotEnabled)
	}
	router.Get("/-/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Only POST or PUT requests allowed"))
	})
	router.Get("/-/reload", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Only POST or PUT requests allowed"))
	})

	router.Get("/debug/*subpath", serveDebug)
	router.Post("/debug/*subpath", serveDebug)

	router.Get("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, o.AppName+" is Healthy.\n")
	})
	router.Head("/-/healthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router.Get("/-/ready", readyf(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, o.AppName+" is Ready.\n")
	}))
	router.Head("/-/ready", readyf(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	return h
}

func serveDebug(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	subpath := route.Param(ctx, "subpath")

	if subpath == "/pprof" {
		http.Redirect(w, req, req.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	if !strings.HasPrefix(subpath, "/pprof/") {
		http.NotFound(w, req)
		return
	}
	subpath = strings.TrimPrefix(subpath, "/pprof/")

	switch subpath {
	case "cmdline":
		pprof.Cmdline(w, req)
	case "profile":
		pprof.Profile(w, req)
	case "symbol":
		pprof.Symbol(w, req)
	case "trace":
		pprof.Trace(w, req)
	default:
		req.URL.Path = "/debug/pprof/" + subpath
		pprof.Index(w, req)
	}
}

// SetReady sets the ready status of our web Handler
func (h *Handler) SetReady(v bool) {
	if v {
		h.ready.Store(1)
		h.metrics.readyStatus.Set(1)
		return
	}

	h.ready.Store(0)
	h.metrics.readyStatus.Set(0)
}

// Verifies whether the server is ready or not.
func (h *Handler) isReady() bool {
	return h.ready.Load() > 0
}

// Checks if server is ready, calls f if it is, returns 503 if it is not.
func (h *Handler) testReady(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.isReady() {
			f(w, r)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Service Unavailable")
		}
	}
}

// Quit returns the receive-only quit channel.
func (h *Handler) Quit() <-chan struct{} {
	return h.quitCh
}

// Reload returns the receive-only channel that signals configuration reload requests.
func (h *Handler) Reload() <-chan chan error {
	return h.reloadCh
}

// Listener creates the TCP listener for web requests.
func (h *Handler) Listener() (net.Listener, error) {
	level.Info(h.logger).Log("msg", "Start listening for connections", "address", h.options.ListenAddress)

	listener, err := net.Listen("tcp", h.options.ListenAddress)
	if err != nil {
		return listener, err
	}
	listener = netutil.LimitListener(listener, h.options.MaxConnections)

	// Monitor incoming connections with conntrack.
	listener = conntrack.NewListener(listener,
		conntrack.TrackWithName("http"),
		conntrack.TrackWithTracing())

	return listener, nil
}

// Run serves the HTTP endpoints.
func (h *Handler) Run(ctx context.Context, listener net.Listener, webConfig string) error {
	if listener == nil {
		var err error
		listener, err = h.Listener()
		if err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", h.router)

	apiPath := "/api"
	if h.options.RoutePrefix != "/" {
		apiPath = h.options.RoutePrefix + apiPath
		level.Info(h.logger).Log("msg", "Router prefix", "prefix", h.options.RoutePrefix)
	}
	av1 := route.New().
		WithInstrumentation(h.metrics.instrumentHandlerWithPrefix("/api/v1")).
		WithInstrumentation(setPathWithPrefix(apiPath + "/v1"))
	h.apiV1.Register(av1)

	mux.Handle(apiPath+"/v1/", http.StripPrefix(apiPath+"/v1", av1))

	errlog := stdlog.New(log.NewStdlibAdapter(level.Error(h.logger)), "", 0)

	spanNameFormatter := otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	})

	httpSrv := &http.Server{
		Handler:     withStackTracer(otelhttp.NewHandler(mux, "", spanNameFormatter), h.logger),
		ErrorLog:    errlog,
		ReadTimeout: h.options.ReadTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- toolkit_web.Serve(listener, httpSrv, &toolkit_web.FlagConfig{WebConfigFile: &webConfig}, h.logger)
	}()

	select {
	case e := <-errCh:
		return e
	case <-ctx.Done():
		httpSrv.Shutdown(ctx)
		return nil
	}
}

func (h *Handler) consoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := route.Param(ctx, "filepath")

	file, err := http.Dir(h.options.ConsoleTemplatesPath).Open(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	text, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx = httputil.ContextFromRequest(ctx, r)

	// Provide URL parameters as a map for easy use. Advanced users may have need for
	// parameters beyond the first, so provide RawParams.
	rawParams, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := map[string]string{}
	for k, v := range rawParams {
		params[k] = v[0]
	}

	h.mtx.RLock()
	els := h.config.GlobalConfig.ExternalLabels
	h.mtx.RUnlock()
	externalLabels := els.Map()

	// Inject some convenience variables that are easier to remember for users
	// who are not used to Go's templating system.
	defs := []string{
		"{{$rawParams := .RawParams }}",
		"{{$params := .Params}}",
		"{{$path := .Path}}",
		"{{$externalLabels := .ExternalLabels}}",
	}

	data := struct {
		RawParams      url.Values
		Params         map[string]string
		Path           string
		ExternalLabels map[string]string
	}{
		RawParams:      rawParams,
		Params:         params,
		Path:           strings.TrimLeft(name, "/"),
		ExternalLabels: externalLabels,
	}

	tmpl := template.NewTemplateExpander(
		ctx,
		strings.Join(append(defs, string(text)), ""),
		"__console_"+name,
		data,
		h.now(),
		template.QueryFunc(rules.EngineQueryFunc(h.queryEngine, h.storage)),
		h.options.ExternalURL,
		nil,
	)
	filenames, err := filepath.Glob(h.options.ConsoleLibrariesPath + "/*.lib")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := tmpl.ExpandHTML(filenames)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, result)
}

func (h *Handler) runtimeInfo() (api_v1.RuntimeInfo, error) {
	status := api_v1.RuntimeInfo{
		StartTime:      h.birth,
		CWD:            h.cwd,
		GoroutineCount: runtime.NumGoroutine(),
		GOMAXPROCS:     runtime.GOMAXPROCS(0),
		GOMEMLIMIT:     debug.SetMemoryLimit(-1),
		GOGC:           os.Getenv("GOGC"),
		GODEBUG:        os.Getenv("GODEBUG"),
	}

	if h.options.TSDBRetentionDuration != 0 {
		status.StorageRetention = h.options.TSDBRetentionDuration.String()
	}
	if h.options.TSDBMaxBytes != 0 {
		if status.StorageRetention != "" {
			status.StorageRetention += " or "
		}
		status.StorageRetention += h.options.TSDBMaxBytes.String()
	}

	metrics, err := h.gatherer.Gather()
	if err != nil {
		return status, errors.Errorf("error gathering runtime status: %s", err)
	}
	for _, mF := range metrics {
		switch *mF.Name {
		case "prometheus_tsdb_wal_corruptions_total":
			status.CorruptionCount = int64(toFloat64(mF))
		case "prometheus_config_last_reload_successful":
			status.ReloadConfigSuccess = toFloat64(mF) != 0
		case "prometheus_config_last_reload_success_timestamp_seconds":
			status.LastConfigTime = time.Unix(int64(toFloat64(mF)), 0).UTC()
		}
	}
	return status, nil
}

func toFloat64(f *io_prometheus_client.MetricFamily) float64 {
	m := f.Metric[0]
	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	if m.Counter != nil {
		return m.Counter.GetValue()
	}
	if m.Untyped != nil {
		return m.Untyped.GetValue()
	}
	return math.NaN()
}

func (h *Handler) version(w http.ResponseWriter, _ *http.Request) {
	dec := json.NewEncoder(w)
	if err := dec.Encode(h.versionInfo); err != nil {
		http.Error(w, fmt.Sprintf("error encoding JSON: %s", err), http.StatusInternalServerError)
	}
}

func (h *Handler) quit(w http.ResponseWriter, _ *http.Request) {
	var closed bool
	h.quitOnce.Do(func() {
		closed = true
		close(h.quitCh)
		fmt.Fprintf(w, "Requesting termination... Goodbye!")
	})
	if !closed {
		fmt.Fprintf(w, "Termination already in progress.")
	}
}

func (h *Handler) reload(w http.ResponseWriter, _ *http.Request) {
	rc := make(chan error)
	h.reloadCh <- rc
	if err := <-rc; err != nil {
		http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
	}
}

func (h *Handler) consolesPath() string {
	if _, err := os.Stat(h.options.ConsoleTemplatesPath + "/index.html"); !os.IsNotExist(err) {
		return h.options.ExternalURL.Path + "/consoles/index.html"
	}
	if h.options.UserAssetsPath != "" {
		if _, err := os.Stat(h.options.UserAssetsPath + "/index.html"); !os.IsNotExist(err) {
			return h.options.ExternalURL.Path + "/user/index.html"
		}
	}
	return ""
}

func setPathWithPrefix(prefix string) func(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
	return func(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handler(w, r.WithContext(httputil.ContextWithPath(r.Context(), prefix+r.URL.Path)))
		}
	}
}
